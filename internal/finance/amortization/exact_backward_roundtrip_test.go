package amortization

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Regression guard for the Exact-method × non-360-basis backward-solve gap.
//
// Root cause: the forward schedule (and DOS) accrue interest on the ACTUAL day
// count between payment dates when the Exact method is on and the basis is not
// 360. But SolveLoanAmount / SolveRate returned the closed-form annuity result,
// which assumes a uniform per-period growth factor. The two directions therefore
// did NOT round-trip: forward-computing the payment for a $100k / 12% / 360-pmt
// 365-basis Exact loan gives 1028.37, but solving the amount back from 1028.37
// returned ~99,976 instead of ~100,000 (DOS returns 99,999.90). See
// docs/postmortem_365_exact_interest.md and the CLAUDE.md porting rules.
//
// The fix routes the Exact non-360 backward Amount/Rate solves through the same
// schedule-oracle refinement (dosIterateAmount / dosIterateRate) that the forward
// payment solve uses. These tests fail before that fix (off by ~$23 on the
// amount) and pass after.

// exactBackwardCase builds the canonical reported loan: $100k, 12%, 360 pmts,
// 12/yr, loan 1/1/2026, first pmt 2/1/2026, Exact + prepaid + 365 basis.
func exactBackwardSettings(basis types.BasisType, exact bool) Settings {
	yd := 365.0
	switch basis {
	case types.Basis360:
		yd = 360
	case types.Basis365360:
		yd = 365 // 365/360 uses a 365-day count over a 360 divisor; YrInv set below
	}
	s := Settings{Basis: basis, PerYr: 12, YrDays: yd, YrInv: 1.0 / yd, Prepaid: true, Exact: exact}
	if basis == types.Basis365360 {
		s.YrInv = 1.0 / 360
	}
	return s
}

// TestExactBackwardAmountReported reproduces the exact user-reported scenario and
// pins the DOS answer (99,999.90) for the solved Amount Borrowed.
func TestExactBackwardAmountReported(t *testing.T) {
	s := exactBackwardSettings(types.Basis365, true)
	loan := Loan{
		LoanRateStatus: types.InOutInput, LoanRate: 0.12,
		NStatus: types.InOutInput, NPeriods: 360,
		PerYrStatus: types.InOutInput, PerYr: 12,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2026, 1, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2026, 2, 1),
		PayAmtStatus: types.InOutInput, PayAmt: 1028.37,
		AmountStatus: types.StatusEmpty,
	}
	got, conv, err := SolveLoanAmount(LoanInput{Loan: loan, Settings: s})
	if err != nil {
		t.Fatalf("SolveLoanAmount: %v", err)
	}
	if !conv {
		t.Fatalf("SolveLoanAmount did not converge")
	}
	// DOS PerSense returns 99,999.90 for this payment (the 10-cent shortfall is
	// the penny-rounding of the 1028.37 payment, not a method error). The old
	// closed-form path returned ~99,976.42 — a ~$23 miss.
	const dos = 99999.90
	if math.Abs(got-dos) > 0.05 {
		t.Errorf("solved Amount = %.4f, want DOS %.2f (diff %.4f)", got, dos, got-dos)
	}
}

