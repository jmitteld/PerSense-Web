// Tests for dispatch_gaps QW1: the PV API echoes the solved Rate
// (PV-8) and As-of Date (PV-9) back in the response so the UI can
// display the backward-solved value. Before QW1, PVResponse carried
// only SumValue and the per-row values — the solved rate / date were
// computed internally and discarded.

package api

import (
	"encoding/json"
	"math"
	"testing"
)

// TestPV8EchoesSolvedRate: forward-discount a lump sum at a known
// rate, then omit the rate and confirm the API solves it AND returns
// it in PVResponse.Rate.
func TestPV8EchoesSolvedRate(t *testing.T) {
	fwd, _ := pvCall(t, `{
		"asOfDate":"2024-01-01",
		"rate":0.085,
		"lumpSums":[{"date":"2031-01-01","amount":25000}]
	}`)
	if fwd.Error != "" {
		t.Fatalf("forward error: %s", fwd.Error)
	}

	body, _ := json.Marshal(map[string]any{
		"asOfDate": "2024-01-01",
		"sumValue": fwd.SumValue,
		"lumpSums": []map[string]any{{"date": "2031-01-01", "amount": 25000}},
	})
	bwd, code := pvCall(t, string(body))
	if code != 200 || bwd.Error != "" {
		t.Fatalf("PV-8 backward failed: code=%d err=%q", code, bwd.Error)
	}
	if math.Abs(bwd.Rate-0.085) > 1e-4 {
		t.Errorf("PVResponse.Rate = %.6f, want 0.085 (solved rate not echoed)", bwd.Rate)
	}
}

// TestPV9EchoesSolvedAsOfDate: forward-discount at a known as-of
// date, then omit the date and confirm the API solves it AND returns
// it in PVResponse.AsOfDate.
func TestPV9EchoesSolvedAsOfDate(t *testing.T) {
	fwd, _ := pvCall(t, `{
		"asOfDate":"2024-01-01",
		"rate":0.06,
		"lumpSums":[{"date":"2030-01-01","amount":50000}]
	}`)
	if fwd.Error != "" {
		t.Fatalf("forward error: %s", fwd.Error)
	}

	body, _ := json.Marshal(map[string]any{
		"rate":     0.06,
		"sumValue": fwd.SumValue,
		"lumpSums": []map[string]any{{"date": "2030-01-01", "amount": 50000}},
	})
	bwd, code := pvCall(t, string(body))
	if code != 200 || bwd.Error != "" {
		t.Fatalf("PV-9 backward failed: code=%d err=%q", code, bwd.Error)
	}
	if bwd.AsOfDate != "2024-01-01" {
		t.Errorf("PVResponse.AsOfDate = %q, want 2024-01-01 (solved date not echoed)", bwd.AsOfDate)
	}
}

// TestForwardCalcEchoesInputAsOf: a plain forward calc echoes the
// rate and as-of date it was given (dispatch_gaps §0.6.1 C1 fix).
func TestForwardCalcEchoesInputAsOf(t *testing.T) {
	fwd, code := pvCall(t, `{
		"asOfDate":"2024-01-01",
		"rate":0.06,
		"lumpSums":[{"date":"2030-01-01","amount":50000}]
	}`)
	if code != 200 || fwd.Error != "" {
		t.Fatalf("forward failed: code=%d err=%q", code, fwd.Error)
	}
	if fwd.AsOfDate != "2024-01-01" {
		t.Errorf("forward AsOfDate = %q, want 2024-01-01 (forward echo)", fwd.AsOfDate)
	}
	if fwd.Rate < 0.0599 || fwd.Rate > 0.0601 {
		t.Errorf("forward Rate = %v, want 0.06 (forward echo)", fwd.Rate)
	}
}
