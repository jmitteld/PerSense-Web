package presentvalue

import (
	"fmt"
	"math"
	"time"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// SumFormula computes the geometric series sum: (1 - e^(n*lnf)) / (1 - e^(lnf))
// With Taylor series approximations for small lnf to avoid precision loss.
//
// This is the core building block for present value of periodic payments.
//
// Ported from legacy/source/PRESVALU.pas: function SumFormula
func SumFormula(lnf, n float64) (float64, error) {
	if math.Abs(lnf) < teeny {
		// Zeroth order: sum = n
		return n, nil
	}

	secondOrder := math.Abs(lnf) < tiny

	var oneMinusExpNrt, oneMinusF float64
	if secondOrder {
		arg := n * lnf
		oneMinusExpNrt = -arg - half*arg*arg
		oneMinusF = -lnf - half*lnf*lnf
	} else {
		expNrt, err := interest.Exxp(n * lnf)
		if err != nil {
			return 0, err
		}
		oneMinusExpNrt = 1 - expNrt

		expF, err := interest.Exxp(lnf)
		if err != nil {
			return 0, err
		}
		oneMinusF = 1 - expF
	}

	// (coverage: excluded — defensive/unreachable: the secondOrder branch
	// already handles |lnf| < tiny (1e-5); on this path |lnf| >= 1e-5, so
	// |1 - exp(lnf)| >= ~1e-5 > teeny (1e-10) and this guard never fires.)
	if math.Abs(oneMinusF) < teeny {
		return n, nil
	}
	return oneMinusExpNrt / oneMinusF, nil
}

// LumpSumValue computes the present value of a single payment.
//
// value = amount * exp(rate * yearsDif(asof, paymentDate))
//
// If asof is after the payment date, the payment is discounted (value < amount).
// If asof is before the payment date, the payment is accumulated (value > amount).
//
// Ported from the lump sum computation in legacy/source/PRESVALU.pas:
// ComputeLumpsumLineValues
func LumpSumValue(amount float64, paymentDate, asOfDate types.DateRec,
	rate float64, basis types.BasisType, yrinv float64) (float64, error) {

	years := dateutil.YearsDif(asOfDate, paymentDate, basis, yrinv, false)
	expVal, err := interest.Exxp(rate * years)
	if err != nil {
		return 0, err
	}
	return amount * expVal, nil
}

// valueFullySpecifiedLump computes the present value at asOf of a
// fully_specified lump row and fills in whichever of {Amount, Value} the user
// left blank, for display. Ported from DOS ComputeLumpsumLineValues
// (PRESVALU.pas:204-224): a Date+Amount row computes Value forward
// (val0 := amt0·exxp(r·YearsDif(asof,date))); a Date+Value row derives the
// face Amount from the Value (amt0 := val0·exxp(-r·YearsDif), ÷LifeProb when
// life-contingent) — a PV row's Value IS its worth at asOf, so the row
// contributes exactly that Value to the screen total. This is the "known"
// side of the DOS field-presence model: only single-field rows are solved
// from the screen residual; a Value-bearing 2-field row is self-determined.
func valueFullySpecifiedLump(ls *LumpSumPayment, asOf types.DateRec, rate float64,
	settings *PVSettings, actu *actuarial.ActuarialConfig) (float64, error) {
	prob := 1.0
	if actu != nil && ls.Act != actuarial.NotContingent {
		prob = actu.LifeProb(ls.Date, ls.Act)
	}
	ls.Prob = prob

	if ls.AmtStatus >= types.InOutDefault {
		// Date + Amount → Value forward.
		val, err := LumpSumValue(ls.Amt, ls.Date, asOf, rate, settings.Basis, settings.YrInv)
		if err != nil {
			return 0, err
		}
		val *= prob
		ls.Val = val
		ls.ValStatus = types.InOutOutput
		return val, nil
	}

	// Date + Value → derive the face Amount (PRESVALU.pas:217-224).
	if prob <= teeny {
		return 0, fmt.Errorf("the date on single payment line is beyond the life span " +
			"for its contingency table, so no finite Amount matches the Value — move " +
			"the date earlier or clear the life contingency")
	}
	factor, err := LumpSumValue(1.0, ls.Date, asOf, rate, settings.Basis, settings.YrInv)
	if err != nil {
		return 0, err
	}
	if math.Abs(factor*prob) < teeny {
		return 0, fmt.Errorf("cannot derive the Amount on a single payment line: the " +
			"discount factor works out to essentially zero, so dividing the Value by " +
			"it gives no answer — check the Date and Rate on that row")
	}
	ls.Amt = ls.Val / (factor * prob)
	ls.AmtStatus = types.InOutOutput
	return ls.Val, nil
}

// valueFullySpecifiedPeriodic is the periodic analogue of
// valueFullySpecifiedLump: From+To+Amount computes Value forward, while
// From+To+Value derives the per-payment Amount from the Value
// (Amount := Value / summationFactor). Only the amount-derivable shape is
// treated as fully_specified here; From+Amount+Value (solve To) and
// To+Amount+Value (solve From) remain row-level backward solves.
func valueFullySpecifiedPeriodic(pp *PeriodicPayment, asOf types.DateRec, rate float64,
	settings *PVSettings, actu *actuarial.ActuarialConfig) (float64, error) {
	cola := pp.COLA
	if pp.COLAStatus < types.InOutDefault {
		cola = 0
	}
	if pp.NInstallments <= 0 {
		if !pp.FromDate.IsUnknown() && !pp.ToDate.IsUnknown() {
			n, ty, tm, td := dateutil.NumberOfInstallmentsRaw(pp.FromDate, pp.ToDate, pp.PerYr, types.OnOrBefore)
			if n < 1 {
				n = 1
			}
			pp.NInstallments = n
			pp.setRawTo(ty, tm, td)
		} else {
			pp.NInstallments = estimateInstallments(pp.FromDate, pp.ToDate, pp.PerYr)
		}
	}

	// Per-unit present-value factor (same factor the forward path multiplies
	// by Amount, and the actuarial path weights per payment).
	var factor float64
	if actu != nil && pp.Act != actuarial.NotContingent {
		f, prob, insts := periodicWithActuarial(pp.Amt, rate, cola, asOf, pp.FromDate, pp.ToDate,
			pp.PerYr, pp.NInstallments, settings, actu, pp.Act)
		factor = f
		pp.Prob = prob
		pp.Installments = insts
	} else {
		ty, tm, td := pp.rawTo()
		f, err := PeriodicSummationRawTo(rate, cola, asOf, pp.FromDate, pp.ToDate,
			ty, tm, td, pp.PerYr, pp.NInstallments, settings)
		if err != nil {
			return 0, err
		}
		factor = f
		pp.Prob = 1.0
	}

	if pp.AmtStatus >= types.InOutDefault {
		val := pp.Amt * factor
		pp.Val = val
		pp.ValStatus = types.InOutOutput
		return val, nil
	}
	// From + To + Value → derive per-payment Amount (DOS: amtn := valn/Summation).
	if math.Abs(factor) < teeny {
		return 0, fmt.Errorf("cannot derive the Amount on a periodic payment line: the " +
			"present value of the payment stream works out to essentially zero, so " +
			"dividing the Value by it gives no answer — check the From Date, To Date, " +
			"Pmts-Yr and Rate on that row")
	}
	pp.Amt = pp.Val / factor
	pp.AmtStatus = types.InOutOutput
	return pp.Val, nil
}

// PeriodicSummation computes the present value factor for a series of periodic
// payments with optional COLA, discounted at the given rate to the as-of date.
//
// This handles both the standard formula path and the exact (period-by-period)
// calculation path. The returned value is a factor: multiply by the payment
// amount to get the present value.
//
// Parameters:
//   - rate: continuously compounded discount rate
//   - cola: continuously compounded COLA rate (0 if none)
//   - asOf: date present value is computed as-of
//   - fromDate, toDate: payment date range
//   - peryr: payments per year
//   - nInstallments: number of installments
//   - settings: computational settings
//
// Ported from legacy/source/PRESVALU.pas: function Summation
func PeriodicSummation(rate, cola float64, asOf, fromDate, toDate types.DateRec,
	peryr, nInstallments int, settings *PVSettings) (float64, error) {
	return PeriodicSummationRawTo(rate, cola, asOf, fromDate, toDate,
		toDate.Time.Year(), int(toDate.Time.Month()), toDate.Time.Day(),
		peryr, nInstallments, settings)
}

// PeriodicSummationRawTo is PeriodicSummation with the To date supplied TWICE:
// once as a normalized types.DateRec (for DateComp / Equal / the exact-mode
// walk, all of which need a real date) and once as the three raw fields DOS
// actually holds in its `daterec`.
//
// The two differ when NumberOfInstallments snapped the terminal onto a
// short month while carrying the from-date's day of month across unclamped —
// DOS's `l.d := f.d` with no bound (INTSUTIL.pas:1013) — producing a daterec
// like 30/2/2037. See dateutil.NumberOfInstallmentsRaw.
//
// The distinction is confined to the `since_from = false` branch below, which is
// the ONLY place PRESVALU.pas's Summation feeds `todate` to YearsDif — and it
// does so twice, for the discount exponent and for the COLA range. Everywhere
// else `todate` is only compared, and the phantom compares identically to its
// normalization (no payment date can land strictly between them, since every
// payment is anchored to the from-date's day). PV fuzzer5 seed 20404: 285.26 on
// a 12.67M valuation.
func PeriodicSummationRawTo(rate, cola float64, asOf, fromDate, toDate types.DateRec,
	toY, toM, toD int, peryr, nInstallments int, settings *PVSettings) (float64, error) {

	realPerYr := interest.RealPerYr(byte(peryr), settings.YrDays)
	lnf := (cola - rate) / realPerYr

	// Check for infinite series. A perpetual stream (toDate == latest) has an
	// infinite PV only when the discount rate cannot outpace COLA growth -- in
	// continuous terms, rate <= ln(1+cola). DOS's stored COLA is already
	// continuous, so its guard compares the continuous COLA (PRESVALU.pas:379).
	// The raw yield `cola` here overstates growth, rejecting FINITE-PV perpetual
	// streams in the band rate in (ln(1+cola), cola] (audit A1 / discrepancies.md
	// Â§31); compare the continuous COLA instead.
	latest := types.LatestDate()
	colaCont := cola
	if cola != 0 {
		colaCont = math.Log1p(cola)
	}
	if colaCont >= rate && toDate.Time.Equal(latest.Time) {
		return 0, fmt.Errorf("a periodic payment that runs forever has an infinite present value when the Rate is less than or equal to the COLA. Either set a real To Date for the row, or raise the Rate above the COLA so the series converges")
	}

	// Annual-COLA mode: the COLA increment is applied once per year
	// at the anniversary of fromDate (or at COLAMonth if specified),
	// rather than smoothly each period. This is the DOS default
	// (PRESVALU.pas: colamonth=ANN, lines 281-305) and is what the
	// help docs assume — the closed-form continuous-COLA formula
	// over-counts the per-payment growth.
	//
	// Only the periodic case with peryr > 1 needs the annual path;
	// peryr=1 (annual) already coincides with the closed-form, and
	// a zero cola has no per-period growth to integrate.
	// Anniversary (ANN) and month-specific (1-12) COLA both step the
	// payment once per year; only continuous (CNT) COLA uses the
	// smooth closed-form fall-through below.
	if cola != 0 && peryr > 1 && settings.COLAMonth != types.COLAContinuous {
		return periodicSumAnnualCOLA(rate, cola, asOf, fromDate, toDate,
			peryr, nInstallments, settings)
	}

	// Continuous-COLA / closed-form path. The COLA is entered as a
	// *yield* (PV_COLA help: "interpreted as yields, not rates"), so
	// convert it to the continuous-rate equivalent ln(1+yield) before
	// it feeds the exp()-based formulas — this keeps continuous COLA
	// consistent with the stepped path's (1+yield) per-year multiplier
	// (DOS stores COLA in continuous form, so `exxp(cola)` = 1+yield).
	if cola != 0 {
		cola = math.Log1p(cola)
		lnf = (cola - rate) / realPerYr
	}

	// Exact mode: period-by-period summation
	if settings.Exact {
		result := 0.0
		t := fromDate
		origDay := fromDate.Time.Day()
		for dateutil.DateComp(t, toDate) <= 0 {
			yrsFromStart := dateutil.YearsDif(t, fromDate, settings.Basis, settings.YrInv, false)
			yrsFromAsOf := dateutil.YearsDif(t, asOf, settings.Basis, settings.YrInv, false)
			part, err := interest.Exxp(yrsFromStart*cola - yrsFromAsOf*rate)
			if err != nil {
				return 0, err
			}
			result += part
			if math.Abs(part) < teeny {
				break // convergence for infinite series
			}
			t, err = dateutil.AddPeriod(t, peryr, origDay, false)
			if err != nil {
				return 0, err
			}
		}
		return result, nil
	}

	// Standard formula path
	var sum float64
	var since float64

	if math.Abs(lnf) < teeny {
		// Zeroth order: sum = n
		sum = float64(nInstallments)
		since = dateutil.YearsDif(asOf, fromDate, settings.Basis, settings.YrInv, false)
	} else {
		// Determine whether asOf is before or after fromDate
		sinceFrom := dateutil.DateComp(asOf, fromDate) <= 0 || toDate.Time.Equal(latest.Time)

		sumF, err := SumFormula(lnf, float64(nInstallments))
		if err != nil {
			return 0, err
		}
		sum = sumF

		if sinceFrom {
			// AsOf <= fromDate: discount from one period before first payment
			stdLoanDate, err := dateutil.AddPeriod(fromDate, peryr, fromDate.Time.Day(), true)
			if err != nil {
				return 0, err
			}
			since = dateutil.YearsDif(asOf, stdLoanDate, settings.Basis, settings.YrInv, false)

			// Multiply by discount factor for one period
			ff, err := interest.Exxp(-rate / realPerYr)
			if err != nil {
				return 0, err
			}
			sum *= ff
		} else {
			// AsOf > fromDate: DOS anchors the geometric series at toDate and
			// sums it in reverse — oneminusexpnrt = 1-exp(-n·lnf), oneminusf =
			// 1-exp(-lnf) (PRESVALU.pas Summation, since_from:=false branch,
			// lines 438-447), which is exactly SumFormula(-lnf, n). The shared
			// SumFormula(lnf, n) above is the since_from (asof<=fromdate)
			// anchoring; reusing it here badly under-values a stream that starts
			// before the as-of date (e.g. a no-COLA periodic: 32k vs the correct
			// 87k). Recompute with -lnf to match DOS. Regression: the COLA=0
			// pre-as-of cases in dos_pv_oracle_test.go (periodic_off).
			sumRev, err := SumFormula(-lnf, float64(nInstallments))
			if err != nil {
				return 0, err
			}
			sum = sumRev
			// accumulate from toDate. Both reads use the RAW terminal: DOS's
			// `todate` here is whatever NumberOfInstallments wrote back through
			// its VAR parameter, which can be an un-representable daterec.
			since = dateutil.YearsDifRawA(asOf, toY, toM, toD, settings.Basis, settings.YrInv, false)
			if cola != 0 {
				yrsRange := dateutil.YearsDifRawZ(toY, toM, toD, fromDate, settings.Basis, settings.YrInv, false)
				colaAdj, err := interest.Exxp(yrsRange * cola)
				if err != nil {
					return 0, err
				}
				sum *= colaAdj
			}
		}
	}

	exprt, err := interest.Exxp(rate * since)
	if err != nil {
		return 0, err
	}
	return exprt * sum, nil
}

// periodicSumAnnualCOLA implements the DOS COLAmonth=ANN summation:
// the COLA multiplier (exp(cola) per year) is applied at the
// anniversary of fromDate, not smoothly each period. Payments
// within the same anniversary year share the same amount; the
// payment in anniversary-year y carries an exp(cola·y) multiplier.
//
// Strategy: iterate period by period, count the number of full
// anniversary years elapsed since fromDate at each payment date,
// and apply exp(cola·yearsElapsed) as the per-payment multiplier
// (the discount toward asOf is unchanged from the continuous case).
//
// Ported from legacy/src/dos_source/PRESVALU.pas function Summation,
// lines 281-305 (per-payment loop with coladate.y increment).
// nextColaAnniversary advances a COLA anniversary by one year with a plain
// year-field increment -- DOS `inc(coladate.y)` (PRESVALU.pas:289,302). This is
// deliberately NOT dateutil.AddYears: AddYears clamps month-ends (Feb-29 ->
// Feb-28), which lands the COLA step one payment early on a leap-day / month-end
// fromDate. The fixed-rate stepped-COLA path (periodicSumAnnualCOLA) already uses
// this plain increment and is oracle-validated; the life and variable-rate paths
// must match it (discrepancies.md Â§29 / audit D1).
func nextColaAnniversary(d types.DateRec) types.DateRec {
	return types.NewDateRec(d.Time.Year()+1, d.Time.Month(), d.Time.Day())
}

// colaAnniversary tracks a COLA anniversary the way DOS does: as raw
// (year, month, day) fields incremented by whole years and NEVER
// normalized into a calendar date. DOS stores the anniversary in a
// daterec whose dateok() checks only 1<=month<=13 (INTSUTIL.pas:584),
// so an invalid Feb-29 in a non-leap year stays a valid comparison
// anchor, and DateComp overlays (d,m,y) as a longint comparing y, then
// m, then d (INTSUTIL.pas:828,66). Go's DateRec is time.Time-backed and
// would normalize Feb-29 -> Mar-01, which lands the COLA step one
// payment late on a leap-day anchor in leap years -- the per-payment
// paths (variable-rate, life, exact) then under-COLA the Feb-29 payment
// every leap year (audit D1-followup / discrepancies.md sec 32). For
// every non-Feb-29 anchor the raw and normalized comparisons are
// identical (the anchor's own month/day is valid in every year), so this
// changes results only for the leap-day corner and never regresses rest.
type colaAnniversary struct{ year, month, day int }

// reached reports whether payment date t is on or after this
// anniversary under DOS's raw (y,m,d) longint DateComp.
func (a colaAnniversary) reached(t types.DateRec) bool {
	ty, tm, td := t.Time.Year(), int(t.Time.Month()), t.Time.Day()
	if ty != a.year {
		return ty > a.year
	}
	if tm != a.month {
		return tm > a.month
	}
	return td >= a.day
}

// next advances the anniversary by one whole year (DOS inc(coladate.y)),
// keeping the raw month/day.
func (a colaAnniversary) next() colaAnniversary { a.year++; return a }

// firstColaAnniversary returns the raw anniversary the first COLA step
// lands on, matching firstCOLAStepDate but without normalizing a Feb-29
// anchor. ANN mode: the anchor's month/day at year+1. Month-specific
// mode: day 1 of that calendar month, the first strictly after fromDate
// (day 1 is valid in every year, so month-specific never needs the raw
// handling; it routes here only so every caller uses one code path).
func firstColaAnniversary(fromDate types.DateRec, settings *PVSettings) colaAnniversary {
	if settings.COLAMonth >= 1 && settings.COLAMonth <= 12 {
		y := fromDate.Time.Year()
		if int(settings.COLAMonth) <= int(fromDate.Time.Month()) {
			y++
		}
		return colaAnniversary{year: y, month: int(settings.COLAMonth), day: 1}
	}
	return colaAnniversary{
		year:  fromDate.Time.Year() + 1,
		month: int(fromDate.Time.Month()),
		day:   fromDate.Time.Day(),
	}
}

// firstCOLAStepDate returns the date the first annual COLA increment
// is applied for a periodic series starting at fromDate.
//
//   - Anniversary mode (COLAMonth = ANN): fromDate + 1 year.
//   - Month-specific mode (COLAMonth = 1..12): the first 1st-of-that-
//     calendar-month strictly after fromDate (DOS SummationForSteppedCola).
//
// Both step by a plain year-field increment (nextColaAnniversary), matching
// DOS and the fixed-rate path -- never AddYears (which clamps month-ends).
func firstCOLAStepDate(fromDate types.DateRec, settings *PVSettings) (types.DateRec, error) {
	if settings.COLAMonth >= 1 && settings.COLAMonth <= 12 {
		cd := types.NewDateRec(fromDate.Time.Year(),
			time.Month(settings.COLAMonth), 1)
		for dateutil.DateComp(cd, fromDate) <= 0 {
			cd = nextColaAnniversary(cd)
		}
		return cd, nil
	}
	return nextColaAnniversary(fromDate), nil
}

// periodicSumAnnualCOLA is a faithful port of DOS PRESVALU.pas
// SummationForSteppedCola (the stepped-COLA summation used when COLA<>0,
// peryr>1 and COLAMonth<>CNT). DOS stores COLA in continuous form
// (ln(1+yield)); the caller passes the raw yield, so we convert once —
// the annual step multiplier is exp(colaCont) = 1+yield.
//
// The routine divides the stream into three parts (PRESVALU.pas:312-362):
// a partial first year (only when COLAMonth is month-specific, not ANN) summed
// per-payment, a middle block of whole years summed with the nominal-spacing
// closed form (SumFormula, twice), and a partial last year summed per-payment.
// Two details make non-360 bases match DOS where an exact per-payment day-count
// loop does NOT: (1) the middle block uses NOMINAL per-period spacing
// (SumFormula(-rate/RealPerYr, RealPerYr)), and (2) COLA anniversaries advance by
// a plain year-field increment (DOS inc(coladate.y)), not AddYears. See
// docs/pv_periodic_divergence_frontier.md.
func periodicSumAnnualCOLA(rate, cola float64, asOf, fromDate, toDate types.DateRec,
	peryr, nInstallments int, settings *PVSettings) (float64, error) {

	colaCont := math.Log1p(cola)
	expCola, err := interest.Exxp(colaCont)
	if err != nil {
		return 0, err
	}
	fromDay := fromDate.Time.Day()
	incYear := nextColaAnniversary
	discountTo := func(t types.DateRec) (float64, error) {
		return interest.Exxp(-dateutil.YearsDif(t, asOf, settings.Basis, settings.YrInv, false) * rate)
	}

	// coladate: first anniversary the COLA step lands (PRESVALU.pas:282-289).
	var coladate types.DateRec
	if settings.COLAMonth >= 1 && settings.COLAMonth <= 12 {
		y := fromDate.Time.Year()
		if int(settings.COLAMonth) <= int(fromDate.Time.Month()) {
			y++
		}
		coladate = types.NewDateRec(y, time.Month(settings.COLAMonth), 1)
	} else {
		coladate = incYear(fromDate) // ANN
	}

	result := 0.0
	t := fromDate

	// Exact per-payment method — required for weekly/biweekly or exact mode,
	// where years do not carry a fixed payment count (PRESVALU.pas:290-310).
	if settings.Exact || peryr == 26 || peryr == 52 {
		normalized := 1.0
		// Raw (y,m,d) anniversary -- DOS holds coladate as an unnormalized
		// daterec so a Feb-29 anchor steps on the Feb-29 payment in leap
		// years, not one payment later (discrepancies.md sec 32). Identical
		// to the DateRec form for every non-Feb-29 anchor.
		rawAnniv := firstColaAnniversary(fromDate, settings)
		for dateutil.DateComp(t, toDate) <= 0 {
			d, err := discountTo(t)
			if err != nil {
				return 0, err
			}
			result += normalized * d
			t, err = dateutil.AddPeriod(t, peryr, fromDay, false)
			if err != nil {
				return 0, err
			}
			if rawAnniv.reached(t) {
				normalized *= expCola
				rawAnniv = rawAnniv.next()
			}
		}
		return result, nil
	}

	// I. Partial first year (only for month-specific COLA; ANN starts at fromdate).
	if settings.COLAMonth != types.COLAAnnual {
		for dateutil.DateComp(t, coladate) < 0 {
			d, err := discountTo(t)
			if err != nil {
				return 0, err
			}
			result += d
			t, err = dateutil.AddPeriod(t, peryr, fromDay, false)
			if err != nil {
				return 0, err
			}
		}
	}

	// II. Middle block: whole years via the nominal-spacing closed form.
	currentPmt := 1.0
	if dateutil.DateComp(t, coladate) >= 0 {
		currentPmt = expCola
		coladate = incYear(coladate)
	}
	// DOS works the rest of this routine on RAW daterec FIELDS. Both
	// `lastof2d.y := todate.y` (PRESVALU.pas:338) and `t.y := t.y+nfullyears`
	// (:352) are bare year-field assignments — no normalization, no
	// CheckForDaysTooLarge. On a 29-February anchor they leave an INVALID
	// 29/2 in a non-leap year, and DOS keeps it: DateComp overlays (d,m,y) as
	// a longint (INTSUTIL.pas:828) so the record still reads month 2, and
	// AddPeriod (INTSUTIL.pas:1208) re-derives the day from orig_day and steps
	// the MONTH field, so the payment after 29/2/2065 is 29 June, not 29 July.
	// Julian() is linear in the day field (VIDEODAT.pas), so 29/2/2065
	// discounts exactly as 1/3/2065 — which is what normalizing gives us, so
	// only the MONTH needs protecting.
	//
	// Go's time.Time-backed DateRec normalizes 29/2/2065 -> 1/3/2065 and
	// carries that into the month, which shifts the whole tail of the stream a
	// month late and silently drops its last payment. On a 29-Feb-2028 through
	// 29-Jul-2065 stream at 3/yr with a 21.36% COLA that is a 3124.96
	// understatement on a 2.06M valuation. Track the raw fields here, exactly
	// as colaAnniversary does for the COLA step itself.
	lastValid, err := dateutil.AddPeriod(t, peryr, fromDay, true) // t minus one period
	if err != nil {
		return 0, err
	}
	lastof2d := rawDateOf(lastValid)
	lastof2d.y = toDate.Time.Year()
	if lastof2d.compTo(toDate) > 0 {
		lastof2d.y--
	}
	nfullyears := lastof2d.y - t.Time.Year()
	if lastof2d.m > int(t.Time.Month()) {
		nfullyears++
	}

	realPerYr := interest.RealPerYr(byte(peryr), settings.YrDays)
	first, err := discountTo(t) // exp(-rate * YearsDif(t, asof))
	if err != nil {
		return 0, err
	}
	innerSum, err := SumFormula(-rate/realPerYr, realPerYr)
	if err != nil {
		return 0, err
	}
	yearSum, err := SumFormula(colaCont-rate, float64(nfullyears))
	if err != nil {
		return 0, err
	}
	result += first * innerSum * currentPmt * yearSum

	// III. Partial last year, summed per payment (PRESVALU.pas:352-361).
	// `t.y := t.y + nfullyears` on the raw record — see the note above.
	tr := rawDateOf(t)
	tr.y += nfullyears
	growth, err := interest.Exxp(float64(nfullyears) * colaCont)
	if err != nil {
		return 0, err
	}
	currentPmt *= growth
	for tr.compTo(toDate) <= 0 {
		// Discount from the RAW record, not its normalization. On the 360 basis
		// YearsDif reads the month and day fields directly, so the 29/2/2029
		// this loop can open on is NOT interchangeable with 1/3/2029 — see
		// dateutil.YearsDifRawZ (PV fuzzer5 seed 8906).
		d, err := interest.Exxp(-rate * dateutil.YearsDifRawZ(tr.y, tr.m, tr.d,
			asOf, settings.Basis, settings.YrInv, false))
		if err != nil {
			return 0, err
		}
		result += d * currentPmt
		next, err := dateutil.AddPeriodFields(tr.y, tr.m, tr.d, peryr, fromDay, false)
		if err != nil {
			return 0, err
		}
		tr = rawDateOf(next)
	}
	return result, nil
}

// rawDate is a Pascal `daterec` held as three independent fields, so a year- or
// month-field assignment can leave a record that is not a real calendar date
// (29 February in a non-leap year, most commonly). DOS does exactly this and
// keeps computing with it; Go's DateRec cannot represent it because time.Time
// normalizes. See the note in periodicSumAnnualCOLA for why that difference is
// load-bearing.
type rawDate struct{ y, m, d int }

func rawDateOf(t types.DateRec) rawDate {
	return rawDate{y: t.Time.Year(), m: int(t.Time.Month()), d: t.Time.Day()}
}

// normalized resolves the raw record to the calendar date DOS's Julian() maps
// it to. Julian is linear in the day field (VIDEODAT.pas: daysBefore[m]+day),
// so an over-long day rolls forward into the next month — the same thing Go's
// time.Date does. Use this for discounting, never for month arithmetic.
func (r rawDate) normalized() types.DateRec {
	return types.NewDateRec(r.y, time.Month(r.m), r.d)
}

// compTo is DOS DateComp against a real date: the record is overlaid as a
// longint of (d, m, y) little-endian (INTSUTIL.pas:828), i.e. compare year,
// then month, then day, on the RAW fields.
func (r rawDate) compTo(o types.DateRec) int {
	oy, om, od := o.Time.Year(), int(o.Time.Month()), o.Time.Day()
	switch {
	case r.y != oy:
		return sign(r.y - oy)
	case r.m != om:
		return sign(r.m - om)
	default:
		return sign(r.d - od)
	}
}

func sign(x int) int {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	}
	return 0
}