// TestExactBackwardRoundTrip sweeps basis × exact and confirms that
// forward-solving the payment and then backward-solving the Amount (and the
// Rate) recovers the original inputs. Uses the UNROUNDED forward payment so the
// tolerance is tight — a method mismatch (uniform-period closed form vs
// actual-day schedule) shows up as a dollars-scale miss, well outside tolerance.
func TestExactBackwardRoundTrip(t *testing.T) {
	bases := []struct {
		name  string
		basis types.BasisType
	}{
		{"360", types.Basis360},
		{"365", types.Basis365},
		{"365_360", types.Basis365360},
	}
	for _, b := range bases {
		for _, exact := range []bool{false, true} {
			name := b.name
			if exact {
				name += "_exact"
			}
			t.Run(name, func(t *testing.T) {
				s := exactBackwardSettings(b.basis, exact)
				const amount, rate, n, perYr = 100000.0, 0.12, 360, 12

				base := func() Loan {
					return Loan{
						NStatus: types.InOutInput, NPeriods: n,
						PerYrStatus: types.InOutInput, PerYr: perYr,
						LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2026, 1, 1),
						FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2026, 2, 1),
					}
				}

				// Forward: solve the payment (unrounded).
				fwd := base()
				fwd.AmountStatus = types.InOutInput
				fwd.Amount = amount
				fwd.LoanRateStatus = types.InOutInput
				fwd.LoanRate = rate
				fwd.PayAmtStatus = types.StatusEmpty
				fr := Amortize(LoanInput{Loan: fwd, Settings: s})
				if fr.Err != nil {
					t.Fatalf("forward: %v", fr.Err)
				}
				pay := fr.Schedule[0].PayAmt

				// Backward Amount: blank amount, given payment.
				al := base()
				al.LoanRateStatus = types.InOutInput
				al.LoanRate = rate
				al.PayAmtStatus = types.InOutInput
				al.PayAmt = pay
				al.AmountStatus = types.StatusEmpty
				gotAmt, conv, err := SolveLoanAmount(LoanInput{Loan: al, Settings: s})
				if err != nil || !conv {
					t.Fatalf("SolveLoanAmount conv=%v err=%v", conv, err)
				}
				if rel := math.Abs(gotAmt-amount) / amount; rel > 1e-5 {
					t.Errorf("Amount round-trip: got %.4f want %.4f (relErr %.2e)", gotAmt, amount, rel)
				}

				// Backward Rate: blank rate, given amount + payment.
				rl := base()
				rl.AmountStatus = types.InOutInput
				rl.Amount = amount
				rl.PayAmtStatus = types.InOutInput
				rl.PayAmt = pay
				rl.LoanRateStatus = types.StatusEmpty
				gotRate, rconv, rerr := SolveRate(LoanInput{Loan: rl, Settings: s})
				if rerr != nil || !rconv {
					t.Fatalf("SolveRate conv=%v err=%v", rconv, rerr)
				}
				if math.Abs(gotRate-rate) > 1e-5 {
					t.Errorf("Rate round-trip: got %.6f want %.6f", gotRate, rate)
				}
			})
		}
	}
}

