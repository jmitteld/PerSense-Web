package amortization

import (
	"math"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// TestDOSPortGoldens pins AmortizeDOS (the faithful structural port) to the real
// DOS oracle on a spread of cases — including the exact stacks the piecewise
// engine could NOT reproduce (multi-ARM+balloon, balloon-straddling-ARM,
// balloon×ARM×skip). Goldens are the oracle's total interest. The port matches
// every one to the cent; the standing TestDOSPortFuzz (PERSENSE_FUZZ=1) holds it
// to zero divergences across the whole random option cube (validated at N=1000).
//
// These are oracle commands for reproduction:
//
//	amort_oracle 10000 0.12 12 12
//	amort_oracle 100000 0.08 360 12 b120=50000 plusreg
//	amort_oracle 329000 0.0693 240 12 b206=22500 adj=58:0.1194: adj=180:0.0653: plusreg
//	amort_oracle 193000 0.1166 240 12 b32=24500 b108=30000 adj=107:0.0791: plusreg
//	amort_oracle 155000 0.1132 300 12 b275=21000 adj=11:0.1098: adj=44:0.0532: skip=5-7 plusreg
func TestDOSPortGoldens(t *testing.T) {
	base := func(amt, rate float64, n int) LoanInput {
		return LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: amt,
				LoanRateStatus: types.InOutInput, LoanRate: rate,
				NStatus: types.InOutInput, NPeriods: n,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
				FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, 2, 1),
			},
			Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
			Fancy:    true,
		}
	}
	cases := []struct {
		name     string
		in       LoanInput
		wantInt  float64
		wantBal0 bool // expect retire to ~0
	}{
		{"plain", base(10000, 0.12, 12), 661.85, true},
		{"balloon", func() LoanInput {
			in := base(100000, 0.08, 360)
			in.Balloons = []BalloonPayment{balloonAt(120, 50000)}
			return in
		}(), 154651.18, true},
		{"multiARM+balloon", func() LoanInput {
			in := base(329000, 0.0693, 240)
			in.Adjustments = []RateAdjustment{adjRateAt(58, 0.1194), adjRateAt(180, 0.0653)}
			in.Balloons = []BalloonPayment{balloonAt(206, 22500)}
			return in
		}(), 426265.36, true},
		{"balloon2_straddle_ARM", func() LoanInput {
			in := base(193000, 0.1166, 240)
			in.Balloons = []BalloonPayment{balloonAt(32, 24500), balloonAt(108, 30000)}
			in.Adjustments = []RateAdjustment{adjRateAt(107, 0.0791)}
			return in
		}(), 249546.50, true},
		{"balloon_2ARM_skip", func() LoanInput {
			in := base(155000, 0.1132, 300)
			in.Adjustments = []RateAdjustment{adjRateAt(11, 0.1098), adjRateAt(44, 0.0532)}
			in.Balloons = []BalloonPayment{balloonAt(275, 21000)}
			in.SkipMonths = skipSetRaw("5-7")
			return in
		}(), 173376.34, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := AmortizeDOS(c.in)
			if r.Err != nil {
				t.Fatalf("err: %v", r.Err)
			}
			if d := math.Abs(r.TotalInt - c.wantInt); d > 0.5 {
				t.Errorf("total interest = %.2f, want DOS %.2f (±0.50)", r.TotalInt, c.wantInt)
			}
			if c.wantBal0 {
				bal := r.Schedule[len(r.Schedule)-1].Principal
				if bal < -1 || bal > 1 {
					t.Errorf("final balance = %.2f, want ~0", bal)
				}
			}
		})
	}
}