// Calculate is the public entry point for present value calculation.
// It runs FirstPass to classify the input, then dispatches to either
// the forward path (frontwardOnly) or BackwardCalc.
//
// Ported from legacy/src/dos_source/PRESVALU.pas: procedure Enter
// (the dispatcher that decides between FrontwardCalc and BackwardCalc).
func Calculate(input PVInput) PVResult {
	// Guard: a two-life contingency (Only 1 / Only 2 / Either / Both
	// Living) needs a second life table and date of birth. Without one
	// the survival projection silently treats person 2 as immortal
	// (s2 = 1), collapsing the two-life cases to single-life equivalents
	// and producing wrong numbers with no signal. Reject it up front,
	// across every forward and backward path, naming the offending row.
	if err := checkSecondLifeProvided(&input); err != nil {
		return PVResult{Err: err}
	}

	// Variable-rate mode (DOS PVL fancy): every row must be fully
	// specified, and we skip FirstPass entirely. Matches DOS:
	// "rates cannot be the target of a computation" on the VR screen.
	if len(input.RateSchedule) > 0 {
		// Variable-rate backward solves: when the screen Sum Value is
		// given and exactly one field is blank, solve it by inverting the
		// true variable-rate forward valuation (which folds in the
		// schedule, COLA and life-contingency weighting). DOS PVLX
		// supports the same set: the amount solve (amtn :=
		// valn/FancySummation), the lump/periodic date solve, and the
		// unknown Payment-on-Death (XPODValue). An unknown rate is not
		// solvable on the VR screen (DOS: rates cannot be the target).
		if input.PresVal.SumValueStatus >= types.InOutDefault {
			if input.Actuarial != nil && input.Actuarial.PODUnknown {
				return solveVariableRatePOD(input)
			}
			if isLump, idx, ok := vrUnknownAmount(&input); ok {
				return solveVariableRateAmount(input, isLump, idx)
			}
			if kind, idx, ok := vrUnknownDate(&input); ok {
				return solveVariableRateDate(input, kind, idx)
			}
		}
		return forwardVariableRate(input)
	}

	// Unknown Payment-on-Death: solve POD from the target Sum Value
	// before the normal dispatch (DOS ComputeUnknownPOD).
	if input.Actuarial != nil && input.Actuarial.PODUnknown &&
		input.PresVal.SumValueStatus >= types.InOutDefault {
		return solveUnknownPOD(input)
	}

	fp := FirstPass(&input)
	if fp.Err != nil {
		return PVResult{Err: fp.Err}
	}

	var result PVResult
	switch {
	case fp.Frontward && fp.Backward:
		// (coverage: excluded — defensive/unreachable: Frontward requires
		// rate+asof present and zero contains_unknown rows, while Backward
		// requires either a contains_unknown row or a missing rate/asof;
		// the two conditions are mutually exclusive in FirstPass.)
		result = PVResult{Err: fmt.Errorf("there is more than one missing field on the screen, so Per%%Sense cannot tell which one to solve for. Leave exactly one cell blank — the field you want computed — and fill in all the others")}
	case fp.Backward:
		result = BackwardCalc(input, &fp)
	case !fp.Frontward:
		result = PVResult{Err: fmt.Errorf("there is not enough inputs to solve this present value. Fill in the Rate and the As-of Date, and complete at least one payment row (a single payment needs a Date and an Amount; a periodic payment needs From Date, To Date, Pmts-Yr and an Amount)")}
	default:
		result = forwardOnly(input)
	}
	// Carry FirstPass advisories (e.g. over-specified rows) through to
	// the caller — they are non-fatal and shouldn't suppress a result.
	result.Warnings = append(result.Warnings, fp.Warnings...)
	appendResultAdvisories(&result, &input)
	return result
}

