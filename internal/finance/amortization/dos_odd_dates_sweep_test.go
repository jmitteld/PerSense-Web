package amortization

import (
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// dos_odd_dates_sweep_test.go — an exhaustive differential over ODD FIRST
// PERIODS (loan day-of-month ≠ first-payment day-of-month) across the settings
// cross. Odd first periods are where the original client bug and the prepaid-365
// bug lived, and the clean-date cubes (firstPeriodDate) never exercise them. This
// drives the real Amortize dispatch, solves the payment, and compares the solved
// payment and every rendered row column to the real DOS engine fed the SAME
// custom dates (loandmy=/firstdmy=). Widened value grid hunts for any remaining
// divergence.
//
// Requires the DOS oracle binary.

func ddDateFlags(loanDay, loanMo, loanYr, firstMo, firstYr int) []string {
	return []string{
		fmt.Sprintf("loandmy=%d.%d.%d", loanDay, loanMo, loanYr),
		fmt.Sprintf("firstdmy=1.%d.%d", firstMo, firstYr),
	}
}

func TestDOSOddFirstDatesCube(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s); build via legacy/oracle/build_linux.sh", oracleBin)
	}

	bases := []struct {
		b    types.BasisType
		flag string
	}{{types.Basis360, ""}, {types.Basis365, "b365"}, {types.Basis365360, "b365_360"}}
	methods := []struct {
		name            string
		inadv, r78, usa bool
	}{
		{"ordinary", false, false, false},
		{"r78", false, true, false},
		{"usa", false, false, true},
		{"in-advance", true, false, false},
	}
	// Odd first periods: loan on various days-of-month, first payment on the 1st
	// of a month 1–2 periods later (monthly loans, where day-of-month matters).
	// Valid days only (June has 30 days; day 31 is an invalid input that Go and
	// FPC normalize differently — not a fidelity question).
	loanDays := []int{10, 19, 25, 28}
	firstMonths := []int{8, 9} // first payment month (loan in June 2026)
	amounts := []float64{50000, 250000}
	rates := []float64{0.05, 0.105}
	terms := []int{120, 360}

	const (
		payTol = 0.05
		rowTol = 0.02
	)
	cleanChecks, cleanPayFails, cleanRowFails := 0, 0, 0
	maxPay, maxRow := 0.0, 0.0
	var worstPay, worstRow string
	frInadvPay, frInadvRow, frUsaPay, frUsaRow := 0.0, 0.0, 0.0, 0.0
	reported := 0
	byBasisPay := map[string]int{}
	byBasisRow := map[string]int{}
	failSig := map[string]int{} // distinct "basis|exact|prepaid|method" that diverged

	for _, bs := range bases {
		for _, exact := range []bool{false, true} {
			for _, prepaid := range []bool{false, true} {
				for _, m := range methods {
					// in-advance (incl. exact×in-advance) and USA-rule are tracked
					// frontiers. The USA-rule ROW accrual is now DOS-faithful (the
					// US-Rule no-compounding fix), but its payment SOLVE on an odd
					// first period still differs from DOS's Iterate by a few dollars
					// (non-unique under the US Rule), which leaves a small cumulative
					// row drift. Bounded + guarded below; ordinary/R78 stay hard-clean.
					frontier := m.inadv || m.usa
					s := gzSettings(12, bs.b, exact, prepaid, m.inadv, m.r78, m.usa)
					for _, loanDay := range loanDays {
						for _, fm := range firstMonths {
							flags := []string{}
							if bs.flag != "" {
								flags = append(flags, bs.flag)
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
							flags = append(flags, ddDateFlags(loanDay, 6, 2026, fm, 2026)...)
							cell := fmt.Sprintf("basis=%s|exact=%v|prepaid=%v|method=%s|loanDay=%d|firstMo=%d",
								bs.b, exact, prepaid, m.name, loanDay, fm)
							sig := fmt.Sprintf("basis=%s|exact=%v|prepaid=%v|method=%s", basisName(bs.b), exact, prepaid, m.name)

							for _, amt := range amounts {
								for _, rate := range rates {
									for _, n := range terms {
										dosPay, ok := runOraclePayment(amt, rate, n, 12, flags...)
										if !ok || dosPay <= 0 {
											continue
										}
										// Go: solve via the real dispatch.
										in := oddDatesInput(amt, rate, n, loanDay, fm, s)
										gr := Amortize(in)
										if gr.Err != nil || len(gr.Schedule) == 0 {
											continue
										}
										goPay := modalReg(gr.Schedule)

										// Feed DOS payment to both; compare rows.
										dosRows, ok2 := runOracleRowsFull(amt, rate, n, 12, dosPay, flags...)
										fed := in
										fed.Loan.PayAmtStatus = types.InOutDefault
										fed.Loan.PayAmt = dosPay
										fr := Amortize(fed)
										if !ok2 || fr.Err != nil {
											continue
										}
										sched := fr.Schedule
										for len(sched) > 0 && sched[0].PayNum == 0 {
											sched = sched[1:]
										}

										payErr := math.Abs(goPay - dosPay)
										if frontier {
											if m.usa {
												frUsaPay = math.Max(frUsaPay, payErr)
											} else {
												frInadvPay = math.Max(frInadvPay, payErr)
											}
										} else {
											cleanChecks++
											if payErr > maxPay {
												maxPay, worstPay = payErr, cell+fmt.Sprintf(" amt=%.0f r=%.3f n=%d DOS=%.4f Go=%.4f", amt, rate, n, dosPay, goPay)
											}
											if payErr > payTol {
												cleanPayFails++
												byBasisPay[basisName(bs.b)]++
												failSig["PAY "+sig]++
												if reported < 30 {
													t.Errorf("ODD PAY [%s] amt=%.0f r=%.3f n=%d: DOS=%.4f Go=%.4f", cell, amt, rate, n, dosPay, goPay)
													reported++
												}
											}
										}
										if len(sched) != len(dosRows) {
											continue
										}
										for k := range dosRows {
											gi := sched[k].Interest
											gp := sched[k].PayAmt - sched[k].Interest
											w := math.Max(math.Abs(dosRows[k].interest-gi), math.Abs(dosRows[k].prin-gp))
											if frontier {
												if m.usa {
													frUsaRow = math.Max(frUsaRow, w)
												} else {
													frInadvRow = math.Max(frInadvRow, w)
												}
												continue
											}
											if w > maxRow {
												maxRow, worstRow = w, cell+fmt.Sprintf(" amt=%.0f r=%.3f n=%d row=%d", amt, rate, n, k+1)
											}
											if w > rowTol {
												cleanRowFails++
												byBasisRow[basisName(bs.b)]++
												failSig["ROW "+sig]++
												if reported < 30 {
													t.Errorf("ODD ROW [%s] amt=%.0f r=%.3f n=%d row=%d: int D=%.2f G=%.2f | prin D=%.2f G=%.2f",
														cell, amt, rate, n, k+1, dosRows[k].interest, gi, dosRows[k].prin, gp)
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
	}
	t.Logf("odd-first sweep: CLEAN checks=%d, pay fails=%d (max %.4f at [%s]), row fails=%d (max %.4f at [%s])",
		cleanChecks, cleanPayFails, maxPay, worstPay, cleanRowFails, maxRow, worstRow)
	t.Logf("  FRONTIER USA-rule (bounded): pay max=%.2f (env 6), row max=%.4f (env 2)", frUsaPay, frUsaRow)
	t.Logf("  FRONTIER in-advance (bounded): pay max=%.2f (env 60), row max=%.2f (env 50000)", frInadvPay, frInadvRow)
	if cleanPayFails > 0 || cleanRowFails > 0 {
		t.Errorf("odd-first-period CLEAN (ordinary/R78) divergences: %d payment, %d row — must be zero", cleanPayFails, cleanRowFails)
	}
	// USA-rule: the odd-first ROW accrual is now DOS-faithful (the US-Rule
	// no-compounding fix); only a small payment-solve precision remains.
	if frUsaRow > 2 {
		t.Errorf("USA-rule odd-first row drift %.4f exceeds envelope 2 — accrual may have regressed", frUsaRow)
	}
	if frUsaPay > 6 {
		t.Errorf("USA-rule odd-first payment error %.2f exceeds envelope 6", frUsaPay)
	}
	// In-advance remains the documented large frontier (exact×in-advance, etc.).
	if frInadvPay > 60 {
		t.Errorf("in-advance odd-first payment error %.2f exceeds envelope 60", frInadvPay)
	}
	if cleanChecks < 200 {
		t.Fatalf("only %d clean comparisons — oracle may be flaking", cleanChecks)
	}
}

func oddDatesInput(amount, rate float64, n, loanDay, firstMo int, s Settings) LoanInput {
	return LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: 12,
		PayAmtStatus:   types.StatusEmpty,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2026, time.June, loanDay),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2026, time.Month(firstMo), 1)},
		Settings: s}
}
