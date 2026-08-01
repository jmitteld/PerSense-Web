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
// STATUS: v2 (round 15, 2026-08-01) covers ALL THREE cells of the client's
// "erase the inputs one at a time" — AMOUNT and RATE through their solver entry
// points (SolveLoanAmount / SolveRate, backward.go:208/:425) and the TERM
// through Amortize with both n and the last date blanked. Round 14 recorded that
// the term axis needed the bdump parser; it does not — amort_oracle.pas:1216
// already echoes `solvedterm <n> last <y>-<m>-<d>`, which carries both cells.
//
// Still open, in order: `non` (blank n only, with a typed last date, which needs
// explicit lastdmy=/loandmy= on both sides), and sub-monthly frequencies (§4a of
// the round 14 write-up — `firstPeriodDate`'s integer division).
//
// TestRoundTripRateHorizonStrata (below) is the round-15 adjudication of round
// 14's open long-horizon finding, and it is a different KIND of test: it aims
// the same instrument at named date-layer boundaries to separate "the solver
// under-converges over a long span" from "the port's calendar is not DOS's out
// there". Read its header before quoting any number out of it.

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
	got, _, ok := rtOracleFields(args)
	return got, ok
}

// rtOracleFields is rtOracle plus the raw whitespace-split stdout, for the TERM
// axis: DOS echoes the recovered term as
//
//	solvedterm 43 last 2034-1-2                       (amort_oracle.pas:1216)
//
// i.e. an INTEGER count and a DATE, neither of which fits the float map. Round
// 14 recorded that this axis "needs the bdump parser"; it does not — the
// `solvedterm` echo already carries both cells, and `bdump` is only needed when
// the balloon grid itself is under test.
func rtOracleFields(args []string) (map[string]float64, []string, bool) {
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
			return got, f, true
		}
	}
	return nil, nil, false
}

// rtSolvedTerm pulls `solvedterm <n> last <y>-<m>-<d>` out of a field slice.
// Absent when DOS refused the term solve, which is a SKIP and not a failure —
// the oracle leaving no answer is exactly the case the self-consistency half of
// this file exists to cover.
func rtSolvedTerm(f []string) (int, string, bool) {
	for i := 0; i+3 < len(f); i++ {
		if f[i] != "solvedterm" || f[i+2] != "last" {
			continue
		}
		n, err := strconv.Atoi(f[i+1])
		if err != nil || n <= 0 {
			return 0, "", false
		}
		return n, f[i+3], true
	}
	return 0, "", false
}

// rtGoLastDate renders the port's last payment date the way amort_oracle.pas:1216
// renders DOS's, so the two are string-comparable. DOS prints y+1900 with no
// zero padding on any field.
func rtGoLastDate(d types.DateRec) string {
	return fmt.Sprintf("%d-%d-%d", d.Time.Year(), int(d.Time.Month()), d.Time.Day())
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
	// Term drawn in YEARS, not periods. Drawing periods uniformly made perYr=1
	// mean a 337-YEAR loan, which put the first rate-axis findings on the far
	// side of §55's year-byte wrap and confounded "the solver under-converges"
	// with "the horizon is past 2155". START_HERE §5: separate the horizon
	// effect before blaming a region. Long-horizon round trips are worth having
	// as their OWN stratum (rtDrawYears, and TestRoundTripRateHorizonStrata
	// below); mixing them into the baseline is what makes a first failure
	// unreadable.
	return rtDrawYears(rng, 2, 40) // comfortably inside the byte, and pre-2100
}

