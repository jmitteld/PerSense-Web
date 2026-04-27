package amortization

import (
	"fmt"
	"math"
	"sort"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

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
func PrepaidInterest(loan *Loan, settings *Settings, truerate float64) (float64, error) {
	if !settings.Prepaid {
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
		// Compute prorate factor for first (possibly short) period
		ydif := dateutil.YearsDif(loan.FirstDate, loan.LoanDate, settings.Basis, settings.YrInv, true)
		prorate := ydif * float64(loan.PerYr)
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

	// Validate minimum required data
	if loan.AmountStatus < types.InOutDefault || loan.PerYrStatus < types.InOutDefault {
		result.Err = fmt.Errorf("insufficient loan data: need amount and payments per year")
		return result
	}

	if !dateutil.DateOK(loan.LoanDate) || !dateutil.DateOK(loan.FirstDate) {
		result.Err = fmt.Errorf("insufficient loan data: need loan date and first payment date")
		return result
	}

	settings := input.Settings
	truerate, _ := ComputeTrueRate(&loan, &settings)
	f := GrowthPerPeriod(&loan, settings.YrInv)

	// Default payment amount if not specified
	d := loan.PayAmt
	if loan.PayAmtStatus < types.InOutDefault {
		// Estimate payment
		if loan.LoanRateStatus >= types.InOutDefault && loan.NPeriods > 0 {
			d = estimatePayment(&loan, f)
		}
	}

	if !input.Fancy {
		// Simple amortization: generate schedule period by period
		result = generateSimpleSchedule(&loan, d, &settings, truerate, f)
	} else {
		// Fancy amortization with full feature set
		SortBalloons(input.Balloons)
		SortAdjustments(input.Adjustments)
		result = generateFancySchedule(input, d, &settings, truerate, f)
	}

	return result
}

// estimatePayment computes an initial payment estimate using the annuity formula.
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
func generateSimpleSchedule(loan *Loan, payment float64, settings *Settings, truerate, f float64) AmortResult {
	var result AmortResult
	p := loan.Amount
	var cumInt float64

	// Compute prorate for first period
	ydif := dateutil.YearsDif(loan.FirstDate, loan.LoanDate, settings.Basis, settings.YrInv, true)
	prorate := ydif * float64(loan.PerYr)

	currentDate := loan.FirstDate
	origDay := loan.FirstDate.Time.Day()

	for i := 0; i < loan.NPeriods; i++ {
		var intThisPd float64
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
		}

		interest.Round2(intThisPd)
		pmt := payment

		// Last payment: adjust to pay off remaining balance
		if i == loan.NPeriods-1 {
			pmt = p + intThisPd
		}

		p = p + intThisPd - pmt
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

// generateFancySchedule handles the full-featured amortization engine with
// balloons, adjustments, prepayments, moratoria, targets, and skip months.
//
// This is a simplified port of RepayFancyLoan that generates the schedule
// directly rather than printing to screen. The core payment-by-payment
// logic is preserved.
func generateFancySchedule(input LoanInput, payment float64, settings *Settings, truerate, f float64) AmortResult {
	var result AmortResult
	loan := input.Loan
	p := loan.Amount
	d := payment
	var cumInt float64
	var usap float64 // USA Rule exempt principal

	origDay := loan.FirstDate.Time.Day()
	currentDate := loan.FirstDate
	prevDate := loan.LoanDate

	// Handle prepaid interest
	if settings.Prepaid && !settings.InAdvance {
		t, _ := dateutil.AddPeriod(loan.FirstDate, loan.PerYr, origDay, true)
		prevDate = t
	}

	nextBalloon := 0 // index into sorted balloons

	for payNum := 1; payNum <= loan.NPeriods+len(input.Balloons)+100; payNum++ {
		// Safety limit to prevent infinite loops
		if payNum > 10000 {
			result.Err = fmt.Errorf("amortization exceeded 10000 periods")
			break
		}

		// Compute interest for this period
		var intThisPd float64
		yd := dateutil.YearsDif(currentDate, prevDate, settings.Basis, settings.YrInv, true)

		if settings.Daily {
			expVal, _ := interest.Exxp(truerate * yd)
			intThisPd = (p - usap) * (expVal - 1)
		} else {
			intThisPd = loan.LoanRate * yd * (p - usap)
		}

		// Check for balloon at this date
		pmt := d
		if currentDate.Time.Month() > 0 && int(currentDate.Time.Month()) <= 12 {
			if input.SkipMonths.MonthSet[currentDate.Time.Month()] {
				pmt = 0
			}
		}

		// Check moratorium
		if input.Moratorium.FirstRepayStatus >= types.InOutDefault {
			if dateutil.DateComp(currentDate, input.Moratorium.FirstRepay) < 0 {
				pmt = intThisPd // interest-only
			}
		}

		// Add balloon payments due at this date
		for nextBalloon < len(input.Balloons) {
			cmp := dateutil.DateComp(input.Balloons[nextBalloon].Date, currentDate)
			if cmp < 0 {
				// Balloon before this date — add as separate payment
				nextBalloon++
			} else if cmp == 0 {
				if settings.PlusRegular {
					pmt += input.Balloons[nextBalloon].Amount
				} else {
					pmt = input.Balloons[nextBalloon].Amount
				}
				nextBalloon++
				break
			} else {
				break
			}
		}

		// Target principal reduction
		if input.Target.TargetStatus >= types.InOutDefault {
			if pmt-intThisPd < input.Target.TargetValue {
				pmt = input.Target.TargetValue + intThisPd
			}
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
		if p < minPmt && p > -minPmt {
			break
		}
		if loan.LastOK && dateutil.DateComp(currentDate, loan.LastDate) >= 0 {
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

		// Check for rate adjustments
		for i := range input.Adjustments {
			adj := &input.Adjustments[i]
			if adj.DateStatus >= types.InOutDefault &&
				dateutil.DateComp(currentDate, adj.Date) > 0 &&
				dateutil.DateComp(prevDate, adj.Date) <= 0 {
				if adj.LoanRateStatus >= types.InOutDefault {
					loan.LoanRate = adj.LoanRate
					truerate, _ = ComputeTrueRate(&loan, settings)
					f = GrowthPerPeriod(&loan, settings.YrInv)
				}
				if adj.AmtOK {
					d = adj.Amount
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
			return monthSet, fmt.Errorf("month %d out of range [1,12]", n)
		}

		if thruflag {
			if lastN == 0 {
				return monthSet, fmt.Errorf("range without start month")
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
