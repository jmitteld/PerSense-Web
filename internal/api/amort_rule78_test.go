// Test for dispatch_gaps R4-1: Rule-of-78 ("sum of the digits")
// amortization. The Settings.R78 flag is now honored by the basic
// schedule engine.

package api

import (
	"math"
	"testing"
)

// TestRule78FrontLoadsInterest: Rule-of-78 allocates more interest to
// the early periods than the ordinary actuarial method, and the
// per-period interest declines by a constant step.
func TestRule78FrontLoadsInterest(t *testing.T) {
	body := `{"amount":100000,"rate":0.08,"loanDate":"2025-01-01",
		"firstDate":"2025-02-01","nPeriods":360,"perYr":12,"payment":733.76%s}`

	normal, code := amzCall(t, sprintf78(body, ""))
	if code != 200 || normal["error"] != nil {
		t.Fatalf("normal calc failed: %v", normal["error"])
	}
	r78, code := amzCall(t, sprintf78(body, `,"rule78":true`))
	if code != 200 || r78["error"] != nil {
		t.Fatalf("rule78 calc failed: %v", r78["error"])
	}

	nSched := normal["schedule"].([]any)
	rSched := r78["schedule"].([]any)
	if len(rSched) == 0 || len(nSched) == 0 {
		t.Fatal("empty schedule")
	}
	nRow0 := nSched[0].(map[string]any)
	rRow0 := rSched[0].(map[string]any)
	// Rule-of-78 front-loads: first period interest must exceed the
	// actuarial first-period interest.
	if rRow0["interest"].(float64) <= nRow0["interest"].(float64) {
		t.Errorf("R78 first-period interest %.2f should exceed actuarial %.2f",
			rRow0["interest"], nRow0["interest"])
	}

	// The R78 per-period interest declines by a constant step.
	r1 := rSched[0].(map[string]any)["interest"].(float64)
	r2 := rSched[1].(map[string]any)["interest"].(float64)
	r3 := rSched[2].(map[string]any)["interest"].(float64)
	step1 := r1 - r2
	step2 := r2 - r3
	if math.Abs(step1-step2) > 0.02 {
		t.Errorf("R78 interest should decline by a constant step; got %.4f then %.4f",
			step1, step2)
	}
}

func sprintf78(tmpl, ins string) string {
	// tiny local formatter to avoid importing fmt just for one verb
	out := ""
	for i := 0; i < len(tmpl); i++ {
		if i+1 < len(tmpl) && tmpl[i] == '%' && tmpl[i+1] == 's' {
			out += ins
			i++
			continue
		}
		out += string(tmpl[i])
	}
	return out
}
