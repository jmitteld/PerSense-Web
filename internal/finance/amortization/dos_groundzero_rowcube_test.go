package amortization

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// dos_groundzero_rowcube_test.go — the exhaustive, ground-zero confidence suite.
//
// Motivated by docs/postmortem_365_exact_interest.md: the prior suite reported
// ~99 confidence yet shipped a real bug because it compared the wrong OUTPUT
// (the regular payment) on the one AXIS where the failure was invisible (360
// basis), and the cell that did expose it was quarantined. This suite is built
// to docs/testing_policy.md:
//
//   - It compares EVERY UI-visible per-row quantity — payment, interest,
//     principal portion, remaining balance, and cumulative interest — not just a
//     summary scalar. (testing_policy §1)
//   - It crosses the full composable settings cube (basis × exact × prepaid ×
//     perYr) against each interest/timing METHOD (ordinary / in-advance / R78 /
//     USA-rule), so interacting factors (exact × basis, method × basis) are
//     exercised together, not in isolation. (testing_policy §2)
//   - It drives the real Amortize dispatch (the path the API/UI use) to SOLVE the
//     payment, then feeds the DOS-solved payment to BOTH engines so the per-row
//     comparison isolates the accrual engine from payment-solve precision.
//     (testing_policy §3)
//   - It partitions cells into CLEAN (asserted to zero divergence at the cent
//     level) and FRONTIER (known, documented gaps tracked with a bounded
//     envelope — see docs/exact_groundzero_findings.md). No frontier is silently
//     suppressed: each is logged and guarded so it cannot worsen. (testing_policy §5)
//
// Requires the DOS oracle binary (build via legacy/oracle/build_linux.sh).

// fullRow is one oracle schedule row: the three printed columns.
type fullRow struct{ interest, prin, bal float64 }

func runOracleRowsFull(amount, rate float64, n, perYr int, pay float64, flags ...string) ([]fullRow, bool) {
	args := []string{
		strconv.FormatFloat(amount, 'f', 2, 64), strconv.FormatFloat(rate, 'f', 10, 64),
		strconv.Itoa(n), strconv.Itoa(perYr), "rows",
		"pay=" + strconv.FormatFloat(pay, 'f', 10, 64)}
	args = append(args, flags...)
	out, err := exec.Command(oracleBin, args...).Output()
	if err != nil {
		return nil, false
	}
	var rows []fullRow
	for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(ln)
		if len(f) == 8 && f[0] == "row" {
			in, _ := strconv.ParseFloat(f[3], 64)
			pr, _ := strconv.ParseFloat(f[5], 64)
			bal, _ := strconv.ParseFloat(f[7], 64)
			rows = append(rows, fullRow{interest: in, prin: pr, bal: bal})
		}
	}
	if len(rows) == 0 {
		return nil, false
	}
	return rows, true
}

func gzSettings(perYr int, basis types.BasisType, exact, prepaid, inadv, r78, usa bool) Settings {
	s := Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360}
	switch basis {
	case types.Basis365:
		s.Basis, s.YrDays, s.YrInv = types.Basis365, 365.25, 1.0/365.25
	case types.Basis365360:
		s.Basis, s.YrDays, s.YrInv = types.Basis365360, 360, 1.0/360
	}
	s.Exact, s.Prepaid, s.InAdvance, s.R78, s.USARule = exact, prepaid, inadv, r78, usa
	return s
}

func gzGoSolvePayment(amount, rate float64, n, perYr int, s Settings) (float64, bool) {
	in := gzLoanInput(amount, rate, n, perYr, s)
	in.Loan.PayAmtStatus = types.StatusEmpty
	r := Amortize(in)
	if r.Err != nil || len(r.Schedule) == 0 {
		return 0, false
	}
	return modalReg(r.Schedule), true
}

func gzGoScheduleWithPayment(amount, rate float64, n, perYr int, s Settings, pay float64) (AmortResult, bool) {
	in := gzLoanInput(amount, rate, n, perYr, s)
	in.Loan.PayAmtStatus = types.InOutDefault // non-hard: no per-period Round2, matching the oracle's defp
	in.Loan.PayAmt = pay
	r := Amortize(in)
	if r.Err != nil || len(r.Schedule) == 0 {
		return r, false
	}
	return r, true
}

