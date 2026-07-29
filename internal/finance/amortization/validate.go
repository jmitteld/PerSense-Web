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
//	C-A-8  moratorium first-repay at/after lastDate (nrepay <= 0)
//	C-A-9  amount/nrepay < target (target unreachable)
//
// §45: C-A-8 and C-A-9 are BOTH moratorium-path-only in DOS — see the
// block comment at the bottom of ValidateInputs.
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

	// V6-9 + audit 20f: a prepayment series cannot start before OR ON the loan
	// date. DOS marks the dates out of order when `DateComp(loandate,
	// pre[i]^.startdate) >= 0` — equality INCLUDED (Amortize.pas:1231-1237).
	// Verified vs the real DOS engine: `amort_oracle 100000 0.08 120 12
	// pre=0:12:12:500` (start = loan date) → "ERR Your dates are out of order."
	for i, p := range input.Prepayments {
		if p.StartDateStatus >= types.InOutDefault &&
			dateutil.DateOK(p.StartDate) &&
			loan.LoanDateStatus >= types.InOutDefault &&
			dateutil.DateOK(loan.LoanDate) &&
			dateutil.DateComp(p.StartDate, loan.LoanDate) <= 0 {
			return fmt.Errorf("Prepayment row %d starts on or before the Loan Date. "+
				"Set the Prepayment start date after the Loan Date.", i+1)
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

	// C-A-8 / C-A-9: the moratorium `nrepay` guards.
	//
	// DOS FIDELITY (discrepancies.md §45). Amortize.pas:1299-1317 is the
	// authority, and BOTH guards live inside ONE conditional arm:
	//
	//	if (h^.lastok) and (DateComp(mor^.first_repay, h^.firstdate) <> 0) then
	//	  begin
	//	    save_last := h^.lastdate;
	//	    nrepay := NumberOfInstallments(mor^.first_repay, h^.lastdate,
	//	                                   h^.peryr, on_or_before);
	//	    h^.lastdate := save_last;
	//	    if (nrepay <= 0) then
	//	      MessageBox('Principal repayment must begin before the last payment date.');
	//	    if (h^.amount / nrepay < targ^.target) then
	//	      MessageBox('Your principal reduction target is too high.');
	//	  end
	//	else
	//	  nrepay := h^.nperiods;      { <-- NO validation on this path }
	//
	// and `mor^.first_repay` reaching that test has already been normalized
	// by Amortize.pas:1260-1288: when a moratorium IS set it is SNAPPED to
	// the payment grid ON_OR_AFTER by NumberOfInstallments' var-result date;
	// when none is set it is DEFAULTED to `balloon[1]^.date` if a balloon
	// precedes firstdate, else to `h^.firstdate` itself.
	//
	// The consequence — and the bug this replaces — is that with no
	// moratorium DOS lands on `DateComp(first_repay, firstdate) = 0`, skips
	// the arm entirely, takes `nrepay := h^.nperiods`, and NEVER VALIDATES
	// THE TARGET AT ALL. The port used to run this check unconditionally
	// against `amount / NPeriods`, so a plain 100k / 360-period loan with a
	// $300 principal minimum was rejected ("Target is too high", since
	// 100000/360 = 277.78 < 300) while the DOS engine computes it happily
	// (oracle: apr 0.076466 on that exact input). Client-reported 2026-07-24.
	//
	// Two further fidelity points folded in here:
	//   * `nrepay` is DOS's installment count from first_repay to LASTDATE
	//     (NumberOfInstallments, on_or_before) — not `NPeriods` minus a
	//     YearsDif-derived moratorium length. Those agree on clean monthly
	//     cases but are not structurally the same function.
	//   * DOS's sibling `nrepay <= 0` arm ("Principal repayment must begin
	//     before the last payment date") had no Go equivalent at all; it is
	//     independent of the Target and is restored below.
	firstRepay := loan.FirstDate
	morShift := false
	if input.Moratorium.FirstRepayStatus >= types.InOutDefault &&
		dateutil.DateOK(input.Moratorium.FirstRepay) &&
		dateutil.DateOK(loan.FirstDate) && loan.PerYr > 0 {
		// Amortize.pas:1262 — snap ON_OR_AFTER onto the payment grid.
		_, snapped := dateutil.NumberOfInstallments(loan.FirstDate,
			input.Moratorium.FirstRepay, int(loan.PerYr), types.OnOrAfter)
		if dateutil.DateOK(snapped) {
			firstRepay = snapped
		} else {
			firstRepay = input.Moratorium.FirstRepay
		}
		morShift = true
	} else if len(input.Balloons) > 0 &&
		input.Balloons[0].DateStatus >= types.InOutDefault &&
		dateutil.DateOK(input.Balloons[0].Date) &&
		dateutil.DateOK(loan.FirstDate) &&
		dateutil.DateComp(input.Balloons[0].Date, loan.FirstDate) < 0 {
		// Amortize.pas:1272-1273. (Unreachable while C-A-4 above rejects a
		// balloon dated before firstDate; mirrored for structural fidelity so
		// this block stays a faithful transcription of the DOS arm.)
		firstRepay = input.Balloons[0].Date
		morShift = true
	}

	// `!input.termHorizonWalk` restores DOS's entry-time h^.lastok. On a
	// term-solve screen DOS reaches this arm with lastok FALSE and skips it
	// outright; the port's synthetic 80-year clone has a LastOK that FirstPass
	// manufactured from a forced n, which made both arms below measure an
	// 80-year term. See types.go's termHorizonWalk note.
	if loan.LastOK && morShift && !input.termHorizonWalk &&
		dateutil.DateOK(loan.LastDate) && dateutil.DateOK(firstRepay) &&
		loan.PerYr > 0 &&
		dateutil.DateComp(firstRepay, loan.FirstDate) != 0 {
		nrepay, _ := dateutil.NumberOfInstallments(firstRepay, loan.LastDate,
			int(loan.PerYr), types.OnOrBefore)
		// C-A-8: Amortize.pas:1306-1310. Guards the divide below.
		if nrepay <= 0 {
			return fmt.Errorf("Principal repayment must begin before the Last Pmt " +
				"Date. Move the Moratorium first-repayment date earlier, or extend " +
				"Last Pmt Date.")
		}
		// C-A-9: Amortize.pas:1311-1316. Only reachable on the moratorium path.
		if input.Target.TargetStatus >= types.InOutDefault &&
			input.Target.TargetValue > 0 &&
			loan.AmountStatus >= types.InOutDefault &&
			loan.Amount/float64(nrepay) < input.Target.TargetValue {
			return fmt.Errorf("The principal-reduction Target is too high to be " +
				"reachable — it exceeds Amount Borrowed divided by the number of " +
				"principal-repaying periods. Lower the Target, or lengthen the term " +
				"by raising # Periods.")
		}
	}

	return nil
}
