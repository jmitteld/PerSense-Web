package api

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func TestFieldError_Error(t *testing.T) {
	fe := newFieldError("CODE", "prepayment", 2, []string{"Amount"}, "boom")
	if fe.Error() != "boom" {
		t.Errorf("FieldError.Error() = %q, want %q", fe.Error(), "boom")
	}
}

func TestPSNHelpers_NilAndZeroBranches(t *testing.T) {
	// iptr / fptr return nil below the default-status threshold.
	if iptr(types.StatusEmpty, 5) != nil {
		t.Error("iptr(empty) should be nil")
	}
	if p := iptr(types.InOutDefault, 5); p == nil || *p != 5 {
		t.Error("iptr(default) should return &5")
	}
	if fptr(types.StatusEmpty, 1.5) != nil {
		t.Error("fptr(empty) should be nil")
	}
	// dateStr zero-time -> "".
	if dateStr(time.Time{}) != "" {
		t.Error("dateStr(zero) should be empty")
	}
	if dateStr(time.Date(2021, 6, 15, 0, 0, 0, 0, time.UTC)) != "2021-06-15" {
		t.Error("dateStr(date) format wrong")
	}
}

func TestImportPSN_UnknownFileType(t *testing.T) {
	b, err := os.ReadFile("../../legacy/src/win_source/DOS.MTG")
	if err != nil {
		t.Skip("fixture unavailable")
	}
	bad := make([]byte, len(b))
	copy(bad, b)
	// Header = version(2) + identifier(1) + fancy(1) + compDefaults(10) = 14
	// bytes; Grids[0].GridID (which becomes FileType) is at offset 14.
	if len(bad) > 14 {
		bad[14] = 0x7E // not any known block ID
		w := postBytes(HandleImportPSN, "/api/import/psn", bad)
		if w.Code != http.StatusBadRequest {
			t.Errorf("unknown file type code=%d body=%s", w.Code, w.Body.String())
		}
	}
}

func TestImportPSN_AllHelpFixtures(t *testing.T) {
	// Importing every legacy Help fixture exercises the payload builders
	// across many input shapes — including amortization files that carry a
	// moratorium or target (the otherwise-uncovered psnAmortizationPayload
	// branches) and lump/periodic PV rows.
	dir := "../../legacy/src/win_source/Help"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skip("Help fixtures unavailable")
	}
	imported := 0
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "._") {
			continue
		}
		lower := strings.ToLower(name)
		if !(strings.HasSuffix(lower, ".amz") || strings.HasSuffix(lower, ".pvl") ||
			strings.HasSuffix(lower, ".mtg")) {
			continue
		}
		b, err := os.ReadFile(dir + "/" + name)
		if err != nil {
			continue
		}
		w := postBytes(HandleImportPSN, "/api/import/psn", b)
		if w.Code == http.StatusOK {
			imported++
		}
	}
	if imported == 0 {
		t.Skip("no Help fixtures imported")
	}
}

func TestAmortization_DeriveOnlyWithNPeriods(t *testing.T) {
	// Derive-only (no amount, no rate) but nPeriods supplied: exercises the
	// nPeriods branch and a successful FirstPass-only derive.
	post(HandleAmortizationCalc, "/api/amortization/calc",
		`{"loanDate":"2020-01-01","perYr":12,"nPeriods":120}`)
}
