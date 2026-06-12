package amortization

import (
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Amortization field-presence DISPATCH validation — the decision of which of the
// four solvable top-row fields {Amount, Rate, Payment, #Periods} to solve, given
// a valid date/frequency context, vs forward vs refuse. Two layers:
//
//  1. TestAmortDispatchCanonical — an oracle-independent decision table: with a
//     valid context and a self-consistent tuple, the screen is solvable iff at
//     most ONE of {A,R,P,N} is blank (one unknown), and refused otherwise. These
//     are the canonical amortization dispatch rules, not a snapshot of the port.
//  2. TestDOSAmortDispatchSweep — the same patterns fed to the REAL DOS engine
//     (amort_oracle `eval` mode) and compared by consequence: solvable-vs-refused
//     and the solved payment.
//
// The consistent tuple is 10000 @ 12% nominal, payment 888.4879, n=12 monthly,
// so any single blank recovers the others.

const (
	dispAmount = 10000.0
	dispRate   = 0.12
	dispPay    = 888.4879
	dispN      = 12
)

func stPresent(b bool) int8 {
	if b {
		return types.InOutInput
	}
	return types.StatusEmpty
}

// goAmortDispatch mirrors the API handler's amortization dispatch
// (handlers.go:1050-1103): solve Amount and/or Rate up front when blank (via
// FirstPass-on-a-copy + the CanCompute guards), then run Amortize, which itself
// solves the payment or the term when those are blank. Returns whether the
// screen is solvable and, if so, the regular payment.
func goAmortDispatch(haveA, haveR, haveP, haveN bool) (bool, float64) {
	// A blank field has BOTH an empty status and a zeroed value — matching the
	// real API (an omitted pointer becomes status-empty, value 0) and the
	// oracle's SetupEval. Leaving the consistent value behind a blank status
	// would let the engine use a "ghost" input the user never supplied.
	valOr0 := func(have bool, v float64) float64 {
		if have {
			return v
		}
		return 0
	}
	nOr0 := dispN
	if !haveN {
		nOr0 = 0
	}
	loan := Loan{
		AmountStatus:   stPresent(haveA),
		Amount:         valOr0(haveA, dispAmount),
		LoanRateStatus: stPresent(haveR),
		LoanRate:       valOr0(haveR, dispRate),
		PayAmtStatus:   stPresent(haveP),
		PayAmt:         valOr0(haveP, dispPay),
		NStatus:        stPresent(haveN),
		NPeriods:       nOr0,
		PerYrStatus:    types.InOutInput,
		PerYr:          12,
		LoanDateStatus: types.InOutInput,
		LoanDate:       nd(2024, time.January, 1),
		FirstStatus:    types.InOutInput,
		FirstDate:      nd(2024, time.February, 1),
		LastStatus:     types.StatusEmpty, // last date blank
	}
	input := LoanInput{
		Loan:     loan,
		Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360},
	}

	if !haveA || !haveR {
		solverLoan := input.Loan
		if err := FirstPass(&solverLoan); err != nil {
			return false, 0
		}
		if solverLoan.FirstStatus > types.StatusEmpty && solverLoan.FirstStatus < types.InOutDefault {
			solverLoan.FirstStatus = types.InOutDefault
		}
		if solverLoan.NPeriods > 0 && solverLoan.NStatus > types.StatusEmpty && solverLoan.NStatus < types.InOutDefault {
			solverLoan.NStatus = types.InOutDefault
		}
		solverInput := input
		solverInput.Loan = solverLoan
		if !haveA {
			solved, _, err := SolveLoanAmount(solverInput)
			if err != nil {
				return false, 0
			}
			input.Loan.AmountStatus = types.InOutInput
			input.Loan.Amount = solved
		}
		if !haveR {
			solved, _, err := SolveRate(solverInput)
			if err != nil {
				return false, 0
			}
			input.Loan.LoanRateStatus = types.InOutInput
			input.Loan.LoanRate = solved
		}
	}

	r := Amortize(input)
	if r.Err != nil || len(r.Schedule) == 0 {
		return false, 0
	}
	return true, r.Schedule[0].PayAmt
}

func blankCount(haveA, haveR, haveP, haveN bool) int {
	n := 0
	for _, b := range []bool{haveA, haveR, haveP, haveN} {
		if !b {
			n++
		}
	}
	return n
}

// TestAmortDispatchCanonical pins the amortization dispatch decision table: in a
// valid context, the screen is solvable iff at most one of {A,R,P,N} is blank.
func TestAmortDispatchCanonical(t *testing.T) {
	for bits := 0; bits < 16; bits++ {
		haveA := bits&1 != 0
		haveR := bits&2 != 0
		haveP := bits&4 != 0
		haveN := bits&8 != 0
		solvable, pay := goAmortDispatch(haveA, haveR, haveP, haveN)
		want := blankCount(haveA, haveR, haveP, haveN) <= 1
		if solvable != want {
			t.Errorf("A=%v R=%v P=%v N=%v: solvable=%v, want %v", haveA, haveR, haveP, haveN, solvable, want)
		}
		// Every solvable cell recovers the consistent payment.
		if solvable && math.Abs(pay-dispPay) > 0.01 {
			t.Errorf("A=%v R=%v P=%v N=%v: payment=%.4f, want ~%.4f", haveA, haveR, haveP, haveN, pay, dispPay)
		}
	}
}

// --- DOS differential ------------------------------------------------------

func b01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

type amortOutcome struct {
	solvable bool
	pay      float64
	raw      string
}

func amortEval(haveA, haveR, haveP, haveN bool) amortOutcome {
	out, err := exec.Command(oracleBin, "eval",
		b01(haveA), b01(haveR), b01(haveP), b01(haveN)).Output()
	raw := strings.TrimSpace(string(out))
	if err != nil {
		return amortOutcome{raw: "FAULT:" + err.Error()}
	}
	if raw == "INSUF" || strings.HasPrefix(raw, "ERR") {
		return amortOutcome{raw: raw}
	}
	f := strings.Fields(raw)
	if len(f) >= 3 && f[0] == "ok" && f[1] == "payment" {
		v, _ := strconv.ParseFloat(f[2], 64)
		return amortOutcome{solvable: true, pay: v, raw: raw}
	}
	return amortOutcome{raw: "UNPARSED:" + raw}
}

func TestDOSAmortDispatchSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("amort oracle not present (%s); build via legacy/oracle/build_linux.sh", oracleBin)
	}
	var bothSolve, bothRefuse, divergences int
	for bits := 0; bits < 16; bits++ {
		haveA := bits&1 != 0
		haveR := bits&2 != 0
		haveP := bits&4 != 0
		haveN := bits&8 != 0

		dos := amortEval(haveA, haveR, haveP, haveN)
		gS, gPay := goAmortDispatch(haveA, haveR, haveP, haveN)

		switch {
		case gS && dos.solvable:
			bothSolve++
			if math.Abs(gPay-dos.pay) > 0.01 {
				t.Errorf("payment mismatch A=%v R=%v P=%v N=%v: DOS %.4f Go %.4f",
					haveA, haveR, haveP, haveN, dos.pay, gPay)
			}
		case !gS && !dos.solvable:
			bothRefuse++
		default:
			divergences++
			t.Errorf("dispatch divergence A=%v R=%v P=%v N=%v: DOS solvable=%v Go solvable=%v (DOS: %q)",
				haveA, haveR, haveP, haveN, dos.solvable, gS, dos.raw)
		}
	}
	t.Logf("amort dispatch sweep: 16 cells | both-solve %d, both-refuse %d, divergences %d",
		bothSolve, bothRefuse, divergences)
}
