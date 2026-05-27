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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
