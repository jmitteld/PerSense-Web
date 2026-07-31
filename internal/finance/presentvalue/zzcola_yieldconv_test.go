package presentvalue

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// Regression guard for the COLA yield->continuous conversion (docs/discrepancies.md §48).
//
// ROOT CAUSE. DOS stores `periodic.cola` ALREADY CONTINUOUS: PRESVALU.pas:281 is
// a bare `exp_cola := exxp(cola)`. The PV screen re-renders that cell through
// PercentValueFromCell's COLAcol arm, `YieldFromRate(rp^, 1)`
// (INTSUTIL.pas:1601-1606), and YieldFromRate(rr,1) = 1*(exxp(rr/1)-1)
// (INTSUTIL.pas:1263-1268). The unique inverse of that display is therefore
// RateFromYield(yield, 1) = 1*lnn(1 + yield/1) (INTSUTIL.pas:1270-1275) — the
// n=1 form. The port applies the conversion at point of use instead of at cell
// entry (the DOS keystroke unit INPUT.pas/PEPANE.pas is not in the surviving
// checkout), so it has to use the same arithmetic to land on the same double.
//
// These sites previously used `math.Log1p(yield)`. log1p is NOT lnn(1+y): it
// never forms the intermediate `1+y`, so it does not incur that rounding. The
// two disagree in the last bits, and a bit-level differential over
// basis x peryr x cola x colamonth (2880 cells, screen total compared as a raw
// float64 rather than as FPC's 6dp text) showed 1048 divergences before this
// change and 0 after — with divergences identically zero on every cola == 0
// cell, which is what localized the defect to the conversion.

// TestCOLAContinuousIsDOSRateFromYield pins the conversion itself to DOS's
// RateFromYield with n = 1, and pins that it is observably NOT math.Log1p.
func TestCOLAContinuousIsDOSRateFromYield(t *testing.T) {
	for _, yrDays := range []float64{360, 365.25} {
		for _, y := range []float64{0.02, 0.03, 0.05, 0.10, 0.125, 0.21, 0.5} {
			got, err := colaContinuous(y, yrDays)
			if err != nil {
				t.Fatalf("colaContinuous(%v, %v): %v", y, yrDays, err)
			}
			// INTSUTIL.pas:1270-1275 with n=1; RealPerYr(1) = 1 (INTSUTIL.pas:1255).
			want, err := interest.RateFromYield(y, 1, yrDays)
			if err != nil {
				t.Fatalf("RateFromYield(%v, 1, %v): %v", y, yrDays, err)
			}
			if math.Float64bits(got) != math.Float64bits(want) {
				t.Errorf("cola=%v yrDays=%v: colaContinuous=%016X want RateFromYield(...,1)=%016X",
					y, yrDays, math.Float64bits(got), math.Float64bits(want))
			}
		}
	}

	// A COLA of 0 must stay exactly 0 — DOS never converts an absent COLA
	// (PRESVALU.pas:610-611 defaults it to 0 when colastatus < inp).
	if v, err := colaContinuous(0, 360); err != nil || v != 0 {
		t.Errorf("colaContinuous(0) = %v, %v; want 0, nil", v, err)
	}

	// The mechanism, made explicit: lnn(1+y) and log1p(y) are different
	// functions. If this ever stops holding, the two conversions have
	// converged and the guard above has quietly stopped testing anything.
	const witness = 0.10
	lnn1p, err := colaContinuous(witness, 360)
	if err != nil {
		t.Fatal(err)
	}
	if math.Float64bits(lnn1p) == math.Float64bits(math.Log1p(witness)) {
		t.Errorf("lnn(1+%v)=%016X is bit-equal to math.Log1p(%v)=%016X — the witness "+
			"no longer distinguishes the two conversions; pick a new one",
			witness, math.Float64bits(lnn1p), witness, math.Float64bits(math.Log1p(witness)))
	}
}

