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

	realPerYr := interest.RealPerYr(byte(peryr), settings.YrDays)
	lnf := (cola - rate) / realPerYr

	// Check for infinite series
	latest := types.LatestDate()
	if lnf >= 0 && toDate.Time.Equal(latest.Time) {
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
			// AsOf > fromDate: accumulate from toDate
			since = dateutil.YearsDif(asOf, toDate, settings.Basis, settings.YrInv, false)
			if cola != 0 {
				yrsRange := dateutil.YearsDif(toDate, fromDate, settings.Basis, settings.YrInv, false)
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
// firstCOLAStepDate returns the date the first annual COLA increment
// is applied for a periodic series starting at fromDate.
//
//   - Anniversary mode (COLAMonth = ANN): fromDate + 1 year.
//   - Month-specific mode (COLAMonth = 1..12): the first 1st-of-that-
//     calendar-month strictly after fromDate (DOS SummationForSteppedCola).
func firstCOLAStepDate(fromDate types.DateRec, settings *PVSettings) (types.DateRec, error) {
	if settings.COLAMonth >= 1 && settings.COLAMonth <= 12 {
		cd := types.NewDateRec(fromDate.Time.Year(),
			time.Month(settings.COLAMonth), 1)
		for dateutil.DateComp(cd, fromDate) <= 0 {
			next, err := dateutil.AddYears(cd, 1, settings.Basis, settings.YrDays)
			if err != nil {
				return cd, err
			}
			cd = next
		}
		return cd, nil
	}
	return dateutil.AddYears(fromDate, 1, settings.Basis, settings.YrDays)
}

func periodicSumAnnualCOLA(rate, cola float64, asOf, fromDate, toDate types.DateRec,
	peryr, nInstallments int, settings *PVSettings) (float64, error) {

	// coladate is the first date the COLA step is applied; each
	// subsequent crossing multiplies the payment amount by (1+cola).
	// Interpretation: the user's COLA value is the *effective annual*
	// growth rate (entering 3.000 means payments grow 3%/year), so
	// the multiplier per year is (1+cola).
	coladate, err := firstCOLAStepDate(fromDate, settings)
	if err != nil {
		return 0, err
	}
	colaPerYear := 1.0 + cola

	result := 0.0
	multiplier := 1.0
	t := fromDate
	origDay := fromDate.Time.Day()

	for k := 0; k < nInstallments; k++ {
		// Advance multiplier past every anniversary t has crossed.
		for dateutil.DateComp(t, coladate) >= 0 {
			multiplier *= colaPerYear
			next, err := dateutil.AddYears(coladate, 1, settings.Basis, settings.YrDays)
			if err != nil {
				return 0, err
			}
			coladate = next
		}
		yrsFromAsOf := dateutil.YearsDif(t, asOf, settings.Basis, settings.YrInv, false)
		discount, err := interest.Exxp(-yrsFromAsOf * rate)
		if err != nil {
			return 0, err
		}
		result += multiplier * discount

		if dateutil.DateComp(t, toDate) > 0 {
			break
		}
		next, err := dateutil.AddPeriod(t, peryr, origDay, false)
		if err != nil {
			return 0, err
		}
		t = next
	}
	return result, nil
}

// Calculate is the public entry point for present value calculation.
// It runs FirstPass to classify the input, then dispatches to either
// the forward path (frontwardOnly) or BackwardCalc.
//
// Ported from legacy/src/dos_source/PRESVALU.pas: procedure Enter
// (the dispatcher that decides between FrontwardCalc and BackwardCalc).
func Calculate(input PVInput) PVResult {
	// Variable-rate mode (DOS PVL fancy): every row must be fully
	// specified, and we skip FirstPass entirely. Matches DOS:
	// "rates cannot be the target of a computation" on the VR screen.
	if len(input.RateSchedule) > 0 {
		// Variable-rate backward solve: when the screen Sum Value is
		// given and exactly one payment amount is blank, solve that
		// amount (DOS PVLX `amtn := valn/FancySummation`).
		if input.PresVal.SumValueStatus >= types.InOutDefault {
			if isLump, idx, ok := vrUnknownAmount(&input); ok {
				return solveVariableRateAmount(input, isLump, idx)
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
	return result
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
		if ls.DateStatus < types.InOutDefault || ls.AmtStatus < types.InOutDefault {
			continue
		}
		val, err := LumpSumValue(ls.Amt, ls.Date, asOf, rate, input.Settings.Basis, input.Settings.YrInv)
		if err != nil {
			result.Err = err
			return result
		}
		// Actuarial adjustment: multiply by life probability
		// Ported from PRESVALU.pas line 212: if (fold_in_life) then val0:=val0*LifeProb(date,a[i]^.act0)
		if input.Actuarial != nil && ls.Act != actuarial.NotContingent {
			prob := input.Actuarial.LifeProb(ls.Date, ls.Act)
			val *= prob
			ls.Prob = prob
		} else {
			ls.Prob = 1.0
		}
		ls.Val = val
		ls.ValStatus = types.InOutOutput
		sumValue += val
	}

	// Compute periodic payment values
	result.Periodics = make([]PeriodicPayment, len(input.Periodics))
	copy(result.Periodics, input.Periodics)

	for i := range result.Periodics {
		pp := &result.Periodics[i]
		if pp.FromDateStatus < types.InOutDefault || pp.ToDateStatus < types.InOutDefault ||
			pp.PerYrStatus < types.InOutDefault || pp.AmtStatus < types.InOutDefault {
			continue
		}
		if pp.PerYr <= 0 {
			continue
		}

		// Compute number of installments if not set
		if pp.NInstallments <= 0 {
			pp.NInstallments = estimateInstallments(pp.FromDate, pp.ToDate, pp.PerYr)
		}

		cola := pp.COLA
		if pp.COLAStatus < types.InOutDefault {
			cola = 0
		}

		// When actuarial is active, force exact (period-by-period) summation
		// and multiply each payment by LifeProb.
		// Ported from PRESVALU.pas line 290: if (fold_in_life) then [force exact]
		// and line 297: if (fold_in_life) then part:=part*LifeProb(t,b[j]^.actn)
		if input.Actuarial != nil && pp.Act != actuarial.NotContingent {
			val, prob := periodicWithActuarial(rate, cola, asOf, pp.FromDate, pp.ToDate,
				pp.PerYr, pp.NInstallments, &input.Settings, input.Actuarial, pp.Act)
			pp.Val = pp.Amt * val
			pp.Prob = prob
		} else {
			factor, err := PeriodicSummation(rate, cola, asOf, pp.FromDate, pp.ToDate,
				pp.PerYr, pp.NInstallments, &input.Settings)
			if err != nil {
				result.Err = err
				return result
			}
			pp.Val = pp.Amt * factor
			pp.Prob = 1.0
		}
		pp.ValStatus = types.InOutOutput
		sumValue += pp.Val
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
// Returns (factor, averageProbability).
//
// Ported from PRESVALU.pas lines 290-300: when fold_in_life is true, the exact
// method is forced and each payment is multiplied by LifeProb.
func periodicWithActuarial(rate, cola float64, asOf, fromDate, toDate types.DateRec,
	peryr, nInstallments int, settings *PVSettings,
	actu *actuarial.ActuarialConfig, contingency byte) (float64, float64) {

	result := 0.0
	probSum := 0.0
	count := 0
	t := fromDate
	origDay := fromDate.Time.Day()

	// Stepped COLA: when COLAMonth is not continuous the payment
	// grows by (1+cola) once a year at the step date, not smoothly.
	// (Continuous COLA keeps the exp(yrsFromStart*cola) form.)
	stepped := cola != 0 && settings.COLAMonth != types.COLAContinuous
	colaMult := 1.0
	colaPerYear := 1.0 + cola
	var coladate types.DateRec
	if stepped {
		cd, e := firstCOLAStepDate(fromDate, settings)
		if e != nil {
			stepped = false
		} else {
			coladate = cd
		}
	}

	for dateutil.DateComp(t, toDate) <= 0 {
		yrsFromAsOf := dateutil.YearsDif(t, asOf, settings.Basis, settings.YrInv, false)
		var part float64
		if stepped {
			for dateutil.DateComp(t, coladate) >= 0 {
				colaMult *= colaPerYear
				next, e := dateutil.AddYears(coladate, 1, settings.Basis, settings.YrDays)
				if e != nil {
					break
				}
				coladate = next
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
		part *= prob
		result += part
		probSum += prob
		count++
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
	return result, avgProb
}

// estimateInstallments computes an approximate number of installments.
func estimateInstallments(from, to types.DateRec, peryr int) int {
	if from.IsUnknown() || to.IsUnknown() {
		return 0
	}
	years := dateutil.YearsDif(to, from, types.Basis360, 1.0/360, false)
	n := int(years*float64(peryr)) + 1
	if n < 1 {
		n = 1
	}
	return n
}
