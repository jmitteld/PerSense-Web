package api

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// ROUND-42 GUARDS — advanced-option STACKING on the Present Value screen.
//
// Round 39 established R39 ("a cell the original computes must be
// TRANSPORTED, never reconstructed") and R40 ("ask which outputs the
// consumer never reads back") on the amortization screen. Round 42 asked
// the same questions of the PV screen's stackable modifiers — the life
// contingency, the Payment on Death, the variable-rate schedule and the
// backward-solve targets — and found the same family of defects.
//
// These are the API-LAYER guards. Round 39's own postmortem is that the
// wire fields it added had ZERO API-layer tests (items A1-A5), so a fix
// verified at the producer shipped broken at the consumer (R42). Each
// test below therefore pins the WIRE, and each was run against the
// pre-fix tree and seen to FAIL (R38).

// r42Qx is the same mock mortality curve podOnlyQx uses, so a failure here
// vs. in pv_pod_only_test.go isolates the stacking, not the table.
func r42Qx() [][]float64 { return podOnlyQx() }

func r42Call(t *testing.T, m map[string]any) PVResponse {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, code := pvCall(t, string(b))
	if code != 200 {
		t.Fatalf("status = %d, want 200 (body %+v)", code, resp)
	}
	return resp
}

func r42Actuarial(pod any) map[string]any {
	a := map[string]any{
		"table1":  r42Qx(),
		"dob1":    "1940-10-10",
		"asOfNow": "2024-01-01",
	}
	if pod != nil {
		a["pod"] = pod
	}
	return a
}

// §75 — THE SOLVED PAYMENT ON DEATH MUST REACH THE WIRE WITH THE RIGHT
// NUMBER ON IT.
//
// DOS: PRESVALU.pas:1268-1269
//
//	if (podunk) then ComputeUnknownPOD;
//	PlacePODOnScreen;
//
// — the solve is followed UNCONDITIONALLY by painting the answer onto the
// screen. The port computed it correctly (presentvalue.solveUnknownPOD)
// and shipped it as `pod`, and no consumer read it: the one number the run
// existed to produce was invisible and the cell stayed blank. The client
// fix is in index.html (§75); this pins the contract it now depends on.
func TestR42_SolvedPODIsOnTheWireAndIsRight(t *testing.T) {
	rows := []map[string]any{{"date": "2030-01-01", "amount": 50000.0, "act": "L"}}

	// Rows-only present value, POD pinned at zero.
	base := r42Call(t, map[string]any{
		"asOfDate": "2024-01-01", "rate": 0.06,
		"actuarial": r42Actuarial(0.0), "lumpSums": rows,
	})
	if base.Error != "" {
		t.Fatalf("baseline rejected: %s", base.Error)
	}
	if base.SumValue <= 0 {
		t.Fatalf("baseline sumValue = %.4f, want > 0", base.SumValue)
	}

	// Now ask for a total $10,000 above that, with the POD cell BLANK.
	const gap = 10000.0
	target := base.SumValue + gap
	got := r42Call(t, map[string]any{
		"asOfDate": "2024-01-01", "rate": 0.06, "sumValue": target,
		"actuarial": r42Actuarial(nil), "lumpSums": rows,
	})
	if got.Error != "" {
		t.Fatalf("POD solve rejected: %s", got.Error)
	}
	// The wire must carry the solved death benefit. This is the assertion
	// that fails on the pre-§75 client contract by being unread; here it
	// pins that the value exists and is not zero.
	if got.POD <= 0 {
		t.Fatalf("wire field `pod` = %.6f, want the solved death benefit > 0", got.POD)
	}
	// And it must be RIGHT: the whole point of the solve is that the POD's
	// present value closes the gap exactly.
	if math.Abs(got.PODValue-gap) > 0.01 {
		t.Errorf("podValue = %.4f, want the %.2f gap it was solved to close", got.PODValue, gap)
	}
	if math.Abs(got.SumValue-target) > 0.01 {
		t.Errorf("sumValue = %.4f, want the requested target %.4f", got.SumValue, target)
	}
	// Non-vacuity: the solved face amount must exceed its own present
	// value (it is discounted and survival-weighted), so a stub returning
	// the gap itself would not pass.
	if got.POD <= got.PODValue {
		t.Errorf("solved POD face %.4f <= its present value %.4f — not a discounted benefit",
			got.POD, got.PODValue)
	}
}

