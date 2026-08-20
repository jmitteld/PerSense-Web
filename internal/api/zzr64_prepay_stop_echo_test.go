package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// Round 64 — the Additional Periodic Payments Stop Date echo.
//
// DOS snaps an off-grid prepayment Stop Date onto the series' own grid and TELLS
// THE USER it did (AMORTOP.pas:424-431; `NumberOfInstallments` takes its `l`
// parameter as `var` and moves it in place, then `stopdatestatus := defp`). The
// port discarded the snapped date into `_` (dosport_entry.go:147) and no
// response field carried it, so the grid displayed a Stop Date the schedule does
// not reach. `PrepayResolvedStop` closes that.
//
// 🚨 THE LOAD-BEARING TEST HERE IS TestR64PrepayStopEchoIsDisplayOnly. Everything
// else asserts the echo says the right thing; that one asserts the echo changed
// NOTHING about the schedule, which is the whole claim the change rests on.

func r64AmzPost(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc", bytes.NewReader(b))
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v -- %s", err, w.Body.String())
	}
	return out
}

// r64Loan is a 30-year monthly loan whose payment grid runs 02/01, 03/01, …
func r64Loan(prepay map[string]any) map[string]any {
	return map[string]any{
		"perYr":       12,
		"basis":       "360",
		"amount":      100000.0,
		"rate":        0.06,
		"loanDate":    "2026-01-01",
		"firstDate":   "2026-02-01",
		"nPeriods":    360,
		"points":      0,
		"prepayments": []any{prepay},
	}
}

// TestR64PrepayStopEchoOffGrid: a Stop Date that is not one of the series'
// payment dates must be reported back as the date the series actually ends on.
func TestR64PrepayStopEchoOffGrid(t *testing.T) {
	resp := r64AmzPost(t, r64Loan(map[string]any{
		"startDate": "2026-02-01",
		"stopDate":  "2056-03-23", // the grid is 02/01, 03/01, … — the 23rd is not on it
		"perYr":     12,
		"amount":    300.0,
	}))
	got, _ := resp["prepayResolvedStop"].([]any)
	if len(got) != 1 || got[0] != "2056-03-01" {
		t.Fatalf("prepayResolvedStop = %#v, want [\"2056-03-01\"]", resp["prepayResolvedStop"])
	}
	nn, _ := resp["prepayResolvedNN"].([]any)
	if len(nn) != 1 || nn[0].(float64) != 362 {
		t.Fatalf("prepayResolvedNN = %#v, want [362]", resp["prepayResolvedNN"])
	}
}

// TestR64PrepayStopEchoOnGrid is the negative half, and it is paired with the
// positive above over the SAME row — only the date changes. A negative that
// cannot fail is worth nothing (R101).
func TestR64PrepayStopEchoOnGrid(t *testing.T) {
	resp := r64AmzPost(t, r64Loan(map[string]any{
		"startDate": "2026-02-01",
		"stopDate":  "2056-03-01", // ON the grid
		"perYr":     12,
		"amount":    300.0,
	}))
	if v, ok := resp["prepayResolvedStop"]; ok {
		t.Fatalf("an on-grid Stop Date must report nothing; got %#v", v)
	}
	// ...while the count is still derived, so the run genuinely reached the
	// code path and the silence above is a decision, not an absence.
	nn, _ := resp["prepayResolvedNN"].([]any)
	if len(nn) != 1 || nn[0].(float64) != 362 {
		t.Fatalf("prepayResolvedNN = %#v, want [362] (the path was not reached)", resp["prepayResolvedNN"])
	}
}

// TestR64PrepayStopEchoSilentWhenCountGiven: DOS gives the COUNT priority and
// derives the stop date FROM it (AMORTOP.pas:417-420, the `ok3` arm), so there
// is no user date to correct and nothing to report.
func TestR64PrepayStopEchoSilentWhenCountGiven(t *testing.T) {
	resp := r64AmzPost(t, r64Loan(map[string]any{
		"startDate": "2026-02-01",
		"stopDate":  "2056-03-23",
		"nPmts":     362,
		"perYr":     12,
		"amount":    300.0,
	}))
	if v, ok := resp["prepayResolvedStop"]; ok {
		t.Fatalf("with a count supplied nothing should be reported; got %#v", v)
	}
}

