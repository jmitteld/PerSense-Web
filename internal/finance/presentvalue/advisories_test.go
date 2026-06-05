package presentvalue

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func pvAdvCodes(ws []string) []string {
	var out []string
	for _, w := range ws {
		if !strings.HasPrefix(w, "@@ADV|") {
			continue
		}
		body := strings.TrimPrefix(w, "@@ADV|")
		head := body
		if i := strings.Index(body, "@@"); i >= 0 {
			head = body[:i]
		}
		if parts := strings.Split(head, "|"); len(parts) >= 2 {
			out = append(out, parts[1])
		}
	}
	return out
}

func pvHasCode(ws []string, code string) bool {
	for _, c := range pvAdvCodes(ws) {
		if c == code {
			return true
		}
	}
	return false
}

// A healthy forward present value must produce no advisories.
func TestPVAdvisoryNoneOnHealthy(t *testing.T) {
	in := PVInput{
		Settings: defaultSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: newDate(2024, time.January, 1),
			R: RateEntry{Status: types.InOutInput, Rate: 0.08},
		},
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: newDate(2025, time.January, 1),
			AmtStatus: types.InOutInput, Amt: 10000,
		}},
	}
	r := Calculate(in)
	if r.Err != nil {
		t.Fatalf("calc: %v", r.Err)
	}
	if c := pvAdvCodes(r.Warnings); len(c) != 0 {
		t.Errorf("expected no advisories on a healthy forward PV; got %v", c)
	}
}

// P-W4: when the target value equals the PV of the first lump alone, the
// second lump's solved amount comes out ~0.
func TestPVAdvisoryPW4_ZeroSolvedAmount(t *testing.T) {
	asOf := newDate(2024, time.January, 1)
	rate := 0.08
	// Forward-value a single lump to learn its PV, then use that as the
	// screen target with a second, amount-blank lump added.
	fwd := Calculate(PVInput{
		Settings: defaultSettings(),
		PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf, R: RateEntry{Status: types.InOutInput, Rate: rate}},
		LumpSums: []LumpSumPayment{{DateStatus: types.InOutInput, Date: newDate(2025, time.January, 1), AmtStatus: types.InOutInput, Amt: 10000}},
	})
	if fwd.Err != nil {
		t.Fatalf("forward: %v", fwd.Err)
	}

	in := PVInput{
		Settings: defaultSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R:              RateEntry{Status: types.InOutInput, Rate: rate},
			SumValueStatus: types.InOutInput, SumValue: fwd.SumValue,
		},
		LumpSums: []LumpSumPayment{
			{DateStatus: types.InOutInput, Date: newDate(2025, time.January, 1), AmtStatus: types.InOutInput, Amt: 10000},
			// Amount blank -> solved; should come out ~0.
			{DateStatus: types.InOutInput, Date: newDate(2026, time.January, 1)},
		},
	}
	r := Calculate(in)
	if r.Err != nil {
		t.Fatalf("calc: %v", r.Err)
	}
	if !pvHasCode(r.Warnings, "P-W4") {
		t.Errorf("expected P-W4 (solved amount ~0); solved amt2=%.4f codes=%v",
			r.LumpSums[1].Amt, pvAdvCodes(r.Warnings))
	}
}