// §77 — ABSENT IS NOT ZERO. Omitting `pod` means "solve for the death
// benefit"; sending 0 means "there is no death benefit". The client used
// to gate on `pod > 0` (index.html getActuarialConfig), so a TYPED zero
// was transmitted as absent and silently became a solve request. The
// server contract the fix relies on is pinned here.
func TestR42_ExplicitZeroPODIsNotASolveRequest(t *testing.T) {
	rows := []map[string]any{{"date": "2030-01-01", "amount": 50000.0, "act": "L"}}
	base := r42Call(t, map[string]any{
		"asOfDate": "2024-01-01", "rate": 0.06,
		"actuarial": r42Actuarial(0.0), "lumpSums": rows,
	})
	target := base.SumValue + 10000

	zero := r42Call(t, map[string]any{
		"asOfDate": "2024-01-01", "rate": 0.06, "sumValue": target,
		"actuarial": r42Actuarial(0.0), "lumpSums": rows,
	})
	if zero.POD != 0 {
		t.Errorf("pod = %.6f with an explicit pod:0 — the engine solved for a "+
			"death benefit the caller said was zero", zero.POD)
	}
	// With POD pinned at zero the typed target is over-determined and must
	// NOT be honoured — the total is the rows' own value.
	if math.Abs(zero.SumValue-base.SumValue) > 0.01 {
		t.Errorf("sumValue = %.4f with pod:0, want the rows-only %.4f",
			zero.SumValue, base.SumValue)
	}

	// The control: the same worksheet with `pod` OMITTED does solve.
	blank := r42Call(t, map[string]any{
		"asOfDate": "2024-01-01", "rate": 0.06, "sumValue": target,
		"actuarial": r42Actuarial(nil), "lumpSums": rows,
	})
	if blank.POD <= 0 {
		t.Fatalf("control: omitting pod must request the solve, got pod=%.6f", blank.POD)
	}
}

// §76 — A RATE SCHEDULE MUST NOT SILENCE THE ADVISORY CHANNEL.
//
// Calculate's variable-rate branch used to `return` from each arm, which
// stepped over appendResultAdvisories at the bottom of the function.
// Stacking a schedule onto a worksheet therefore removed every advisory
// the SAME worksheet raises at a fixed rate. advisories.go's own doc
// comment recorded the bypass as though it were a design note.
//
// The pair below is the R36 before/after on one worksheet: the fixed-rate
// arm is the positive control (it must produce P-W4, or the test is
// vacuous), and the variable-rate arm must now produce it too.
func TestR42_VariableRateDoesNotSuppressAdvisories(t *testing.T) {
	// A second lump row with a blank amount, and a target equal to the
	// first row's value alone — so the solved amount comes out ~0 (P-W4).
	rows := []map[string]any{
		{"date": "2030-01-01", "amount": 50000.0},
		{"date": "2031-01-01"},
	}
	sched := []map[string]any{
		{"date": "2024-01-01", "trueRate": 0.06},
		{"date": "2028-01-01", "trueRate": 0.08},
	}

	fwdFixed := r42Call(t, map[string]any{
		"asOfDate": "2024-01-01", "rate": 0.06,
		"lumpSums": []map[string]any{{"date": "2030-01-01", "amount": 50000.0}},
	})
	fwdVR := r42Call(t, map[string]any{
		"asOfDate": "2024-01-01", "rate": 0.06, "rateSchedule": sched,
		"lumpSums": []map[string]any{{"date": "2030-01-01", "amount": 50000.0}},
	})

	hasPW4 := func(ws []string) bool {
		for _, w := range ws {
			if strings.Contains(w, "P-W4") {
				return true
			}
		}
		return false
	}

	fixed := r42Call(t, map[string]any{
		"asOfDate": "2024-01-01", "rate": 0.06, "sumValue": fwdFixed.SumValue,
		"lumpSums": rows,
	})
	// POSITIVE CONTROL (R24): without it a passing VR arm proves nothing.
	if !hasPW4(fixed.Warnings) {
		t.Fatalf("positive control failed: the FIXED-rate arm produced no P-W4 "+
			"(warnings=%v). The test can no longer detect the suppression it "+
			"was written for.", fixed.Warnings)
	}

	vr := r42Call(t, map[string]any{
		"asOfDate": "2024-01-01", "rate": 0.06, "sumValue": fwdVR.SumValue,
		"rateSchedule": sched, "lumpSums": rows,
	})
	if !hasPW4(vr.Warnings) {
		t.Errorf("§76: stacking a rate schedule suppressed P-W4 — the identical "+
			"worksheet raises it at a fixed rate. warnings=%v", vr.Warnings)
	}
	// The advisory pass must not have been run twice.
	if n := len(vr.Warnings); n > 1 {
		t.Errorf("§76: %d warnings on the VR arm, want 1 — appendResultAdvisories "+
			"is being applied more than once. warnings=%v", n, vr.Warnings)
	}
	// And it must not have moved a number (advisories never change arithmetic).
	if math.Abs(vr.SumValue-fwdVR.SumValue) > 0.01 {
		t.Errorf("§76 changed a computed number: sumValue %.6f vs %.6f",
			vr.SumValue, fwdVR.SumValue)
	}
}

