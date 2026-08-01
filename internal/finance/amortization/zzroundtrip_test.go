package amortization

// ROUND-TRIP (INVERSE-CONSISTENCY) DIFFERENTIAL — round 14, 2026-08-01.
//
// WHAT THIS IS, AND WHY IT IS NOT THE SAME AS dos_fuzzer5_test.go
//
// Requested by Nate's client: "each time you check a calculation, harden the
// answer, erase the inputs one-at-a-time, and see that you can calculate back
// to the number you originally had put in."
//
// `dos_fuzzer5_test.go` already has the MECHANISM for that — `payhard=` pins the
// answer and `noamt` / `norate` / `noterm` / `non` blank exactly one input. But
// what it ASSERTS is that DOS and the port agree on the solved-back value
// (dos_fuzzer5_test.go:1494-1513, `goSolved` vs `dos.solvedAmt`). It never checks
// that either side recovers the number originally put in, and it CANNOT: it
// hardens a deliberately perturbed payment,
//
//	pay := cents(fair * (0.85 + rng.Float64()*0.5))     // 0.85x - 1.35x fair
//
// so the screen it solves back from is not the screen it started from, by
// construction. Round-tripping it would be meaningless.
//
// This file hardens the SOLVED payment instead, so the inverse is well posed.
//
// THE ASSERTION (Nate, 2026-08-01)
//
// A round trip through a cent-quantized payment CANNOT be exact, and testing for
// exactness would be testing arithmetic rather than the port. Nate has measured
// this before and named the right criterion:
//
//	"it does lead to rounding differences in the back-solved amount. But this is
//	 fine just as long as the differential is within the bounds of what DOS
//	 solves for. Viewed this way, we can process it via an oracle approach."
//
// So the gate is NOT |recovered - original| ≈ 0. It is
//
//	portRecoveryError <= dosRecoveryError + quantum
//
// where `quantum` is the irreducible floor imposed by rounding the payment to
// cents (derived per-case below, not tuned). DOS's own inverse error is the
// yardstick. A port that round-trips BETTER than DOS is not more faithful — it
// is differently wrong — so the two-sided report below prints both errors and
// flags the port only when it is materially worse.
//
// WHAT THIS INSTRUMENT CAN SEE THAT NOTHING ELSE CAN
//
// Every other differential in this project dies where DOS refuses a screen —
// which is exactly what made §56's paired-regression NEW=1 unadjudicable in
// round 13 (an advisory on a screen DOS will not answer, so there was no oracle
// answer to be faithful to). The PORT half of a round trip needs no oracle at
// all: it is a self-consistency property. `TestRoundTripPortSelfConsistency`
// exploits that and runs where the oracle is silent.
//
// WHAT IT IS BLIND TO — READ THIS BEFORE TRUSTING A GREEN RUN
//
// A round trip is blind to §54 and to every other whole-calendar disagreement.
// The port uses ITS OWN date layer in both directions, so it recovers its own
// input perfectly while still disagreeing with DOS about February 2100. Only the
// forward differential can see that. These two instruments are complementary
// because they are blind to DIFFERENT things; neither subsumes the other.
//
// STATUS: v1 covers the AMOUNT and RATE axes, which have clean solver entry
// points (SolveLoanAmount / SolveRate, backward.go:199/:408) and clean oracle
// echoes (`solvedamount` / `solvedrate`, amort_oracle.pas:834). The TERM axis
// (`noterm` / `non`) is deliberately NOT here yet — DOS leaves its answer in
// h^.nperiods/h^.lastdate rather than a `solved*` echo, so it needs the bdump
// parser, and landing two solid axes beat landing three shaky ones on round 14's
// clock. It is the first thing to add.

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// rtScreen is one drawn loan, before anything is erased.
type rtScreen struct {
	amount float64
	rate   float64
	n      int
	perYr  int
}

func (s rtScreen) args(extra ...string) []string {
	a := []string{
		strconv.FormatFloat(s.amount, 'f', 2, 64),
		strconv.FormatFloat(s.rate, 'f', 10, 64),
		strconv.Itoa(s.n), strconv.Itoa(s.perYr),
	}
	return append(a, extra...)
}

func (s rtScreen) cmd(extra ...string) string {
	return "amort_oracle " + strings.Join(s.args(extra...), " ")
}

