package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestPayoffBalanceDOSValues pins PayoffBalance to values produced by the REAL DOS
// engine (legacy/oracle/amort_oracle `payoff=` query), so the port is guarded even
// where the Pascal oracle can't be built (the differential TestPayoffVsOracleSweep
// skips then). These cover the branches DOS's ComputeBalanceFromDate distinguishes —
// and the exact user-reported loan (in-advance + Rule-of-78), which the old
// client-side arrears-only formula mispriced (it returned 100,480.61, exceeding the
// principal; DOS returns 101,422.75 because Rule-of-78 front-loads interest).
func TestPayoffBalanceDOSValues(t *testing.T) {
	mk := func(inadv, r78, exact, prepaid bool, basis types.BasisType) LoanInput {
		yd := 365.0
		if basis == types.Basis360 {
			yd = 360
		}
		s := Settings{Basis: basis, PerYr: 12, YrDays: yd, YrInv: 1.0 / yd,
			Prepaid: prepaid, InAdvance: inadv, R78: r78, Exact: exact}
		if basis == types.Basis365360 {
			s.YrInv = 1.0 / 360
		}
		loan := Loan{
			NStatus: types.InOutInput, NPeriods: 360,
			PerYrStatus: types.InOutInput, PerYr: 12,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2015, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2015, 2, 1),
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.08,
			PayAmtStatus: types.StatusEmpty,
		}
		return LoanInput{Loan: loan, Settings: s}
	}
	d := func(y, m, day int) types.DateRec { return types.NewDateRec(y, time.Month(m), day) }

	cases := []struct {
		name string
		in   LoanInput
		asOf types.DateRec
		want float64
	}{
		// The user's exact loan — R78 takes precedence over in-advance in DOS.
		{"user inadv+r78+exact+b365+prepaid @5/1", mk(true, true, true, true, types.Basis365), d(2015, 5, 1), 101422.7480},
		{"arrears/360 on-payment @5/1", mk(false, false, false, false, types.Basis360), d(2015, 5, 1), 100462.6771},
		{"arrears/360 before-first @1/15", mk(false, false, false, false, types.Basis360), d(2015, 1, 15), 100311.1111},
		{"arrears/360 after-loan @1/1/2050", mk(false, false, false, false, types.Basis360), d(2050, 1, 1), 0.0},
		{"r78/360 mid-period @5/15", mk(false, true, false, false, types.Basis360), d(2015, 5, 15), 101000.8239},
	}
	for _, c := range cases {
		got, err := PayoffBalance(c.in, c.asOf)
		if err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
			continue
		}
		if math.Abs(got-c.want) > 0.01 {
			t.Errorf("%s: PayoffBalance = %.4f, DOS = %.4f (diff %.4f)", c.name, got, c.want, got-c.want)
		}
	}

	// Before the loan date is an error (DOS Amortize.pas:1095).
	if _, err := PayoffBalance(mk(false, false, false, false, types.Basis360), d(2014, 12, 1)); err == nil {
		t.Errorf("payoff before loan date should error")
	}
}