// §78 — CHARACTERIZATION, NOT A FIX.
//
// DOS sets `podunk` FALSE at the top of Enter (PRESVALU.pas:1156) and only
// ever sets it TRUE when there is nothing else left to solve — either the
// frontward branch's "All blanks are filled in. Proceed only to compute an
// unknown POD amount." (:1177-1179) or the no-payments branch (:1207) — and
// in BOTH cases only if the user does not press Escape. BackwardCalc runs
// on its own path with podunk false, so the POD solve NEVER pre-empts
// another backward solve in DOS.
//
// The port fires it on (POD absent AND a Sum Value present), BEFORE
// FirstPass, so it pre-empts. Round 42 measured that pre-emption and found
// it currently INERT: solveUnknownPOD's first step re-enters Calculate with
// POD=0 and the SAME blank field and target still in place, so the nested
// backward solve drives the residual to solver noise and the solved POD is
// identically ~0 — the outer answer is bit-identical to the POD=0 arm.
//
// Because it is inert it is FILED, not changed (R20: a fix that changes
// nothing has not been confirmed). This test pins the inertness, so the day
// the dispatch order or a solver tolerance moves, it is caught here instead
// of silently changing a shipped answer.
func TestR42_PODSolveIsInertWhenAnotherFieldIsBlank(t *testing.T) {
	mk := func(pod any) map[string]any {
		return map[string]any{
			"asOfDate": "2024-01-01", "sumValue": 150000.0, // rate deliberately blank
			"actuarial": r42Actuarial(pod),
			"periodics": []map[string]any{{
				"fromDate": "2024-01-01", "toDate": "2044-01-01",
				"perYr": 12, "amount": 1000.0, "act": "L",
			}},
		}
	}
	blank := r42Call(t, mk(nil))
	zero := r42Call(t, mk(0.0))
	if blank.Error != "" || zero.Error != "" {
		t.Fatalf("errors: blank=%q zero=%q", blank.Error, zero.Error)
	}
	// Non-vacuity: a rate really was solved on both arms.
	if blank.Rate <= 0 || zero.Rate <= 0 {
		t.Fatalf("no rate solved (blank=%.9f zero=%.9f) — the test is not "+
			"exercising a backward solve any more", blank.Rate, zero.Rate)
	}
	if blank.Rate != zero.Rate {
		t.Errorf("§78 is no longer inert: solved rate %.15f with the POD blank vs "+
			"%.15f with POD=0. The POD pre-emption now MOVES a shipped answer, so "+
			"the DOS gate (PRESVALU.pas:1156,1177-1179,1207) must be ported before "+
			"this ships.", blank.Rate, zero.Rate)
	}
	if math.Abs(blank.POD) > 1e-6 {
		t.Errorf("§78: solved POD = %.9g, expected ~0 — the residual "+
			"solveUnknownPOD divides by is no longer solver noise.", blank.POD)
	}
}
