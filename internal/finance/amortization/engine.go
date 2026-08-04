package amortization

import (
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// exactDaily reports whether the loan uses DOS's "exact" interest method on a
// non-360 basis: true daily (actual-day/365) accrual where no closed-form
// formula applies and every period must be iterated. DOS requires BOTH the
// Exact setting AND a non-360 basis — the Exact help text states "365 DAY MUST
// ALSO BE SELECTED" and AMORTOP.pas:625 gates the actual-day `YearsDif` branch
// on `not ((basis=x360) or (not exact))`. On the 360 basis Exact is a no-op,
// matching DOS. See docs/postmortem_365_exact_interest.md.
func exactDaily(s *Settings) bool {
	return s.Exact && s.Basis != types.Basis360
}

// GrowthPerPeriod computes the growth factor per payment period.
// This is (1 + rate/n) where n is the effective periods per year,
// with special handling for weekly (52) and biweekly (26) frequencies.
//
// Ported from legacy/source/AMORTOP.pas: function GrowthPerPeriod
func GrowthPerPeriod(loan *Loan, yrinv float64) float64 {
	switch loan.PerYr {
	case 52:
		return 1 + 7*yrinv*loan.LoanRate
	case 26:
		return 1 + 14*yrinv*loan.LoanRate
	default:
		return 1 + loan.LoanRate/interest.RealPerYr(byte(loan.PerYr), 1.0/yrinv)
	}
}

// ComputeTrueRate converts the loan rate to a continuously compounded rate
// for use in daily interest calculations.
//
// Ported from legacy/source/AMORTOP.pas: procedure ComputeTrueRate
func ComputeTrueRate(loan *Loan, settings *Settings) (float64, error) {
	rr, err := interest.ReportedRate(loan.LoanRate, byte(loan.PerYr), settings.PerYr, settings.YrDays)
	if err != nil {
		return 0, err
	}
	return interest.RateFromYield(rr, settings.PerYr, settings.YrDays)
}

// PrepaidInterest computes the prepaid interest amount from loan date
// to first payment date (or one period before first payment).
//
// Ported from legacy/source/AMORTOP.pas: function PrepaidInterest
//
// The `prepaid` DOS global this function reads is FORCED on for in-advance
// loans by FirstPass (Amortize.pas:206-209: `if df.c.in_advance then prepaid :=
// true else prepaid := df.c.prepaid`), so an in-advance loan collects the
// annuity-due settlement interest regardless of the user's prepaid setting.
// 2026-07-11 audit finding 20a: the port gated on settings.Prepaid alone, so
// every in-advance loan with points solved an APR ~0.22-0.29 points low (the
// APR target is amount·(1−points) − PrepaidInterest, Amortize.pas:547).
// Verified vs the real DOS engine:
//
//	amort_oracle 100000 0.10 120 12 payhard=1500 inadv pts=0.03 apr → apr 0.141184 (Go was 0.138956)
//	amort_oracle 100000 0.10 120 12 payhard=1500 inadv b60=20000 pts=0.03 apr → apr 0.110437 (Go was 0.107513)
func PrepaidInterest(loan *Loan, settings *Settings, truerate float64) (float64, error) {
	if !settings.Prepaid && !settings.InAdvance {
		return 0, nil
	}
	if settings.InAdvance {
		ydif := dateutil.YearsDif(loan.FirstDate, loan.LoanDate, settings.Basis, settings.YrInv, true)
		return loan.Amount * loan.LoanRate * ydif, nil
	}

	t := loan.FirstDate
	var err error
	t, err = dateutil.AddPeriod(t, loan.PerYr, loan.FirstDate.Time.Day(), true)
	if err != nil {
		return 0, err
	}
	ydif := dateutil.YearsDif(t, loan.LoanDate, settings.Basis, settings.YrInv, true)

	if settings.Daily {
		expVal, err := interest.Exxp(truerate * ydif)
		if err != nil {
			return 0, err
		}
		return loan.Amount * (expVal - 1), nil
	}
	return loan.Amount * loan.LoanRate * ydif, nil
}

// settlementLinePrints mirrors the PREPAID disjunct of DOS MakeTable's
// settlement-line gate (Amortize.pas:1476):
//
//	if ((prepaid) and (PrepaidInterest>0)) or
//	   ((h^.pointsstatus>empty) and (h^.points<>0)) then
//	  begin
//	          {Line entry for prepaid interest}
//	    with Payment do
//	      begin
//	        payment.paynum := -1;
//	        interest := PrepaidInterest + h^.points*h^.amount;
//	        if hard_payment then Round2(interest); (* @round *)
//	        payamt := interest;
//	        date := h^.loandate;
//	        principal := h^.amount;
//	      end;
//	    nextpayment.date := h^.firstdate;
//	    DecideWhetherToPrintALine(h^.loandate, h^.firstdate, Output, bCommaSeperated);
//	  end;
//
// Two consequences the port must reproduce:
//
//  1. `prepaid` is FORCED true for every in-advance loan by FirstPass
//     (Amortize.pas:206-209), so the effective gate on an in-advance screen is
//     just "is the settlement interest strictly positive?".
//
//  2. The totals are accumulated INSIDE DecideWhetherToPrintALine
//     (AMORTOP.pas:1062-1077: `cumint := cumint + payment.interest;
//     cumamt := cumamt + payment.payamt`), so a suppressed settlement line is
//     excluded from DOS's grand totals as well as from the row list.
//
// PrepaidInterest is amount*loanrate*YearsDif(firstdate,loandate) on the
// in-advance arm (AMORTOP.pas:182), so it goes NEGATIVE whenever a backward
// solve returns a negative rate — and DOS then prints nothing. Repro
// (fuzzer5 seed 21001, class mor+prepay2+skip|norate), solvedrate
// -0.0058998944 ⇒ PrepaidInterest -78.47:
//
//	amort_oracle 159600.54 0.0675010000 96 12 inadv loandmy=24.12.2023 \
//	  firstdmy=24.1.2024 mor=16 pre=67:21:12:156.86 payhard=2560.52 norate
//	DOS int=-3773.82 paid=155826.72 | Go (pre-fix) int=-3852.29 paid=155748.25
//
// Callers pass the LOCALLY computed, UNROUNDED stub — not a re-call of
// PrepaidInterest — because the emission sites work on the running principal
// `p` (which differs from loan.Amount on segment sub-loans) and because DOS
// evaluates the gate before its `if hard_payment then Round2` .
//
// What this gate does NOT suppress: DOS's `repay_from := firstdate − 1 period`
// / prorate := 1 (Amortize.pas:1277-1282) and RepayFancyLoan's in-advance base
// shift (AMORTOP.pas:1100-1180) both live outside MakeTable and run regardless
// of whether the line printed. Confirmed empirically on the repro above: DOS's
// first amortizing row is still firstDate+1period = 2/24/24.
func settlementLinePrints(rawStub float64) bool { return rawStub > 0 }

// SortBalloons sorts balloon payments by date (ascending).
// Ported from legacy/source/AMORTOP.pas: procedure SortBalloons
func SortBalloons(balloons []BalloonPayment) {
	sort.Slice(balloons, func(i, j int) bool {
		return dateutil.DateComp(balloons[i].Date, balloons[j].Date) < 0
	})
}

// SortAdjustments sorts rate adjustments by date (ascending).
// Ported from legacy/source/AMORTOP.pas: procedure SortAdj
func SortAdjustments(adjustments []RateAdjustment) {
	sort.Slice(adjustments, func(i, j int) bool {
		return dateutil.DateComp(adjustments[i].Date, adjustments[j].Date) < 0
	})
}

// RepayLoan computes the remaining principal after all payments for a
// simple (non-fancy) loan using the closed-form growth formula.
//
// Ported from legacy/source/AMORTOP.pas: procedure RepayLoan
func RepayLoan(principal, payment float64, loan *Loan, settings *Settings, yrinv float64) float64 {
	f := GrowthPerPeriod(loan, yrinv)
	p := principal
	d := payment

	if settings.InAdvance {
		ff := (f - 1) / (2 - f)
		for i := 0; i < loan.NPeriods; i++ {
			p = p + ff*(p-d) - d
		}
	} else {
		// First-period prorate for the closed-form RepayLoan terminal: DOS's
		// SOLVE-side global `prorate := YearsDif(first_repay, repay_from) *
		// peryr` — the ACTUAL-day count on the active basis (Amortize.pas:1286)
		// — NOT the display schedule's month-based whole-period rule. The two
		// coincide on the 360 basis (30/360 YearsDif IS month arithmetic), but
		// on 365/365-360 a clean calendar month is 31/366·12 ≈ 1.016, and DOS's
		// plain-loan Iterate terminals (amount, rate, payment) all carry it.
		// 2026-07-11 pass-2 finding 3 — verified vs the real DOS engine:
		//
		//	amort_oracle 120000 0.11 36 12 b365 noamt pay=3929.2311 → solvedamount 120000.0012
		//	(the whole-period port solved 120017.87)
		//	amort_oracle 120000 0 36 12 b365 norate pay=3929.2311  → solvedrate 0.1100000067
		//	(and the actual-day prorate + constant-f recursion reproduces DOS's
		//	solved b365 payment 3929.2311 = annuity 3928.6461 + 24.82/42.42)
		//
		// (An earlier actual-day-always revision broke PREPAID clean-boundary
		// 365 loans; the prepaid pin below now takes precedence, which is the
		// missing piece that revision lacked — DOS pins prepaid prorate to 1
		// BEFORE this global is consulted.)
		prorate := dateutil.YearsDif(loan.FirstDate, loan.LoanDate, settings.Basis, settings.YrInv, true) * float64(loan.PerYr)
		if prorate <= 0 {
			prorate = firstPeriodProrate(loan.LoanDate, loan.FirstDate, loan.PerYr, settings)
		}
		// Prepaid pins prorate to EXACTLY 1 whenever the settlement stub is in
		// force (loan taken one period or more before the first payment):
		// DOS `if prepaid then begin repay_from := firstdate − 1 period;
		// prorate := 1; end` (Amortize.pas:1277-1282; prepaid itself is cleared
		// for a short first period, :1252-1259). This matters at the day-count
		// frequencies where a natural period is NOT 1 under firstPeriodProrate
		// (weekly: 7·52 = 364 ≠ 365.25). 2026-07-11 pass-2 finding 6 — verified
		// vs the real DOS engine:
		//
		//	amort_oracle 250000 0.145 26 26 b365 prepaid → payment 10353.4897
		//	(non-prepaid control 10353.1770 = the un-pinned port value)
		//	amort_oracle 10000 0.06 24 24 firstdmy=16.1.2024 b365 prepaid → 429.8121
		if settings.Prepaid && dateutil.DateOK(loan.LoanDate) && dateutil.DateOK(loan.FirstDate) {
			if ns, err := dateutil.AddPeriod(loan.FirstDate, loan.PerYr, loan.FirstDate.Time.Day(), true); err == nil &&
				dateutil.DateComp(loan.LoanDate, ns) <= 0 {
				prorate = 1
			}
		}
		ff := 1 + (f-1)*prorate
		p = p*ff - d // first payment
		for i := 1; i < loan.NPeriods; i++ {
			if p < 0 {
				p = p - d
			} else {
				p = p*f - d
			}
		}
	}
	return p
}

// Amortize computes the full amortization schedule for a loan.
// This is the main entry point for amortization calculations.
//
// For simple loans (non-fancy), it uses the closed-form RepayLoan.
// For fancy loans (with balloons, adjustments, prepayments, etc.),
// it uses the period-by-period RepayFancyLoan engine.
//
// Ported from legacy/source/Amortize.pas: procedure Enter + related

// coerceSubMonthlyBasis applies DOS's engine-level weekly/biweekly basis
// coercion. It must run at EVERY public entry point, because DOS has exactly one
// path — MakeTable's pre-table preprocessing coerces at Amortize.pas:297-303 and
// every solve then runs INSIDE MakeTable, downstream of it. The port split those
// solves into separate exported functions, so each needs its own call or it runs
// on an uncoerced screen. It is idempotent, so a nested call (Amortize ->
// AmortizeDOS) is a no-op.
//
// Discovered by widening the fuzzer's payment-frequency axis to DOS's full set on
// 2026-07-30: with only Amortize coercing, 29 of 35 failures on the new
// frequencies were RATE solves, because SolveRate never saw the coerced basis.
// Weekly/biweekly on a 360-day basis is coerced to 365 — a 30/360 day count
// is meaningless below a monthly period. DOS does this in the ENGINE, in the
// pre-table preprocessing every entry to MakeTable passes through
// (Amortize.pas:297-303):
//
//	if (peryr in [26, 52]) and (df.c.basis=x360) then
//	  begin
//	    MessageBox('Changing to 365 day basis for weekly/biweekly payments.', ...);
//	    df.c.basis := x365;
//	    SetYrDays;
//	    UpdateSettings;
//	  end;
//
// The port had it only at the HTTP layer (internal/api/handlers.go), so the
// shipped web path agreed with DOS but any caller building a LoanInput
// directly — internal callers, tests, and the differential fuzzers — accrued
// 30/360 where the specification accrues actual/365.25. Measured on
//
//	amort_oracle 100000 0.08 950 52 loandmy=25.12.2024 firstdmy=1.1.2025 payhard=200
//	DOS interest 89976.62 | Go 90438.60   (delta 461.98, 0.51%)
//
// and the term solve on the same screen was 3 periods out. Adding `b365`
// makes every value agree exactly, which is the proof DOS coerces internally.
//
// YrDays/YrInv are re-derived through interest.NewCalcContext, which IS DOS's
// SetYrDays (INTSUTIL.pas:333-338 — the active 3/94 variant: x365 => 365.25,
// else 360); recomputing rather than hardcoding keeps the two in step.
// input.Settings is updated too, so the schedule generators — which take
// their own copy — see the coerced basis, exactly as DOS's later readers see
// the mutated global.
//
// 2026-07-30 DetermineLastPaymentDate audit, finding B.
func coerceSubMonthlyBasis(input *LoanInput) {
	if input.Loan.PerYr != 26 && input.Loan.PerYr != 52 {
		return
	}
	if input.Settings.Basis != types.Basis360 {
		return
	}
	{
		cc := interest.NewCalcContext(types.Basis365, byte(input.Loan.PerYr))
		input.Settings.Basis = cc.Basis
		input.Settings.YrDays = cc.YrDays
		input.Settings.YrInv = cc.YrInv
	}
}

func Amortize(input LoanInput) (result AmortResult) {
	// TotalPaid is DOS's grand-total "Total payments" cell, and DOS does not
	// sum the payment column to get it — PrintGrandTotals (AMORTOP.pas:884-895)
	// builds the line from `h^.amount + int_to_date`, i.e. the loan principal
	// plus cumulative interest, unconditionally and on both the wide and narrow
	// formats. The two agree on every schedule that retires normally, which is
	// why the summed form went unnoticed; they part company as soon as the walk
	// itself is degenerate. On fuzzer5 seed 20635 DOS's own secant "converges"
	// to a payment of 3.63e21 and the payment column then runs
	// +3.63e21/-3.63e21 down the page: the port reproduced DOS's rows to the
	// cent (verified row-by-row) but its summed total collapsed to 13,575.42 —
	// the surviving points settlement — where DOS still printed 838,715.90.
	//
	// So the sum is recomputed here in DOS's own shape, at the single public
	// entry, after whichever engine ran. loan.Amount is h^.amount: nothing
	// between here and the table mutates it (a backward-solved amount is
	// written into the input before Amortize is called), and points reach the
	// total through int_to_date exactly as they do in DOS, where the settlement
	// row books points as interest (Amortize.pas:1482-1483).
	defer func() {
		if result.Err == nil {
			result.TotalPaid = input.Loan.Amount + result.TotalInt
		}
	}()

	loan := input.Loan

	// Captured for the result-sanity advisory pass (A-W7): a solved
	// unknown-prepayment amount, recorded at its solve site below.
	prepaySolvedAmt, prepaySolved := 0.0, false
	// Captured for A-W11: whether the regular payment is a hard user input
	// (vs. computed). A balloon is dropped when the payment is computed.
	payWasInput := loan.PayAmtStatus == types.InOutInput
	// TackOnFinalBalloon state (Amortize.pas:1040-1088), filled just before the
	// fancy table is generated. tackEchoIdx is the index into input.Balloons of a
	// tack-on row that stayed LIVE, so the echo loop can mark it; -1 when none.
	var tack tackOnResult
	tackEchoIdx := -1

	// Validate minimum required data
	if loan.AmountStatus < types.InOutDefault {
		result.Err = fmt.Errorf("Amount Borrowed is blank. Enter the loan principal, " +
			"or leave it blank and supply Pmt Amount, Loan Rate and # Periods for " +
			"Per%%Sense to solve the loan amount.")
		return result
	}
	if loan.PerYrStatus < types.InOutDefault {
		result.Err = fmt.Errorf("Pmts/Yr is blank. Enter how many payments are made " +
			"per year (for example 12 for monthly) so a schedule can be built.")
		return result
	}

	if !dateutil.DateOK(loan.LoanDate) {
		result.Err = fmt.Errorf("Loan Date is blank. Enter the date the loan is made " +
			"so the schedule has a starting point.")
		return result
	}

	// FirstPass: derive any of {firstDate, lastDate, nPeriods} the
	// caller left blank but can be computed from the others. Mirrors
	// DOS Amortize.pas: procedure FirstPass.
	if err := FirstPass(&loan); err != nil {
		result.Err = err
		return result
	}
	// CheckPrepayments' count-to-date arm (AMORTOP.pas:416-419). Runs here — after
	// FirstPass, before the engine dispatch and before the term solve — so both
	// engines and every downstream consumer see StopDate as the authoritative
	// series bound, which is the only bound DOS's walk has. See
	// CheckPrepaymentStops for why AddNPeriods != n x AddPeriod.
	CheckPrepaymentStops(input.Prepayments)
	// DOS's h^.lastok, captured HERE and never refreshed — FirstPass is the port
	// of Amortize.pas:220-243, the one place DOS assigns the flag, and DOS never
	// assigns it again (DetermineLastPaymentDate writes lastdate/laststatus/
	// nperiods/nstatus but leaves lastok alone). Go's loan.LastOK stops being
	// DOS's flag the moment the term solve below sets it true. See
	// LoanInput.dosLastOK.
	input.dosLastOK = loan.LastOK
	// Capture the post-FirstPass term + dates so API callers can echo
	// derived values back to the UI (e.g. Help Example 1c — supply
	// first + last dates, get nPeriods back).
	result.NPeriods = loan.NPeriods
	result.FirstDate = loan.FirstDate
	result.LastDate = loan.LastDate
	if !dateutil.DateOK(loan.FirstDate) {
		result.Err = fmt.Errorf("The first payment date could not be determined. " +
			"Fill in 1st Pmt Date, or supply Loan Date and Pmts/Yr so Per%%Sense " +
			"can default it to one period after the loan date.")
		return result
	}
	// At least two regular payments. DOS rejects a single-payment loan
	// (firstDate >= lastDate, Amortize.pas:1221-1226). A loan with exactly one
	// installment — expressed either as NPeriods == 1 or as FirstDate ==
	// LastDate — is not a valid amortization. The reversed-dates case
	// (first > last) is left to the dedicated first-after-last validation.
	// See docs/n1_minimum_term_finding.md.
	if (loan.NStatus >= types.InOutDefault && loan.NPeriods == 1) ||
		(dateutil.DateOK(loan.LastDate) && dateutil.DateComp(loan.FirstDate, loan.LastDate) == 0) {
		result.Err = fmt.Errorf("There must be at least two regular payments. " +
			"Extend the term (# Periods or Last Pmt Date) so the loan has at least " +
			"two installments.")
		return result
	}
	input.Loan = loan

	// Amortize.pas:258-271 — Enter snaps every ADJUSTMENT date ONTO the payment
	// grid, before SortAdj runs, and every later reader sees the snapped value
	// because `adj[i]^.date` is passed as NumberOfInstallments' VAR `l`:
	//
	//	for i := 1 to (nlines[AMZadjblock]) do
	//	  begin
	//	    if (adj[i]^.datestatus >= defp) and (h^.firststatus >= defp) then
	//	      begin
	//	        t := adj[i]^.date;
	//	        ta := NumberOfInstallments(h^.firstdate, adj[i]^.date, peryr, on_or_before);
	//	        if (DateComp(t, adj[i]^.date) <> 0) then
	//	          adj[i]^.datestatus := defp;
	//	              {Let user know we've adjusted rate change date to be on a payment date.}
	//	      end;
	//	    ...
	//	  end;
	//	SortAdj(nlines[AMZadjblock]);
	//
	// This is the adjustment-side twin of the moratorium snap immediately below,
	// and it had no port at all. Everything downstream reads the SNAPPED date:
	// the Re_Amortize trigger in DecideWhetherToPrintALine (AMORTOP.pas:1075) and
	// in RepayFancyLoan (AMORTOP.pas:1215), RepayFancyLoan's
	// `stopdate := adj[adjnum]^.date` (AMORTOP.pas:1140), Re_Amortize's
	// `NumberOfInstallments(adj[next_adj]^.date, h^.lastdate, ...)`
	// (AMORTOP.pas:1547) and its balloon discount
	// `YearsDif(balloon[i]^.date, adj[next_adj]^.date)` (AMORTOP.pas:1563).
	//
	// As with the moratorium, the snap is not a mere period alignment.
	// NumberOfInstallments' monthly branch ends (INTSUTIL.pas:1018) with
	//
	//	if (flast) then l.d := daysinm(l) else l.d := f.d;
	//
	// where `flast := LastDayFn(f, peryr)` is true when h^.firstdate is the LAST
	// day of its month — so a month-end first-payment date drags the adjustment's
	// day out to the end of ITS month.
	//
	// 2026-07-25 fuzzer5 seed 20214 is exactly that case. Minimal reproducer:
	//
	//	amort_oracle 321223.74 0.0832100000 240 12 loandmy=28.1.2025 \
	//	  firstdmy=28.2.2025 pre=22:12:52:58.51 adj=24:0.1287840000:3768.05
	//	→ DOS int 531767.66, Go int 532519.83 (dInt +752.17)
	//
	// firstdate = 2/28/2025 IS February's last day, so flast = true. adj=24 is
	// entered at 1/28/2027; mdiff/ddiff leave the month alone but the day is
	// stretched to daysinm(January) = 31 → 1/31/2027. The Re_Amortize trigger
	// tests the LOOKAHEAD row (`DateComp(nextt, adj[next_adj]^.date) > 0`), so
	// with a WEEKLY prepayment stream running across the boundary DOS holds the
	// old rate through the 1/30/2027 extra row and switches on 2/6/2027, while
	// the port switched a row earlier on 1/30/2027. `-mode ra` on the trace
	// oracle prints the trigger directly:
	//
	//	DW t=127-1-28 nextt=127-1-30 paynum=32 next_adj=1 adjd=127-1-31 cmp=-1
	//	DW t=127-1-30 nextt=127-2-6  paynum=33 next_adj=1 adjd=127-1-31 cmp=1
	//	RA enter next_adj=1 pdate=127-1-30 ndate=127-2-6 rate=0.0832100000
	//
	// The three necessary conditions the option sweep found all fall out of this:
	// a month-end first-payment date (loan day 15 agrees), a prepayment stream
	// fine-grained enough to place a row strictly inside (adj.Date, snapped]
	// (peryr=12 agrees — its extras land on the 28th), and a stream that actually
	// crosses the boundary (pre=22:8:52 ends first and agrees).
	//
	// Placed BEFORE ValidateInputs because DOS snaps before SortAdj, so C-A-1's
	// duplicate-date arm sees the snapped dates. Idempotent: a date already on the
	// grid re-snaps to itself.
	if loan.FirstStatus >= types.InOutDefault && dateutil.DateOK(loan.FirstDate) &&
		loan.PerYr > 0 {
		for i := range input.Adjustments {
			a := &input.Adjustments[i]
			if a.DateStatus < types.InOutDefault || !dateutil.DateOK(a.Date) {
				continue
			}
			_, snapped := dateutil.NumberOfInstallments(loan.FirstDate, a.Date,
				int(loan.PerYr), types.OnOrBefore)
			if !dateutil.DateOK(snapped) {
				continue
			}
			if dateutil.DateComp(a.Date, snapped) != 0 {
				a.Date = snapped
				a.DateStatus = types.InOutDefault
			}
		}
	}

	// Cross-field validations (DOS Amortize.pas: procedure Enter
	// preflight + SortAdj/SortBalloons error arms).
	if err := ValidateInputs(&input); err != nil {
		result.Err = err
		return result
	}
	loan = input.Loan

	// Amortize.pas:1260-1266 — TABLE_START snaps the moratorium's first-repayment
	// date ONTO the payment grid, and every downstream reader sees the snapped
	// value because `mor^.first_repay` is passed as NumberOfInstallments' VAR `l`:
	//
	//	if (mor^.first_repaystatus >= defp) then
	//	  begin
	//	    t := mor^.first_repay;              {save for comparison}
	//	    nrepay := NumberOfInstallments(h^.firstdate, mor^.first_repay,
	//	                                   h^.peryr, on_or_after);
	//	    if (DateComp(t, mor^.first_repay) <> 0) then
	//	      mor^.first_repaystatus := defp;
	//	    repay_from := mor^.first_repay;
	//	    ...
	//
	// ComputeNext's moratorium arm then tests `DateComp(date, mor^.first_repay) < 0`
	// (AMORTOP.pas:640/645) against the SNAPPED date, so the snap decides which rows
	// stay interest-only. The port previously snapped in two places — ValidateInputs
	// (for its nrepay guards) and buildDosEng — but the PIECEWISE walk read
	// input.Moratorium.FirstRepay raw, so any moratorium date off the grid produced a
	// one-row shift in where principal begins.
	//
	// The snap is not merely a period alignment. NumberOfInstallments' monthly branch
	// ends (INTSUTIL.pas:1018) with
	//
	//	if (flast) then l.d := daysinm(l) else l.d := f.d;
	//
	// where `flast := LastDayFn(f, peryr)` is true when h^.firstdate is the LAST day
	// of its month. So a month-end first-payment date makes the whole grid month-end
	// and drags first_repay's day out to the end of ITS month — which can push it
	// past the payment that shares its nominal day.
	//
	// 2026-07-25 seed 20103 is exactly that case:
	//
	//	amort_oracle 443127.98 0.1041860000 108 12 b365 inadv loandmy=28.1.2025 \
	//	  firstdmy=28.2.2025 mor=14 b33=116901.66 pre=49:2:12:478.51 \
	//	  pre=47:116:24:270.16 targ=858.10 skip=6 pts=0.031183 payhard=6029.65
	//
	// firstdate = 2/28/2025 IS February's last day, so flast = true. mor=14 gives
	// first_repay = 3/28/2026; mdiff = 0 and ddiff = 0 leave the month alone, but the
	// day is then stretched to daysinm(March) = 31 → 3/31/2026. DOS therefore keeps
	// the 3/28/2026 row interest-only and starts principal on 4/28/2026; the port
	// started it on 3/28/2026, one row early, and the shift rode all the way to the
	// terminating balloon (DOS 622554.11 vs Go 618889.20, dInt = 2412.64).
	//
	// mor=13 is the revealing control: first_repay = 2/28/2026 is itself a month end,
	// so `flast and llast` holds, the on_or_after arm declines to advance, and
	// daysinm(February 2026) = 28 leaves the date untouched — which is why 13 alone
	// out of 10..14 does not shift by a period.
	//
	// Snapping here (immediately after Enter's validations, where DOS does it) fixes
	// both engines at once. It is idempotent — a date already on the grid re-snaps to
	// itself — so buildDosEng's own call is left in place as a no-op.
	snapMoratoriumFirstRepay(&input)

	coerceSubMonthlyBasis(&input)
	settings := input.Settings
	// DOS clears prepaid outright when the loan is taken STRICTLY AFTER the
	// natural start of the first period (a SHORT first period — there is no
	// settlement span to prepay): `t := firstdate − 1 period; if DateComp(t,
	// loandate) < 0 and not in_advance then prepaid := false`
	// (Amortize.pas:1252-1259). This matters on the 360 basis with clamped
	// month-end pairs, where the natural start lands before the loan date
	// (loan Jan 31 → first Feb 29 → natural start Jan 29). 2026-07-12 pass-3
	// finding AF2 (prepaid variant) — verified vs the real DOS engine:
	//
	//	amort_oracle 50000 0.10 14 12 loandmy=31.1.2024 firstdmy=29.2.2024 prepaid
	//	→ payment 3797.6090, interest 3166.53 — identical to the non-prepaid
	//	run (Go previously solved 3798.6555 with prepaid semantics kept)
	if settings.Prepaid && !settings.InAdvance && dateutil.DateOK(loan.FirstDate) {
		if ns, nerr := dateutil.AddPeriod(loan.FirstDate, loan.PerYr,
			loan.FirstDate.Time.Day(), true); nerr == nil &&
			dateutil.DateComp(ns, loan.LoanDate) < 0 {
			settings.Prepaid = false
			input.Settings.Prepaid = false
		}
	}
	truerate, err := ComputeTrueRate(&loan, &settings)
	if err != nil {
		result.Err = fmt.Errorf("The Loan Rate could not be converted to an "+
			"internal rate (%w). Enter a Loan Rate in a normal range — for "+
			"example 6 for 6%% — and check that Pmts/Yr is set correctly.", err)
		return result
	}
	f := GrowthPerPeriod(&loan, settings.YrInv)

	// Faithful-port delegation (global-Iterate refactor M3): for the validated
	// advanced-option domain, run the structural DOS port (AmortizeDOS), which
	// matches the oracle to the cent across the whole option cube (0 divergences
	// at N=1000) where the piecewise engine below drifts on stacked options. The
	// gate keeps everything outside that domain on the piecewise engine.
	if dosPortCanHandle(input, loan, &settings) {
		pin := input
		pin.Loan = loan // post-FirstPass (term/dates derived)

		// TackOnFinalBalloon (Amortize.pas:1386-1394 → :1040-1088) runs
		// immediately BEFORE the table is generated, so it is applied here rather
		// than inside AmortizeDOS: on the merge / sub-minpmt arms DOS leaves the
		// row inside nballoons and the table is built WITH it, so the row has to
		// be spliced into the balloon set the walk sees.
		ptack := tackOnFinalBalloon(pin, &settings)
		if ptack.Fired && ptack.Live {
			pin.Balloons = append([]BalloonPayment(nil), pin.Balloons...)
			if ptack.MergeIdx >= 0 {
				pin.Balloons[ptack.MergeIdx].Amount = ptack.Amount
				pin.Balloons[ptack.MergeIdx].AmountStatus = types.InOutOutput
			} else {
				pin.Balloons = append(pin.Balloons, BalloonPayment{
					DateStatus:   types.InOutOutput,
					Date:         ptack.Date,
					AmountStatus: types.InOutOutput,
					Amount:       ptack.Amount,
				})
			}
		}

		res := AmortizeDOS(pin)
		if res.Err == nil && ptack.Fired {
			if ptack.Live {
				// AmortizeDOS's echo (dosport_entry.go) drops any row whose
				// DateStatus is below InOutDefault — and the NON-MERGE splice above
				// writes DOS's own `datestatus := outp` (Amortize.pas:1061), which in
				// the port's ordering is InOutOutput(1) < InOutDefault(2). So the
				// appended row is absent from res.Balloons and the index test below
				// silently marked nothing. Match by DATE instead, and append when the
				// echo dropped it. Same defect, same fix, as the piecewise engine's
				// echo loop — see the note there for the seed-8958 repro.
				marked := false
				for i := range res.Balloons {
					if dateutil.DateComp(res.Balloons[i].Date, ptack.Date) == 0 {
						res.Balloons[i].Amount = ptack.Amount
						res.Balloons[i].Solved = true
						res.Balloons[i].TackedOn = true
						marked = true
						break
					}
				}
				if !marked {
					res.Balloons = append(res.Balloons, ResolvedBalloon{
						Date: ptack.Date, Amount: ptack.Amount, Solved: true, TackedOn: true,
					})
				}
				if ptack.Adjusted {
					res.Warnings = append(res.Warnings,
						"Please note that the amount of your terminating balloon has been ajusted.")
				}
			} else {
				// De-activated (DOS dec(nballoons)): displayed, but excluded from
				// the payment table and the APR.
				res.Balloons = append(res.Balloons, ResolvedBalloon{
					Date: ptack.Date, Amount: ptack.Amount, Solved: true, TackedOn: true,
				})
			}
		}
		// AmortizeDOS returns before the piecewise engine's A9 APR block, so ARM
		// and stacked-option loans got NO APR (result.APR stayed 0) while DOS
		// computes one. Apply the same DOS-faithful APR pass here, using the modal
		// solved payment from the schedule.
		if res.Err == nil && loan.PointsStatus >= types.InOutDefault && len(res.Schedule) > 0 {
			pmt := payoffRegularPayment(res, loan)
			pin.Loan.PayAmt = pmt
			pin.Loan.PayAmtStatus = types.InOutInput
			applyAPR(&res, pin, loan, &settings, pmt, truerate, f)
		}
		applyPointsSettlement(&res, &loan, &settings, truerate) // audit A12 — DOS settlement line precedes both engines
		return res
	}

	// The Advanced-Options toggle as the CALLER set it — captured before the
	// internal routing forcings below (exactDaily) contaminate input.Fancy.
	// DOS's `fancy` is only this UI toggle; DetermineLastPaymentDate's
	// closed-form-vs-walk dispatch keys on it, never on exact/USA/basis
	// (AMORTOP.pas:1383-1397). 2026-07-12 pass-3 finding P3-F1.
	uiFancy := input.Fancy

	// Exact interest on a non-360 basis: DOS routes every non-360 loan through
	// the iterated RepayFancyLoan engine (Amortize.pas:1493 `… or not
	// (basis=x360)`) and, under the exact method, accrues actual-day interest
	// for which no closed form applies. Force the period-by-period (fancy)
	// engine and its schedule-oracle payment solve so the schedule and the
	// solved payment both reflect true daily accrual.
	if exactDaily(&settings) {
		input.Fancy = true
	}

	// US-Rule routing. DOS uses TWO DIFFERENT terminal conditions for USA loans,
	// and they do NOT match — reading only the schedule-display selection
	// (Amortize.pas:1493) misses this:
	//
	//   SOLVE  (Iterate, AMORTOP.pas:1438/1464): RepayFancyLoan iff
	//            `fancy OR (exact AND basis<>x360)`     — USA is NOT a trigger.
	//   DISPLAY (Amortize.pas:1493):               RepayFancyLoan iff
	//            `fancy OR (exact AND !R78) OR non-360` — USA is NOT a trigger.
	//
	// `fancy` is purely the Advanced-Options UI toggle (AmortizationScreenUnit.pas:
	// ToggleAdvanced) — it is NOT set by USA/exact/basis. USA-rule only gates the
	// `usap` accumulation INSIDE RepayFancyLoan (ComputeNext, AMORTOP.pas:656); it
	// never changes WHICH terminal is chosen. So DOS solves a USA loan's
	// payment/amount/rate with the plain RepayLoan recursion (usap has no effect on
	// the solved number) unless the loan is independently `(exact AND non-360)` or
	// carries advanced options; the usap-aware fancy walk is used only to RENDER the
	// schedule when the display condition fires.
	//
	// USA-rule loans on a non-360 (or exact) basis are rendered by DOS's usap-aware
	// RepayFancyLoan, which never compounds unpaid interest — but the SOLVE stays on
	// the plain RepayLoan terminal (USA is not an Iterate trigger). Route only the
	// DISPLAY to the fancy engine via usaFancyDisplay; the exactDaily forcing above
	// already covers exact × non-360 (where DOS's Iterate itself runs the fancy
	// terminal). 2026-07-12 pass-3 finding AF4 (supersedes the earlier note that
	// forced input.Fancy here and accepted a bounded solve envelope) — verified vs
	// the real DOS engine: the solved payment is IDENTICAL with and without `usa`
	// on every basis/flag combination probed:
	//
	//	amort_oracle 100000 0.09 120 12 usa inadv b365 → 1260.9130 (= plain inadv;
	//	  the fancy-terminal solve gave 1273.3378)
	//	amort_oracle 200000 0.09 1040 26 b365 usa → 709.6785 (= plain; Go 709.6245)
	//	amort_oracle 474551.76 0.22106832 25 26 b365 usa → 21142.6074 (fuzz cluster,
	//	  19 cases $0.006–$3.60 — same root)
	//
	// The exact term carries DOS's `and (not df.c.R78)` verbatim. R78 SUPPRESSES
	// exact in the display gate, so an R78 loan on the 360 basis takes DOS's PLAIN
	// walk — the sum-of-digits allocation at Amortize.pas:1506-1530 — no matter what
	// USA-rule says, and DOS's plain walk has no `usap` concept at all (it never
	// subtracts usap from the accruing balance, so US Rule is simply inert there).
	// Dropping the `!R78` made `usa + exact + r78` at the 360 basis render through
	// the usap-aware fancy walk, which computes day-count interest on the balance
	// instead of the R78 split and retires the loan early.
	//
	// 2026-07-25 fuzzer5 seed 9306 — verified vs the real DOS engine:
	//
	//	amort_oracle 418088.57 0.0599310000 72 4 exact prepaid inadv plusreg \
	//	  r78 usa pts=0.009556 payhard=12051.93
	//	→ DOS int=459909.45 paid=877998.02 over 72 rows, first row
	//	  int 12319.19 prin -267.26 (= r78base·72 with
	//	  r78base = (72·12051.93 − 418088.57)/(0.5·72·73) = 171.0998);
	//	  the port rendered 51 rows, int=186593.17 paid=604681.74.
	usaFancyDisplay := false
	if settings.USARule && !input.Fancy && !exactDaily(&settings) &&
		((settings.Exact && !settings.R78) || settings.Basis != types.Basis360) {
		usaFancyDisplay = true
	}

	// NOTE on in-advance × non-360 basis (docs/ui_sweep_findings.md #A): DOS
	// deliberately uses TWO engines for such a loan. The PAYMENT is solved by
	// Iterate→RepayLoan (AMORTOP.pas:1437 selects RepayLoan when the loan is
	// neither fancy nor exact), the SIMPLE annuity-due recursion — basis-
	// INDEPENDENT (DOS solves the same payment on 360/365/365-360). The SCHEDULE
	// is then rendered by RepayFancyLoan (AMORTOP.pas:1493 routes every non-360
	// loan there), with basis-dependent actual-day accrual. We reproduce that
	// split below: the payment stays on the simple (non-fancy) solve, and only the
	// schedule DISPLAY is redirected to generateFancySchedule for non-360. So we do
	// NOT force input.Fancy here (that would move the SOLVE onto the fancy schedule
	// and mis-price the payment).

	// Whether the loan term was known on INPUT (before the A6 solve below).
	// DOS's MakeTable dispatch is an else-if chain in which
	// DetermineLastPaymentDate (solve the term) sits AHEAD of the unkpre
	// prepayment-duration branch (AMORTIZE.pas:1350-1367): when the term is
	// being derived, the prepayment duration is NOT separately solved — the
	// prepayment simply runs until the loan retires. AO10 below therefore fires
	// only when the term was already known here.
	termKnownOnInput := loan.NStatus >= types.InOutDefault || loan.LastStatus >= types.InOutDefault

	// A6 (DetermineLastPaymentDate, AMORTOP.pas:1323-1407): when the
	// caller supplied a payment but neither the term nor a last date,
	// derive the number of periods closed-form from the payment.
	if loan.NStatus < types.InOutDefault && loan.LastStatus < types.InOutDefault &&
		loan.PayAmtStatus >= types.InOutDefault && loan.PayAmt > 0 &&
		loan.LoanRateStatus >= types.InOutDefault && dateutil.DateOK(loan.FirstDate) {
		// A term solve with an INCOMPLETE rate-adjustment row aborts in DOS:
		// SufficientDataOnScreen requires `nadj = 0 or adj_fully_specified`
		// (Amortize.pas:884-887), so the screen never reaches the solve.
		//
		// A FULLY specified ARM used to be refused here too, on the strength of
		// 2026-07-12 pass-3 finding P3-F3:
		//
		//	amort_oracle 100000 0.08 0 12 adj=24:0.09:1100 payhard=1050 noterm
		//	→ ERR "Payment amount is too small to compute number of periods."
		//
		// That refusal was an ORACLE ARTIFACT, retired 2026-07-29. The oracle then
		// carried df.c.centurydiv = 20 instead of the shipped 50 (PEDATA.pas:67),
		// and centurydiv's one computational use is the wall RepayFancyLoan puts
		// on its probe walk (AMORTOP.pas:1143-1147, `stopdate.y := 100 +
		// pred(df.c.centurydiv)`) — Pascal year 119, i.e. calendar 2019, which is
		// BEFORE the loan even starts. Every term solve that needed more than a
		// handful of periods hit the wall unretired and took the ABORT at
		// AMORTOP.pas:1344-1345, and the ones with adjustments simply need more
		// periods than most. With centurydiv restored to 50 the wall moves to 2049
		// and the same line answers `solvedterm 152 last 2036-9-1`; the widened
		// backward-solve fuzzer (seed 21000) then caught the port refusing
		//
		//	amort_oracle 136802.76 0.0838450000 17 1 b365 prepaid plusreg r78 usa \
		//	  loandmy=25.7.2023 firstdmy=25.7.2024 mor=84 b96=38861.78 \
		//	  b108=7092.32 b144=23099.45 adj=60:0.0306990000:20796.04 targ=1965.83 \
		//	  payhard=17428.90 noterm
		//
		// where DOS answers `solvedterm 11 last 2034-7-25`. Nothing in
		// DetermineLastPaymentDate special-cases nadj: the fancy branch runs the
		// same RepayFancyLoan walk, and Re_Amortize fires inside it on the
		// `entire` arm of AMORTOP.pas:1216, so a fully specified ARM is walked
		// exactly like any other advanced option. The walk below now handles it.
		if adjRowsNotFullySpecified(input.Adjustments) {
			result.Err = fmt.Errorf("The number of periods cannot be solved " +
				"while a Rate Adjustment row is incomplete. Fill in the date, " +
				"rate AND payment on every Adjustment row, or enter # Periods " +
				"directly.")
			return result
		}
		// Dispatch on the CALLER's Advanced-Options toggle (uiFancy), not the
		// internally-forced input.Fancy: DOS's DetermineLastPaymentDate takes
		// the closed-form log branch for ANY loan without advanced options —
		// exact and USA are not triggers (AMORTOP.pas:1383-1397). The closed
		// form and the walk can report n one apart at retire-exactly
		// boundaries, and DOS's REPORTED term comes from the closed form (it
		// can even report n=25 while rendering 24 rows). 2026-07-12 pass-3
		// finding P3-F1 — verified vs the real DOS engine:
		//
		//	amort_oracle 10000 0.09 0 12 b365 usa payhard=456.90 noterm
		//	→ solvedterm 25 last 2026-2-1 (rendering 24 rows; Go reported 24)
		//	amort_oracle 10000 0.09 0 4 b365 exact payhard=1379.62 noterm
		//	→ solvedterm 9 last 2026-4-1 (Go's fancy walk said 8)
		//	amort_oracle 10000 0.09 0 4 b365 usa payhard=1379.68 noterm
		//	→ solvedterm 8 last 2026-1-1, interest 1038.87 (Go said 9 / 1038.90)
		if uiFancy {
			// Fancy mode: balloons/prepayments/adjustments make the
			// closed form inapplicable — run the schedule unbounded
			// and observe when the loan retires.
			n, last, pres, err := solveFancyTermFromPayment(input)
			if err != nil {
				result.Err = err
				return result
			}
			// DOS re-stamps every DERIVED prepayment window from the probe
			// walk's series cursor before it renders the table — see
			// rewritePrepayWindowsAfterTermSolve.
			if pres != nil {
				input.Prepayments = pres
			}
			loan.NPeriods = n
			loan.NStatus = types.InOutOutput
			loan.LastDate = last
			loan.LastStatus = types.InOutOutput
			loan.LastOK = true
			input.Loan = loan
		} else {
			n, err := solveNPeriodsFromPayment(&loan, &settings, f)
			if err != nil {
				result.Err = err
				return result
			}
			loan.NPeriods = n
			loan.NStatus = types.InOutOutput
			if last, err := dateutil.AddNPeriods(loan.FirstDate, loan.PerYr, n-1); err == nil {
				loan.LastDate = last
				loan.LastStatus = types.InOutOutput
				loan.LastOK = true
			}
			input.Loan = loan
		}
	}

	// Default payment amount if not specified
	d := loan.PayAmt
	if loan.PayAmtStatus < types.InOutDefault {
		// Estimate payment
		if loan.LoanRateStatus >= types.InOutDefault && loan.NPeriods > 0 {
			d = estimatePayment(&loan, f)
			// First-period proration of the PAYMENT solve. DOS prorates the first
			// period by the ACTUAL-day fraction regardless of basis or the exact
			// flag — `prorate := YearsDif(first_repay, repay_from) * peryr`
			// (Amortize.pas:1286, where repay_from = loanDate when prepaid is OFF).
			// So the 365 and 365/360 bases shift the payment even on a clean
			// monthly boundary (e.g. 31/365.25·12 = 1.0185 of a period, not 1),
			// matching DOS's higher 365 payment; on the 360 basis a clean month is
			// exactly one period, so 360 loans are unchanged. In prepaid mode the
			// odd interest is taken at settlement and the first period is a whole
			// period (prorate = 1), so the payment is NOT augmented.
			//
			// NOTE the deliberate DOS asymmetry: the SCHEDULE display still accrues
			// whole-month interest for non-exact loans (firstPeriodProrate /
			// DaysCloseEnough), so this higher payment slightly over-amortizes and
			// the final row's payment shrinks — exactly as the DOS engine renders
			// it. Exact loans solve via dosIteratePayment instead (below).
			// The first-period proration matches DOS's ARREARS branch of RepayLoan
			// (AMORTOP.pas:1284 `ff := 1 + (f-1)*prorate`). DOS's IN-ADVANCE branch
			// (AMORTOP.pas:1276-1279) is a flat annuity-due recursion with NO
			// proration, so the in-advance payment is basis-independent — DOS solves
			// the same payment on 360/365/365-360. Gating on !InAdvance reproduces
			// that; without it a non-360 in-advance payment was mis-prorated (~1.7%,
			// docs/ui_sweep_findings.md #A). On the 360 basis the proration is 1.0
			// anyway (clean month = one period), so 360 in-advance was already right.
			if !settings.Prepaid && !settings.InAdvance && !exactDaily(&settings) && !hasAnyAdvancedOption(input) && math.Abs(f-1) > teeny &&
				dateutil.DateOK(loan.LoanDate) && dateutil.DateOK(loan.FirstDate) {
				ydif := dateutil.YearsDif(loan.FirstDate, loan.LoanDate, settings.Basis, settings.YrInv, true)
				if prorate := ydif * float64(loan.PerYr); prorate > 0 &&
					math.Abs(prorate-1) > teeny {
					ffFirst := 1 + (f-1)*prorate
					d *= ffFirst / f
				}
			}
			// DOS seed parity: EstimateAndRefinePayment (Amortize.pas:384-401) subtracts
			// the present value of every balloon and prepayment from the principal BEFORE
			// the annuity, so the regular payment only retires the remainder. Scaling the
			// annuity by adjp/amount reproduces that adjusted seed, so the shared Newton
			// (dosIteratePayment) starts on DOS's answer rather than a spurious near-zero
			// of the ill-conditioned prepayment-replace terminal.
			//
			// The seed is not cosmetic. DOS's Iterate is a SECANT (AMORTOP.pas:1415-1495)
			// with no bracketing, so on a non-monotone terminal it lands on whichever
			// root its seed walks it to — and the option terminals routinely have two.
			// Reproducing DOS's schedule therefore means reproducing DOS's SEED to the
			// formula, which is `adjp * (f-1) / (1 - exxp(-nrepay * lnn(f)))` over the
			// AMORTIZING count nrepay, with every present value discounted back to
			// repay_from. See dosNrepay / dosRepayFrom.
			//
			// The seed is formed in DOS's OWN SHAPE — one additive adjp, then one
			// quotient over the AMORTIZING count nrepay — rather than reconstructed
			// multiplicatively from the plain annuity. The two are algebraically
			// identical and differ by ~2 ULP, and on the terminal's flat plateaus 2
			// ULP decides which root the bracket-free secant walks to. See
			// dosSeedPayment for the seed-20622 measurement.
			//
			// Scope: exactly the cases the multiplicative reconstruction used to
			// cover — a balloon or prepayment series to discount, or an amortizing
			// window shorter than the term. All of them imply hasAnyAdvancedOption,
			// so the odd-first proration above (gated on !hasAnyAdvancedOption) has
			// not fired and there is nothing on `d` to preserve. With neither
			// condition DOS's formula degenerates to adjp = amount and nrepay =
			// NPeriods, i.e. estimatePayment's own expression operation for
			// operation, so leaving `d` alone there is not an approximation.
			if (len(input.Balloons) > 0 || len(input.Prepayments) > 0 ||
				dosNrepay(input, &loan) != loan.NPeriods) && math.Abs(loan.Amount) > tiny {
				if seed, ok := dosSeedPayment(input, &loan, &settings, f); ok {
					d = seed
				}
			}
		}
	}

	// Snapshot the post-FirstPass term + dates. The schedule
	// generators below replace `result` wholesale, and they take
	// `&loan` so could in principle mutate it — keep our own copy of
	// what the engine derived so we can echo it back regardless.
	derivedNPeriods := loan.NPeriods
	derivedFirstDate := loan.FirstDate
	derivedLastDate := loan.LastDate

	if !input.Fancy {
		// Blank payment: the closed-form estimate (the ordinary in-arrears
		// annuity, even with the prepaid-OFF odd-first augmentation above) does
		// not match DOS for an odd first period OR for in-advance (annuity-due)
		// loans. DOS's closed-form shortcut applies only to the plain
		// 360 / prepaid / in-arrears / natural-first case and ITERATES otherwise
		// (Amortize.pas:402-416 — the shortcut condition explicitly requires
		// `not in_advance`). Mirror that: refine the estimate with the
		// schedule-oracle bisection, which converges to the level payment that
		// drives the UNFORCED terminal balance of the real forward schedule
		// (which already models in-advance interest timing) to zero. The snap
		// guard keeps an already-exact estimate untouched (no sub-cent bisection
		// noise) and only adopts a materially different refined payment.
		// DOS runs Iterate for a blank-payment plain loan (AMORTOP.pas:1437,
		// RepayLoan branch) EXCEPT the closed-form shortcut, which requires
		// `(not exact) and prepaid and (not in_advance)` (Amortize.pas:402-408).
		// So DOS iterates whenever exact OR not-prepaid OR in-advance — the
		// scope below (bounded to the RepayLoan terminal, i.e. non-exact-daily).
		// dosIterateSimplePayment IS that same double-precision Iterate over
		// RepayLoan, seeded from DOS's annuity estimate `d`, so it reproduces the
		// DOS engine both when it converges AND when it refuses.
		if loan.PayAmtStatus < types.InOutDefault &&
			loan.LoanRateStatus >= types.InOutDefault && loan.NPeriods > 0 &&
			!exactDaily(&settings) &&
			(settings.InAdvance || !settings.Prepaid || settings.Exact) {
			refIn := input
			refIn.Loan = loan
			// Seed from DOS's OWN estimate — the raw annuity `adjp·(f-1)/denom`
			// EstimateAndRefinePayment hands to Iterate (Amortize.pas:397-401),
			// NOT the odd-first-augmented `d`. The augmentation nudges `d` onto the
			// knife-edge root, which would let Iterate's seed-is-already-converged
			// fast path (|terminal| < half-penny) accept an ill-conditioned loan
			// DOS refuses. Seeding from the raw annuity reproduces DOS's secant
			// PATH, hence its convergence verdict, exactly.
			refined, ok := dosIterateSimplePayment(refIn, estimatePayment(&loan, f))
			if !ok {
				// DOS-fidelity refusal: on an ill-conditioned high-rate/long-term
				// loan the schedule's terminal balance is so steep in the payment
				// that Iterate exhausts its 20-step budget above tolerance, and DOS
				// blocks with this exact message and NO schedule (AMORTOP.pas:1489).
				// The port previously skipped Iterate here (needPaymentRefine was
				// false for the arrears natural-first case DOS still iterates) and
				// returned the knife-edge annuity payment, which rendered a
				// degenerate schedule (negative amortization + a terminating balloon
				// many times the principal). 2026-07-16 fuzzer3 finding; oracle-
				// validated boundary (500000 0.28 120 4 → payment; 200 4 → refuse).
				result.Err = fmt.Errorf(
					"Computation of payment amount or interest rate did not converge.")
				return result
			}
			// Adopt the refined payment only where the closed-form estimate is
			// inexact (in-advance / odd first period) — the pre-existing
			// refinement scope. RepayLoan's in-advance branch is the annuity-due
			// recursion (basis-INDEPENDENT); odd-first arrears loans use its
			// prorated first period. The snap guard keeps an already-exact estimate
			// untouched (no sub-cent noise); the natural-first arrears case keeps
			// its closed-form d (only the convergence check above applies to it).
			if refined > 0 && math.Abs(refined-d) > 1e-3 && needPaymentRefine(&loan, &settings) {
				d = refined
			}
		}
		// Schedule DISPLAY. DOS renders some loans with RepayFancyLoan even though
		// the payment above was solved by the simple RepayLoan model (Amortize.pas:
		// 1493 vs the Iterate terminal at AMORTOP.pas:1438). Mirror that split by
		// redirecting ONLY the display to the fancy engine:
		//   - in-advance × non-360: actual-day accrual on the settlement-shifted
		//     schedule (docs/ui_sweep_findings.md #A);
		//   - USA-rule × non-360 (arrears): the usap-aware walk that never compounds
		//     unpaid interest (usaFancyDisplay, computed above). The payment was
		//     solved by RepayLoan (usap has no effect on it), so only the rows differ.
		//   - Exact (arrears) at the 360 basis: Amortize.pas:1493's exact term has
		//     no basis guard, so DOS walks dates for the ROWS while the payment
		//     stays on RepayLoan. Inert on whole-month grids, live at peryr=24.
		//     See the §56 arm below.
		if (settings.InAdvance && settings.Basis != types.Basis360 && !exactDaily(&settings)) ||
			usaFancyDisplay {
			dispInput := input
			dispInput.Loan = loan
			result = generateFancySchedule(dispInput, d, &settings, truerate, f)
		} else if settings.Exact && settings.InAdvance && !settings.R78 {
			// Exact × in-advance at the 360 basis: DOS's DISPLAY routing
			// `(exact and not R78) or non-360` (Amortize.pas:1493) renders the
			// settlement-row + one-period base-shift shape even where exact
			// day-counting is inert — while the PAYMENT stays the plain
			// annuity-due RepayLoan solve (the Iterate gate `fancy or (exact
			// and non-360)`, AMORTOP.pas:1438, does not fire at 360).
			// 2026-07-11 pass-2 finding 5 — verified vs the real DOS engine:
			//
			//	amort_oracle 104844 0.0593783730 36 12 inadv exact dumpraw
			//	→ payment 3172.2326 (= exact-OFF), rows at 3/1, 4/1, …,
			//	  interest 10421.61 (the port rendered the unshifted 9875.16)
			//
			// (Non-360 exact in-advance reaches the same generator via the
			// exactDaily fancy routing; this arm covers 360 display-only.)
			//
			// R78 SUPPRESSES exact in the display gate — `(exact and not R78)`
			// — so an R78 loan uses the plain in-advance R78 schedule (the
			// sum-of-digits allocation whose total is n·d − principal),
			// never the exact settlement-shifted shape. 2026-07-13 pass-4
			// finding — verified vs the real DOS engine:
			//
			//	amort_oracle 100000 0.1173 48 24 r78 exact inadv → interest
			//	11944.77 (= the r78+inadv value = n·d − principal; the exact
			//	arm rendered 12477.59, adding a spurious exact settlement)
			ein := input
			ein.Loan = loan
			result = generateExactInAdvanceSchedule(ein, d, &settings)
		} else if settings.Exact && !settings.R78 && !wholeMonthGrid(loan.PerYr) &&
			!input.inBackwardSolve {
			// Exact × ARREARS at the 360 basis — discrepancies §56.
			//
			// Amortize.pas:1493 carries NO basis guard on its exact term:
			//
			//	if (fancy) or ((df.c.exact) and (not df.c.R78))
			//	           or (not (df.c.basis=x360)) then RepayFancyLoan
			//
			// so at the 360 basis DOS SOLVES the payment with the nominal
			// RepayLoan (the Iterate gate AMORTOP.pas:1438 IS basis-guarded and
			// does not fire) and then RENDERS the schedule with the date-walking
			// RepayFancyLoan. The port's exactDaily() helper collapses every
			// reader of `exact` to `Exact && Basis != 360`, which is right for
			// the four basis-guarded sites but wrong for this one, so display
			// stayed on generateSimpleSchedule.
			//
			// It survived because on any grid where the period is a whole number
			// of months the two engines agree ALGEBRAICALLY, not by accident:
			// AddPeriod forces d := orig_day every period (INTSUTIL.pas:1240), so
			// DaysCloseEnough is always true, so ComputeNext takes the nominal
			// branch (AMORTOP.pas:627) and timedif = Δm/12 = 1/peryr — exactly
			// RepayLoan's f-1 = loanrate/RealPerYr(peryr). peryr 26 and 52 reach
			// the fancy generator anyway, because coerceSubMonthlyBasis rewrites
			// their basis to 365 upstream and exactDaily then fires.
			//
			// That leaves peryr=24 as the ONLY frequency where this gate is
			// observable at a genuine 360 basis, and it is: the semi-monthly
			// AddPeriod branch (INTSUTIL.pas:1217-1238) walks d±15 rather than
			// pinning the anchor, so DaysCloseEnough fails and timedif comes from
			// YearsDif's 30/360 rules — which are NOT 15/360 across February.
			// Verified vs the real DOS engine:
			//
			//	amort_oracle 40606.39 0.094051 600 24 \
			//	  loandmy=29.6.2021 firstdmy=29.7.2021 exact dumpraw
			//	→ the 29th/14th grid; every row is 15/360 EXCEPT
			//	  2/14/22 -> 2/28/22 (clamped, 14/360, interest 148.21) and
			//	  2/28/22 -> 3/14/22 (INTSUTIL.pas:800's `(a.m=2) and (a.d>27)`
			//	  correction, 14/360, interest 148.11).
			//	  DOS interest 63873.37 / paid 104479.76 over 595 rows;
			//	  the port rendered 65385.02 / 105991.41 over 602 — i.e. exactly
			//	  its own exact-OFF answer, seven rows long.
			//
			// The redirect is DISPLAY-ONLY, matching the in-advance arm above and
			// NOT the 2026-07-25 seed-20110 finding, which was about routing the
			// whole exact×360 COMPUTATION to the piecewise engine (dosport_entry.go
			// :451).
			//
			// WHY THE wholeMonthGrid() TERM IS HERE, AND WHY IT IS NOT A HACK.
			// The literal gate — `exact and not R78`, with no frequency test, as
			// DOS writes it — was implemented first and it REGRESSED round 12's
			// zzyear_byte_horizon_test.go case E, a 1080-period MONTHLY exact loan
			// running to 2119: interest 176010.29 against DOS's 175844.60, one row
			// short. That is START_HERE §5's first trap. On a whole-month grid the
			// two DOS engines are identical ALGEBRAICALLY (the AddPeriod /
			// DaysCloseEnough / timedif = Δm/12 chain above), so DOS cannot tell
			// which one ran; the port CAN, because §54 is deferred and its walk
			// dates drift a month at February 2100 while DOS's plain mod-4 leap
			// rule does not. Sending a grid DOS proves inert through the port's
			// date walk therefore imports a §54 artifact and nothing else.
			//
			// So the narrowing loses no DOS behaviour: it declines the redirect
			// exactly where DOS's own two engines provably agree. It is scoped to
			// the identity, not to the symptom — case D below is peryr=24 on the
			// protected 15th anchor, where DaysCloseEnough DOES hold, and it still
			// goes through the walk and still matches. If §54 is ever closed, this
			// term can be dropped and the gate made literal again.
			//
			// Measured inert on every whole-month grid regardless: peryr
			// 1/2/4/6/12 × four anchor days are bit-identical through this branch.
			//
			// WHY inBackwardSolve IS IN THE GATE. Amortize.pas:1493 is MakeTable's
			// DISPLAY dispatch — it is not a solver terminal. DOS's backward
			// solves run Iterate, whose terminal (AMORTOP.pas:1438/:1464) IS
			// basis-guarded and therefore uses the nominal RepayLoan at x360;
			// the exact table is rendered only afterwards, from the solved
			// answer. The port reaches its solvers' trial evaluations through
			// this same Amortize, so without the term a rate/amount solve would
			// bisect on a residual DOS never computes. It cost a real regression
			// to learn: paired_regression 44000-44039 returned NEW=1 on
			//
			//	amort_oracle 291207.99 0.1209560000 2688 24 exact prepaid \
			//	  loandmy=29.5.2024 firstdmy=29.7.2024 pts=0.009110 \
			//	  payhard=1962.94 norate bdump
			//
			// a semi-monthly exact RATE solve, where the redirect moved the
			// solved schedule by 3038.69. This is the same rule as
			// dosPortCanHandle's inBackwardSolve check (dosport_entry.go:428)
			// and it is set only by the solvers, so the OUTER Amortize that
			// renders the final table still takes the arm.
			dispInput := input
			dispInput.Loan = loan
			result = generateFancySchedule(dispInput, d, &settings, truerate, f)
		} else {
			// Simple amortization: generate schedule period by period
			result = generateSimpleSchedule(&loan, d, &settings, truerate, f)
		}
	} else {
		// Fancy amortization with full feature set
		SortBalloons(input.Balloons)
		SortAdjustments(input.Adjustments)

		// AO2 (EstimateAndRefineBalloon, Amortize.pas:628-663): a
		// balloon with a date but no amount is a "target balloon" —
		// solve the amount that drives the schedule's final balance
		// to zero.
		unknownBalloon := -1
		for i := range input.Balloons {
			if input.Balloons[i].DateStatus >= types.InOutDefault &&
				input.Balloons[i].AmountStatus < types.InOutDefault {
				if unknownBalloon >= 0 {
					result.Err = fmt.Errorf(
						"More than one Balloon has a date but no amount. " +
							"Per%%Sense can solve only one unknown Balloon amount at a " +
							"time — fill in an amount for all but one of the Balloon rows.")
					return result
				}
				unknownBalloon = i
			}
		}
		if unknownBalloon >= 0 {
			// DOS requires a KNOWN payment before it will solve an unknown
			// balloon: SufficientDataOnScreen (Amortize.pas:889-891) admits an
			// `unkballoon > 0` screen only when `payamtstatus >= defp` (and the
			// rate is known), otherwise the table is refused with "not enough
			// data". 2026-07-11 adjudication of TestAPIAmortBalloonAmountEchoed:
			// with BOTH the payment and the balloon amount blank, the port
			// previously let the balloon claim the solve with an implicit $0
			// payment (solving a nonsensical 120266.39 balloon on a plain
			// self-amortizing loan); the old test expectation (~0) was equally
			// non-DOS. Verified vs the real DOS engine:
			//
			//	amort_oracle 100000 0.06 120 12 dateballoon=37
			//	→ NO schedule, payment unsolved, balloon status stays empty
			//	amort_oracle 100000 0.06 120 12 payhard=1110.21 solveballoon=37
			//	→ balloon 1109.6700 (payment known → balloon solves; it lands ON
			//	  a payment date in REPLACE mode, so it covers the payment it
			//	  replaces — NOT ~0)
			if loan.PayAmtStatus < types.InOutDefault {
				result.Err = fmt.Errorf("There is not enough data to compute the " +
					"table: a Balloon has a date but no amount AND Pmt Amount is " +
					"blank. Enter the Pmt Amount so the Balloon amount can be " +
					"solved, or fill in the Balloon amount and leave Pmt Amount " +
					"blank instead.")
				return result
			}
			amt, err := SolveBalloonAmount(input, unknownBalloon)
			if err != nil {
				result.Err = fmt.Errorf("The Balloon amount could not be solved: %w. "+
					"Check the Balloon Date and the loan terms, or enter the Balloon "+
					"amount directly.", err)
				return result
			}
			input.Balloons[unknownBalloon].Amount = amt
			input.Balloons[unknownBalloon].AmountStatus = types.InOutOutput
		}

		// AO9 (EstimateAndRefinePeriodicPrepayment, Amortize.pas:665):
		// a prepayment series with a start date but no amount is an
		// "unknown prepayment" — solve the per-payment amount that
		// drives the schedule's final balance to zero.
		unknownPrepay := -1
		for i := range input.Prepayments {
			if input.Prepayments[i].StartDateStatus >= types.InOutDefault &&
				input.Prepayments[i].PaymentStatus < types.InOutDefault {
				if unknownPrepay >= 0 {
					result.Err = fmt.Errorf(
						"More than one Prepayment has a start date but no amount. " +
							"Per%%Sense can solve only one unknown Prepayment amount at a " +
							"time — fill in an amount for all but one of the Prepayment rows.")
					return result
				}
				unknownPrepay = i
			}
		}
		if unknownPrepay >= 0 {
			// Same DOS sufficiency rule as the unknown balloon above: an unknown
			// prepayment is only solvable when the payment is KNOWN
			// (SufficientDataOnScreen, Amortize.pas:893-895 — `(unkpre > 0) and
			// … (payamtstatus >= defp)`). Verified vs the real DOS engine:
			//
			//	amort_oracle 100000 0.06 120 12 presolve=6:12:12              → prepay 0.0000 (unsolved)
			//	amort_oracle 100000 0.06 120 12 payhard=800 presolve=6:12:12 → prepay 3265.5268
			if loan.PayAmtStatus < types.InOutDefault {
				result.Err = fmt.Errorf("There is not enough data to compute the " +
					"table: a Prepayment has a start date but no amount AND Pmt " +
					"Amount is blank. Enter the Pmt Amount so the Prepayment amount " +
					"can be solved, or fill in the Prepayment amount and leave Pmt " +
					"Amount blank instead.")
				return result
			}
			amt, err := SolvePrepaymentAmount(input, unknownPrepay)
			if err != nil {
				result.Err = fmt.Errorf("The Prepayment amount could not be solved: %w. "+
					"Give the Prepayment a stop date or payment count so the solve is "+
					"bounded, or enter the Prepayment amount directly.", err)
				return result
			}
			input.Prepayments[unknownPrepay].Payment = amt
			prepaySolvedAmt, prepaySolved = amt, true
			// Mark the solved amount as a known input so the schedule
			// engine applies it (the prepayment loop skips a series
			// whose PaymentStatus is below InOutDefault).
			input.Prepayments[unknownPrepay].PaymentStatus = types.InOutInput
		}

		// AO10 (DeterminePrepaymentDuration, Amortize.pas:709-774): a
		// prepayment series with a known amount but no stop date and
		// no payment count — solve how long it must run to retire the
		// loan, then pin NN and the stop date.
		for i := range input.Prepayments {
			pp := &input.Prepayments[i]
			if termKnownOnInput &&
				pp.StartDateStatus >= types.InOutDefault &&
				pp.PaymentStatus >= types.InOutDefault &&
				pp.StopDateStatus < types.InOutDefault &&
				pp.NNStatus < types.InOutDefault {
				nn, stop, err := SolvePrepaymentDuration(input, i)
				if err != nil {
					result.Err = fmt.Errorf(
						"The Prepayment duration could not be solved: %w. Check the "+
							"Prepayment amount and start date, or supply a stop date or "+
							"payment count directly.", err)
					return result
				}
				input.Prepayments[i].NN = nn
				input.Prepayments[i].NNStatus = types.InOutInput
				input.Prepayments[i].StopDate = stop
				input.Prepayments[i].StopDateStatus = types.InOutInput
			}
		}

		// Dav Holle provision (Amortize.pas:1430-1434): when the
		// regular payment is a user-input "hard" number, balloon
		// amounts and adjusted payment amounts are hardened to whole
		// cents so the schedule uses the standard penny treatment.
		if loan.PayAmtStatus == types.InOutInput {
			for i := range input.Balloons {
				input.Balloons[i].Amount = interest.Round2(input.Balloons[i].Amount)
			}
			for i := range input.Adjustments {
				input.Adjustments[i].Amount = interest.Round2(input.Adjustments[i].Amount)
			}
		}

		// When the regular payment was left blank, solve it so the schedule
		// amortizes over the stated term WITH the fancy features active. The
		// closed-form estimatePayment ignores balloons, targets, etc., which
		// left the loan under/over-amortized: a known balloon didn't reduce the
		// payment (it retired early), and a principal-minimum target paid the
		// loan off before the term. Mirrors DOS Amortize.pas' EstimateAndRefine
		// payment-iteration when fancy options are active.
		//
		// Skip this when an unknown balloon/prepayment was just solved — in that
		// field-presence dispatch the balloon/prepayment is the unknown and the
		// (estimated) payment is the known. Skip-months keep their existing,
		// well-tested refinement; everything else (known balloon, target,
		// rate/payment adjustments, known prepayment) uses DOS's Newton Iterate
		// over the unforced fancy terminal (dosIteratePayment).
		solvedUnknown := unknownBalloon >= 0 || unknownPrepay >= 0
		skipActive := input.SkipMonths.SkipStatus >= types.InOutDefault &&
			anySkip(input.SkipMonths.MonthSet)
		// A known balloon should REDUCE the regular payment so principal + balloon
		// amortize over the term; a principal-minimum (target) should be solved so
		// the schedule still retires exactly at the term. Rate/payment adjustments
		// re-amortize their own payment, and prepayments are extra payments meant
		// to shorten the term — neither should have the regular payment globally
		// re-solved, so they're excluded here.
		hasKnownBalloon := false
		for i := range input.Balloons {
			// Presence by STATUS: a $0 balloon in REPLACE mode is a real skipped
			// installment the payment solve must account for (pass-2 finding 9;
			// oracle: `10000 0.12 24 12 b12=0` → payment 491.2571, not the plain
			// 470.7347).
			if input.Balloons[i].AmountStatus >= types.InOutDefault {
				hasKnownBalloon = true
				break
			}
		}
		targetActive := input.Target.TargetStatus >= types.InOutDefault
		hasPrepay := false
		for i := range input.Prepayments {
			if input.Prepayments[i].PaymentStatus >= types.InOutDefault {
				hasPrepay = true
				break
			}
		}
		// Blank-payment dispatch guard. Only solve the regular payment when it was
		// left blank (PayAmtStatus < InOutDefault) AND no unknown balloon/prepayment
		// already claimed this row as the field to solve (!solvedUnknown — those are
		// mutually exclusive dispatch outcomes) AND the rate and term are known so a
		// payment CAN be solved. The else-if chain below then picks the refinement
		// path by which fancy options are active (see each branch's comment).
		if loan.PayAmtStatus < types.InOutDefault && !solvedUnknown &&
			loan.LoanRateStatus >= types.InOutDefault && loan.NPeriods > 0 {
			if len(input.Adjustments) > 0 && (hasKnownBalloon || hasPrepay || skipActive || targetActive) {
				// Rate adjustment + a downstream option (balloon / prepayment /
				// skipped months / target): DOS solves the INITIAL payment at the
				// ORIGINAL rate accounting for that option, ignoring the
				// adjustment (which re-amortizes its OWN payment at the adjustment
				// date — the AO5 path in generateFancySchedule, now refined for
				// each of these options). Solving the base payment with the
				// adjustment present is ill-posed: the re-amortization absorbs the
				// balance, so the terminal is insensitive to the base payment and
				// the bisection cannot bracket — leaving the plain closed-form seed
				// (which ignores the option). Strip the adjustments for the
				// base-payment solve so it accounts for the option at the original
				// rate. This must run BEFORE the skip-only branch, or an ARM+skip
				// loan would solve its base payment with the adjustment present.
				stripped := input
				stripped.Adjustments = nil
				// `refined != 0`, not `> 0`: DOS has no positivity check on the solved
				// payment (Amortize.pas:416-421 stores whatever AMORTOP.pas:1415-1495
				// Iterate returns) — see the hasPrepay arm below for the regime where
				// the DOS-faithful root is negative.
				refined, ok := dosIteratePayment(stripped, d)
				if ok && refined != 0 {
					d = refined
				} else if !ok {
					// DOS-FAITHFUL FAILURE PROPAGATION for the BASE-PAYMENT solve,
					// the sibling of the AO5 segment-payment propagation in
					// fancybisect.go/solveSegmentPayment. EstimateAndRefinePayment is
					//
					//	if Iterate(p, usap, h^.loandate, h^.firstdate, d, til_adj) then
					//	  h^.payamt := d
					//	else
					//	  begin
					//	    errorflag := true;
					//	    EstimateAndRefinePayment := false;
					//	  end;            {Amortize.pas:416-426}
					//
					// and `errorflag` is the engine-wide condemnation: both Enter and
					// MakeTable bail on `if (errorflag) then exit` (Amortize.pas:1204,
					// :1219, :1458). DOS draws NO TABLE and shows Iterate's own
					// non-convergence message (AMORTOP.pas:1489). The port used to drop
					// the failure on the floor and render the whole schedule at the
					// UNREFINED closed-form annuity seed — inventing an answer where DOS
					// refuses to answer.
					//
					// This arm strips the adjustments before iterating, mirroring
					// Iterate's own `til_adj` walk (Re_Amortize gate, AMORTOP.pas:1215),
					// so the terminal Go drives is DOS's terminal to the last bit — which
					// is exactly why the failure must be propagated rather than papered
					// over: when the two disagree it is by 1 ULP on a FLAT plateau, and
					// on a flat plateau the seed is not an approximation of the root, it
					// is an arbitrary point. 2026-07-28 fuzzer5 seed 20584:
					//
					//	amort_oracle 346743.73 0.0695930000 22 1 b365_360 prepaid r78 usa \
					//	  loandmy=17.7.2023 firstdmy=17.7.2024 mor=72 b120=7494.79 \
					//	  pre=24:348:52:132.49 pre=144:13:52:119.15 \
					//	  adj=36:0.1241760000:29542.58 adj=156:0.0766450000: \
					//	  adj=204:0.0715610000: targ=6818.09
					//
					// Both engines seed at x=30075.3405400514 and evaluate the terminal
					// to 397949.672203057, agreeing to 1 ULP — but DOS gets …572 at the
					// seed and …571 at the +0.1% probe while Go gets …571 then …572. The
					// sign flip sends DOS's secant to +1.03e17, whence the far-field
					// probe cancels back to 49008 and it converges in five more steps to
					// 64133.2516; Go's slope is +0 exactly, delta is 0, x never moves and
					// it stalls at count=25. The port then walked 380 rows at the SEED
					// 30075.34 (dInt −2610.62). Refusing is the honest outcome: the
					// plateau makes the base payment genuinely unrecoverable here.
					result.Err = fmt.Errorf("Computation of payment amount or " +
						"interest rate did not converge.")
					return result
				}
			} else if skipActive {
				// DOS's Iterate (dosIteratePayment) over the UNFORCED terminal
				// (generateFancyScheduleMode Output=nil) — the same single Newton DOS
				// uses. The terminal runs the full term with the full payment (no early
				// minpmt stop, no fold), so it is monotone in the payment and the Newton
				// converges. `refined != 0`, not `> 0`: DOS has no positivity check on
				// the solved payment (Amortize.pas:416-421 stores whatever
				// AMORTOP.pas:1415-1495 Iterate returns) — see the hasPrepay arm below
				// for the regime where the DOS-faithful root is negative.
				refined, ok := dosIteratePayment(input, d)
				if ok && refined != 0 {
					d = refined
				} else if !ok && allMonthsSkipped(input.SkipMonths.MonthSet) {
					// Every calendar month skipped: no payment can retire the loan,
					// the terminal is insensitive to the payment, and DOS's Iterate
					// fails with its non-convergence MessageBox (pass-2 finding 10;
					// oracle: `10000 0.12 24 12 skip=1-12` → "Computation of payment
					// amount or interest rate did not converge." — the port
					// previously emitted a silent all-zero-payment schedule).
					result.Err = fmt.Errorf("Computation of payment amount or " +
						"interest rate did not converge. Every month is skipped, so " +
						"no payment can retire the loan — remove some months from " +
						"Skip Months.")
					return result
				}
			} else if (hasKnownBalloon || targetActive) &&
				len(input.Adjustments) == 0 && !hasPrepay {
				// DOS's Iterate over the unforced fancy terminal solves the regular
				// payment that retires principal + balloon over the term. Adopt
				// whatever it converges to, INCLUDING a negative payment: a dominating
				// balloon (PV ≥ the loan) over-funds the principal, so the regular
				// payment DOS solves is negative (the borrower receives refunds), e.g.
				// `323522.49 0.048776 108 12 b365_360 exact prepaid inadv b90=502514.89`
				// → DOS payment -291.38. The port formerly kept the positive
				// plain-annuity seed and fired the A-W11 "balloon ignored" advisory
				// instead — a deliberate UX divergence now dropped in favour of DOS
				// convergence (2026-07-16 fancy fuzzer3). The A-W11 advisory no longer
				// fires here because the balloon IS applied in the schedule (its row is
				// the max payment), so appendResultAdvisories' maxPay test fails.
				if refined, ok := dosIteratePayment(input, d); ok {
					d = refined
				}
			} else if exactDaily(&settings) && !hasPrepay &&
				(!settings.InAdvance || !hasAnyAdvancedOption(input)) {
				// (Adjustments do NOT exclude this arm: the Iterate terminal skips
				// them — DOS's Re_Amortize gate, AMORTOP.pas:1215 — so an exact ARM
				// solves the same base payment as the plain exact loan. Pass-3
				// P3-F4: `amort_oracle 100000 0.08 120 12 b365 exact adj=24:0.10:`
				// → payment 1213.0959 = the plain exact value; the port previously
				// kept the non-exact closed-form 1213.2759.)
				// Exact (true-daily) loan with no balloon/target/adjustment/prepay:
				// solve the payment with the faithful port of DOS's Newton/secant
				// Iterate (dosIteratePayment), driving the continuous full-term
				// terminal balance to zero — matches DOS to the penny.
				//
				// This now covers BOTH the ordinary IN-ARREARS case and the
				// IN-ADVANCE (annuity-due) case: repayExactTerminal models the
				// in-advance settlement-shifted schedule shape (the dedicated path in
				// generateExactInAdvanceSchedule), closing the former exact×in-advance
				// frontier. Exact × in-advance WITH advanced options still falls
				// through to the documented frontier behaviour below.
				//
				// Seed the secant from the closed form, which carries the exact
				// actual-day first-period proration, rather than the unprorated
				// estimate `d`. For very long exact terms the terminal balance is
				// so sensitive to the payment that a poor seed makes the secant
				// diverge — it then returns ok=false and the engine keeps the
				// closed-form seed (~0.06% high, retiring the loan early). The
				// closed-form seed carries the exact first-period proration, so the
				// secant converges for the exact loans reachable here (incl. the
				// longest / highest-rate ones, via the relative acceptance clause in
				// dosIterate) without any bisection fallback.
				seed := d
				if cf, err := SolvePaymentClosedForm(input); err == nil && cf > 0 {
					seed = cf
				}
				// `refined != 0`, not `> 0`: DOS has no positivity check on the solved
				// payment (Amortize.pas:416-421 over AMORTOP.pas:1415-1495).
				if refined, ok := dosIteratePayment(input, seed); ok && refined != 0 {
					d = refined
				}
			} else if settings.Prepaid && !exactDaily(&settings) && !settings.InAdvance &&
				!hasKnownBalloon && !hasPrepay && !targetActive && !skipActive &&
				len(input.Adjustments) == 0 &&
				input.Moratorium.FirstRepayStatus >= types.InOutDefault {
				// DOS EARLY-EXIT (Amortize.pas:402-407): a PREPAID loan with none
				// of exact / in-advance / balloon / prepayment / target / skip
				// takes the plain closed-form annuity over the amortizing count
				// `nrepay` and EXITS — it does NOT Iterate. Moratorium is NOT
				// excluded from that condition, and DOS sets nrepay for a
				// moratorium to `NumberOfInstallments(first_repay, lastdate,
				// on_or_before)` (Amortize.pas:1302). At a day-count frequency
				// (weekly/biweekly/semimonthly) that uniform-period closed form
				// differs from the actual-day Iterate the mor-alone (non-prepaid)
				// case uses — so the prepaid moratorium payment is HIGHER.
				// 2026-07-13 pass-4 — verified vs the real DOS engine:
				//
				//	amort_oracle 100000 0.10 104 52 prepaid mor=3 → 1186.6343
				//	(the closed form over nrepay=92 on the weekly-forced 365
				//	 basis; the day-count Iterate gives the mor-alone 1186.5113,
				//	 which DOS uses only WITHOUT prepaid)
				//	amort_oracle 250000 0.1397 120 26 prepaid mor=4 → 2973.7798
				//
				// (Monthly/quarterly are unaffected: there the closed form and
				// the Iterate coincide, so this branch reproduces the same value
				// the refinement would.)
				if d2, ok := prepaidMoratoriumEarlyExit(loan, &settings, input.Moratorium, f); ok && d2 > 0 {
					d = d2
				}
			} else if !hasPrepay &&
				(settings.InAdvance ||
					oddFirstPeriod(loan.LoanDate, loan.FirstDate, loan.PerYr, &settings)) {
				// Universal non-shortcut refinement: any remaining fancy loan that
				// DOS would iterate rather than close-form — an odd first period OR
				// in-advance (annuity-due) — with no balloon/target/prepayment of
				// its own (e.g. a moratorium, or a plain odd-first fancy loan).
				// Snap-guarded so an already-exact estimate is kept. The Newton runs
				// over the unforced fancy terminal (fancyTerminal), DOS's own Iterate
				// terminal — see docs/dos_known_frontier.md #38.
				//
				// Adjustments do NOT exclude this arm (pass-5 P4-N7, 2026-07-14):
				// DOS's EstimateAndRefinePayment refines EVERY odd-first/in-advance
				// loan, and its Iterate walk STRIPS adjustments (Re_Amortize gate,
				// AMORTOP.pas:1215), so the base REGULAR payment is the plain
				// no-adjustment value — exactly what dosIteratePayment (which also
				// strips them) returns. The exact-daily arm above already admits
				// adjustments for this reason; a day-count first period (first
				// payment ON the loan date → zero-length first period) is always
				// "odd", so this refine now fires for payment-only adjustments at
				// day-count frequencies too. Oracle: `100000 0.06 72 24 adj=24::2083`
				// → payment 1515.5786 (the port kept the un-refined 1519.3676).
				// `refined != 0`, not `> 0`: DOS has no positivity check on the solved
				// payment (Amortize.pas:416-421 over AMORTOP.pas:1415-1495).
				if refined, ok := dosIteratePayment(input, d); ok && refined != 0 &&
					math.Abs(refined-d) > 1e-3 {
					d = refined
				}
			} else if hasPrepay && len(input.Adjustments) == 0 {
				// Known prepayment series with a blank REGULAR payment. DOS solves
				// the regular payment against the FULL schedule — the prepayments
				// (which REPLACE the regular payment when PlusRegular is off, or add
				// to it when on) are part of the loan the regular payment must
				// retire. The plain closed-form seed `d` ignores the series entirely
				// (it is the option-blind annuity), so refine it with the
				// schedule-oracle bisection, which drives the real prepayment-aware
				// terminal balance to zero (DOS EstimateAndRefinePayment with the
				// prepayment schedule). Covers arrears and in-advance alike.
				//
				// No `refined > 0` filter. DOS stores whatever Iterate returns —
				// `if Iterate(p, usap, h^.loandate, h^.firstdate, d, til_adj) then
				// h^.payamt := d else errorflag := true` (Amortize.pas:416-421) — and
				// Iterate itself has no positivity check (AMORTOP.pas:1415-1495). A
				// NEGATIVE regular payment is a real DOS regime here, not a solver
				// artefact: DOS's own seed is adjp = amount - SUM(prepay PV)
				// (Amortize.pas:390-396), so a prepayment series that over-funds the
				// principal drives it negative, and under a moratorium ComputeNext's
				// balloonpos=0 arm rewrites each collision row as
				// `payamt := payamt - d + interest` (AMORTOP.pas:641-642) — the row's
				// principal reduction is (extra - d), which only a negative d can make
				// large. The gate rejected exactly those roots and silently kept the
				// option-blind annuity seed, rendering a schedule whose balance GROWS.
				// 2026-07-25 fuzzer5 pass 2 — verified vs the real DOS engine:
				//
				//	amort_oracle 316506.64 0.1169630000 24 1 inadv mor=60 \
				//	  pre=36:149:12:393.79 pre=132:105:12:245.67 targ=8770.72 \
				//	  pts=0.033799
				//	→ payment -25827.5109, 208 rows, totalInt 552279.34. Go's Newton
				//	already converged to -25827.5109 (the terminal is clean and
				//	monotone: term(-30000) = -205600.87, term(-25827.5109) = -0.0007,
				//	term(0) = +1272659.68); only the gate stopped it, leaving a 208-row
				//	walk with totalInt 729193.47. Forcing DOS's payment through Go's
				//	walk already agreed row-for-row, so the walk was never at fault.
				refined, ok := dosIteratePayment(input, d)
				if ok && refined != 0 {
					d = refined
				} else if !ok {
					// DOS's Iterate did not converge (dosIteratePayment !ok): a
					// prepayment series that spans/over-fills the loan leaves no
					// regular payment able to retire it, so the terminal is
					// insensitive to the payment and DOS blocks with "Computation of
					// payment amount or interest rate did not converge." (AMORTOP.pas:
					// 1489). The port previously kept the option-blind seed and
					// rendered a DEGENERATE schedule whose balance GROWS (e.g. a
					// $360k loan with a pre=12:6:1:356.96 series ballooned to a $744k
					// terminal). Match DOS: refuse, no schedule. 2026-07-16 fancy
					// fuzzer3 (dos_fuzzer3_fancy_test.go).
					result.Err = fmt.Errorf("Computation of payment amount or " +
						"interest rate did not converge.")
					return result
				}
			}
		}

		// TackOnFinalBalloon (Amortize.pas:1386-1394 → :1040-1088). DOS runs
		// this IMMEDIATELY BEFORE generating the table, so on the two arms where
		// it leaves the row inside nballoons — the merge arm and the sub-minpmt
		// arm — the row still takes part in that table and in the APR. Placement
		// here mirrors that ordering exactly.
		tackIn := input
		tackIn.Loan = loan
		tackIn.Loan.PayAmt = d
		// DOS's `fancy` at Amortize.pas:1386 is the Advanced-Options UI toggle
		// ALONE. The port additionally FORCES input.Fancy for internal routing
		// (exactDaily / US-Rule, above), so an ORDINARY loan with no advanced
		// options can reach this branch with input.Fancy true — DOS would never
		// tack a terminating balloon onto it (and it has no residual to tack).
		// Gate on the caller's toggle, the same `uiFancy` capture
		// DetermineLastPaymentDate's dispatch already keys on (P3-F1).
		tackIn.Fancy = uiFancy
		tack = tackOnFinalBalloon(tackIn, &settings)
		if tack.Fired && tack.Live {
			input.Balloons = append([]BalloonPayment(nil), input.Balloons...)
			if tack.MergeIdx >= 0 {
				// merge_w_existing: DOS overwrites the user's terminating balloon
				// in place and repaints the amount cell as an output.
				input.Balloons[tack.MergeIdx].Amount = tack.Amount
				input.Balloons[tack.MergeIdx].AmountStatus = types.InOutOutput
				tackEchoIdx = tack.MergeIdx
			} else {
				input.Balloons = append(input.Balloons, BalloonPayment{
					DateStatus:   types.InOutOutput,
					Date:         tack.Date,
					AmountStatus: types.InOutOutput,
					Amount:       tack.Amount,
				})
				tackEchoIdx = len(input.Balloons) - 1
			}
		}

		result = generateFancySchedule(input, d, &settings, truerate, f)
	}

	// DOS's ONE-SIDED-ADJUSTMENT PRE-PASS (Amortize.pas:1408-1419). See
	// unreachedAdjPrepass.
	if err := unreachedAdjPrepass(input, &settings, d, truerate, f, &result); err != nil {
		return AmortResult{Err: err}
	}

	// A9: when the caller supplied discount points, compute the APR —
	// the rate that equates the present value of the scheduled
	// payments to the borrower's net proceeds (Amortize.pas: function
	// EstimateAndRefineAPRwithPoints).
	if result.Err == nil && loan.PointsStatus >= types.InOutDefault &&
		len(result.Schedule) > 0 {
		applyAPR(&result, input, loan, &settings, d, truerate, f)
	}

	// A-W9: the regular payment does not amortize the loan over the stated term,
	// so the last scheduled payment absorbs the residual. DOS has no such string
	// — it shows the terminating balloon instead (surfaced on result.Balloons
	// below) — but the port keeps the advisory because the WEB payment table,
	// unlike the DOS grid, gives the user no other cue that the final row is
	// carrying a lump sum. appendScheduleWarnings carries the identical text for
	// AmortizeDOS; the two engines must stay byte-for-byte identical here.
	if result.Err == nil && len(result.Schedule) > 0 && d > 0 {
		last := result.Schedule[len(result.Schedule)-1]
		if last.PayAmt > d*1.5 && last.PayAmt-d > minPmt {
			result.Warnings = append(result.Warnings,
				"The regular payment does not amortize the loan over the stated "+
					"term — the final payment includes an implied terminating "+
					"balloon for the remaining balance.")
		}
	}

	// DA_TerminatingBalloonChanged (Amortize.pas:1071-1073): on the merge arm DOS
	// recomputed the amount the user typed into their terminating balloon, so it
	// says so. Text copied verbatim from the DOS message box, spelling included.
	if tack.Fired && tack.Adjusted {
		result.Warnings = append(result.Warnings,
			"Please note that the amount of your terminating balloon has been ajusted.")
	}

	// Re-apply the snapshotted post-FirstPass term + dates. The
	// schedule generators return a fresh AmortResult that overwrites
	// the assignments we made earlier, so without this step a
	// successful run would echo NPeriods=0 / zero dates back to the API.
	result.NPeriods = derivedNPeriods
	result.FirstDate = derivedFirstDate
	result.LastDate = derivedLastDate

	// ...EXCEPT the last date, when Re_Amortize's unguarded `var l` snap moved
	// DOS's h^.lastdate global (task #94, AMORTOP.pas:1547). derivedLastDate is
	// the FirstPass snapshot taken before the walk, and FirstPass derives that
	// cell via AddNPeriods, which has no month-end stickiness — so re-applying it
	// unconditionally would throw the snap away. DOS has one global and the
	// display reads it AFTER the prepass, so the snap wins where it happened.
	// Restricted to a valid DateRec so the zero value (no snap) cannot blank a
	// good date.
	if dateutil.DateOK(result.reAmortLastDate) {
		result.LastDate = result.reAmortLastDate
	}

	// Unusually-high-rate sanity check. DOS shows this warning only on
	// the mortgage screen (MortgageScreenUnit.pas:222); the amortization
	// screen has no equivalent. A typo'd rate is just as damaging here,
	// so we extend the same soft warning to amortization. LoanRate is a
	// nominal fraction (0.06 = 6%), so the threshold is 20% nominal
	// directly. Fire only on a user-entered rate, never a solved one.
	// Appended here (after the schedule generators return a fresh
	// result) so it survives the result reassignment above.
	if loan.LoanRateStatus == types.InOutInput && loan.LoanRate > unusuallyHighRate {
		result.Warnings = append(result.Warnings, fmt.Sprintf(
			"Loan Rate of about %.2f%% is unusually high — double-check it was "+
				"entered in percent (for example 6 for 6%%, not 0.06 or 600).",
			loan.LoanRate*100))
	}

	// Echo the balloons the engine used — including any "target" balloon whose
	// amount it solved (AmountStatus becomes Output) — so the UI can fill the
	// blank Amount cell with the computed value.
	for i := range input.Balloons {
		b := input.Balloons[i]
		// The LIVE tack-on row is spliced in above with datestatus = outp, exactly
		// as DOS writes it (`balloon[nballoons]^.datestatus := outp`,
		// Amortize.pas:1061). But the port's status ordering is
		// InOutOutput(1) < InOutDefault(2) < InOutInput(3), and "present in the
		// grid" is spelled `>= InOutDefault` everywhere — so a row DOS marks as
		// computed reads to this loop as an EMPTY row and was dropped. DOS has no
		// such problem: BalloonValues2Grid walks the raw array by `nballoons` and
		// never consults datestatus.
		//
		// Observable as a §46 fuzz divergence ("DOS tacked a terminating balloon
		// the port did not"), seed 8958 / N=1000:
		//
		//	amort_oracle 288101.56 0.0643470000 26 2 b365 inadv r78 mor=30 \
		//	  b72=73939.89 b84=33039.48 b102=17175.22 pre=96:74:52:69.44 \
		//	  targ=3932.71 pts=0.024049 payhard=15471.38
		//	DOS row 4: 1/1/2037 15472.2100, nballoons=4  (payamt 15471.38, so the
		//	residual 0.83 is under minpmt and DOS does NOT dec(nballoons))
		//
		// The port computed 15472.2100 on 2037-01-01 with Live=true and then threw
		// the row away here. Only the LIVE NON-MERGE arm was affected: the merge
		// arm keeps the user's own datestatus, and the de-activated arm is appended
		// explicitly below. That arm needs |balloon - payamt| < minpmt ($1.00), so
		// it is rare in fuzz — 1 hit in 909 compared cases.
		isTack := i == tackEchoIdx
		if !isTack && b.DateStatus < types.InOutDefault {
			continue
		}
		if !dateutil.DateOK(b.Date) {
			continue
		}
		result.Balloons = append(result.Balloons, ResolvedBalloon{
			Date:     b.Date,
			Amount:   b.Amount,
			Solved:   b.AmountStatus == types.InOutOutput,
			TackedOn: isTack,
		})
	}
	// The DE-ACTIVATED tack-on row (DOS dec(nballoons)). It is not in
	// input.Balloons — it took no part in the schedule or the APR — but DOS
	// paints it into the Balloon Payments grid all the same, because
	// BalloonValues2Grid walks the raw array and ignores nballoons.
	if tack.Fired && !tack.Live {
		result.Balloons = append(result.Balloons, ResolvedBalloon{
			Date:     tack.Date,
			Amount:   tack.Amount,
			Solved:   true,
			TackedOn: true,
		})
	}

	appendResultAdvisories(&result, &input, &loan, prepaySolvedAmt, prepaySolved, payWasInput)
	applyPointsSettlement(&result, &loan, &settings, truerate)
	return result
}

// applyPointsSettlement adds the discount-points charge to the schedule's
// settlement line, mirroring DOS MakeTable (Amortize.pas:1476-1491): the
// loan-date line fires when `(prepaid and PrepaidInterest>0) or (points<>0)`
// with `interest := PrepaidInterest + points*amount`, and that interest flows
// into the totals. 2026-07-11 audit finding A12 — the port previously used
// Points only for the APR, so a points-bearing loan's schedule and total
// interest were missing the charge. Verified vs the real DOS engine:
//
//	amort_oracle 10000 0.12 12 12 pts=0.02 dumpraw
//	→ L0: 1/1/24 int 200.00 bal 10000.00; totals interest 861.85 (vs 661.85 plain)
//	amort_oracle 10000 0.12 12 12 pts=0.02 prepaid loandmy=15.1.2024 firstdmy=1.3.2024 dumpraw
//	→ L0: 1/15/24 int 253.33 (= 53.33 prepaid stub + 200.00 points, ONE combined row)
//	amort_oracle 10000 0.12 12 12 pts=0.02 b6=2000 pay=730 dumpraw
//	→ fancy path gets the same L0 (the DOS settlement line precedes the
//	  fancy/simple split), so this runs for every engine at the Amortize level.
func applyPointsSettlement(result *AmortResult, loan *Loan, settings *Settings, truerate float64) {
	if result.Err != nil || len(result.Schedule) == 0 {
		return
	}
	if loan.PointsStatus < types.InOutDefault || loan.Points == 0 {
		return
	}
	pts := loan.Points * loan.Amount
	hardPayment := loan.PayAmtStatus == types.InOutInput
	first := &result.Schedule[0]
	if first.PayNum == 0 && dateutil.DateComp(first.Date, loan.LoanDate) == 0 {
		// Existing settlement stub (prepaid / in-advance): DOS combines the
		// points charge into the SAME line — and, crucially, rounds the COMBINED
		// sum once: `interest := PrepaidInterest + h^.points*h^.amount; if
		// hard_payment then Round2(interest)` (Amortize.pas:1482-1483). Rounding
		// the stub and the points separately loses a cent whenever the two
		// sub-half-cent fractions add across the boundary (2026-07-22 finding:
		// stub 3.8604 + pts 9.0047 → DOS 12.87, separate rounding gave 12.86).
		// The generators record the stub's unrounded value in rawSettlement so
		// the combined line can be recomputed here; the delta then propagates to
		// the running IntToDate and the totals exactly as DOS accumulates the
		// rounded line. See dos_pennyfold_settlement_test.go.
		if hardPayment && result.hasRawSettlement {
			combined := interest.Round2(result.rawSettlement + pts)
			delta := combined - first.Interest
			first.PayAmt += delta
			first.Interest = combined
			for i := range result.Schedule {
				result.Schedule[i].IntToDate += delta
			}
			result.TotalPaid += delta
			result.TotalInt += delta
			return
		}
		// No raw stub recorded (e.g. the AmortizeDOS structural-port path):
		// keep the previous per-component behavior.
		if hardPayment {
			pts = interest.Round2(pts)
		}
		first.PayAmt += pts
		first.Interest += pts
	} else {
		// No stub was emitted. DOS still adds the RAW PrepaidInterest here:
		// the `PrepaidInterest > 0` test in the block-entry condition
		//
		//	if ((prepaid) and (PrepaidInterest>0)) or
		//	   ((h^.pointsstatus>empty) and (h^.points<>0)) then
		//
		// gates only whether the settlement LINE exists; once points bring the
		// line into being through the second disjunct, the body is the
		// unconditional sum `interest := PrepaidInterest + h^.points*h^.amount`
		// (Amortize.pas:1476-1491) — sign and all.
		//
		// PrepaidInterest is normally 0 when no stub fires: the generator's stub
		// gate is `loanDate < firstDate − 1 period`, and DOS clears `prepaid`
		// outright on the other side of that boundary (Amortize.pas:1252-1259,
		// ported at engine.go:425-432), so `settings.Prepaid` here already
		// carries DOS's cleared value. The exception is the EXACT boundary,
		// loanDate == firstDate − 1 period, where prepaid survives and DOS
		// evaluates YearsDif on two IDENTICAL dates — which is not zero on the
		// 360 basis, because INTSUTIL.pas's 30/360 body carries the Feb clause
		//
		//	else if (a.m=2) and (a.d>27) then til:=til-(30-a.d)/360; {Feb 28 or 29}
		//
		// so YearsDif(28.2.2023, 28.2.2023) = −2/360 and PrepaidInterest is
		// NEGATIVE. 2026-07-26, amortization fuzzer5 seed 20250 — verified vs
		// the real DOS engine with an instrumented Amortize.pas:1482, which
		// printed `SETTLE pi=-214.676434 pts=0.01402200 amt=455080.0600
		// fee=6381.132601`:
		//
		//	amort_oracle 455080.06 0.0849120000 12 1 prepaid pts=0.014022 \
		//	  loandmy=28.2.2023 firstdmy=28.2.2024
		//	→ DOS interest 294277.57 = 288111.11 baseline + (6381.13 − 214.68);
		//	  the port added the bare 6381.13 fee and reported 294492.24.
		//
		// hasRawSettlement guards the case where a stub WAS recorded but does
		// not sit at Schedule[0] — there the prepaid term is already banked.
		fee := pts
		if settings != nil && !result.hasRawSettlement {
			if pi, err := PrepaidInterest(loan, settings, truerate); err == nil {
				fee += pi
			}
		}
		if hardPayment {
			fee = interest.Round2(fee)
		}
		// No stub — emit the settlement line at the loan date.
		row := PaymentRecord{
			PayNum:    0,
			Date:      loan.LoanDate,
			PayAmt:    fee,
			Interest:  fee,
			Principal: loan.Amount,
		}
		result.Schedule = append([]PaymentRecord{row}, result.Schedule...)
		pts = fee
	}
	for i := range result.Schedule {
		result.Schedule[i].IntToDate += pts
	}
	result.TotalPaid += pts
	result.TotalInt += pts
}

// anySkip reports whether any month in the set is flagged for skip.
func anySkip(set [13]bool) bool {
	for m := 1; m <= 12; m++ {
		if set[m] {
			return true
		}
	}
	return false
}

// estimatePayment computes an initial payment estimate using the annuity formula.
// oddFirstPeriod reports whether the first payment is not exactly one
// compounding period after the loan date — a short or long "odd" first
// period. The closed-form payment estimate assumes a full first period, so
// when this is true the blank-payment solve must refine the estimate against
// the actual (prorated) schedule to match DOS. Ported behavior: DOS's
// EstimateAndRefinePayment iterates (Amortize.pas:416) for every non-trivial
// case; we reproduce that with the schedule-oracle bisection only where the
// closed form is actually inexact, which is the odd-first period.
// needPaymentRefine reports whether the blank-payment closed-form estimate for a
// PLAIN (non-fancy) loan must be refined against the real schedule to match DOS.
// This encodes DOS's payment-solve shortcut (Amortize.pas:402): DOS uses the
// closed-form estimate directly only for the plain case and ITERATES otherwise.
// For a plain loan the closed form is exact iff the first period is calendar-
// natural and the payment is in-arrears — so refine when the first period is odd
// OR the loan is in-advance (annuity-due). R78 has its own precomputed split and
// is never refined here. Centralizing this keeps the "every non-shortcut solve is
// refined" guarantee in one place rather than rediscovering each case as a bug.
func needPaymentRefine(loan *Loan, s *Settings) bool {
	// DOS's payment solve is entirely R78-AGNOSTIC: EstimateAndRefinePayment
	// (Amortize.pas:377-430) routes purely on in_advance / dates / options and
	// never inspects the R78 flag — R78 only changes the interest SPLIT of the
	// resulting schedule, so an R78 loan's payment is identical to the plain
	// loan's. The refine trigger is therefore the SAME as the non-R78 case
	// (in-advance OR an odd first period). 2026-07-13 pass-4 finding: the prior
	// `if s.R78 { return s.InAdvance }` special-case skipped the odd-first
	// refine for arrears R78, leaving the unrefined estimate — verified wrong
	// vs the real DOS engine (with first payment ON the loan date, a
	// zero-length first period):
	//
	//	amort_oracle 10000 0.1213 36 24 r78 → payment 302.9827 (= the plain
	//	semimonthly payment; the un-refined estimate was 304.5140)
	//
	// The former A4 in-advance case is preserved (in-advance still refines);
	// only the arrears odd-first R78 case changes.
	return s.InAdvance || oddFirstPeriod(loan.LoanDate, loan.FirstDate, loan.PerYr, s)
}

func oddFirstPeriod(loanDate, firstDate types.DateRec, perYr int, s *Settings) bool {
	if !dateutil.DateOK(loanDate) || !dateutil.DateOK(firstDate) {
		return false
	}
	// Decide against firstPeriodProrate — the SAME first-period length the closed-form
	// RepayLoan terminal uses. A clean calendar first period (even on 365) is a whole
	// period (prorate=1) and must NOT be treated as odd, matching DOS's forward
	// payment (odd-first oracle cube). Only a genuine odd-DAY stub, or a fractional
	// non-360 count on a non-clean boundary, refines.
	return math.Abs(firstPeriodProrate(loanDate, firstDate, perYr, s)-1) > 1e-6
}

// firstPeriodProrate returns the first period's length as a fraction of one
// payment period. DOS uses an exact MONTH-based fraction on clean period
// boundaries — matching day-of-month and a month-dividing frequency — regardless
// of the basis: months / (12/perYr). Only a genuine odd-DAY stub (loan
// day-of-month ≠ first-payment day) uses the basis-specific actual-day count.
//
// This matters on the 365 basis: a calendar-natural first period is not exactly
// 1/perYr of the actual (366-day leap) year — YearsDif*perYr = 182/366*2 = 0.9945
// instead of 1.0 — which skewed the first schedule row. DOS treats it as a whole
// period (prorate = 1). On the 360 basis the two already agree (30/360 makes a
// whole month exactly 1/12 of a year), so this changes only 365-basis behavior,
// and only on clean boundaries — odd-day stubs (already DOS-faithful) are
// untouched. Weekly/biweekly (perYr not dividing 12) keep the day-based count.
func firstPeriodProrate(loanDate, firstDate types.DateRec, perYr int, s *Settings) float64 {
	// Exact interest on a non-360 basis accrues on the ACTUAL day count for
	// EVERY period — it does not take the clean-month whole-period shortcut.
	// DOS AMORTOP.pas:625: the whole-month `timedif` is used only when
	// `(basis=x360) or (not exact)`; otherwise it is `YearsDif(date, prevdate)`.
	// Clean-boundary gate is DOS DaysCloseEnough (INTSUTIL.pas:716-727), not
	// exact day equality: an end-of-month clamped pair (loan Jan 31 → first
	// Feb 29) is a whole period too (audit finding A1; see periodYearFraction).
	// On the 360 basis the whole-month shortcut is SKIPPED: DOS's first-period
	// prorate there is the raw 30/360 YearsDif (Amortize.pas:1286+1516), which
	// equals a whole period on ordinary clean pairs (Jan 1 → Feb 1 = 30/360)
	// but NOT on clamped or February month-end pairs. 2026-07-12 pass-3
	// finding AF2 — verified vs the real DOS engine:
	//
	//	amort_oracle 50000 0.10 14 12 loandmy=31.1.2024 firstdmy=29.2.2024 rows
	//	→ row1 int 402.78 (= 29/360; Go's whole-month shortcut gave 416.67),
	//	  interest 3166.53
	//	amort_oracle 50000 0.10 14 12 loandmy=28.2.2025 firstdmy=28.3.2025 rows
	//	→ row1 int 388.89 (Feb-end d1→30 rule: 28/360)
	//	quarterly 31.1→30.4 → 1250.00 and annual 31.1→31.1 → 5000.00 (both =
	//	raw 30/360 YearsDif too; the shortcut only ever disagreed near Feb)
	if s.Basis != types.Basis360 && !exactDaily(s) && perYr > 0 && 12%perYr == 0 &&
		dateutil.DaysCloseEnough(firstDate, loanDate, perYr) {
		months := (firstDate.Time.Year()-loanDate.Time.Year())*12 +
			(int(firstDate.Time.Month()) - int(loanDate.Time.Month()))
		if months >= 0 {
			return float64(months) / float64(12/perYr)
		}
	}
	return dateutil.YearsDif(firstDate, loanDate, s.Basis, s.YrInv, true) * float64(perYr)
}

// periodYearFraction returns the length of the [prev, cur] interval in YEARS for
// per-period interest accrual. On a clean month boundary (matching day-of-month,
// month-dividing frequency) it returns the exact month-based fraction
// `months / 12` — basis-independent, matching DOS's per-period accrual
// (`p*(f-1)`); only a genuine partial/odd-day span (an off-cycle balloon or
// prepayment remainder, or a day stub) uses the basis-specific actual-day
// `YearsDif`. This is the fancy-schedule analog of firstPeriodProrate: it keeps
// the 365 basis from skewing each row's interest split (31- vs 28-day months) on
// balloon/prepayment/moratorium/skip loans, and makes the in-advance accrual
// around skipped months match DOS. (Daily compounding still needs the true day
// count and is handled separately by the caller.)
func periodYearFraction(prev, cur types.DateRec, perYr int, s *Settings) float64 {
	// Exact interest on a non-360 basis uses the ACTUAL day count for every
	// period (DOS AMORTOP.pas:625 YearsDif branch), so it skips the clean-month
	// whole-period shortcut.
	//
	// The shortcut's gate is DOS `DaysCloseEnough` (INTSUTIL.pas:716-727), NOT
	// exact day equality: an end-of-month CLAMPED pair (Jan 31 → Feb 29 → Mar
	// 31) and the semimonthly 15th/month-end pair are whole periods too, and
	// the semimonthly (peryr=24) case carries DOS's ±half-month day adjustment
	// (AMORTOP.pas:628-629). 2026-07-11 audit finding A1 (supersedes V6-7's
	// "presentation-grade" classification): with the equality-only gate every
	// month-end fancy loan's Feb-adjacent rows accrued 29/360 or 31/360
	// instead of DOS's 30/360 — ~$32/row on $100k@12%. Verified vs the real
	// DOS engine:
	//
	//	amort_oracle 100000 0.12 24 12 loandmy=31.12.2023 firstdmy=31.1.2024 targ=0.01 rows
	//	→ DOS 2/29/24 int 962.93, 3/31/24 int 925.48 (Go was 930.84 / 956.02)
	//	amort_oracle 100000 0.10 48 24 firstdmy=15.1.2024 b12=20000 plusreg pay=2229.8065
	//	→ DOS 2/29/24 int 393.79 = exactly 1/24 year (Go was 367.54 = 14/360)
	if !exactDaily(s) && perYr > 0 && dateutil.DaysCloseEnough(cur, prev, perYr) {
		timedif := float64(cur.Time.Year()-prev.Time.Year()) +
			float64(int(cur.Time.Month())-int(prev.Time.Month()))/12.0
		if perYr == 24 {
			timedif += math.Round(2.0*float64(cur.Time.Day()-prev.Time.Day())/30.0) / 24.0
		}
		// NO positivity guard. DOS (AMORTOP.pas:625-632) is a plain if/else —
		// once DaysCloseEnough is true it USES the shortcut value, even when
		// that value is exactly 0:
		//
		//	if ((df.c.basis=x360) or (not df.c.exact))
		//	   and DaysCloseEnough(date, prevdate, h^.peryr) then
		//	  begin
		//	    timedif := (date.y - prevdate.y) + (date.m - prevdate.m) / 12;
		//	    ...
		//	  end
		//	else timedif := YearsDif(date, prevdate);
		//
		// A zero-length period is reachable, and in-advance loans reach it
		// routinely. Amortize.pas:205-208 (FirstPass) forces the internal
		// prepaid flag on for in-advance:
		//
		//	if (df.c.in_advance) then prepaid := true
		//	else prepaid := df.c.prepaid;
		//
		// so RepayFancyLoan's prologue (AMORTOP.pas:1151-1158) takes the
		// prepaid arm but SKIPS the subtract — `paidthru := firstdate` with no
		// AddPeriod — and NextPayment.Init seeds prevdate = FirstDate itself.
		// An extra (prepayment/balloon) dated exactly ON FirstDate is then
		// emitted with balloonpos = -1 and `date := nextextra.date` = FirstDate,
		// so date == prevdate and the shortcut yields 0 + 0/12 = 0. Confirmed
		// against the real engine with the `-mode cn` trace oracle:
		//
		//	CN d=125-2-28 bpos=-1 p=100000.000000 usap=0.000000
		//	   int=0.000000 pay=100.000000 td=0.00000000 pout=99900.000000
		//
		// The discarded-zero path was NOT harmless, because YearsDif's 360
		// branch is not zero at a coincident pair: its Feb-28/29 rule
		// (INTSUTIL.pas, "version adopted 12/93")
		//
		//	else if (a.m=2) and (a.d>27) then til := til - (30-a.d)/360;
		//
		// makes YearsDif(2/28/25, 2/28/25) = -2/360, i.e. NEGATIVE interest.
		// Minimal repro:
		//
		//	amort_oracle 100000 0.10 10 12 inadv plusreg \
		//	  loandmy=28.1.2025 firstdmy=28.2.2025 pre=1:1:12:100
		//	→ DOS int=5041.89, row 1 (2/28/25) int 0.00 prin 100.00 bal 99900.00
		//	   Go  int=4983.99, row 1          int -55.56          bal 99844.44
		//	   (-55.56 = 100000 * 0.10 * -2/360)
		//
		// Controls that already agreed and stay agreeing: firstdmy=15.2.2025
		// (not a month end, so the Feb rule never fires), pre=7 (the extra
		// lands off the grid so date != prevdate), and the same case without
		// `inadv` (prepaid stays false, paidthru := loandate).
		//
		// Surfaced by fuzzer5 seed 20109:
		//	316129.01 0.1381460000 30 2 prepaid inadv plusreg usa
		//	loandmy=28.8.2024 firstdmy=28.2.2025 mor=36 b60=19835.95
		//	b120=75878.78 b144=84886.53 pre=54:269:52:73.29
		//	pre=6:225:26:183.98 targ=3128.92 pts=0.005454
		//	→ was DOS int=527032.47 vs Go 526397.94 (dInt 634.53).
		return timedif
	}
	return dateutil.YearsDif(cur, prev, s.Basis, s.YrInv, true)
}

// dosRepayFrom reproduces DOS's `repay_from` — "the date on which you begin
// amortizing" (Amortize.pas:1259-1293). It is the origin every present value in
// EstimateAndRefinePayment's seed is discounted BACK to, and it is NOT the loan
// date except in the plain arrears case:
//
//	if (mor^.first_repaystatus >= defp) then      { moratorium }
//	   repay_from := mor^.first_repay;  AddPeriod(repay_from, peryr, firstdate.d, subtract)
//	else if prepaid then
//	   repay_from := h^.firstdate;      AddPeriod(repay_from, peryr, firstdate.d, subtract)
//	else
//	   repay_from := h^.loandate;
//
// The port used to discount from LoanDate unconditionally, which on a deep
// moratorium understates every balloon/prepayment discount by the whole
// interest-only window — pushing the seed high enough to land the secant on a
// DIFFERENT root of a non-monotone terminal (see dosPaymentSeed).
func dosRepayFrom(input LoanInput, loan *Loan, settings *Settings) types.DateRec {
	base := types.UnknownDate()
	switch {
	case input.Moratorium.FirstRepayStatus >= types.InOutDefault:
		base = input.Moratorium.FirstRepay
	case settings.Prepaid || settings.InAdvance:
		// `prepaid` here is DOS's GLOBAL, not the user's checkbox. FirstPass
		// (Amortize.pas:205-208) forces it on for an in-advance loan —
		//
		//	if (df.c.in_advance) then prepaid := true else prepaid := df.c.prepaid;
		//
		// — and the later demotion (Amortize.pas:1252-1259, applied to
		// settings.Prepaid at engine.go:425) is itself gated on `not
		// df.c.in_advance`, so in-advance reaches this block with prepaid TRUE
		// unconditionally. The port read settings.Prepaid alone, so an in-advance
		// loan whose user prepaid box was OFF discounted the seed's balloon and
		// prepayment present values from the LOAN DATE instead of firstdate minus
		// one period.
		//
		// That is not a rounding-scale error. Iterate is a bracket-free secant on
		// a terminal with several roots, so the seed SELECTS the root (see the
		// note below on fuzzer5 seed 8911). 2026-07-27 fuzzer5 cycle 22 seed
		// 20311, verified vs the real DOS engine:
		//
		//	amort_oracle 67513.25 0.1295380000 9 1 exact inadv r78 usa \
		//	  loandmy=5.11.2023 firstdmy=5.3.2025 b16=8293.43 b40=15041.66 \
		//	  b64=13961.98 pre=52:65:24:23.38 targ=292.65 pts=0.003898
		//	→ DOS int=45214.15 paid=112727.40 (payment -25814.8739); the
		//	  loan-date origin walked the secant to the OTHER root, +35275.7825,
		//	  for int=50264.21 paid=117777.46. Both roots retire the loan (the
		//	  DOS `dumpraw` tail is 0.00 either way), so only the seed tells them
		//	  apart.
		base = loan.FirstDate
	default:
		return loan.LoanDate
	}
	if !dateutil.DateOK(base) || !dateutil.DateOK(loan.FirstDate) {
		return loan.LoanDate
	}
	back, err := dateutil.AddPeriod(base, loan.PerYr, loan.FirstDate.Time.Day(), true)
	if err != nil {
		return loan.LoanDate
	}
	return back
}

// dosNrepay reproduces DOS's `nrepay` — "the real number of payments over which
// to amortize" (Amortize.pas:1298-1319). When principal repayment starts later
// than the first payment date (a moratorium, or a balloon that precedes
// FirstDate), the seed annuity runs over the SHORTER amortizing window, not the
// full term:
//
//	if (h^.lastok) and (DateComp(mor^.first_repay, h^.firstdate) <> 0) then
//	   nrepay := NumberOfInstallments(mor^.first_repay, h^.lastdate, h^.peryr, on_or_before)
//	else
//	   nrepay := h^.nperiods;
func dosNrepay(input LoanInput, loan *Loan) int {
	firstRepay := types.UnknownDate()
	if input.Moratorium.FirstRepayStatus >= types.InOutDefault {
		firstRepay = input.Moratorium.FirstRepay
	} else if len(input.Balloons) > 0 &&
		input.Balloons[0].DateStatus >= types.InOutDefault &&
		dateutil.DateOK(input.Balloons[0].Date) && dateutil.DateOK(loan.FirstDate) &&
		dateutil.DateComp(input.Balloons[0].Date, loan.FirstDate) < 0 {
		firstRepay = input.Balloons[0].Date
	} else {
		return loan.NPeriods
	}
	if !loan.LastOK || !dateutil.DateOK(firstRepay) || !dateutil.DateOK(loan.LastDate) ||
		!dateutil.DateOK(loan.FirstDate) ||
		dateutil.DateComp(firstRepay, loan.FirstDate) == 0 {
		return loan.NPeriods
	}
	n, _ := dateutil.NumberOfInstallments(firstRepay, loan.LastDate, loan.PerYr, types.OnOrBefore)
	if n <= 0 {
		return loan.NPeriods
	}
	return n
}

// NOTE: there used to be a dosSeedPVFactor here, returning adjp/amount so that
// callers could scale a plain annuity into the option-loan seed:
//
//	d := estimatePayment(...) * dosSeedPVFactor(...)
//
// Both call sites now use dosSeedPayment instead, which builds the seed the way
// DOS does — additively, then over a single quotient. The factored form is
// algebraically identical but rounds twice more, and the ~2 ULP that costs is
// enough to send DOS's bracket-free secant to a different root. See
// dosSeedPayment for the seed-20622 measurement.

// dosSeedAdjP builds DOS's `adjp` — the principal net of the present value of
// every balloon and every prepayment series — exactly as EstimateAndRefinePayment
// forms it (Amortize.pas:384-396):
//
//	adjp := h^.amount;
//	rate := RateFromYield(h^.loanrate, h^.peryr);
//	for i := 1 to user_nballoons do
//	  adjp := adjp - balloon[i]^.amount * exxp(-rate*YearsDif(balloon[i]^.date, repay_from));
//	for i := 1 to npre do
//	  begin
//	    FirstLastAndFF(rate, first, last, ff, i);
//	    if (abs(1-ff)>teeny) then
//	      adjp := adjp - pre[i]^.payment * (first - last*ff) / (1 - ff)
//	    else adjp := adjp - pre[i]^.payment * pre[i]^.nn;
//	  end;
//
// It is deliberately an ADDITIVE accumulator returned as an ABSOLUTE amount, not
// a ratio: dosSeedPayment feeds it straight into DOS's single quotient so the
// seed is bit-identical. See dosSeedPayment for why the last bit matters.
func dosSeedAdjP(input LoanInput, loan *Loan, settings *Settings) float64 {
	truerate, _ := ComputeTrueRate(loan, settings)
	origin := dosRepayFrom(input, loan, settings)
	adjp := loan.Amount
	for i := range input.Balloons {
		b := &input.Balloons[i]
		if b.AmountStatus >= types.InOutDefault && b.DateStatus >= types.InOutDefault {
			// isLoanCalc=TRUE. DOS's YearsDif (INTSUTIL.pas:787-823) picks its
			// non-360 form from the SCREEN, not the call site: `thisrun in
			// [iPVL,iINV,iCHR]` (or basis=x365_360) gets the flat Julian/yrinv
			// form, and everything else — this is iAMZ — gets the leap-aware
			// year-by-year walk that divides each calendar year by its OWN 365 or
			// 366. On a plain x365 loan the two differ (yrinv is 1/365.25): for
			// 2025-12-01 → 2027-03-01 the loan form gives 455/365 = 1.2465753
			// where the PV form gives 455/365.25 = 1.2457221. That feeds the
			// balloon discount, hence adjp, hence the seed — and Iterate is a
			// bracket-free secant, so a seed off in the 4th decimal can walk to a
			// different root entirely.
			yd := dateutil.YearsDif(b.Date, origin, settings.Basis, settings.YrInv, true)
			if disc, e := interest.Exxp(-truerate * yd); e == nil {
				if dpTraceSeed {
					fmt.Fprintf(os.Stderr, "BAL i=%d amt=%.17g yd=%.17g disc=%.17g adjpbefore=%.17g\n",
						i+1, b.Amount, yd, disc, adjp)
				}
				adjp -= b.Amount * disc
			}
		}
	}
	for i := range input.Prepayments {
		pp := &input.Prepayments[i]
		if pp.PaymentStatus < types.InOutDefault || pp.StartDateStatus < types.InOutDefault ||
			pp.PerYrStatus < types.InOutDefault {
			continue
		}
		stop := pp.StopDate
		if pp.StopDateStatus < types.InOutDefault && pp.NNStatus >= types.InOutDefault && pp.NN > 0 {
			// AddNPeriods, not nn-1 iterated AddPeriod calls — CheckPrepayments
			// (AMORTOP.pas:416-419) uses the year-shortcut routine and the two
			// disagree for peryr=24 off-grid anchors (see CheckPrepaymentStops).
			if sd, e := dateutil.AddNPeriods(pp.StartDate, pp.PerYr, pp.NN-1); e == nil {
				stop = sd
			}
		}
		// DOS's FirstLastAndFF (Amortize.pas:370-375) verbatim:
		//
		//	first := exxp(-rate * YearsDif(pre[i]^.startdate, repay_from));
		//	last  := exxp(-rate * YearsDif(pre[i]^.stopdate,  repay_from));
		//	ff    := exxp(-rate / pre[i]^.peryr);
		//
		// Two details the port had wrong. The discount origin is repay_from, not
		// the loan date. And `ff` — the per-instalment discount factor of the
		// geometric sum — uses the SERIES' own frequency `pre[i]^.peryr`, not the
		// loan's: a weekly series inside a monthly loan steps 1/52 of a year per
		// instalment, so using 1/12 inflated (1-ff) by ~4x and understated the
		// series' present value by the same factor, leaving adjp — and the seed
		// built from it — far too high.
		ffPre, _ := interest.Exxp(-truerate / float64(pp.PerYr))
		// isLoanCalc=TRUE — see the balloon loop above; this is the iAMZ screen.
		first, _ := interest.Exxp(-truerate * dateutil.YearsDif(pp.StartDate, origin, settings.Basis, settings.YrInv, true))
		last, _ := interest.Exxp(-truerate * dateutil.YearsDif(stop, origin, settings.Basis, settings.YrInv, true))
		if dpTraceSeed {
			fmt.Fprintf(os.Stderr, "FLF i=%d first=%.17g last=%.17g ff=%.17g pay=%.17g peryr=%d\n",
				i+1, first, last, ffPre, pp.Payment, pp.PerYr)
			fmt.Fprintf(os.Stderr, "PRE i=%d adjpbefore=%.17g term=%.17g\n",
				i+1, adjp, pp.Payment*(first-last*ffPre)/(1-ffPre))
		}
		if math.Abs(1-ffPre) > teeny {
			adjp -= pp.Payment * (first - last*ffPre) / (1 - ffPre)
		} else if pp.NNStatus >= types.InOutDefault {
			adjp -= pp.Payment * float64(pp.NN)
		}
	}
	// NO non-positive guard. DOS's EstimateAndRefinePayment (Amortize.pas:384-401)
	// subtracts the balloon and prepayment present values from `adjp` and feeds the
	// result straight into `d := adjp*(f-1)/denom` — a dominating balloon, or a
	// prepayment series that starts BEFORE repay_from (whose exxp(-rate*YearsDif)
	// then amplifies rather than discounts), simply produces a NEGATIVE seed and
	// DOS iterates from there.
	//
	// The port used to fall back to the plain-annuity seed (factor 1) here, on the
	// theory that no regular payment can retire a "negative remainder" (advisory
	// A-W11). That reasoning is about the SCHEDULE; this value is only the SEED,
	// and Iterate is a bracket-free secant (AMORTOP.pas:1415-1495) on a terminal
	// that, for an option loan, is a sawtooth with several roots — so the seed does
	// not merely speed convergence, it SELECTS the root. Substituting a "sensible"
	// seed lands on a different root than DOS and produces a wholly different, and
	// wrong, schedule.
	//
	// Measured on fuzzer5 seed 8911 (exact + mor=128 + targ=235.26 + skip=1,7 +
	// b146/b192 + two prepayment series starting ~10 years before repay_from):
	// adjp = -14433.902981, DOS seed d = -202.319641, which the secant walks to
	// 172.562450 — DOS's own answer of 172.5624. The guarded seed +3739.103330
	// walked instead to 178.764017 (the terminal crosses zero near BOTH, with a
	// discontinuous drop between 176.5 and 177.0), overstating total interest by
	// 1141.19 over the 265-row schedule.
	return adjp
}

// dosSeedPayment computes DOS's base-payment seed in DOS's OWN shape — one
// additive `adjp` followed by ONE quotient (Amortize.pas:397-401):
//
//	denom := (1 - exxp(-nrepay * lnn(f)));
//	if (abs(denom) < teeny) then d := adjp / nrepay
//	else d := adjp * (f - 1) / denom;
//
// This exists to be BIT-identical, not merely mathematically equal. The port
// used to reconstruct the same value multiplicatively —
//
//	d := amount*(f-1)/(1-f^-N)      {estimatePayment}
//	d := d * (adjp/amount)          {dosSeedPVFactor}
//	d := d * (1-f^-N)/(1-f^-nrepay) {the nrepay swap}
//
// — which is algebraically identical and off by a couple of ULP, because it
// rounds five extra times and cancels (1-f^-N) against itself rather than never
// forming it. Two ULP in the seed is not a rounding curiosity here. DOS's
// Iterate is a BRACKET-FREE SECANT (AMORTOP.pas:1415-1495) on an option-loan
// terminal that is a sawtooth with several roots, and the terminal has FLAT
// PLATEAUS — regions where the probe and the seed evaluate to the same double.
// On a plateau the secant's slope is decided by the LAST BIT of the two
// terminal values, so a 2-ULP seed difference flips the sign of the slope,
// throws the next iterate to the far field (±1e17), and lands the walk in a
// different basin — or, when the slope comes out exactly 0, stalls it.
//
// Measured on fuzzer5 seed 20622:
//
//	amort_oracle 425820.45 0.1175910000 23 1 b365 prepaid r78 usa \
//	  loandmy=8.11.2025 firstdmy=8.11.2026 b36=73551.47 b204=98688.56 \
//	  pre=96:45:24:314.18 pre=144:42:26:121.02 adj=24:0.0944850000:50593.01 \
//	  adj=60:0.0577340000: adj=192:0.0648990000:51378.45 targ=9334.38 pts=0.031740
//
// DOS seeds at d = 44838.166556627104, the multiplicative port at
// 44838.166556627089. Rows 0-6 of the terminal walk are bit-identical because
// their payments are target-floored (`targ + interest`, no `d` in the formula);
// row 7 is the first row whose payment contains `d` (ComputeNext's balloonpos-0
// arm, `payamt - d + targ + interest`) and it diverges by 1.5e-11, which carries
// to a ~3-ULP difference in the terminal. DOS then read 397949.672…572 at the
// seed and …571 at the +0.1% probe; the port read them the other way round. The
// sign flip sent DOS's secant to +1.03e17 (whence the far-field probe cancels
// back to a convergent basin) and the port's to −1.24e17 — two different roots
// from the same worksheet.
//
// Returns ok=false when the exponentials overflow or nrepay cannot be derived,
// in which case the caller keeps its own estimate.
func dosSeedPayment(input LoanInput, loan *Loan, settings *Settings, f float64) (float64, bool) {
	nrepay := dosNrepay(input, loan)
	if nrepay <= 0 {
		return 0, false
	}
	adjp := dosSeedAdjP(input, loan, settings)
	lnf, e1 := interest.Lnn(f)
	if e1 != nil {
		return 0, false
	}
	ex, e2 := interest.Exxp(-float64(nrepay) * lnf)
	if e2 != nil {
		return 0, false
	}
	denom := 1 - ex
	var d float64
	if math.Abs(denom) < teeny {
		d = adjp / float64(nrepay)
	} else {
		d = adjp * (f - 1) / denom
	}
	if dpTraceSeed {
		tr, _ := ComputeTrueRate(loan, settings)
		fmt.Fprintf(os.Stderr, "SEED adjp=%.17g rate=%.17g f=%.17g nrepay=%d "+
			"denom=%.17g d=%.17g amount=%.17g loanrate=%.17g peryr=%d nper=%d\n",
			adjp, tr, f, nrepay, denom, d, loan.Amount, loan.LoanRate,
			loan.PerYr, loan.NPeriods)
	}
	return d, true
}

// dpTraceSeed dumps the base-payment seed in the same field order as the DOS
// `SEED` trace (an EstimateAndRefinePayment writeln patched into a diagnostic
// oracle build), so the two can be diffed term by term at full precision.
var dpTraceSeed = os.Getenv("DPTRACESEED") != ""

// prepaidMoratoriumEarlyExit reproduces DOS's EstimateAndRefinePayment
// early-exit (Amortize.pas:402-407) for a prepaid moratorium loan: the payment
// is the plain closed-form annuity `amount·(f−1)/(1−f^−nrepay)` over the
// AMORTIZING count, with NO Iterate refinement. `nrepay` is DOS's moratorium
// amortizing count `NumberOfInstallments(first_repay, lastdate, on_or_before)`
// (Amortize.pas:1302), where first_repay is first snapped ON_OR_AFTER to the
// payment grid (Amortize.pas:1261). Ported from legacy/src/dos_source/
// Amortize.pas. Returns ok=false when the count cannot be derived.
// snapMoratoriumFirstRepay pulls the moratorium's first-repayment date onto the
// payment grid, the way DOS does once in its screen prepass:
//
//	t := mor^.first_repay;                                  { save for comparison }
//	nrepay := NumberOfInstallments(h^.firstdate, mor^.first_repay,
//	                               h^.peryr, on_or_after);  { Amortize.pas:1263 }
//	if (DateComp(t, mor^.first_repay) <> 0) then
//	  mor^.first_repaystatus := defp;   { tell the user we moved it }
//
// `mor^.first_repay` is a VAR parameter, so that call REWRITES it, and every
// later reader — repay_from (:1267), the nrepay guard (:1302) and ComputeNext's
// interest-only test `DateComp(date, mor^.first_repay) < 0` (AMORTOP.pas:641/647)
// — sees the snapped date. See the long note at the Amortize call site for why
// the snap can move the date by a whole period rather than merely aligning it
// (NumberOfInstallments' `if (flast) then l.d := daysinm(l)` month-end arm).
//
// This lives in DOS's PrepareScreenForOutput at :1263, which runs BEFORE the
// EstimateAndRefine* solves it dispatches at :1333-1421. That ordering is the
// whole point of factoring it out here: Go's backward solvers are entered
// directly by the API handler, ahead of Amortize, so each of them has to do the
// snap itself or it solves against a moratorium boundary DOS never used.
//
// 2026-07-29, fuzzer5 backward-solve widening, `norate` arm:
//
//	amort_oracle 110863.16 0.0467900000 120 12 loandmy=28.1.2023 \
//	  firstdmy=28.2.2023 mor=3 payhard=1537.25 norate
//	DOS solvedrate 0.1081533781 | Go 0.1093337397
//
// firstdate 2/28/2023 IS February's last day, so flast holds and first_repay
// 4/28/2023 stretches to daysinm(April) = 4/30/2023 — past the 4/28 payment,
// which therefore stays interest-only. Amortize snapped (the forward walk at
// DOS's rate agreed to the cent); SolveRate did not, and its fancyTerminal calls
// generateFancyScheduleMode directly, so the rate solve inverted a schedule one
// amortizing period longer than the one it would go on to display. Go returned
// the same rate for 1/1/2024 and 28/1/2023 loan dates — date-independence that
// DOS does not have — which is what identified it.
//
// Idempotent: a date already on the grid re-snaps to itself, so it is safe to
// call on every entry point and at every nesting depth.
func snapMoratoriumFirstRepay(input *LoanInput) {
	if input.Moratorium.FirstRepayStatus < types.InOutDefault ||
		!dateutil.DateOK(input.Moratorium.FirstRepay) ||
		!dateutil.DateOK(input.Loan.FirstDate) || input.Loan.PerYr <= 0 {
		return
	}
	if _, snapped := dateutil.NumberOfInstallments(input.Loan.FirstDate,
		input.Moratorium.FirstRepay, int(input.Loan.PerYr),
		types.OnOrAfter); dateutil.DateOK(snapped) {
		input.Moratorium.FirstRepay = snapped
	}
}

func prepaidMoratoriumEarlyExit(loan Loan, s *Settings, mor Moratorium, f float64) (float64, bool) {
	if !dateutil.DateOK(loan.FirstDate) || !dateutil.DateOK(loan.LastDate) ||
		!dateutil.DateOK(mor.FirstRepay) {
		return 0, false
	}
	// Snap first_repay ON_OR_AFTER to the payment grid (the var-result date of
	// NumberOfInstallments is the snapped installment date).
	_, firstRepay := dateutil.NumberOfInstallments(loan.FirstDate, mor.FirstRepay,
		int(loan.PerYr), types.OnOrAfter)
	if !dateutil.DateOK(firstRepay) {
		firstRepay = mor.FirstRepay
	}
	// nrepay = amortizing installments from first_repay to lastdate, on_or_before.
	nrepay, _ := dateutil.NumberOfInstallments(firstRepay, loan.LastDate,
		int(loan.PerYr), types.OnOrBefore)
	if nrepay <= 0 {
		return 0, false
	}
	tmp := loan
	tmp.NPeriods = nrepay
	return estimatePayment(&tmp, f), true
}

func estimatePayment(loan *Loan, f float64) float64 {
	if math.Abs(f-1) < teeny {
		return loan.Amount / float64(loan.NPeriods)
	}
	numer := loan.Amount * (f - 1)
	lnf, _ := interest.Lnn(f)
	expVal, _ := interest.Exxp(-float64(loan.NPeriods) * lnf)
	denom := 1 - expVal
	if math.Abs(denom) < teeny {
		return loan.Amount / float64(loan.NPeriods)
	}
	return numer / denom
}

// generateSimpleSchedule builds the schedule for a non-fancy loan.
// MaxSchedulePeriods bounds how many payment rows the engine will generate.
// It matches the fancy engine's runaway-schedule guard (engine.go ~1090) and
// protects the simple-schedule path — and the API boundary — from an
// adversarial or fat-fingered period count (e.g. a billion) that would
// otherwise allocate unbounded memory. Legitimate loans stay well under it:
// even an 80-year weekly loan is ~4,160 periods.
const MaxSchedulePeriods = 10000

func generateSimpleSchedule(loan *Loan, payment float64, settings *Settings, truerate, f float64) AmortResult {
	var result AmortResult
	// Defense in depth: the API rejects oversized period counts up front, but
	// a count can also be derived from far-apart dates. Refuse to build a
	// schedule larger than the guard rather than churn memory.
	if loan.NPeriods > MaxSchedulePeriods {
		result.Err = fmt.Errorf("the schedule would have %d payments, more than the %d-payment maximum — check the term, payment frequency, and dates", loan.NPeriods, MaxSchedulePeriods)
		return result
	}
	p := loan.Amount
	var cumInt float64

	// hardPayment: the regular payment is a user-supplied "hard"
	// number (not solved by the engine). DOS the "Dav Holle
	// provision" — rounds per-period interest to whole cents in this
	// case so the schedule uses the standard penny treatment
	// (Amortize.pas:1483 `if hard_payment then Round2(interest)`).
	hardPayment := loan.PayAmtStatus == types.InOutInput

	currentDate := loan.FirstDate
	origDay := loan.FirstDate.Time.Day()

	// Semimonthly (24/yr) day-31 first date: DOS's schedule WALK never lands
	// on the 31st — its AddPeriod(24) round trip re-grids the row dates to the
	// 1st/16th (subtract: 31−15 → 16; add: 16+15 = 31 ≥ 31 → day 1 next month;
	// INTSUTIL.pas:1208-1237), while the SOLVE keeps the original date's
	// prorate (h^.firstdate itself is never rewritten). Re-grid only the walk
	// start here; all stable days (1, 15, 16, 29, 30, snap-window) round-trip
	// to themselves. 2026-07-12 pass-3 finding AF3 — verified vs the real DOS
	// engine:
	//
	//	amort_oracle 50000 0.10 26 24 loandmy=15.1.2024 firstdmy=31.1.2024 b365 rows
	//	→ payment 2033.5386 (original 16-day prorate — a global first-date
	//	  re-grid moved it to 2034.09, wrong), rows on 2/1, 2/16, …, row1 int
	//	  232.24 (17 actual days from 1/15; the 31st-anchored walk gave 208.33)
	if loan.PerYr == 24 {
		if back, e1 := dateutil.AddPeriod(currentDate, 24, origDay, true); e1 == nil {
			if rt, e2 := dateutil.AddPeriod(back, 24, origDay, false); e2 == nil &&
				dateutil.DateComp(rt, currentDate) != 0 {
				currentDate = rt
			}
		}
	}

	// Compute the natural start of the first regular period (one
	// period before FirstDate). When in prepaid mode and the loan
	// date precedes that natural start, emit a separate "row 0" for
	// the settlement-period interest. This mirrors DOS AMORTOP.pas:
	// PrepaidInterest is collected at closing and the schedule's
	// first regular row spans exactly one full period.
	//
	// Without this split, the first regular row's interest column
	// bundles the settlement-day interest into pmt #1, which
	// distorts the per-row breakdown even though totals match.
	prorate := 1.0
	// When the prepaid settlement stub is emitted, the first REGULAR period
	// runs naturalStart → FirstDate (repay_from := firstdate − 1 period,
	// Amortize.pas:1277-1281) — remembered here so the actual-day accrual
	// branches below anchor row 1 on the shifted start, not the loan date.
	hasStub := false
	var stubStart types.DateRec
	if settings.Prepaid && !settings.InAdvance {
		naturalStart, err := dateutil.AddPeriod(loan.FirstDate, loan.PerYr, origDay, true)
		if err == nil {
			if dateutil.DateComp(loan.LoanDate, naturalStart) < 0 {
				hasStub = true
				stubStart = naturalStart
				// Settlement stub: emit row 0.
				stubYd := dateutil.YearsDif(naturalStart, loan.LoanDate,
					settings.Basis, settings.YrInv, true)
				var stubInt float64
				if settings.Daily {
					expVal, _ := interest.Exxp(truerate * stubYd)
					stubInt = p * (expVal - 1)
				} else {
					stubInt = p * loan.LoanRate * stubYd
				}
				// MakeTable prints (and totals) the settlement line only when
				// PrepaidInterest > 0 — see settlementLinePrints.
				if settlementLinePrints(stubInt) {
					result.rawSettlement, result.hasRawSettlement = stubInt, true
					cumInt += stubInt
					result.Schedule = append(result.Schedule, PaymentRecord{
						PayNum:    0,
						Date:      loan.LoanDate,
						PayAmt:    stubInt,
						Interest:  stubInt,
						Principal: p,
						IntToDate: cumInt,
					})
					result.TotalPaid += stubInt
					result.TotalInt += stubInt
				}
				// First regular period is now exactly one period long.
				// NOT gated: DOS sets repay_from/prorate outside MakeTable.
				prorate = 1.0
			} else {
				// Loan starts on or after the natural start of the first regular
				// period, so no settlement stub is emitted — but `prepaid` is
				// still in force, and DOS's prepaid arm sets the first-period
				// length UNCONDITIONALLY:
				//
				//	if prepaid then
				//	  begin
				//	    repay_from := h^.firstdate;
				//	    AddPeriod(repay_from, h^.peryr, h^.firstdate.d, subtract);
				//	    prorate := 1;
				//	  end
				//
				// (Amortize.pas:1277-1282). There is no second test on the loan
				// date: reaching here at all means loanDate is not BEFORE the
				// natural start, and DOS clears `prepaid` outright when it is
				// strictly AFTER (:1252-1259, ported at engine.go:425-432), so
				// this branch is only reachable on the EXACT boundary loanDate ==
				// firstDate − 1 period — which is precisely the case DOS calls
				// "prepaid for free" and gives a first period of exactly 1.
				//
				// firstPeriodProrate is not 1 there on the 360 basis, because it
				// goes through YearsDif and INTSUTIL.pas's 30/360 body carries the
				// Feb clause `else if (a.m=2) and (a.d>27) then
				// til:=til-(30-a.d)/360` — so a Feb-28 pair measures 358/360 of a
				// year, not a whole one. 2026-07-26, amortization fuzzer5 seed
				// 20250 fallout — verified vs the real DOS engine:
				//
				//	amort_oracle 455080.06 0.0849120000 12 1 prepaid \
				//	  loandmy=28.2.2023 firstdmy=28.2.2024 rows
				//	→ payment 61932.5974, row 1 int 38641.76 (= amount·rate·1.0),
				//	  total interest 288111.11; the port prorated row 1 by
				//	  358/360 (38427.08, the NON-prepaid row-1 value) and the
				//	  0.9944 factor compounded down the whole schedule to
				//	  287584.94 — $526.17 light.
				//
				// The 365-basis regression this line was written to fix is
				// unaffected: there the whole-period answer IS 1, which is what
				// this now returns directly rather than by way of a helper.
				prorate = 1.0
			}
		}
	} else {
		// Non-prepaid mode: first period accrues interest for the
		// entire LoanDate → FirstDate span, possibly more than one
		// period. Month-based on clean boundaries (basis-independent),
		// actual days only for odd-day stubs — see firstPeriodProrate.
		prorate = firstPeriodProrate(loan.LoanDate, loan.FirstDate, loan.PerYr, settings)
	}

	// In-advance (annuity-due) prorate factor. When set, the payment
	// is made at the START of each period and interest accrues on the
	// post-payment balance: p = p + ff*(p-d) - d. This mirrors the
	// in-advance branch of RepayLoan (AMORTOP.pas: in_advance), so the
	// schedule and the closed-form solvers agree.
	inAdvanceFF := 0.0
	if settings.InAdvance {
		inAdvanceFF = (f - 1) / (2 - f)
	}

	// In-advance row 0: the FIRST period's interest is charged IN ADVANCE at the loan
	// date — a closing settlement the borrower pays at time 0 (DOS emits it as a
	// PayNum-0 row dated the loan date, with interest = p·(f-1)·prorate, the principal
	// unchanged; confirmed vs the oracle `dumpraw` L0). The per-payment rows below
	// then carry the SUBSEQUENT in-advance interest on the declining balance. The
	// simple in-advance path previously omitted this row, so the total interest/paid
	// was short by the settlement (≈$500 on a 100k/6% monthly) in EVERY in-advance
	// loan — the exact-in-advance path already emits the equivalent row 0. Validated
	// to the cent vs the DOS oracle: TestProductionInAdvanceBaseline.
	// MakeTable prints (and totals) the settlement line only when the settlement
	// interest is strictly positive — see settlementLinePrints.
	if settings.InAdvance {
		stubInt := p * (f - 1) * prorate
		if settlementLinePrints(stubInt) {
			result.rawSettlement, result.hasRawSettlement = stubInt, true
			if hardPayment {
				stubInt = interest.Round2(stubInt)
			}
			cumInt += stubInt
			result.Schedule = append(result.Schedule, PaymentRecord{
				PayNum:    0,
				Date:      loan.LoanDate,
				PayAmt:    stubInt,
				Interest:  stubInt,
				Principal: p,
				IntToDate: cumInt,
			})
			result.TotalPaid += stubInt
			result.TotalInt += stubInt
		}
	}

	// Rule-of-78 ("sum of the digits") interest allocation. The total
	// interest (n*payment - amount) is front-loaded: period k gets
	// interest proportional to (n+1-k). r78step is decremented from a
	// seed of r78step*(n+1) so the first period's interest is
	// n*r78step. Ported from Amortize.pas:1506-1530.
	//
	// R78 applies ONLY on the 360-day basis. DOS routes any non-360 basis
	// through RepayFancyLoan (Amortize.pas:1493: `… or (not (df.c.basis=x360))
	// then RepayFancyLoan`), the standard per-period walk that does NOT apply
	// the sum-of-digits split — so on the 365 basis R78 is silently a no-op and
	// the borrower gets ordinary amortization interest. Match that.
	// R78 takes PRECEDENCE over in-advance: DOS MakeTable checks `if df.c.r78`
	// BEFORE `if df.c.in_advance` in the simple loop (Amortize.pas:1507/1533),
	// so with both flags set the schedule is the sum-of-digits split SEEDED
	// from the in-advance-solved (annuity-due) payment, plus the in-advance
	// settlement row at the loan date. 2026-07-11 audit finding A4: the port
	// previously disabled R78 under in-advance (and skipped the annuity-due
	// payment refine), rendering the annuity-due schedule at the in-arrears
	// payment. Verified vs the real DOS engine:
	//
	//	amort_oracle 100000 0.10 24 12 r78 inadv dumpraw
	//	→ payment 4579.8857 (annuity-due; Go solved 4614.4926),
	//	  row 0: 1/1/24 int 833.33 (settlement), row 1: 793.38 (= n·r78base),
	//	  … row 24: 33.06, interest 10750.59.
	r78 := settings.R78 && loan.NPeriods > 0 &&
		settings.Basis == types.Basis360
	var r78step, r78int float64
	if r78 {
		// (The in-advance settlement row at the loan date — DOS PrepaidInterest,
		// 833.33 in the dumpraw above — is already emitted by the shared
		// in-advance pre-loop stub in this function; the R78 rows themselves
		// stay on the UNSHIFTED dates firstDate…lastDate.)
		n := float64(loan.NPeriods)
		r78step = (n*payment - loan.Amount) / (0.5 * n * (n + 1))
		r78int = r78step * (n + 1)
		// Hard payment: DOS Round2 is a VAR procedure applied to the RUNNING
		// accumulator — the seed (Amortize.pas:1511) and each period's value
		// (:1524-1528) — so the next period subtracts from the ROUNDED value.
		// 2026-07-11 pass-2 finding 7: rounding only the emitted copy drifts a
		// cent per row. Verified: `amort_oracle 10000 0.1237 24 12 r78
		// payhard=471.73 rows` → rows 2-4 int 101.31 / 96.90 / 92.49 (the
		// unrounded-accumulator port gave 101.32 / 96.91 / 92.51).
		if hardPayment {
			r78int = interest.Round2(r78int)
		}
	}

	// walkPeriods bounds the table by DOS's HORIZON as well as by its period
	// count. DOS's one table loop ends on
	//
	//	until … or (DateComp(WhenToStop^.date, stopdate) >= 0)   AMORTOP.pas:1221
	//
	// with stopdate = very_last (:1142), which for an option-free screen is
	// exactly h^.lastdate. Normally the two bounds coincide — the last period IS
	// the last date — and this clamp is inert. They come APART when the derived
	// last date WRAPPED: DOS stores a date's year in a byte, so a 300-year term
	// has its horizon truncated mod 256 (docs/discrepancies.md §55) and the
	// period count then overruns the date. Without the clamp the walk cycles the
	// calendar repeatedly instead of stopping. Verified vs the real DOS engine:
	//
	//	amort_oracle 421052.18 0.047119 7200 24 loandmy=15.4.2029 \
	//	    firstdmy=15.5.2029
	//	  → 1056 rows, interest 875474.53, final row 4/30/2073 absorbing
	//	    421875.06 of principal (Go emitted all 7200 rows and 5,542,481.87 of
	//	    interest, having wrapped through the calendar 28 times)
	//	amort_oracle intutil noi 2029 5 15 2073 4 30 24 on_or_before → n 1056
	//
	// COUNTED FROM THE ENDPOINTS, not tested row by row. A per-row
	// `DateComp(currentDate, lastDate) >= 0` is the literal transcription of
	// DOS's `until`, and it regresses any schedule whose walk dates have drifted
	// from DOS's — which §54 (no century leap correction, deferred) makes happen
	// to every day-29/30 grid crossing February 2100. Measured: the row-by-row
	// form truncated `amort_oracle 114948.20 0.025189 1080 12 loandmy=29.4.2029
	// firstdmy=29.5.2029 exact` one row early (interest 175844.03 vs DOS's
	// 175844.60) and was the single NEW divergence in that sweep. Counting off
	// NumberOfInstallments uses dateutil's DOS-faithful arithmetic on the two
	// endpoints, so a mid-walk drift cannot move the bound.
	// ROUND 21 CORRECTION — §62. `NumberOfInstallments` IS THE WRONG COUNTER
	// HERE, AND IT IS WRONG IN THE IN-SCOPE DIRECTION.
	//
	// It answers a DATE-DIFFERENCE question ("how many whole periods fit
	// between these two dates", INTSUTIL.pas:936), and DOS does not use it to
	// bound the table — DOS bounds the table by walking ITS OWN GRID and
	// comparing each payment date to stopdate. The two answers come apart
	// whenever the last payment date was CLAMPED, because the clamped date is
	// on the grid but is short of a whole period from the first date:
	//
	//	loandmy=29.2.2024 firstdmy=29.3.2024, monthly, n=12
	//	  last date, both engines:   28 Feb 2025    (day 29 clamped)
	//	  amort_oracle intutil noi 2024 3 29 2025 2 28 12 on_or_before
	//	                             -> n 11 last 2025 1 29
	//	  DOS's own table:           12 rows
	//	  the port, with the clamp:  11 rows, the 12th period's interest never
	//	                             charged and its principal folded into row 11
	//
	// A ROUTINE THAT IS FAITHFUL AT AN UNFAITHFUL SITE IS A DEFECT — §59's
	// lesson, one round later. `NumberOfInstallments` is a correct port of DOS's
	// `noi`; DOS simply never calls it here.
	//
	// The class is ordinary and entirely in scope: any loan anchored on the
	// 29th, 30th or 31st whose last payment lands in a February. Measured over a
	// probe of 2,736 in-scope screens (last payment 2097-2099, day 15/28/29/30,
	// all four frequencies): 66 divergences, ALL of them day 29 or 30 with a
	// February last date, and none at day 15 or 28.
	//
	// WHAT THE CLAMP WAS FOR, AND WHY THE REPLACEMENT KEEPS IT. Its only job is
	// the §55 year-byte WRAP: a 300-year term truncates its derived horizon mod
	// 256, so the period count overruns the date and an unbounded walk cycles
	// the calendar. Counting steps along the port's own payment GRID answers
	// that just as well — a wrapped last date is in the past, so the grid walk
	// stops at once — while agreeing with DOS on a clamped last date, because a
	// clamped date IS a grid point. Verified on the wrap case this clamp was
	// built for:
	//
	//	amort_oracle 421052.18 0.047119 7200 24 loandmy=15.4.2029 firstdmy=15.5.2029
	//	  -> 1056 rows, interest 875474.53, both engines, before and after.
	//
	// AND IT COSTS NOTHING PAST 2100 EITHER — WHICH IS THE §54 RESULT OF THIS
	// ROUND. The first version of this fix stepped the grid with
	// dateutil.AddPeriod, whose result is a types.DateRec; that re-exposed §54
	// (a DateRec cannot hold 29 Feb 2100, normalises to 1 March, and the next
	// step then reads March as its starting month and skips one), and the same
	// probe went from 22 to 406 out-of-scope divergences. Counting on RAW (y, m)
	// fields with DOS's own DaysInM removes that: the impossible date is never
	// materialised because a COUNTER only ever compares the day, never stores
	// it. Measured over the probe's full 4,560 screens — 2,736 in scope and
	// 1,824 past 2099, day 15/28/29/30, all four frequencies:
	//
	//	before this fix   66 in-scope divergences,  22 out-of-scope
	//	AddPeriod grid     0 in-scope divergences, 406 out-of-scope
	//	raw-field grid     0 and 0
	//
	// The general lesson, and it re-prices §54: the two-date-layer cost is real
	// only where the port must STORE a date DOS can form and Go cannot. Where it
	// only needs to COUNT or COMPARE, DOS's calendar is available through
	// dateutil at field level and the §51/§54 refactor is not required to get
	// the right answer. Verified unchanged on both boundary cases this bound has
	// ever been about:
	//
	//	421052.18 0.047119 7200 24 loandmy=15.4.2029 firstdmy=15.5.2029 (§55 wrap)
	//	114948.20 0.025189 1080 12 loandmy=29.4.2029 firstdmy=29.5.2029 exact (§54)
	walkPeriods := loan.NPeriods
	if loan.LastOK && dateutil.DateOK(loan.LastDate) && dateutil.DateOK(loan.FirstDate) {
		if n := installmentsOnPaymentGrid(loan.FirstDate, loan.LastDate,
			loan.PerYr, origDay, walkPeriods); n > 0 && n < walkPeriods {
			walkPeriods = n
		}
	}

	retired := false
	for i := 0; i < walkPeriods; i++ {
		lastPd := i == walkPeriods-1

		var intThisPd float64
		pmt := payment

		if r78 {
			// Sum-of-digits interest: declines by r78step each period. With a
			// hard payment the ACCUMULATOR itself is rounded (see the seed
			// comment above — DOS Amortize.pas:1524-1528).
			r78int -= r78step
			if hardPayment {
				r78int = interest.Round2(r78int)
			}
			intThisPd = r78int
			if lastPd {
				pmt = p + intThisPd
			}
			p = p + intThisPd - pmt
		} else if settings.InAdvance {
			// Payment made at the START of the period; interest accrues on
			// the post-payment balance (p - pmt). DOS charges this even on the
			// final period: the regular payment is used for the interest
			// calculation, then the actual final payment clears the remaining
			// balance plus that interest (AMORTOP.pas in_advance branch — the
			// final row carries (p-d)*f_1/(2-f), not zero). The p < pmt guard
			// clamps the near-payoff case where the balance is below the
			// regular payment.
			if p < pmt {
				intThisPd = 0
			} else {
				intThisPd = inAdvanceFF * (p - pmt)
			}
			if hardPayment {
				intThisPd = interest.Round2(intThisPd)
			}
			// Retire on the final scheduled period OR early when the regular
			// payment clears the balance — DOS's WhenToStop truncates an
			// OVERFUNDED in-advance loan too (the final row pays the remaining
			// balance with zero in-advance interest, since interest accrues on
			// the post-payment balance). 2026-07-12 pass-3 finding AF5 —
			// verified vs the real DOS engine:
			//
			//	amort_oracle 100000 0.09 120 12 payhard=1300 inadv rows
			//	→ 115 rows; row 115 int 0.00 prin 342.74 bal 0.00, paid
			//	  149292.74 (Go previously emitted all 120 rows with the
			//	  balance running to −6157.26)
			// Fold threshold is DOS's minpmt ($1.00), not zero: the simple loop
			// folds any sub-$1 post-payment balance into the current row
			// (`payment.principal < minpmt → payamt += principal`,
			// Amortize.pas:1546-1550). See dos_pennyfold_settlement_test.go.
			if lastPd || p+intThisPd-pmt < minPmt {
				pmt = p + intThisPd
				retired = true
			}
			p = p + intThisPd - pmt
		} else {
			if i == 0 {
				// First period may be short
				ff := 1 + (f-1)*prorate
				intThisPd = p * (ff - 1)
			} else {
				intThisPd = p * (f - 1)
			}

			if settings.Daily {
				// Daily compounding uses truerate and actual day count
				var prevDate types.DateRec
				if i == 0 {
					prevDate = loan.LoanDate
					if hasStub {
						prevDate = stubStart
					}
				} else {
					prevDate, _ = dateutil.AddPeriod(currentDate, loan.PerYr, origDay, true)
				}
				yd := dateutil.YearsDif(currentDate, prevDate, settings.Basis, settings.YrInv, true)
				expVal, _ := interest.Exxp(truerate * yd)
				intThisPd = p * (expVal - 1)
			} else if loan.PerYr == 26 || loan.PerYr == 52 || exactDaily(settings) ||
				(loan.PerYr == 24 && settings.Basis != types.Basis360) {
				// Weekly/biweekly, semimonthly on a non-360 basis, OR the exact
				// method on a non-360 basis: DOS routes every non-360 loan through
				// RepayFancyLoan, whose ComputeNext accrual is the
				// DaysCloseEnough-gated timedif (AMORTOP.pas:625-632) — actual
				// days for genuine partial spans, the whole-period fraction (with
				// the semimonthly ±half-month adjustment) for clean grid pairs.
				// periodYearFraction encodes exactly that gate.
				//
				// 2026-07-11 pass-2 finding 2: semimonthly non-360 rows previously
				// kept the constant p*(f-1), diverging on BOTH natural grids.
				// Verified vs the real DOS engine:
				//
				//	amort_oracle 10000 0.06 24 24 firstdmy=16.1.2024 b365 pay=429.7945 rows
				//	→ row2 int 25.17 (16 actual days; Go was 23.99), totals 314.56
				//	amort_oracle 10000 0.06 24 24 loandmy=15.1.2024 firstdmy=30.1.2024 b365
				//	→ row1 = whole 25.00 (1/24; Go prorated 24.59), totals 315.50
				//
				// Under prepaid with a settlement stub, row 1 anchors on the
				// SHIFTED start (repay_from = firstdate − 1 period) — the
				// loan→first span was already collected in row 0. Anchoring on
				// the loan date charged the whole span AGAIN and capitalized
				// it. 2026-07-12 pass-3 finding P3-F6 — verified vs the real
				// DOS engine:
				//
				//	amort_oracle 50000 0.11 52 26 b365 prepaid loandmy=1.1.2024 firstdmy=1.1.2027 dumpraw
				//	→ L0 settlement 16289.04, row1 (1/1/27) int 210.96 bal
				//	  49138.15, interest 22075.72 (Go's row1 was 16500.00 =
				//	  50000·0.11·3 on top of row 0 — totals 42260.55, ~2×)
				var prevDate types.DateRec
				if i == 0 {
					prevDate = loan.LoanDate
					if hasStub {
						prevDate = stubStart
					}
				} else {
					prevDate, _ = dateutil.AddPeriod(currentDate, loan.PerYr, origDay, true)
				}
				intThisPd = p * loan.LoanRate * periodYearFraction(prevDate, currentDate, loan.PerYr, settings)
			}

			if hardPayment {
				intThisPd = interest.Round2(intThisPd)
			}

			// Retire on the final scheduled period OR early when the regular
			// payment would clear/overshoot the balance (an over-amortizing loan —
			// e.g. the 365/360 basis, whose actual-day-prorated payment retires one
			// period before the nominal term). Fold the residual into this payment
			// and stop, exactly as DOS's WhenToStop does, instead of running extra
			// periods that produce a bogus negative-interest final row.
			// Threshold is DOS's minpmt ($1.00), not zero: the simple loop folds
			// any sub-$1 post-payment balance into the current row
			// (`payment.principal < minpmt → payamt += principal`,
			// Amortize.pas:1546-1550). See dos_pennyfold_settlement_test.go.
			if lastPd || p+intThisPd-pmt < minPmt {
				pmt = p + intThisPd
				retired = true
			}

			p = p + intThisPd - pmt
		}
		cumInt += intThisPd

		result.Schedule = append(result.Schedule, PaymentRecord{
			PayNum:    i + 1,
			Date:      currentDate,
			PayAmt:    pmt,
			Interest:  intThisPd,
			Principal: p,
			IntToDate: cumInt,
		})

		result.TotalPaid += pmt
		result.TotalInt += intThisPd

		// Stop once the loan has retired early (over-amortized), so no extra
		// negative-interest rows are emitted past payoff.
		if retired && !lastPd {
			break
		}

		// Advance date
		if !lastPd {
			nextDate, err := dateutil.AddPeriod(currentDate, loan.PerYr, origDay, false)
			if err != nil {
				result.Err = err
				return result
			}
			currentDate = nextDate
		}
	}

	result.FinalPrinc = p
	return result
}

// hasAnyAdvancedOption reports whether the loan carries any Advanced-Option
// feature (balloon, prepayment series, rate/payment adjustment, target,
// moratorium, or skip-months). The dedicated exact-in-advance schedule shape is
// only used when NONE of these are present — DOS's in-advance handling of
// balloons (AMORTOP.pas:1162-1176) and the other options remains the existing
// (documented) frontier behaviour.
func hasAnyAdvancedOption(input LoanInput) bool {
	if len(input.Balloons) > 0 || len(input.Prepayments) > 0 || len(input.Adjustments) > 0 {
		return true
	}
	if input.Target.TargetStatus >= types.InOutDefault {
		return true
	}
	if input.Moratorium.FirstRepayStatus >= types.InOutDefault {
		return true
	}
	return anySkip(input.SkipMonths.MonthSet)
}

// generateExactInAdvanceSchedule builds the schedule for an exact (true-daily)
// in-advance loan with no advanced options. DOS routes every non-360-basis loan
// through RepayFancyLoan; under the exact method on the in-advance (annuity-due)
// timing the schedule has a distinct SHAPE — NOT the ordinary in-advance schedule
// with daily accrual:
//
//   - a row-0 SETTLEMENT-interest row at the loan date: interest =
//     amount * rate * YearsDif(firstDate, loanDate), principal 0, balance
//     unchanged (the in-advance "time-0" interest, collected at closing).
//   - the base date is shifted one period later (AMORTOP.pas:1159-1177), so the
//     first amortizing row lands at firstDate + 1 period.
//   - n-1 amortizing rows, each accruing actual-day interest on the period's
//     opening balance (AMORTOP.pas:625 YearsDif branch via ComputeNext), with the
//     final row retiring the loan (WhenToStop folds the residual into it).
//
// The settlement row is emitted regardless of the prepaid flag — for in-advance
// DOS collects the time-0 interest either way (verified against the DOS oracle:
// prepaid on/off produce identical schedules). Ported from
// legacy/src/dos_source/AMORTOP.pas: RepayFancyLoan (in_advance branch).
// See docs/exact_groundzero_findings.md "Exact × in-advance structure".
func generateExactInAdvanceSchedule(input LoanInput, payment float64, settings *Settings) AmortResult {
	return generateExactInAdvanceScheduleMode(input, payment, settings, false)
}

// generateExactInAdvanceScheduleMode is generateExactInAdvanceSchedule with an
// UNFORCED option. When unforced is true it never folds the residual into the
// final row and never stops at early payoff: every period carries the regular
// payment and the balance is allowed to over-amortize negative, leaving the
// terminal (negative) balance in FinalPrinc. This is the stream DOS discounts in
// its APR present-value walk (RepayFancyLoan value_calc, Amortize.pas:553-556),
// as opposed to the truncated DISPLAY schedule.
func generateExactInAdvanceScheduleMode(input LoanInput, payment float64, settings *Settings, unforced bool) AmortResult {
	var result AmortResult
	loan := input.Loan
	if loan.NPeriods > MaxSchedulePeriods {
		result.Err = fmt.Errorf("the schedule would have %d payments, more than the %d-payment maximum — check the term, payment frequency, and dates", loan.NPeriods, MaxSchedulePeriods)
		return result
	}
	p := loan.Amount
	d := payment
	hardPayment := loan.PayAmtStatus == types.InOutInput
	origDay := loan.FirstDate.Time.Day()
	var cumInt float64

	// USA Rule exempt principal, exactly as in generateFancyScheduleMode. This
	// generator is a SHAPE specialization of DOS's RepayFancyLoan, not a
	// different engine: DOS builds the exact/in-advance schedule with the very
	// same ComputeNext (AMORTOP.pas:636-639, 656-661), so the USA-rule accrual
	// base and accumulator apply here too —
	//
	//	interest := h^.loanrate * timedif * (p - usap);
	//	...
	//	p := p + interest - payamt;
	//	if (df.c.USARule) then begin
	//	  usap := usap + interest - payamt;
	//	  if (usap < 0) then usap := 0;
	//	  end;
	//
	// Omitting it made `exact + inadv + usa` compound unpaid interest under
	// negative amortization instead of pinning the accruing base at the original
	// principal. Fuzzer5 cycle-24 seed 20329:
	//
	//	amort_oracle 26051.61 0.1247400000 42 2 b365 exact inadv usa \
	//	  loandmy=12.7.2025 firstdmy=12.1.2026 pts=0.016226 payhard=1527.01
	//	DOS int 68665.89 / paid 94717.50; port gave 81757.24 / 107808.85.
	//
	// Period interest (1624.84) exceeds the hard payment (1527.01), so the loan
	// negatively amortizes for its whole term and `p - usap` stays exactly at
	// 26051.61 — DOS's total is 42 accrual windows at the original principal
	// plus the points, which is what it reports. Reached both directly (the
	// !input.Fancy arm) and via generateFancyScheduleMode's delegation, so the
	// live accumulator is inherited from LoanInput.initUsap the same way a
	// segment solve inherits it there.
	usap := input.initUsap

	// Row 0: settlement interest at the loan date (the in-advance time-0 interest).
	// Simple actual-day interest over loanDate→firstDate; principal unchanged.
	// MakeTable prints (and totals) this line only when the settlement interest
	// is strictly positive — see settlementLinePrints.
	stubYd := dateutil.YearsDif(loan.FirstDate, loan.LoanDate, settings.Basis, settings.YrInv, true)
	stubInt := (p - usap) * loan.LoanRate * stubYd
	if settlementLinePrints(stubInt) {
		result.rawSettlement, result.hasRawSettlement = stubInt, true
		if hardPayment {
			stubInt = interest.Round2(stubInt)
		}
		cumInt += stubInt
		result.Schedule = append(result.Schedule, PaymentRecord{
			PayNum:    0,
			Date:      loan.LoanDate,
			PayAmt:    stubInt,
			Interest:  stubInt,
			Principal: p,
			IntToDate: cumInt,
		})
		result.TotalPaid += stubInt
		result.TotalInt += stubInt
	}

	// Amortizing rows: n-1 rows, each one period after the previous, the first at
	// firstDate + 1 period. prevDate starts at firstDate (the shifted base date).
	prevDate := loan.FirstDate
	curDate := loan.FirstDate
	for k := 1; k < loan.NPeriods; k++ {
		nd, err := dateutil.AddPeriod(curDate, loan.PerYr, origDay, false)
		if err != nil {
			result.Err = err
			return result
		}
		curDate = nd
		yd := periodYearFraction(prevDate, curDate, loan.PerYr, settings)
		intThisPd := (p - usap) * loan.LoanRate * yd
		if hardPayment {
			intThisPd = interest.Round2(intThisPd)
		}
		pmt := d
		// Final amortizing row retires the loan; an over-amortizing payment retires
		// early. DOS WhenToStop folds the residual into the payment. In unforced
		// (APR value) mode we never fold: the regular payment runs the full term.
		// The fold threshold is DOS's minpmt ($1.00), NOT zero: DOS folds any
		// remaining balance below minpmt into the CURRENT payment (`principal <
		// minpmt → payamt += principal; principal := 0`, AMORTOP.pas:1208-1211),
		// so a positive sub-$1 residual retires the loan on this row instead of
		// being dropped. 2026-07-22 manual-testing finding (penny-scale loan):
		// amort_oracle 1.15 0.03 5 12 b365 exact inadv prepaid payhard=0.29
		// loandmy=15.10.2026 firstdmy=1.12.2026 → one amortizing row (1/1/27)
		// with final payment 1.15; the port left balance 0.86 outstanding. See
		// dos_pennyfold_settlement_test.go.
		if !unforced && (k == loan.NPeriods-1 || p+intThisPd-pmt < minPmt) {
			pmt = p + intThisPd
		}
		p = p + intThisPd - pmt
		if settings.USARule {
			usap = usap + intThisPd - pmt
			if usap < 0 {
				usap = 0
			}
		}
		cumInt += intThisPd
		result.Schedule = append(result.Schedule, PaymentRecord{
			PayNum:    k,
			Date:      curDate,
			PayAmt:    pmt,
			Interest:  intThisPd,
			Principal: p,
			IntToDate: cumInt,
		})
		result.TotalPaid += pmt
		result.TotalInt += intThisPd
		prevDate = curDate
		if !unforced && p < minPmt && p > -minPmt && k < loan.NPeriods-1 {
			break
		}
	}
	result.FinalPrinc = p
	return result
}

// generateFancySchedule handles the full-featured amortization engine with
// balloons, adjustments, prepayments, moratoria, targets, and skip months.
//
// This is a simplified port of RepayFancyLoan that generates the schedule
// directly rather than printing to screen. The core payment-by-payment
// logic is preserved.
// generateFancySchedule renders the DISPLAY schedule (forced final row) — DOS's
// RepayFancyLoan with a non-nil Output. The payment/rate solvers instead use the
// UNFORCED terminal via fancyTerminal (RepayFancyLoan with Output=nil).
func generateFancySchedule(input LoanInput, payment float64, settings *Settings, truerate, f float64) AmortResult {
	return generateFancyScheduleMode(input, payment, settings, truerate, f, false)
}

// fancyTerminal returns the UNFORCED terminal balance of the fancy schedule at
// regular payment x — the Newton residual DOS's Iterate drives to zero
// (RepayFancyLoan called with Output=nil, AMORTOP.pas:1437). DOS does NOT force the
// final row and STOPS as soon as the running balance drops below minpmt
// (one-sided: WhenToStop.principal < minpmt, AMORTOP.pas:1195) or the schedule
// reaches very_last. That one-sided stop keeps the residual a continuous, monotone
// function of x. Positive ⇒ under-amortized (still owes); negative ⇒ over-amortized.
func fancyTerminal(input LoanInput, x float64, settings *Settings, truerate, f float64) float64 {
	in := input
	in.Loan.PayAmtStatus = types.InOutDefault
	in.Loan.PayAmt = x
	// Iterate's walk NEVER re-amortizes at adjustments: DOS's Re_Amortize gate
	// is `((next_adj <= adjnum) or entire)` (AMORTOP.pas:1215), and Iterate
	// calls RepayFancyLoan with entire=til_adj=FALSE and adjnum=0
	// (:1439/:1465 via Amortize.pas:416/460/477) — so the payment, amount,
	// and rate solves see a walk with NO rate changes and NO payment
	// re-solves; the ARM only shapes the DISPLAY (and the balloon-amount /
	// APR walks, which run with entire/value_calc and keep their
	// adjustments). 2026-07-12 pass-3 findings P3-F4/F5 — verified vs the
	// real DOS engine (the solved value is IDENTICAL with and without the
	// adjustment):
	//
	//	amort_oracle 100000 0.08 120 12 adj=24:0.09:1100 → payment 1213.2759 (= plain)
	//	amort_oracle 0 0.08 120 12 b365 exact adj=24:0.10: pay=1213.0959 noamt
	//	→ solvedamount 99999.9973 (Go's adj-aware terminal wandered a flat
	//	  residual to 26824.60; rate solve returned −0.98)
	//	amort_oracle 100000 0 120 12 b365 adj=24:0.10: pay=1213.2759 norate
	//	→ solvedrate 0.0799999918 (Go drifted 2.5e-5)
	// ...but ONLY when the caller is Iterate-like. DOS's Re_Amortize gate is
	//
	//	else if ((next_adj <= adjnum) or entire) and ... then Re_Amortize(p);
	//	                                                   { AMORTOP.pas:1215-1216 }
	//
	// so `entire` ALONE enables the re-amortizations. Iterate passes
	// entire=til_adj=FALSE (:1439/:1465), which is what the note above describes;
	// but DetermineLastPaymentDate's residual probe passes `entire` (:1344), so on
	// that path the adjustments MUST stay. Stripping them unconditionally measured
	// the residual on the adjustment-free loan, which refuses far sooner — so a
	// screen DOS answers got Go's refusal imported from a different loan. Measured
	// on
	//	amort_oracle 150000 0.09 120 12 loandmy=15.3.2024 firstdmy=15.4.2024 \
	//	  adj=36:0.06:1400 payhard=1250 noterm
	// DOS solves term 183 (last 2039-6-15); with the adjustment stripped DOS
	// ITSELF refuses that screen, which is exactly the wrong answer Go was
	// reporting. Caught by the 2026-07-30 DetermineLastPaymentDate audit at ~0.9%
	// of noterm-mode cases, every occurrence carrying an `adj` token.
	if !in.entireWalk {
		in.Adjustments = nil
	}
	// The walk is bounded by veryLast (= loan.LastDate). When a solver calls this
	// directly (not via Amortize), FirstPass has not derived LastDate from NPeriods,
	// so derive it here: LastDate = FirstDate + (NPeriods-1) periods.
	// The anchor is FirstDate's own day EXCEPT for a segment sub-loan, which
	// carries DOS's phantom snapped day (see LoanInput.gridAnchorDay). DOS's own
	// very_last is `h^.lastdate` (AMORTOP.pas:1300) — the WHOLE screen's last
	// date, stepped off the original firstdate's day — so a sub-walk anchored on
	// the phantom day must derive the same bound, or it trips the veryLast exit
	// on a date the real engine never reaches.
	if !in.Loan.LastOK && in.Loan.NPeriods > 0 && dateutil.DateOK(in.Loan.FirstDate) {
		day := in.anchorDayFor(&in.Loan)
		last := in.Loan.FirstDate
		for k := 1; k < in.Loan.NPeriods; k++ {
			if nd, e := dateutil.AddPeriod(last, in.Loan.PerYr, day, false); e == nil {
				last = nd
			}
		}
		in.Loan.LastDate = last
		in.Loan.LastOK = true
	}
	res := generateFancyScheduleMode(in, x, settings, truerate, f, true)
	if dpTraceTermRows {
		fmt.Fprintf(os.Stderr, "TERMROWS x=%.17g final=%.17g rows=%d\n",
			x, res.FinalPrinc, len(res.Schedule))
		for i, r := range res.Schedule {
			fmt.Fprintf(os.Stderr, "TERMROW %3d %s pay=%.17g int=%.17g bal=%.17g\n",
				i, r.Date.Time.Format("2006-1-2"), r.PayAmt, r.Interest, r.Principal)
		}
	}
	return res.FinalPrinc
}

// dpTraceTermRows dumps every row of the UNFORCED Newton terminal walk. Set
// DPTRACETERMROWS=1 with a narrow -run filter; it is extremely verbose.
var dpTraceTermRows = os.Getenv("DPTRACETERMROWS") != ""

// runAdjustmentPrePass reproduces DOS's pre-table adjustment solve followed by
// the Dav Holle Round2 sweep (Amortize.pas:1408-1436), writing the ROUNDED
// solved amounts back onto input.Adjustments with AmtOK set so the caller's
// walk consumes them through Re_Amortize's reuse branch (AMORTOP.pas:1514)
// rather than re-solving each one inline off an already-rounded balance path.
// See LoanInput.inAdjPrePass for why the ordering is load-bearing.
// runAdjustmentPrePass returns the month-end snap DOS's Re_Amortize leaves in
// h^.lastdate when the pre-pass itself fires (task #94). The zero DateRec means
// no snap happened. The caller must apply it immediately AFTER determineVeryLast,
// mirroring DOS's ordering: DetermineVeryLast runs at Amortize.pas:1320 and the
// adjustment prepass at :1405-1416, so the walk horizon is built from the
// UN-snapped date while every later reader — including the Last Pmt Date cell —
// sees the snapped one.
func runAdjustmentPrePass(input LoanInput, payment float64, settings *Settings, truerate, f float64, hardPayment bool) types.DateRec {
	// Only the segment-solve path (solveSegmentPayment) yields a value the
	// sweep can round — DOS's own gate is `(user_nballoons > 0) or (npre > 0) or
	// ((exact) and (basis<>x360))` (AMORTOP.pas:1571). Without it Re_Amortize
	// keeps the plain annuity seed, which the sweep never sees, so the ordering
	// is moot and the extra walk would be pure cost.
	if len(input.Balloons) == 0 && len(input.Prepayments) == 0 && !exactDaily(settings) {
		return types.DateRec{}
	}
	// DOS's loop is UNCONDITIONAL over every adjustment that carries a rate but
	// no amount (Amortize.pas:1408-1414):
	//
	//	for i := 1 to nadj do
	//	  if (adj[i]^.loanratestatus >= defp) and (adj[i]^.amountstatus < defp) then
	//	    if (not EstimateAndRefineAdjPayment(i)) then exit;
	//
	// There is no "at least two" threshold. An earlier `blank < 2` shortcut here
	// rested on the premise that a lone blank adjustment "walks the same path
	// either way" — false whenever the DISPLAY walk stops short of the
	// adjustment but the entire=true APR value walk reaches it. Seed 20509 is
	// exactly that: a 192-row display schedule with a rate-only adjustment at
	// row 204. DOS's dedicated til_adj pre-pass solves it from the loan date and
	// stores -46710.154181; the APR value walk then reuses that. With the
	// pre-pass skipped, Go let the value walk solve it inline off a different
	// path and got -43782.45 — 2927.70 per payment adrift, an ~876 error in the
	// APR seed value, enough to steer the secant off DOS's overflow path so Go
	// produced a schedule where DOS refused the screen.
	//
	// The predicate is DOS's: the rate is present and the amount is blank. The
	// old test read DateStatus, which is not a field DOS consults here.
	blank := 0
	for i := range input.Adjustments {
		a := &input.Adjustments[i]
		if a.LoanRateStatus >= types.InOutDefault && a.AmountStatus < types.InOutDefault {
			blank++
		}
	}
	if blank == 0 {
		return types.DateRec{}
	}

	pre := input
	pre.inAdjPrePass = true
	// DOS's pre-pass is RepayFancyLoan(p, usap, h^.loandate, h^.firstdate, nil,
	// false, til_adj, no_value_calc, adjnum) — Output=nil and entire=FALSE, i.e.
	// the UNFORCED walk, whichever mode the caller happens to be in. The stored
	// amounts are then shared by the display walk AND the later APR value walk,
	// exactly as adj[]^.amount is in DOS.
	pre.entireWalk = false
	// The pre-pass must not publish its RAW secant roots onto the caller's rows;
	// only the rounded harvest below may. Give it private copies.
	pre.Adjustments = append([]RateAdjustment(nil), input.Adjustments...)
	pre.Balloons = append([]BalloonPayment(nil), input.Balloons...)
	preRes := generateFancyScheduleMode(pre, payment, settings, truerate, f, true)
	for i := range input.Adjustments {
		dst := &input.Adjustments[i]
		src := &pre.Adjustments[i]
		if dst.AmtOK || !src.AmtOK {
			// Not solved by the pre-pass (the unforced walk paid the loan off
			// before reaching it, exactly as DOS's bounded pre-pass would). Leave
			// it blank; the caller's walk still solves it inline.
			continue
		}
		// `if (hard_payment) and (fancy) then begin
		//    for i:=1 to nadj do Round2(adj[i]^.amount); ... end;`
		// — the sweep itself (Amortize.pas:1430-1435). Only the AMOUNT: DOS
		// never touches adj[i]^.loanrate, so an AO6 solved rate stays raw.
		//
		// The sweep is hard_payment-gated; the SOLVE LOOP above it
		// (Amortize.pas:1408-1417) is not. Before 2026-07-31 round 9 the whole
		// pre-pass was gated on hardPayment, which conflated the two — see the
		// call site. When the payment is solved rather than hard, DOS still
		// stores the raw secant root that Re_Amortize left on the row
		// (AMORTOP.pas:1579-1581 / :1590-1591), unrounded.
		dst.Amount = src.Amount
		if hardPayment {
			dst.Amount = interest.Round2(src.Amount)
		}
		dst.AmountStatus = types.InOutOutput
		dst.AmtOK = true
	}
	return preRes.reAmortLastDate
}

func generateFancyScheduleMode(input LoanInput, payment float64, settings *Settings, truerate, f float64, unforced bool) AmortResult {
	// Exact (true-daily) in-advance with no advanced options has a distinct DOS
	// schedule SHAPE (settlement row + one-period base shift + n-1 amortizing
	// rows) that the general per-period walk below does not produce. Route it to
	// the dedicated generator. Advanced options keep the existing behaviour.
	// Gate on Exact && InAdvance at ANY basis: DOS applies the shape at 360 too
	// (pass-2 finding 5); the generator's YearsDif accrual degrades to whole
	// 30/360 months there, exactly like DOS.
	if settings.Exact && settings.InAdvance && !hasAnyAdvancedOption(input) {
		return generateExactInAdvanceScheduleMode(input, payment, settings, unforced)
	}

	var result AmortResult
	loan := input.Loan
	p := loan.Amount
	d := payment
	var cumInt float64
	// USA Rule exempt principal. Normally 0 at the start of a walk, but a
	// SEGMENT solve inherits the live accumulator from the boundary row: DOS's
	// Re_Amortize calls `Iterate(p, usap, ...)` with the unit-level global
	// (AMORTOP.pas:1577), Iterate saves it as `initusa` (:1436) and restores it
	// before every trial walk (:1457). See LoanInput.initUsap.
	usap := input.initUsap
	var negRateNoted bool // A-W12 emitted once if an AO6 adjustment implies a negative rate

	// hardPayment: a user-supplied regular payment triggers the DOS
	// "Dav Holle provision" — per-period interest is rounded to whole
	// cents (AMORTOP.pas:637 `if hard_payment then Round2(interest)`).
	hardPayment := loan.PayAmtStatus == types.InOutInput

	// The Dav Holle adjustment pre-pass (Amortize.pas:1408-1436). Solve every
	// blank adjustment amount along the UNROUNDED path first, apply the Round2
	// sweep once, and let the walk below consume the stored values through the
	// AmtOK branch — exactly DOS's ordering. See LoanInput.inAdjPrePass for the
	// full derivation and the seed-20431 evidence.
	//
	// ROUND 9 (2026-07-31, discrepancies §50). The old gate here was
	// `!input.inAdjPrePass && hardPayment`, which conflated DOS's two distinct
	// steps. The SOLVE LOOP is unconditional —
	//
	//	for i := 1 to nadj do
	//	  if (adj[i]^.loanratestatus >= defp) and (adj[i]^.amountstatus < defp) then
	//	    if (not EstimateAndRefineAdjPayment(i)) then exit;
	//	                                       {Amortize.pas:1408-1412}
	//
	// — while only the Dav Holle Round2 sweep that follows it is hard-payment
	// gated (`if (hard_payment) and (fancy)`, Amortize.pas:1430-1435). The
	// hardPayment test therefore belongs INSIDE runAdjustmentPrePass, on the
	// rounding, not on whether the pre-pass runs at all. With the pre-pass
	// skipped, a solved-payment screen walked the display pass with a pristine
	// h^.lastdate where DOS had already snapped it, so every adjustment's
	// NumberOfInstallments counted one period short.
	var prePassLastSnap types.DateRec
	if !input.inAdjPrePass {
		prePassLastSnap = runAdjustmentPrePass(input, payment, settings, truerate, f, hardPayment)
	}

	// DetermineVeryLast (AMORTOP.pas:1293-1304). Extracted to
	// determineVeryLast so TackOnFinalBalloon (tackon.go) resolves the
	// terminating-balloon date from the identical rule.
	veryLast := determineVeryLast(&loan, input.Balloons, input.Prepayments)

	// Task #94, the pre-pass half. `payhard=` makes hardPayment true, so the
	// pre-pass above pre-solves the blank adjustment amount and sets AmtOK; the
	// display walk then takes the hasAmt branch and never reaches the
	// payment-solve site that performs the snap. In DOS the pre-pass's OWN
	// Re_Amortize had already mutated the h^.lastdate global, so the snap is
	// visible either way. Applied HERE — after determineVeryLast, before the walk
	// — because that is exactly DOS's ordering (:1320 then :1405): the horizon is
	// built from the un-snapped date, so the schedule does not move, and only the
	// reported cell changes.
	if dateutil.DateOK(prePassLastSnap) {
		result.reAmortLastDate = prePassLastSnap
	}

	// adjLastDate is the piecewise engine's stand-in for DOS's h^.lastdate
	// GLOBAL *as seen by AMORTOP.pas:1547 only*, i.e. the `var l` argument of
	//
	//	n := NumberOfInstallments(adj[next_adj]^.date, h^.lastdate, h^.peryr, on_or_after);
	//
	// That call is unguarded (contrast Amortize.pas:1301-1304, which brackets
	// its own call with save_last/restore), so INTSUTIL.pas:1003+1018 write the
	// month-end snap straight back into the global and EVERY LATER Re_Amortize
	// counts against the snapped date. With two adjustments, adjustment 2's `n`
	// is derived from the date adjustment 1 snapped — that is the whole
	// mechanism of discrepancies §50, and the piecewise engine had no carrier
	// for it (it recorded the snap into result.reAmortLastDate, a DISPLAY cell,
	// and re-read the pristine loan.LastDate on the next adjustment).
	//
	// THE TRAP, and why this is a separate variable rather than loan.LastDate.
	// Writing the snap back into loan.LastDate was tried on 2026-07-30 and moved
	// a schedule by 6838.28 (amort_oracle 252424.20 0.1170790000 44 2 prepaid usa
	// … → DOS int=495329.73, Go int=502168.01, first divergence row 385/552).
	// The piecewise walk reads loan.LastDate at sites where DOS reads very_last —
	// the two engines' readers are NOT 1:1, and only the STRUCTURAL port has its
	// horizon (e.veryLast) pinned before the walk, which is why round 8's fix
	// could safely mutate e.loan.LastDate there and this one cannot. See
	// claude/lost_session_recovery_and_reamortize_correction_2026-07-30.md for
	// the asymmetric site model.
	//
	// Seeded from the pre-pass snap for the same reason round 8's structural fix
	// carries it: DOS's pre-pass (Amortize.pas:1408-1417) runs its own
	// Re_Amortize before the display walk, so the walk's FIRST adjustment
	// already counts against a snapped global.
	adjLastDate := loan.LastDate
	if dateutil.DateOK(prePassLastSnap) {
		adjLastDate = prePassLastSnap
	}

	// Normally FirstDate's own day, but a segment sub-loan carries DOS's phantom
	// snapped day (INTSUTIL.pas:1013 assigns `l.d := f.d` with no clamp, and
	// AMORTOP.pas:1150-1152 anchors the whole sub-walk on it). See
	// LoanInput.gridAnchorDay.
	origDay := input.anchorDayFor(&loan)
	// §67 — DOS DOES NOT USE THE TYPED FirstDate AS THE FIRST PAYMENT DATE.
	// RepayFancyLoan anchors the walk one period BEFORE FirstDate and lets
	// ComputeNext step forward onto it:
	//
	//	t := firstdate;                                  { AMORTOP.pas:1148 }
	//	AddPeriod(t, h^.peryr, firstdate.d, subtract);   { AMORTOP.pas:1150 }
	//	...
	//	    AddPeriod(t, h^.peryr, firstdate.d, add);    { AMORTOP.pas:1165 }
	//
	// For every frequency but semi-monthly that round trip is the identity, so
	// the port's old `currentDate := loan.FirstDate` agreed with DOS by
	// accident. AddPeriod's peryr=24 branch is the ONLY one carrying a
	// `d >= 31` normalization (INTSUTIL.pas:1216-1237), which makes it the only
	// branch that is NOT invertible: 31 Oct steps back to 16 Oct, and 16+15=31
	// trips `if (d>=31)` on the way forward, landing on 1 Nov. peryr 26/52 step
	// Julian +/- whole days and peryr 1..12 assign `d := orig_day`; both invert
	// exactly.
	//
	// Reproduced against the real DOS engine on first-payment days 15/28/29/30/31
	// at peryr 24, with peryr 26 and 12 controls — 5/5 including all four days
	// that do NOT move (docs/discrepancies.md §67). Ported as the ROUND TRIP and
	// not as `if day == 31`: DOS does not special-case the day, and a clamp
	// would drift on every input the sweep did not draw (CLAUDE.md, "replicate
	// the DOS logic; do NOT patch around a divergence").
	currentDate := loan.FirstDate
	if back, err := dateutil.AddPeriod(loan.FirstDate, loan.PerYr, origDay, true); err == nil {
		if fwd, err2 := dateutil.AddPeriod(back, loan.PerYr, origDay, false); err2 == nil {
			currentDate = fwd
		}
	}
	prevDate := loan.LoanDate

	// Handle the first-period interest accrual window.
	//
	// In prepaid mode the borrower pays settlement-day interest at
	// closing and the first regular payment then covers exactly one
	// full period. Compute the "natural" start of that first full
	// period — one period before FirstDate.
	//
	// Two situations to distinguish (DOS AMORTOP.pas handles both
	// via PrepaidInterest + a normalized first period):
	//   (A) LoanDate < naturalStart: there is a settlement stub from
	//       LoanDate to naturalStart. Emit a row 0 for that stub
	//       (interest only, balance unchanged) and run the first
	//       regular period from naturalStart to FirstDate.
	//   (B) LoanDate >= naturalStart: the loan starts within the
	//       first regular period (e.g. quarterly loan that closes
	//       only one month before the first quarterly payment). No
	//       stub; the first regular period runs from LoanDate to
	//       FirstDate and will accrue less than a full period of
	//       interest — the day-count-based formula handles this
	//       naturally as long as prevDate stays at LoanDate.
	if settings.Prepaid && !settings.InAdvance {
		naturalStart, err := dateutil.AddPeriod(loan.FirstDate, loan.PerYr, origDay, true)
		if err == nil {
			if dateutil.DateComp(loan.LoanDate, naturalStart) < 0 {
				// Case A: emit settlement-period row 0.
				stubYd := dateutil.YearsDif(naturalStart, loan.LoanDate,
					settings.Basis, settings.YrInv, true)
				var stubInt float64
				if settings.Daily {
					expVal, _ := interest.Exxp(truerate * stubYd)
					stubInt = p * (expVal - 1)
				} else {
					stubInt = p * loan.LoanRate * stubYd
				}
				// MakeTable prints (and totals) the line only when the
				// settlement interest is > 0 — see settlementLinePrints.
				if settlementLinePrints(stubInt) {
					result.rawSettlement, result.hasRawSettlement = stubInt, true
					cumInt += stubInt
					result.Schedule = append(result.Schedule, PaymentRecord{
						PayNum:    0,
						Date:      loan.LoanDate,
						PayAmt:    stubInt,
						Interest:  stubInt,
						Principal: p,
						IntToDate: cumInt,
					})
					result.TotalPaid += stubInt
					result.TotalInt += stubInt
				}
				// NOT gated: DOS's repay_from shift lives outside MakeTable.
				prevDate = naturalStart
			} else {
				// Case B: short first period; prevDate stays as
				// LoanDate so yd accurately captures the partial
				// period.
				prevDate = loan.LoanDate
			}
		}
	}

	// In-advance (annuity-due) FANCY loans have a distinct DOS schedule SHAPE,
	// verified row-for-row against the real DOS oracle (dumpraw). DOS
	// RepayFancyLoan (AMORTOP.pas:1159-1187) for an in-advance loan:
	//   - emits a row-0 SETTLEMENT-interest row at the loan date: interest =
	//     amount * rate * <one period>, principal 0, balance unchanged (the
	//     annuity-due "time-0" interest collected at closing), then
	//   - shifts the base date one period forward (AddPeriod(t,...,add)) so the
	//     first amortizing payment lands at FirstDate + 1 period, and
	//   - accrues ORDINARY opening-balance interest on the shifted walk — NOT
	//     the annuity-due (p-d)(f-1)/(2-f) factor: ComputeNext uses plain
	//     `loanrate * timedif * (p - usap)` even in-advance (AMORTOP.pas:636).
	// The advanced-option hooks (skip, moratorium, balloon, prepayment, target)
	// then run against this shifted walk exactly as in arrears mode — the
	// opening-balance interest the main loop already computes below (line ~1582)
	// is the correct DOS value, so NO post-payment recompute is applied.
	//
	// This replaces the former post-payment interest-recompute approximation
	// that left the bounded in-advance × fancy corner (docs/dos_known_frontier.md
	// #38). Daily (continuously-compounded) in-advance is not reshaped here — it
	// keeps its existing handling. Ported from
	// legacy/src/dos_source/AMORTOP.pas: RepayFancyLoan (in_advance branch).
	inAdvanceFancy := settings.InAdvance && !settings.Daily
	if inAdvanceFancy {
		// Settlement interest uses ACTUAL-DAY YearsDif (DOS PrepaidInterest,
		// AMORTOP.pas:182 amount*loanrate*YearsDif(firstdate,loandate)) — NOT the
		// month-based clean-boundary shortcut the amortizing rows use. On the 360
		// basis the two agree (30/360 whole month = 1/12); on 365 the settlement
		// stub is actual-day (e.g. Jan = 31/366) while a clean whole-month
		// amortizing period stays month-based. Matches generateExactInAdvanceSchedule.
		stubYd := dateutil.YearsDif(loan.FirstDate, loan.LoanDate, settings.Basis, settings.YrInv, true)
		stubInt := p * loan.LoanRate * stubYd
		// MakeTable prints (and totals) this line only when PrepaidInterest > 0
		// — see settlementLinePrints. The base shift below is NOT gated: it
		// lives in RepayFancyLoan, not MakeTable.
		if settlementLinePrints(stubInt) {
			result.rawSettlement, result.hasRawSettlement = stubInt, true
			if hardPayment {
				stubInt = interest.Round2(stubInt)
			}
			cumInt += stubInt
			result.Schedule = append(result.Schedule, PaymentRecord{
				PayNum:    0,
				Date:      loan.LoanDate,
				PayAmt:    stubInt,
				Interest:  stubInt,
				Principal: p,
				IntToDate: cumInt,
			})
			result.TotalPaid += stubInt
			result.TotalInt += stubInt
		}
		// Shift the base one period: the first amortizing period accrues from
		// FirstDate (the settlement row already covered LoanDate→FirstDate) and
		// the first payment date is FirstDate + 1 period.
		if shifted, err := dateutil.AddPeriod(loan.FirstDate, loan.PerYr, origDay, false); err == nil {
			currentDate = shifted
			prevDate = loan.FirstDate
		}
	}

	nextBalloon := 0 // index into sorted balloons

	// DOS in-advance first-balloon fold (AMORTOP.pas:1161-1186). In in-advance
	// mode DOS folds a balloon dated ON the first payment date into the closing
	// payment: `firstd := balloon[1].amount + d; AddPeriod(t,+)` and then
	// `if in_advance and nballoons>0 and DateComp(balloon[1].date,firstdate)<=0
	// then inc(next_balloon)` — the leading balloon is consumed. That folded
	// `firstd` is vestigial: DOS never reads nextpayamt back into the balance, so
	// the balloon exerts NO principal effect on the in-advance schedule; its only
	// mark is having flipped the loan into fancy mode. A balloon AFTER firstDate
	// still amortizes normally (it is not balloon[1] at the first-date test).
	//
	// The shifted base above already lands the first amortizing row at
	// FirstDate+1period, so Go otherwise drained this balloon as an OFF-CYCLE
	// extra at FirstDate (before currentDate) and mis-applied its full amount as
	// principal — over-amortizing the trial schedule and driving the Amount solve
	// to the wrong root. Consuming it here reproduces DOS. Verified vs the oracle:
	//
	//	amort_oracle 275069.64 0.1174620000 20 2 b365_360 exact prepaid inadv \
	//	  b6=260983.21 payhard=7796.40 noamt → solvedamount 87264.05 (the plain
	//	  20-row schedule, $260,983 balloon dropped); b7=.. onward DO amortize.
	if inAdvanceFancy && len(input.Balloons) > 0 &&
		input.Balloons[0].DateStatus >= types.InOutDefault &&
		dateutil.DateComp(input.Balloons[0].Date, loan.FirstDate) == 0 {
		nextBalloon = 1
	}

	// prepayApplied[i] counts how many extra payments prepayment
	// series i has applied so far. Used to honor Prepayment.NN — a
	// series specified as "NN extra payments" must stop after NN
	// extras even when no StopDate was given. See dispatch_gaps AO8 /
	// CLAUDE.md outstanding item #4.
	prepayApplied := make([]int, len(input.Prepayments))

	// nextDates[i] is the running "next extra due" cursor for prepayment
	// series i. It is kept LOCAL rather than written back into
	// input.Prepayments[i].NextDate: the cursor is transient generation
	// state, and Go shares the Prepayments backing array with the
	// caller, so persisting it would make Amortize non-idempotent. A
	// second run on the same input would see a half-advanced cursor and
	// build a different schedule — which previously defeated the
	// iterateNewton backward solver (it evaluates many trials against
	// one shared input, and the poisoned cursor flattened the residual
	// so Newton's step ran away). Each entry starts Unknown and is
	// seeded from StartDate on first use, matching the prior behavior.
	nextDates := make([]types.DateRec, len(input.Prepayments))
	for i := range nextDates {
		nextDates[i] = types.UnknownDate()
	}

	// prepayExhausted[i] marks a series whose NEXT extra date is not
	// representable — AddPeriod returned an error, which past peryr 26/52 means
	// the Julian/MDY day-70000 ceiling (26 Aug 2091, VIDEODAT.pas:373).
	//
	// Both advance sites below used to write `if err == nil { nextDates[i] = next }`
	// and do nothing on error. That leaves the cursor pointing AT the extra just
	// paid, so the series is immediately due again at the same date. Every filter
	// that admits it re-admits it forever.
	//
	// This was latent until round 19: the walk never reached far enough for
	// AddPeriod to fail, because the A2 block's own `AddDays(lastPending, 1)` hit
	// the same ceiling first and stopped the walk (discrepancies §59). Fixing §59
	// let the walk run into the region where THIS one bites, and the drain loop
	// span-appended rows until the process was OOM-killed.
	//
	// Retiring the series is both terminating and faithful: a series whose next
	// date cannot be represented has no further extras to pay, which is exactly
	// what DOS's CheckOffBalloon produces when MDY stamps the error byte.
	//
	// It is tracked as its own flag rather than by poking a sentinel into
	// nextDates because every filter site already tests nextDates for ordering,
	// and a magic date would have to be understood at each of them. One named
	// boolean is checkable; a sentinel date is the kind of thing that gets
	// compared with DateComp by accident.
	prepayExhausted := make([]bool, len(input.Prepayments))

	// reAmortRowIdx is the Schedule index that the row DOS's Re_Amortize
	// recomputes for ITSELF will occupy — i.e. the index of the first row emitted
	// after an adjustment crossing fires. It exists solely to decide whether the
	// entire-walk residual fold applies at the NEXT crossing; -1 means no
	// crossing has fired yet. See the adjustment block below.
	reAmortRowIdx := -1

	// applyPendingAdjustments fires each not-yet-applied rate adjustment at the
	// FIRST emitted row whose date passes it, mirroring DOS exactly.
	//
	// DOS AMORTOP.pas:1062-1077 (DecideWhetherToPrintALine) tests the LOOKAHEAD
	// row, not the row just committed:
	//
	//	if (next_adj <= nadj) and (DateComp(nextt, adj[next_adj]^.date) > 0) then
	//	  Re_Amortize(p);
	//
	// with nextt = nextpayment.date. Re_Amortize then rolls back one row
	// (`NextPayment := Payment; NextPayment.ComputeNext(p, usap)`,
	// AMORTOP.pas:1601-1603) and recomputes it at the new rate. Net effect: the
	// new rate/payment takes hold on the first row whose date is STRICTLY AFTER
	// the adjustment date, and `p` at the solve is the balance after the last
	// row on-or-before it. Both facts matter when off-cycle extra rows fall
	// inside the period that straddles the adjustment.
	//
	// The previous port fired this at the REGULAR period boundary
	// (prevDate <= adj.Date < currentDate) at the BOTTOM of the loop, so every
	// off-cycle prepayment/balloon row emitted inside that period picked up the
	// new rate a row (or several) too early, and the segment solve saw the
	// balance as of the previous REGULAR row instead of the last extra.
	//
	// 2026-07-26, amortization fuzzer5 seed 20214 (cycle 12), minimal repro:
	//
	//	amort_oracle 321223.74 0.0832100000 240 12 loandmy=28.1.2025 \
	//	  firstdmy=28.2.2025 pre=22:12:52:58.51 adj=24:0.1287840000:3768.05
	//
	// The adjustment lands on 1/28/2027 and is snapped to 1/31/2027 by the
	// Amortize.pas:258-271 grid snap (see the block above). A 52/yr prepayment
	// stream puts an extra row on 1/30/2027, strictly inside (1/28, 2/28].
	// DOS holds the OLD rate through 1/30 (int 143.07) and switches on 2/6
	// (int 664.45); the port charged the new rate from 1/30 (int 221.42) and
	// drifted +662.20 of interest over the remaining schedule.
	//
	// `prevDate` is maintained as the date of the LAST EMITTED ROW (the drain
	// block assigns prevDate = drainDate), so it is already DOS's payment.date
	// and needs no separate cursor. Returns true when the adjustment machinery
	// set result.Err and the caller must abandon the schedule.
	adjApplied := make([]bool, len(input.Adjustments))
	applyPendingAdjustments := func(rowDate types.DateRec, payNumNow int) bool {
		// Check for rate adjustments. ROW-granular trigger (2026-07-26):
		// the adjustment takes effect on the FIRST period whose advance stepped over
		// its date, i.e. prevDate <= adj.Date < rowDate (adj.Date lies in the
		// half-open interval we just moved across). This fires the adjustment exactly
		// once, on the period boundary that passes it, regardless of whether the
		// adjustment date lands on a payment date.
		//
		// (Payment/amount/rate Iterate terminals never reach this block —
		// fancyTerminal strips the adjustments from its walk, mirroring DOS's
		// Re_Amortize gate `((next_adj <= adjnum) or entire)` with
		// entire=til_adj=FALSE and adjnum=0 during Iterate, AMORTOP.pas:1215.
		// The balloon-amount and APR walks pass their adjustments through and
		// DO re-amortize here, like DOS's entire/value_calc walks.)
		// DOS's fold lives in the MAIN repeat loop (AMORTOP.pas:1209-1213), between
		// `NextPayment.ComputeNext` and the Re_Amortize dispatch. The row
		// Re_Amortize computes for ITSELF at its own tail —
		//
		//	NextPayment := Payment;
		//	NextPayment.ComputeNext(p, usap);   (AMORTOP.pas:1604-1610)
		//
		// — never passes through that fold: control returns straight to the top of
		// the repeat loop, which copies it into Payment untouched. So when two
		// adjustments fall in consecutive periods, the SECOND Re_Amortize reads
		// `p := Payment.principal` (AMORTOP.pas:1508) from an UNFOLDED row and
		// resumes from the accumulated negative balance, where the first read a
		// folded 0. Carrying the fold into both is the divergence fuzzer5 seed 8910
		// found (N=1000):
		//
		//	amort_oracle 277418.13 0.0549570000 15 1 b365 exact b12=31574.74 \
		//	  b72=73969.36 adj=24:0.0402370000:34304.32 \
		//	  adj=120:0.1191180000:36527.48 adj=132:0.0250190000:38003.28 \
		//	  targ=1460.06 pts=0.034853 bdump
		//	  DOS terminating balloon 1/1/2039  -160132.9051
		//	  port                              -119810.4122
		//
		// The schedule DISPLAYED agrees to the penny (int 84173.40, paid 361591.53,
		// retiring 1/1/2034) — only the tack-on probe walk, which runs on past
		// retirement to 1/1/2039, crosses the 1/1/2034 and 1/1/2035 adjustments in
		// consecutive periods. The 1/1/2034 crossing reads a folded 0 in both
		// engines; the 1/1/2035 crossing must read -36527.48, and the port was
		// re-folding it to 0. Everything downstream compounds off that.
		//
		// The test is ROW-granular, not period-granular, because DOS's `Payment` is
		// the previous ROW and the fold runs once per row. Re_Amortize fires on the
		// FIRST row whose date passes the adjustment (`DateComp(nextpayment.date,
		// adj[next_adj]^.date) > 0`, AMORTOP.pas:1216) and recomputes exactly that
		// one row; every row after it is an ordinary main-loop row and IS folded.
		// So the suppression holds only while the Re_Amortize row is still the last
		// row emitted. Period granularity was fuzzer5 seed 8905 (N=1000):
		//
		//	amort_oracle 363404.28 0.1009740000 40 4 b365 plusreg b12=64936.40 \
		//	  b81=68617.60 pre=69:100:26:328.74 adj=39:0.0823720000:16620.67 \
		//	  adj=87:0.0651110000:16817.86 adj=90:0.0237260000:14859.23 \
		//	  targ=1722.10 pts=0.037924 payhard=14434.42 bdump
		//	  DOS terminating balloon 1/1/2034  -171011.9000
		//	  port                              -190966.5400
		//
		// adj=87 (4/1/2031) and adj=90 (7/1/2031) ARE consecutive quarterly periods,
		// but a 26/yr prepayment series puts six off-cycle extra rows between them:
		// DOS's Re_Amortize row is 4/14/2031, and by 7/7/2031 `Payment` is the
		// 7/1/2031 regular row, folded to 0 like any other. Suppressing on period
		// adjacency carried an unfolded −18805.49 into the second crossing.
		foldSuppressed := reAmortRowIdx >= 0 && len(result.Schedule) == reAmortRowIdx+1
		for i := range input.Adjustments {
			adj := &input.Adjustments[i]
			if adj.DateStatus >= types.InOutDefault && !adjApplied[i] &&
				dateutil.DateComp(rowDate, adj.Date) > 0 &&
				dateutil.DateComp(adj.Date, loan.LoanDate) >= 0 {
				adjApplied[i] = true
				// The next row appended is the one Re_Amortize recomputes.
				reAmortRowIdx = len(result.Schedule)
				// DOS's entire-walk residual fold, observed through Re_Amortize.
				// In an `entire` walk with value_calc FALSE (the ONLY such call
				// site is EstimateAndRefineBalloon's very_last probe,
				// Amortize.pas:637) the fold at AMORTOP.pas:1209-1213 zeroes the
				// REPORTED principal of every row whose balance is < minpmt --
				// one-sided, so negative balances fold too -- while the walk's
				// own balance keeps going. Re_Amortize then starts from that
				// reported value:
				//	p := Payment.principal;            (AMORTOP.pas:1508)
				// so the adjustment resumes from 0, not from the accumulated
				// negative balance. `p` here is exactly DOS's Payment.principal
				// position (last emitted row, date <= adj.Date), so applying the
				// fold at this point reproduces it. See LoanInput.entireWalk.
				if input.entireWalk && p < minPmt && !foldSuppressed {
					p = 0
				}
				hasRate := adj.LoanRateStatus >= types.InOutDefault
				hasAmt := adj.AmtOK
				remaining := loan.NPeriods - payNumNow
				if hasRate {
					loan.LoanRate = adj.LoanRate
					// Same latch as the AO6 site below: DOS's Re_Amortize runs
					// ComputeTrueRate on the adjustment's rate (AMORTOP.pas:
					// 1509-1512), and a rate with `1 + rate/peryr <= 0` makes
					// RateFromYield evaluate lnn(x<=0), which sets errorflag and
					// refuses the screen at MakeTable (Amortize.pas:1455).
					// Reached here with a USER-supplied rate rather than a
					// solved one, but the DOS path is identical.
					var trErr error
					truerate, trErr = ComputeTrueRate(&loan, settings)
					if trErr != nil {
						result.Err = trErr
						return true
					}
					f = GrowthPerPeriod(&loan, settings.YrInv)
				}
				if hasAmt {
					d = adj.Amount
				}
				// AO5 (EstimateAndRefineAdjPayment): a rate change
				// with no new payment re-amortizes the current
				// balance over the remaining term at the new rate —
				// otherwise the old payment no longer amortizes the
				// loan cleanly after the rate moves. Balloons dated
				// after the adjustment reduce the principal the
				// regular payment must retire, so their value is
				// discounted back to the adjustment date and netted
				// off (DOS Re_Amortize balloon term, AMORTOP.pas:1561).
				//
				// AO7 (re-amortize at current rate): a date-only
				// adjustment row supplies neither a new rate nor a
				// new payment, and asks DOS to re-solve the regular
				// payment over the remaining term at the *unchanged*
				// rate. This is useful when an upcoming balloon (or
				// drift left over from a prior adjustment) means the
				// running payment no longer amortizes the loan
				// cleanly — AO7 resets the payment without changing
				// the rate. The same re-amortize formula handles
				// both AO5 and AO7: when no new rate was supplied,
				// `f` and `truerate` keep their pre-adjustment values
				// (set further up only when `hasRate`), so the solve
				// uses the current rate.
				if !hasAmt && remaining > 0 {
					// DOS's Re_Amortize seed, verbatim (AMORTOP.pas:1545-1568):
					//
					//	n := NumberOfInstallments(adj[next_adj]^.date, h^.lastdate,
					//	                          h^.peryr, on_or_after);
					//	f := GrowthPerPeriod;
					//	adjp := p;
					//	next_balloon := old_next_balloon;
					//	for i := next_balloon to user_nballoons do
					//	  adjp := adjp - balloon[i]^.amount
					//	          * exxp(-RateFromYield(h^.loanrate, h^.peryr)
					//	                 * YearsDif(balloon[i]^.date, adj[next_adj]^.date));
					//	denom := (1 - exxp(-pred(n) * lnn(f)));
					//	if (abs(denom) < teeny) then d := adjp / pred(n)
					//	else d := adjp * (f - 1) / denom;
					//
					// Three details the port had wrong (2026-07-25 fuzzer5 triage):
					//
					//  1. WHICH balloons are netted off. DOS restores `next_balloon`
					//     to `old_next_balloon` — the index of the next UNCONSUMED
					//     balloon — and nets every balloon from there to the end. The
					//     adjustment is processed at the BOTTOM of the row loop, after
					//     rowDate has already advanced to the next row, so a
					//     balloon falling on that next row has NOT been consumed yet
					//     and DOS does net it off. The port tested
					//     `b.Date > rowDate`, which dropped exactly that balloon.
					//  2. WHERE the balloons are discounted back to. DOS discounts to
					//     `adj[next_adj]^.date` — the ADJUSTMENT date — not to the
					//     next row's date.
					//  3. WHAT rate discounts them. DOS uses
					//     `RateFromYield(h^.loanrate, h^.peryr)`, the continuously
					//     compounded true rate, not the nominal `h^.loanrate`.
					//
					// On `156423.83 0.1083100000 24 1 plusreg mor=108 b132=7755.60
					// b192=35589.38 b216=41094.90 adj=48:0.0852820000:24633.90
					// adj=84:0.1444110000:14450.80 adj=204:0.0488530000: targ=3253.54`
					// the 1/1/2042 balloon sits on the row right after the 1/1/2041
					// rate adjustment, so (1) alone left the re-solved payment at
					// 15623.45 against DOS's 8880.36 — the loan retired three years
					// early and 4,447.27 of interest went missing.
					adjDate := adj.Date
					discRate := truerate
					if rr, e := interest.RateFromYield(loan.LoanRate, byte(loan.PerYr),
						settings.YrDays); e == nil {
						discRate = rr
					}
					netBal := p
					for bi := range input.Balloons {
						b := &input.Balloons[bi]
						if b.AmountStatus >= types.InOutDefault &&
							dateutil.DateComp(b.Date, prevDate) > 0 {
							// isLoanCalc MUST be true. DOS's YearsDif (INTSUTIL.pas:787-824)
							// dispatches on the SCREEN:
							//
							//	else if (thisrun in [iPVL,iINV,iCHR]) or (df.c.basis=x365_360)
							//	  then YearsDif:=(Julian(z)-Julian(a))*yrinv
							//	else begin {iAMZ or possibly iRBT} ...leap-aware... end
							//
							// Re_Amortize only ever runs on the amortization screen
							// (thisrun = iAMZ), so on a 365 basis it takes the leap-aware
							// calendar-year branch - 365 or 366 days per ACTUAL year, with
							// 12/31 -> 1/1 counted in the OLD year - not the flat 1/365.25
							// `yrinv` divisor. Passing false routed this balloon discount
							// through the PVL form. The two branches agree on the 360 and
							// 365/360 bases, so it was invisible except on a plain 365
							// basis, where a whole-year span comes out 730/365.25 = 1.99863
							// instead of the true 2.0.
							//
							// That is ~0.7 basis points of the discount, which looks
							// harmless - but the seed it perturbs feeds an UNBRACKETED
							// secant. 2026-07-25 fuzzer5 seed 9011, verified against the
							// real DOS engine's own Iterate trace
							// (scripts/build_trace_oracle.sh -mode itr):
							//
							//	amort_oracle 288965.30 0.0877190000 14 1 b365 exact usa \
							//	  b48=56915.88 pre=84:79:24:122.78 adj=24:0.1154130000: \
							//	  adj=36:0.1260590000:44958.93 adj=96:0.1403990000: targ=8527.82
							//
							//	ITR0 seedx=26683.5524179702 p=235663.5578172626
							//	ITR n=6 ... newx=30866535432146680.0
							//	ITR n=7 ... newx=44412.0000000000
							//	ITR n=15 p=-0.0000000005 newx=67636.9891584331
							//
							// The terminal is FLAT at the seed (a target-floored plateau),
							// so |final-p| lands just above `teeny`, the secant overflows to
							// 3.09e16, cancels back to exactly 44412 and converges on the
							// SECOND root, 67636.99. The port's seed 26682.4715 - 1.08
							// lower, purely from this day-count - cancelled to a different
							// point and converged on the FIRST root, -27444.24, where the
							// target floor then pinned every row at 8527.82 + interest.
							// 68,360.21 of interest went missing over 89 rows.
							yd := dateutil.YearsDif(b.Date, adjDate,
								settings.Basis, settings.YrInv, true)
							if disc, e := interest.Exxp(-discRate * yd); e == nil {
								netBal -= b.Amount * disc
							}
						}
					}
					// DOS's Re_Amortize (AMORTOP.pas:1545-1569) sets adjp := p
					// (the current balance) and subtracts the future balloons with
					// NO clamp — so an over-funded segment (balance already retired,
					// or future balloons exceeding it) yields a NEGATIVE re-solved
					// payment, which DOS carries through the value walk as a refund.
					// An earlier `if netBal < 0 { netBal = 0 }` clamp here was
					// DOS-absent: it zeroed the payment past an overpayment payoff,
					// so a rate adjustment scheduled after the loan retires gave the
					// wrong APR (pass 7 — `100000 0.10 120 12 payhard=2500 pts=3
					// adj=60:0.06:` → DOS 0.129605, clamped port 0.144859). Removed
					// to match DOS; the display walk stops at payoff before reaching
					// such an adjustment, so this only changes the entire/value walk.
					// USA-rule carry (V6-2 / R9 fix): the running usap
					// is part of engine state and survives the
					// adjustment naturally — the per-period loop at
					// line 965-969 continues to update it. The new
					// payment matches DOS Re_Amortize at
					// AMORTOP.pas:1545-1569: amortize the full
					// outstanding `netBal` over the remaining term at
					// the new rate, with no special usap split. (An
					// earlier port subtracted usap from netBal and
					// added a linear paydown term, on the theory that
					// usap should retire linearly. The standard
					// per-period rule retires usap much faster than
					// linear, so that adjustment left a large residual
					// on negative-amort + ARM loans; reverting to the
					// DOS formula closes that gap. Exact retirement on
					// the most pathological cases still requires the
					// full DOS Iterate routine — task #103.)
					// The seed annuity runs over `pred(n)` periods, NOT the
					// remaining payment count: DOS counts n from the ADJUSTMENT
					// date to the last payment date on_or_after, then discounts
					// over n-1 (AMORTOP.pas:1566). The count matters because the
					// Iterate below is an unbracketed secant — see dosPaymentSeed.
					//
					// The `var l` snap this call performs also PERSISTS into
					// loan.LastDate, because AMORTOP.pas:1547 passes the
					// h^.lastdate GLOBAL by reference with no save/restore guard
					// (contrast the guarded call at Amortize.pas:1301-1304). See
					// the long note at reAmortize in dosport_walk.go — this is the
					// piecewise engine's copy of the same site, needed because
					// dosPortCanHandle routes r78, in-advance, daily,
					// exact×non-360 and REPLACE-mode extras away from the
					// structural port. RAW + clamp for the phantom-daterec reason
					// given there. 2026-07-29 task #94, fuzzer5 seed 21081.
					seedN := remaining
					if loan.LastOK && dateutil.DateOK(adjDate) && dateutil.DateOK(adjLastDate) {
						n, sy, sm, sd := dateutil.NumberOfInstallmentsRaw(adjDate,
							adjLastDate, loan.PerYr, types.OnOrAfter)
						if n > 1 {
							seedN = n - 1
						}
						lastDay := sd
						if dim := dateutil.DaysInM(types.NewDateRec(sy, time.Month(sm), 1)); lastDay > dim {
							lastDay = dim
						}
						snapped := types.NewDateRec(sy, time.Month(sm), lastDay)
						result.reAmortLastDate = snapped
						// The VAR write-back (AMORTOP.pas:1547 → INTSUTIL.pas:1018).
						// Confined to adjLastDate — see the declaration for why this
						// must NOT touch loan.LastDate.
						adjLastDate = snapped
					}
					d = annuityPayment(netBal, f, seedN)
					if dpTrace {
						fmt.Fprintf(os.Stderr, "GRA adj=%s p=%.6f netBal=%.6f "+
							"remaining=%d seedN=%d f=%.10f seed=%.6f usap=%.6f "+
							"prev=%s next=%s entire=%v\n",
							adjDate.Time.Format("2006-1-2"), p, netBal, remaining, seedN, f, d, usap,
							prevDate.Time.Format("2006-1-2"), rowDate.Time.Format("2006-1-2"),
							input.entireWalk)
					}
					// DOS uses that analytic value only as a SEED, then calls
					// Iterate to drive the tail's terminal balance to exactly
					// zero when balloons (or prepayments) are present
					// (AMORTOP.pas:1561-1582). The seed's discounted-balloon
					// term is a first-order correction and leaves a residual on
					// balloon-bearing ARMs (the long-standing task #103 gap).
					// Refine it the same way DOS does — a schedule-oracle solve
					// of the adjustment payment.
					// DOS refines (Iterate) only for balloons/prepayments. For
					// skipped months or a target it keeps the plain annuity seed
					// and lets the FINAL payment absorb the residual (the oracle
					// shows a ballooned last row + higher interest, e.g.
					// ARM->9.38%@m70 + skip=6 on $88k/360 re-amortizes to ~$692
					// and dumps $63k in the last row). Refining for skip/target
					// would over-amortize and under-report interest, so they are
					// excluded here; the base-payment solve still strips the
					// adjustment (dispatch above) so the PRE-adjustment payment is
					// option-aware.
					// M1 (global-Iterate refactor): use DOS's til_adj SEGMENT solve
					// (solveSegmentPayment — sub-loan over [adj → last] at the
					// current rate, ignoring later adjustments) instead of the
					// entire-schedule refineAdjustmentPayment, which was ill-posed
					// with a second ARM still pending and so had to be gated to a
					// single adjustment. The segment solve composes for 2+ ARMs.
					// Trigger stays balloon/prepay only (DOS Iterates just for those;
					// skip/target keep the plain annuity + ballooned final row).
					// DOS's nested til_adj Iterate fires for balloons, prepayments,
					// OR the exact method on a non-360 basis (AMORTOP.pas:1571
					// `(user_nballoons > 0) or (npre > 0) or ((exact) and
					// (basis<>x360))`).
					// `user_nballoons > 0` counts the USER's balloons, not the ones
					// still ahead of the boundary — solveSegmentPayment applies the
					// literal DOS gate; this outer test is only the cheap superset.
					if len(input.Balloons) > 0 || len(input.Prepayments) > 0 || exactDaily(settings) {
						// No `refined > 0` filter: DOS's Re_Amortize stores whatever
						// Iterate returns, and a negative segment payment is a real
						// DOS regime (moratorium + REPLACE-mode prepay collision —
						// see solveSegmentPayment's note). The comment above already
						// records that DOS carries a negative re-solved payment
						// through the value walk as a refund.
						// seedN, NOT remaining (2026-07-31, discrepancies §53).
						// `remaining` is a COUNT (`loan.NPeriods - payNumNow`); DOS has
						// no such count here. Its segment Iterate calls RepayFancyLoan,
						// whose ComputeNext decides regular-vs-extra row by row against
						// the h^.lastdate GLOBAL (AMORTOP.pas:606) — the same global the
						// Re_Amortize VAR snap moves. So the number of REGULAR payments
						// DOS solves over is implied by the snapped date, and seedN —
						// derived from NumberOfInstallmentsRaw(adjDate, adjLastDate, …)
						// just above — is exactly that number. The two agree whenever no
						// snap has fired; they differ by one period when it has, and that
						// one period is the whole of §53: the segment was solved over 12
						// periods instead of 13, returning 5170.29 where DOS's Iterate
						// returns 4720.33.
						// CAPPED BY veryLast. DOS bounds its segment walk two ways at
						// once and the port needs both: ComputeNext decides regular-vs-
						// extra against the SNAPPED h^.lastdate (AMORTOP.pas:606), while
						// RepayFancyLoan's until-clause stops the walk at `stopdate`,
						// which for the segment Iterate (adjnum=0) is `very_last`
						// (AMORTOP.pas:1140-1142). And very_last is computed by
						// DetermineVeryLast at Amortize.pas:1320 — BEFORE the adjustment
						// pre-pass at :1408 — so it never sees the snap.
						//
						// The snap can therefore push h^.lastdate PAST very_last, and when
						// it does the extra period is unreachable: the walk ends first.
						// Passing the uncapped seedN cost a real regression, caught by the
						// randomized goamort sweep before this landed:
						//
						//	amort_oracle 90498.48 0.108453 84 12 loandmy=28.2.2023 \
						//	  firstdmy=28.5.2023 adj=18:0.1434: adj=24:0.1013: \
						//	  adj=37:0.0380: b17=3533.04
						//	-> DOS 31961.76; uncapped seedN gave 32059.15
						//
						// There the 28.2.2025 adjustment (Feb 28 IS a month end) snapped
						// lastdate 28.4.2030 -> 30.4.2030, and the 28.3.2026 adjustment then
						// snapped that to 28.5.2030 — one month past very_last = 28.4.2030,
						// because this screen has no trailing option to extend very_last.
						// DOS solves 49 periods; seedN says 50.
						segN := seedN
						if dateutil.DateComp(adjLastDate, veryLast) > 0 {
							segN = remaining
						}
						refined, ok, bad := solveSegmentPayment(
							input, loan, *settings, p, prevDate, rowDate, segN, d, usap)
						if bad {
							// DOS-FAITHFUL FAILURE PROPAGATION, the AO5 mirror of
							// the AO6 rate branch below. Re_Amortize's amount arm is
							//
							//	if Iterate(p, usap, Payment.date, t, d, til_adj) then
							//	  begin ...store d... end
							//	else
							//	  begin abort := true; errorflag := true; end;
							//	                        {AMORTOP.pas:1577-1587}
							//
							// `abort` stops the walk at this very row (the
							// until-clause at :1221 tests it) and `errorflag`
							// condemns the screen —
							// EstimateAndRefineAdjPayment returns `(not errorflag)`
							// (Amortize.pas:338) and both Enter and MakeTable bail
							// on `if (errorflag) then exit` (:1204, :1219, :1458).
							// DOS draws no table and shows Iterate's own message
							// (AMORTOP.pas:1489). Continuing here at the unrefined
							// annuity seed — which the port did — invents a
							// schedule DOS never produces.
							result.Err = fmt.Errorf("Computation of payment amount " +
								"or interest rate did not converge.")
							return true
						}
						if ok && refined != 0 {
							if dpTrace {
								fmt.Fprintf(os.Stderr, "GRA refined=%.6f (seed %.6f)\n", refined, d)
							}
							d = refined
							// TABLE_START's "Dav Holle provision"
							// (Amortize.pas:1429-1435): when the regular
							// payment is a hard user number and the screen is
							// fancy, DOS hardens EVERY adjustment amount and
							// balloon amount to whole cents just before the
							// table is drawn —
							//
							//   if (hard_payment) and (fancy) then begin
							//     for i:=1 to nadj do Round2(adj[i]^.amount);
							//     for i:=1 to nballoons do Round2(balloon[i]^.amount);
							//   end;
							//
							// That runs AFTER EstimateAndRefineAdjPayment has
							// solved the blank adjustment amounts, so the value
							// the table walk consumes is Round2(solved), not the
							// raw secant root: Re_Amortize re-reads it through
							// `if (adj[next_adj]^.amtok) then d := adj[...]^.amount`
							// (AMORTOP.pas:1514-1517) on the display pass.
							// The port solves inline during the walk, so the
							// rounding has to be applied here instead.
							//
							// 2026-07-25 fuzzer5 seed 9015, the `dInt=2541.40`
							// case: DOS's marker line reads `Payment fixed at
							// 16305.49` while the secant root is
							// 16305.4873947269. Walking the unrounded root
							// dropped 0.0026 less principal on each of the 11
							// constant-interest US-Rule rows, so the boundary
							// balance at the SECOND adjustment came out
							// 280320.688658 against DOS's 280320.66 — a 1-to-3
							// cent balance drift with the payment and interest
							// columns already matching to the cent.
							//
							// ORDERING (2026-07-27, seed 20431). Rounding HERE
							// is only correct for the LAST blank adjustment on
							// the walk. DOS's pre-pass solves adjustment k+1 on
							// a path that still carries adjustment k UNROUNDED,
							// because the Round2 sweep fires once, later, at
							// TABLE_START. An earlier note here claimed the gap
							// was bounded by "~2e-6" of payment; that is wrong.
							// On a target-floored segment the terminal is FLAT,
							// so a 5.88-cent boundary-balance shift moved the
							// root off the plateau, the Newton stalled, the
							// solve was rejected outright and the raw annuity
							// seed survived — 23.64 of interest and a whole row.
							// runAdjustmentPrePass now performs the unrounded
							// solve pass and the single sweep ahead of this
							// walk, so on any input with two or more blank
							// adjustments `hasAmt` is already true here and this
							// branch never runs. It remains for the single-
							// adjustment case, where inline rounding and the
							// sweep are equivalent, and for adjustments the
							// bounded pre-pass never reached.
							//
							// Suppressed during the pre-pass itself — that walk
							// IS the unrounded path.
							//
							// Only the AMOUNT is hardened. DOS's sweep never
							// touches adj[i]^.loanrate, so the AO6 solved rate
							// below stays raw.
							if hardPayment && !input.inAdjPrePass {
								d = interest.Round2(d)
							}
							// DOS stores the solved segment payment on the
							// adjustment row and sets amtok (AMORTOP.pas:
							// 1579-1581), so any LATER walk over the same screen
							// (the APR value pass, a payoff, a re-render) reuses
							// it instead of re-solving — "If it's already outp,
							// we don't want to re-compute it. This saves time and
							// it's essential for APR value calculation."
							adj.Amount = d
							adj.AmountStatus = types.InOutOutput
							adj.AmtOK = true
						}
					}
				}
				// AO6 (EstimateAndRefineAdjRate, Amortize.pas:1415):
				// a new payment with no new rate — solve the rate at
				// which that payment amortizes the balance over the
				// remaining term, and continue the schedule at it.
				// This is the mirror image of AO5 (rate given,
				// payment solved), so an adjustment always keeps the
				// loan on its original term.
				if hasAmt && !hasRate && remaining > 0 {
					// The uniform annuity fit. Kept ONLY as the fallback answer for the
					// `!ok2` arm below and as the failure signal the shared `!ok` refusal
					// reads — it is NOT what the schedule adopts. See the block below.
					r, ok := solveAdjRate(p, d, remaining, loan, settings.YrInv)
					// THE SCHEDULE SOLVE IS THE ONLY SOLVE (2026-07-25). DOS has no
					// uniform pre-fit anywhere. Its rate branch (AMORTOP.pas:1520-1535) is
					//
					//	d := adj[next_adj]^.amount;
					//	if Iterate(p, usap, payment.date, nextpayment.date,
					//	           h^.loanrate, til_adj) then ...
					//	else errorflag := true;
					//
					// i.e. it Iterates the REAL til_adj walk, seeded from the CURRENT
					// h^.loanrate, unconditionally. solveSegmentRate is that walk, so it
					// now runs unconditionally too, seeded from loan.LoanRate.
					//
					// This block used to be gated: escalate to the real walk only on the
					// exact method, a day-count frequency at a non-360 basis, a
					// still-shaping option (moratorium / target / skip / pending balloon /
					// remaining prepayment), a non-positive balance, or a failed uniform
					// fit — and otherwise adopt the uniform rate. Each of those triggers
					// was added because some fuzz case proved the uniform fit wrong there,
					// which is the shape of a gate that is chasing an approximation rather
					// than modelling the engine. Two failure modes it could not cover:
					//
					//   - WRONG BASIN. The uniform terminal (balanceAfterN, constant
					//     GrowthPerPeriod) is a different function of the rate than the
					//     real fancy terminal, so its secant lands in a different root
					//     basin even where both converge.
					//   - MISSED CONDEMNATION. The uniform terminal never calls
					//     ComputeTrueRate, so it can wander through rates that in DOS drive
					//     `1 + yy/nn <= 0` into lnn and condemn the entire screen
					//     (INTSUTIL.pas:1164-1171). The port then answered where DOS
					//     refuses — the worst divergence direction. 2026-07-25 fuzzer5 seed
					//     8953, verified against the real DOS engine (stack from an
					//     instrumented lnn: MakeTable -> EstimateAndRefineAdjRate ->
					//     RepayFancyLoan -> Re_Amortize -> Iterate -> RepayFancyLoan ->
					//     ComputeTrueRate -> RateFromYield -> lnn(-0.340032)):
					//
					//	amort_oracle 239002.99 0.0387330000 9 1 b365 prepaid plusreg \
					//	  r78 usa b36=50170.30 b48=59339.78 adj=24::36865.13 \
					//	  adj=60::44594.82 adj=84:0.1251140000:37475.79 payhard=38664.26
					//	  DOS: ERR "Error: The data you have specified contain an
					//	       inconsistency."
					//	  gated port: int=36323.13 paid=275326.12 rows=6
					//
					//	At the 1/1/2030 boundary (bal 26587.98, new pay 44594.82,
					//	4 periods left) none of the old triggers fired — the balloons
					//	at months 36/48 were already behind it — so the port adopted the
					//	uniform r = -1.8315 and built a schedule. DOS's Iterate trials a
					//	rate near -1.34 on its way, hits the lnn guard and abandons the
					//	table.
					//
					// Running the real walk everywhere also closed a same-seed value
					// divergence (adj1+balloon2+mor+prepay1+pts, dInt 799.70 on
					// `392756.46 0.0972330000 48 4 exact plusreg usa mor=54 b60=9221.40
					// b90=24937.73 pre=3:106:26:432.75 adj=87::16833.75 pts=0.007878
					// payhard=13572.73`), and left the whole ./internal/finance/... suite
					// green — so it is strictly closer to DOS, not merely differently
					// wrong.
					//
					// The old gate's evidence is preserved for the record; each case below
					// is now handled by the unconditional walk:
					//	amort_oracle 100000 0.06 78 26 adj=24::2000 -> int 24895.73
					//	  (uniform left 24914.75: day-count actual-day drift)
					//	amort_oracle 190352.81 0.1136450000 52 4 mor=75 \
					//	  adj=48::7713.08 payhard=9090.79 -> int 125732.90
					//	  (the moratorium-blind uniform fit gave 201696.08)
					//	amort_oracle 190352.81 0.1136450000 52 4 \
					//	  pre=108:165:52:30.57 adj=48::7713.08 payhard=9090.79
					//	  -> int 206962.54 (the prepay-blind fit gave 263246.67)
					//	amort_oracle 394528.20 0.1281160000 288 12 b365 exact plusreg \
					//	  mor=93 adj=17::6284.10 adj=94::6283.43 adj=206::5926.60 \
					//	  skip=1,3,5 pts=0.000169 -> payment 6426.0866, int 884618.14
					//	  (uniform blew up: bal 394528.20, pay 6284.10, n 271 ->
					//	   r 84.80%, terminal 3.3e13, ok=false)
					{
						rr, ok2, bad := solveSegmentRate(input, loan, *settings, p,
							prevDate, rowDate, remaining, d, loan.LoanRate, usap, adjLastDate)
						if bad {
							// DOS-FAITHFUL SCREEN CONDEMNATION. A trial rate in
							// the implied-rate secant drove `1 + yy/nn <= 0` into
							// lnn, which in DOS raises this exact message and sets
							// errorflag (INTSUTIL.pas:1164-1171). errorflag is the
							// engine-wide abort: MakeTable produces NO table, so
							// the screen shows the error and nothing else. Emitting
							// a schedule here — which the port used to do, having
							// discarded the error — is a divergence in the worst
							// direction: an answer where DOS refuses to answer.
							result.Err = fmt.Errorf("Error: The data you have " +
								"specified contain an inconsistency.")
							return true
						}
						if ok2 {
							r, ok = rr, true
						} else if p <= 0 {
							// DOS's Iterate is the ONLY rate solve it has
							// (AMORTOP.pas:1520-1531), and `else errorflag :=
							// true` is what happens when it fails. On a
							// non-positive balance the port's uniform annuity
							// fast path has no DOS counterpart at all, so
							// keeping its answer here would be inventing a
							// convergence out of a screen DOS refuses. Drop it
							// and let the shared !ok arm below refuse.
							ok = false
						}
					}
					if !ok {
						// DOS-FAITHFUL FAILURE PROPAGATION (Amortize.pas:1415-1418):
						// a payment-only adjustment makes DOS solve the IMPLIED rate
						// (EstimateAndRefineAdjRate); when that Iterate cannot drive
						// the tail's terminal balance within half a penny it returns
						// false and DOS's dispatch does `if (not
						// EstimateAndRefineAdjRate(i)) then exit` — the whole table is
						// abandoned and the user sees "Computation of payment amount or
						// interest rate did not converge" (AMORTOP.pas:1489). This
						// happens when the new payment is too low to amortize the
						// balance over the remaining term at ANY rate in DOS's |rate|<2
						// band (e.g. a $0.09 payment on a ~$95k balance).
						//
						// The port previously DROPPED this failure on the floor (no
						// else branch): it kept the un-adjusted rate and let the tiny
						// payment negative-amortize into a final-fold balloon, emitting
						// a schedule DOS never produces. `solveAdjRate` already uses
						// DOS's own acceptance test (terminal balance within half a
						// penny), and Go's secant is at least as strong as DOS's
						// 20-iteration Newton, so {Go solve fails} ⊆ {DOS solve fails}
						// — propagating the failure can only converge toward DOS, never
						// introduce a new disagreement. 2026-07-13 pass-4 P4-N6.
						//
						//	amort_oracle 100000 0.08 36 12 adj=12::0.09
						//	  -> ERR "...did not converge" (Go was building an
						//	     18868.66-interest ballooned schedule at the old rate)
						result.Err = fmt.Errorf("Per%%Sense could not compute an "+
							"interest rate for the payment-only Adjustment dated "+
							"%d/%d/%d: the new Pmt Amount (%.2f) is too low to amortize "+
							"the balance over the remaining term at any rate. Raise the "+
							"Adjustment's Pmt Amount, or supply a new Rate on that row "+
							"instead.",
							int(adj.Date.Time.Month()), adj.Date.Time.Day(),
							adj.Date.Time.Year(), d)
						return true
					}
					{
						loan.LoanRate = r
						// DOS FREEZE (mirror of the AO5 amount freeze): a solved
						// implied rate is stored with loanratestatus := outp
						// (AMORTOP.pas:1524-1526, "If it's already outp, we don't
						// want to re-compute it"), so an outer Iterate solves it
						// once at the first trial and reuses it after.
						adj.LoanRate = r
						adj.LoanRateStatus = types.InOutOutput
						// DOS does NOT swallow this. Re_Amortize calls
						// ComputeTrueRate on the freshly solved adjustment rate
						// (AMORTOP.pas:1509-1512 and :1538-1541), and
						// ComputeTrueRate is `truerate := RateFromYield(
						// ReportedRate(h^.loanrate), df.c.peryr)`
						// (AMORTOP.pas:1265-1266), whose body is
						// `RateFromYield := nn*lnn(1+yy/nn)` (INTSUTIL.pas:1274).
						// When the implied rate is so negative that
						// `1 + rate/peryr <= 0`, lnn takes the x<=0 arm
						// (INTSUTIL.pas:1167-1171):
						//
						//	MessageBox('Error: The data you have specified
						//	  contain an inconsistency.', DO_LnnNegative);
						//	lnn:=0;
						//	errorflag:=true; overflowflag:=true;
						//
						// and `errorflag` is cleared in exactly one place in the
						// unit (FirstPass, Amortize.pas:204), so it is still set
						// when MakeTable tests `if( errorflag ) then exit`
						// (Amortize.pas:1455) — the whole screen is refused.
						// Same latch as the APR exxp overflow documented in
						// claude/apr_exxp_overflow_is_a_real_refusal_2026-07-27.
						//
						// Note this is NOT the A-W12 case below. A mildly
						// negative implied rate keeps `1 + rate/peryr > 0`, and
						// DOS really does run it and draw negative-interest
						// rows. Only the `<= -peryr` cliff refuses.
						//
						// Fuzzer5 cycle-29 seed 20384 (N=3000):
						//
						//	amort_oracle 118726.73 0.0667150000 11 1 b365_360 \
						//	  r78 usa loandmy=18.2.2024 firstdmy=18.2.2025 \
						//	  mor=36 pre=48:113:52:41.33 pre=12:97:24:140.54 \
						//	  adj=60:0.0304150000: adj=84:0.0501060000:16128.17 \
						//	  adj=96::16645.94 targ=3056.22 pts=0.002966
						//
						// The payment-only adjustment at period 96 implies a
						// rate of -1.65244256 against peryr 1, so DOS evaluates
						// lnn(-0.6524425615) and refuses with "Error: The data
						// you have specified contain an inconsistency."
						// (confirmed by a scratch oracle printing the lnn and
						// RateFromYield call sites). The port swallowed the
						// error here and walked on with a stale truerate,
						// emitting a 213-row schedule with interest -534877.47
						// for what is an 11-period loan.
						var trErr error
						truerate, trErr = ComputeTrueRate(&loan, settings)
						if trErr != nil {
							result.Err = trErr
							return true
						}
						f = GrowthPerPeriod(&loan, settings.YrInv)
						// AO6 with a new payment too LOW to amortize the
						// balance over the remaining term implies a
						// NEGATIVE rate (the balance can only reach zero on
						// schedule if interest is credited, not charged).
						// DOS computes and runs the negative rate, producing
						// negative interest rows after the adjustment. That
						// is correct but surprising, so surface it as a Note
						// (does not change any number).
						if r < 0 && !negRateNoted {
							result.add(types.NoteTier, "A-W12", []string{"adjustment"},
								"A payment-only adjustment set a new payment too low to amortize the "+
									"loan at a positive rate, so Per%Sense fit a negative interest rate "+
									"for the periods after it — you'll see negative interest from that "+
									"date and the balance barely moving. Raise the new payment if you "+
									"intended a positive rate.")
							negRateNoted = true
						}
					}
				}
			}
		}
		return false
	}

	// drainAll is set by the past-last-regular-payment block below (the "A2"
	// block) once the regular schedule is over and only trailing extras remain.
	// It widens the two drain predicates from "strictly before currentDate" to
	// "on or before currentDate", which is what lets currentDate be set to the
	// LAST pending extra's own date rather than one day past it.
	//
	// Why not one day past it (discrepancies §59, round 19). The block used to do
	// `currentDate = AddDays(lastPending, 1)`, and AddDays is Julian()+n then
	// MDY() — so it inherits DOS's `daynumber > 70000` MDY range guard
	// (VIDEODAT.pas:373; day 70000 is 26 Aug 2091). On a schedule whose last
	// pending extra falls past that ceiling the +1 FAILED, the block fell through
	// to its `break`, and the walk stopped at the last regular payment date with
	// every trailing balloon undrained. That break was commented "coverage:
	// excluded — defensive: jump not representable" and was in fact reachable on
	// any long-horizon schedule.
	//
	// DOS has no such failure because it never forms this date at all: the "+1
	// day" is a PORT-ONLY construct for re-entering the drain loop, whereas DOS's
	// ComputeNext (AMORTOP.pas:602-613) just tests the extra's own date against
	// h^.lastdate and never round-trips through MDY. Applying a faithful DOS
	// guard at a site DOS does not have is the defect.
	//
	// Equivalence: with jump = lastPending+1, the drain predicates admitted every
	// extra dated <= lastPending, and the block was entered only when
	// jump > currentDate, i.e. lastPending >= currentDate. The form below states
	// both directly, so behaviour is unchanged wherever AddDays succeeded.
	drainAll := false

	for payNum := 1; payNum <= loan.NPeriods+len(input.Balloons)+100; payNum++ {
		// Safety limit to prevent infinite loops
		if payNum > 10000 {
			result.Err = fmt.Errorf("The schedule grew past 10000 payments without " +
				"the loan paying off. The Pmt Amount may be too small to cover the " +
				"interest — raise the Pmt Amount, or leave it blank for Per%%Sense to " +
				"compute a payment that retires the loan.")
			break
		}

		// Off-cycle prepayment draining. Any prepayment series whose next due
		// date falls STRICTLY BEFORE this regular payment date is emitted as
		// its own dated row, with its own partial-period interest accrued from
		// the previous row's date — exactly as DOS emits an extra that lands
		// between two regular dates (balloonpos < 0, AMORTOP.pas:608-613).
		// Drain all such rows (possibly several, from one or more series)
		// before the regular period is computed.
		for {
			drainIdx := -1
			var drainDate types.DateRec
			for i := range input.Prepayments {
				pp := &input.Prepayments[i]
				if pp.PaymentStatus < types.InOutDefault || pp.PerYrStatus < types.InOutDefault ||
					pp.StartDateStatus < types.InOutDefault {
					continue
				}
				if prepayExhausted[i] {
					continue // next extra not representable — see prepayExhausted
				}
				if nextDates[i].IsUnknown() {
					nextDates[i] = pp.StartDate
				}
				// Stop-date retirement is POST-APPLICATION, not a pre-check.
				// DOS CheckOffBalloon (AMORTOP.pas:545-572) advances nextdate
				// and only THEN tests DateComp(nextdate, stopdate) > 0, and
				// FindNextExtra (AMORTOP.pas:490-543) takes pre[1]^.nextdate
				// unconditionally — it consults neither status nor stopdate.
				// So a live series ALWAYS pays at least one extra, even when
				// its stopdate already precedes its startdate. Crossed windows
				// like that are produced by DOS's own post-term-solve
				// prepayment-window rewrite (AMORTOP.pas:1335-1380), e.g.
				// seed 21003: start 4/1/2034, stop 7/1/2033, nn -8 — DOS pays
				// one extra there, a pre-check pays none (Δ 800.11 on the
				// terminating balloon).
				if prepayApplied[i] > 0 && pp.StopDateStatus >= types.InOutDefault &&
					dateutil.DateComp(nextDates[i], pp.StopDate) > 0 {
					continue
				}
				if pp.NNStatus >= types.InOutDefault && pp.NN > 0 && prepayApplied[i] >= pp.NN {
					continue
				}
				if cmp := dateutil.DateComp(nextDates[i], currentDate); cmp > 0 || (cmp == 0 && !drainAll) {
					continue // on or after the regular date — handled below
				}
				if drainIdx < 0 || dateutil.DateComp(nextDates[i], drainDate) < 0 {
					drainIdx = i
					drainDate = nextDates[i]
				}
			}
			// Off-cycle balloon: a balloon dated STRICTLY BEFORE this regular
			// payment is emitted at its own date too. DOS RepayFancyLoan applies a
			// balloon on its exact date — accruing partial interest up to it, then
			// the next regular period accrues only from the balloon date forward
			// (AMORTOP.pas:608-613, the balloonpos<0 branch). An odd first period
			// shifts the regular payment dates off the balloon's monthly grid,
			// which is exactly when a balloon lands between two regular dates; the
			// previous code folded it into the next payment (a few weeks late),
			// diverging from DOS. Pick the balloon date if it precedes the earliest
			// pending prepayment.
			// A balloon dated on OR BEFORE the earliest pending prepayment joins
			// this drain row. The `<=` is load-bearing: DOS's FindNextExtra
			// (AMORTOP.pas:486-543) builds ONE nextextra record per date, and its
			// balloon arm has a DateComp case 0 that MERGES into the prepayment
			// sum already accumulated in nextextra.amount rather than deferring
			// the balloon to a row of its own. See the merge rule below.
			drainBalloon := false
			if nextBalloon < len(input.Balloons) {
				bd := input.Balloons[nextBalloon].Date
				cmp := dateutil.DateComp(bd, currentDate)
				if (cmp < 0 || (cmp == 0 && drainAll)) &&
					(drainIdx < 0 || dateutil.DateComp(bd, drainDate) <= 0) {
					drainBalloon = true
					drainDate = bd
				}
			}
			if drainIdx < 0 && !drainBalloon {
				break
			}
			// Sum every event due exactly at drainDate, advancing each.
			//
			// DOS merges coincident extras into a SINGLE row (AMORTOP.pas:486-543,
			// FindNextExtra). Prepayment series sharing a date sum:
			//
			//	0: begin  {Extra payment comes from i as well as ...}
			//	     xsource := (xsource or (1 shl i));
			//	     nextextra.amount := nextextra.amount + pre[i]^.payment;
			//	   end;
			//
			// and a balloon on that same date then either ADDS to or REPLACES the
			// accumulated sum, depending on the Plus-Regular flag:
			//
			//	if (next_balloon <= nballoons) then
			//	  case DateComp(balloon[next_balloon]^.date, nextextra.date) of
			//	    0: begin
			//	         xsource := (xsource or FR_BALLOON);
			//	         if (df.c.plus_regular) then
			//	           nextextra.amount := nextextra.amount + balloon[...]^.amount
			//	         else
			//	           nextextra.amount := balloon[...]^.amount;
			//	       end;
			//
			// CheckOffBalloon (AMORTOP.pas:545-570) then consumes EVERY bit set in
			// xsource — so the prepayment series still advance (and still count
			// against their NN) even when the balloon replaced their amount. That
			// is why preSum is accumulated and the series advanced unconditionally
			// below, and only the emitted `offPay` follows the replace rule.
			//
			// Go's REGULAR-date coincidence path already implements this (§40d,
			// the `anyCoincident && balloonCoincident && !settings.PlusRegular`
			// block further down). Only this off-cycle drain was missing it: the
			// two arms used to be mutually exclusive, so a balloon and a
			// prepayment landing on the same off-cycle date produced TWO rows.
			//
			// 2026-07-29 fuzzer5 seed 40017 — verified against the real DOS engine:
			//
			//	amort_oracle 68684.07 0.0647090000 44 4 b68=18595.25 b74=19682.65 \
			//	  pre=14:80:12:117.95 payhard=2910.40
			//
			// The 9/1/2029 balloon 18595.25 coincides with the monthly 117.95
			// prepayment. DOS emits one row (int 404.64, prin 18190.61, bal
			// 56848.69 — payamt = the balloon alone); the port emitted `pay 117.95
			// int 404.64 bal 75325.99` followed by `pay 18595.25 int -0.00 bal
			// 56730.74`, and again at 3/1/2030 for b74. DOS 97 rows vs Go 99, and
			// dInt = -91.22 on the minimal case.
			var preSum, balSum float64
			havePre, haveBal := false, false
			if drainBalloon {
				// All balloons sharing this exact off-cycle date combine into one
				// dated row (their amount is the payment; principal reduction is
				// amount − accrued interest, computed below).
				for nextBalloon < len(input.Balloons) &&
					dateutil.DateComp(input.Balloons[nextBalloon].Date, drainDate) == 0 {
					balSum += input.Balloons[nextBalloon].Amount
					nextBalloon++
					haveBal = true
				}
			}
			for i := range input.Prepayments {
				pp := &input.Prepayments[i]
				if pp.PaymentStatus < types.InOutDefault || pp.PerYrStatus < types.InOutDefault ||
					pp.StartDateStatus < types.InOutDefault {
					continue
				}
				if prepayExhausted[i] {
					continue // next extra not representable — see prepayExhausted
				}
				if nextDates[i].IsUnknown() {
					nextDates[i] = pp.StartDate
				}
				// POST-application retirement — must stay identical to the
				// selection filter above, or the drain loop can pick a series
				// this block then skips, leaving nextDates unadvanced and the
				// `for {}` drain spinning forever.
				if prepayApplied[i] > 0 && pp.StopDateStatus >= types.InOutDefault &&
					dateutil.DateComp(nextDates[i], pp.StopDate) > 0 {
					continue
				}
				if pp.NNStatus >= types.InOutDefault && pp.NN > 0 && prepayApplied[i] >= pp.NN {
					continue
				}
				if dateutil.DateComp(nextDates[i], drainDate) == 0 {
					preSum += pp.Payment
					havePre = true
					prepayApplied[i]++
					if next, err := dateutil.AddPeriod(nextDates[i], pp.PerYr,
						pp.originDay(), false); err == nil {
						nextDates[i] = next
					} else {
						// Next extra is not representable — retire the series.
						// See prepayExhausted's declaration; without this the
						// cursor stays on the extra just paid and the drain loop
						// re-pays it without bound.
						prepayExhausted[i] = true
					}
				}
			}
			var offPay float64
			if havePre && haveBal && !settings.PlusRegular {
				offPay = balSum // REPLACE, per FindNextExtra's non-plus_regular arm
			} else {
				offPay = preSum + balSum
			}
			// Partial-period interest from the previous row's date to drainDate.
			// DOS computes the off-cycle row's timedif through the same
			// DaysCloseEnough-gated path as a regular row (ComputeNext sets
			// `date := nextextra.date` BEFORE the shared timedif block,
			// AMORTOP.pas:608-632), so an extra that lands a clean month from
			// the previous row accrues the month fraction, and only a genuine
			// mid-month span uses actual days. periodYearFraction encodes
			// exactly that (audit finding A1).
			// DOS trigger point for an off-cycle row (see applyPendingAdjustments):
			// an adjustment whose date this row has passed takes effect HERE, before
			// this row accrues, with p still the balance after the previous row.
			if applyPendingAdjustments(drainDate, payNum-1) {
				result.FinalPrinc = p
				return result
			}
			ydOff := dateutil.YearsDif(drainDate, prevDate, settings.Basis, settings.YrInv, true)
			var intOff float64
			if settings.Daily {
				expVal, _ := interest.Exxp(truerate * ydOff)
				intOff = (p - usap) * (expVal - 1)
			} else {
				intOff = loan.LoanRate * periodYearFraction(prevDate, drainDate, loan.PerYr, settings) * (p - usap)
			}
			if hardPayment {
				intOff = interest.Round2(intOff)
			}
			offCyclePaidOff := false
			// Cap the off-cycle payment at the remaining balance only on the
			// DISPLAY schedule. The solver's unforced terminal must apply the
			// extra in full and let the balance go negative — DOS's Iterate
			// criterion — otherwise every payment whose pre-extra balance the
			// extra overshoots yields a flat-zero residual and the bisection
			// is degenerate (audit finding A2: the trailing-balloon payment
			// solved $1.20 off inside that flat region).
			// DOS's fold threshold is the one-sided `< minpmt` ($1.00,
			// AMORTOP.pas:14), NOT `<= 0`, and it applies to EVERY event row —
			// off-cycle extras included — because RepayFancyLoan's repeat body
			// runs it on whatever ComputeNext produced:
			//
			//	if ((not h^.lastok) or (entire)) and (WhenToStop^.principal < minpmt)
			//	   and (not value_calc) then
			//	  begin
			//	    WhenToStop^.payamt := WhenToStop^.payamt + WhenToStop^.principal;
			//	    WhenToStop^.principal := 0;
			//	  end;                                     (AMORTOP.pas:1208-1211)
			//
			// The regular-row path at the bottom of this walk already uses this
			// threshold; the drain block was still capping at `<= 0`, so a series
			// extra that left a sub-dollar stub (fuzzer5 seed 21033: 0.12 left at
			// 7/14/2035) failed to retire the loan and the walk emitted one more
			// extra a period later, moving the reported terminal — and with it the
			// terminating balloon — 7 days late (DOS 7/14/2035 333.60 vs Go
			// 7/21/2035 294.31).
			if !unforced && p+intOff-offPay < minPmt {
				offPay = p + intOff
				offCyclePaidOff = true
			}
			p = p + intOff - offPay
			if settings.USARule {
				usap = usap + intOff - offPay
				if usap < 0 {
					usap = 0
				}
			}
			cumInt += intOff
			result.Schedule = append(result.Schedule, PaymentRecord{
				PayNum:    payNum,
				Date:      drainDate,
				PayAmt:    offPay,
				Interest:  intOff,
				Principal: p,
				IntToDate: cumInt,
			})
			result.TotalPaid += offPay
			result.TotalInt += intOff
			prevDate = drainDate
			if offCyclePaidOff && !unforced {
				result.FinalPrinc = p
				return result
			}
			// DOS's terminator runs after EVERY event row, not only after a
			// regular one. RepayFancyLoan's `repeat` body copies NextPayment,
			// calls ComputeNext — which yields whichever comes first, the next
			// regular payment or the next EXTRA (FindNextExtra, AMORTOP.pas:
			// 490-543) — folds, and then tests
			//
			//	until (((not h^.lastok) or (Output<>nil)) and (WhenToStop^.principal = 0))
			//	                                                  (AMORTOP.pas:1218)
			//
			// So on the very_last probe (Output=nil, entire=TRUE, and lastok
			// FALSE because DetermineLastPaymentDate never sets it — see
			// LoanInput.dosLastOK) an OFF-CYCLE extra that drives the balance
			// below minpmt stops the walk right there, exactly as the regular-row
			// check at the bottom of this loop does. Without this the drain keeps
			// paying extras out to very_last.
			//
			// Found 2026-07-29, fuzzer5 seed 21005:
			//
			//	amort_oracle 401633.75 0.0338160000 24 2 b365_360 plusreg \
			//	  firstdmy=28.11.2025 mor=72 b84=81243.95 b96=94037.35 \
			//	  b102=16246.29 pre=48:202:26:364.05 pre=90:46:24:314.41 \
			//	  payhard=21237.12 noterm
			//
			// The 5/28/2034 regular payment leaves 268.28; the biweekly series'
			// 6/10/2034 extra takes it to -95.44 and DOS stops. The port ran on to
			// the 6/24/2034 extra and tacked on -459.62 against DOS's -95.44 —
			// one whole extra (364.05 + interest) too far.
			if unforced && input.entireWalk && !input.dosLastOK && p < minPmt {
				result.FinalPrinc = p
				return result
			}
		}

		// Past the last REGULAR payment date, DOS emits NO further regular
		// payments: once the walk's regular date passes h^.lastdate, every
		// remaining extra is forced off-cycle (`if DateComp(date, lastdate) > 0
		// then balloonpos := -1` — AMORTOP.pas:602-613), so a trailing balloon
		// gets ONE row at its own date with all intervening interest accrued in
		// a single span. 2026-07-11 audit finding A2: this walk previously kept
		// paying the full regular payment out to veryLast, emitting phantom
		// regular rows. Verified vs the real DOS engine:
		//
		//	amort_oracle 100000 0.08 24 12 b30=50000 → payment 2668.85,
		//	interest 14052.47; rows jump from 1/1/26 straight to the 7/1/26
		//	balloon (Go emitted five phantom 2245.62 rows and solved 2245.62).
		//
		// Jump the regular date past the last pending extra so the off-cycle
		// drain block (top of this loop) emits each remaining extra at its own
		// date; when nothing is pending, the schedule is complete.
		// `adjLastDate`, NOT `loan.LastDate` (2026-07-31, discrepancies §53).
		// The DOS line this block cites — `if DateComp(date, h^.lastdate) > 0`,
		// AMORTOP.pas:606 inside ComputeNext — reads the h^.lastdate GLOBAL, and
		// that global is exactly what §50's adjustment pre-pass and every
		// Re_Amortize MUTATE through the unguarded VAR parameter at
		// AMORTOP.pas:1547. Round 9 ported the mutation but deliberately confined
		// the carrier to the NumberOfInstallmentsRaw site; this is the OTHER
		// reader, and leaving it on the pristine date made the port stop emitting
		// regular payments a month before DOS did.
		//
		// Worked example (§53's reduced case, adjustments at 30.9.2032 and
		// 30.7.2033, last payment 30.7.2034):
		//   adj 1: f=30.9.2032 is a month end so flast=true, llast=false, ddiff=0
		//          -> no month step, then `l.d := daysinm(l)` (INTSUTIL.pas:1018)
		//          -> lastdate 30.7.2034 becomes 31.7.2034
		//   adj 2: f=30.7.2033 flast=false, l=31.7.2034 llast=TRUE, ddiff=+1
		//          -> `l.m := l.m + monthsbtwn` fires (INTSUTIL.pas:1003), then
		//             `else l.d := f.d` restores day 30
		//          -> lastdate becomes 30.8.2034, a MONTH past the real last payment
		// so DOS's ComputeNext still calls 30.8.2034 a regular payment row and the
		// port jumped straight to the trailing prepayment. That one row is worth
		// 220.75 of interest on the reduced case and it also changes the segment
		// re-solve, because DOS's Iterate walks this same ComputeNext.
		if loan.LastOK && dateutil.DateComp(currentDate, adjLastDate) > 0 {
			var lastPending types.DateRec
			havePending := false
			if nextBalloon < len(input.Balloons) {
				// balloons are date-sorted; the final one bounds the jump.
				lastPending = input.Balloons[len(input.Balloons)-1].Date
				havePending = true
			}
			for i := range input.Prepayments {
				pp := &input.Prepayments[i]
				if pp.PaymentStatus < types.InOutDefault || pp.PerYrStatus < types.InOutDefault ||
					pp.StartDateStatus < types.InOutDefault {
					continue
				}
				if prepayExhausted[i] {
					continue // next extra not representable — see prepayExhausted
				}
				if nextDates[i].IsUnknown() {
					nextDates[i] = pp.StartDate
				}
				// POST-application retirement — same rule as the drain filters.
				if prepayApplied[i] > 0 && pp.StopDateStatus >= types.InOutDefault &&
					dateutil.DateComp(nextDates[i], pp.StopDate) > 0 {
					continue
				}
				if pp.NNStatus >= types.InOutDefault && pp.NN > 0 && prepayApplied[i] >= pp.NN {
					continue
				}
				if dateutil.DateComp(nextDates[i], veryLast) > 0 {
					continue // beyond the schedule's true end — never due
				}
				// The series' remaining extras run to its (explicit or derived)
				// stop bound; veryLast already reflects it, so jumping past
				// veryLast drains the whole tail.
				if !havePending || dateutil.DateComp(veryLast, lastPending) > 0 {
					lastPending = veryLast
				}
				havePending = true
			}
			if !havePending {
				break // regular schedule over, no trailing extras — done
			}
			// Equivalent to the old `jump := lastPending+1; if jump > currentDate`,
			// but without the Julian/MDY round-trip that made it fail past day
			// 70000 (26 Aug 2091). See drainAll's declaration.
			if dateutil.DateComp(lastPending, currentDate) >= 0 {
				currentDate = lastPending
				drainAll = true
				continue // drain block emits the trailing extras at their dates
			}
			break // (coverage: excluded — defensive: lastPending before currentDate)
		}

		// DOS trigger point for a regular row (see applyPendingAdjustments).
		if applyPendingAdjustments(currentDate, payNum-1) {
			result.FinalPrinc = p
			return result
		}
		// Compute interest for this period
		var intThisPd float64
		yd := dateutil.YearsDif(currentDate, prevDate, settings.Basis, settings.YrInv, true)
		// Per-period (month-based) year fraction for whole/clean periods — used by
		// the non-Daily accrual so the 365 basis doesn't skew the per-row split;
		// actual days only for partial/odd-day spans. Daily needs the true day
		// count (yd) for continuous compounding.
		ydReg := periodYearFraction(prevDate, currentDate, loan.PerYr, settings)

		if settings.Daily {
			expVal, _ := interest.Exxp(truerate * yd)
			intThisPd = (p - usap) * (expVal - 1)
		} else {
			intThisPd = loan.LoanRate * ydReg * (p - usap)
		}
		if hardPayment {
			intThisPd = interest.Round2(intThisPd)
		}

		// Skip-months: suppress the regular payment in a flagged calendar month.
		// The outer condition is purely a bounds check — MonthSet is indexed 1..12,
		// so guard the index before reading it — not a business rule; the inner
		// test is the actual "this month is skipped" decision.
		pmt := d
		if currentDate.Time.Month() > 0 && int(currentDate.Time.Month()) <= 12 {
			if input.SkipMonths.MonthSet[currentDate.Time.Month()] {
				pmt = 0
			}
		}

		// Check moratorium.
		//
		// Before FirstRepay: pay interest only (no principal reduction).
		//
		// There is NO re-solve of the payment at the FirstRepay boundary. DOS
		// solves ONE global payment `d` for the whole schedule and never
		// revisits it — EstimateAndRefinePayment (Amortize.pas:375-427) builds
		// the closed-form seed, takes the prepaid early-exit, and then refines
		// with a SINGLE call:
		//
		//	Iterate(p, usap, h^.loandate, h^.firstdate, d, til_adj)
		//
		// spanning the ENTIRE walk. The moratorium is not a segment boundary to
		// DOS at all; it is just a branch inside ComputeNext (AMORTOP.pas:640-653)
		// that the one global `d` has to satisfy along with everything else. The
		// solved `d` is therefore already sized so the loan retires despite the
		// interest-only window — which is exactly why AM_EX13 still lands on the
		// help's $2,152.63 with no boundary recompute at all.
		//
		// A boundary recompute used to live here. It was wrong in two ways:
		//
		//  1. It CLOBBERED the global `d`, and `d` is not only the payment — it
		//     is an operand of ComputeNext's coincident-extra arms, which read
		//     `payamt := payamt - d + ...` (AMORTOP.pas:641,646). So on any row
		//     carrying a balloon or prepayment the substituted `d` corrupted the
		//     arithmetic even where the regular payment itself looked plausible.
		//  2. DOS's global `d` is routinely NEGATIVE on moratorium + target
		//     screens (the target floor, not `d`, drives the post-moratorium
		//     rows). The recompute always re-solved to a positive annuity, so it
		//     could not reproduce those schedules at all.
		//
		// Verified against the real DOS engine, 2026-07-25 fuzzer5 triage — each
		// of these went from a four- to six-figure interest divergence to
		// row-for-row agreement when the recompute was removed:
		//
		//	amort_oracle 240082.84 0.1185770000 44 4 b365_360 inadv mor=63 \
		//	  b78=64744.09 b99=29352.65 pre=54:47:24:403.54 targ=1699.64 \
		//	  pts=0.035829
		//	→ d = -15822.0559, 83 rows, interest 193034.00 (Go had 262299.96)
		//
		//	amort_oracle 483614.33 0.0782320000 40 4 prepaid inadv mor=48 \
		//	  b54=59772.13 b57=102242.25 b96=102320.75 pre=69:38:24:680.91 \
		//	  pre=12:29:12:1038.96 targ=3460.67 pts=0.015258
		//	→ d = -10439.2763, 90 rows, interest 220493.77 (Go had 242869.73)
		//
		//	amort_oracle 375943.67 0.1295600000 24 1 exact mor=144 \
		//	  pre=72:47:26:155.41 pre=180:96:24:335.82 \
		//	  adj=228:0.0413830000:63806.36 targ=10332.26 pts=0.009825
		//	→ d = -72729.8817, 159 rows, interest 804922.13 (Go had 624265.24)
		//
		// When the user GAVE the payment DOS likewise uses it AS-IS through the
		// moratorium: the interest-only periods defer principal and the residual
		// rolls to the end.
		// The interest-only floor itself is applied AFTER the extras merge
		// below (audit finding A3): DOS's ComputeNext adjusts the MERGED payamt
		// — `payamt := payamt - d + interest` when a coincident extra is
		// present, plain `payamt := interest` otherwise (AMORTOP.pas:641-650) —
		// so the extra survives the interest-only floor.

		// Accumulate the extra payments (balloons + prepayment series) that
		// fall on THIS regular payment date, plus any off-cycle balloons whose
		// date falls strictly before it. Coincident extras combine with the
		// regular payment per PlusRegular (DOS Paymenttype.ComputeNext,
		// AMORTOP.pas:614-621): ON adds on top of the regular payment, OFF
		// (the default) REPLACES it — an additional-payment schedule. Off-cycle
		// PREPAYMENTS are emitted as their own dated rows by the draining block
		// at the top of this loop, not here.
		var coincidentExtra, offCycleExtra float64
		anyCoincident := false
		// Whether a BALLOON lands on this regular date, tracked separately from the
		// prepayment extras: in REPLACE mode DOS lets the balloon replace the
		// prepay sum, not add to it (see the balloonCoincident fold below).
		balloonCoincident := false
		var balloonExtra float64

		for nextBalloon < len(input.Balloons) {
			cmp := dateutil.DateComp(input.Balloons[nextBalloon].Date, currentDate)
			if cmp < 0 {
				// (coverage: excluded — defensive/unreachable: a balloon dated
				// strictly before currentDate is emitted at its own date by the
				// off-cycle drain block at the top of this loop, so by the time
				// execution reaches here every remaining balloon is on or after
				// currentDate. This legacy fold is kept as a fallback.)
				offCycleExtra += input.Balloons[nextBalloon].Amount
				nextBalloon++
			} else if cmp == 0 {
				coincidentExtra += input.Balloons[nextBalloon].Amount
				balloonExtra += input.Balloons[nextBalloon].Amount
				balloonCoincident = true
				anyCoincident = true
				nextBalloon++
			} else {
				break
			}
		}

		// Prepayment series coincident with this regular date.
		//
		// Mirrors DOS FindNextExtra at AMORTOP.pas:490-572: each active series
		// has a NextDate starting at StartDate; when NextDate matches the
		// current period it is an extra on (or replacing) the regular payment,
		// then NextDate advances by 12/PerYr months. When NextDate passes
		// StopDate (or NN extras have been applied) the series is exhausted.
		for i := range input.Prepayments {
			pp := &input.Prepayments[i]
			if pp.PaymentStatus < types.InOutDefault || pp.PerYrStatus < types.InOutDefault {
				continue
			}
			if pp.StartDateStatus < types.InOutDefault {
				continue
			}
			if prepayExhausted[i] {
				continue // next extra not representable — see prepayExhausted
			}
			if nextDates[i].IsUnknown() {
				nextDates[i] = pp.StartDate
			}
			// Stop-date retirement is POST-APPLICATION — see the matching note
			// in the off-cycle drain above. DOS retires a series only after it
			// has paid an extra and its advanced nextdate has passed stopdate
			// (CheckOffBalloon, AMORTOP.pas:545-572), so a crossed window
			// (stop < start, as produced by the post-term-solve rewrite at
			// AMORTOP.pas:1335-1380) still pays exactly one extra.
			if prepayApplied[i] > 0 && pp.StopDateStatus >= types.InOutDefault &&
				dateutil.DateComp(nextDates[i], pp.StopDate) > 0 {
				continue
			}
			// NN-bounded series: once NN extra payments have been applied, the
			// series is exhausted even if no StopDate was supplied. Mirrors DOS
			// Paymenttype.ComputeNext. See dispatch_gaps AO8.
			if pp.NNStatus >= types.InOutDefault && pp.NN > 0 &&
				prepayApplied[i] >= pp.NN {
				continue
			}
			if dateutil.DateComp(nextDates[i], currentDate) == 0 {
				coincidentExtra += pp.Payment
				anyCoincident = true
				prepayApplied[i]++
				next, err := dateutil.AddPeriod(nextDates[i], pp.PerYr,
					pp.originDay(), false)
				if err == nil {
					nextDates[i] = next
				} else {
					// Not representable — retire. See prepayExhausted.
					prepayExhausted[i] = true
				}
			}
		}

		// REPLACE-mode balloon × prepayment collision (§40d). DOS builds nextextra
		// from the prepayment series first, then folds the balloon in — and in
		// REPLACE mode the balloon OVERWRITES the accumulated prepay sum rather
		// than adding to it (FindNextExtra, AMORTOP.pas:518-527):
		//
		//	if (next_balloon <= nballoons) then
		//	  case DateComp(balloon[next_balloon]^.date, nextextra.date) of
		//	    0: begin
		//	         xsource := (xsource or FR_BALLOON);
		//	         if (df.c.plus_regular) then
		//	           nextextra.amount := nextextra.amount + balloon[...]^.amount
		//	         else
		//	           nextextra.amount := balloon[...]^.amount;
		//	       end;
		//
		// The prepay series are still CONSUMED (their xsource bits reach
		// CheckOffBalloon at :545-570, so nextdate advances and an NN-bounded
		// series still burns an installment) — they simply are not paid. The port
		// summed them, overpaying every balloon row that collided with a
		// prepayment by the extra's amount. 2026-07-25 fuzzer5 pass 2 — verified
		// vs the real DOS engine:
		//
		//	amort_oracle 372389.04 0.0642960000 288 12 b365_360 prepaid mor=77 \
		//	  b80=92172.31 b93=89306.11 b156=41955.00 pre=121:348:52:36.03 \
		//	  pre=73:208:24:232.34 adj=33:0.0861800000:2797.05 adj=104::3253.89 \
		//	  adj=206::2331.35 targ=479.38 pts=0.002197 payhard=3012.87
		//	→ 9/1/2030 (balloon 92172.31 collides with the semi-monthly 232.34):
		//	  DOS pays exactly 92172.31; the port paid 92404.65.
		if anyCoincident && balloonCoincident && !settings.PlusRegular {
			coincidentExtra = balloonExtra
		}
		if anyCoincident {
			if settings.PlusRegular {
				pmt += coincidentExtra
			} else {
				pmt = coincidentExtra
			}
		}
		pmt += offCycleExtra

		// Moratorium interest-only / target floors — applied to the MERGED
		// payment, exactly mirroring DOS ComputeNext's balloonpos case
		// (AMORTOP.pas:641-654). DOS gives the MORATORIUM precedence (the
		// interest-only branch comes before the target branch, an else-if), so a
		// target does NOT force principal during the interest-only window (see
		// docs/amort_option_combo_divergences.md for the mor+targ history).
		//
		// 2026-07-11 audit finding A3: when a coincident extra is present
		// (balloonpos=0) DOS KEEPS the extra component — `payamt := payamt − d +
		// interest` (moratorium) / `payamt := payamt − d + targ + interest`
		// (target) — while the plain-regular row (balloonpos=1) takes the flat
		// `interest` / `targ + interest`. The port previously applied the flat
		// form to coincident rows too, dropping the `payamt − d` term. Verified
		// vs the real DOS engine:
		//
		//	amort_oracle 100000 0.08 60 12 pay=1500 targ=800 b12=500 rows
		//	→ row 1/1/25: pay 403.48 (= 500 − 1500 + 800 + 603.48), bal 90721.58
		//	(the port paid 1403.48 = flat 800 + 603.48).
		inMoratorium := input.Moratorium.FirstRepayStatus >= types.InOutDefault &&
			dateutil.DateComp(currentDate, input.Moratorium.FirstRepay) < 0
		if inMoratorium {
			if anyCoincident {
				pmt = pmt - d + intThisPd
			} else {
				pmt = intThisPd
			}
		} else if input.Target.TargetStatus >= types.InOutDefault {
			if pmt-intThisPd < input.Target.TargetValue {
				if anyCoincident {
					pmt = pmt - d + input.Target.TargetValue + intThisPd
				} else {
					pmt = input.Target.TargetValue + intThisPd
				}
			}
		}

		// In-advance (annuity-due) fancy loans: DOS accrues ORDINARY
		// opening-balance interest on the settlement-shifted schedule (set up
		// before the loop as inAdvanceFancy) — which is exactly intThisPd as
		// already computed above (loanrate * ydReg * (p - usap)). So NO
		// per-period recompute is applied here; the annuity-due behaviour comes
		// from the schedule STRUCTURE (settlement row + one-period base shift),
		// not an interest-formula tweak. See AMORTOP.pas:636 (ComputeNext uses
		// plain in-arrears interest even when in_advance is set) and
		// docs/dos_known_frontier.md #38.

		// Early payoff: if this period's payment would clear the
		// balance (or overshoot it negative — which happens once
		// prepayments or a balloon accelerate the loan), trim the
		// payment so the balance lands exactly on zero and stop.
		// Mirrors DOS WhenToStop, which folds the residual principal
		// into the final payment.
		payoffNow := false
		// DOS decides "this is the very last row" by DATE, not by payment count:
		// PrintAndReset (AMORTOP.pas:1004) is literally
		//
		//	if (DateComp(date,very_last)=0) then begin
		//	  {Adjust last payment to cover entire remaining principal.}
		//	  payamt:=payamt+principal; cumamt:=cumamt+principal; principal:=0;
		//	  end;
		//
		// with no gate on in_advance and no gate on which options are present. The
		// port used `payNum >= loan.NPeriods` as a proxy for that test. The proxy
		// holds in arrears, but under interest-in-advance the annuity-due geometry
		// puts the FINAL scheduled row at payNum = NPeriods-1 (the settlement row
		// consumes the extra slot), so `payNum >= NPeriods` is never true and the
		// fold silently never fires. That is why each branch below had to carry a
		// `!settings.InAdvance` guard — the guards were papering over the wrong
		// terminal test, not encoding a real DOS distinction. Testing the date, as
		// DOS does, covers both geometries and lets the guards go.
		// 2026-07-24 fuzzer5 finding: 37 of 92 divergent stacked-option cases were
		// in-advance loans whose interest matched DOS to the cent and whose totals
		// were short by exactly the unfolded residual, e.g.
		//
		//	amort_oracle 37518.85 0.1320700000 14 1 exact inadv mor=84 		//	  b132=1164.09 payhard=8055.34
		//	→ final row int 1834.09 prin 13887.29 bal 0.00, paid 94948.17
		//	  (Go left bal 7666.04, paid 87282.13; every other row to the cent)
		atVeryLast := dateutil.DateComp(currentDate, veryLast) >= 0
		terminalRow := payNum >= loan.NPeriods || atVeryLast

		// A plain loan routed through the fancy engine (no advanced options) is a
		// normal amortization that must retire on its FINAL scheduled payment. DOS
		// folds any residual into that last payment so the balance lands exactly on
		// zero (WhenToStop / the simple schedule's "Last payment: adjust to pay off
		// remaining balance"). Loans reach the fancy engine "plain" when forced
		// there by the exact method OR the US-Rule routing; without this clear they
		// leave a few cents/dollars of under-amortization on the last row. In-advance
		// has its own annuity-due final-row handling, so it is excluded; any
		// balloon/prepayment/adjustment/target/moratorium/skip keeps the existing
		// well-tested terminal behaviour.
		plainFancy := !settings.InAdvance && len(input.Balloons) == 0 &&
			len(input.Prepayments) == 0 && len(input.Adjustments) == 0 &&
			input.Target.TargetStatus < types.InOutDefault &&
			input.Moratorium.FirstRepayStatus < types.InOutDefault &&
			!anySkip(input.SkipMonths.MonthSet)

		// probeARMRow: this row is the one DOS's Re_Amortize recomputes for ITSELF,
		// and we are standing in for DOS's DetermineLastPaymentDate probe walk.
		// There, and ONLY there, the residual fold is written and then thrown away.
		//
		// DOS's probe walk runs RepayFancyLoan with Output = nil and adjnum = 0, so
		//	WhenToStop := @NextPayment                        (AMORTOP.pas:1127-1130)
		// — the LOOKAHEAD row, not the committed one. Inside one iteration of the
		// main repeat loop the order is
		//	NextPayment.ComputeNext(p, usapart);              (AMORTOP.pas:1206)
		//	if ((not h^.lastok) or (entire)) and (WhenToStop^.principal < minpmt)
		//	   and (not value_calc) then begin
		//	     WhenToStop^.payamt := WhenToStop^.payamt + WhenToStop^.principal;
		//	     WhenToStop^.principal := 0;                  (AMORTOP.pas:1208-1211)
		//	   end;
		//	... Re_Amortize ...                               (AMORTOP.pas:1216)
		// and Re_Amortize's own tail rebuilds the very row that was just folded:
		//	NextPayment := Payment;
		//	NextPayment.ComputeNext(p, usap);                 (AMORTOP.pas:1604-1610)
		// so both the folded payamt and the `principal := 0` are wiped. The loop
		// terminator
		//	until (((not h^.lastok) or (Output<>nil)) and (WhenToStop^.principal = 0))
		//	                                                  (AMORTOP.pas:1218)
		// tests `principal = 0` EXACTLY, and only the (now-erased) fold ever sets it,
		// so the probe walk does NOT stop on the ARM row — it emits one more period,
		// and can chain if that period is itself an ARM row.
		//
		// The DISPLAY walk cannot hit this: it has Output <> nil, so
		// WhenToStop = @Payment, and Re_Amortize only READS Payment. That asymmetry
		// is the whole bug: Go emulates DOS's probe with a display walk
		// (solveFancyTermFromPayment → Amortize), so without this the solved term
		// comes back one period short whenever the retiring row is the ARM row.
		//
		// 2026-07-29 fuzzer5 seed 21048 (rotation cycle 33):
		//	479830.30 0.1193830000 11 1 plusreg loandmy=6.11.2024 firstdmy=6.11.2025 \
		//	  mor=48 b60=142213.99 b72=89002.59 adj=84:0.1401610000:88074.72 \
		//	  payhard=87136.86 noterm
		//	DOS n=9 last=11/6/2033   port n=8 last=11/6/2032
		// Row 8 (11/6/2032) is the first row strictly after adj=84 (11/6/2031), so
		// DOS folds its old-rate balance (59455.91 + 7098.00 − 87136.86 ≈ −20582.95)
		// to 0, then Re_Amortize rebuilds it at 14.0161%/88074.72 as
		// int 8333.40 / prin −20285.41 — unfolded. Row 9 then carries
		// int −2843.22, prin −111203.35, payamt −23128.63, and the walk stops there.
		// With the term forced to 9 the whole schedule already matched DOS to the
		// cent, so the term solve was the sole defect.
		//
		// reAmortRowIdx is set by applyPendingAdjustments to len(result.Schedule) at
		// the moment the crossing fires — i.e. the index the row now being built will
		// occupy — so equality here means "this row IS the Re_Amortize row".
		// The gate covers BOTH walks that run with WhenToStop = @NextPayment:
		//   * input.termHorizonWalk — DetermineLastPaymentDate's probe, which Go
		//     emulates with a display walk (so `unforced` is false there), and
		//   * unforced — every genuine Output=nil walk, of which the one that
		//     matters here is EstimateAndRefineBalloon's very_last/tack-on probe
		//     (entireWalk). Iterate's trial walks strip their adjustments
		//     (fancyTerminal), so reAmortRowIdx never leaves -1 for them.
		probeARMRow := (input.termHorizonWalk || unforced) && reAmortRowIdx >= 0 &&
			len(result.Schedule) == reAmortRowIdx

		if unforced || probeARMRow {
			// Unforced Newton-terminal mode (RepayFancyLoan Output=nil): apply the
			// regular payment/options as-is and never fold — the residual balance IS
			// the Newton signal. The one-sided minpmt stop is applied after the row.
		} else if p+intThisPd-pmt < minPmt {
			// DOS's per-row fold: ANY post-payment balance below minpmt ($1.00) —
			// not just <= 0 — folds into THIS row's payment and retires the loan
			// (`WhenToStop^.principal < minpmt → payamt += principal; principal :=
			// 0`, AMORTOP.pas:1208-1211; display mode has entire=true so the gate
			// `(not lastok) or entire` always passes). A positive sub-$1 residual
			// before the last scheduled row was previously dropped (the walk broke
			// without folding). 2026-07-22 penny-scale finding — see
			// dos_pennyfold_settlement_test.go (exact_arrears case).
			pmt = p + intThisPd
			payoffNow = true
		} else if plainFancy && terminalRow && p+intThisPd-pmt > 0 {
			// Final scheduled payment of a plain loan: DOS's very-last fold
			// retires ANY residual into it — large neg-am residuals included
			// (PrintAndReset, AMORTOP.pas:~1004). 2026-07-11 pass-2 finding 1:
			// the previous `residual < one payment` gate was a port invention
			// ("left for the advisory") that left USA/exact non-360 neg-am
			// loans unretired, inconsistent with Go's own 360-basis simple
			// path. Verified vs the real DOS engine:
			//
			//	amort_oracle 100000 0.15 36 12 b365 usa payhard=900
			//	→ interest 45000.00 paid 145000.00, final row int 1250.00
			//	  prin 112250.00 bal 0.00 (Go left bal 112600, paid 32400)
			//	amort_oracle 100000 0.15 36 12 b365 exact payhard=900
			//	→ paid 148179.40
			pmt = p + intThisPd
			payoffNow = true
		} else if terminalRow && len(input.Adjustments) > 0 && atVeryLast &&
			p+intThisPd-pmt > 0 {
			// THE `atVeryLast` GUARD (2026-07-31, discrepancies §52). DOS's display
			// very-last fold is keyed on `very_last` and on nothing else —
			// PrintAndReset, AMORTOP.pas:1004:
			//
			//	if (DateComp(date,very_last)=0) then begin
			//	 {Adjust last payment to cover entire remaining principal.}
			//	    payamt:=payamt+principal;
			//	    cumamt:=cumamt+principal;
			//	    principal:=0;
			//	    end;
			//
			// and `very_last` is max(h^.lastdate, last balloon date, every
			// prepayment stopdate) — DetermineVeryLast, AMORTOP.pas:1293-1304.
			// It is NOT the last regular payment date whenever a trailing option
			// runs past it.
			//
			// `terminalRow` is `payNum >= loan.NPeriods || atVeryLast`, so it goes
			// true at the last REGULAR payment even when veryLast is later. Without
			// the guard this branch folded the residual there and stopped the walk,
			// swallowing every trailing prepayment-only row. The prepayment branch
			// below and the dateless-option branch after it both already carried the
			// guard for exactly this reason; the ARM branch is checked FIRST, so on
			// a loan carrying BOTH an adjustment and a trailing prepayment series it
			// won the race and the guard on the later branch never got a chance.
			//
			// Measured before the guard (`amort_oracle 100000 0.08 180 12
			// loandmy=1.1.2024 firstdmy=1.2.2024 adj=44:0.0706: pre=21:45:2:351.90`):
			// DOS emits 200 rows and 68710.43 of interest, walking the prepay series
			// out to its stop date; the port emitted 180 rows and 67086.46, dumping
			// 5662.07 of principal into row 180. Removing ANY one of the three
			// conditions — the adjustment, the trailing overshoot, or a prepay
			// frequency below the payment frequency — made the port agree, which is
			// what made this look like an exotic interaction rather than a missing
			// guard.
			//
			// Final scheduled payment of an ARM whose plain re-amortization left a
			// residual — most visibly with skipped months, where DOS keeps the
			// skip-blind annuity after the reset and the loan negative-amortizes,
			// so the LAST scheduled payment balloons to pay off the remaining
			// balance (DOS WhenToStop / "Last payment: adjust to pay off remaining
			// balance"; the oracle's final row shows the dumped principal). Interest
			// is unchanged — it already accrued on these balances — only the final
			// payment and balance move, retiring the loan to $0 like DOS. (Plain
			// ARMs without a residual fold ~nothing; over-amortizing ARMs retire
			// early and never reach this branch.)
			pmt = p + intThisPd
			payoffNow = true
		} else if terminalRow && len(input.Balloons) > 0 &&
			nextBalloon >= len(input.Balloons) && len(input.Prepayments) == 0 &&
			p+intThisPd-pmt > 0 {
			// Final scheduled payment of a balloon-bearing loan with a residual —
			// DOS's display very-last fold retires it into this payment
			// (PrintAndReset, AMORTOP.pas:~1004). 2026-07-11 audit finding 20b:
			// with a hard payment below interest (neg-am) and a balloon in
			// REPLACE mode, the schedule previously ended with the residual as a
			// huge final balance and totals short by it. Verified vs the real
			// DOS engine:
			//
			//	amort_oracle 100000 0.08 240 12 payhard=600 usa b60=20000
			//	→ paid 235543.41, final row L239: 72743.41 → bal 0.00
			//	(Go left final balance 72143.41, paid 163400.00; rows to the
			//	cent otherwise. Non-USA control: DOS paid 238513.72. ADD-mode
			//	already folded correctly.)
			//
			// Gated on all balloons consumed (a TRAILING balloon's tail is the
			// off-cycle drain's job) and no prepayment series (their fold has
			// its own branch below with a veryLast guard for NN-derived
			// trailing rows).
			pmt = p + intThisPd
			payoffNow = true
		} else if terminalRow && len(input.Prepayments) > 0 && atVeryLast &&
			p+intThisPd-pmt > 0 {
			// Final row of a prepayment-series loan with a residual — DOS's
			// display very-last fold retires it into this payment exactly as
			// for plain/balloon loans (PrintAndReset, AMORTOP.pas:~1004).
			// 2026-07-12 pass-3 finding AF6 — verified vs the real DOS engine:
			//
			//	amort_oracle 50000 0.08 60 12 payhard=900 pre=6:12:12:200
			//	→ final row int 138.15 prin 20722.07 bal 0.00, paid 65560.22
			//	  (Go previously left bal 19960.22 unretired)
			//	amort_oracle 200000 0.09 480 12 payhard=1600 pre=12:120:12:100
			//	→ paid 4258344.89 (Go previously 588000.00)
			//
			// The veryLast guard keeps NN-derived TRAILING prepay rows (which
			// run past the last regular payment) ahead of the fold.
			pmt = p + intThisPd
			payoffNow = true
		} else if terminalRow &&
			len(input.Balloons) == 0 && len(input.Prepayments) == 0 &&
			len(input.Adjustments) == 0 &&
			atVeryLast && p+intThisPd-pmt > 0 {
			// Final scheduled row of a DATELESS-option loan (skip-months and/or
			// moratorium and/or principal-minimum, with no balloon / prepayment /
			// adjustment) carrying a residual. DOS's display very-last fold retires
			// ANY residual into the last row (PrintAndReset, AMORTOP.pas:~1004) —
			// the same rule the plain / ARM / balloon / prepayment branches above
			// already implement. This class was the hole: `plainFancy` excludes it
			// by construction (it requires NO advanced option) and the three
			// option-specific branches key on the presence of a DATED option, so a
			// skip-only, moratorium-only or target-only loan routed to the
			// piecewise engine (i.e. under the exact method — a non-exact loan goes
			// to AmortizeDOS, which folds correctly) ended its table with the
			// residual left outstanding and its totals short by exactly that
			// residual. 2026-07-24 §46 — verified vs the real DOS engine:
			//
			//	amort_oracle 100000 0.08 360 12 b365_360 exact prepaid pts=0 \
			//	  loandmy=1.1.2025 firstdmy=1.2.2025 payhard=800.00 skip=1,3,5
			//	→ interest 333436.00 paid 433436.00
			//	  (Go: paid 216000.00, final row pay 0.00, bal 217436.00 left)
			//	… same loan skip=2,3,5 → paid 440966.64 (Go 216000.00)
			//	amort_oracle … 120 12 … payhard=700.00 targ=50.00
			//	→ interest 78531.31 paid 178531.31 (Go paid 85327.72)
			//
			// DOS folds even when the terminal month is itself SKIPPED (the
			// skip=1,3,5 case ends on a January): the skip zeroes the REGULAR
			// payment, it does not suppress the terminating payoff. Interest is
			// unchanged either way — it has already accrued on these balances —
			// only the final payment and the closing balance move.
			pmt = p + intThisPd
			payoffNow = true
		} else if inAdvanceFancy && !hasAnyAdvancedOption(input) && loan.LastOK &&
			atVeryLast && p+intThisPd-pmt > 0 {
			// Last scheduled row of a PLAIN in-advance loan rendered by the fancy
			// engine (a non-360 basis, routed here for actual-day display while the
			// payment was solved by the simple annuity-due RepayLoan model). That
			// annuity-due payment does not retire the actual-day schedule, so DOS's
			// WhenToStop folds the whole remaining balance into this final row (the
			// oracle's last row shows prin = the dumped balance, bal → 0). Match it.
			pmt = p + intThisPd
			payoffNow = true
		}

		// Apply payment
		p = p + intThisPd - pmt
		if settings.USARule {
			usap = usap + intThisPd - pmt
			if usap < 0 {
				usap = 0
			}
		}
		cumInt += intThisPd

		result.Schedule = append(result.Schedule, PaymentRecord{
			PayNum:    payNum,
			Date:      currentDate,
			PayAmt:    pmt,
			Interest:  intThisPd,
			Principal: p,
			IntToDate: cumInt,
		})

		result.TotalPaid += pmt
		result.TotalInt += intThisPd

		// Check termination conditions
		if payoffNow {
			if payNum < loan.NPeriods {
				result.Warnings = append(result.Warnings, fmt.Sprintf(
					"Loan retired early — paid off at payment %d of a scheduled %d.",
					payNum, loan.NPeriods))
			}
			break
		}
		// Balance is essentially zero (within one minPmt either side): the loan has
		// Unforced Newton-terminal mode (RepayFancyLoan called from Iterate,
		// Output=nil): DOS does NOT stop early or fold — verified by tracing DOS's
		// per-period ComputeNext at an overpaying trial (docs/iterate_collapse_plan.md):
		// it applies the FULL regular payment every period through very_last, so an
		// overpayment drives the balance well negative (e.g. −82207 at period n) rather
		// than stopping at the first sub-minpmt crossing (−26488). Running the full term
		// makes the terminal a single-zero (monotone) function of the payment — the
		// spurious second zero came from the early stop. Only the display path stops.
		if !unforced && !probeARMRow && p < minPmt && p > -minPmt {
			// Display mode: balance essentially zero (within one minPmt either side)
			// ⇒ the loan retired, so stop even if scheduled periods remain.
			//
			// !probeARMRow: on the probe walk's Re_Amortize row DOS's stop test is
			// `WhenToStop^.principal = 0` EXACTLY (AMORTOP.pas:1218) and the only
			// writer of that 0 — the fold — has just been clobbered by Re_Amortize's
			// tail. A tolerance band here would stop one period early for the same
			// reason the fold would. See the probeARMRow note above.
			break
		}
		// ...EXCEPT when DOS's own h^.lastok is false. The comment above is written
		// for Iterate's trial walk, where lastok IS true (Iterate only runs on a
		// screen that already carries a term) and DOS therefore does run the full
		// term. It does not cover the OTHER Output=nil caller,
		// EstimateAndRefineBalloon's very_last probe (entire=TRUE ⇒ entireWalk),
		// which on a solved-term screen runs with lastok FALSE. There the first arm
		// of the terminator
		//	until (((not h^.lastok) or (Output<>nil)) and (WhenToStop^.principal = 0))
		//	                                                       (AMORTOP.pas:1218)
		// reduces to `(not lastok) and (principal = 0)`, and the entire-walk fold
		//	if ((not h^.lastok) or (entire)) and (WhenToStop^.principal < minpmt)
		//	   and (not value_calc) then begin
		//	     WhenToStop^.payamt := WhenToStop^.payamt + WhenToStop^.principal;
		//	     WhenToStop^.principal := 0;
		//	   end;                                          (AMORTOP.pas:1208-1211)
		// sets principal to 0 on the FIRST row whose balance falls below minpmt
		// (one-sided, so negative balances trip it too). So DOS stops there.
		//
		// The fold itself needs no reproduction: it is sum-preserving, and the
		// probe reads `nextpayment.payamt + nextpayment.principal`, which the
		// engine reads back as lastRow.PayAmt + FinalPrinc.
		//
		// dosport_walk.go:144/:188 already models this pair correctly; this is the
		// piecewise engine catching up. 2026-07-29 fuzzer5 Class E-3b (amount half).
		//
		// !probeARMRow: same clobber as above. This stop models the fold, and on
		// the Re_Amortize row Re_Amortize's tail (AMORTOP.pas:1604-1610) rebuilds
		// NextPayment from scratch, erasing the `principal := 0` the fold wrote —
		// so DOS emits one more period. 2026-07-29 fuzzer5 seed 21048, tack-on
		// half: the port reported the ARM row's own principal (−20285.41) as the
		// terminating balloon instead of the following row's (−111203.35).
		if unforced && input.entireWalk && !input.dosLastOK && p < minPmt && !probeARMRow {
			break
		}
		// Reached the schedule's true end: veryLast is the LATEST of the last regular
		// payment date and any later balloon / prepayment-stop date (computed above),
		// so this only fires once every dated event has been emitted. In unforced mode
		// veryLast bounds the walk unconditionally (LastOK may be unset for a trial x).
		if (unforced || loan.LastOK) && dateutil.DateComp(currentDate, veryLast) >= 0 {
			break
		}

		// Advance to next date
		prevDate = currentDate
		nextDate, err := dateutil.AddPeriod(currentDate, loan.PerYr, origDay, false)
		if err != nil {
			result.Err = err
			return result
		}
		currentDate = nextDate

	}

	result.FinalPrinc = p
	return result
}

// MonthSetFromString parses a skip-months string like "6-8" or "1,6,12"
// into a boolean array indexed by month (1-12).
//
// Ported from legacy/source/Amortize.pas: function MonthSetFromString
func MonthSetFromString(s string) ([13]bool, error) {
	var monthSet [13]bool
	if s == "" {
		return monthSet, nil
	}

	i := 0
	var lastN int
	thruflag := false

	for i < len(s) {
		// Skip non-digit, non-dash chars
		for i < len(s) && !isDigit(s[i]) && s[i] != '-' {
			i++
		}
		if i >= len(s) {
			break
		}

		if s[i] == '-' {
			thruflag = true
			i++
			continue
		}

		// Parse 1-2 digit number
		n := int(s[i] - '0')
		i++
		if i < len(s) && isDigit(s[i]) {
			n = n*10 + int(s[i]-'0')
			i++
		}

		if n < 1 || n > 12 {
			return monthSet, fmt.Errorf("Skip Months contains the month number %d, "+
				"which is out of range. Use month numbers 1 through 12, for example "+
				"\"6-8,12\".", n)
		}

		if thruflag {
			if lastN == 0 {
				return monthSet, fmt.Errorf("Skip Months has a range dash with no " +
					"starting month. Write a range as start-end, for example \"6-8\".")
			}
			if lastN <= n {
				for m := lastN; m <= n; m++ {
					monthSet[m] = true
				}
			} else {
				// Wrap around: e.g. 10-2 means Oct,Nov,Dec,Jan,Feb
				for m := lastN; m <= 12; m++ {
					monthSet[m] = true
				}
				for m := 1; m <= n; m++ {
					monthSet[m] = true
				}
			}
			thruflag = false
		} else {
			monthSet[n] = true
		}
		lastN = n
	}

	return monthSet, nil
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// BalanceAtDate returns the outstanding loan balance as of `date`,
// given a generated schedule. It reads the balance the engine
// already recorded after each payment (PaymentRecord.Principal), so
// the result is correct even when the schedule contains balloons,
// rate adjustments, prepayments, or a moratorium — unlike a
// payment-minus-interest walk, which drifts on those.
//
// Ported from legacy/src/dos_source/Amortize.pas: procedure
// ComputeBalanceFromDate.
func BalanceAtDate(schedule []PaymentRecord, loanAmount float64, date types.DateRec) float64 {
	bal := loanAmount
	for i := range schedule {
		if dateutil.DateComp(schedule[i].Date, date) > 0 {
			break
		}
		bal = schedule[i].Principal
	}
	// DOS's ComputeBalanceFromDate (Amortize.pas:1090-1150) does not clamp a
	// negative (over-funded) balance to zero — it reports the actual signed
	// remainder. Removed the DOS-absent `if bal < 0 { bal = 0 }` clamp so this
	// helper is faithful to the DOS engine (pass 11, 2026-07-14).
	return bal
}

// applyAPR computes the DOS-faithful APR (A9, EstimateAndRefineAPRwithPoints) and
// stores it on res when discount points are present. Shared by the piecewise
// engine and the AmortizeDOS delegation so ARM/option loans — which route to
// AmortizeDOS and would otherwise return with APR=0 — get an APR too. The present
// value is discounted over the FULL-term value stream (aprValueCashflows), not the
// truncated display schedule (see the §9 APR fix).
func applyAPR(res *AmortResult, input LoanInput, loan Loan, settings *Settings, payment, truerate, f float64) {
	if res.Err != nil || loan.PointsStatus < types.InOutDefault || len(res.Schedule) == 0 {
		return
	}
	prepaid, _ := PrepaidInterest(&loan, settings, truerate)
	netProceeds := loan.Amount*(1-loan.Points) - prepaid
	aprSched := aprValueCashflows(input, payment, settings, truerate, f, res.Schedule)
	apr, conv, overflow := ComputeAPRWithPoints(aprSched, loan.LoanDate, netProceeds,
		loan.LoanRate, byte(loan.PerYr), settings)
	if overflow {
		// DOS's exxp sets errorflag as well as overflowflag
		// (INTSUTIL.pas:1150-1151), and errorflag is the engine-wide abort: the
		// screen refuses and MakeTable emits NO table, not merely a blank APR.
		// Verified against the real DOS engine, 2026-07-25 fuzzer5 pass 2:
		//
		//	amort_oracle 228705.18 0.1184100000 168 12 prepaid mor=74 \
		//	  b76=13746.09 b109=39014.47 pre=3:307:24:112.28 pre=8:251:52:90.98 \
		//	  adj=15:0.0995710000:2108.84 adj=30::3427.66 adj=119::2317.34 \
		//	  targ=109.74 pts=0.035592 payhard=2656.35
		//	→ ERR Overflow error: answer too large for this computer's numeric
		//	  format.
		//
		// Drop the `pts=` token from that same line and DOS produces the table
		// happily (interest -320653.18, which the port matches row for row), so
		// the refusal is the APR secant's, not the walk's: the moratorium leaves
		// the schedule value-negative, the secant drives v_rate deeply negative,
		// and -v_rate * YearsDif crosses 70. The port used to report that
		// 555-row schedule with a fabricated APR.
		res.APR = 0
		res.APRConverged = false
		// errorflag condemns the TABLE too, not just the APR field: the oracle
		// Halts before it prints a single row, and the DOS screen shows the
		// message box in place of a schedule. Drop the rows so the port refuses
		// as completely as the engine it mirrors.
		res.Schedule = nil
		res.TotalInt = 0
		res.TotalPaid = 0
		res.FinalPrinc = 0
		res.Err = fmt.Errorf("Overflow error: answer too large for this "+
			"computer's numeric format. The Annual Percentage Rate could not "+
			"be refined for this screen: %w", interest.ErrOverflow)
		return
	}
	res.APR = apr
	res.APRConverged = conv
}

// DateForBalance is the inverse of BalanceAtDate: it returns the
// first payment date on which the outstanding balance has fallen to
// or below `target`. The bool is false when the balance never
// reaches the target within the schedule.
//
// NOTE: this is a naive POST-payment row-snap and it does NOT match DOS's
// ComputeDateFromBalance (which the frontend also reimplements client-side). Use
// ComputeDateFromBalanceDOS for the DOS-faithful result. Retained for the callers
// that still depend on the row-snap semantics.
func DateForBalance(schedule []PaymentRecord, target float64) (types.DateRec, bool) {
	for i := range schedule {
		if schedule[i].PayNum < 1 {
			continue // skip the settlement-stub row
		}
		if schedule[i].Principal <= target {
			return schedule[i].Date, true
		}
	}
	return types.DateRec{}, false
}

// ComputeDateFromBalanceDOS is the DOS-faithful inverse lookup (Amortize.pas:1153,
// ComputeDateFromBalance). DOS runs the walk in balance_calc mode and stops at the
// first payment whose PRE-payment-style balance drops below the target: for
// arrears that quantity is `principal + payamt`, for in-advance `principal +
// payamt - interest` (AMORTOP.pas:1113-1119 BalanceStop). It returns that
// payment's date and the corrected balance actually reached (which may differ
// slightly from the requested target). This differs from the naive row-snap
// DateForBalance by ~one payment — verified to the day against the oracle's
// `datefrombalance` query for arrears across the balance range.
//
// Caveat: the in-advance base-date shift is not fully reproduced from the display
// schedule; arrears (the common case) is exact, in-advance can be off by a period.
// A fully faithful in-advance result needs the balance_calc walk (see
// docs/discrepancies.md §13).
func ComputeDateFromBalanceDOS(schedule []PaymentRecord, target float64, inAdvance bool) (date types.DateRec, corrected float64, ok bool) {
	const minpmt = 1.0
	for i := range schedule {
		r := schedule[i]
		if r.PayNum < 1 {
			continue // skip the settlement-stub row
		}
		q := r.Principal + r.PayAmt
		if inAdvance {
			q -= r.Interest
		}
		if q < target+minpmt {
			return r.Date, q, true
		}
	}
	return types.DateRec{}, 0, false
}

// allMonthsSkipped reports whether every calendar month 1..12 is in the skip
// set — the degenerate input DOS's payment solve refuses (pass-2 finding 10).
func allMonthsSkipped(set [13]bool) bool {
	for m := 1; m <= 12; m++ {
		if !set[m] {
			return false
		}
	}
	return true
}

// wholeMonthGrid reports whether a payment frequency advances the schedule by a
// whole number of CALENDAR MONTHS per period — DOS's AddPeriod `else` branch
// (INTSUTIL.pas:1239-1250), which sets `d := orig_day` and steps the month by
// `12 div peryr`. Those are the grids on which RepayFancyLoan's date walk and
// RepayLoan's nominal recursion are algebraically the same computation:
// DaysCloseEnough always holds (the day-of-month is pinned, or both endpoints
// are last-of-month and LastDayFn fires), so ComputeNext's timedif is
// Δm/12 = 1/peryr = RepayLoan's f-1.
//
// peryr 24 walks d±15 (INTSUTIL.pas:1217-1238) and 26/52 walk a fixed Julian
// offset (:1212-1216), so neither is a whole-month grid.
//
// Used only by the §56 Exact×360 display gate, to decline a redirect DOS cannot
// observe. See the comment at that site for why that matters while §54 is open.
func wholeMonthGrid(perYr int) bool {
	return perYr > 0 && perYr <= 12 && 12%perYr == 0
}

// installmentsOnPaymentGrid counts how many payments of the schedule's own grid
// fall on or before lastDate, starting at firstDate (which is installment 1) and
// stepping with the SAME dateutil.AddPeriod the walk itself uses. It stops at
// cap, so a horizon far beyond the term costs nothing.
//
// This is the counter DOS's table loop implies — `until … DateComp(
// WhenToStop^.date, stopdate) >= 0` (AMORTOP.pas:1221) — expressed as a bound
// computed once rather than as a per-row test, so it can still be reasoned about
// alongside the period count. See the long note at its call site (§62) for why
// dateutil.NumberOfInstallments, which answers a date-DIFFERENCE question, is
// the wrong counter for this job even though it is a correct port of DOS's noi.
//
// A date the port cannot represent, or an AddPeriod that fails, returns 0 —
// "no opinion" — leaving the caller's period count in force rather than
// silently truncating the schedule. Round 19's §59 is the reason that is
// spelled out: an `if err == nil { advance }` with no else is how a date
// routine's failure turns into a wrong answer instead of an error.
func installmentsOnPaymentGrid(firstDate, lastDate types.DateRec, perYr, origDay, cap int) int {
	if cap <= 0 || !dateutil.DateOK(firstDate) || !dateutil.DateOK(lastDate) {
		return 0
	}
	if dateutil.DateComp(firstDate, lastDate) > 0 {
		return 0
	}
	ly, lm, ld := lastDate.Time.Year(), int(lastDate.Time.Month()), lastDate.Time.Day()

	// MONTHLY FAMILY — stepped on RAW FIELDS, in DOS's calendar.
	//
	// dateutil.AddPeriod is DOS-faithful but hands back a types.DateRec, and a
	// DateRec cannot hold 29 February 2100 (§54): it normalises to 1 March, and
	// the NEXT step then reads March as the month it is stepping from and skips
	// a month entirely. Counting on the raw (y, m) pair with DOS's own DaysInM
	// avoids materialising the impossible date at all — the day field is only
	// ever compared, never stored — so the count agrees with DOS on both sides
	// of the century boundary. No DateRec here is ever built with an invalid
	// day: DaysInM is asked about the 1st.
	if perYr >= 1 && perYr <= 12 && 12%perYr == 0 {
		step := 12 / perYr
		y, m := firstDate.Time.Year(), int(firstDate.Time.Month())
		n := 1
		for n < cap {
			m += step
			for m > 12 {
				m -= 12
				y++
				// DOS stores the year in a BYTE based at 1900 (§55); a horizon
				// past 2155 wraps rather than growing. Mirroring it here is what
				// keeps this counter doing the job it was originally added for.
				if y-1900 > 255 {
					y = 1900 + (y-1900)%256
				}
			}
			d := origDay
			if dim := int(dateutil.DaysInM(types.NewDateRec(y, time.Month(m), 1))); d > dim {
				d = dim
			}
			if y > ly || (y == ly && m > lm) || (y == ly && m == lm && d > ld) {
				break
			}
			n++
		}
		return n
	}

	// 24 / 26 / 52 step through Julian day numbers rather than month fields, so
	// they have no month to lose and go through the shared routine.
	n := 1
	d := firstDate
	for n < cap {
		nd, err := dateutil.AddPeriod(d, perYr, origDay, false)
		if err != nil {
			return 0
		}
		if dateutil.DateComp(nd, lastDate) > 0 {
			break
		}
		d = nd
		n++
	}
	return n
}