// CheckSecondLifeProvided is the exported form of the two-life guard, so
// the API layer can reuse the exact same validation and message instead
// of duplicating it. Returns nil when the request is consistent.
func CheckSecondLifeProvided(input PVInput) error {
	return checkSecondLifeProvided(&input)
}

// checkSecondLifeProvided returns an error when any payment row uses a
// two-life contingency (Only 1 / Only 2 / Either / Both Living) but the
// actuarial config has no usable second life — no second life table, or
// no valid second date of birth. Without one, survivalProb2 silently
// defaults person 2 to certain survival (s2 = 1), so the two-life cases
// degenerate to single-life equivalents and produce wrong numbers with
// no signal. A two-life contingency with no actuarial config at all is
// not flagged here: with Actuarial == nil the contingency is inert (no
// weighting is applied on any path), so no degenerate weighting occurs.
func checkSecondLifeProvided(input *PVInput) error {
	if input.Actuarial == nil {
		return nil
	}
	if input.Actuarial.Table2 != nil && dateutil.DateOK(input.Actuarial.DOB2) {
		return nil
	}
	msg := func(rowKind string, n int, act byte) error {
		return fmt.Errorf("%s line %d uses the %q life-contingency, which depends on a second person, but no second life table and date of birth are set. Add the second person's life table and date of birth, or choose a single-life contingency (Living or Deceased)",
			rowKind, n, actuarial.ContingencyLabel(act))
	}
	for i := range input.LumpSums {
		if actuarial.RequiresSecondLife(input.LumpSums[i].Act) {
			return msg("single payment", i+1, input.LumpSums[i].Act)
		}
	}
	for j := range input.Periodics {
		if actuarial.RequiresSecondLife(input.Periodics[j].Act) {
			return msg("periodic payment", j+1, input.Periodics[j].Act)
		}
	}
	return nil
}