func gzLoanInput(amount, rate float64, n, perYr int, s Settings) LoanInput {
	return LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
		PayAmtStatus:   types.StatusEmpty,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr)},
		Settings: s}
}

func basisFlag(b types.BasisType) (string, bool) {
	switch b {
	case types.Basis365:
		return "b365", true
	case types.Basis365360:
		return "b365_360", true
	default:
		return "", false
	}
}

func basisName(b types.BasisType) string {
	switch b {
	case types.Basis365:
		return "365"
	case types.Basis365360:
		return "365/360"
	default:
		return "360"
	}
}

// Documented frontier envelopes (see docs/exact_groundzero_findings.md). A
// frontier may not exceed its envelope — that is a hard regression guard — but
// within it the cell is a known, tracked gap rather than a clean cell.
const (
	// Bucket A — exact × in-advance: true daily accrual is not implemented for
	// the annuity-due (in-advance) schedule, so both the schedule and the solved
	// payment diverge. Large; tracked only so it cannot grow.
	envExactInadvRowPay = 50000.0
	// Bucket B — in-advance (non-exact): the annuity-due final row is an
	// off-by-one in row count and the payment carries a small reconstruction
	// imprecision. Rows are validated by TestDOSFancyFlagSweep; here only the
	// payment is bounded.
	envInadvPay = 1.5
	// Bucket C — non-360 basis payment (365 & 365/360, non-exact): DOS's
	// closed-form payment uses a basis day-count factor the Go closed form does
	// not, a long-standing ~rel-3e-4 gap the old relative-tolerance cube hid.
	// The SCHEDULE rows are exact (validated below); only the solved payment
	// differs. Documented in docs/exact_groundzero_findings.md.
	envNon360Pay = 120.0
)