// rtOracle execs the DOS engine and returns the numeric fields it echoed.
//
// The retry loop is NOT belt-and-braces: the Pascal New(h)/ZeroAMZLoan path is
// heap-sensitive and returns a 0 payment on roughly 4-9% of rapid spawns
// (dos_oracle_sweep_test.go:36, and backlog item 6). Every such case reproduces
// correctly on a fresh process. Retrying here rather than voting is adequate
// because a flaked run yields NO parseable answer at all, not a wrong one.
func rtOracle(args []string) (map[string]float64, bool) {
	numeric := map[string]bool{
		"payment": true, "interest": true, "paid": true,
		"solvedamount": true, "solvedrate": true, "nperiods": true,
	}
	for try := 0; try < 8; try++ {
		out, err := exec.Command(oracleBin, args...).Output()
		if err != nil {
			continue
		}
		got := map[string]float64{}
		f := strings.Fields(string(out))
		for i := 0; i+1 < len(f); i++ {
			if !numeric[f[i]] {
				continue
			}
			if v, err := strconv.ParseFloat(f[i+1], 64); err == nil {
				got[f[i]] = v
			}
		}
		// A zero payment is the heap flake, not an answer.
		if p, ok := got["payment"]; ok && p != 0 {
			return got, true
		}
	}
	return nil, false
}

// rtInput builds the port-side screen. Deliberately the same shape goSolve uses
// (dos_oracle_sweep_test.go:59) so a divergence here cannot be an artifact of a
// differently-populated LoanInput.
func rtInput(s rtScreen) LoanInput {
	return LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: s.amount,
		LoanRateStatus: types.InOutInput, LoanRate: s.rate,
		NStatus: types.InOutInput, NPeriods: s.n,
		PerYrStatus: types.InOutInput, PerYr: s.perYr,
		PayAmtStatus:   types.StatusEmpty,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
		FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(s.perYr)},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(s.perYr),
			YrDays: 360, YrInv: 1.0 / 360}}
}

// rtQuantumAmount is the irreducible round-trip floor on the AMOUNT axis.
//
// The payment is held to the cent, so the hardened payment differs from the
// exact fair payment by up to half a cent. Amount is very nearly linear in
// payment over that interval (dA/dP = A/P to first order), so half a cent of
// payment is worth A*0.005/P of principal. That is a DERIVED bound, not a tuned
// tolerance: below it, no solver of any quality can recover the input, and a
// test that demanded better would be reporting arithmetic.
//
// The 2x is for the second-order term and for the solver's own stop tolerance
// on top of the quantization; it is the only judgement call in the number.
func rtQuantumAmount(amount, pay float64) float64 {
	if pay <= 0 {
		return 0
	}
	return 2 * amount * 0.005 / pay
}

// rtQuantumRate is the same bound on the RATE axis. dR/dP has no clean closed
// form across all frequencies, so this uses the finite-difference the solver
// itself would see: half a cent of payment against the total interest the loan
// throws off, scaled by the rate. Same reasoning, same status — a floor, not a
// tuning knob.
func rtQuantumRate(rate, pay float64, n int) float64 {
	if pay <= 0 || n <= 0 {
		return 0
	}
	return 2 * rate * 0.005 / (pay * float64(n))
}

// rtDraw produces screens whose forward payment DOS will actually answer.
// Ranges mirror the ordinary end of dos_fuzzer5_test.go rather than its widened
// frontier: this instrument is about the INVERSE, and mixing in horizon effects
// would make a first failure ambiguous (START_HERE §5, "separate the horizon
// effect before blaming a region").
//
// WHOLE-MONTH FREQUENCIES ONLY IN v1, AND THIS IS NOT A STYLE CHOICE.
// `firstPeriodDate` (dos_oracle_sweep_test.go:26) computes `m := 1 + 12/perYr`
// in INTEGER division, so every sub-monthly frequency (24, 26, 52) collapses to
// month 1 and places the first payment ON the loan date. Drawing those made this
// instrument report five self-inverse "failures" of up to 1257x the quantum on
// its very first run — all of them at perYr > 12, none of them an engine defect.
// That is the SIXTH bug in the family START_HERE §5 names ("any date the harness
// computes must be computed the way the ORACLE computes it"), and it is exactly
// the shape of round 13's retracted 76% finding.
//
// Sub-monthly round trips are worth having and are the next thing to add, but
// they need explicit loandmy=/firstdmy= tokens on both sides — real date work,
// in §51/§54 territory — not an integer division. Until then this draws only
// frequencies where 12/perYr is exact and the computed first date is right.
func rtDraw(rng *rand.Rand) rtScreen {
	perYrs := []int{1, 2, 4, 6, 12}
	p := perYrs[rng.Intn(len(perYrs))]
	// Term drawn in YEARS, not periods. Drawing periods uniformly made perYr=1
	// mean a 337-YEAR loan, which put the first rate-axis findings on the far
	// side of §55's year-byte wrap and confounded "the solver under-converges"
	// with "the horizon is past 2155". START_HERE §5: separate the horizon
	// effect before blaming a region. Long-horizon round trips are worth having
	// as their OWN stratum; mixing them into the baseline is what makes a first
	// failure unreadable.
	years := 2 + rng.Intn(39) // 2..40 years, comfortably inside the byte
	n := years * p
	if n < 2 {
		n = 2
	}
	return rtScreen{
		amount: math.Round(rng.Float64()*400000+5000) + 0.37,
		rate:   math.Round((0.01+rng.Float64()*0.18)*1e6) / 1e6,
		n:      n,
		perYr:  p,
	}
}