// solveUnknownPOD back-solves the Payment-on-Death amount from a
// target Sum Value. The POD's present value is linear in the POD
// amount, so the solve is closed-form: compute the present value of
// everything except the POD, then divide the residual by the
// present value of a unit (POD = 1) death benefit.
//
// Ported from legacy/src/dos_source/PRESVALU.pas: ComputeUnknownPOD
// (the podunk path).
func solveUnknownPOD(input PVInput) PVResult {
	target := input.PresVal.SumValue

	// Present value of every row with no death benefit.
	a0 := *input.Actuarial
	a0.POD = 0
	a0.PODUnknown = false
	in0 := input
	in0.Actuarial = &a0
	res0 := Calculate(in0)
	if res0.Err != nil {
		return res0
	}

	// Present value of a unit death benefit. POD's present value is
	// linear in POD, so probe with a large amount and divide — this
	// keeps the cent-rounding inside PODValue negligible (probing
	// with POD=1 would let the rounding swamp the sub-dollar result).
	const probe = 1e6
	a1 := *input.Actuarial
	a1.POD = probe
	a1.PODUnknown = false
	unit := a1.PODValue(input.PresVal.AsOf, input.PresVal.R.Rate) / probe
	if math.Abs(unit) < teeny {
		return PVResult{Err: fmt.Errorf(
			"cannot solve for the Payment on Death amount because the life-contingency settings give it no chance of being paid (zero death probability). Check the age and life-table settings, or enter the Payment on Death amount yourself instead of leaving it blank")}
	}

	solved := (target - res0.SumValue) / unit

	// Re-run with the solved POD so the result carries a consistent
	// PODValue and Sum Value.
	af := *input.Actuarial
	af.POD = solved
	af.PODUnknown = false
	inf := input
	inf.Actuarial = &af
	result := Calculate(inf)
	result.POD = solved
	return result
}