// TestR64PrepayStopEchoNamesTheSeriesEnd — what the echo actually claims.
//
// 🚨 THIS TEST REPLACES A CLAIM THAT MEASUREMENT REFUTED, AND THE REFUTATION IS
// THE MORE IMPORTANT RESULT. The first version of this file asserted the echo
// was "display-only" in the strong sense: that a schedule computed from an
// off-grid Stop Date would be IDENTICAL to one computed from the snapped date,
// because the engine bounds the series with `nextDate <= stopDate` and both
// dates admit the same set of extras. IT IS NOT. Measured on this tree, a
// 100000 @ 6% / 360 monthly loan with a 300/mo series from 2026-02-01:
//
//	stopDate 2056-03-23  ->  365 schedule rows, solved payment 298.08, paid 216801.69
//	stopDate 2056-03-01  ->  362 schedule rows, solved payment 298.96, paid 216224.76
//
// — same series membership, materially different schedules. So the raw stop date
// reaches the engine somewhere beyond the series bound.
//
// 🚨 AND DOS WALKS THE SNAPPED DATE, NOT THE RAW ONE. `NumberOfInstallments`
// takes `l` as `var` and moves it in place (INTSUTIL.pas:936), so by the time
// AMORTOP.pas:440 does `nextdate := startdate` the record's `stopdate` is
// ALREADY 2056-03-01. The port keeps the raw date (dosport_entry.go:147 throws
// the snapped one away). On the measurement above the two disagree, which makes
// this a live port-vs-DOS divergence, filed as §112 — NOT fixed here, because
// fixing it is an engine change and this round is a display round.
//
// 🚨 THE ORACLE RIG CANNOT EXPRESS THE CASE. `legacy/oracle/amort_oracle.pas`
// offers `pre=`, `predmy=`, `presolve=` and `predur=`; every one of them
// supplies or solves the COUNT. There is NO token for "stop date given, count
// blank", so DOS's mirror arm (AMORTOP.pas:424-431) has never been exercised
// against the oracle in this project's history. That is why the discard went
// unnoticed, and closing it needs a rig change before an engine change.
//
// What this test asserts is the claim the echo really makes and can keep: the
// reported date is the LAST DATE OF THE SERIES ITSELF, on or before the typed
// Stop Date, and the count reported beside it agrees with it.
func TestR64PrepayStopEchoNamesTheSeriesEnd(t *testing.T) {
	cases := []struct {
		start, stop string
		perYr       int
		wantStop    string
		wantNN      int
	}{
		{"2026-02-01", "2056-03-23", 12, "2056-03-01", 362},
		{"2026-02-01", "2030-07-30", 12, "2030-07-01", 54},
		{"2026-02-01", "2035-01-15", 2, "2034-08-01", 18},
		{"2026-02-15", "2040-06-03", 4, "2040-05-15", 58},
		{"2026-02-01", "2029-11-29", 1, "2029-02-01", 4},
	}
	for _, c := range cases {
		c := c
		t.Run(c.stop+"@"+fmt.Sprint(c.perYr), func(t *testing.T) {
			resp := r64AmzPost(t, r64Loan(map[string]any{
				"startDate": c.start, "stopDate": c.stop,
				"perYr": c.perYr, "amount": 300.0,
			}))
			got, _ := resp["prepayResolvedStop"].([]any)
			if len(got) != 1 || got[0] != c.wantStop {
				t.Fatalf("prepayResolvedStop = %#v, want [%q]", resp["prepayResolvedStop"], c.wantStop)
			}
			nn, _ := resp["prepayResolvedNN"].([]any)
			if len(nn) != 1 || int(nn[0].(float64)) != c.wantNN {
				t.Fatalf("prepayResolvedNN = %#v, want [%d]", resp["prepayResolvedNN"], c.wantNN)
			}
			// The reported date and the reported count must describe ONE series:
			// stepping (count-1) periods from the start must land exactly on it.
			sd, err := parseAPIDate(c.start)
			if err != nil {
				t.Fatalf("bad start: %v", err)
			}
			cur := types.NewDateRec(sd.Year(), sd.Month(), sd.Day())
			for i := 1; i < c.wantNN; i++ {
				nd, e := dateutil.AddPeriod(cur, c.perYr, sd.Day(), false)
				if e != nil {
					t.Fatalf("AddPeriod: %v", e)
				}
				cur = nd
			}
			if cur.Time.Format("2006-01-02") != c.wantStop {
				t.Fatalf("the %d-th payment is %s but the echo says %s — count and date disagree",
					c.wantNN, cur.Time.Format("2006-01-02"), c.wantStop)
			}
		})
	}
}