// TestDOSGroundZeroRowCube is the exhaustive settings-cube row-level differential.
func TestDOSGroundZeroRowCube(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s); build via legacy/oracle/build_linux.sh", oracleBin)
	}

	bases := []types.BasisType{types.Basis360, types.Basis365, types.Basis365360}
	perYrs := []int{1, 2, 4, 12}
	methods := []struct {
		name            string
		inadv, r78, usa bool
	}{
		{"ordinary", false, false, false},
		{"in-advance", true, false, false},
		{"r78", false, true, false},
		{"usa", false, false, true},
	}
	amounts := []float64{50000, 200000}
	rates := []float64{0.06, 0.11}
	yearsList := []int{8, 20}

	// CLEAN tolerances — the engines are fed the SAME payment, so a faithful port
	// tracks to the cent. Allow one cent of display rounding (a hair more for the
	// long-schedule float accumulation) plus a tiny relative slack.
	rowTol := func(v float64) float64 { return 0.03 + 1e-6*math.Abs(v) }
	payTol := 0.05

	cleanChecks, cleanRowFails, cleanPayFails := 0, 0, 0
	var worstCleanRow, worstCleanPay string
	maxCleanRow, maxCleanPay := 0.0, 0.0
	reported := 0

	// Frontier trackers: max observed error per frontier class.
	frExactInadv, frInadvPay, frNon360Pay := 0.0, 0.0, 0.0
	covered := map[string]int{}

	for _, basis := range bases {
		bf, hasBF := basisFlag(basis)
		for _, exact := range []bool{false, true} {
			for _, prepaid := range []bool{false, true} {
				for _, method := range methods {
					inadv, r78, usa := method.inadv, method.r78, method.usa
					_ = usa
					// Classify this cell.
					//   rowClean: row values are asserted to the cent. Clean for
					//     every method except in-advance (annuity-due rows are
					//     validated separately by TestDOSFancyFlagSweep; the final
					//     row count differs by one, and exact×in-advance accrual is
					//     unimplemented).
					//   payClean: the solved payment is asserted to the cent. Clean
					//     only on the 360 basis (non-in-advance); non-360 bases carry
					//     a documented closed-form day-count gap, and exact / in-
					//     advance use iterated/annuity-due solves with bounded
					//     precision.
					//     EXCEPT exact×in-advance (on a non-360 basis, where the exact
					//     method is live), now CLEAN: the dedicated
					//     generateExactInAdvanceSchedule reproduces DOS's settlement
					//     row + one-period base shift + n-1 amortizing rows exactly.
					//     360-basis in-advance (where exact is a no-op) and non-exact
					//     in-advance stay frontiers.
					exactInadv := exact && basis != types.Basis360
					rowClean := !inadv || exactInadv
					// Payment is clean (asserted to the cent) for every non-in-advance
					// loan: the 360 closed form, the exact path (DOS Iterate ported),
					// AND non-360 non-exact (the first-period actual-day prorate now
					// matches DOS, Amortize.pas:1286). Exact×in-advance (non-360) is now
					// clean too (dosIteratePayment drives the settlement-shifted terminal
					// balance to zero). Other in-advance cells remain a frontier
					// (annuity-due reconstruction precision).
					payClean := !inadv || exactInadv
					for _, perYr := range perYrs {
						var flags []string
						if hasBF {
							flags = append(flags, bf)
						}
						if exact {
							flags = append(flags, "exact")
						}
						if prepaid {
							flags = append(flags, "prepaid")
						}
						if inadv {
							flags = append(flags, "inadv")
						}
						if r78 {
							flags = append(flags, "r78")
						}
						if usa {
							flags = append(flags, "usa")
						}
						cell := fmt.Sprintf("basis=%s|exact=%v|prepaid=%v|method=%s|py=%d",
							basisName(basis), exact, prepaid, method.name, perYr)
						s := gzSettings(perYr, basis, exact, prepaid, inadv, r78, usa)

						for _, amount := range amounts {
							for _, rate := range rates {
								for _, ny := range yearsList {
									n := ny * perYr
									dosPay, ok := runOraclePayment(amount, rate, n, perYr, flags...)
									if !ok || dosPay <= 0 {
										continue
									}
									goPay, gok := gzGoSolvePayment(amount, rate, n, perYr, s)
									dosRows, ok2 := runOracleRowsFull(amount, rate, n, perYr, dosPay, flags...)
									goRes, gok2 := gzGoScheduleWithPayment(amount, rate, n, perYr, s, dosPay)
									if !gok || !ok2 || !gok2 {
										continue
									}
									covered[cell]++

									// --- Payment-solve comparison ---
									payErr := math.Abs(goPay - dosPay)
									if payClean {
										cleanChecks++
										if payErr > maxCleanPay {
											maxCleanPay = payErr
											worstCleanPay = fmt.Sprintf("%s amt=%.0f r=%.2f n=%d DOS=%.4f Go=%.4f", cell, amount, rate, n, dosPay, goPay)
										}
										if payErr > payTol {
											cleanPayFails++
											if reported < 30 {
												t.Errorf("CLEAN PAY [%s] amt=%.0f r=%.2f n=%d: DOS=%.4f Go=%.4f (err %.4f)",
													cell, amount, rate, n, dosPay, goPay, payErr)
												reported++
											}
										}
									} else {
										switch {
										case exactInadv:
											// Closed frontier: exact×in-advance (non-360) is now
											// CLEAN, so this never fires. Kept as a guard.
											frExactInadv = math.Max(frExactInadv, payErr)
										case inadv:
											frInadvPay = math.Max(frInadvPay, payErr)
										default: // non-360 basis, non-exact, non-inadv
											frNon360Pay = math.Max(frNon360Pay, payErr)
										}
									}

									// --- Row-level comparison (same payment fed to both) ---
									// The oracle's `rows` output excludes the paynum-0
									// settlement-interest line (in-advance / prepaid); drop the
									// Go settlement row so the per-payment sequences align.
									goSched := goRes.Schedule
									for len(goSched) > 0 && goSched[0].PayNum == 0 {
										goSched = goSched[1:]
									}
									if len(goSched) != len(dosRows) {
										if rowClean {
											cleanRowFails++
											if reported < 30 {
												t.Errorf("CLEAN ROWCOUNT [%s] amt=%.0f r=%.2f n=%d: DOS=%d Go=%d",
													cell, amount, rate, n, len(dosRows), len(goSched))
												reported++
											}
										}
										continue
									}
									var cum, goCum float64
									for k := range dosRows {
										gi := goSched[k].Interest
										gp := goSched[k].PayAmt - goSched[k].Interest
										gb := goSched[k].Principal
										cum += dosRows[k].interest
										goCum += gi
										worst := math.Max(math.Abs(dosRows[k].interest-gi),
											math.Max(math.Abs(dosRows[k].prin-gp), math.Abs(dosRows[k].bal-gb)))
										if !rowClean {
											if exactInadv {
												frExactInadv = math.Max(frExactInadv, worst)
											}
											continue
										}
										if worst > maxCleanRow {
											maxCleanRow = worst
											worstCleanRow = fmt.Sprintf("%s amt=%.0f r=%.2f n=%d row=%d int(D=%.4f G=%.4f) prin(D=%.4f G=%.4f) bal(D=%.4f G=%.4f)",
												cell, amount, rate, n, k+1, dosRows[k].interest, gi, dosRows[k].prin, gp, dosRows[k].bal, gb)
										}
										if math.Abs(dosRows[k].interest-gi) > rowTol(gi) ||
											math.Abs(dosRows[k].prin-gp) > rowTol(gp) ||
											math.Abs(dosRows[k].bal-gb) > rowTol(gb) {
											cleanRowFails++
											if reported < 30 {
												t.Errorf("CLEAN ROW [%s] amt=%.0f r=%.2f n=%d row=%d: int D=%.4f G=%.4f | prin D=%.4f G=%.4f | bal D=%.4f G=%.4f",
													cell, amount, rate, n, k+1, dosRows[k].interest, gi, dosRows[k].prin, gp, dosRows[k].bal, gb)
												reported++
											}
										}
									}
									// Cumulative interest over the AMORTIZING rows. (Go's per-row
									// IntToDate column additionally includes the paynum-0
									// settlement interest for exact-in-advance — matching DOS's
									// displayed column — so it is validated separately by
									// TestDOSExactInAdvanceSettlement via the dumpraw totals.)
									if rowClean && len(goSched) > 0 {
										gCum := goCum
										if math.Abs(gCum-cum) > 0.05+1e-5*math.Abs(cum) {
											cleanRowFails++
											if reported < 30 {
												t.Errorf("CLEAN CUMINT [%s] amt=%.0f r=%.2f n=%d: DOS=%.2f Go=%.2f",
													cell, amount, rate, n, cum, gCum)
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
	}

	// Coverage: every (cell) combination must have produced at least one comparison.
	expectedCells := len(bases) * 2 * 2 * len(methods) * len(perYrs)
	if len(covered) < expectedCells {
		t.Errorf("coverage gap: only %d/%d settings cells produced a comparison", len(covered), expectedCells)
	}

	t.Logf("CLEAN cells: %d comparisons, row fails=%d (max clean row err=%.4f at [%s]), pay fails=%d (max clean pay err=%.4f at [%s])",
		cleanChecks, cleanRowFails, maxCleanRow, worstCleanRow, cleanPayFails, maxCleanPay, worstCleanPay)
	t.Logf("FRONTIERS (documented, bounded — docs/exact_groundzero_findings.md): exact×inadv max=%.2f (env %.0f) | inadv pay max=%.4f (env %.1f) | non-360 pay max=%.2f (env %.0f)",
		frExactInadv, envExactInadvRowPay, frInadvPay, envInadvPay, frNon360Pay, envNon360Pay)

	// Frontier regression guards: a documented gap may not get worse.
	if frExactInadv > envExactInadvRowPay {
		t.Errorf("exact×in-advance error %.2f exceeds documented envelope %.0f", frExactInadv, envExactInadvRowPay)
	}
	if frInadvPay > envInadvPay {
		t.Errorf("in-advance payment error %.4f exceeds documented envelope %.1f", frInadvPay, envInadvPay)
	}
	if frNon360Pay > envNon360Pay {
		t.Errorf("non-360 payment error %.2f exceeds documented envelope %.0f", frNon360Pay, envNon360Pay)
	}
	// The CLEAN set must be exactly that — zero divergence at the cent level.
	if cleanRowFails > 0 || cleanPayFails > 0 {
		t.Errorf("CLEAN set has %d row and %d payment divergences — these must be zero (see individual CLEAN ... reports above)",
			cleanRowFails, cleanPayFails)
	}
	if cleanChecks < 300 {
		t.Fatalf("only %d clean comparisons ran — oracle may be flaking", cleanChecks)
	}
}
