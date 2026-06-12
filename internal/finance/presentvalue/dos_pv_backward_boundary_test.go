package presentvalue

import (
	"math"
	"os"
	"testing"
)

// PV backward-solver BOUNDARY differential vs the real DOS engine. The existing
// TestDOSPV*BackwardDirectSweep tests cover the seven solvers over moderate
// random inputs (rates 1-14%, moderate terms). This pins the EXTREMES the random
// sweeps don't reach — near-zero and very high rates, very long terms, biweekly
// frequency, and tiny/huge target magnitudes — where a solver is most likely to
// diverge (Newton damping, closed-form cancellation, date overflow).
//
// Each case forward-computes the target with the bit-identical forward oracle,
// then solves it back with BOTH engines and requires agreement. Reuses the
// helpers in dos_pv_oracle_test.go.

func TestDOSPVBackwardBoundarySweep(t *testing.T) {
	if _, err := os.Stat(pvOracleBin()); err != nil {
		t.Skipf("PV oracle not present (%s); build via TARGET=pv_oracle legacy/oracle/build_linux.sh", pvOracleBin())
	}

	rates := []float64{0.0001, 0.001, 0.005, 0.01, 0.25, 0.40, 0.60}
	lumpMonths := []int{1, 6, 60, 240, 600} // up to 50 years
	amounts := []float64{1, 100, 50000, 5_000_000}
	perYrs := []int{1, 2, 4, 12, 26, 52}

	relFail := func(t *testing.T, name string, dos, got float64, fails *int) {
		rel := math.Abs(dos-got) / math.Max(1, math.Abs(got))
		if rel > 1e-6 {
			*fails++
			if *fails <= 10 {
				t.Errorf("%s: DOS=%.8g Go=%.8g (rel %.2e)", name, dos, got, rel)
			}
		}
	}
	dayFail := func(t *testing.T, name string, dos, got interface{ Format(string) string }, dd float64, fails *int) {
		if dd > 1.5 {
			*fails++
			if *fails <= 8 {
				t.Errorf("%s: DOS=%s Go=%s (%.0f days)", name, dos.Format("2006-01-02"), got.Format("2006-01-02"), dd)
			}
		}
	}

	// --- PV-1 lump AMOUNT (extreme rate × term × magnitude) ---
	la, laF := 0, 0
	for _, r := range rates {
		for _, m := range lumpMonths {
			for _, a := range amounts {
				sv, ok := runPVLumpOracle(a, r, m)
				if !ok || sv <= 0 {
					continue
				}
				dos, ok1 := runBkLumpAmtOracle(sv, r, m)
				gv, ok2 := goBkLumpAmount(sv, r, m)
				if !ok1 || !ok2 {
					continue
				}
				la++
				relFail(t, "PV1 lump-amt", dos, gv, &laF)
			}
		}
	}
	t.Logf("PV-1 lump amount boundary: checked %d, fails %d", la, laF)

	// --- PV-2 lump DATE (recover the payment date over extremes) ---
	ld, ldF := 0, 0
	for _, r := range rates {
		for _, m := range lumpMonths {
			const a = 12345.0
			sv, ok := runPVLumpOracle(a, r, m)
			if !ok || sv <= 0 {
				continue
			}
			dos, ok1 := runBkLumpDateOracle(sv, a, r, 12) // fixed wrong seed
			gv, ok2 := goBkLumpDate(sv, a, r, 12)
			if !ok1 || !ok2 {
				continue
			}
			ld++
			dd := math.Abs(dos.Time.Sub(gv.Time).Hours() / 24)
			dayFail(t, "PV2 lump-date", dos.Time, gv.Time, dd, &ldF)
		}
	}
	t.Logf("PV-2 lump date boundary: checked %d, fails %d", ld, ldF)

	// --- PV-4 periodic AMOUNT (extreme rate × frequency × long term) ---
	pa, paF := 0, 0
	for _, r := range rates {
		for _, py := range perYrs {
			for _, mult := range []int{1, 50} {
				n := mult * py
				if n < 2 {
					n = 2
				}
				sv, ok := runPVPeriodicOracle(500, r, py, n, 0, false)
				if !ok || sv <= 0 {
					continue
				}
				dos, ok1 := runBkPerAmtOracle(sv, r, py, n)
				gv, ok2 := goBkPeriodicAmount(sv, r, py, n)
				if !ok1 || !ok2 {
					continue
				}
				pa++
				relFail(t, "PV4 per-amt", dos, gv, &paF)
			}
		}
	}
	t.Logf("PV-4 periodic amount boundary: checked %d, fails %d", pa, paF)

	// --- PV-8 RATE solve (recover tiny / high implied rates over long terms) ---
	rs, rsF := 0, 0
	for _, r := range rates {
		for _, m := range lumpMonths {
			const a = 100000.0
			sv, ok := runPVLumpOracle(a, r, m)
			if !ok || sv <= 0 {
				continue
			}
			dos, ok1 := runBkRateOracle(sv, a, m)
			gv, ok2 := goBkRate(sv, a, m)
			if !ok1 || !ok2 {
				continue
			}
			rs++
			if math.Abs(dos-gv) > 1e-7 { // rates are small ⇒ absolute compare
				rsF++
				if rsF <= 8 {
					t.Errorf("PV8 rate (r=%.4f m=%d): DOS=%.10f Go=%.10f", r, m, dos, gv)
				}
			}
		}
	}
	t.Logf("PV-8 rate boundary: checked %d, fails %d", rs, rsF)

	// --- PV-9 AS-OF solve (long horizons, recover the valuation date) ---
	as, asF := 0, 0
	for _, r := range []float64{0.001, 0.01, 0.25, 0.50} {
		for _, m := range []int{12, 120, 360, 600} {
			const a = 100000.0
			sv, ok := runPVLumpOracle(a, r, m)
			if !ok || sv <= 0 {
				continue
			}
			dos, ok1 := runBkAsofOracle(sv, a, r, m)
			gv, ok2 := goBkAsOf(sv, a, r, m)
			if !ok1 || !ok2 {
				continue
			}
			as++
			dd := math.Abs(dos.Time.Sub(gv.Time).Hours() / 24)
			dayFail(t, "PV9 asof", dos.Time, gv.Time, dd, &asF)
		}
	}
	t.Logf("PV-9 as-of boundary: checked %d, fails %d", as, asF)

	// --- PV-5 / PV-6 periodic TO/FROM date (extreme rate × frequency × term) ---
	td, tdF, fd, fdF := 0, 0, 0, 0
	for _, r := range rates {
		for _, py := range perYrs {
			for _, mult := range []int{1, 40} {
				nTrue := mult * py
				if nTrue < 2 {
					nTrue = 2
				}
				sv, ok := runPVPeriodicOracle(750, r, py, nTrue, 0, false)
				if !ok || sv <= 0 {
					continue
				}
				// to-date (seed deliberately wrong; both recover the true date).
				// The TO-date inverse is ill-conditioned at long terms: the late
				// payments are so heavily discounted that PV is nearly flat in the
				// to-date, so many dates satisfy the target to tolerance and DOS/Go
				// may pick different (all valid) ones. Only assert exact agreement
				// where the problem is well-posed — the last payment still retains
				// >~5% discount weight (rate × years < 3). (The FROM-date inverse,
				// which moves the heavily-weighted start, stays well-conditioned and
				// is asserted across the full range below.)
				years := float64(nTrue) / float64(py)
				if r*years <= 3 {
					if dos, ok1 := runBkPerTodateOracle(sv, 750, r, py, 1+nTrue/2); ok1 {
						if gv, ok2 := goBkPeriodicTodate(sv, 750, r, py, 1+nTrue/2); ok2 {
							td++
							dd := math.Abs(dos.Time.Sub(gv.Time).Hours() / 24)
							dayFail(t, "PV5 per-todate", dos.Time, gv.Time, dd, &tdF)
						}
					}
				}
				// from-date
				if dos, ok1 := runBkPerFromdateOracle(sv, 750, r, py, nTrue); ok1 {
					if gv, ok2 := goBkPeriodicFromdate(sv, 750, r, py, nTrue); ok2 {
						fd++
						dd := math.Abs(dos.Time.Sub(gv.Time).Hours() / 24)
						dayFail(t, "PV6 per-fromdate", dos.Time, gv.Time, dd, &fdF)
					}
				}
			}
		}
	}
	t.Logf("PV-5 periodic to-date boundary: checked %d, fails %d | PV-6 from-date: checked %d, fails %d", td, tdF, fd, fdF)
}
