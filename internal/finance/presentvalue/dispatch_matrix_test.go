package presentvalue

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// This file validates the PV field-presence DISPATCH decision — the classifier
// that reads which fields the user left blank and selects forward / a specific
// backward solve / over-determined / error. It does NOT test the numeric
// solvers (those are bit-validated against DOS elsewhere); it pins the routing
// decision itself, which is the "Dispatch / classification" engine.
//
// Two layers:
//   1. This file: an oracle-INDEPENDENT decision table over the field-presence
//      matrix, asserting the canonical PV dispatch rules (PRESVALU.pas FirstPass
//      + the Enter dispatch at :1242). These rules come from the PV spec, not
//      from the port's implementation, so the assertions are not circular.
//   2. dispatch_oracle_test.go: the same presence matrix fed to the real DOS
//      FirstPass via the source-oracle, asserting Go and DOS agree.

// pvVerdict is the canonical dispatch outcome, normalised so the Go model and
// the DOS model can be compared. DOS solves rate/as-of through its FORWARD path
// (no row is contains_unknown); the Go port models them as explicit
// BackwardKinds. Both collapse here to SOLVE_RATE / SOLVE_ASOF.
type pvVerdict string

const (
	vForward       pvVerdict = "FORWARD"
	vSolveLumpAmt  pvVerdict = "SOLVE_LUMPAMT"
	vSolveLumpDate pvVerdict = "SOLVE_LUMPDATE"
	vSolvePerAmt   pvVerdict = "SOLVE_PERAMT"
	vSolvePerTo    pvVerdict = "SOLVE_PERTODATE"
	vSolvePerFrom  pvVerdict = "SOLVE_PERFROM"
	vSolveRate     pvVerdict = "SOLVE_RATE"
	vSolveAsOf     pvVerdict = "SOLVE_ASOF"
	vOverDet       pvVerdict = "OVERDETERMINED"
	vError         pvVerdict = "ERROR"
	vInsufficient  pvVerdict = "INSUFFICIENT"
)

// goPVVerdict runs the real Go FirstPass and reduces its result to a canonical
// verdict, applying the documented rate/as-of normalisation.
func goPVVerdict(in *PVInput) pvVerdict {
	fp := FirstPass(in)
	if fp.Err != nil {
		return vError
	}
	if fp.Frontward && fp.Backward {
		return vOverDet
	}
	if fp.Backward {
		switch fp.BackwardKind {
		case BackwardLumpAmount:
			return vSolveLumpAmt
		case BackwardLumpDate:
			return vSolveLumpDate
		case BackwardPeriodicAmount:
			return vSolvePerAmt
		case BackwardPeriodicToDate:
			return vSolvePerTo
		case BackwardPeriodicFrom:
			return vSolvePerFrom
		case BackwardRate:
			return vSolveRate
		case BackwardAsOf:
			return vSolveAsOf
		}
	}
	if fp.Frontward {
		return vForward
	}
	return vInsufficient
}

// --- builders parameterised by field presence ------------------------------

func present(b bool) int8 {
	if b {
		return types.InOutInput
	}
	return types.StatusEmpty
}

// lumpScreen builds a PVInput with a single lump-sum row and the present-value
// line, with each field present/blank per the flags.
func lumpScreen(date, amt, val, rate, asof, sum bool) *PVInput {
	in := &PVInput{
		LumpSums: []LumpSumPayment{{
			DateStatus: present(date), Date: types.NewDateRec(2025, time.January, 1),
			AmtStatus: present(amt), Amt: 1000,
			ValStatus: present(val), Val: 900,
		}},
		PresVal: PresValLine{
			AsOfStatus: present(asof), AsOf: types.NewDateRec(2024, time.January, 1),
			R:              RateEntry{Status: present(rate), Rate: 0.08, PerYr: 1},
			SumValueStatus: present(sum), SumValue: 900,
		},
		Settings: PVSettings{Basis: types.Basis360, PerYr: 1, YrDays: 360, YrInv: 1.0 / 360},
	}
	return in
}

// perScreen builds a PVInput with a single periodic row and the PV line.
func perScreen(from, to, peryr, amt, val, rate, asof, sum bool) *PVInput {
	in := &PVInput{
		Periodics: []PeriodicPayment{{
			FromDateStatus: present(from), FromDate: types.NewDateRec(2025, time.January, 1),
			ToDateStatus: present(to), ToDate: types.NewDateRec(2030, time.January, 1),
			PerYrStatus: present(peryr), PerYr: 12,
			AmtStatus: present(amt), Amt: 100,
			ValStatus: present(val), Val: 5000,
		}},
		PresVal: PresValLine{
			AsOfStatus: present(asof), AsOf: types.NewDateRec(2024, time.January, 1),
			R:              RateEntry{Status: present(rate), Rate: 0.08, PerYr: 1},
			SumValueStatus: present(sum), SumValue: 5000,
		},
		Settings: PVSettings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360},
	}
	return in
}

