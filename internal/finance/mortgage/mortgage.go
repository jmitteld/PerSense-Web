// Package mortgage implements mortgage comparison calculations ported from
// the legacy Delphi/Pascal Mortgage.pas module.
//
// It provides the core Summation formula for present value of periodic payments,
// the Calc procedure for computing missing mortgage fields, APR iteration,
// and crossover APR comparison between two mortgages.
//
// All monetary values use float64 to match the original Pascal real type behavior.
// The caller is responsible for converting to/from decimal.Decimal at the
// boundary (API/display layer). Internal calculations must use float64 to
// preserve the exact numerical behavior of the original iterative algorithms.
//
// Ported from legacy/source/Mortgage.pas
package mortgage

import (
	"fmt"
	"math"

	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// --- Constants matching original Pascal values ---

const teeny = types.Teeny   // 1E-10
const tiny = types.Tiny     // 1E-5
const small = types.Small   // 1E-4
const twelfth = types.Twelfth // 1/12
const half = types.Half     // 0.5

// MtgLine represents one row of the mortgage comparison screen with
// float64 fields for internal calculation. This mirrors the Pascal
// mortgageline record but uses native float64 for computation.
//
// Ported from legacy/source/PETYPES.PAS: mortgageline record
type MtgLine struct {
	PriceStatus    int8
	Price          float64
	PointsStatus   int8
	Points         float64
	PctStatus      int8
	Pct            float64 // downpayment percentage (0-1)
	CashStatus     int8
	Cash           float64
	FinancedStatus int8
	Financed       float64
	YearsStatus    int8
	Years          int
	RateStatus     int8
	Rate           float64 // true (continuously compounded) rate
	TaxStatus      int8
	Tax            float64
	MonthlyStatus  int8
	Monthly        float64
	WhenStatus     int8
	When           int // years to balloon
	HowMuchStatus  int8
	HowMuch        float64 // balloon amount
	BalloonStat    types.BalloonStatus
}

// ZeroMortgage initializes all fields to empty/zero.
// Ported from legacy/source/Mortgage.pas: procedure ZeroMortgage
func ZeroMortgage(m *MtgLine) {
	*m = MtgLine{BalloonStat: types.BalloonBlank}
}

// IsEmpty returns true if all data fields are empty.
// Ported from legacy/source/Mortgage.pas: function MortgageIsEmpty
func IsEmpty(m *MtgLine) bool {
	return m.PriceStatus == types.StatusEmpty &&
		m.PointsStatus == types.StatusEmpty &&
		m.PctStatus == types.StatusEmpty &&
		m.CashStatus == types.StatusEmpty &&
		m.FinancedStatus == types.StatusEmpty &&
		m.YearsStatus == types.StatusEmpty &&
		m.RateStatus == types.StatusEmpty &&
		m.TaxStatus == types.StatusEmpty &&
		m.MonthlyStatus == types.StatusEmpty &&
		m.WhenStatus == types.StatusEmpty &&
		m.HowMuchStatus == types.StatusEmpty
}

// EnoughDataForAPR returns true if the mortgage has sufficient data
// for an APR calculation (financed, monthly, rate, and years all present).
// Ported from legacy/source/Mortgage.pas: function EnoughDataForAPR
func EnoughDataForAPR(m *MtgLine) bool {
	return m.FinancedStatus > 0 && interest.OK(m.Financed) &&
		m.MonthlyStatus > 0 && interest.OK(m.Monthly) &&
		m.RateStatus > 0 && interest.OK(m.Rate) &&
		m.YearsStatus > 0 && interest.OK(float64(m.Years))
}

// --- Core calculation functions ---

// Summation computes the present value of n monthly payments discounted at
// rate r over t years. This is the core formula used throughout the mortgage
// calculations.
//
// Formula: f * (1 - last) / (1 - f)
//
//	where f = e^(-r/12), last = e^(-r*t)
//
// For zero rate, returns 12*t (undiscounted payment count).
//
// Ported from legacy/source/Mortgage.pas: function Summation
func Summation(r, t float64) (float64, error) {
	if math.Abs(r) < teeny {
		return 12 * t, nil
	}
	last, err := interest.Exxp(-r * t)
	if err != nil {
		return 0, err
	}
	f, err := interest.Exxp(-r * twelfth)
	if err != nil {
		return 0, err
	}
	denom := 1 - f
	if math.Abs(denom) < teeny {
		return 12 * t, nil
	}
	return f * (1 - last) / denom, nil
}

// CalcResult holds the output of a Calc operation, including any
// fields that were computed and any error messages.
type CalcResult struct {
	Line MtgLine // updated mortgage line with computed fields
	Err  error   // nil on success
}

// Calc computes missing fields for a single mortgage row.
// Given price, % down (or cash or financed), years, and rate, it can compute:
//   - Cash and financed from pct (or pct from cash/financed)
//   - Monthly payment
//   - Price (given monthly and pct or cash)
//   - Balloon amount (if when is specified but howmuch is not)
//
// Ported from legacy/source/Mortgage.pas: procedure Calc
func Calc(m MtgLine) CalcResult {
	result := CalcResult{Line: m}
	ei := &result.Line

	// FirstPass validation
	if ei.YearsStatus == types.InOutInput && ei.Years <= 0 {
		result.Err = fmt.Errorf("mortgage term must be a positive number")
		return result
	}

	// Determine balloon status
	if ei.WhenStatus == types.InOutInput {
		if ei.HowMuchStatus == types.InOutInput {
			ei.BalloonStat = types.BalloonKnown
		} else {
			ei.BalloonStat = types.BalloonUnk
		}
	} else if ei.HowMuchStatus == types.InOutInput {
		result.Err = fmt.Errorf("must specify years to balloon payment")
		return result
	} else {
		ei.BalloonStat = types.BalloonBlank
	}

	if ei.PriceStatus == types.InOutInput && ei.FinancedStatus > types.StatusEmpty &&
		ei.Financed > ei.Price {
		result.Err = fmt.Errorf("amount borrowed cannot exceed price")
		return result
	}

	// Compute cash/pct/financed from price
	if ei.PriceStatus == types.InOutInput {
		if err := computeCashPctAndFinanced(ei); err != nil {
			result.Err = err
			return result
		}
	}

	// Main calculation: need pct/cash/financed + years + rate
	hasFunding := ei.PctStatus == types.InOutInput || ei.CashStatus == types.InOutInput || ei.FinancedStatus == types.InOutInput
	if hasFunding && ei.YearsStatus == types.InOutInput && ei.RateStatus == types.InOutInput {

		var balloonval float64
		if ei.BalloonStat == types.BalloonKnown {
			bv, err := interest.Exxp(-ei.Rate * float64(ei.When))
			if err != nil {
				result.Err = err
				return result
			}
			balloonval = ei.HowMuch * bv
		}

		if ei.PriceStatus == types.InOutInput {
			if ei.MonthlyStatus == types.InOutInput {
				// Both price and monthly specified
				if ei.BalloonStat == types.BalloonUnk {
					// Compute unknown balloon
					if err := balloonCalc(ei, balloonval); err != nil {
						result.Err = err
						return result
					}
				} else {
					result.Err = fmt.Errorf("leave price or monthly payment or balloon amount blank to be computed")
					return result
				}
			} else if ei.BalloonStat != types.BalloonUnk {
				// Compute monthly from price
				summ, err := Summation(ei.Rate, float64(ei.Years))
				if err != nil {
					result.Err = err
					return result
				}
				if math.Abs(summ) < teeny {
					result.Err = fmt.Errorf("summation too small")
					return result
				}
				ei.Monthly = (ei.Price*(1-ei.Pct)-balloonval)/summ + ei.Tax
				ei.MonthlyStatus = types.InOutOutput
			}
		} else if ei.MonthlyStatus == types.InOutInput && ei.BalloonStat != types.BalloonUnk {
			// Compute price from monthly
			summ, err := Summation(ei.Rate, float64(ei.Years))
			if err != nil {
				result.Err = err
				return result
			}
			paymentValue := (ei.Monthly - ei.Tax) * summ

			if ei.PctStatus == types.InOutInput {
				ei.Price = (paymentValue + balloonval) / (1 - ei.Pct)
			} else if ei.CashStatus == types.InOutInput {
				ei.Price = ei.Cash + (1-ei.Points)*(paymentValue+balloonval)
			} else {
				result.Err = fmt.Errorf("fill in percent down or cash required for price computation")
				return result
			}

			if err := computeCashPctAndFinanced(ei); err != nil {
				result.Err = err
				return result
			}
			ei.PriceStatus = types.InOutOutput
		}
	}

	return result
}

// computeCashPctAndFinanced computes the missing fields among pct, cash, financed
// given that price is known.
// Ported from legacy/source/Mortgage.pas: procedure ComputeCashPctAndFinanced
func computeCashPctAndFinanced(ei *MtgLine) error {
	if math.Abs(ei.Price) < teeny {
		return fmt.Errorf("price too small")
	}

	if ei.PctStatus == types.InOutInput {
		ei.Cash = ei.Price * (ei.Pct + (1-ei.Pct)*ei.Points)
		ei.CashStatus = types.InOutOutput
		ei.Financed = ei.Price * (1 - ei.Pct)
		ei.FinancedStatus = types.InOutOutput
	} else if ei.CashStatus == types.InOutInput {
		ei.Pct = (ei.Cash/ei.Price - ei.Points) / (1 - ei.Points)
		if ei.Pct >= 0.995 {
			return fmt.Errorf("cash too close to price")
		}
		ei.PctStatus = types.InOutOutput
		ei.Financed = ei.Price * (1 - ei.Pct)
		ei.FinancedStatus = types.InOutOutput
	} else if ei.FinancedStatus == types.InOutInput {
		ei.Pct = 1 - (ei.Financed / ei.Price)
		if ei.Pct >= 0.995 {
			return fmt.Errorf("financed amount too close to price")
		}
		ei.PctStatus = types.InOutOutput
		ei.Cash = ei.Price * (ei.Pct + (1-ei.Pct)*ei.Points)
		ei.CashStatus = types.InOutOutput
	}
	return nil
}

// balloonCalc computes the unknown balloon payment amount.
// Ported from legacy/source/Mortgage.pas: procedure BalloonCalc
func balloonCalc(ei *MtgLine, balloonval float64) error {
	summ, err := Summation(ei.Rate, float64(ei.Years))
	if err != nil {
		return err
	}
	bv := ei.Price*(1-ei.Pct) - (ei.Monthly-ei.Tax)*summ - balloonval
	expWhen, err := interest.Exxp(ei.Rate * float64(ei.When))
	if err != nil {
		return err
	}
	ei.HowMuch = bv * expWhen
	ei.HowMuchStatus = types.InOutOutput
	return nil
}

// --- APR calculation ---

// TerminalBalloon computes the remaining balance at time t years after loan date.
// Includes the regular payment that would be due on that date.
//
// Ported from legacy/source/Mortgage.pas: function TerminalBalloon
func TerminalBalloon(ei *MtgLine, t float64) (float64, error) {
	summ, err := Summation(ei.Rate, t-twelfth)
	if err != nil {
		return 0, err
	}
	result := ei.Financed - (ei.Monthly-ei.Tax)*summ

	if ei.BalloonStat != types.BalloonBlank && float64(ei.When) <= t {
		bv, err := interest.Exxp(-ei.Rate * float64(ei.When))
		if err != nil {
			return 0, err
		}
		result -= ei.HowMuch * bv
	}

	expRt, err := interest.Exxp(ei.Rate * t)
	if err != nil {
		return 0, err
	}
	return result * expRt, nil
}

// ValueOfPaymentsForTerminatedLoan computes the present value (as of loan date,
// using rate r) of all loan payments up to time t, including a terminal balloon at t.
//
// Ported from legacy/source/Mortgage.pas: function ValueOfPaymentsForTerminatedLoan
func ValueOfPaymentsForTerminatedLoan(ei *MtgLine, r, t float64) (float64, error) {
	summ, err := Summation(r, t-twelfth)
	if err != nil {
		return 0, err
	}
	result := (ei.Monthly - ei.Tax) * summ

	if ei.BalloonStat != types.BalloonBlank && float64(ei.When) < t {
		bv, err := interest.Exxp(-r * float64(ei.When))
		if err != nil {
			return 0, err
		}
		result += ei.HowMuch * bv
	}

	if t <= float64(ei.Years) {
		tb, err := TerminalBalloon(ei, t)
		if err != nil {
			return 0, err
		}
		discount, err := interest.Exxp(-r * t)
		if err != nil {
			return 0, err
		}
		result += tb * discount
	}

	return result, nil
}

// IterateToFindAPR uses Newton's method to find the APR of a mortgage,
// optionally terminated at time t. Returns the APR as a yield (effective rate)
// and whether the iteration converged.
//
// Ported from legacy/source/Mortgage.pas: function IterateToFindAPRofTerminatedLoan
func IterateToFindAPR(ei MtgLine, t float64, yrdays float64) (apr float64, converged bool, err error) {
	target := ei.Financed * (1 - ei.Points)
	apr = ei.Rate + ei.Points/float64(ei.Years) // first guess
	value, err := ValueOfPaymentsForTerminatedLoan(&ei, apr, t)
	if err != nil {
		return 0, false, err
	}
	oldvalue := value
	delta := small

	apr += delta
	for count := 0; count < 20; count++ {
		value, err = ValueOfPaymentsForTerminatedLoan(&ei, apr, t)
		if err != nil {
			return 0, false, err
		}
		denom := value - oldvalue
		var newdelta float64
		if math.Abs(denom) > teeny {
			newdelta = (target - value) * delta / denom
		} else {
			newdelta = small
		}
		delta = newdelta
		apr += delta
		oldvalue = value

		if math.Abs(delta) < teeny {
			break
		}
	}

	converged = math.Abs(delta) < tiny

	// Convert to yield at monthly compounding
	apr, err = interest.YieldFromRate(apr, 12, yrdays)
	if err != nil {
		return 0, false, err
	}
	return apr, converged, nil
}

// FullTermAPR computes the APR for a mortgage held to its full term.
// Ported from legacy/source/Mortgage.pas: function IterateToFindAPR
func FullTermAPR(ei MtgLine, yrdays float64) (apr float64, converged bool, err error) {
	return IterateToFindAPR(ei, float64(ei.Years)+twelfth, yrdays)
}

// OneMonthAPR computes the APR if the loan is paid off when the first
// payment is due (the worst-case APR with points).
//
// Ported from legacy/source/Mortgage.pas: function OneMonthAPR
func OneMonthAPR(ei *MtgLine, yrdays float64) (float64, error) {
	yld, err := interest.YieldFromRate(ei.Rate, 12, yrdays)
	if err != nil {
		return 0, err
	}
	aprRate := 12 * (1 + yld/12) / (1 - ei.Points)
	return interest.YieldFromRate(aprRate, 12, yrdays)
}

// APRComparisonResult holds the result of comparing two mortgages' APRs.
type APRComparisonResult struct {
	APR1          float64 // APR of mortgage 1
	APR1Converged bool
	APR2          float64 // APR of mortgage 2
	APR2Converged bool
	// CrossoverAPR and CrossoverTime are set if the APRs cross
	CrossoverAPR  float64
	CrossoverTime float64 // years
	// Summary describes which mortgage is better
	Summary string
}

// CompareAPRs compares the APRs of two mortgage lines.
//
// Ported from legacy/source/Mortgage.pas: procedure ReportComparisonOfAPRs
func CompareAPRs(e1, e2 MtgLine, yrdays float64) (APRComparisonResult, error) {
	var result APRComparisonResult

	result.APR1, result.APR1Converged, _ = FullTermAPR(e1, yrdays)
	result.APR2, result.APR2Converged, _ = FullTermAPR(e2, yrdays)

	apr1short, err := OneMonthAPR(&e1, yrdays)
	if err != nil {
		return result, err
	}
	apr2short, err := OneMonthAPR(&e2, yrdays)
	if err != nil {
		return result, err
	}

	// Check if one mortgage is always better
	if (apr1short < apr2short && result.APR1 <= result.APR2) ||
		(apr1short <= apr2short && result.APR1 < result.APR2) {
		result.Summary = "Mortgage 1 is always better."
		return result, nil
	}
	if (apr2short < apr1short && result.APR2 <= result.APR1) ||
		(apr2short <= apr1short && result.APR2 < result.APR1) {
		result.Summary = "Mortgage 2 is always better."
		return result, nil
	}

	// Try to find crossover point
	apr, t, found, err := iterateToFindCrossoverAPRandTime(e1, e2, yrdays)
	if err != nil {
		return result, err
	}
	if found {
		result.CrossoverAPR = apr
		result.CrossoverTime = t
		years := int(t)
		months := int(math.Round(12 * (t - float64(years))))
		var timestr string
		if years > 0 {
			timestr = fmt.Sprintf("%d years", years)
		}
		if months != 0 {
			if timestr != "" {
				timestr += ", "
			}
			timestr += fmt.Sprintf("%d months", months)
		}
		better := "1"
		if result.APR1 >= result.APR2 {
			better = "2"
		}
		result.Summary = fmt.Sprintf("APRs cross at %.4f%% for duration %s. "+
			"If held longer than %s, Mortgage %s is better.",
			100*apr, timestr, timestr, better)
	} else {
		result.Summary = "Crossover computation did not converge."
	}

	return result, nil
}

// iterateToFindCrossoverAPRandTime finds the time and APR where two mortgages'
// APRs are equal using 2D Newton iteration.
//
// Ported from legacy/source/Mortgage.pas: function IterateToFindCrossoverAPRandTime
func iterateToFindCrossoverAPRandTime(e1, e2 MtgLine, yrdays float64) (apr, t float64, found bool, err error) {
	const maxcount = 40

	targetFn := func(e *MtgLine, r, t float64) (float64, error) {
		vpmt, err := ValueOfPaymentsForTerminatedLoan(e, r, t)
		if err != nil {
			return 0, err
		}
		return 1 - vpmt/(e.Financed*(1-e.Points)), nil
	}

	// First guesses
	t = 0.25 * float64(e1.Years+e2.Years)
	apr1, conv1, _ := FullTermAPR(e1, yrdays)
	apr2, conv2, _ := FullTermAPR(e2, yrdays)
	var r float64
	if conv1 && conv2 {
		rfy, err := interest.RateFromYield(half*(apr1+apr2), 12, yrdays)
		if err != nil {
			return 0, 0, false, err
		}
		r = rfy
	} else {
		r = half * (e1.Rate + e1.Points/t + e2.Rate + e2.Points/t)
	}

	baser := r
	baset := 1.0
	// Reset
	baset += 2
	t = baset
	r = baser

	var target1, target2 float64

	for count := 0; count < maxcount; count++ {
		if t < 0 {
			baset += 2
			t = baset
			r = baser
		}

		// Compute partial derivatives via finite differences
		dr := tiny
		dt := small

		lasttarget1, err := targetFn(&e1, r, t)
		if err != nil {
			return 0, 0, false, err
		}
		lasttarget2, err := targetFn(&e2, r, t)
		if err != nil {
			return 0, 0, false, err
		}

		rPlus := r + dr
		t1r, err := targetFn(&e1, rPlus, t)
		if err != nil {
			return 0, 0, false, err
		}
		t2r, err := targetFn(&e2, rPlus, t)
		if err != nil {
			return 0, 0, false, err
		}
		dTarg1dr := (t1r - lasttarget1) / dr
		dTarg2dr := (t2r - lasttarget2) / dr

		lasttarget1 = t1r
		lasttarget2 = t2r
		lastt := t
		tPlus := t + dt
		target1, err = targetFn(&e1, rPlus, tPlus)
		if err != nil {
			return 0, 0, false, err
		}
		target2, err = targetFn(&e2, rPlus, tPlus)
		if err != nil {
			return 0, 0, false, err
		}
		dTarg1dt := (target1 - lasttarget1) / dt
		dTarg2dt := (target2 - lasttarget2) / dt

		det := dTarg1dt*dTarg2dr - dTarg1dr*dTarg2dt
		if math.Abs(det) < teeny {
			break
		}
		invdet := 1 / det

		dr = (dTarg2dt*target1 - dTarg1dt*target2) * invdet
		dt = (-dTarg2dr*target1 + dTarg1dr*target2) * invdet

		r = r + dr // note: using original lastr = r before dr increment
		t = lastt + dt

		if math.Abs(target1) < teeny && math.Abs(target2) < teeny {
			break
		}
	}

	if math.Abs(target1) > tiny || math.Abs(target2) > tiny {
		return 0, 0, false, nil
	}

	apr, err = interest.YieldFromRate(r, 12, yrdays)
	if err != nil {
		return 0, 0, false, err
	}
	t = twelfth * math.Trunc(12*t) // round down to prev full month

	found = t <= float64(e1.Years) && t <= float64(e2.Years) && t > 0 && r < 1 && r >= 0
	return apr, t, found, nil
}
