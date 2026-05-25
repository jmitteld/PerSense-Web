// Input validation for the amortization screen, mirroring the
// per-field-combination error arms in DOS Amortize.pas: procedure Enter
// and its helpers SortAdj, SortBalloons, CheckPrepayments.
//
// These validations catch combinations of user inputs that are
// internally inconsistent (e.g. balloon scheduled before the loan even
// begins) so we don't silently produce a wrong schedule.
//
// Ported from legacy/src/dos_source/Amortize.pas and AMORTOP.pas.

package amortization

import (
	"fmt"
	"math"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// ValidateInputs runs the DOS-faithful pre-dispatch validation arms.
// Returns the first error encountered; callers should treat any error
// as a screen-level rejection.
//
// Validation arms (in order):
//
//	C-A-1  two adjustment rows on the same date
//	C-A-2  first adjustment date <= loanDate
//	C-A-3  last adjustment date >= lastDate (only when lastOK)
//	C-A-4  first balloon date < firstDate
//	C-A-5  firstDate >= lastDate (less than two regular payments)
//	C-A-6  moratorium first-repay > firstDate
//	C-A-7  balloon before moratorium first-repay
//	C-A-9  amount/nPeriods < target (target unreachable)
//
// The C-A-IDs refer to entries in docs/missing_flows_pass2.md.
//
// Adjustments and balloons must already be sorted (the engine calls
// SortAdjustments/SortBalloons before invoking the schedule; this
// function is safe to call before or after sorting since it sorts
// what it needs).
func ValidateInputs(input *LoanInput) error {
	loan := &input.Loan

	// Pre-sort so dup/order checks are reliable.
	SortBalloons(input.Balloons)
	SortAdjustments(input.Adjustments)

	// C-A-5: payment dates must be monotonic. DOS Amortize.pas Enter
	// arm "DateComp(firstDate, lastDate) >= 0" treats firstDate ==
	// lastDate as an error too, but the Go port supports a degenerate
	// one-payment loan (NPeriods=1 yields firstDate == lastDate), so
	// we only reject the strictly-out-of-order case.
	if loan.LastOK && dateutil.DateOK(loan.FirstDate) &&
		dateutil.DateOK(loan.LastDate) {
		if dateutil.DateComp(loan.FirstDate, loan.LastDate) > 0 {
			return fmt.Errorf("1st Pmt Date is after Last Pmt Date. Make sure 1st " +
				"Pmt Date comes first, or clear one of the two dates and let " +
				"Per%%Sense derive it.")
		}
	}

	// V6-9: the loan date must not fall after the first payment date
	// (the loan would begin after a payment is already due). DOS
	// Amortize.pas Enter rejects this dates-out-of-order case.
	if loan.LoanDateStatus >= types.InOutDefault &&
		loan.FirstStatus >= types.InOutDefault &&
		dateutil.DateOK(loan.LoanDate) && dateutil.DateOK(loan.FirstDate) &&
		dateutil.DateComp(loan.LoanDate, loan.FirstDate) > 0 {
		return fmt.Errorf("Loan Date is after 1st Pmt Date — a payment would be due " +
			"before the loan is made. Set Loan Date on or before 1st Pmt Date.")
	}

	// V6-9: a prepayment series cannot start before the loan exists.
	for i, p := range input.Prepayments {
		if p.StartDateStatus >= types.InOutDefault &&
			dateutil.DateOK(p.StartDate) &&
			loan.LoanDateStatus >= types.InOutDefault &&
			dateutil.DateOK(loan.LoanDate) &&
			dateutil.DateComp(p.StartDate, loan.LoanDate) < 0 {
			return fmt.Errorf("Prepayment row %d starts before the Loan Date. "+
				"Set the Prepayment start date on or after the Loan Date.", i+1)
		}
	}

	// V6-10: DOS rejects in-advance interest combined with rate
	// adjustments — the annuity-due accrual and the ARM re-amortize
	// are not defined together (AMORTOP.pas:1294-1298).
	if input.Settings.InAdvance && len(input.Adjustments) > 0 {
		return fmt.Errorf("Rate Adjustments cannot be used together with " +
			"in-advance interest. Remove the Adjustment rows, or turn off " +
			"in-advance interest in the Basis options.")
	}

	// C-A-1, C-A-2, C-A-3: adjustment validations.
	for i, a := range input.Adjustments {
		if a.DateStatus < types.InOutDefault {
			continue
		}
		if i > 0 {
			prev := input.Adjustments[i-1]
			if prev.DateStatus >= types.InOutDefault &&
				dateutil.DateComp(prev.Date, a.Date) == 0 {
				return fmt.Errorf("Two Rate Adjustments fall on the same date "+
					"(line %d). Give each Adjustment its own date, or combine them "+
					"into one row.", i+1)
			}
		}
		if loan.LoanDateStatus >= types.InOutDefault &&
			dateutil.DateOK(loan.LoanDate) &&
			dateutil.DateComp(a.Date, loan.LoanDate) <= 0 {
			return fmt.Errorf("Rate Adjustment on line %d is dated on or before "+
				"the Loan Date. Set the Adjustment date after the Loan Date.", i+1)
		}
		if loan.LastOK && dateutil.DateOK(loan.LastDate) &&
			dateutil.DateComp(a.Date, loan.LastDate) >= 0 {
			return fmt.Errorf("Rate Adjustment on line %d is dated on or after "+
				"the Last Pmt Date, so it would never take effect. Move the "+
				"Adjustment date earlier, or extend Last Pmt Date.", i+1)
		}
	}

	// C-A-4, C-A-7: balloon validations.
	for i, b := range input.Balloons {
		if b.DateStatus < types.InOutDefault {
			continue
		}
		if loan.FirstStatus >= types.InOutDefault &&
			dateutil.DateOK(loan.FirstDate) &&
			dateutil.DateComp(b.Date, loan.FirstDate) < 0 {
			return fmt.Errorf("Balloon on line %d is dated before the 1st Pmt "+
				"Date. Set the Balloon date on or after the 1st Pmt Date.", i+1)
		}
		// C-A-7: balloon before moratorium first-repay.
		if input.Moratorium.FirstRepayStatus >= types.InOutDefault &&
			dateutil.DateOK(input.Moratorium.FirstRepay) &&
			dateutil.DateComp(b.Date, input.Moratorium.FirstRepay) < 0 {
			return fmt.Errorf("Balloon on line %d is dated before the Moratorium "+
				"first-repayment date, when no principal is being repaid yet. Move "+
				"the Balloon date later, or shorten the Moratorium.", i+1)
		}
	}

	// C-A-6: moratorium first-repay must not precede firstDate.
	// During a moratorium, payments are interest-only from firstDate
	// until moratorium.FirstRepay, then switch to full amortization.
	// A first-repay date BEFORE firstDate would mean principal
	// repayment starts before any payment exists, which is nonsense.
	// DOS Amortize.pas Enter emits "principal repayment cannot
	// precede first pay" for this case.
	if input.Moratorium.FirstRepayStatus >= types.InOutDefault &&
		loan.FirstStatus >= types.InOutDefault &&
		dateutil.DateOK(input.Moratorium.FirstRepay) &&
		dateutil.DateOK(loan.FirstDate) &&
		dateutil.DateComp(input.Moratorium.FirstRepay, loan.FirstDate) < 0 {
		return fmt.Errorf("The Moratorium first-repayment date is before the 1st " +
			"Pmt Date, so principal repayment would start before any payment is " +
			"made. Set the Moratorium date on or after the 1st Pmt Date.")
	}

	// C-A-9: target must be reachable. If the user demands more
	// principal reduction per period than amount/n, the target is
	// impossible. Only fires when both amount and N are known.
	//
	// V6-8: with a moratorium, the interest-only periods reduce no
	// principal, so the target floor is checked against
	// amount / nrepay (post-moratorium period count), not
	// amount / NPeriods — DOS Amortize.pas.
	if input.Target.TargetStatus >= types.InOutDefault &&
		input.Target.TargetValue > 0 &&
		loan.AmountStatus >= types.InOutDefault &&
		loan.NStatus >= types.InOutDefault &&
		loan.NPeriods > 0 {
		nrepay := loan.NPeriods
		if input.Moratorium.FirstRepayStatus >= types.InOutDefault &&
			dateutil.DateOK(input.Moratorium.FirstRepay) &&
			dateutil.DateOK(loan.FirstDate) && loan.PerYr > 0 {
			morPeriods := int(math.Round(dateutil.YearsDif(
				input.Moratorium.FirstRepay, loan.FirstDate,
				types.Basis360, 1.0/360, true) * float64(loan.PerYr)))
			if morPeriods > 0 && morPeriods < loan.NPeriods {
				nrepay = loan.NPeriods - morPeriods
			}
		}
		perPmt := loan.Amount / float64(nrepay)
		if perPmt < input.Target.TargetValue {
			return fmt.Errorf("The principal-reduction Target is too high to be " +
				"reachable — it exceeds Amount Borrowed divided by the number of " +
				"repaying periods. Lower the Target, or lengthen the term by " +
				"raising # Periods.")
		}
	}

	return nil
}