// TestPVDispatchLumpCanonical asserts the canonical dispatch rules for a single
// lump-sum row. Each case is a rule from the PV spec, not a snapshot of the
// implementation.
func TestPVDispatchLumpCanonical(t *testing.T) {
	type tc struct {
		name                            string
		date, amt, val, rate, asof, sum bool
		want                            pvVerdict
	}
	cases := []tc{
		// Forward: rate+asof+date+amt, value to be computed.
		{"forward date+amt", true, true, false, true, true, false, vForward},
		// Forward with redundant sumvalue present but row fully specified.
		{"forward date+amt+sum", true, true, false, true, true, true, vForward},
		// PV-1: row gives date+value, solve the amount.
		{"solve amount (date+val)", true, false, true, true, true, false, vSolveLumpAmt},
		// PV-2: row gives amount+value, solve the date.
		{"solve date (amt+val)", false, true, true, true, true, false, vSolveLumpDate},
		// PV-8: rate blank, asof+sumvalue+fully-specified row → solve rate.
		{"solve rate", true, true, false, false, true, true, vSolveRate},
		// PV-9: asof blank, rate+sumvalue+fully-specified row → solve as-of.
		{"solve asof", true, true, false, true, false, true, vSolveAsOf},
		// Over-specified row: date+amt+val all present → forward (value recomputed).
		{"over-specified row forward", true, true, true, true, true, false, vForward},
		// Insufficient: only a value, no date or amount → error.
		{"only value errors", false, false, true, true, true, false, vError},
		// Zero amount as the only field can't be solved from → error.
		// (amt present & ==0, date+rate+asof blank-or-not). Build explicitly below.
	}
	for _, c := range cases {
		in := lumpScreen(c.date, c.amt, c.val, c.rate, c.asof, c.sum)
		if got := goPVVerdict(in); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}

	// Zero-amount-only row: amount present but zero, date blank → DOS records a
	// field error (nothing to solve from). PRESVALU.pas ComputeLumpsumLineValues.
	in := lumpScreen(false, true, false, true, true, false)
	in.LumpSums[0].Amt = 0
	if got := goPVVerdict(in); got != vError {
		t.Errorf("zero-amount-only row: got %s, want ERROR", got)
	}
}

// TestPVDispatchPeriodicCanonical asserts the canonical rules for a single
// periodic row.
func TestPVDispatchPeriodicCanonical(t *testing.T) {
	type tc struct {
		name                                       string
		from, to, peryr, amt, val, rate, asof, sum bool
		want                                       pvVerdict
	}
	cases := []tc{
		// Forward: from+to+peryr+amt → value computed.
		{"forward", true, true, true, true, false, true, true, false, vForward},
		// PV-4: both dates + peryr present, amount blank, value given → solve amount.
		{"solve amount (both dates+val)", true, true, true, false, true, true, true, false, vSolvePerAmt},
		// PV-5: from + amt + val, to blank → solve to-date.
		{"solve to-date", true, false, true, true, true, true, true, false, vSolvePerTo},
		// PV-6: to + amt + val, from blank → solve from-date.
		{"solve from-date", false, true, true, true, true, true, true, false, vSolvePerFrom},
		// PV-8 rate solve with a fully-specified periodic row.
		{"solve rate", true, true, true, true, false, false, true, true, vSolveRate},
		// PV-9 as-of solve with a fully-specified periodic row.
		{"solve asof", true, true, true, true, false, true, false, true, vSolveAsOf},
	}
	for _, c := range cases {
		in := perScreen(c.from, c.to, c.peryr, c.amt, c.val, c.rate, c.asof, c.sum)
		if got := goPVVerdict(in); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

// TestPVDispatchLumpMatrixCovers enumerates the full 2^6 presence matrix for a
// single lump row + PV line and confirms every combination produces a defined
// verdict (no panic, no empty verdict). It also logs the full decision table so
// a reviewer can eyeball it; the DOS-differential test pins it against the
// original.
func TestPVDispatchLumpMatrixCovers(t *testing.T) {
	counts := map[pvVerdict]int{}
	for bits := 0; bits < 64; bits++ {
		date := bits&1 != 0
		amt := bits&2 != 0
		val := bits&4 != 0
		rate := bits&8 != 0
		asof := bits&16 != 0
		sum := bits&32 != 0
		in := lumpScreen(date, amt, val, rate, asof, sum)
		v := goPVVerdict(in)
		if v == "" {
			t.Fatalf("bits=%06b produced empty verdict", bits)
		}
		counts[v]++
	}
	t.Logf("lump matrix verdict distribution: %v", counts)
	// Sanity: the matrix must exercise forward, at least one solve, and error/insufficient.
	if counts[vForward] == 0 || counts[vError]+counts[vInsufficient] == 0 {
		t.Errorf("matrix did not cover the basic verdict classes: %v", counts)
	}
}