// TestPVCOLAScreenTotalMatchesDOSBits asserts the PV screen total is the SAME
// float64 the DOS engine produces — not merely equal to 6 decimal places.
//
// PROVENANCE. Every `wantBits` below is the raw float64 of `c[1]^.sumvalue`
// taken from the real DOS engine via legacy/oracle/pv_oracle (built by
// legacy/oracle/build_linux.sh), with its `Writeln('pv ', ...:0:6)` temporarily
// extended to also emit `hexstr(qword, 16)` of the same variable. Command shape:
//
//	pv_oracle table <RATE> <BASIS> detail none <COLAMONTH> asof=1.1.2028 \
//	          per=1.6.2030:1.6.2085:<PERYR>:1000.00:<COLA>
//
// The 6dp figure printed by the stock oracle is given alongside each case so the
// value can be re-checked without the precision patch. NOTE that the stock 6dp
// text is NOT usable as the golden: FPC's `:0:6` rounds to ~16 significant
// digits before rounding to 6 decimals, so it renders a value just below a half
// as if it were above one (see TestFPCSixDecimalRenderingIsNotAValueDifference).
func TestPVCOLAScreenTotalMatchesDOSBits(t *testing.T) {
	cases := []struct {
		name      string
		rate      float64
		basis     types.BasisType
		peryr     int
		cola      float64
		colaMonth byte
		dos6dp    string
		wantBits  uint64
	}{
		{"360/12/ann", 0.10, types.Basis360, 12, 0.03, types.COLAAnnual, "129664.291510", 0x40FFA804AA06CE62},
		{"365360/6/cnt", 0.075, types.Basis365360, 6, 0.05, types.COLAContinuous, "145976.670096", 0x4101D1C55C5B1315},
		{"365360/6/nov", 0.10, types.Basis365360, 6, 0.10, 11, "229879.671589", 0x410C0FBD5F6A4422},
		{"365/24/jun", 0.18, types.Basis365, 24, 0.02, 6, "96430.221974", 0x40F78AE38D34245F},
		{"360/26/ann", 0.125, types.Basis360, 26, 0.05, types.COLAAnnual, "243703.524067", 0x410DBFBC314A44FF},
		{"365/52/cnt", 0.06, types.Basis365, 52, 0.03, types.COLAContinuous, "1205303.128973", 0x41326437210462F4},
		{"365360/3/ann", 0.09, types.Basis365360, 3, 0.10, types.COLAAnnual, "149809.370833", 0x4102498AF7770FBE},
		{"360/2/nov", 0.14, types.Basis360, 2, 0.05, 11, "16082.960737", 0x40CF697AF970797C},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := PVSettings{Basis: tc.basis, PerYr: 12, COLAMonth: tc.colaMonth}
			// Mirrors pvprobe / tblInput / DOS SetYrDays (INTSUTIL.pas:333):
			// 365.25 for x365, 360 otherwise (x365_360 counts actual days over
			// a 360 denominator).
			if tc.basis == types.Basis365 {
				s.YrDays, s.YrInv = 365.25, 1/365.25
			} else {
				s.YrDays, s.YrInv = 360, 1.0/360
			}
			res := Calculate(PVInput{
				Periodics: []PeriodicPayment{{
					FromDateStatus: types.InOutInput, FromDate: types.NewDateRec(2030, time.June, 1),
					ToDateStatus: types.InOutInput, ToDate: types.NewDateRec(2085, time.June, 1),
					PerYrStatus: types.InOutInput, PerYr: tc.peryr,
					AmtStatus: types.InOutInput, Amt: 1000.00,
					COLAStatus: types.InOutInput, COLA: tc.cola,
				}},
				PresVal: PresValLine{
					AsOfStatus: types.InOutInput, AsOf: types.NewDateRec(2028, time.January, 1),
					R: RateEntry{Status: types.InOutInput, Rate: tc.rate, PerYr: 1},
				},
				Settings: s,
			})
			if res.Err != nil {
				t.Fatalf("Calculate: %v", res.Err)
			}
			if got := math.Float64bits(res.SumValue); got != tc.wantBits {
				t.Errorf("screen total bits = %016X (%.17g), want DOS %016X (oracle 6dp %s)\n"+
					"  delta = %d ULP — the COLA conversion no longer matches "+
					"RateFromYield(yield, 1) (INTSUTIL.pas:1270)",
					got, res.SumValue, tc.wantBits, tc.dos6dp,
					int64(got)-int64(tc.wantBits))
			}
		})
	}
}

// TestFPCSixDecimalRenderingIsNotAValueDifference records why a 1-in-the-last-
// printed-place disagreement against an oracle's `:0:6` output is NOT evidence
// of an engine divergence, so the next differential run does not spend a round
// chasing one (round 5 logged two such cases as unexplained noise; both were
// bit-identical).
//
// FPC's Str/Write for a double converts to ~16 significant digits and THEN
// rounds to the requested decimals, so it double-rounds: a value whose exact
// expansion continues ...4999xxx just past the cut is first pulled up to
// ...5000 and then rounded half-up, landing one ulp-of-last-place above what
// correct rounding gives. Go's strconv is correctly rounded and does not.
//
// Measured over 200,000 random doubles in 1e2..1e6 (the PV/amort magnitude
// band), FPC's `:0:6` disagreed with Go's `%.6f` on 15 of them (0.0075%), every
// one of that shape.
//
// The value below is the real instance found by the 2880-cell bit sweep:
//
//	pv_oracle table 0.125 365360 detail none ann asof=1.1.2028 \
//	          per=1.6.2030:1.6.2085:12:1000.00:0.02
//	-> pv 83347.209124   (bits 40F459335891E214, exact 83347.2091234999825...)
//
// Both engines hold bit pattern 40F459335891E214. Go renders 83347.209123,
// which is the correctly rounded 6dp form; FPC renders 83347.209124.
func TestFPCSixDecimalRenderingIsNotAValueDifference(t *testing.T) {
	const dosBits = 0x40F459335891E214
	x := math.Float64frombits(dosBits)

	if got := strconvFormat6(x); got != "83347.209123" {
		t.Errorf("Go 6dp rendering = %s, want 83347.209123 (correctly rounded)", got)
	}
	// The 7th decimal onward is 4999825..., i.e. strictly BELOW a half, so
	// correct rounding must go down. This is the property FPC violates.
	frac := x*1e6 - math.Floor(x*1e6)
	if frac >= 0.5 {
		t.Fatalf("test premise broken: fractional part %v is not below 0.5", frac)
	}
}

func strconvFormat6(x float64) string {
	return strconv.FormatFloat(x, 'f', 6, 64)
}
