// FirstPass for the amortization screen: classify the top-row inputs and
// derive any of {firstDate, lastDate, nPeriods} that the user left blank
// but can be computed from the others.
//
// Ported from legacy/src/dos_source/Amortize.pas: procedure FirstPass
// (lines 196-321), specifically the three derivation arms:
//
//   A-FP-defFirst (DefaultFirstPaymentDate): firstStatus < defp AND
//     loanDateStatus > defp AND peryrStatus >= defp ->
//     firstDate := loanDate + 1 period, firstStatus := defp.
//
//   A-FP-last: firstStatus >= defp AND nStatus >= defp ->
//     lastDate := firstDate + (n-1) periods, lastStatus := outp, lastOK.
//
//   A-FP-n: firstStatus >= defp AND lastStatus >= defp ->
//     nPeriods := NumberOfInstallments(firstDate, lastDate),
//     nStatus := outp, lastOK.
//
// These three arms run on field-presence and produce closed-form output;
// they don't iterate. The DOS code emits A-FP-defFirst before A-FP-last
// and A-FP-n so that supplying loanDate+peryr alone is enough to start
// the schedule with a sensible default first-payment date.

package amortization

import (
	"fmt"
	"math"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// FirstPass walks the top-row inputs and derives whichever of
// {firstDate, lastDate, nPeriods} the user left blank but can be
// computed from the others. It also sets loan.LastOK based on whether
// the last payment date is now known. Mutates the loan in place.
//
// Returns an error if the resulting input is internally inconsistent
// (e.g. lastDate <= firstDate, or n cannot be derived).
//
// Ported from legacy/src/dos_source/Amortize.pas: procedure FirstPass.
func FirstPass(loan *Loan) error {
	// A-FP-defFirst (DefaultFirstPaymentDate, Amortize.pas:184-194): when peryr
	// is known but firstDate is blank, default it the way DOS does — first of the
	// SECOND following month when the loan day > 1 (e.g. 6/13 -> 8/1), or the next
	// period when the loan is already on the 1st (6/1 -> 7/1). The earlier port
	// derived loanDate + one period keeping the day, which placed the first
	// payment a month early and (with prepaid interest on) produced no odd first
	// period for the settlement stub.
	if loan.FirstStatus < types.InOutDefault &&
		loan.LoanDateStatus >= types.InOutDefault &&
		loan.PerYrStatus >= types.InOutDefault &&
		loan.PerYr > 0 &&
		dateutil.DateOK(loan.LoanDate) {
		derr := func(e error) error {
			return fmt.Errorf("The 1st Pmt Date could not be derived from the "+
				"Loan Date and Pmts/Yr (%w). Enter the 1st Pmt Date directly.", e)
		}
		base := loan.LoanDate
		if base.Time.Day() > 1 {
			// DOS sets the day to the 1st, then advances one period.
			base = types.NewDateRec(base.Time.Year(), base.Time.Month(), 1)
			snapped, err := dateutil.AddPeriod(base, loan.PerYr, 1, false)
			if err != nil {
				return derr(err)
			}
			base = snapped
		}
		next, err := dateutil.AddPeriod(base, loan.PerYr, 1, false)
		if err != nil {
			return derr(err)
		}
		loan.FirstDate = next
		loan.FirstStatus = types.InOutDefault
	}

	// A-FP-last: derive lastDate from firstDate + (n-1) periods.
	if loan.LastStatus < types.InOutDefault &&
		loan.FirstStatus >= types.InOutDefault &&
		loan.NStatus >= types.InOutDefault &&
		loan.NPeriods > 0 &&
		loan.PerYr > 0 &&
		dateutil.DateOK(loan.FirstDate) {
		last, err := dateutil.AddNPeriods(loan.FirstDate, loan.PerYr,
			loan.NPeriods-1)
		if err != nil {
			return fmt.Errorf("The Last Pmt Date could not be derived from the "+
				"1st Pmt Date and # Periods (%w). Check # Periods for an unusually "+
				"large value, or enter the Last Pmt Date directly.", err)
		}
		loan.LastDate = last
		loan.LastStatus = types.InOutOutput
		loan.LastOK = true
	} else if loan.NStatus < types.InOutDefault &&
		loan.LastStatus >= types.InOutDefault &&
		loan.FirstStatus >= types.InOutDefault &&
		loan.PerYr > 0 &&
		dateutil.DateOK(loan.FirstDate) &&
		dateutil.DateOK(loan.LastDate) {
		// A-FP-n: derive nPeriods from firstDate + lastDate.
		// DOS NumberOfInstallments at INTSUTIL.pas rounds
		// peryr * yearsDif(first, last) to the nearest int.
		yrs := dateutil.YearsDif(loan.LastDate, loan.FirstDate,
			types.Basis360, 1.0/360, true)
		n := int(math.Round(yrs*float64(loan.PerYr))) + 1
		if n <= 0 {
			return fmt.Errorf("Last Pmt Date is on or before 1st Pmt Date, so the " +
				"number of periods cannot be derived. Make sure Last Pmt Date falls " +
				"after 1st Pmt Date.")
		}
		loan.NPeriods = n
		loan.NStatus = types.InOutOutput
		loan.LastOK = true
	} else if loan.LastStatus >= types.InOutDefault {
		// Last date was supplied directly.
		loan.LastOK = true
	}

	return nil
}