// forwardOnly performs the forward present value calculation: given
// rate and as-of date, sum the present value of all populated payment
// rows.
//
// Ported from legacy/source/PRESVALU.pas: procedure FrontwardCalc
// (sumvalue computation, lines 666-692).
func forwardOnly(input PVInput) PVResult {
	var result PVResult
	pv := input.PresVal

	if pv.R.Status <= types.StatusEmpty || pv.AsOfStatus <= types.StatusEmpty {
		result.Err = fmt.Errorf("the present value cannot be computed without both a Rate and an As-of Date. Fill in both fields, or if you want Per%%Sense to solve for one of them, leave just that one blank and supply the target Present Value")
		return result
	}

	rate := pv.R.Rate
	asOf := pv.AsOf

	// Compute lump sum values
	result.LumpSums = make([]LumpSumPayment, len(input.LumpSums))
	copy(result.LumpSums, input.LumpSums)

	var sumValue float64
	for i := range result.LumpSums {
		ls := &result.LumpSums[i]
		hasDate := ls.DateStatus >= types.InOutDefault
		hasAmt := ls.AmtStatus >= types.InOutDefault
		hasVal := ls.ValStatus >= types.InOutDefault
		// A fully_specified lump row is Date plus Amount OR Value; the missing
		// one is derived (valueFullySpecifiedLump). Amount+Value with no Date
		// is a row-level Date solve handled by BackwardCalc, not here.
		if !hasDate || (!hasAmt && !hasVal) {
			continue
		}
		val, err := valueFullySpecifiedLump(ls, asOf, rate, &input.Settings, input.Actuarial)
		if err != nil {
			result.Err = err
			return result
		}
		sumValue += val
	}

	// Compute periodic payment values
	result.Periodics = make([]PeriodicPayment, len(input.Periodics))
	copy(result.Periodics, input.Periodics)

	for i := range result.Periodics {
		pp := &result.Periodics[i]
		if pp.FromDateStatus < types.InOutDefault || pp.ToDateStatus < types.InOutDefault ||
			pp.PerYrStatus < types.InOutDefault {
			continue
		}
		hasAmt := pp.AmtStatus >= types.InOutDefault
		hasVal := pp.ValStatus >= types.InOutDefault
		// From+To+Amount computes Value forward; From+To+Value derives the
		// per-payment Amount from the Value. A row with neither is not
		// self-determined here (it is the screen-residual unknown or blank).
		if !hasAmt && !hasVal {
			continue
		}
		if pp.PerYr <= 0 {
			continue
		}
		val, err := valueFullySpecifiedPeriodic(pp, asOf, rate, &input.Settings, input.Actuarial)
		if err != nil {
			result.Err = err
			return result
		}
		sumValue += val
	}

	// Actuarial: add Payment on Death value
	// Ported from PRESVALU.pas line 689: if (fold_in_life) then sumvalue:=sumvalue+PodValue(asof,r.rate)
	if input.Actuarial != nil && input.Actuarial.POD != 0 {
		podVal := input.Actuarial.PODValue(asOf, rate)
		result.PODValue = podVal
		sumValue += podVal
	}

	result.SumValue = sumValue
	// Echo the rate and as-of date used, so a forward result carries
	// them the same way a backward solve does (dispatch_gaps §0.6.1).
	result.Rate = rate
	result.AsOf = asOf
	return result
}

