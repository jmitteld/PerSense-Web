package api

// Tests for the order-invariance guarantee that lets the mortgage grid's
// column-sort feature (see cmd/persense/static/index.html — onMtgHeaderClick,
// applyMtgSort) reorder rows freely without affecting any row's computed
// result.
//
// The mortgage API is stateless: each /api/mortgage/calc request is a single
// row with no awareness of sibling rows or visual position. So sorting on
// the frontend cannot, by construction, change what an individual row's
// computation returns — provided the frontend keeps each row's input cells
// glued to that row when it reorders them.
//
// These tests pin the API half of that guarantee: identical request bodies
// produce identical responses regardless of when (or in what order) they
// are sent. The frontend-side guarantee — that sort correctly moves each
// row's input/output state along with its DOM — would require a browser
// integration test; that's intentionally out of scope here.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

// mortgageRow is a single grid row's input body, paired with a stable
// label used to identify it across reorderings.
type mortgageRow struct {
	label string
	body  string
	// Numeric values used for the test's sort comparators below. Stored
	// here (rather than parsed out of body) to keep the test
	// self-contained and avoid coupling test sort behavior to whatever
	// helpers the frontend uses.
	price float64
	rate  float64
}

// calcMortgageRow posts one row to HandleMortgageCalc and returns the
// decoded response. Any non-200 status or API-level error fails the test
// immediately — these helper-driven tests assume the inputs are valid.
func calcMortgageRow(t *testing.T, body string) MortgageResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/mortgage/calc", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	HandleMortgageCalc(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp MortgageResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected API error: %s", resp.Error)
	}
	return resp
}

// sampleMortgageRows returns a heterogeneous batch of mortgage rows that
// exercises forward calc, balloon-amount solve, and APR-with-points. The
// rows are deliberately ordered so that no two sort orders coincide.
func sampleMortgageRows() []mortgageRow {
	return []mortgageRow{
		// Mortgage Help Example 1: $200k house, 20yr, 8%, 2 pts, 20% down, $200 tax.
		{
			label: "ex1-200k-20yr-8pct-2pts",
			body:  `{"price":200000,"points":0.02,"pctDown":0.20,"years":20,"rate":0.08,"tax":200}`,
			price: 200000, rate: 0.08,
		},
		// Mortgage Help Example 4 step 2: 240k, 15yr, 8.1%, monthly hardened
		// at 1777.79, balloon at year 15 — the case that prompted this work.
		{
			label: "ex4-240k-15yr-8.1pct-balloon15",
			body:  `{"price":240000,"pctDown":0,"years":15,"rate":0.081,"monthly":1777.79,"balloonYears":15}`,
			price: 240000, rate: 0.081,
		},
		// Plain 30-year at 6%.
		{
			label: "plain-300k-30yr-6pct",
			body:  `{"price":300000,"pctDown":0.20,"years":30,"rate":0.06,"tax":150}`,
			price: 300000, rate: 0.06,
		},
		// Smaller loan, shorter term, higher rate.
		{
			label: "small-150k-15yr-7pct",
			body:  `{"price":150000,"pctDown":0.10,"years":15,"rate":0.07,"tax":100}`,
			price: 150000, rate: 0.07,
		},
		// MS_EX2 shape: solve for Price from Cash + Monthly (a backward
		// calc). Including this means the test batch covers both forward
		// and backward paths — a sort that scrambled the input/output
		// flag state of a cell would surface here.
		{
			label: "ex2-solve-price-from-cash+monthly",
			body:  `{"points":0.015,"cash":56000,"years":30,"rate":0.085,"tax":200,"monthly":1650}`,
			price: 0, rate: 0.085, // price unset on input — placed at the front under price-asc
		},
	}
}

// responsesEquivalent compares two MortgageResponse values on the fields
// the engine populates. JSON round-trip is used so that fields with the
// "omitempty" tag that legitimately differ between requests (e.g. balloon
// fields only present when balloon was supplied) don't cause a false
// positive — both sides go through the same serializer.
func responsesEquivalent(t *testing.T, label string, want, got MortgageResponse) {
	t.Helper()
	jw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	jg, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	if string(jw) != string(jg) {
		t.Errorf("row %q: response differs across orderings\n  want=%s\n  got =%s",
			label, jw, jg)
	}
}

