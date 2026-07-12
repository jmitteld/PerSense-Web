package api

import (
	"fmt"
	"math"
	"testing"
)

// Regression guard for the "365/360" basis kicker on Present Value discounting.
//
// ROOT CAUSE (2026-07-12 audit, discrepancies.md §27): on the 365/360 basis DOS
// discounts with an internal rate scaled in YIELD space by kicker = 365/360
// (PEDATA.pas:141; INTSUTIL.pas PercentValueFromCell tratecol/x365_360), while
// YearsDif stays Julian/360. The Go port applied neither — it discounted at the
// raw displayed true rate over Julian/360 — leaving the 365/360 PV ~1.5% low on
// discount magnitude (both lumps and periodics). pvKickerRate now applies the
// transform at the handler boundary.
//
// PROVENANCE (real DOS engine, Present Value screen, basis 365/360, Exact=YES):
// as-of 04/25/2006, Rate Type Loan 5.0000% (→ true rate 4.9896%), lump
// $5,000,000 on 10/01/2027, periodic $3,500/mo from 05/15/2030 through
// 12/15/2060 (per/yr 12, COLA 0).
//
//	DOS: lump 1,664,120  periodic 189,127.30  total 1,853,247.7
//
// (Before the fix the port returned lump 1,689,336.53 / periodic 193,898.27.)
// The 365 basis (which never used the kicker) must be unaffected.
func TestPVBasis365360KickerMatchesDOS(t *testing.T) {
	trueRate := 12 * math.Log1p(0.05/12) // Loan 5% → continuous true rate

	post := func(basis string) PVResponse {
		body := fmt.Sprintf(`{
			"asOfDate":"2006-04-25","rate":%v,"basis":"%s","exact":true,
			"lumpSums":[{"date":"2027-10-01","amount":5000000}],
			"periodics":[{"fromDate":"2030-05-15","toDate":"2060-12-15","perYr":12,"amount":3500,"cola":0}]
		}`, trueRate, basis)
		return postPV(t, body)
	}

	// 365/360: matches DOS to the cent (lump printed at whole dollars in DOS).
	r := post("365/360")
	if r.Error != "" {
		t.Fatalf("365/360 error: %s", r.Error)
	}
	if !approxEqual(r.LumpSums[0].Value, 1664120.45, 0.5) {
		t.Errorf("365/360 lump = %.2f, want ~1664120 (DOS)", r.LumpSums[0].Value)
	}
	if !approxEqual(r.Periodics[0].Value, 189127.30, 0.01) {
		t.Errorf("365/360 periodic = %.2f, want 189127.30 (DOS)", r.Periodics[0].Value)
	}
	// The displayed rate must round-trip (forward run undoes the kicker on echo).
	if !approxEqual(r.Rate, trueRate, 1e-9) {
		t.Errorf("365/360 rate echo = %.10f, want the input %.10f (kicker must not leak out)", r.Rate, trueRate)
	}

	// 365 basis is unaffected by the kicker.
	r365 := post("365")
	if !approxEqual(r365.LumpSums[0].Value, 1715891.57, 0.01) {
		t.Errorf("365 lump = %.2f, want 1715891.57 (unchanged)", r365.LumpSums[0].Value)
	}
	if !approxEqual(r365.Periodics[0].Value, 198980.58, 0.01) {
		t.Errorf("365 periodic = %.2f, want 198980.58 (unchanged)", r365.Periodics[0].Value)
	}
}
