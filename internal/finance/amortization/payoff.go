package amortization

import (
	"fmt"
	"math"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// PayoffBalance computes the balance / payoff owed on a loan as of asOf, a faithful
// port of DOS ComputeBalanceFromDate (legacy/src/dos_source/Amortize.pas:1090-1151,
// the w^ "as-of balance" payoff pointer). MakeTable invokes it at Amortize.pas:1424
// when a payoff date is set.
//
// DOS uses DIFFERENT formulas per computational mode — the point the old client-side
// JS payoff tool missed (it always applied the arrears accrual, so an in-advance or
// Rule-of-78 loan came out wrong, e.g. exceeding the original principal):
//
//	before the loan date              -> error (no meaning)                    :1095
//	loan date .. 1st payment          -> amount grown at the loan rate, less
//	                                     the prepaid odd-period interest        :1097
//	after the loan ends               -> 0                                      :1102
//	Rule-of-78 (and not fancy)        -> sum-of-digits balance                  :1134
//	in-advance                        -> balance DISCOUNTED to the next payment :1125
//	arrears                           -> balance ACCRUED since the last payment :1127
//
// DOS's branch order puts R78 (non-fancy) ahead of the in-advance/arrears split
// (:1107 `if (not R78) or fancy ... else {R78}`), so a plain R78 loan takes the R78
// formula even when in-advance is also set — matching the DOS engine exactly (see
// TestPayoffVsOracleSweep).
//
// The per-payment balances come from the engine's own DOS-validated schedule, so the
// payoff stays consistent with the schedule the user sees. Validated to the cent
// against the real DOS engine via the extended amort_oracle `payoff=` query.
func PayoffBalance(input LoanInput, asOf types.DateRec) (float64, error) {
	if !dateutil.DateOK(asOf) {
		return 0, fmt.Errorf("Enter a payoff date (MM/DD/YYYY) to look up the balance owed on that date.")
	}
	loan := input.Loan
	s := input.Settings

	// DOS Amortize.pas:1095 — a balance before the loan date is meaningless.
	if dateutil.DateComp(asOf, loan.LoanDate) < 0 {
		return 0, fmt.Errorf("The payoff date is before the loan date. Enter a date on or after the loan date.")
	}

	res := Amortize(input)
	if res.Err != nil {
		return 0, res.Err
	}
	amount := loan.Amount
	rate := loan.LoanRate

	firstDate := res.FirstDate
	if !dateutil.DateOK(firstDate) {
		firstDate = loan.FirstDate
	}

	// very_last = the last scheduled regular payment date.
	var veryLast types.DateRec
	for i := range res.Schedule {
		if res.Schedule[i].PayNum >= 1 {
			veryLast = res.Schedule[i].Date
		}
	}

	// DOS Amortize.pas:1097 — between the loan date and the first payment, the
	// balance is the principal grown at the loan rate, less the prepaid odd-period
	// interest when the loan is prepaid (you get some of it back).
	//
	// "Prepaid" here is DOS's FORCED global: FirstPass turns it on for in-advance
	// loans (Amortize.pas:206-209), so an in-advance payoff before the first
	// payment rebates the annuity-due settlement interest too. 2026-07-11 audit
	// finding 20e — verified vs the real DOS engine:
	//
	//	amort_oracle 100000 0.10 120 12 inadv payoff=15.1.2024 → 99555.5556
	//	(Go returned the arrears 100388.8889; Δ = the 833.33 settlement interest)
	if dateutil.DateComp(asOf, firstDate) < 0 {
		bal := amount * (1 + rate*dateutil.YearsDif(asOf, loan.LoanDate, s.Basis, s.YrInv, true))
		if s.Prepaid || s.InAdvance {
			bal -= settlementInterest(res)
		}
		return bal, nil
	}

	// DOS Amortize.pas:1102 — after the loan is fully paid, the balance is 0.
	if dateutil.DateOK(veryLast) && dateutil.DateComp(asOf, veryLast) > 0 {
		return 0, nil
	}

	fancy := hasAnyAdvancedOption(input)

	// DOS Amortize.pas:1134 — Rule-of-78 (non-fancy) sum-of-digits balance.
	if s.R78 && !fancy {
		d := payoffRegularPayment(res, loan)
		n := float64(loan.NPeriods)
		// i = installments made on or before the payoff date, and lastInstall = the
		// date of that last installment (DOS NumberOfInstallments returns it in the
		// var `lastpmtdate` — the crux the first cut missed). Uses the ENTERED first
		// date, not any in-advance shift, exactly as DOS does.
		i, lastInstall := dateutil.NumberOfInstallments(firstDate, asOf, int(loan.PerYr), types.OnOrBefore)
		fi := float64(i)
		if denom := 0.5 * n * (n + 1); denom != 0 {
			r78base := (n*d - amount) / denom
			r78interest := r78base * fi * (-0.5*fi + (n + 0.5))
			bal := amount + r78interest - fi*d // balance as of the last installment
			// DOS Amortize.pas:1144-1147: if the payoff date IS the last installment
			// date, add one payment back (the balance BEFORE that payment); otherwise
			// accrue simple interest from the last installment to the payoff date.
			if dateutil.DateComp(asOf, lastInstall) == 0 {
				bal += d
			} else {
				bal *= 1 + rate*dateutil.YearsDif(asOf, lastInstall, s.Basis, s.YrInv, true)
			}
			return bal, nil
		}
	}

	// DOS Amortize.pas:1107-1132 — arrears / in-advance. `payment` is the last
	// regular payment STRICTLY before asOf (its post-payment balance is the base);
	// `nextpayment` is the first payment on or after asOf. This mirrors how DOS's
	// RepayFancyLoan stops with very_last = asOf (the loop breaks once the next
	// payment reaches asOf).
	//
	// The row source is normally the engine's own display schedule — but DOS's
	// payoff walk is ALWAYS RepayFancyLoan, where the US Rule accrues interest on
	// (p − usap), even though DOS's DISPLAY for a plain (non-fancy) USA loan takes
	// the simple loop where usap is inert. So for a plain USA-rule loan the payoff
	// balances must come from the usap-aware fancy walk, NOT the display rows —
	// DOS's payoff intentionally disagrees with its own displayed balance column
	// there. 2026-07-11 audit finding 20c, verified vs the real DOS engine
	// (100000 0.08 360 12 payhard=600 usa, a neg-am loan):
	//
	//	payoff=1.7.2026  → DOS 102612.9862 (display row 6/1/26 shows 102125.10 on BOTH sides)
	//	payoff=15.1.2029 → DOS 104323.7562
	//	payoff=1.1.2054  → DOS 124760.7602
	payoffSched := res.Schedule
	if s.USARule && !fancy && !s.R78 {
		wl := loan
		wl.NPeriods = res.NPeriods
		wl.FirstDate = firstDate
		if dateutil.DateOK(res.LastDate) {
			wl.LastDate = res.LastDate
			wl.LastOK = true
		}
		in2 := input
		in2.Loan = wl
		s2 := s
		tr, _ := ComputeTrueRate(&wl, &s2)
		fg := GrowthPerPeriod(&wl, s2.YrInv)
		wr := generateFancyScheduleMode(in2, payoffRegularPayment(res, loan), &s2, tr, fg, false)
		if wr.Err == nil && len(wr.Schedule) > 0 {
			payoffSched = wr.Schedule
		}
	}
	balance := amount
	lastPmtDate := loan.LoanDate
	if s.Prepaid {
		// DOS :1118-1120 — for a prepaid loan with asOf before the first payment
		// walk, payment.date is one period before the first date.
		if pd, err := dateutil.AddPeriod(firstDate, int(loan.PerYr), firstDate.Time.Day(), true); err == nil {
			lastPmtDate = pd
		}
	}
	nextPmtDate := firstDate
	for i := range payoffSched {
		r := payoffSched[i]
		if r.PayNum < 1 {
			continue
		}
		if dateutil.DateComp(r.Date, asOf) < 0 {
			balance = r.Principal
			lastPmtDate = r.Date
		} else {
			nextPmtDate = r.Date
			break
		}
	}

	rif := payoffRateInForce(input, res, asOf)
	if s.InAdvance {
		// DOS's in-advance payoff (Amortize.pas:1124-1125) reads
		// `payment.principal` and `nextpayment.date` from the balance_calc
		// RepayFancyLoan walk — a DISTINCT walk from the display schedule. For
		// in-advance the walk's base_date starts at firstdate (not firstdate−1),
		// so its payment dates are shifted one period later, and each period
		// accrues plain opening-balance interest (ComputeNext, AMORTOP.pas:636)
		// with NO settlement row. Reading the DISPLAY rows' balances/dates (the
		// old code) mis-selects both operands and left every in-advance payoff
		// ~0.2–0.7% low. Reconstruct the walk directly. 2026-07-13 pass-4 —
		// verified vs the real DOS engine:
		//
		//	amort_oracle 100000 0.0632 48 12 payhard=684.67 payoff=15.6.2025 inadv
		//	→ 97096.1096 (the display-row formula gave 96909.2958)
		// The walk models moratorium, target, and skip months (the ComputeNext
		// schedule-shaping options). It does NOT model dated extras — balloons,
		// prepayment series, or rate/payment adjustments — whose principal path the
		// display schedule captures but this closed walk does not. Only attempt the
		// walk when no such extra is present; otherwise fall through to the
		// display-row rebate.
		if len(input.Balloons) == 0 && len(input.Prepayments) == 0 && len(input.Adjustments) == 0 {
			if bal, ok := inAdvancePayoffBalance(loan, payoffRegularPayment(res, loan),
				&s, firstDate, truerateFor(&loan, &s), asOf, rif,
				loan.PayAmtStatus == types.InOutInput, input.Moratorium, input.Target,
				input.SkipMonths); ok {
				return bal, nil
			}
		}
		// Fallback (fancy in-advance the walk does not model — dated extras): the
		// display-row rebate. DOS :1125 — rebate the prepaid interest from asOf to
		// the next payment.
		return balance * (1 - rif*dateutil.YearsDif(nextPmtDate, asOf, s.Basis, s.YrInv, true)), nil
	}
	// DOS :1127 — accrue interest from the last payment to asOf.
	return balance * (1 + rif*dateutil.YearsDif(asOf, lastPmtDate, s.Basis, s.YrInv, true)), nil
}

// truerateFor returns the continuously-compounded true rate (for daily
// compounding) or 0 when daily mode is off.
func truerateFor(loan *Loan, s *Settings) float64 {
	if !s.Daily {
		return 0
	}
	tr, _ := ComputeTrueRate(loan, s)
	return tr
}

// inAdvancePayoffBalance reconstructs DOS's in-advance balance_calc walk
// (RepayFancyLoan called from ComputeBalanceFromDate, Amortize.pas:1114-1125)
// for a plain (non-advanced-option) in-advance loan and returns the payoff
// balance. It replicates Paymenttype.ComputeNext (AMORTOP.pas:596-664) with the
// in-advance base-date initialization (base_date := firstdate, AMORTOP.pas:
// 1159-1177) so the walk's `payment.principal` and `nextpayment.date` match
// DOS's, then applies DOS's rebate `payment.principal·(1 − rif·YearsDif(
// nextpayment.date, asOf))`. Returns ok=false when the loan carries options the
// walk does not model (the caller then falls back to the display-row formula).
func inAdvancePayoffBalance(loan Loan, d float64, s *Settings, firstDate types.DateRec,
	truerate float64, asOf types.DateRec, rif float64, hardPayment bool,
	mor Moratorium, targ Target, skip SkipMonths) (float64, bool) {
	if d <= 0 || !dateutil.DateOK(firstDate) {
		return 0, false
	}
	origDay := firstDate.Time.Day()
	peryr := int(loan.PerYr)
	// DOS in-advance init: t := firstdate − 1 period, then + 1 period back to
	// firstdate (AMORTOP.pas:1149-1165); paidthru := firstdate (the `if not
	// in_advance` subtract at :1154 is skipped for in-advance). So base_date and
	// prevdate both start at firstdate.
	baseDate := firstDate
	prevDate := firstDate
	p := loan.Amount

	// One ComputeNext step (AMORTOP.pas:596-664), advancing base_date/prevdate
	// and p, and reporting the new payment date. Returns the payment date.
	step := func() types.DateRec {
		date, err := dateutil.AddPeriod(baseDate, peryr, origDay, false)
		if err != nil {
			return date
		}
		var timedif float64
		if (s.Basis == types.Basis360 || !s.Exact) && dateutil.DaysCloseEnough(date, prevDate, peryr) {
			timedif = float64(date.Time.Year()-prevDate.Time.Year()) +
				float64(int(date.Time.Month())-int(prevDate.Time.Month()))/12
			if peryr == 24 {
				timedif += math.Round(2*float64(int(date.Time.Day())-prevDate.Time.Day())/30) / 24
			}
		} else {
			timedif = dateutil.YearsDif(date, prevDate, s.Basis, s.YrInv, true)
		}
		var intr float64
		if s.Daily {
			ev, _ := interest.Exxp(truerate * timedif)
			intr = (ev - 1) * p
		} else {
			intr = loan.LoanRate * timedif * p
		}
		if hardPayment {
			intr = interest.Round2(intr)
		}
		// Skip months zero payamt FIRST (ComputeNext AMORTOP.pas:599 —
		// `if (date.m in skipmonthset) then payamt:=0 else payamt := d`), before
		// the moratorium/target arm below.
		payamt := d
		if skip.SkipStatus >= types.InOutDefault && int(date.Time.Month()) >= 1 &&
			int(date.Time.Month()) <= 12 && skip.MonthSet[date.Time.Month()] {
			payamt = 0
		}
		// Moratorium interest-only / target floor (ComputeNext balloonpos=1
		// arm, AMORTOP.pas:647-650). The moratorium OVERWRITES the skip-zero
		// (a skipped month inside the moratorium still accrues interest-only);
		// the target floor sees payamt=0 for a past-moratorium skipped month, so
		// 0−interest < target ⇒ payamt := target+interest (target overrides skip).
		if mor.FirstRepayStatus >= types.InOutDefault && dateutil.DateComp(date, mor.FirstRepay) < 0 {
			payamt = intr
		} else if targ.TargetStatus >= types.InOutDefault && payamt-intr < targ.TargetValue {
			payamt = targ.TargetValue + intr
		}
		prevDate = date
		p = p + intr - payamt
		baseDate = date
		return date
	}

	// Pre-loop ComputeNext (AMORTOP.pas:1195) establishes the first payment.
	nextDate := step()
	paymentPrincipal := loan.Amount
	// Loop: Payment := NextPayment; advance NextPayment; stop when
	// NextPayment.date >= very_last(=asOf) (AMORTOP.pas:1200-1220, WhenToStop =
	// @NextPayment for balance_calc).
	guard := 0
	for dateutil.DateComp(nextDate, asOf) < 0 {
		paymentPrincipal = p
		nextDate = step()
		guard++
		if guard > MaxSchedulePeriods {
			return 0, false
		}
	}
	return paymentPrincipal * (1 - rif*dateutil.YearsDif(nextDate, asOf, s.Basis, s.YrInv, true)), true
}

// settlementInterest returns the odd first-period interest that a prepaid loan
// collects at settlement — the PayNum-0 stub row's interest (DOS PrepaidInterest).
func settlementInterest(res AmortResult) float64 {
	for i := range res.Schedule {
		if res.Schedule[i].PayNum == 0 {
			return res.Schedule[i].Interest
		}
	}
	return 0
}

// payoffRegularPayment returns the loan's regular (modal) payment — the user's
// input when given, else the modal scheduled payment (skips settlement/first/last
// oddities), matching DOS h^.payamt.
func payoffRegularPayment(res AmortResult, loan Loan) float64 {
	if loan.PayAmtStatus >= types.InOutInput && loan.PayAmt > 0 {
		return loan.PayAmt
	}
	// Modal regular payment: the most common PayAmt among regular rows (PayNum>=1),
	// which skips the settlement stub, an augmented odd first period, and the
	// adjusted final row.
	counts := map[int64]int{}
	amtOf := map[int64]float64{}
	bestKey, bestN := int64(0), 0
	for i := range res.Schedule {
		r := res.Schedule[i]
		if r.PayNum < 1 || r.PayAmt <= 0 {
			continue
		}
		k := int64(r.PayAmt*100 + 0.5)
		counts[k]++
		amtOf[k] = r.PayAmt
		if counts[k] > bestN {
			bestN, bestKey = counts[k], k
		}
	}
	if bestN > 0 {
		return amtOf[bestKey]
	}
	return loan.PayAmt
}

// payoffRateInForce returns the annual rate DOS's payoff stub accrues at — a
// faithful port of DOS RateInForce (Amortize.pas:617-626):
//
//	i:=1;
//	while (i<nlines[AMZAdjBlock]) and (DateComp(date,adj[i]^.date)>=0) do inc(i);
//	RateInForce:=adj[i]^.loanrate;
//
// i.e. the FORWARD scan stops at the first adjustment dated strictly AFTER the
// payoff date (or the last row), and returns THAT row's rate. Consequences the
// port must reproduce (2026-07-11 audit finding 20d, all oracle-verified):
// once any adjustment exists the BASE rate is unreachable — a payoff stub
// before the first ARM accrues at the ARM's future rate (`adj=48:0.09:
// payoff=1.2.2024` → 100750.00 = 9%, not 8%); between two ARMs the stub uses
// the SECOND one's rate (`adj=48:0.09: adj=96:0.11: payoff=1.2.2029` →
// 88585.3257, the 11%); and a payment-only AO6 row carries the IMPLIED rate
// DOS solved for it (EstimateAndRefineAdjRate), which the port recomputes here
// via solveAdjRate on the schedule state at the adjustment date.
func payoffRateInForce(input LoanInput, res AmortResult, asOf types.DateRec) float64 {
	loan := input.Loan
	var adjs []RateAdjustment
	for i := range input.Adjustments {
		if input.Adjustments[i].DateStatus >= types.InOutDefault &&
			dateutil.DateOK(input.Adjustments[i].Date) {
			adjs = append(adjs, input.Adjustments[i])
		}
	}
	if len(adjs) == 0 {
		return loan.LoanRate
	}
	SortAdjustments(adjs)
	i := 0
	for i < len(adjs)-1 && dateutil.DateComp(asOf, adjs[i].Date) >= 0 {
		i++
	}
	a := adjs[i]
	if a.LoanRateStatus >= types.InOutDefault {
		return a.LoanRate
	}
	if a.AmountStatus >= types.InOutDefault && a.Amount > 0 {
		// AO6 (payment-only): DOS filled adj^.loanrate with the implied rate it
		// solved so the new payment retires the balance over the remaining term.
		bal := loan.Amount
		made := 0
		for k := range res.Schedule {
			r := res.Schedule[k]
			if r.PayNum < 1 {
				continue
			}
			// A payment dated ON the adjustment date is made BEFORE the
			// adjustment applies — DOS pays the installment first and only
			// then re-solves the adjusted payment (same order as Go's own
			// forward walk, whose rows match DOS), so the implied-rate
			// recompute sees that payment's balance and one fewer remaining
			// period. 2026-07-12 pass-3 finding AF1 — verified vs the real
			// DOS engine:
			//
			//	amort_oracle 100000 0.09 120 12 payhard=1300 adj=24::1100 payoff=1.2.2024
			//	→ 100449.9644 (solveAdjRate(85596.32, 1100, 96) = 0.05399573
			//	reproduces DOS to 9 digits; strict < gave 97 remaining →
			//	0.05417546 → 100451.4622)
			if dateutil.DateComp(r.Date, a.Date) <= 0 {
				bal = r.Principal
				made++
			} else {
				break
			}
		}
		if remaining := res.NPeriods - made; remaining > 0 {
			if r, ok := solveAdjRate(bal, a.Amount, remaining, loan, input.Settings.YrInv); ok {
				return r
			}
		}
	}
	// AO7 (date-only) or unsolvable AO6: DOS re-amortizes at the rate already in
	// force, so fall back to the nearest earlier rate-bearing row, else the base.
	for k := i - 1; k >= 0; k-- {
		if adjs[k].LoanRateStatus >= types.InOutDefault {
			return adjs[k].LoanRate
		}
	}
	return loan.LoanRate
}
