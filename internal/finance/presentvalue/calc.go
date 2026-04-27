package presentvalue

import (
	"fmt"
	"math"

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
		return 0, fmt.Errorf("value of payments extending forever is infinite when interest rate <= COLA")
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

// Calculate performs a forward present value calculation: given rates and dates,
// compute the present value of all payments.
//
// Ported from legacy/source/PRESVALU.pas: procedure FrontwardCalc (sumvalue computation)
func Calculate(input PVInput) PVResult {
	var result PVResult
	pv := input.PresVal

	if pv.R.Status <= types.StatusEmpty || pv.AsOfStatus <= types.StatusEmpty {
		result.Err = fmt.Errorf("need rate and as-of date for present value calculation")
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

	for dateutil.DateComp(t, toDate) <= 0 {
		yrsFromStart := dateutil.YearsDif(t, fromDate, settings.Basis, settings.YrInv, false)
		yrsFromAsOf := dateutil.YearsDif(t, asOf, settings.Basis, settings.YrInv, false)
		part, err := interest.Exxp(yrsFromStart*cola - yrsFromAsOf*rate)
		if err != nil {
			break
		}
		prob := actu.LifeProb(t, contingency)
		part *= prob
		result += part
		probSum += prob
		count++
		if math.Abs(part) < teeny {
			break
		}
		t, err = dateutil.AddPeriod(t, peryr, origDay, false)
		if err != nil {
			break
		}
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