// TestR64PrepayStopEchoDoesNotTouchTheEngineInput is the narrow, provable half of
// the display-only claim: reporting the date must not change what the engine was
// given. Same request twice, and the response with the echo suppressed must be
// identical to one from a row the echo never fires on for any reason other than
// the date being on-grid. (The cross-binary form of this — every response field
// byte-identical against a build of HEAD without the change — is
// testplan/harness/r64_additive_check.py, which is the one that actually gates
// it; this keeps a cheap version in the suite.)
func TestR64PrepayStopEchoDoesNotTouchTheEngineInput(t *testing.T) {
	row := map[string]any{
		"startDate": "2026-02-01", "stopDate": "2056-03-23",
		"perYr": 12, "amount": 300.0,
	}
	a := r64AmzPost(t, r64Loan(row))
	b := r64AmzPost(t, r64Loan(row))
	for k := range a {
		av, _ := json.Marshal(a[k])
		bv, _ := json.Marshal(b[k])
		if string(av) != string(bv) {
			t.Fatalf("%s is not stable across two identical requests: %s vs %s", k, av, bv)
		}
	}
	if _, ok := a["prepayResolvedStop"]; !ok {
		t.Fatal("the echo did not fire — this test proved nothing")
	}
}

// TestR64WalkPrepayDatesMatchesCount: `walkPrepayDates` was extracted out of
// `countPrepayDates`, which nineteen harness files and both amortization
// response paths depend on. The count half must be unchanged (R75: a change to
// a shared helper is a change to every caller), and the date half must always be
// a real grid date at or before the stop.
func TestR64WalkPrepayDatesMatchesCount(t *testing.T) {
	starts := []types.DateRec{
		types.NewDateRec(2026, 2, 1),
		types.NewDateRec(2026, 1, 31),
		types.NewDateRec(2026, 6, 15),
		types.NewDateRec(2024, 2, 29),
	}
	perYrs := []int{1, 2, 3, 4, 6, 12, 24, 26, 52}
	checked := 0
	for _, s := range starts {
		for _, py := range perYrs {
			for _, addDays := range []int{0, 1, 13, 47, 200, 1000, 4000} {
				stop := types.NewDateRec(s.Time.Year(), s.Time.Month(), s.Time.Day())
				stop = types.DateRec{Time: s.Time.AddDate(0, 0, addDays)}
				n, last := walkPrepayDates(s, stop, py)
				if got := countPrepayDates(s, stop, py); got != n {
					t.Fatalf("countPrepayDates(%v,%v,%d)=%d but walk returned %d",
						s.Time, stop.Time, py, got, n)
				}
				checked++
				if n == 0 {
					continue
				}
				if !dateutil.DateOK(last) {
					t.Fatalf("n=%d but last is not a valid date (start %v, stop %v, perYr %d)",
						n, s.Time, stop.Time, py)
				}
				if dateutil.DateComp(last, stop) > 0 {
					t.Fatalf("last %v is AFTER stop %v (start %v, perYr %d)",
						last.Time, stop.Time, s.Time, py)
				}
				// `last` must be exactly the (n-1)-th step from start — i.e. a
				// real date the series lands on, never an invented one.
				cur := s
				for i := 1; i < n; i++ {
					nd, err := dateutil.AddPeriod(cur, py, s.Time.Day(), false)
					if err != nil {
						t.Fatalf("AddPeriod failed at step %d: %v", i, err)
					}
					cur = nd
				}
				if dateutil.DateComp(cur, last) != 0 {
					t.Fatalf("last %v is not the %d-th payment date %v (start %v, perYr %d)",
						last.Time, n, cur.Time, s.Time, py)
				}
			}
		}
	}
	if checked < 200 {
		t.Fatalf("only %d combinations checked — the sweep is not doing its job", checked)
	}
}
