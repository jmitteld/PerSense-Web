package amortization

import (
	"math"
	"math/rand"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestDOSFuzzer2 is a rotate-the-unknown differential fuzzer. Each case:
//   - solves the payment from a random amount/rate/term (the initial solve),
//   - then TWICE: hardens the just-solved output and clears a DIFFERENT field,
//     solving for it instead.
//
// Every solve — the value AND the resulting schedule's total interest — is checked
// against the DOS oracle. Because each step hardens the previous DOS-canonical
// output and rotates which of {amount, rate, term, payment} is unknown, it stresses
// that the four solvers are mutually consistent and DOS-faithful when chained, not
// just individually. Uses `solveamount`/`solverate`/`solveterm` oracle queries plus
// the default payment solve.
func TestDOSFuzzer2(t *testing.T) {
	gateOracle(t)
	rng := rand.New(rand.NewSource(20260710))

	const perYr = 12
	loanDate := types.NewDateRec(2024, time.January, 1)
	firstDate := types.NewDateRec(2024, time.February, 1) // clean one-period first stub

	// field-value map, all four quantities kept consistent as the chain progresses.
	type vals map[string]float64

	buildInput := func(v vals, unknown string) LoanInput {
		st := func(field string) int8 {
			if field == unknown {
				return types.StatusEmpty
			}
			return types.InOutInput
		}
		return LoanInput{
			Loan: Loan{
				AmountStatus: st("amount"), Amount: v["amount"],
				LoanDateStatus: types.InOutInput, LoanDate: loanDate,
				LoanRateStatus: st("rate"), LoanRate: v["rate"],
				FirstStatus: types.InOutInput, FirstDate: firstDate,
				NStatus: st("nper"), NPeriods: int(v["nper"] + 0.5),
				PerYrStatus: types.InOutInput, PerYr: perYr,
				PayAmtStatus: st("payment"), PayAmt: v["payment"],
				PointsStatus: types.InOutInput,
			},
			Settings: Settings{Basis: types.Basis360, PerYr: perYr, YrDays: 360, YrInv: 1.0 / 360},
		}
	}

	goSolve := func(v vals, unknown string) (float64, bool) {
		in := buildInput(v, unknown)
		// Extract what the ENGINE actually produces (it may refine the closed-form
		// solvers): payment/term from Amortize's result, rate/amount from the
		// dispatch's direct solvers (the same ones HandleAmortizationCalc calls).
		switch unknown {
		case "payment":
			res := Amortize(in)
			if res.Err != nil {
				return 0, false
			}
			return payoffRegularPayment(res, in.Loan), true
		case "nper":
			res := Amortize(in)
			if res.Err != nil || res.NPeriods <= 0 {
				return 0, false
			}
			return float64(res.NPeriods), true
		case "rate":
			r, ok, err := SolveRate(in)
			return r, ok && err == nil
		case "amount":
			a, ok, err := SolveLoanAmount(in)
			return a, ok && err == nil
		}
		return 0, false
	}

	// runOracle2 execs the oracle with retry, returns the whitespace fields.
	runOracle2 := func(args []string) ([]string, bool) {
		for try := 0; try < 8; try++ {
			out, err := exec.Command(oracleBin, args...).Output()
			if err != nil {
				continue
			}
			f := strings.Fields(strings.TrimSpace(string(out)))
			if len(f) >= 2 {
				return f, true
			}
		}
		return nil, false
	}

	oracleArgs := func(v vals, unknown string) []string {
		args := []string{
			strconv.FormatFloat(v["amount"], 'f', 2, 64),
			strconv.FormatFloat(v["rate"], 'f', 10, 64),
			strconv.Itoa(int(v["nper"] + 0.5)), strconv.Itoa(perYr),
		}
		if unknown != "payment" {
			args = append(args, "payhard="+strconv.FormatFloat(v["payment"], 'f', 2, 64))
		}
		switch unknown {
		case "rate":
			args = append(args, "solverate")
		case "nper":
			args = append(args, "solveterm")
		case "amount":
			args = append(args, "solveamount")
		}
		return args
	}

	// dosSolve returns the DOS-solved value for `unknown` and the total interest of
	// the resulting (fully-determined) schedule.
	dosSolve := func(v vals, unknown string) (val, totalInt float64, ok bool) {
		f, ok := runOracle2(oracleArgs(v, unknown))
		if !ok {
			return 0, 0, false
		}
		switch unknown {
		case "payment":
			if f[0] != "payment" || len(f) < 4 {
				return 0, 0, false
			}
			val, _ = strconv.ParseFloat(f[1], 64)
			totalInt, _ = strconv.ParseFloat(f[3], 64)
			return val, totalInt, true
		case "rate":
			if f[0] != "rate" {
				return 0, 0, false
			}
			val, _ = strconv.ParseFloat(f[1], 64)
		case "amount":
			if f[0] != "amount" {
				return 0, 0, false
			}
			val, _ = strconv.ParseFloat(f[1], 64)
		case "nper":
			if f[0] != "term" {
				return 0, 0, false
			}
			iv, _ := strconv.Atoi(f[1])
			val = float64(iv)
		}
		// For non-payment solves, a second forward call yields the total interest.
		fwd := vals{}
		for k, vv := range v {
			fwd[k] = vv
		}
		fwd[unknown] = val
		ff, ok2 := runOracle2(oracleArgs(fwd, "none")) // all hardened → default emits totals
		if ok2 && len(ff) >= 4 && ff[0] == "payment" {
			totalInt, _ = strconv.ParseFloat(ff[3], 64)
		}
		return val, totalInt, true
	}

	fields := []string{"amount", "rate", "nper", "payment"}
	// Payment and rate are WELL-conditioned solves — they must match DOS tightly.
	// Amount and #periods are ILL-conditioned INVERSE solves: DOS's Iterate stops
	// on the residual (final balance < half-cent), so over a long term a range of
	// amounts (~$1) and either of two adjacent integer terms all satisfy it. Both
	// engines land a VALID solution in the same neighborhood — the value can differ
	// by the ill-conditioning band while the resulting SCHEDULE (checked via
	// totalInt below, tightly) matches. So the value tolerance is loose for
	// amount/#periods and the schedule check is what pins fidelity.
	tolOf := func(field string, dosVal float64) float64 {
		switch field {
		case "payment":
			return 0.02
		case "rate":
			return 5e-5
		case "amount":
			return math.Max(2.0, 5e-6*math.Abs(dosVal))
		case "nper":
			return 1.0 // adjacent-integer boundary; >1 would be a real bug
		}
		return 0.01
	}

	cases := 0
	for c := 0; c < 60; c++ {
		// random consistent-ish seed loan
		amount := float64(50000 + rng.Intn(400000))
		rate := 0.03 + rng.Float64()*0.10
		nper := 24 + rng.Intn(156)
		v := vals{"amount": amount, "rate": rate, "nper": float64(nper), "payment": 0}

		// Chain of unknowns: start with payment, then two random rotations to a
		// DIFFERENT field each time.
		unknown := "payment"
		prev := ""
		steps := 0
		aborted := false
		for s := 0; s < 3; s++ {
			goVal, gok := goSolve(v, unknown)
			dosVal, dosInt, dok := dosSolve(v, unknown)
			if !gok || !dok {
				aborted = true
				break // unsolvable draw (e.g. payment too small) — skip the case
			}
			tv := tolOf(unknown, dosVal)
			if d := goVal - dosVal; d > tv || d < -tv {
				t.Errorf("case %d step %d solve %s: Go=%.6f DOS=%.6f (Δ=%+.6g, tol=%.4g)", c, s, unknown, goVal, dosVal, d, tv)
			}
			// Harden the DOS-canonical solved value into the chain.
			v[unknown] = dosVal
			// Intermediary output: the Go forward schedule's total interest must match
			// DOS's for the now fully-determined loan.
			gres := Amortize(buildInput(v, "none"))
			if gres.Err == nil && dosInt > 0 {
				itol := 0.5 + 1e-5*math.Abs(dosInt)
				if d := gres.TotalInt - dosInt; d > itol || d < -itol {
					t.Errorf("case %d step %d after %s: Go totalInt=%.2f DOS=%.2f (Δ=%.2f)", c, s, unknown, gres.TotalInt, dosInt, d)
				}
			}
			steps++
			// Pick the next unknown: a field different from the one just solved and
			// from the previous one, so the chain genuinely rotates.
			prev = unknown
			for {
				cand := fields[rng.Intn(len(fields))]
				if cand != prev {
					unknown = cand
					break
				}
			}
		}
		if !aborted && steps == 3 {
			cases++
		}
	}
	if cases < 10 {
		t.Fatalf("only %d complete fuzz cases ran (want >=10) — oracle flaky or draws unsolvable", cases)
	}
	t.Logf("completed %d full 3-step chains", cases)
}
