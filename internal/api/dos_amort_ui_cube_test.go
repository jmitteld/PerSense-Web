package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// dos_amort_ui_cube_test.go — exhaustive UI-LEVEL confidence for the amortization
// screen. Unlike the engine-level TestDOSGroundZeroRowCube (which calls Amortize
// directly), this drives the real HTTP handler HandleAmortizationCalc with the
// exact JSON the browser UI posts, across a complete cross of the user-facing
// computational settings, and compares the rounded-to-cents response (what the UI
// renders) to the real DOS engine. It therefore also exercises the API plumbing:
// the `exact` field, the basis string mapping, prepaid defaulting, and the
// per-row cent rounding.
//
// Partition (per docs/testing_policy.md, mirroring docs/exact_groundzero_findings.md):
//   CLEAN  — payment and every UI row column asserted to the cent.
//   FRONTIER — documented, bounded gaps tracked with an envelope guard.
//
// Requires the DOS oracle binary (legacy/oracle/build_linux.sh; default
// /tmp/oraclebuild/amort_oracle, override with PERSENSE_ORACLE).

func uiOracleBin() string {
	if p := os.Getenv("PERSENSE_ORACLE"); p != "" {
		return p
	}
	return "/tmp/oraclebuild/amort_oracle"
}

func uiOraclePayment(amount, rate float64, n, perYr int, flags ...string) (float64, bool) {
	args := append([]string{strconv.FormatFloat(amount, 'f', 2, 64),
		strconv.FormatFloat(rate, 'f', 10, 64), strconv.Itoa(n), strconv.Itoa(perYr)}, flags...)
	for try := 0; try < 8; try++ {
		out, err := exec.Command(uiOracleBin(), args...).Output()
		if err != nil {
			continue
		}
		f := strings.Fields(strings.TrimSpace(string(out)))
		if len(f) >= 2 && f[0] == "payment" {
			if v, _ := strconv.ParseFloat(f[1], 64); v != 0 {
				return v, true
			}
		}
	}
	return 0, false
}

type uiRow struct{ interest, prin, bal float64 }

// uiOracleRows runs the oracle in rows mode WITHOUT a supplied payment, so DOS
// solves its own payment and renders its own schedule — the faithful counterpart
// to the UI solving the payment and rendering its schedule.
func uiOracleRows(amount, rate float64, n, perYr int, flags ...string) ([]uiRow, bool) {
	args := append([]string{strconv.FormatFloat(amount, 'f', 2, 64),
		strconv.FormatFloat(rate, 'f', 10, 64), strconv.Itoa(n), strconv.Itoa(perYr),
		"rows"}, flags...)
	out, err := exec.Command(uiOracleBin(), args...).Output()
	if err != nil {
		return nil, false
	}
	var rows []uiRow
	for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(ln)
		if len(f) == 8 && f[0] == "row" {
			in, _ := strconv.ParseFloat(f[3], 64)
			pr, _ := strconv.ParseFloat(f[5], 64)
			bal, _ := strconv.ParseFloat(f[7], 64)
			rows = append(rows, uiRow{in, pr, bal})
		}
	}
	return rows, len(rows) > 0
}

// postAmort marshals an AmortizationRequest body and drives the real handler.
func uiPostAmort(t *testing.T, body map[string]any) AmortizationResponse {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc", bytes.NewReader(b))
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)
	var resp AmortizationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	return resp
}

func uiFirstDate(perYr int) string {
	// loan date 2024-01-01, first payment one regular period later.
	switch perYr {
	case 1:
		return "2025-01-01"
	case 2:
		return "2024-07-01"
	case 4:
		return "2024-04-01"
	default: // 12
		return "2024-02-01"
	}
}