// periodicWithActuarial computes the present value factor for periodic payments
// with life contingency, using exact (period-by-period) summation where each
// payment is weighted by the survival probability.
//
// Returns (factor, averageProbability, installments). `factor` is the
// unit-payment present value (the caller multiplies it by the payment Amount);
// `installments` carries the dollar-valued per-payment-date breakdown the DOS
// PVL table prints (date, if-paid value, probability, weighted value).
//
// `amount` is the per-period payment, used only to dollar-scale the per-payment
// installment detail; it does not affect `factor`.
//
// Ported from PRESVALU.pas lines 290-300: when fold_in_life is true, the exact
// method is forced and each payment is multiplied by LifeProb. The per-payment
// detail mirrors pvltable.pas PrintNextPayment (lines 514-533).
func periodicWithActuarial(amount, rate, cola float64, asOf, fromDate, toDate types.DateRec,
	peryr, nInstallments int, settings *PVSettings,
	actu *actuarial.ActuarialConfig, contingency byte) (float64, float64, []PeriodicInstallment) {

	result := 0.0
	probSum := 0.0
	count := 0
	var installments []PeriodicInstallment
	t := fromDate
	origDay := fromDate.Time.Day()

	// Stepped COLA: when COLAMonth is not continuous the payment
	// grows by (1+cola) once a year at the step date, not smoothly.
	// (Continuous COLA keeps the exp(yrsFromStart*cola) form.)
	stepped := cola != 0 && settings.COLAMonth != types.COLAContinuous
	colaMult := 1.0
	colaPerYear := 1.0 + cola
	var coladate colaAnniversary
	if stepped {
		coladate = firstColaAnniversary(fromDate, settings)
	}

	for dateutil.DateComp(t, toDate) <= 0 {
		yrsFromAsOf := dateutil.YearsDif(t, asOf, settings.Basis, settings.YrInv, false)
		var part float64
		if stepped {
			// Single `if`, not a catch-up loop: DOS's exact/life branch of
			// SummationForSteppedCola (PRESVALU.pas:303-306) steps the
			// multiplier at most once per payment, exactly like
			// UpdateAmountWithCola. On a Feb-29 anchor the raw anniversary is
			// unreachable in non-leap years, so coladate lags the payment
			// cursor by a year and a `for` would double-COLA the next
			// leap-day payment. See the note at variablerate.go for the
			// oracle evidence.
			if coladate.reached(t) {
				colaMult *= colaPerYear
				// Raw (y,m,d) anniversary (DOS inc(coladate.y) on an
				// unnormalized daterec) -- keeps the life path's COLA steps
				// aligned with DOS on a leap-day fromDate, where a normalized
				// Feb-29 -> Mar-01 would step one payment late every leap year
				// (audit D1-followup, sec 32).
				coladate = coladate.next()
			}
			disc, err := interest.Exxp(-yrsFromAsOf * rate)
			if err != nil {
				break
			}
			part = colaMult * disc
		} else {
			// Continuous COLA: the entered yield is converted to its
			// continuous-rate equivalent ln(1+yield) (see PeriodicSummation).
			colaCont := math.Log1p(cola)
			yrsFromStart := dateutil.YearsDif(t, fromDate, settings.Basis, settings.YrInv, false)
			p, err := interest.Exxp(yrsFromStart*colaCont - yrsFromAsOf*rate)
			if err != nil {
				break
			}
			part = p
		}
		prob := actu.LifeProb(t, contingency)
		ifpdUnit := part // discounted unit value, NOT survival-weighted (DOS ifpd per $1)
		result += ifpdUnit * prob
		probSum += prob
		count++
		// Per-payment-date breakdown, dollar-scaled, mirroring the DOS PVL
		// table's per-row probability column (pvltable.pas:514-533).
		installments = append(installments, PeriodicInstallment{
			Date:   t,
			IfPaid: amount * ifpdUnit,
			Prob:   prob,
			Value:  amount * ifpdUnit * prob,
		})
		// Note: the toDate-bounded loop is enough to terminate; we
		// intentionally do NOT early-break on `part < teeny` here.
		// For Living and non-contingent paths the probabilities decay
		// monotonically and early-break is harmless, but Dead-
		// contingent (and Only1/Only2/Either) probabilities are
		// non-monotone — they start near zero for a young insured
		// and grow over time. An early break on the first iteration
		// would silently zero-out the Dead-contingent value and
		// violate the Living+Dead = non-contingent complementarity
		// property the help advertises.
		nt, e := dateutil.AddPeriod(t, peryr, origDay, false)
		if e != nil {
			break
		}
		t = nt
	}

	avgProb := 0.0
	if count > 0 {
		avgProb = probSum / float64(count)
	}
	return result, avgProb, installments
}

// estimateInstallments returns the number of periodic installments between
// from and to. It ports DOS PRESVALU.pas ComputePeriodicLineValues, which counts
// with NumberOfInstallments(fromdate, todate, peryr, ON_OR_BEFORE) — a
// calendar-walk that respects the day-of-month and is basis-independent
// (PRESVALU.pas:607). The prior implementation approximated the count with a
// Basis360 year-fraction truncation (int(YearsDif*peryr)+1); that agreed with
// DOS only at the day-of-month=1 / 360-basis corner the old sweep pinned and
// diverged (wrong installment count → wrong geometric-sum length) elsewhere.
func estimateInstallments(from, to types.DateRec, peryr int) int {
	if from.IsUnknown() || to.IsUnknown() {
		return 0
	}
	n, _ := dateutil.NumberOfInstallments(from, to, peryr, types.OnOrBefore)
	if n < 1 {
		n = 1
	}
	return n
}
