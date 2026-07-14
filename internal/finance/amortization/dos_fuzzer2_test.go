package amortization

import (
	"math"
	"math/rand"
	"os"
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

	// runOracle2 execs the oracle with retry. The bool is "ran": true when the
	// oracle produced a usable line (a solution token OR an "ERR <msg>" refusal),
	// false only when the binary could not be executed after all retries (flake).
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
			// The oracle has no "solveamount" verb; amount is solved with
			// `noamt` (blank the amount so MakeTable solves it), which emits the
			// normal totals line PLUS `solvedamount X`. amort_oracle.pas:609-616,914.
			args = append(args, "noamt")
		}
		return args
	}

	// idxOf returns the index of tok in f, or -1. Used because `noamt` output is
	// multi-line (totals line + `solvedamount X`) and runOracle2 flattens it.
	idxOf := func(f []string, tok string) int {
		for i, x := range f {
			if x == tok {
				return i
			}
		}
		return -1
	}

	// DOS outcome tri-state. The oracle exits 0 in every case and writes its
	// result to stdout: a solution token line ("payment/rate/amount/term …")
	// when DOS solves, or "ERR <message>" when the DOS engine genuinely refuses
	// (e.g. "Payment amount is too small to compute number of periods"). A true
	// exec failure (binary can't run) yields no output after runOracle2's retries.
	// Distinguishing dosRefused from dosFlake is what lets us cross-check
	// unsolvable draws against DOS instead of silently skipping them.
	const (
		dosSolved  = iota // DOS returned a solution token → value parsed
		dosRefused        // DOS ran and emitted ERR / a non-solution line
		dosFlake          // oracle could not be executed — indeterminate
	)

	// dosSolve returns the DOS-solved value, the total interest of the resulting
	// (fully-determined) schedule, and which of the three outcomes occurred.
	//
	// The solution token differs by query and is not always first:
	//   payment  → "payment P interest I paid T"          (token "payment", pos 0)
	//   rate     → "rate R status S"                      (token "rate",    pos 0)
	//   nper     → "term N last .. status S"              (token "term",    pos 0)
	//   amount   → "payment .. interest I .. \n solvedamount A"  (token "solvedamount")
	// A genuine DOS refusal is an "ERR <msg>" line, in which the query's token is
	// absent — that is how dosRefused is detected (vs dosFlake, no output at all).
	dosSolve := func(v vals, unknown string) (val, totalInt float64, outcome int) {
		f, ran := runOracle2(oracleArgs(v, unknown))
		if !ran {
			return 0, 0, dosFlake
		}
		tok := map[string]string{
			"payment": "payment", "rate": "rate", "amount": "solvedamount", "nper": "term",
		}[unknown]
		at := idxOf(f, tok)
		if at < 0 || at+1 >= len(f) {
			// Ran but the solution token is absent (an "ERR <msg>" line) → DOS
			// genuinely refuses this draw.
			return 0, 0, dosRefused
		}
		switch unknown {
		case "payment", "amount":
			// Both carry a totals line, so total interest comes from the SAME
			// output — no second call needed.
			val, _ = strconv.ParseFloat(f[at+1], 64)
			if j := idxOf(f, "interest"); j >= 0 && j+1 < len(f) {
				totalInt, _ = strconv.ParseFloat(f[j+1], 64)
			}
			// Oracle numerical-breakdown sentinel: the DOS engine occasionally
			// emits "payment 0.0000 interest -1.00 paid -1.00" instead of an ERR
			// for a pathological input. A valid schedule never has negative total
			// paid — treat it as indeterminate (flake), not a real "DOS solved 0".
			if p := idxOf(f, "paid"); p >= 0 && p+1 < len(f) {
				if paid, _ := strconv.ParseFloat(f[p+1], 64); paid < 0 {
					return 0, 0, dosFlake
				}
			}
			return val, totalInt, dosSolved
		case "rate":
			val, _ = strconv.ParseFloat(f[at+1], 64)
		case "nper":
			iv, _ := strconv.Atoi(f[at+1])
			val = float64(iv)
		}
		// rate/term queries Halt before emitting totals, so a second forward call
		// (all fields hardened) yields the schedule's total interest.
		fwd := vals{}
		for k, vv := range v {
			fwd[k] = vv
		}
		fwd[unknown] = val
		ff, ok2 := runOracle2(oracleArgs(fwd, "none")) // all hardened → default emits totals
		if ok2 {
			if j := idxOf(ff, "interest"); j >= 0 && j+1 < len(ff) {
				totalInt, _ = strconv.ParseFloat(ff[j+1], 64)
			}
		}
		return val, totalInt, dosSolved
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

	// Seed-loan iteration count: default 60, overridable via FUZZER2_CASES so a
	// larger differential run (e.g. 100) can be requested without editing the test.
	nSeeds := 60
	if v := os.Getenv("FUZZER2_CASES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			nSeeds = n
		}
	}
	cases := 0
	unsolvableAgreed := 0 // draws both engines agree are unsolvable (verified consistent)
	flaked := 0           // draws skipped because the oracle could not be executed
	for c := 0; c < nSeeds; c++ {
		// random consistent-ish seed loan
		amount := float64(50000 + rng.Intn(400000))
		// Quantize the rate to a realistic 6-decimal grid so BOTH engines receive
		// an IDENTICAL value. oracleArgs formats the rate to fixed precision; an
		// un-quantized full-float rate rounds differently for the oracle than the
		// value Go computes with, an unfair differential — and can land on a point
		// that trips an oracle numerical edge (the payment 0 / interest -1 / paid -1
		// sentinel, guarded in dosSolve). Solved rates the chain hardens are already
		// 6dp (the oracle emits h^.loanrate:0:6), so this keeps every rate ≤6dp.
		rate := math.Round((0.03+rng.Float64()*0.10)*1e6) / 1e6
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
			dosVal, dosInt, dOutcome := dosSolve(v, unknown)

			if os.Getenv("DEBUG_FUZZ") != "" {
				t.Logf("DBG case %d step %d unknown=%s v=%v -> goVal=%.4f gok=%v dosVal=%.4f dOutcome=%d",
					c, s, unknown, v, goVal, gok, dosVal, dOutcome)
			}

			if dOutcome == dosFlake {
				// Oracle could not be executed for this draw — we cannot
				// adjudicate Go against DOS, so skip WITHOUT a verdict.
				flaked++
				aborted = true
				break
			}
			dosOK := dOutcome == dosSolved

			// SOLVABILITY AGREEMENT: an "unsolvable draw" is only a legitimate
			// skip if DOS agrees it is unsolvable. A one-sided solve — Go solves
			// where DOS errors, or DOS solves where Go gives up — is a real
			// divergence, not something to skip silently.
			if gok != dosOK {
				t.Errorf("case %d step %d solve %s: SOLVABILITY MISMATCH — Go solved=%v, DOS solved=%v "+
					"(inputs amount=%.2f rate=%.8f nper=%d payhard=%.2f; Go=%.6f DOS=%.6f)",
					c, s, unknown, gok, dosOK,
					v["amount"], v["rate"], int(v["nper"]+0.5), v["payment"], goVal, dosVal)
				aborted = true
				break
			}

			if !gok {
				// Both engines agree the draw is unsolvable — verified consistent.
				unsolvableAgreed++
				aborted = true
				break
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
	t.Logf("completed %d full 3-step chains; %d draws mutually-unsolvable (Go and DOS agreed); %d skipped on oracle flake",
		cases, unsolvableAgreed, flaked)
}
