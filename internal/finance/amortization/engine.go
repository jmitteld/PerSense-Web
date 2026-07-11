package amortization

import (
	"fmt"
	"math"
	"sort"

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
func Amortize(input LoanInput) AmortResult {
	var result AmortResult
	loan := input.Loan

	// Captured for the result-sanity advisory pass (A-W7): a solved
	// unknown-prepayment amount, recorded at its solve site below.
	prepaySolvedAmt, prepaySolved := 0.0, false
	// Captured for A-W11: whether the regular payment is a hard user input
	// (vs. computed). A balloon is dropped when the payment is computed.
	payWasInput := loan.PayAmtStatus == types.InOutInput

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

	// Cross-field validations (DOS Amortize.pas: procedure Enter
	// preflight + SortAdj/SortBalloons error arms).
	if err := ValidateInputs(&input); err != nil {
		result.Err = err
		return result
	}
	loan = input.Loan

	settings := input.Settings
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
		res := AmortizeDOS(pin)
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
		applyPointsSettlement(&res, &loan) // audit A12 — DOS settlement line precedes both engines
		return res
	}

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
	// RepayFancyLoan, which never compounds unpaid interest. Force the fancy engine so
	// the SCHEDULE matches the real DOS engine row-for-row (validated by the odd-first
	// and USA/R78 oracle cubes). NOTE: this also routes the payment solve through the
	// fancy terminal; the DOS Iterate solve terminal technically stays on RepayLoan for
	// USA (AMORTOP.pas:1438), so the internal forward↔backward round-trip is only
	// bounded for USA odd-first loans (see the USA frontier in TestExactBackwardRoundTripFuzz).
	// Matching the oracle-validated SCHEDULE takes priority over that Go-internal check.
	if settings.USARule && (settings.Exact || settings.Basis != types.Basis360) {
		input.Fancy = true
	}
	usaFancyDisplay := false

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
		if input.Fancy {
			// Fancy mode: balloons/prepayments/adjustments make the
			// closed form inapplicable — run the schedule unbounded
			// and observe when the loan retires.
			n, last, err := solveFancyTermFromPayment(input)
			if err != nil {
				result.Err = err
				return result
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
			if (len(input.Balloons) > 0 || len(input.Prepayments) > 0) && math.Abs(loan.Amount) > tiny {
				d *= dosSeedPVFactor(input, &loan, &settings)
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
		if loan.PayAmtStatus < types.InOutDefault &&
			loan.LoanRateStatus >= types.InOutDefault && loan.NPeriods > 0 &&
			needPaymentRefine(&loan, &settings) {
			refIn := input
			refIn.Loan = loan
			// DOS solves a plain loan's payment with Iterate over RepayLoan
			// (AMORTOP.pas:1437 else-branch), NOT RepayFancyLoan. dosIterateSimplePayment
			// drives that same RepayLoan terminal to zero. RepayLoan's in-advance branch
			// is the annuity-due recursion — basis-INDEPENDENT (no actual-day accrual,
			// no first-period proration), so it yields DOS's basis-independent in-advance
			// payment WITHOUT the earlier basis-360 substitution; the real-basis fancy
			// schedule is still rendered below with the solved payment. Odd-first arrears
			// loans use RepayLoan's prorated first period. The snap guard keeps an
			// already-exact estimate untouched (no sub-cent noise).
			if refined, ok := dosIterateSimplePayment(refIn, d); ok && refined > 0 &&
				math.Abs(refined-d) > 1e-3 {
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
		// On the 360 basis DOS uses the simple RepayLoan for display too, so it is
		// left untouched.
		if (settings.InAdvance && settings.Basis != types.Basis360 && !exactDaily(&settings)) ||
			usaFancyDisplay {
			dispInput := input
			dispInput.Loan = loan
			result = generateFancySchedule(dispInput, d, &settings, truerate, f)
		} else if settings.Exact && settings.InAdvance {
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
			ein := input
			ein.Loan = loan
			result = generateExactInAdvanceSchedule(ein, d, &settings)
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
				if refined, ok := dosIteratePayment(stripped, d); ok && refined > 0 {
					d = refined
				}
			} else if skipActive {
				// DOS's Iterate (dosIteratePayment) over the UNFORCED terminal
				// (generateFancyScheduleMode Output=nil) — the same single Newton DOS
				// uses. The terminal runs the full term with the full payment (no early
				// minpmt stop, no fold), so it is monotone in the payment and the Newton
				// converges; keeping the seed only if the solve returns a positive root.
				refined, ok := dosIteratePayment(input, d)
				if ok && refined > 0 {
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
				// refined > 0: a dominating balloon (PV ≥ the loan) makes the
				// terminal-zero payment negative; keep the plain-annuity seed so the
				// balloon-ignored path (advisory A-W11) still fires, rather than
				// surfacing a negative payment.
				if refined, ok := dosIteratePayment(input, d); ok && refined > 0 {
					d = refined
				}
			} else if exactDaily(&settings) && len(input.Adjustments) == 0 && !hasPrepay &&
				(!settings.InAdvance || !hasAnyAdvancedOption(input)) {
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
				if refined, ok := dosIteratePayment(input, seed); ok && refined > 0 {
					d = refined
				}
			} else if len(input.Adjustments) == 0 && !hasPrepay &&
				(settings.InAdvance ||
					oddFirstPeriod(loan.LoanDate, loan.FirstDate, loan.PerYr, &settings)) {
				// Universal non-shortcut refinement: any remaining fancy loan that
				// DOS would iterate rather than close-form — an odd first period OR
				// in-advance (annuity-due) — but with no balloon/target/adjustment/
				// prepayment of its own (e.g. a moratorium, or a plain odd-first
				// fancy loan). Snap-guarded so an already-exact estimate is kept.
				// The Newton runs over the unforced fancy terminal (fancyTerminal),
				// DOS's own Iterate terminal — see docs/dos_known_frontier.md #38.
				if refined, ok := dosIteratePayment(input, d); ok && refined > 0 &&
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
				if refined, ok := dosIteratePayment(input, d); ok && refined > 0 {
					d = refined
				}
			}
		}

		result = generateFancySchedule(input, d, &settings, truerate, f)
	}

	// A9: when the caller supplied discount points, compute the APR —
	// the rate that equates the present value of the scheduled
	// payments to the borrower's net proceeds (Amortize.pas: function
	// EstimateAndRefineAPRwithPoints).
	if result.Err == nil && loan.PointsStatus >= types.InOutDefault &&
		len(result.Schedule) > 0 {
		applyAPR(&result, input, loan, &settings, d, truerate, f)
	}

	// TackOnFinalBalloon (Amortize.pas:1040-1088): when the loan is
	// over-specified — the regular payment does not amortize it over
	// the stated term — the final payment absorbs the residual as an
	// implied terminating balloon. DOS appends it as a balloon row
	// and advises the user; here the residual is already folded into
	// the last scheduled payment, so flag it with an advisory.
	if result.Err == nil && len(result.Schedule) > 0 && d > 0 {
		last := result.Schedule[len(result.Schedule)-1]
		if last.PayAmt > d*1.5 && last.PayAmt-d > minPmt {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"The regular payment does not amortize the loan over the stated "+
					"term — the final payment of %.2f includes an implied "+
					"terminating balloon of about %.2f.", last.PayAmt, last.PayAmt-d))
		}
	}

	// Re-apply the snapshotted post-FirstPass term + dates. The
	// schedule generators return a fresh AmortResult that overwrites
	// the assignments we made earlier, so without this step a
	// successful run would echo NPeriods=0 / zero dates back to the API.
	result.NPeriods = derivedNPeriods
	result.FirstDate = derivedFirstDate
	result.LastDate = derivedLastDate

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
		if b.DateStatus < types.InOutDefault || !dateutil.DateOK(b.Date) {
			continue
		}
		result.Balloons = append(result.Balloons, ResolvedBalloon{
			Date:   b.Date,
			Amount: b.Amount,
			Solved: b.AmountStatus == types.InOutOutput,
		})
	}

	appendResultAdvisories(&result, &input, &loan, prepaySolvedAmt, prepaySolved, payWasInput)
	applyPointsSettlement(&result, &loan)
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
func applyPointsSettlement(result *AmortResult, loan *Loan) {
	if result.Err != nil || len(result.Schedule) == 0 {
		return
	}
	if loan.PointsStatus < types.InOutDefault || loan.Points == 0 {
		return
	}
	pts := loan.Points * loan.Amount
	hardPayment := loan.PayAmtStatus == types.InOutInput
	if hardPayment {
		pts = interest.Round2(pts)
	}
	first := &result.Schedule[0]
	if first.PayNum == 0 && dateutil.DateComp(first.Date, loan.LoanDate) == 0 {
		// Existing settlement stub (prepaid / in-advance): DOS combines the
		// points charge into the SAME line.
		first.PayAmt += pts
		first.Interest += pts
	} else {
		// No stub — emit a points-only settlement line at the loan date.
		row := PaymentRecord{
			PayNum:    0,
			Date:      loan.LoanDate,
			PayAmt:    pts,
			Interest:  pts,
			Principal: loan.Amount,
		}
		result.Schedule = append([]PaymentRecord{row}, result.Schedule...)
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
	if s.R78 {
		// DOS's payment solve is R78-agnostic (EstimateAndRefinePayment routes
		// purely on in_advance/dates; R78 only changes the interest SPLIT), so
		// an in-advance R78 loan still needs the annuity-due refine — audit
		// finding A4: `amort_oracle 100000 0.10 24 12 r78 inadv` → payment
		// 4579.8857, exactly the `inadv`-alone payment (the r78-alone payment
		// is the plain 4614.4926). The arrears R78 odd-first case keeps the
		// historical no-refine behavior (untested against the oracle; the R78
		// cube covers natural-first only).
		return s.InAdvance
	}
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
	if !exactDaily(s) && perYr > 0 && 12%perYr == 0 &&
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
		if timedif > 0 {
			return timedif
		}
	}
	return dateutil.YearsDif(cur, prev, s.Basis, s.YrInv, true)
}

func dosSeedPVFactor(input LoanInput, loan *Loan, settings *Settings) float64 {
	truerate, _ := ComputeTrueRate(loan, settings)
	adjp := loan.Amount
	for i := range input.Balloons {
		b := &input.Balloons[i]
		if b.AmountStatus >= types.InOutDefault && b.DateStatus >= types.InOutDefault {
			yd := dateutil.YearsDif(b.Date, loan.LoanDate, settings.Basis, settings.YrInv, false)
			if disc, e := interest.Exxp(-truerate * yd); e == nil {
				adjp -= b.Amount * disc
			}
		}
	}
	ffPre, _ := interest.Exxp(-truerate / float64(loan.PerYr))
	for i := range input.Prepayments {
		pp := &input.Prepayments[i]
		if pp.PaymentStatus < types.InOutDefault || pp.StartDateStatus < types.InOutDefault ||
			pp.PerYrStatus < types.InOutDefault {
			continue
		}
		stop := pp.StopDate
		if pp.StopDateStatus < types.InOutDefault && pp.NNStatus >= types.InOutDefault && pp.NN > 0 {
			stop = pp.StartDate
			day := pp.StartDate.Time.Day()
			for k := 1; k < pp.NN; k++ {
				if nd, e := dateutil.AddPeriod(stop, pp.PerYr, day, false); e == nil {
					stop = nd
				}
			}
		}
		first, _ := interest.Exxp(-truerate * dateutil.YearsDif(pp.StartDate, loan.LoanDate, settings.Basis, settings.YrInv, false))
		last, _ := interest.Exxp(-truerate * dateutil.YearsDif(stop, loan.LoanDate, settings.Basis, settings.YrInv, false))
		if math.Abs(1-ffPre) > teeny {
			adjp -= pp.Payment * (first - last*ffPre) / (1 - ffPre)
		} else if pp.NNStatus >= types.InOutDefault {
			adjp -= pp.Payment * float64(pp.NN)
		}
	}
	if math.Abs(loan.Amount) < tiny {
		return 1
	}
	// A dominating balloon/prepayment (present value ≥ the loan) drives adjp
	// non-positive — no regular payment can retire a "negative remainder". The port
	// ignores such a balloon when the payment is computed (advisory A-W11), so fall
	// back to the plain-annuity seed (factor 1) rather than a negative seed that
	// would negative-amortize.
	if adjp <= tiny {
		return 1
	}
	return adjp / loan.Amount
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
	if settings.Prepaid && !settings.InAdvance {
		naturalStart, err := dateutil.AddPeriod(loan.FirstDate, loan.PerYr, origDay, true)
		if err == nil {
			if dateutil.DateComp(loan.LoanDate, naturalStart) < 0 {
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
				// First regular period is now exactly one period long.
				prorate = 1.0
			} else {
				// Loan starts on or after the natural start of the first
				// regular period (no settlement stub). The first period's
				// length is a whole period on a clean month boundary
				// (months/perYr — basis-independent under the standard method,
				// matching DOS's `(basis=x360) or (not exact)` whole-month
				// rule) and the actual-day fraction only for an odd-day stub or
				// under the exact method. Using raw YearsDif here prorated a
				// clean 365-basis first period by ~1.0185 of a period (31/30.44)
				// — DOS treats it as exactly one period — which skewed every row
				// of a prepaid 365 schedule. firstPeriodProrate is the canonical
				// first-period length (same call the non-prepaid branch uses).
				prorate = firstPeriodProrate(loan.LoanDate, loan.FirstDate, loan.PerYr, settings)
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
	if settings.InAdvance {
		stubInt := p * (f - 1) * prorate
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

	retired := false
	for i := 0; i < loan.NPeriods; i++ {
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
			if i == loan.NPeriods-1 {
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
			if i == loan.NPeriods-1 {
				pmt = p + intThisPd
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
				var prevDate types.DateRec
				if i == 0 {
					prevDate = loan.LoanDate
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
			if i == loan.NPeriods-1 || p+intThisPd-pmt <= 0 {
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
		if retired && i < loan.NPeriods-1 {
			break
		}

		// Advance date
		if i < loan.NPeriods-1 {
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

	// Row 0: settlement interest at the loan date (the in-advance time-0 interest).
	// Simple actual-day interest over loanDate→firstDate; principal unchanged.
	stubYd := dateutil.YearsDif(loan.FirstDate, loan.LoanDate, settings.Basis, settings.YrInv, true)
	stubInt := p * loan.LoanRate * stubYd
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
		intThisPd := p * loan.LoanRate * yd
		if hardPayment {
			intThisPd = interest.Round2(intThisPd)
		}
		pmt := d
		// Final amortizing row retires the loan; an over-amortizing payment retires
		// early. DOS WhenToStop folds the residual into the payment. In unforced
		// (APR value) mode we never fold: the regular payment runs the full term.
		if !unforced && (k == loan.NPeriods-1 || p+intThisPd-pmt <= 0) {
			pmt = p + intThisPd
		}
		p = p + intThisPd - pmt
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
	// The walk is bounded by veryLast (= loan.LastDate). When a solver calls this
	// directly (not via Amortize), FirstPass has not derived LastDate from NPeriods,
	// so derive it here: LastDate = FirstDate + (NPeriods-1) periods.
	if !in.Loan.LastOK && in.Loan.NPeriods > 0 && dateutil.DateOK(in.Loan.FirstDate) {
		day := in.Loan.FirstDate.Time.Day()
		last := in.Loan.FirstDate
		for k := 1; k < in.Loan.NPeriods; k++ {
			if nd, e := dateutil.AddPeriod(last, in.Loan.PerYr, day, false); e == nil {
				last = nd
			}
		}
		in.Loan.LastDate = last
		in.Loan.LastOK = true
	}
	return generateFancyScheduleMode(in, x, settings, truerate, f, true).FinalPrinc
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
	var usap float64      // USA Rule exempt principal
	var negRateNoted bool // A-W12 emitted once if an AO6 adjustment implies a negative rate

	// hardPayment: a user-supplied regular payment triggers the DOS
	// "Dav Holle provision" — per-period interest is rounded to whole
	// cents (AMORTOP.pas:637 `if hard_payment then Round2(interest)`).
	hardPayment := loan.PayAmtStatus == types.InOutInput

	// DetermineVeryLast (AMORTOP.pas:1293-1304): the schedule must run
	// to the LATEST of {lastDate, last balloon date, every prepayment
	// stop date} — not just lastDate. Otherwise a balloon dated after
	// the last regular payment, or a prepayment series whose stop date
	// extends past lastDate, would be silently cut off.
	veryLast := loan.LastDate
	for _, b := range input.Balloons {
		if b.DateStatus >= types.InOutDefault &&
			dateutil.DateComp(b.Date, veryLast) > 0 {
			veryLast = b.Date
		}
	}
	for _, pp := range input.Prepayments {
		if pp.StopDateStatus >= types.InOutDefault &&
			dateutil.DateComp(pp.StopDate, veryLast) > 0 {
			veryLast = pp.StopDate
		}
		// NN-derived stop date (DOS DetermineVeryLast + CheckPrepayments,
		// AMORTOP.pas:400/1293-1304): a series specified by COUNT (NN extras) with
		// no explicit stop date still ends on a definite date — StartDate plus
		// (NN-1) prepayment periods. When that derived date runs PAST the last
		// regular payment date, DOS extends the schedule to it (the loan's residual
		// principal is retired by those trailing extras). Go previously extended
		// veryLast only for an EXPLICIT StopDate, so an NN-only series whose last
		// extra fell after the last payment date was cut one (or more) rows short,
		// leaving the balance unretired. Deriving the stop here mirrors DOS for both
		// arrears and in-advance loans.
		if pp.StopDateStatus < types.InOutDefault &&
			pp.NNStatus >= types.InOutDefault && pp.NN > 0 &&
			pp.PerYrStatus >= types.InOutDefault && pp.PerYr > 0 &&
			pp.StartDateStatus >= types.InOutDefault {
			derived := pp.StartDate
			startDay := pp.StartDate.Time.Day()
			ok := true
			for k := 1; k < pp.NN; k++ {
				nd, err := dateutil.AddPeriod(derived, pp.PerYr, startDay, false)
				if err != nil {
					ok = false
					break
				}
				derived = nd
			}
			if ok && dateutil.DateComp(derived, veryLast) > 0 {
				veryLast = derived
			}
		}
	}

	origDay := loan.FirstDate.Time.Day()
	currentDate := loan.FirstDate
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
		// Shift the base one period: the first amortizing period accrues from
		// FirstDate (the settlement row already covered LoanDate→FirstDate) and
		// the first payment date is FirstDate + 1 period.
		if shifted, err := dateutil.AddPeriod(loan.FirstDate, loan.PerYr, origDay, false); err == nil {
			currentDate = shifted
			prevDate = loan.FirstDate
		}
	}

	nextBalloon := 0 // index into sorted balloons

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

	// Moratorium tracking: moratoriumActive once we observe any
	// interest-only periods; moratoriumRecomputed once we've
	// re-solved d at the FirstRepay boundary so we only do it once.
	// Keyed on the FIRST amortizing payment date (currentDate), not FirstDate:
	// for an in-advance fancy loan the base date is shifted one period (above),
	// so the first amortizing row lands at FirstDate+1period. A moratorium whose
	// FirstRepay is on or before that shifted first date produces NO interest-only
	// rows and must not trigger the boundary recompute (DOS then just solves the
	// plain in-advance payment). For non-in-advance loans currentDate == FirstDate
	// here, so this is unchanged.
	moratoriumActive := input.Moratorium.FirstRepayStatus >= types.InOutDefault &&
		dateutil.DateComp(currentDate, input.Moratorium.FirstRepay) < 0
	moratoriumRecomputed := false

	// armDuringMoratorium: an ARM whose date falls inside the interest-only
	// window. DOS does NOT recompute the payment at the moratorium boundary in
	// this case — the ARM's Re_Amortize (AMORTOP.pas:1547) already set the payment
	// as the annuity over [ARM date → last date], IGNORING that amortization is
	// deferred until FirstRepay. That payment is therefore sized for MORE periods
	// than actually amortize, so the loan under-amortizes and DOS balloons the
	// final scheduled payment. If Go re-solved at the boundary it would instead
	// retire smoothly over the shorter window (reporting up to ~$129k less
	// interest). So when an ARM governs from inside the moratorium, suppress the
	// boundary recompute and let the AO5 payment + final-fold reproduce DOS.
	armDuringMoratorium := false
	if input.Moratorium.FirstRepayStatus >= types.InOutDefault {
		for ai := range input.Adjustments {
			if input.Adjustments[ai].DateStatus >= types.InOutDefault &&
				dateutil.DateComp(input.Adjustments[ai].Date, input.Moratorium.FirstRepay) < 0 {
				armDuringMoratorium = true
				break
			}
		}
	}

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
				if nextDates[i].IsUnknown() {
					nextDates[i] = pp.StartDate
				}
				if pp.StopDateStatus >= types.InOutDefault &&
					dateutil.DateComp(nextDates[i], pp.StopDate) > 0 {
					continue
				}
				if pp.NNStatus >= types.InOutDefault && pp.NN > 0 && prepayApplied[i] >= pp.NN {
					continue
				}
				if dateutil.DateComp(nextDates[i], currentDate) >= 0 {
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
			drainBalloon := false
			if nextBalloon < len(input.Balloons) {
				bd := input.Balloons[nextBalloon].Date
				if dateutil.DateComp(bd, currentDate) < 0 &&
					(drainIdx < 0 || dateutil.DateComp(bd, drainDate) < 0) {
					drainBalloon = true
					drainDate = bd
				}
			}
			if drainIdx < 0 && !drainBalloon {
				break
			}
			// Sum every event due exactly at drainDate, advancing each.
			var offPay float64
			if drainBalloon {
				// All balloons sharing this exact off-cycle date combine into one
				// dated row (their amount is the payment; principal reduction is
				// amount − accrued interest, computed below).
				for nextBalloon < len(input.Balloons) &&
					dateutil.DateComp(input.Balloons[nextBalloon].Date, drainDate) == 0 {
					offPay += input.Balloons[nextBalloon].Amount
					nextBalloon++
				}
			} else {
				for i := range input.Prepayments {
					pp := &input.Prepayments[i]
					if pp.PaymentStatus < types.InOutDefault || pp.PerYrStatus < types.InOutDefault ||
						pp.StartDateStatus < types.InOutDefault {
						continue
					}
					if nextDates[i].IsUnknown() {
						nextDates[i] = pp.StartDate
					}
					if pp.StopDateStatus >= types.InOutDefault &&
						dateutil.DateComp(nextDates[i], pp.StopDate) > 0 {
						continue
					}
					if pp.NNStatus >= types.InOutDefault && pp.NN > 0 && prepayApplied[i] >= pp.NN {
						continue
					}
					if dateutil.DateComp(nextDates[i], drainDate) == 0 {
						offPay += pp.Payment
						prepayApplied[i]++
						if next, err := dateutil.AddPeriod(nextDates[i], pp.PerYr,
							pp.StartDate.Time.Day(), false); err == nil {
							nextDates[i] = next
						}
					}
				}
			}
			// Partial-period interest from the previous row's date to drainDate.
			// DOS computes the off-cycle row's timedif through the same
			// DaysCloseEnough-gated path as a regular row (ComputeNext sets
			// `date := nextextra.date` BEFORE the shared timedif block,
			// AMORTOP.pas:608-632), so an extra that lands a clean month from
			// the previous row accrues the month fraction, and only a genuine
			// mid-month span uses actual days. periodYearFraction encodes
			// exactly that (audit finding A1).
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
			if !unforced && p+intOff-offPay <= 0 {
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
		if loan.LastOK && dateutil.DateComp(currentDate, loan.LastDate) > 0 {
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
				if nextDates[i].IsUnknown() {
					nextDates[i] = pp.StartDate
				}
				if pp.StopDateStatus >= types.InOutDefault &&
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
			if jump, err := dateutil.AddDays(lastPending, 1); err == nil &&
				dateutil.DateComp(jump, currentDate) > 0 {
				currentDate = jump
				continue // drain block emits the trailing extras at their dates
			}
			break // (coverage: excluded — defensive: jump not representable)
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
		// Before FirstRepay: pay interest only (no principal reduction). At
		// FirstRepay, if the regular payment was being SOLVED (left blank), we
		// re-solve it over the remaining periods on the unchanged principal so
		// the loan still amortizes — without this the post-moratorium payment
		// is too low (AM_EX13's help: the no-moratorium baseline of $2,024.02
		// is wrong; the right answer is $2,152.63). When the user GAVE the
		// payment, DOS uses it AS-IS through the moratorium (the interest-only
		// periods simply defer principal and any residual rolls to the end) —
		// it does not re-amortize. So the recompute is gated on a blank payment.
		if input.Moratorium.FirstRepayStatus >= types.InOutDefault {
			if dateutil.DateComp(currentDate, input.Moratorium.FirstRepay) < 0 {
				// Interest-only during the moratorium. Applied AFTER the extras
				// merge below (audit finding A3): DOS's ComputeNext adjusts the
				// MERGED payamt — `payamt := payamt − d + interest` when a
				// coincident extra is present, plain `payamt := interest`
				// otherwise (AMORTOP.pas:641-650) — so the extra survives the
				// interest-only floor. The flag defers the assignment.
			} else if !moratoriumRecomputed && moratoriumActive &&
				loan.PayAmtStatus < types.InOutDefault && !armDuringMoratorium {
				// First period at or after FirstRepay, payment solved — recompute d.
				// Suppressed when an ARM governs from inside the moratorium
				// (armDuringMoratorium): there DOS keeps the ARM's over-the-full-term
				// payment and balloons the final, rather than re-amortizing here.
				remaining := loan.NPeriods - payNum + 1
				if inAdvanceFancy {
					// In-advance fancy loans amortize over n-1 rows (the settlement
					// row consumed the first period via the base shift), so the count
					// of payments remaining from this boundary is one fewer than the
					// arrears schedule. Without this the post-moratorium payment is
					// solved for one period too many and the loan under-amortizes
					// (~2-6% low vs DOS on deep/biting moratoriums).
					remaining--
				}
				if remaining > 0 {
					tempLoan := loan
					tempLoan.Amount = p
					tempLoan.NPeriods = remaining
					d = estimatePayment(&tempLoan, f)
					// estimatePayment is the balloon-BLIND annuity seed. DOS
					// solves the loan as a SINGLE payment that retires the whole
					// schedule (moratorium interest-only periods included) to
					// zero, so its post-moratorium payment accounts for any later
					// balloon/prepayment. Refine the seed against the real
					// remaining schedule so a balloon AFTER the moratorium retires
					// the loan like DOS instead of over-paying and retiring early
					// (docs/amort_option_combo_divergences.md §3).
					if refined, ok := solveSegmentPayment(
						input, loan, *settings, p, prevDate, currentDate, remaining, d); ok {
						d = refined
					}
					pmt = d
				}
				moratoriumRecomputed = true
			}
		}

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
			if nextDates[i].IsUnknown() {
				nextDates[i] = pp.StartDate
			}
			if pp.StopDateStatus >= types.InOutDefault &&
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
					pp.StartDate.Time.Day(), false)
				if err == nil {
					nextDates[i] = next
				}
			}
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
		if unforced {
			// Unforced Newton-terminal mode (RepayFancyLoan Output=nil): apply the
			// regular payment/options as-is and never fold — the residual balance IS
			// the Newton signal. The one-sided minpmt stop is applied after the row.
		} else if p+intThisPd-pmt <= 0 {
			pmt = p + intThisPd
			payoffNow = true
		} else if plainFancy && payNum >= loan.NPeriods && p+intThisPd-pmt > 0 {
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
		} else if payNum >= loan.NPeriods && len(input.Adjustments) > 0 &&
			!settings.InAdvance && p+intThisPd-pmt > 0 {
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
		} else if payNum >= loan.NPeriods && len(input.Balloons) > 0 &&
			nextBalloon >= len(input.Balloons) && len(input.Prepayments) == 0 &&
			!settings.InAdvance && p+intThisPd-pmt > 0 {
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
			// off-cycle drain's job) and no prepayment series (their trailing
			// semantics are separately validated; DOS-consistent folding for
			// prepay+underfunded corners is untested and left as-is).
			pmt = p + intThisPd
			payoffNow = true
		} else if inAdvanceFancy && !hasAnyAdvancedOption(input) && loan.LastOK &&
			dateutil.DateComp(currentDate, veryLast) >= 0 && p+intThisPd-pmt > 0 {
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
		if !unforced && p < minPmt && p > -minPmt {
			// Display mode: balance essentially zero (within one minPmt either side)
			// ⇒ the loan retired, so stop even if scheduled periods remain.
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

		// Check for rate adjustments. The two DateComp clauses are a crossing test:
		// the adjustment takes effect on the FIRST period whose advance stepped over
		// its date, i.e. prevDate <= adj.Date < currentDate (adj.Date lies in the
		// half-open interval we just moved across). This fires the adjustment exactly
		// once, on the period boundary that passes it, regardless of whether the
		// adjustment date lands on a payment date.
		for i := range input.Adjustments {
			adj := &input.Adjustments[i]
			if adj.DateStatus >= types.InOutDefault &&
				dateutil.DateComp(currentDate, adj.Date) > 0 &&
				dateutil.DateComp(prevDate, adj.Date) <= 0 {
				hasRate := adj.LoanRateStatus >= types.InOutDefault
				hasAmt := adj.AmtOK
				remaining := loan.NPeriods - payNum
				if hasRate {
					loan.LoanRate = adj.LoanRate
					truerate, _ = ComputeTrueRate(&loan, settings)
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
					netBal := p
					remainingBalloon := false
					for bi := range input.Balloons {
						b := &input.Balloons[bi]
						if b.AmountStatus >= types.InOutDefault &&
							dateutil.DateComp(b.Date, currentDate) > 0 {
							remainingBalloon = true
							yd := dateutil.YearsDif(b.Date, currentDate,
								settings.Basis, settings.YrInv, false)
							if disc, e := interest.Exxp(-loan.LoanRate * yd); e == nil {
								netBal -= b.Amount * disc
							}
						}
					}
					if netBal < 0 {
						netBal = 0
					}
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
					d = annuityPayment(netBal, f, remaining)
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
					if remainingBalloon || len(input.Prepayments) > 0 {
						if refined, ok := solveSegmentPayment(
							input, loan, *settings, p, prevDate, currentDate, remaining, d); ok && refined > 0 {
							d = refined
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
					if r, ok := solveAdjRate(p, d, remaining, loan,
						settings.YrInv); ok {
						loan.LoanRate = r
						truerate, _ = ComputeTrueRate(&loan, settings)
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
	if bal < 0 {
		bal = 0
	}
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
	apr, conv := ComputeAPRWithPoints(aprSched, loan.LoanDate, netProceeds,
		loan.LoanRate, byte(loan.PerYr), settings)
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