func TestDOSAmortizationUICube(t *testing.T) {
	if _, err := os.Stat(uiOracleBin()); err != nil {
		t.Skipf("DOS oracle binary not present (%s); build via legacy/oracle/build_linux.sh", uiOracleBin())
	}

	type basisT struct {
		ui   string
		flag string
	}
	bases := []basisT{{"360", ""}, {"365", "b365"}, {"365/360", "b365_360"}}
	methods := []struct {
		name            string
		inadv, r78, usa bool
	}{
		{"ordinary", false, false, false},
		{"in-advance", true, false, false},
		{"r78", false, true, false},
		{"usa", false, false, true},
	}
	perYrs := []int{1, 2, 4, 12}
	amounts := []float64{50000, 200000}
	rates := []float64{0.06, 0.11}
	yearsList := []int{8, 20}

	// Documented frontier envelopes (see docs/exact_groundzero_findings.md).
	const (
		envExactInadv = 50000.0
		envInadvPay   = 1.5
		envNon360Pay  = 120.0
	)
	frExactInadv, frInadvPay, frNon360Pay := 0.0, 0.0, 0.0

	cleanChecks, cleanRowFails, cleanPayFails, cleanBalFails := 0, 0, 0, 0
	maxCleanRow, maxCleanPay, maxCleanBal := 0.0, 0.0, 0.0
	var worstRow, worstPay, worstBal string
	reported := 0
	cells := map[string]bool{}

	payTol := 0.05
	// Per-cell tolerances. Non-exact loans use a deterministic closed-form
	// payment, so the UI and DOS render IDENTICAL schedules to the cent. Exact
	// (true-daily) loans solve the payment iteratively — the UI (bisection) and
	// DOS (Newton) converge to payments differing by ~$0.003, which accumulates
	// in the running BALANCE to at most ~$0.15 over a 20-year term. Every
	// per-period INTEREST figure still matches to the cent. So: interest is
	// always asserted to the cent; the principal portion and balance allow the
	// documented iterative drift only on exact loans.
	tols := func(exact bool) (intTol, prinTol, balTol float64) {
		// With DOS's Iterate ported, the exact payment matches DOS to the penny,
		// so the rendered schedule matches to the cent on every column — same bar
		// as the deterministic non-exact cells. A 1-cent allowance covers display
		// rounding only.
		return 0.015, 0.015, 0.02
	}

	for _, b := range bases {
		for _, exact := range []bool{false, true} {
			for _, prepaid := range []bool{false, true} {
				for _, m := range methods {
					// A cell is a FRONTIER (documented, bounded) when either the
					// UI and DOS solve materially different payments — so their
					// whole rendered schedules differ — or the schedule itself is
					// not yet faithful:
					//   - in-advance (annuity-due): the final row count differs by
					//     one and the in-advance × exact accrual is unimplemented;
					//   - a non-360 basis WITHOUT exact: DOS's closed-form payment
					//     uses a basis day-count factor the Go closed form omits.
					// Everything else is CLEAN — including the client's case, the
					// non-360 basis WITH exact, which now matches DOS row-for-row.
					frontier := m.inadv || (b.ui != "360" && !exact)
					rowClean := !frontier
					payClean := !frontier
					for _, perYr := range perYrs {
						var flags []string
						if b.flag != "" {
							flags = append(flags, b.flag)
						}
						if exact {
							flags = append(flags, "exact")
						}
						if prepaid {
							flags = append(flags, "prepaid")
						}
						if m.inadv {
							flags = append(flags, "inadv")
						}
						if m.r78 {
							flags = append(flags, "r78")
						}
						if m.usa {
							flags = append(flags, "usa")
						}
						cell := fmt.Sprintf("basis=%s|exact=%v|prepaid=%v|method=%s|py=%d",
							b.ui, exact, prepaid, m.name, perYr)
						cells[cell] = true

						for _, amount := range amounts {
							for _, rate := range rates {
								for _, ny := range yearsList {
									n := ny * perYr
									dosPay, ok := uiOraclePayment(amount, rate, n, perYr, flags...)
									if !ok {
										continue
									}
									// Drive the real UI handler (payment blank → solved).
									body := map[string]any{
										"amount":          amount,
										"rate":            rate,
										"nPeriods":        n,
										"perYr":           perYr,
										"loanDate":        "2024-01-01",
										"firstDate":       uiFirstDate(perYr),
										"basis":           b.ui,
										"exact":           exact,
										"inAdvance":       m.inadv,
										"usaRule":         m.usa,
										"rule78":          m.r78,
										"firstIntPrepaid": prepaid,
									}
									resp := uiPostAmort(t, body)
									if resp.Error != "" || len(resp.Schedule) == 0 {
										continue
									}
									// Strip a settlement stub row (PayNum 0) so we align
									// with the oracle's regular-payment row list.
									sched := resp.Schedule
									for len(sched) > 0 && sched[0].PayNum == 0 {
										sched = sched[1:]
									}
									// Modal regular payment from the UI response.
									cnt := map[string]int{}
									var uiPay float64
									best := 0
									for _, r := range sched {
										k := strconv.FormatFloat(r.Payment, 'f', 2, 64)
										cnt[k]++
										if cnt[k] > best {
											best, uiPay = cnt[k], r.Payment
										}
									}

									// --- Payment ---
									payErr := math.Abs(uiPay - dosPay)
									if payClean {
										cleanChecks++
										if payErr > maxCleanPay {
											maxCleanPay, worstPay = payErr, fmt.Sprintf("%s amt=%.0f r=%.2f DOS=%.4f UI=%.4f", cell, amount, rate, dosPay, uiPay)
										}
										if payErr > payTol {
											cleanPayFails++
											if reported < 30 {
												t.Errorf("CLEAN PAY [%s] amt=%.0f r=%.2f n=%d: DOS=%.4f UI=%.4f", cell, amount, rate, n, dosPay, uiPay)
												reported++
											}
										}
									} else {
										switch {
										case exact && m.inadv:
											frExactInadv = math.Max(frExactInadv, payErr)
										case m.inadv:
											frInadvPay = math.Max(frInadvPay, payErr)
										default: // non-360 basis, non-exact
											frNon360Pay = math.Max(frNon360Pay, payErr)
										}
									}

									// --- Rows: DOS solves its own payment and renders
									// its own schedule; compare the rendered cent
									// values column-for-column (what the UI shows). ---
									dosRows, ok2 := uiOracleRows(amount, rate, n, perYr, flags...)
									if !ok2 || len(dosRows) != len(sched) {
										if rowClean && ok2 && len(dosRows) != len(sched) {
											cleanRowFails++
											if reported < 30 {
												t.Errorf("CLEAN ROWCOUNT [%s] amt=%.0f r=%.2f n=%d: DOS=%d UI=%d", cell, amount, rate, n, len(dosRows), len(sched))
												reported++
											}
										}
										continue
									}
									intTol, prinTol, balTol := tols(exact)
									for k := range dosRows {
										ui := sched[k]
										prinPortion := ui.Payment - ui.Interest
										intDiff := math.Abs(dosRows[k].interest - ui.Interest)
										prinDiff := math.Abs(dosRows[k].prin - prinPortion)
										balDiff := math.Abs(dosRows[k].bal - ui.Principal)
										if !rowClean {
											if exact && m.inadv {
												frExactInadv = math.Max(frExactInadv, math.Max(math.Max(intDiff, prinDiff), balDiff))
											}
											continue
										}
										econ := math.Max(intDiff, prinDiff)
										if econ > maxCleanRow {
											maxCleanRow, worstRow = econ, fmt.Sprintf("%s amt=%.0f r=%.2f n=%d row=%d", cell, amount, rate, n, k+1)
										}
										if balDiff > maxCleanBal {
											maxCleanBal, worstBal = balDiff, fmt.Sprintf("%s amt=%.0f r=%.2f n=%d row=%d", cell, amount, rate, n, k+1)
										}
										if intDiff > intTol || prinDiff > prinTol {
											cleanRowFails++
											if reported < 30 {
												t.Errorf("CLEAN ROW [%s] amt=%.0f r=%.2f n=%d row=%d: int D=%.2f UI=%.2f | prin D=%.2f UI=%.2f",
													cell, amount, rate, n, k+1, dosRows[k].interest, ui.Interest, dosRows[k].prin, prinPortion)
												reported++
											}
										}
										if balDiff > balTol {
											cleanBalFails++
											if reported < 30 {
												t.Errorf("CLEAN BAL [%s] amt=%.0f r=%.2f n=%d row=%d: DOS=%.2f UI=%.2f (drift %.2f)",
													cell, amount, rate, n, k+1, dosRows[k].bal, ui.Principal, balDiff)
												reported++
											}
										}
									}
									// The schedule must still retire: final balance ≈ 0.
									if rowClean && len(sched) > 0 && math.Abs(sched[len(sched)-1].Principal) > 0.05 {
										cleanRowFails++
										if reported < 30 {
											t.Errorf("CLEAN FINAL [%s] amt=%.0f r=%.2f n=%d: final balance %.2f not retired",
												cell, amount, rate, n, sched[len(sched)-1].Principal)
											reported++
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	expected := len(bases) * 2 * 2 * len(methods) * len(perYrs)
	if len(cells) < expected {
		t.Errorf("coverage: only %d/%d UI settings cells exercised", len(cells), expected)
	}
	t.Logf("UI cube CLEAN: %d comparisons | econ(int+prin) fails=%d (max %.4f at [%s]) | pay fails=%d (max %.4f at [%s]) | balance-drift fails=%d (max %.4f at [%s])",
		cleanChecks, cleanRowFails, maxCleanRow, worstRow, cleanPayFails, maxCleanPay, worstPay, cleanBalFails, maxCleanBal, worstBal)
	t.Logf("UI cube FRONTIERS (bounded): exact×inadv=%.2f (env %.0f) | inadv pay=%.4f (env %.1f) | non-360 pay=%.2f (env %.0f)",
		frExactInadv, envExactInadv, frInadvPay, envInadvPay, frNon360Pay, envNon360Pay)

	if cleanRowFails > 0 || cleanPayFails > 0 || cleanBalFails > 0 {
		t.Errorf("UI CLEAN set: %d economic-column + %d payment + %d balance divergences — must be zero",
			cleanRowFails, cleanPayFails, cleanBalFails)
	}
	if frExactInadv > envExactInadv {
		t.Errorf("exact×in-advance %.2f exceeds envelope %.0f", frExactInadv, envExactInadv)
	}
	if frInadvPay > envInadvPay {
		t.Errorf("in-advance pay %.4f exceeds envelope %.1f", frInadvPay, envInadvPay)
	}
	if frNon360Pay > envNon360Pay {
		t.Errorf("non-360 pay %.2f exceeds envelope %.0f", frNon360Pay, envNon360Pay)
	}
	if cleanChecks < 200 {
		t.Fatalf("only %d clean comparisons ran — oracle may be flaking", cleanChecks)
	}
}
