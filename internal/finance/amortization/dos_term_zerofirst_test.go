package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestTermSolveZeroFirstPeriodMatchesDOS guards the 2026-07-16 fuzz-hunt finding:
// a term (# periods) solve on a loan whose first payment date EQUALS the loan
// date — a zero-length first period, e.g. a semimonthly loan booked with the
// first payment on the loan date — must use the same first-period prorate as the
// forward schedule (and as DOS's closed form), not default to 1.0.
//
// The closed form n = round(1.4999 + ln(1 - p1(1-ff)/(ff·d)) / ln ff)
// (AMORTOP.pas:1388) is fed p1 = p·ffFirst − d, ffFirst = 1 + (f−1)·prorate. With
// a zero first period the prorate is 0 (ffFirst = 1); defaulting it to 1.0 makes
// the first payment accrue a full period of interest that never happens, which —
// near the interest-only boundary — over-counts the term badly. The port used to
// report NPeriods=410 while its own schedule retired in 377 rows; DOS reports 377.
//
// Oracle provenance:
//
//	amort_oracle 3432511.32 0.2157590000 238 24 loandmy=1.1.2020 firstdmy=1.1.2020 payhard=31699.20 solveterm
//	  → term 377          (zero first period)
//	amort_oracle 3432511.32 0.2159630000 238 24 loandmy=1.1.2020 firstdmy=16.1.2020 payhard=31699.20 solveterm
//	  → term 410          (realistic +15-day first period; unchanged)
func TestTermSolveZeroFirstPeriodMatchesDOS(t *testing.T) {
	ld := types.NewDateRec(2020, time.January, 1)
	mk := func(fd types.DateRec) LoanInput {
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 3432511.32,
			LoanRateStatus: types.InOutInput, LoanRate: 0.215963,
			PayAmtStatus: types.InOutInput, PayAmt: 31699.20,
			NStatus:     types.StatusEmpty,
			PerYrStatus: types.InOutInput, PerYr: 24,
			LoanDateStatus: types.InOutInput, LoanDate: ld,
			FirstStatus: types.InOutInput, FirstDate: fd,
		}, Settings: Settings{Basis: types.Basis360}}
	}

	// Zero first period (firstDate = loanDate): DOS solves 377.
	rz := Amortize(mk(ld))
	if rz.Err != nil {
		t.Fatalf("zero-first-period term solve errored: %v", rz.Err)
	}
	if rz.NPeriods != 377 {
		t.Errorf("zero-first-period term solve: NPeriods=%d, want 377 (DOS)", rz.NPeriods)
	}
	// The reported term must equal the rendered schedule length (internal
	// consistency — the bug was NPeriods=410 while len(schedule)=377).
	if rz.NPeriods != len(rz.Schedule) {
		t.Errorf("term %d != rendered rows %d — solve/schedule inconsistent", rz.NPeriods, len(rz.Schedule))
	}

	// Realistic +15-day first period: DOS solves 410 (regression guard — the fix
	// must not perturb the normal, positive-prorate path).
	rn := Amortize(mk(types.NewDateRec(2020, time.January, 16)))
	if rn.Err != nil {
		t.Fatalf("realistic-first-period term solve errored: %v", rn.Err)
	}
	if rn.NPeriods != 410 {
		t.Errorf("realistic-first-period term solve: NPeriods=%d, want 410 (DOS)", rn.NPeriods)
	}
}