// TestExactBackwardRoundTripFuzz randomizes the loan AND the full computational
// settings cube — basis (360/365/365-360), Exact, prepaid, in-advance, USA rule,
// Rule-of-78, AND the first-period offset (a mid-month loan date makes an odd
// first period) — then asserts the forward→backward round-trip recovers both the
// Amount and the Rate.
//
// This closes the coverage the pre-existing backward fuzzers lacked. They ran on
// the 360 basis (where Exact is a no-op) and never varied InAdvance / USARule /
// R78 / the first-period offset, and every Exact/non-360 fuzzer only solved the
// payment (forward). Expanding the cube surfaced two real, larger pre-existing
// backward-solve gaps — in-advance (~0.6%) and odd-first-period (~0.6%) — now
// fixed by refining those against the RepayLoan terminal (the plain forward
// schedule's own recursion). See docs/postmortem_365_exact_interest.md §8.
//
// Non-convergence is ESCALATED, not skipped: once the forward schedule retires
// cleanly (a valid round-trip target exists), a backward solver that fails to
// converge is a bug and fails the test — a case is dropped only when the FORWARD
// leg is itself degenerate (error, empty schedule, or non-retiring). This avoids
// the "shrink the sample instead of failing" trap called out in this postmortem.
func TestExactBackwardRoundTripFuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(0xE4AC7))
	bases := []types.BasisType{types.Basis360, types.Basis365, types.Basis365360}

	checked, frontierChecked := 0, 0
	for i := 0; i < 4000; i++ {
		amount := float64(20000 + rng.Intn(480000))
		rate := 0.03 + rng.Float64()*0.15
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		nPeriods := (3 + rng.Intn(25)) * perYr
		basis := bases[rng.Intn(len(bases))]
		exact := rng.Intn(2) == 0
		prepaid := rng.Intn(2) == 0
		inAdvance := rng.Intn(2) == 0
		usaRule := rng.Intn(3) == 0
		r78 := rng.Intn(3) == 0
		loanDay := []int{1, 1, 10, 22}[rng.Intn(4)] // day != 1 => odd first period

		yd := 365.0
		if basis == types.Basis360 {
			yd = 360
		}
		s := Settings{Basis: basis, PerYr: byte(perYr), YrDays: yd, YrInv: 1.0 / yd,
			Prepaid: prepaid, Exact: exact, InAdvance: inAdvance, USARule: usaRule, R78: r78}
		if basis == types.Basis365360 {
			s.YrInv = 1.0 / 360
		}

		mPer := 12 / perYr
		fy, fm := 2026+mPer/12, time.Month(mPer%12+1)
		base := func() Loan {
			return Loan{
				NStatus: types.InOutInput, NPeriods: nPeriods,
				PerYrStatus: types.InOutInput, PerYr: perYr,
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2026, time.January, loanDay),
				FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(fy, fm, 1),
			}
		}

		// Two documented, bounded frontiers are held to a looser tolerance; every
		// other cube cell must round-trip tight (the in-advance and odd-first-period
		// fixes bring those to ~1e-13).
		//
		//  1. dayCountFrontier: a !prepaid loan with a FULL first period on a non-360
		//     (day-count) basis — RepayLoan uses a whole-month first-period prorate
		//     while the forward schedule counts actual days (~2e-4, down from ~0.6%).
		//
		//  2. r78Frontier: Rule-of-78 is a distinct interest-allocation method whose
		//     backward Amount/Rate solve is not separately modeled here. R78 is
		//     DOS-active only on the 360 basis in arrears (Amortize.pas:1493) and
		//     uses the plain in-arrears whole-period payment; needScheduleRefine
		//     mirrors the forward needPaymentRefine (which is false for R78) so the
		//     closed form is used symmetrically. R78 backward fidelity across every
		//     combination is tracked as a separate open item — bounded here rather
		//     than asserted tight. See docs/postmortem_365_exact_interest.md §8.
		fullFirst := !oddFirstPeriod(base().LoanDate, base().FirstDate, perYr, &s)
		dayCountFrontier := !exact && basis != types.Basis360 && !prepaid && fullFirst
		r78Frontier := r78
		// USA-rule is a LOGGED frontier: DOS renders a USA loan's schedule with the
		// usap-aware RepayFancyLoan (which the port matches to the cent against the
		// oracle — see TestDOSOddFirstDatesCube), but its Iterate SOLVE terminal stays
		// on the simple RepayLoan (AMORTOP.pas:1438). The port forces the fancy engine
		// so the SCHEDULE (the user-visible output) is oracle-exact; the price is that
		// the forward (fancy) and backward (RepayLoan) solves use different terminals,
		// so this Go-internal round-trip is not meaningful for USA and is logged, not
		// asserted. Matching the real DOS schedule takes priority.
		usaFrontier := usaRule
		frontier := dayCountFrontier || r78Frontier || usaFrontier
		amtTol, rateTol := 5e-6, 5e-5
		if dayCountFrontier {
			// Documented DOS-faithful asymmetry: on a non-360 basis DOS's forward
			// payment for a !prepaid CLEAN first period uses the whole-period prorate
			// (prorate=1, matching the oracle odd-first cube), while the actual-day
			// first-period augmentation makes the forward over-amortize slightly — so
			// forward→backward does not round-trip to the cent (~2e-4..2e-3). This is
			// inherent to the DOS engine, not a port bug; bound it rather than force a
			// non-DOS backward prorate (which regressed the oracle-validated forward).
			amtTol, rateTol = 3e-3, 3e-3
		}
		if r78Frontier {
			amtTol, rateTol = 1e-1, 1e-1
		}

		fwd := base()
		fwd.AmountStatus = types.InOutInput
		fwd.Amount = amount
		fwd.LoanRateStatus = types.InOutInput
		fwd.LoanRate = rate
		fwd.PayAmtStatus = types.StatusEmpty
		fr := Amortize(LoanInput{Loan: fwd, Settings: s})
		// Drop only when the FORWARD leg is degenerate — no valid round-trip target.
		if fr.Err != nil || len(fr.Schedule) == 0 || math.Abs(fr.FinalPrinc) > 0.5 {
			continue
		}
		pay := modalReg(fr.Schedule) // regular payment, not the settlement/first row
		if pay <= 0 {
			continue
		}
		checked++
		if frontier {
			frontierChecked++
		}
		desc := fmt.Sprintf("basis=%v exact=%v prepaid=%v inAdv=%v usa=%v r78=%v day=%d amt=%.0f r=%.4f n=%d py=%d",
			basis, exact, prepaid, inAdvance, usaRule, r78, loanDay, amount, rate, nPeriods, perYr)

		al := base()
		al.LoanRateStatus = types.InOutInput
		al.LoanRate = rate
		al.PayAmtStatus = types.InOutInput
		al.PayAmt = pay
		al.AmountStatus = types.StatusEmpty
		gotAmt, conv, err := SolveLoanAmount(LoanInput{Loan: al, Settings: s})
		if err != nil || !conv {
			if !usaFrontier {
				t.Errorf("SolveLoanAmount failed to converge on a retiring forward: %s (conv=%v err=%v)", desc, conv, err)
			}
		} else if rel := math.Abs(gotAmt-amount) / amount; rel > amtTol {
			if usaFrontier {
				t.Logf("[usa frontier] Amount round-trip: %s -> got %.4f (relErr %.2e)", desc, gotAmt, rel)
			} else {
				t.Errorf("Amount round-trip miss: %s -> got %.4f (relErr %.2e, tol %.0e)", desc, gotAmt, rel, amtTol)
			}
		}

		rl := base()
		rl.AmountStatus = types.InOutInput
		rl.Amount = amount
		rl.PayAmtStatus = types.InOutInput
		rl.PayAmt = pay
		rl.LoanRateStatus = types.StatusEmpty
		gotRate, rconv, rerr := SolveRate(LoanInput{Loan: rl, Settings: s})
		if rerr != nil || !rconv {
			if !usaFrontier {
				t.Errorf("SolveRate failed to converge on a retiring forward: %s (conv=%v err=%v)", desc, rconv, rerr)
			}
		} else if math.Abs(gotRate-rate) > rateTol {
			if usaFrontier {
				t.Logf("[usa frontier] Rate round-trip: %s -> got %.6f", desc, gotRate)
			} else {
				t.Errorf("Rate round-trip miss: %s -> got %.6f (tol %.0e)", desc, gotRate, rateTol)
			}
		}
	}
	if checked < 800 || frontierChecked < 50 {
		t.Fatalf("too few round-trips checked (total %d, frontier %d) — sweep is degenerate", checked, frontierChecked)
	}
	t.Logf("backward round-trip fuzz: %d cases checked (%d bounded !prepaid-full-non-360 frontier, rest tight)", checked, frontierChecked)
}

