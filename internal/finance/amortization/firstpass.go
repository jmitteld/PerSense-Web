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
	//
	// DOS runs this arm whenever first+n are present, WITHOUT checking
	// laststatus — a user-typed Last Pmt Date is unconditionally OVERWRITTEN
	// (Amortize.pas:220-226: `AddNPeriods(...); laststatus := outp`), so on an
	// over-determined {first, n, last} row N WINS. 2026-07-11 audit finding A7:
	// the port previously gated this arm on `LastStatus < InOutDefault`,
	// preserving the stale user date, and the fancy walk then truncated at it
	// (12 rows instead of 24 on the audit case). Verified vs the real DOS
	// engine: `amort_oracle 10000 0.12 24 12 pay=488 b6=3000 lastdmy=1.1.2025`
	// → identical to the run without lastdmy (payment 488, interest 854.89,
	// 24 rows) — the supplied last date is ignored.
	if loan.FirstStatus >= types.InOutDefault &&
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
		// A-FP-n: derive nPeriods from firstDate + lastDate via DOS
		// NumberOfInstallments(first, last, peryr, ON_OR_BEFORE), which both
		// counts and SNAPS the last date onto the on-or-before payment day
		// (VAR parameter — DOS keeps the snapped date, Amortize.pas:229).
		//
		// 2026-07-11 audit finding A9: the port previously rounded
		// `YearsDif(first,last)×perYr`, which rounds UP across a payment
		// boundary the snap rounds DOWN. Verified vs the real DOS engine:
		// first 1/15/2024, last 3/12/2025 → `amort_oracle intutil noi 2024 1
		// 15 2025 3 12 12 on_or_before` → n 14, last snapped to 2/15/2025
		// (the port derived 15).
		n, snapped := dateutil.NumberOfInstallments(loan.FirstDate, loan.LastDate,
			loan.PerYr, types.OnOrBefore)
		if n <= 0 {
			return fmt.Errorf("Last Pmt Date is on or before 1st Pmt Date, so the " +
				"number of periods cannot be derived. Make sure Last Pmt Date falls " +
				"after 1st Pmt Date.")
		}
		if n == math.MaxInt32 {
			// The DOS "forever" sentinel (last date in year 2149) — a schedule
			// cannot be generated from it. DOS would attempt maxint rows; the
			// port refuses with a clear message instead.
			return fmt.Errorf("Last Pmt Date is the \"forever\" sentinel, so the " +
				"number of periods cannot be derived. Enter a real Last Pmt Date " +
				"or # Periods directly.")
		}
		loan.NPeriods = n
		loan.LastDate = snapped
		loan.NStatus = types.InOutOutput
		loan.LastOK = true
	} else if loan.LastStatus >= types.InOutDefault {
		// Last date was supplied directly.
		loan.LastOK = true
	}

	return nil
}
