// Canary tests for dispatch_gaps.md §1.3 items 1 and 2: the Go
// engine implements mortgage.CompareAPRs and mortgage.GenerateRows
// (with passing unit tests) but neither has an HTTP route. The
// frontend therefore reimplements them client-side, missing features
// (crossover-time, N-D what-if) that the engine already provides.
//
// EXPECTED TO FAIL TODAY (404). When Phase 2 of dispatch_gaps.md
// wires the routes in cmd/persense/main.go and adds the
// HandleMortgageCompare / HandleMortgageWhatIf handlers, these
// canaries flip to green.
//
// Implementation note: this test builds a ServeMux mirroring the
// production wiring in cmd/persense/main.go. When the new routes
// land, ADD THEM HERE TOO; the canary catches both "handler
// missing" and "route not wired" states.
//
// See docs/test_plan.md §1 (Wave 1 canaries) C-8, C-9.

package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// productionMux returns a ServeMux mirroring cmd/persense/main.go's
// route table. Update this in lockstep with main.go.
func productionMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/mortgage/calc", HandleMortgageCalc)
	mux.HandleFunc("/api/mortgage/compare", HandleMortgageCompare)
	mux.HandleFunc("/api/mortgage/whatif", HandleMortgageWhatIf)
	mux.HandleFunc("/api/amortization/calc", HandleAmortizationCalc)
	mux.HandleFunc("/api/presentvalue/calc", HandlePVCalc)
	return mux
}

// TestCanaryC8_MortgageCompareRouteMissing documents
// dispatch_gaps.md §1.3 item 1: mortgage.CompareAPRs at
// internal/finance/mortgage/mortgage.go:475 is fully implemented
// (including the Newton-iterated crossover time) but no HTTP route
// exposes it.
//
// The frontend's compareMtgAPR (cmd/persense/static/index.html:1644)
// runs a local heuristic on stored APR values and never computes
// crossover years.
//
// Pairs with: dispatch_gaps M14. Fix: add a POST
// /api/mortgage/compare route in main.go that calls
// mortgage.CompareAPRs.
func TestCanaryC8_MortgageCompareRouteMissing(t *testing.T) {
	body := `{
		"a": {"price":200000,"pctDown":0.20,"years":30,"rate":0.06,"points":0.0},
		"b": {"price":200000,"pctDown":0.20,"years":30,"rate":0.0575,"points":0.02}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/mortgage/compare",
		bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	productionMux().ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Errorf("CANARY: POST /api/mortgage/compare returned 404 — the engine has " +
			"CompareAPRs (mortgage.go:475) but no HTTP route exposes it " +
			"(dispatch_gaps M14). Add the route in cmd/persense/main.go and " +
			"in productionMux() above.")
		return
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

// TestCanaryC9_MortgageWhatIfRouteMissing documents
// dispatch_gaps.md §1.3 item 2: mortgage.GenerateRows at
// internal/finance/mortgage/rowgen.go:52 is implemented and
// unit-tested but no HTTP route exposes it. The frontend
// (index.html:1699) reimplements what-if by looping calls to
// /api/mortgage/calc — costly and missing the DOS 3-D variation
// path.
//
// Pairs with: dispatch_gaps M15. Fix: add POST /api/mortgage/whatif.
func TestCanaryC9_MortgageWhatIfRouteMissing(t *testing.T) {
	body := `{
		"base":{"price":200000,"pctDown":0.20,"years":30,"rate":0.06,"points":0.0},
		"vary":"rate",
		"increment":0.0025,
		"count":5
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/mortgage/whatif",
		bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	productionMux().ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Errorf("CANARY: POST /api/mortgage/whatif returned 404 — the engine has " +
			"GenerateRows (rowgen.go:52) but no HTTP route exposes it " +
			"(dispatch_gaps M15). Add the route in cmd/persense/main.go and " +
			"in productionMux() above.")
		return
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}