// rtDrawYears is rtDraw with an explicit calendar span, so a stratum can be
// aimed at a named date-layer boundary. The loan date is 2024-01-01 (rtInput),
// so `years` maps directly onto the final year: 76 reaches 2100 (§54's century
// leap rule), 131 reaches 2155 (§55's year byte).
func rtDrawYears(rng *rand.Rand, minYears, maxYears int) rtScreen {
	perYrs := []int{1, 2, 4, 6, 12}
	p := perYrs[rng.Intn(len(perYrs))]
	span := maxYears - minYears + 1
	if span < 1 {
		span = 1
	}
	years := minYears + rng.Intn(span)
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

// rtEndYear is the calendar year the schedule runs to, given rtInput's fixed
// 2024-01-01 loan date. Used to bucket a case by which date-layer boundary it
// crosses, without needing either engine to tell us.
func (s rtScreen) endYear() int {
	if s.perYr <= 0 {
		return 2024
	}
	return 2024 + s.n/s.perYr
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

	var drawn, comparedAmt, comparedRate, comparedTerm int
	var worseAmt, worseRate, worseTerm, worseTermDate, bothDriftTerm, maxDriftTerm int
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

		// ---- axis 3: erase the TERM, solve back ----
		//
		// This is the third and last of the client's cells, and it is the only
		// one with TWO answers: DOS leaves the recovered term in h^.nperiods AND
		// h^.lastdate. Comparing the count alone is insufficient — a grid that
		// is off by a period can still land on the right COUNT with the wrong
		// final date (the same reasoning dos_fuzzer5_test.go:1518 gives).
		//
		// `noterm` blanks BOTH cells, which is the honest "erase this input":
		// `non` blanks only n and leaves a typed last date in force, so it hands
		// the solver most of the answer back. `non` needs an explicit lastdmy=
		// on both sides and is deliberately left for v3.
		//
		// There is NO quantum here. The term is an integer, so the recovery
		// error is an integer count of periods, and the DOS-relative criterion
		// carries the whole burden: rounding the payment to the cent can
		// legitimately push the loan one period past its original term, but it
		// pushes DOS exactly as far as it pushes the port.
		if got, f, ok := rtOracleFields(s.args(payTok, "noterm")); ok && got != nil {
			if nDOS, lastDOS, has := rtSolvedTerm(f); has {
				in := rtInput(s)
				in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, pay
				in.Loan.NStatus, in.Loan.NPeriods = types.StatusEmpty, 0
				// LastOK left false, exactly as the oracle leaves it
				// (amort_oracle.pas:843-845); forcing it would hide a
				// divergence in the derivation instead of exposing one.
				in.Loan.LastStatus, in.Loan.LastOK = types.StatusEmpty, false
				gr := Amortize(in)
				if gr.Err == nil && gr.NPeriods > 0 {
					comparedTerm++
					errDOS := nDOS - s.n
					errGo := gr.NPeriods - s.n
					if errDOS < 0 {
						errDOS = -errDOS
					}
					if errGo < 0 {
						errGo = -errGo
					}
					if errGo > maxDriftTerm {
						maxDriftTerm = errGo
					}
					switch {
					case errGo > errDOS:
						worseTerm++
						t.Errorf("ROUND-TRIP term: port recovers worse than DOS\n"+
							"  %s\n  original n %d | DOS back %d (off by %d) | Go back %d (off by %d)",
							s.cmd(payTok, "noterm"), s.n, nDOS, errDOS, gr.NPeriods, errGo)
					case nDOS == gr.NPeriods && lastDOS != rtGoLastDate(gr.LastDate):
						// Same count, different final date. Oracle-adjudicated,
						// so this is a real divergence and not an internal
						// consistency complaint — but at the baseline draw
						// (≤40 years, ending ≤2064) it cannot be §54, so do not
						// reach for the calendar to explain it.
						worseTermDate++
						t.Errorf("ROUND-TRIP term: counts agree, LAST DATE differs\n"+
							"  %s\n  n %d | DOS last %s | Go last %s",
							s.cmd(payTok, "noterm"), nDOS, lastDOS, rtGoLastDate(gr.LastDate))
					case errGo != 0 && errGo == errDOS:
						// Both sides missed the original by the same amount.
						// That is the payment quantum doing its job, not a
						// defect — logged so a run of them cannot hide inside a
						// green count.
						bothDriftTerm++
					}
				}
			}
		}
	}

	t.Logf("round-trip vs DOS: drawn %d | amount compared %d, port-worse %d (max err/quantum %.2f)"+
		" | rate compared %d, port-worse %d (max err/quantum %.2f)"+
		" | term compared %d, port-worse %d, date-differs %d, both-drifted %d (max port drift %d periods)",
		drawn, comparedAmt, worseAmt, maxRatioAmt, comparedRate, worseRate, maxRatioRate,
		comparedTerm, worseTerm, worseTermDate, bothDriftTerm, maxDriftTerm)

	if comparedAmt == 0 && comparedRate == 0 && comparedTerm == 0 {
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

// ---------------------------------------------------------------------------
// ADJUDICATION OF ROUND 14'S LONG-HORIZON RATE FINDING
// ---------------------------------------------------------------------------
//
// Round 14 drew the term in PERIODS, so perYr=1 meant terms out to 337 years,
// and the rate axis reported 3 of 49 cases where the port recovered the entered
// rate ~50,000x worse than DOS did (DOS to ~1e-9, port to ~1e-4). Re-drawn at
// 2-40 years the same instrument went 80/80 clean, which established that the
// effect is the HORIZON and not SolveRate in general — but no further.
//
// It was explicitly NOT adjudicated, because two explanations were still
// entangled:
//
//	(a) the port's rate solver genuinely under-converges over a long span, or
//	(b) the port's DATES are not DOS's dates out there, so the two engines are
//	    solving different loans and the "recovery error" is a calendar
//	    disagreement wearing a solver's clothes.
//
// (b) has two named, separate boundaries, and THEY ARE NOT THE SAME YEAR:
//
//	§54  the century leap rule. DOS tests `y mod 4` and so believes 2100 IS a
//	     leap year; the port's Gregorian layer knows it is not. First bites at
//	     the Feb 2100 crossing — 76 years past rtInput's 2024-01-01 loan date.
//	§55  the year BYTE. DOS's daterec stores y as a Pascal byte, so every year
//	     assignment truncates mod 256 and the calendar wraps past 2155 — 131
//	     years out.
//
// Round 14's own three findings straddle both, which is why they could not be
// read: 337 periods at perYr=4 ends in 2108 (past §54, inside §55), 130 periods
// at perYr=1 ends in 2154 (past §54, still inside §55), and 316 at perYr=1 ends
// in 2340 (past both). Two of the three are INSIDE the year byte, so §55 alone
// was never a sufficient explanation.
//
// This test separates them by drawing four strata against those boundaries and
// reporting the port-worse rate in each:
//
//	A  2- 40 years   ends <=2064   the baseline region, and the standing gate
//	B  50- 74 years  ends <=2098   long span, but NO §54 crossing   <-- the key
//	C  80-125 years  ends 2104..   crosses §54, inside §55's byte
//	D 140-320 years  ends 2164..   past both
//
// **Stratum B is the whole experiment.** If B is clean and C is not, the
// degradation is the CALENDAR (§54) and this is the first quantified price of
// the deferred date-layer refactor — which is exactly what START_HERE §3 item 3
// asks for. If B degrades too, it is the solver, and backlog item 9 (no bit
// coverage on the backward solvers) becomes the priority instead.
//
// THE MATCHED-n CONTROL, and why the strata alone would not settle it: horizon
// in years and the period COUNT are confounded across strata at a fixed perYr.
// The control block below holds n fixed and varies perYr, so the same solver
// does the same number of iterations over a short and a long calendar span. If
// the failure follows the SPAN and not the count, it is not the solver's
// iteration budget.
//
// This test FAILS on strata A and B — those are in-region and a port-worse case
// there is a defect on the spot. C and D are REPORTED, not asserted: they are
// known-broken calendar territory (§54, §55, both deferred by decision), and
// failing on them would just re-report a decision Nate has already made.
func TestRoundTripRateHorizonStrata(t *testing.T) {
	rtRequireOracle(t)
	if testing.Short() {
		t.Skip("long-horizon stratification: not a short-mode test")
	}

	per := 25
	if v := os.Getenv("PERSENSE_RT_HORIZON_N"); v != "" {
		if k, err := strconv.Atoi(v); err == nil && k > 0 {
			per = k
		}
	}
	seed := int64(1501)
	if v := os.Getenv("PERSENSE_RT_SEED"); v != "" {
		if k, err := strconv.ParseInt(v, 10, 64); err == nil {
			seed = k
		}
	}

	strata := []struct {
		name     string
		lo, hi   int
		asserted bool // false = report only (known calendar territory)
		why      string
	}{
		{"A_2-40yr_to2064", 2, 40, true, "baseline; the standing gate's region"},
		{"B_50-74yr_to2098", 50, 74, true, "long span, NO §54 crossing — the key stratum"},
		{"C_80-125yr_2104+", 80, 125, false, "crosses §54's Feb 2100, inside §55's byte"},
		{"D_140-320yr_2164+", 140, 320, false, "past §54 AND §55"},
	}

	type stat struct {
		compared, worse, refused, fwdSkip int
		maxRatio, sumErrGo, sumErrDOS     float64
	}
	res := map[string]*stat{}

	for _, st := range strata {
		rng := rand.New(rand.NewSource(seed))
		s0 := &stat{}
		res[st.name] = s0
		for i := 0; i < per; i++ {
			s := rtDrawYears(rng, st.lo, st.hi)
			r := rtRateRecovery(s)
			switch r.why {
			case "refused":
				s0.refused++
				continue
			case "forward":
				s0.fwdSkip++
				continue
			}
			if !r.ok {
				continue
			}
			s0.compared++
			s0.sumErrGo += r.errGo
			s0.sumErrDOS += r.errDOS
			if r.errDOS > 0 {
				if ratio := r.errGo / r.errDOS; ratio > s0.maxRatio {
					s0.maxRatio = ratio
				}
			}
			// The gate is the same criterion as the standing test: the port's
			// recovery error must stay within DOS's own. rtRateRecovery has
			// already added the quantum to errDOS, so this comparison is bare.
			if r.errGo > r.errDOS {
				s0.worse++
				if st.asserted {
					t.Errorf("[%s] long-horizon rate round trip: port worse than DOS (ends %d)\n"+
						"  %s\n  original %.10f | DOS back err %.2e | Go back err %.2e (%.0fx)",
						st.name, s.endYear(), r.cmd, s.rate, r.errDOS, r.errGo,
						r.errGo/math.Max(r.errDOS, 1e-300))
				}
			}
		}
	}

	for _, st := range strata {
		s0 := res[st.name]
		mark := "ASSERTED"
		if !st.asserted {
			mark = "reported"
		}
		var meanGo, meanDOS float64
		if s0.compared > 0 {
			meanGo = s0.sumErrGo / float64(s0.compared)
			meanDOS = s0.sumErrDOS / float64(s0.compared)
		}
		t.Logf("stratum %-18s [%s] compared %2d, port-worse %2d | mean err Go %.2e vs DOS %.2e"+
			" | worst Go/DOS %.0fx | DOS refused %d, forward-skip %d  (%s)",
			st.name, mark, s0.compared, s0.worse, meanGo, meanDOS, s0.maxRatio,
			s0.refused, s0.fwdSkip, st.why)
	}

	// ---- matched-n control: same iteration count, different calendar span ----
	//
	// Reported, never asserted. Its job is to make the strata readable, not to
	// gate: if perYr=12 (short span) is clean at the same n where perYr=1 (long
	// span) degrades, the period count is exonerated and the calendar is not.
	t.Logf("--- matched-n control: identical period counts, different spans ---")
	for _, n := range []int{60, 100, 150, 250} {
		for _, p := range []int{12, 1} {
			rng := rand.New(rand.NewSource(seed + int64(n)))
			var compared, worse int
			var sumGo, sumDOS float64
			for i := 0; i < per; i++ {
				s := rtDrawYears(rng, 2, 40) // amount/rate draw only
				s.n, s.perYr = n, p
				r := rtRateRecovery(s)
				if !r.ok {
					continue
				}
				compared++
				sumGo += r.errGo
				sumDOS += r.errDOS
				if r.errGo > r.errDOS {
					worse++
				}
			}
			span := "short"
			if p == 1 {
				span = "LONG "
			}
			var mGo, mDOS float64
			if compared > 0 {
				mGo, mDOS = sumGo/float64(compared), sumDOS/float64(compared)
			}
			t.Logf("  n=%3d perYr=%2d (%s, ends %d): compared %2d, port-worse %2d | mean err Go %.2e vs DOS %.2e",
				n, p, span, 2024+n/p, compared, worse, mGo, mDOS)
		}
	}
}

// rtRateRecovery runs one rate-axis round trip and returns the two recovery
// errors, with the per-case quantum ALREADY folded into the DOS side so callers
// compare bare. `why` names the reason a case dropped out, so a stratum that
// measured nothing cannot be mistaken for a stratum that measured zero
// divergences (START_HERE §5: "a small-sample zero is not closure").
type rtRateResult struct {
	ok             bool
	errGo, errDOS  float64 // errDOS already carries the per-case quantum
	rGo, rDOS, pay float64
	cmd            string // the exact repro line
	why            string // "" | "refused" | "forward" | "goerr"
}

func rtRateRecovery(s rtScreen) rtRateResult {
	fwd, ok0 := rtOracle(s.args())
	if !ok0 {
		return rtRateResult{why: "refused"}
	}
	payDOS := fwd["payment"]
	gpay, _, gok := goSolve(s.amount, s.rate, s.n, s.perYr)
	if !gok {
		return rtRateResult{why: "forward"}
	}
	if math.Abs(gpay-payDOS) > 0.01+1e-9*math.Abs(payDOS) {
		return rtRateResult{why: "forward"}
	}
	pay := math.Round(payDOS*100) / 100
	payTok := "payhard=" + strconv.FormatFloat(pay, 'f', 2, 64)
	repro := s.cmd(payTok, "norate")

	got, ok1 := rtOracle(s.args(payTok, "norate"))
	if !ok1 {
		return rtRateResult{cmd: repro, why: "refused"}
	}
	rDOS, has := got["solvedrate"]
	if !has {
		return rtRateResult{cmd: repro, why: "refused"}
	}
	in := rtInput(s)
	in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, pay
	in.Loan.LoanRateStatus, in.Loan.LoanRate = types.StatusEmpty, 0
	rGo, _, err := SolveRate(in)
	if err != nil {
		return rtRateResult{cmd: repro, why: "goerr"}
	}
	q := rtQuantumRate(s.rate, pay, s.n)
	return rtRateResult{
		ok: true, errGo: math.Abs(rGo - s.rate), errDOS: math.Abs(rDOS-s.rate) + q,
		rGo: rGo, rDOS: rDOS, pay: pay, cmd: repro,
	}
}

var _ = fmt.Sprintf
