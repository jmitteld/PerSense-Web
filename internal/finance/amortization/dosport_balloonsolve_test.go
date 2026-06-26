package amortization

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// ao2Input builds an AO2 case: loan with a GIVEN (under-amortizing) payment and a
// date-only target balloon to solve.
func ao2Input(amount, rate, pay float64, n, perYr, balloonMonths int) LoanInput {
	return LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			NStatus: types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: perYr,
			PayAmtStatus: types.InOutInput, PayAmt: pay,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr),
		},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr),
			YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Fancy: true,
		Balloons: []BalloonPayment{{
			DateStatus: types.InOutInput, Date: dateMonthsAfterLoan(balloonMonths),
			AmountStatus: types.StatusEmpty, // unknown ⇒ solve
		}},
	}
}

// TestDOSPortAO2BalloonSolve validates the port's AO2 (target-balloon-amount) solve
// two ways: (1) round-trip (always) — the solved balloon must retire the loan
// (forward FinalPrinc ≈ 0); since the port's forward balloon walk is already
// oracle-exact and DOS's EstimateAndRefineBalloon is DEFINED as the Iterate that
// zeros that walk's terminal, a balloon that retires the loan is the DOS-faithful
// answer. (2) ORACLE cross-check (opt-in, PERSENSE_FUZZ) — feed the port's solved
// balloon back to the DOS oracle as a KNOWN balloon and confirm it reproduces the
// port's total interest. No oracle-harness change needed (reuses the b<m>= token).
// NOTE: the production piecewise engine does NOT agree here (it diverges from DOS
// on this solve, like AO7+balloon), so it is deliberately NOT used as the reference.
func TestDOSPortAO2BalloonSolve(t *testing.T) {
	rng := rand.New(rand.NewSource(52525))
	nCases := 300
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, e := strconv.Atoi(s); e == nil && v > 0 {
			nCases = v
		}
	}
	ran, retireFail, vsDiv, oraclesRun := 0, 0, 0, 0
	_, oracleErr := os.Stat(oracleBin)
	oracleOK := os.Getenv("PERSENSE_FUZZ") != "" && oracleErr == nil
	for i := 0; i < nCases; i++ {
		perYr := []int{12, 6, 4, 12, 12}[rng.Intn(5)]
		clean := 12 / perYr
		amount := float64(int(60000+rng.Float64()*400000)/1000) * 1000
		rate := math.Round((0.04+rng.Float64()*0.08)*10000) / 10000
		n := []int{12, 24, 36, 48}[rng.Intn(4)]
		// A balloon strictly inside the term, on a payment date.
		bMonths := (1 + rng.Intn(n-1)) * clean
		// Given payment: deliberately BELOW the fully-amortizing level so a positive
		// balloon is required. Use ~55-90% of the natural payment.
		natural := annuityPayment(amount, GrowthPerPeriod(&Loan{LoanRate: rate, PerYr: perYr}, 1.0/360), n)
		pay := math.Round(natural*(0.55+rng.Float64()*0.35)*100) / 100

		in := ao2Input(amount, rate, pay, n, perYr, bMonths)
		port := AmortizeDOS(in)
		if port.Err != nil || len(port.Schedule) == 0 || len(port.Balloons) != 1 {
			continue
		}
		ran++
		// (1) round-trip: the port's solved balloon must retire the loan. Because the
		// port's forward balloon walk is already oracle-exact, a balloon that drives
		// the terminal to zero IS the DOS-faithful answer (DOS's EstimateAndRefineBalloon
		// is defined as exactly that Iterate).
		if math.Abs(port.FinalPrinc) > math.Max(1.0, 1e-6*amount) {
			retireFail++
			if retireFail <= 6 {
				t.Errorf("AO2 not retired: amt=%.0f r=%.4f n=%d py=%d b@%d pay=%.2f → FinalPrinc=%.2f",
					amount, rate, n, perYr, bMonths, pay, port.FinalPrinc)
			}
		}
		// (2) ORACLE cross-check (no harness change): feed the port's SOLVED balloon
		// back to the DOS oracle as a KNOWN balloon with the same hard payment, and
		// confirm the oracle reproduces the port's total interest. The given payment
		// is hard (rounds per-period interest), so use payhard=.
		if oracleOK {
			solved := port.Balloons[0].Amount
			flags := []string{
				fmt.Sprintf("b%d=%s", bMonths, strconv.FormatFloat(solved, 'f', 2, 64)),
				fmt.Sprintf("payhard=%s", strconv.FormatFloat(pay, 'f', 2, 64)),
				"plusreg",
			}
			dosInt, dok := runOracleInterestFlags(amount, rate, n, perYr, flags...)
			if dok {
				oraclesRun++
				tol := math.Max(0.10, 1e-5*math.Abs(dosInt))
				if math.Abs(port.TotalInt-dosInt) > tol {
					vsDiv++
					if vsDiv <= 8 {
						t.Errorf("AO2 vs ORACLE: amt=%.0f r=%.4f n=%d py=%d b@%d pay=%.2f solved=%.2f → DOSport int=%.2f oracle int=%.2f (Δ %.2f)",
							amount, rate, n, perYr, bMonths, pay, solved, port.TotalInt, dosInt, port.TotalInt-dosInt)
					}
				}
			}
		}
	}
	t.Logf("AO2 balloon solve: ran=%d retireFail=%d oraclesRun=%d oracleDiv=%d", ran, retireFail, oraclesRun, vsDiv)
	if ran == 0 {
		t.Fatal("no AO2 cases ran")
	}
}