// TestMortgageCalcResultIsOrderInvariant submits the same heterogeneous
// batch of rows in several distinct orders and asserts that each row's
// response is byte-identical to its response under the original order.
// This is the central guarantee that lets the frontend's column-sort
// feature reorder rows without changing what any individual row computes.
func TestMortgageCalcResultIsOrderInvariant(t *testing.T) {
	rows := sampleMortgageRows()

	// Snapshot: compute every row in its original (declaration) order.
	baseline := make(map[string]MortgageResponse, len(rows))
	for _, r := range rows {
		baseline[r.label] = calcMortgageRow(t, r.body)
	}

	// Each named ordering represents an arbitrary post-sort arrangement.
	// We don't try to match a real column sort here — that's covered by
	// TestMortgageCalcSortedByColumnPreservesResults below. The point of
	// these permutations is to exercise enough distinct orderings that
	// any state-carry-over bug between requests would surface.
	orderings := map[string][]int{
		"reverse":           {4, 3, 2, 1, 0},
		"balloon-first":     {1, 0, 4, 2, 3},
		"backward-calc-mid": {0, 2, 4, 3, 1},
		"alternating":       {0, 4, 1, 3, 2},
	}

	// Iterate orderings in a deterministic sequence so a regression's
	// failure log points at the same offender every run.
	names := make([]string, 0, len(orderings))
	for name := range orderings {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		perm := orderings[name]
		t.Run(name, func(t *testing.T) {
			for _, idx := range perm {
				r := rows[idx]
				got := calcMortgageRow(t, r.body)
				responsesEquivalent(t, r.label, baseline[r.label], got)
			}
		})
	}
}

// TestMortgageCalcSortedByColumnPreservesResults exercises the specific
// "sort then calculate" user flow: a batch of rows is reordered using
// the same comparator the frontend applies (numeric ascending on a
// chosen column, with stable tiebreak), and each row is then re-posted
// in its new position. Each row's response must still match its
// pre-sort response.
//
// This is functionally a subset of the invariance test above, but
// framed to mirror the user-visible flow: build rows → click a header
// → re-run Calculate. Failures here read more naturally as "sort by
// <column> broke row <label>".
func TestMortgageCalcSortedByColumnPreservesResults(t *testing.T) {
	type sortKey struct {
		name string
		less func(a, b mortgageRow) bool
	}
	sortKeys := []sortKey{
		{
			name: "price-asc",
			less: func(a, b mortgageRow) bool { return a.price < b.price },
		},
		{
			name: "price-desc",
			less: func(a, b mortgageRow) bool { return a.price > b.price },
		},
		{
			name: "rate-asc",
			less: func(a, b mortgageRow) bool { return a.rate < b.rate },
		},
		{
			name: "rate-desc",
			less: func(a, b mortgageRow) bool { return a.rate > b.rate },
		},
	}

	for _, sk := range sortKeys {
		t.Run(sk.name, func(t *testing.T) {
			// Compute baseline responses in the original (unsorted) order.
			rows := sampleMortgageRows()
			baseline := make(map[string]MortgageResponse, len(rows))
			for _, r := range rows {
				baseline[r.label] = calcMortgageRow(t, r.body)
			}

			// Sort the rows and re-compute each in its new position.
			sorted := make([]mortgageRow, len(rows))
			copy(sorted, rows)
			sort.SliceStable(sorted, func(i, j int) bool {
				return sk.less(sorted[i], sorted[j])
			})

			for newPos, r := range sorted {
				got := calcMortgageRow(t, r.body)
				want := baseline[r.label]
				if got.Monthly != want.Monthly ||
					got.Financed != want.Financed ||
					got.BalloonAmount != want.BalloonAmount ||
					got.APR != want.APR ||
					got.Cash != want.Cash {
					t.Errorf("after sort %q, row %q at new position %d: result differs\n  want=%+v\n  got =%+v",
						sk.name, r.label, newPos, want, got)
				}
			}
		})
	}
}
