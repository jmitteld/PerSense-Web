package presentvalue

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// Regression guard for audit finding D1-followup (discrepancies.md §32): the
// per-payment stepped-COLA paths (variable-rate, life, and the exact fixed-rate
// branch) advanced the COLA anniversary through nextColaAnniversary, whose
// NewDateRec NORMALIZES a Feb-29 anchor to Mar-01. DOS instead holds the
// anniversary as a raw, unnormalized daterec {29,2,y} — its dateok() only checks
// 1<=month<=13 (INTSUTIL.pas:584) and DateComp overlays (d,m,y) as a longint
// comparing y, then m, then d (INTSUTIL.pas:828). So DOS's Feb-29 anniversary
// sorts BETWEEN Feb-28 and Mar-01, and in a leap year the anniversary lands
// exactly on the Feb-29 payment. The normalized Mar-01 form stepped one payment
// late every leap year, under-COLA'ing that Feb-29 payment (~0.05% low on a
// 5-year monthly stream). All three per-payment paths now route the anniversary
// through colaAnniversary (raw y,m,d), matching DOS exactly.
//
// PROVENANCE (real DOS engine via the pv_oracle vrp_gen mode, pvlfancy path):
// as-of 01/01/2024, periodic $1,000/mo, from 02/29/2024, 60 payments, 12/yr,
// single flat rate 5.0000%, COLA 3.0000% (ANN). DOS Sum Value:
//
//	x360     56,655.888694
//	x365     56,660.904252  (PVL 365.25-day year)
//	x365/360 56,553.090569
//
// Before the fix the port returned ~56,629.26 / ~56,634.28 / ~56,526.55.
func TestColaLeapAnchorMatchesDOS_D1Followup(t *testing.T) {
	// (1) Raw-anniversary semantics: a Feb-29 anniversary is reached by a
	// Feb-29 payment (leap year) but NOT by a Feb-28 payment (non-leap), and
	// is reached by any March payment — DOS's y,m,d longint ordering.
	an := colaAnniversary{year: 2028, month: 2, day: 29}
	if !an.reached(types.NewDateRec(2028, time.February, 29)) {
		t.Error("Feb-29 anniversary must be reached by the Feb-29 leap payment")
	}
	if an2 := (colaAnniversary{year: 2025, month: 2, day: 29}); an2.reached(types.NewDateRec(2025, time.February, 28)) {
		t.Error("Feb-29 anniversary must NOT be reached by a Feb-28 (non-leap) payment")
	}
	if an3 := (colaAnniversary{year: 2025, month: 2, day: 29}); !an3.reached(types.NewDateRec(2025, time.March, 1)) {
		t.Error("Feb-29 anniversary must be reached by a Mar-01 payment")
	}

	// (2) Value match against the DOS oracle constants above, across all three
	// bases, through the variable-rate path (single flat rate == fixed rate).
	run := func(basis types.BasisType) float64 {
		ctx := interest.NewCalcContext(basis, 12)
		asOf := types.NewDateRec(2024, time.January, 1)
		fromDate := types.NewDateRec(2024, time.February, 29)
		endM := (2 - 1) + 60
		toDate := types.NewDateRec(2024+endM/12, time.Month((endM%12)+1), 29)
		s := PVSettings{Basis: basis, PerYr: 12, COLAMonth: types.COLAAnnual,
			Exact: false, YrDays: ctx.YrDays, YrInv: ctx.YrInv}
		sched := []RateLine{{Date: types.NewDateRec(1900, time.January, 1), Rate: 0.05}}
		v, _, _, err := vrPeriodicValue(1000, 0.03, asOf, fromDate, toDate, 12, sched, &s, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, tc := range []struct {
		basis types.BasisType
		want  float64
		name  string
	}{
		{types.Basis360, 56655.888694, "x360"},
		{types.Basis365, 56660.904252, "x365"},
		{types.Basis365360, 56553.090569, "x365/360"},
	} {
		got := run(tc.basis)
		if got < tc.want-0.01 || got > tc.want+0.01 {
			t.Errorf("VR Feb-29 leap-anchor %s = %.6f, want DOS %.6f (pre-fix ~%.2f)",
				tc.name, got, tc.want, got) // pre-fix was ~26 low
		}
	}

	// (3) The exact fixed-rate path must agree with the VR single-rate path on
	// the same leap-anchored inputs (both now use the raw anniversary).
	ctx := interest.NewCalcContext(types.Basis360, 12)
	asOf := types.NewDateRec(2024, time.January, 1)
	fromDate := types.NewDateRec(2024, time.February, 29)
	endM := (2 - 1) + 60
	toDate := types.NewDateRec(2024+endM/12, time.Month((endM%12)+1), 29)
	sExact := PVSettings{Basis: types.Basis360, PerYr: 12, COLAMonth: types.COLAAnnual,
		Exact: true, YrDays: ctx.YrDays, YrInv: ctx.YrInv}
	fx, err := periodicSumAnnualCOLA(0.05, 0.03, asOf, fromDate, toDate, 12, 60, &sExact)
	if err != nil {
		t.Fatal(err)
	}
	if fxScaled := fx * 1000; fxScaled < 56655.878694 || fxScaled > 56655.898694 {
		t.Errorf("fixed-rate exact Feb-29 leap-anchor x360 = %.6f, want DOS 56655.888694", fxScaled)
	}
}