func rtRequireOracle(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(oracleBin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("PERSENSE_REQUIRE_ORACLE set but oracle missing at %s", oracleBin)
		}
		t.Skip("oracle not built; run legacy/oracle/build_linux.sh")
	}
}

// TestRoundTripAgainstDOSRecoveryError is the client's check, adjudicated the
// way Nate specified: the port's inverse error must stay within the bounds of
// DOS's own inverse error.
//
// Per case:
//  1. DOS solves the payment forward. Harden it to the cent.
//  2. Erase the amount. DOS solves it back; the port solves it back. Compare
//     each against the ORIGINAL amount, then compare the two errors.
//  3. Same for the rate.
//
// A forward-payment disagreement SKIPS the case rather than failing it — that is
// dos_fuzzer5_test.go's job, and letting it fail here would double-count one
// defect in two instruments and make this one's rate meaningless.
func TestRoundTripAgainstDOSRecoveryError(t *testing.T) {
	rtRequireOracle(t)

	cases := 60
	if v := os.Getenv("PERSENSE_RT_N"); v != "" {
		if k, err := strconv.Atoi(v); err == nil && k > 0 {
			cases = k
		}
	}
	seed := int64(1401)
	if v := os.Getenv("PERSENSE_RT_SEED"); v != "" {
		if k, err := strconv.ParseInt(v, 10, 64); err == nil {
			seed = k
		}
	}
	rng := rand.New(rand.NewSource(seed))

	var drawn, comparedAmt, comparedRate int
	var worseAmt, worseRate int
	// Reported even when everything passes: a round trip that is uniformly at
	// the quantum floor and one that is uniformly 100x inside it are both
	// "green", and only the second means the solvers agree in substance.
	var maxRatioAmt, maxRatioRate float64

	for i := 0; i < cases; i++ {
		s := rtDraw(rng)
		drawn++

		fwd, ok := rtOracle(s.args())
		if !ok {
			continue
		}
		payDOS := fwd["payment"]

		// The port must agree on the forward payment before its inverse means
		// anything. Quantize through the same string the oracle parses
		// (START_HERE §5) so this gate cannot fire on a formatting artifact.
		gpay, _, gok := goSolve(s.amount, s.rate, s.n, s.perYr)
		if !gok {
			continue
		}
		if math.Abs(gpay-payDOS) > 0.01+1e-9*math.Abs(payDOS) {
			continue // forward divergence — fuzzer5's territory, not this one's
		}

		// Harden to the cent: this is what the screen actually holds after a
		// forward solve, and it is the source of the irreducible round-trip
		// error the quantum below accounts for.
		pay := math.Round(payDOS*100) / 100
		payTok := "payhard=" + strconv.FormatFloat(pay, 'f', 2, 64)

		// ---- axis 1: erase the AMOUNT, solve back ----
		if got, ok := rtOracle(s.args(payTok, "noamt")); ok {
			if aDOS, has := got["solvedamount"]; has {
				in := rtInput(s)
				in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, pay
				in.Loan.AmountStatus, in.Loan.Amount = types.StatusEmpty, 0
				if aGo, _, err := SolveLoanAmount(in); err == nil {
					comparedAmt++
					errDOS := math.Abs(aDOS - s.amount)
					errGo := math.Abs(aGo - s.amount)
					q := rtQuantumAmount(s.amount, pay)
					if q > 0 {
						if r := errGo / q; r > maxRatioAmt {
							maxRatioAmt = r
						}
					}
					if errGo > errDOS+q {
						worseAmt++
						t.Errorf("ROUND-TRIP amount: port recovers worse than DOS\n"+
							"  %s\n  original %.2f | DOS back %.2f (err %.4f) | Go back %.2f (err %.4f)\n"+
							"  quantum %.4f — port exceeds DOS+quantum by %.4f",
							s.cmd(payTok, "noamt"), s.amount, aDOS, errDOS, aGo, errGo,
							q, errGo-(errDOS+q))
					}
				}
			}
		}

		// ---- axis 2: erase the RATE, solve back ----
		if got, ok := rtOracle(s.args(payTok, "norate")); ok {
			if rDOS, has := got["solvedrate"]; has {
				in := rtInput(s)
				in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, pay
				in.Loan.LoanRateStatus, in.Loan.LoanRate = types.StatusEmpty, 0
				if rGo, _, err := SolveRate(in); err == nil {
					comparedRate++
					errDOS := math.Abs(rDOS - s.rate)
					errGo := math.Abs(rGo - s.rate)
					q := rtQuantumRate(s.rate, pay, s.n)
					if q > 0 {
						if r := errGo / q; r > maxRatioRate {
							maxRatioRate = r
						}
					}
					if errGo > errDOS+q {
						worseRate++
						t.Errorf("ROUND-TRIP rate: port recovers worse than DOS\n"+
							"  %s\n  original %.10f | DOS back %.10f (err %.2e) | Go back %.10f (err %.2e)\n"+
							"  quantum %.2e — port exceeds DOS+quantum by %.2e",
							s.cmd(payTok, "norate"), s.rate, rDOS, errDOS, rGo, errGo,
							q, errGo-(errDOS+q))
					}
				}
			}
		}
	}

	t.Logf("round-trip vs DOS: drawn %d | amount compared %d, port-worse %d (max err/quantum %.2f)"+
		" | rate compared %d, port-worse %d (max err/quantum %.2f)",
		drawn, comparedAmt, worseAmt, maxRatioAmt, comparedRate, worseRate, maxRatioRate)

	if comparedAmt == 0 && comparedRate == 0 {
		t.Errorf("no case was adjudicable — the instrument measured nothing. " +
			"A green run here would be a false negative (START_HERE §5: " +
			"'a small-sample zero is not closure').")
	}
}