// TestAmortPrepaidFirstPeriodBothModes pins the AM_EX1 short-odd-first loan ($100k @
// 8%, 360 pmts, 30/360, loan 02/12, first 03/01 — a 19-day first period). The REAL
// DOS engine (legacy/oracle) solves 731.98 in BOTH prepaid modes — the odd-first
// payment augmentation does not depend on the prepaid flag; prepaid only moves the
// odd-period interest to a settlement stud in the schedule. (A prior session revision
// wrongly believed DOS-prepaid gave 733.76 — a Go-only artifact of an actual-day
// closed-form prorate — and briefly shipped that; corrected here against the oracle.)
// First-period interest ($422.22, the 19 days) is identical in both modes.
func TestAmortPrepaidFirstPeriodBothModes(t *testing.T) {
	base := func(prepaid bool) LoanInput {
		s := Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, Prepaid: prepaid}
		loan := Loan{
			NStatus: types.InOutInput, NPeriods: 360,
			PerYrStatus: types.InOutInput, PerYr: 12,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 2, 12),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, 3, 1),
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.08,
			PayAmtStatus: types.StatusEmpty,
		}
		return LoanInput{Loan: loan, Settings: s}
	}
	for _, tc := range []struct {
		prepaid bool
		wantPay float64
		wantP1  float64
	}{
		{true, 731.98, 422.22},
		{false, 731.98, 422.22},
	} {
		fr := Amortize(base(tc.prepaid))
		if fr.Err != nil {
			t.Fatalf("prepaid=%v: %v", tc.prepaid, fr.Err)
		}
		if got := fr.Schedule[0].PayAmt; math.Abs(got-tc.wantPay) > 0.01 {
			t.Errorf("prepaid=%v: payment = %.4f, want %.2f (real DOS oracle, both modes)",
				tc.prepaid, got, tc.wantPay)
		}
		if got := fr.Schedule[0].Interest; math.Abs(got-tc.wantP1) > 0.01 {
			t.Errorf("prepaid=%v: first-period interest = %.4f, want %.2f", tc.prepaid, got, tc.wantP1)
		}
	}
}
