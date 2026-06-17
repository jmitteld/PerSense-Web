package amortization

import (
	"github.com/persense/persense-port/internal/types"
	"testing"
	"time"
)

func TestDbg2(t *testing.T) {
	mk := func() LoanInput {
		in := baseInput30y()
		in.Prepayments = []Prepayment{{
			StartDateStatus: types.InOutInput,
			StartDate:       types.NewDateRec(2024, time.February, 1),
			PerYrStatus:     types.InOutInput,
			PerYr:           12,
			PaymentStatus:   types.InOutInput,
			Payment:         500,
			NextDate:        types.NewDateRec(2024, time.February, 1),
		}}
		return in
	}
	// with NN bounded so AO10 does not fire
	a := mk()
	a.Prepayments[0].NNStatus = types.InOutInput
	a.Prepayments[0].NN = 1 << 20
	ra := Amortize(a)
	t.Logf("NN=1M: rows=%d final=%.2f err=%v", len(ra.Schedule), ra.FinalPrinc, ra.Err)
	// with stopdate far future
	b := mk()
	b.Prepayments[0].StopDateStatus = types.InOutInput
	b.Prepayments[0].StopDate = types.NewDateRec(2060, 1, 1)
	rb := Amortize(b)
	t.Logf("stop2060: rows=%d final=%.2f", len(rb.Schedule), rb.FinalPrinc)
}