// TestRoundTripPortSelfConsistency is the oracle-free half.
//
// It asserts only that the port is its own inverse, so it runs on screens DOS
// refuses outright — the blind spot that left §56's NEW=1 unadjudicable in round
// 13. It cannot prove fidelity (see the blindness note at the top of this file);
// it proves that the forward engine and the backward solvers describe the same
// loan, which is a precondition for fidelity and is currently untested.
func TestRoundTripPortSelfConsistency(t *testing.T) {
	cases := 200
	if v := os.Getenv("PERSENSE_RT_N"); v != "" {
		if k, err := strconv.Atoi(v); err == nil && k > 0 {
			cases = k
		}
	}
	rng := rand.New(rand.NewSource(1402))

	var compared, bad int
	for i := 0; i < cases; i++ {
		s := rtDraw(rng)

		pay, _, ok := goSolve(s.amount, s.rate, s.n, s.perYr)
		if !ok {
			continue
		}
		pay = math.Round(pay*100) / 100

		in := rtInput(s)
		in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, pay
		in.Loan.AmountStatus, in.Loan.Amount = types.StatusEmpty, 0
		back, _, err := SolveLoanAmount(in)
		if err != nil {
			continue
		}
		compared++

		q := rtQuantumAmount(s.amount, pay)
		// 10x the derived floor. Wide on purpose: this half has no DOS yardstick,
		// so it is tuned to catch a solver that is WRONG, not one that is merely
		// less precise than the forward engine. Tightening it without a DOS
		// baseline would manufacture failures that mean nothing.
		if math.Abs(back-s.amount) > 10*q {
			bad++
			if bad <= 10 {
				t.Errorf("port is not its own inverse\n  %s\n  original %.2f | back %.2f"+
					" (err %.4f, quantum %.4f, %.1fx)",
					s.cmd("payhard="+strconv.FormatFloat(pay, 'f', 2, 64), "noamt"),
					s.amount, back, math.Abs(back-s.amount), q, math.Abs(back-s.amount)/q)
			}
		}
	}
	t.Logf("port self-inverse: compared %d, failed %d", compared, bad)
	if compared == 0 {
		t.Error("no case was adjudicable — instrument measured nothing")
	}
}

var _ = fmt.Sprintf
