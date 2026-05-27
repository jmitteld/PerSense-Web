// Test for dispatch_gaps FP2 / A9: amortization APR-with-points
// (DOS EstimateAndRefineAPRwithPoints).

package api

import (
	"math"
	"testing"
)

// TestAmortAPRWithPoints: discount points raise the APR above the
// note rate, because the borrower's net proceeds shrink while the
// payment stream is unchanged.
func TestAmortAPRWithPoints(t *testing.T) {
	// No points: APR should be close to the 6% note rate.
	noPts, code := amzCall(t, `{
		"amount":200000,"rate":0.06,"loanDate":"2025-01-01",
		"nPeriods":360,"perYr":12,"payment":1199.10,"points":0.0
	}`)
	if code != 200 || noPts["error"] != nil {
		t.Fatalf("no-points calc failed: code=%d err=%v", code, noPts["error"])
	}
	apr0, ok := noPts["apr"].(float64)
	if !ok {
		t.Fatalf("no apr in response: %v", noPts)
	}
	if apr0 < 0.055 || apr0 > 0.065 {
		t.Errorf("zero-points APR = %.5f, expected near the 6%% note rate", apr0)
	}

	// 2 points: APR must rise.
	withPts, code := amzCall(t, `{
		"amount":200000,"rate":0.06,"loanDate":"2025-01-01",
		"nPeriods":360,"perYr":12,"payment":1199.10,"points":0.02
	}`)
	if code != 200 || withPts["error"] != nil {
		t.Fatalf("with-points calc failed: code=%d err=%v", code, withPts["error"])
	}
	apr2, _ := withPts["apr"].(float64)
	if apr2 <= apr0 {
		t.Errorf("APR with 2 points (%.5f) should exceed APR with no points (%.5f)",
			apr2, apr0)
	}
	if conv, _ := withPts["aprConverged"].(bool); !conv {
		t.Errorf("APR solve did not converge")
	}
}

// TestAmortAPRZeroVsOmittedPointsContract pins the exact dispatch the
// Amortization screen depends on after the "(computed)" placeholder
// bug was fixed.
//
// The frontend Points cell ships with value="0". Earlier the frontend
// suppressed the points field whenever the value was not strictly
// positive, on the theory that DOS skips the APR for an untouched
// default (Amortize.pas:1420, `if pointsstatus > defp`). The web port
// has no separate "user-typed 0" vs "default 0" status, so suppressing
// at 0 meant the request body carried no points field, the handler
// never set PointsStatus, the engine's `>= InOutDefault` guard skipped
// the APR computation, and the green APR cell stayed at its
// "(computed)" placeholder forever. The fix is to forward points
// whenever the cell has a parseable number (including 0), and this
// test pins that contract from the API side:
//
//   - "points": 0 (explicit) MUST return an APR equal to the loan
//     rate. The help text and the field tooltip both promise this
//     ("equals the loan rate when Points is 0").
//   - Omitting points entirely MUST return no APR. This is the older
//     dispatch where APR is opt-in — the frontend is the only client
//     that controls which branch fires, and it now always opts in.
//
// If either half of this contract drifts, the Amortization screen's
// APR cell regresses to "(computed)" or starts disagreeing with the
// help text.
func TestAmortAPRZeroVsOmittedPointsContract(t *testing.T) {
	const loanRate = 0.06
	base := `{"amount":200000,"rate":0.06,"loanDate":"2025-01-01",
		"firstDate":"2025-02-01","nPeriods":360,"perYr":12`

	// Half 1: explicit "points":0 must compute an APR equal to the
	// loan rate (within solver tolerance).
	resp, code := amzCall(t, base+`,"points":0}`)
	if code != 200 || resp["error"] != nil {
		t.Fatalf("explicit-zero-points calc failed: code=%d err=%v",
			code, resp["error"])
	}
	apr, ok := resp["apr"].(float64)
	if !ok {
		t.Fatalf("explicit \"points\":0 must return an APR (UI relies " +
			"on this) — got no apr in response")
	}
	if math.Abs(apr-loanRate) > 1e-4 {
		t.Errorf("explicit \"points\":0 APR = %.6f, expected the loan "+
			"rate %.4f (help text: \"equals the loan rate when Points "+
			"is 0\")", apr, loanRate)
	}
	if conv, _ := resp["aprConverged"].(bool); !conv {
		t.Errorf("APR solve did not converge with points=0")
	}

	// Half 2: omitting points entirely must NOT compute an APR.
	// `omitempty` on the response field drops a zero/absent APR from
	// the JSON, so the key should be missing.
	resp2, code := amzCall(t, base+`}`)
	if code != 200 || resp2["error"] != nil {
		t.Fatalf("omitted-points calc failed: code=%d err=%v",
			code, resp2["error"])
	}
	if v, present := resp2["apr"]; present {
		t.Errorf("omitting points should leave APR uncomputed "+
			"(omitempty drops the field) — got apr=%v", v)
	}
}
