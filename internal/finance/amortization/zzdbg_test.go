package amortization

import (
	"github.com/persense/persense-port/internal/types"
	"testing"
	"time"
)

func TestDbgPrepay(t *testing.T) {
	in := baseInput30y()
	in.Loan.NPeriods = 60
	in.Loan.LastDate = types.NewDateRec(2029, time.January, 1)
	in.Loan.LastOK = true
	in.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput,
		StartDate:       types.NewDateRec(2024, time.February, 1),
		StopDateStatus:  types.InOutInput,
		StopDate:        types.NewDateRec(2029, time.January, 1),
		PerYrStatus:     types.InOutInput,
		PerYr:           12,
		NextDate:        types.NewDateRec(2024, time.February, 1),
		PaymentStatus:   types.InOutInput,
		Payment:         3000,
	}}
	res := Amortize(in)
	t.Logf("with prepay=3000 fixed: FinalPrinc=%.2f sched=%d err=%v", res.FinalPrinc, len(res.Schedule), res.Err)
}
