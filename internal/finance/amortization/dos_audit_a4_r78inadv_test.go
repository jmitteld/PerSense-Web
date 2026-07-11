package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// A4: R78 × in-advance — R78 split seeded from the annuity-due payment, plus
// the settlement row. Oracle: `amort_oracle 100000 0.10 24 12 r78 inadv dumpraw`
// → payment 4579.8857; row0 1/1/24 int 833.33; row1 793.38; row24 33.06;
// totals interest 10750.59.
func TestA4R78InAdvance(t *testing.T) {
	loan := Loan{
		AmountStatus: types.InOutInput, Amount: 100000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.10,
		PayAmtStatus: types.StatusEmpty,
		NStatus:      types.InOutInput, NPeriods: 24,
		PerYrStatus: types.InOutInput, PerYr: 12,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
	}
	s := basicSettings()
	s.R78 = true
	s.InAdvance = true
	res := Amortize(LoanInput{Loan: loan, Settings: s})
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	if len(res.Schedule) != 25 {
		t.Fatalf("rows = %d, want 25 (settlement + 24 R78 rows)", len(res.Schedule))
	}
	r0, r1, rn := res.Schedule[0], res.Schedule[1], res.Schedule[24]
	pay := r1.PayAmt
	if math.Abs(pay-4579.8857) > 0.05 {
		t.Errorf("payment = %.4f, want 4579.8857 (annuity-due; oracle)", pay)
	}
	if math.Abs(r0.Interest-833.33) > 0.02 || r0.Date.Time.Format("2006-01-02") != "2024-01-01" {
		t.Errorf("row0 = %s int %.4f, want 2024-01-01 int 833.33", r0.Date.Time.Format("2006-01-02"), r0.Interest)
	}
	if math.Abs(r1.Interest-793.38) > 0.05 {
		t.Errorf("row1 int = %.4f, want 793.38 (R78 split of the annuity-due payment)", r1.Interest)
	}
	if math.Abs(rn.Interest-33.06) > 0.05 {
		t.Errorf("row24 int = %.4f, want 33.06", rn.Interest)
	}
	if math.Abs(res.TotalInt-10750.59) > 0.5 {
		t.Errorf("total interest = %.2f, want 10750.59 (oracle)", res.TotalInt)
	}
	// Controls: each flag alone unchanged (oracle: r78-only pay 4614.4926;
	// inadv-only pay 4579.8857).
	s2 := basicSettings()
	s2.R78 = true
	res2 := Amortize(LoanInput{Loan: loan, Settings: s2})
	if p2 := res2.Schedule[0].PayAmt; math.Abs(p2-4614.4926) > 0.05 {
		t.Errorf("r78-only payment = %.4f, want 4614.4926", p2)
	}
}
