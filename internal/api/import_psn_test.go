// import_psn_test.go — smoke tests for the /api/import/psn handler.
//
// We don't have hand-rolled .psn binary fixtures, so the tests
// exercise the failure paths (wrong method, garbage body, oversized
// body) and the success header-parse path indirectly via an empty
// file rejection.

package api

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// importLegacy posts a real legacy worksheet file to the handler and returns the
// decoded success response. The three DOS.{MTG,AMZ,PVL} files live in the
// read-only legacy tree at the repo root.
func importLegacy(t *testing.T, name string) PSNImportResponse {
	t.Helper()
	body, err := os.ReadFile("../../legacy/src/win_source/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/import/psn", bytes.NewReader(body))
	w := httptest.NewRecorder()
	HandleImportPSN(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("import %s: status %d, body %s", name, w.Code, w.Body.String())
	}
	var r PSNImportResponse
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("import %s: response not JSON: %v", name, err)
	}
	if r.Error != "" {
		t.Fatalf("import %s returned error: %s", name, r.Error)
	}
	return r
}

func near(t *testing.T, name string, p *float64, want, tol float64) {
	t.Helper()
	if p == nil {
		t.Errorf("%s is nil, want ~%.4f", name, want)
		return
	}
	if math.Abs(*p-want) > tol {
		t.Errorf("%s = %.4f, want ~%.4f", name, *p, want)
	}
}

// TestImportPSN_MortgageSuccess imports the real DOS.MTG worksheet end-to-end
// through the HTTP handler and checks the screen + the user-input fields. The
// payload carries inputs only; computed outputs (Cash, Monthly here) are
// intentionally omitted so the frontend recomputes them.
func TestImportPSN_MortgageSuccess(t *testing.T) {
	r := importLegacy(t, "DOS.MTG")
	if r.Screen != "mortgage" || r.Mortgage == nil {
		t.Fatalf("screen = %q, mortgage payload nil = %v", r.Screen, r.Mortgage == nil)
	}
	if len(r.Mortgage.Lines) != 2 {
		t.Fatalf("got %d mortgage lines, want 2", len(r.Mortgage.Lines))
	}
	l := r.Mortgage.Lines[0]
	near(t, "price", l.Price, 10000, 1e-3)
	near(t, "pctDown", l.PctDown, 0.03, 1e-6)
	near(t, "tax", l.Tax, 6, 1e-3)
	if l.Years == nil || *l.Years != 4 {
		t.Errorf("years = %v, want 4", l.Years)
	}
	if l.Monthly != nil {
		t.Errorf("monthly should be omitted (computed output), got %.2f", *l.Monthly)
	}
}

func TestImportPSN_AmortizationSuccess(t *testing.T) {
	r := importLegacy(t, "DOS.AMZ")
	if r.Screen != "amortization" || r.Amortization == nil {
		t.Fatalf("screen = %q, amortization payload nil = %v", r.Screen, r.Amortization == nil)
	}
	a := r.Amortization
	near(t, "amount", a.Amount, 10000, 1e-3)
	near(t, "rate", a.Rate, 0.09, 1e-6)
	if a.NPeriods != 120 {
		t.Errorf("nPeriods = %d, want 120", a.NPeriods)
	}
	if a.PerYr != 2 {
		t.Errorf("perYr = %d, want 2", a.PerYr)
	}
	if a.LoanDate == "" {
		t.Error("loanDate should be set")
	}
}

func TestImportPSN_PresentValueSuccess(t *testing.T) {
	r := importLegacy(t, "DOS.PVL")
	if r.Screen != "presentvalue" || r.PresentValue == nil {
		t.Fatalf("screen = %q, PV payload nil = %v", r.Screen, r.PresentValue == nil)
	}
	pv := r.PresentValue
	if len(pv.LumpSums) != 1 {
		t.Fatalf("got %d lump sums, want 1", len(pv.LumpSums))
	}
	near(t, "lump amount", pv.LumpSums[0].Amount, 5000, 1e-3)
	if len(pv.Periodics) != 2 {
		t.Errorf("got %d periodics, want 2", len(pv.Periodics))
	}
}

func TestImportPSN_RejectsGet(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/import/psn", nil)
	w := httptest.NewRecorder()
	HandleImportPSN(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestImportPSN_RejectsEmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/import/psn", bytes.NewReader(nil))
	w := httptest.NewRecorder()
	HandleImportPSN(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty body: status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var r PSNImportResponse
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("body not JSON: %v\nbody: %s", err, w.Body.String())
	}
	if r.Error == "" {
		t.Error("expected an error message, got none")
	}
}

func TestImportPSN_RejectsGarbage(t *testing.T) {
	// Garbage bytes — no '%' identifier at the right offset, so the
	// header parser will reject.
	garbage := make([]byte, 64)
	for i := range garbage {
		garbage[i] = 0xAA
	}
	req := httptest.NewRequest(http.MethodPost, "/api/import/psn", bytes.NewReader(garbage))
	w := httptest.NewRecorder()
	HandleImportPSN(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("garbage body: status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var r PSNImportResponse
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	if !strings.Contains(r.Error, "identifier") && !strings.Contains(r.Error, "header") {
		t.Errorf("expected header-parse error, got %q", r.Error)
	}
}

func TestImportPSN_RejectsOversized(t *testing.T) {
	// 300 KB body — well above the 256 KB cap.
	big := make([]byte, 300<<10)
	req := httptest.NewRequest(http.MethodPost, "/api/import/psn", bytes.NewReader(big))
	w := httptest.NewRecorder()
	HandleImportPSN(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("oversized body: status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var r PSNImportResponse
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	if !strings.Contains(r.Error, "too large") {
		t.Errorf("expected too-large error, got %q", r.Error)
	}
}
