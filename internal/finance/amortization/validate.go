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
			return fmt.Errorf("first date must not be after last date")
		}
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
				return fmt.Errorf("two rate adjustments on the same day "+
					"(line %d)", i+1)
			}
		}
		if loan.LoanDateStatus >= types.InOutDefault &&
			dateutil.DateOK(loan.LoanDate) &&
			dateutil.DateComp(a.Date, loan.LoanDate) <= 0 {
			return fmt.Errorf("rate adjustment cannot precede the loan "+
				"(line %d)", i+1)
		}
		if loan.LastOK && dateutil.DateOK(loan.LastDate) &&
			dateutil.DateComp(a.Date, loan.LastDate) >= 0 {
			return fmt.Errorf("rate adjustment cannot fall on or after "+
				"the last payment (line %d)", i+1)
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
			return fmt.Errorf("balloon cannot precede the first regular "+
				"payment (line %d)", i+1)
		}
		// C-A-7: balloon before moratorium first-repay.
		if input.Moratorium.FirstRepayStatus >= types.InOutDefault &&
			dateutil.DateOK(input.Moratorium.FirstRepay) &&
			dateutil.DateComp(b.Date, input.Moratorium.FirstRepay) < 0 {
			return fmt.Errorf("balloon cannot precede the first principal "+
				"repayment (moratorium, line %d)", i+1)
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
		return fmt.Errorf("principal repayment cannot precede the " +
			"first regular payment (moratorium)")
	}

	// C-A-9: target must be reachable. If the user demands more
	// principal reduction per period than amount/n, the target is
	// impossible. Only fires when both amount and N are known.
	if input.Target.TargetStatus >= types.InOutDefault &&
		input.Target.TargetValue > 0 &&
		loan.AmountStatus >= types.InOutDefault &&
		loan.NStatus >= types.InOutDefault &&
		loan.NPeriods > 0 {
		perPmt := loan.Amount / float64(loan.NPeriods)
		if perPmt < input.Target.TargetValue {
			return fmt.Errorf("principal reduction target is too high " +
				"(target exceeds amount/nPeriods)")
		}
	}

	return nil
}
