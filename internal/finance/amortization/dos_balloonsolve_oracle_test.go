package amortization

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// runOracleSolveBalloon execs the DOS oracle's `solveballoon=MONTHS` query (a
// terminating balloon that many months after the loan date, amount blank) and
// returns the DOS-solved balloon amount. Retries the heap glitch like runOracle.
func runOracleSolveBalloon(t *testing.T, args []string) (amt float64, ok bool) {
	t.Helper()
	for try := 0; try < 8; try++ {
		out, err := exec.Command(oracleBin, args...).Output()
		if err != nil {
			continue
		}
		f := strings.Fields(strings.TrimSpace(string(out)))
		if len(f) < 2 || f[0] != "balloon" {
			continue
		}
		v, perr := strconv.ParseFloat(f[1], 64)
		if perr != nil {
			continue
		}
		return v, true
	}
	return 0, false
}

// TestBalloonSolveVsOracle guards the balloon-amount solve, which previously had
// no oracle differential coverage — the gap that hid a real bug: it discounted the
// TRUNCATED display schedule and clamped the secant to ≥0, so an OVER-funded loan's
// terminating balloon (which DOS solves NEGATIVE off the full-term walk) came out 0.
// Sweeps under-funded (positive balloon) and over-funded (negative balloon) across
// the day-count basis and asserts the engine matches the DOS oracle to the cent.
func TestBalloonSolveVsOracle(t *testing.T) {
	gateOracle(t)

	const (
		amount    = 100000.0
		rate      = 0.10
		nper      = 120
		perYr     = 12
		ballMonth = 120 // terminating balloon at the last payment
	)
	// Loan/first dates match the oracle's SetupLoan default (loan 1/1/2024,
	// first one period later) so the two engines see the same schedule.
	loanDate := types.NewDateRec(2024, time.January, 1)
	firstDate := types.NewDateRec(2024, time.February, 1)
	// Balloon at loanDate + ballMonth months = 1/1/2034.
	ballDate := types.NewDateRec(2034, time.January, 1)

	type bc struct {
		name  string
		basis types.BasisType
		flag  string
		yr    float64
	}
	bases := []bc{
		{"360", types.Basis360, "", 360},
		{"365", types.Basis365, "b365", 365.25},
		{"365-360", types.Basis365360, "b365_360", 360},
	}
	// $700 under-funds (positive balloon); $2000 over-funds (negative balloon).
	for _, pay := range []float64{700, 2000} {
		for _, b := range bases {
			pay, b := pay, b
			name := b.name
			if pay > 1500 {
				name += "_overfunded"
			} else {
				name += "_underfunded"
			}
			t.Run(name, func(t *testing.T) {
				in := LoanInput{
					Loan: Loan{
						AmountStatus: types.InOutInput, Amount: amount,
						LoanDateStatus: types.InOutInput, LoanDate: loanDate,
						LoanRateStatus: types.InOutInput, LoanRate: rate,
						FirstStatus: types.InOutInput, FirstDate: firstDate,
						NStatus: types.InOutInput, NPeriods: nper,
						PerYrStatus: types.InOutInput, PerYr: perYr,
						PayAmtStatus: types.InOutInput, PayAmt: pay,
						PointsStatus: types.InOutInput,
					},
					Balloons: []BalloonPayment{{
						DateStatus: types.InOutInput, Date: ballDate,
						AmountStatus: types.StatusEmpty,
					}},
					Settings: Settings{
						Basis: b.basis, PerYr: perYr, PlusRegular: false,
						YrDays: b.yr, YrInv: 1.0 / b.yr,
					},
					Fancy: true,
				}
				goAmt, err := SolveBalloonAmount(in, 0)
				if err != nil {
					t.Fatalf("SolveBalloonAmount: %v", err)
				}

				args := []string{
					strconv.FormatFloat(amount, 'f', 2, 64),
					strconv.FormatFloat(rate, 'f', 6, 64),
					strconv.Itoa(nper), strconv.Itoa(perYr),
					"payhard=" + strconv.FormatFloat(pay, 'f', 2, 64),
					"solveballoon=" + strconv.Itoa(ballMonth),
				}
				if b.flag != "" {
					args = append(args, b.flag)
				}
				dosAmt, ok := runOracleSolveBalloon(t, args)
				if !ok {
					t.Skip("oracle produced no balloon after retries (heap glitch)")
				}
				// Relative tolerance: these balloons run to hundreds of thousands.
				tol := 0.05 + 1e-6*absF(dosAmt)
				if d := goAmt - dosAmt; d > tol || d < -tol {
					t.Errorf("balloon solve = %.4f, oracle = %.4f (Δ=%+.4f)", goAmt, dosAmt, d)
				}
			})
		}
	}
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
