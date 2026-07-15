// Pass-8 (2026-07-14) regression: the faithful port AmortizeDOS must apply DOS's
// prepaid-clearing rule itself, so it is correct when invoked STANDALONE (as the
// dosport fuzzers do), not only when the production Amortize wrapper clears
// prepaid before delegating.
//
// DOS clears prepaid outright when the loan is taken strictly AFTER the natural
// period start — firstDate minus one period — i.e. an odd/LONG first stub pushes
// that natural start before the loan date (`if DateComp(natural_start, loandate)
// < 0 and not in_advance then prepaid := false`, Amortize.pas). Before the fix
// AmortizeDOS kept prepaid on those loans and diverged from the oracle
// (TestDOSPortFuzzBasis / TestDOSPortFuzzMerged, opt-in PERSENSE_FUZZ — the
// oddFirst+prepaid bucket). Production was always correct (the wrapper cleared
// prepaid), so this never affected the API; the goldens below guard the raw port
// so a normal `go test` catches a regression without needing the oracle or the
// opt-in fuzz flag.
//
// Goldens transcribed from amort_oracle (loan date 2024-01-01, first = loanDate +
// N months, 360 basis, PlusRegular):
//
//	amort_oracle 152000 0.0699 48 2 first=4 prepaid -> interest 160132.93
//	amort_oracle 100000 0.08   36 12 first=3 prepaid -> interest 14144.25
//	amort_oracle 250000 0.055  60 4 first=5 prepaid -> interest 121059.48
//	amort_oracle 80000  0.10   24 6 first=1 prepaid -> interest 16918.96
package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func TestPass8DosPortPrepaidClearing(t *testing.T) {
	monthsAfter := func(m int) types.DateRec {
		return types.NewDateRec(2024+m/12, time.Month(m%12+1), 1)
	}
	mk := func(amount, rate float64, n, perYr, firstMonths int) LoanInput {
		return LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: amount,
				LoanRateStatus: types.InOutInput, LoanRate: rate,
				NStatus: types.InOutInput, NPeriods: n,
				PerYrStatus: types.InOutInput, PerYr: perYr,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
				FirstStatus: types.InOutInput, FirstDate: monthsAfter(firstMonths),
			},
			Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true, Prepaid: true},
			Fancy:    true,
		}
	}
	cases := []struct {
		name                  string
		amount, rate          float64
		n, perYr, firstMonths int
		wantInt               float64
	}{
		{"perYr2 longFirst=4", 152000, 0.0699, 48, 2, 4, 160132.93},
		{"perYr12 first=3", 100000, 0.08, 36, 12, 3, 14144.25},
		{"perYr4 longFirst=5", 250000, 0.055, 60, 4, 5, 121059.48},
		{"perYr6 shortFirst=1", 80000, 0.10, 24, 6, 1, 16918.96},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := mk(c.amount, c.rate, c.n, c.perYr, c.firstMonths)
			// The raw faithful port must match DOS (this is the fix)...
			if r := AmortizeDOS(in); r.Err != nil || math.Abs(r.TotalInt-c.wantInt) > 0.05 {
				t.Errorf("AmortizeDOS interest=%.2f want %.2f (err=%v) — prepaid-clearing", r.TotalInt, c.wantInt, r.Err)
			}
			// ...and so must the production entry (always did).
			if r := Amortize(mk(c.amount, c.rate, c.n, c.perYr, c.firstMonths)); r.Err != nil || math.Abs(r.TotalInt-c.wantInt) > 0.05 {
				t.Errorf("Amortize interest=%.2f want %.2f (err=%v)", r.TotalInt, c.wantInt, r.Err)
			}
		})
	}
}
