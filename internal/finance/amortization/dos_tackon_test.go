package amortization

import (
	"math"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// ---------------------------------------------------------------------------
// TackOnFinalBalloon differential tests — discrepancies.md §46.
//
// DOS computes an implied TERMINATING balloon for any over-specified fancy loan
// (Amortize.pas:1386-1394 → :1040-1088), paints it into the Balloon Payments
// grid as an output cell, and then de-activates it with dec(nballoons) so it
// takes no part in the payment table or the APR. The web port previously showed
// nothing there — the source of the client's "DOS version adds the balloon
// payments" report (§44 Finding B).
//
// The oracle's `bdump` token (legacy/oracle/amort_oracle.pas:910-931) emits the
// balloon GRID exactly as the DOS screen paints it, INCLUDING the de-activated
// row, which is what makes this differentially testable at all.
// ---------------------------------------------------------------------------

type oracleBalloonRow struct {
	date             types.DateRec
	amount           float64
	dstatus, astatus int
}

type oracleBalloonDump struct {
	nballoons int
	rows      []oracleBalloonRow
}

// runOracleBalloonDump execs the real DOS engine with `bdump` and parses the
// balloon grid it paints.
func runOracleBalloonDump(amount, rate float64, n, perYr int, flags ...string) (oracleBalloonDump, bool) {
	args := append([]string{
		strconv.FormatFloat(amount, 'f', 2, 64),
		strconv.FormatFloat(rate, 'f', 17, 64),
		strconv.Itoa(n), strconv.Itoa(perYr),
	}, flags...)
	args = append(args, "bdump")
	for try := 0; try < 8; try++ {
		out, err := exec.Command(oracleBin, args...).Output()
		if err != nil {
			continue
		}
		var d oracleBalloonDump
		sawHeader := false
		for _, ln := range strings.Split(string(out), "\n") {
			f := strings.Fields(ln)
			switch {
			case len(f) >= 4 && f[0] == "nballoons":
				d.nballoons, _ = strconv.Atoi(f[1])
				sawHeader = true
			case len(f) >= 9 && f[0] == "balloonrow":
				var r oracleBalloonRow
				md := strings.Split(f[3], "/")
				if len(md) != 3 {
					continue
				}
				mm, _ := strconv.Atoi(md[0])
				dd, _ := strconv.Atoi(md[1])
				yy, _ := strconv.Atoi(md[2])
				r.date = types.NewDateRec(yy, monthOf(mm), dd)
				r.dstatus, _ = strconv.Atoi(f[5])
				r.amount, _ = strconv.ParseFloat(f[7], 64)
				r.astatus, _ = strconv.Atoi(f[9])
				d.rows = append(d.rows, r)
			}
		}
		if sawHeader {
			return d, true
		}
	}
	return oracleBalloonDump{}, false
}

func monthOf(m int) time.Month { return time.Month(m) }

// TestTackOnFinalBalloonMatchesDOSGrid pins the tacked-on terminating balloon —
// its DATE, its AMOUNT, and its exclusion from the table — against the real DOS
// engine's own balloon grid, across the shapes that make a loan over-specified.
func TestTackOnFinalBalloonMatchesDOSGrid(t *testing.T) {
	gateOracle(t)

	base := []string{"b365_360", "exact", "prepaid", "pts=0",
		"loandmy=1.1.2025", "firstdmy=1.2.2025"}

	// The Go side of each case, built to the same loan the oracle flags describe.
	mk := func(pay float64, n int, mut func(*LoanInput)) LoanInput {
		in := LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: 100000,
				LoanRateStatus: types.InOutInput, LoanRate: 0.08,
				NStatus: types.InOutInput, NPeriods: n,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PayAmtStatus: types.InOutInput, PayAmt: pay,
				PointsStatus: types.InOutInput, Points: 0,
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2025, 1, 1),
				FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2025, 2, 1),
			},
			Fancy: true,
			Settings: Settings{Basis: types.Basis365360, PerYr: 12, YrDays: 360,
				YrInv: 1.0 / 360, Exact: true, Prepaid: true},
		}
		if mut != nil {
			mut(&in)
		}
		return in
	}

	cases := []struct {
		name  string
		flags []string
		in    LoanInput
	}{
		{
			// Skip months: three months a year carry no payment, so a payment that
			// would amortize a 360-month loan leaves a large residual.
			name:  "skip",
			flags: []string{"payhard=800.00", "skip=1,3,5"},
			in: mk(800, 360, func(in *LoanInput) {
				ms, _ := MonthSetFromString("1,3,5")
				in.SkipMonths = SkipMonths{SkipStatus: types.InOutInput,
					SkipStr: "1,3,5", MonthSet: ms}
			}),
		},
		{
			// A mid-term balloon: very_last is still lastdate, so this is the
			// NON-merge arm with a user balloon already in the array.
			name:  "midterm_balloon",
			flags: []string{"payhard=750.00", "bdate=15.6.2031:33333.00"},
			in: mk(750, 360, func(in *LoanInput) {
				in.Balloons = append(in.Balloons, BalloonPayment{
					DateStatus: types.InOutInput, Date: types.NewDateRec(2031, 6, 15),
					AmountStatus: types.InOutInput, Amount: 33333})
			}),
		},
		{
			// A prepayment series bounded by count.
			name:  "prepay_series",
			flags: []string{"payhard=900.00", "predmy=1.3.2028:60:12:150"},
			in: mk(900, 120, func(in *LoanInput) {
				in.Prepayments = append(in.Prepayments, Prepayment{
					StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2028, 3, 1),
					NNStatus: types.InOutInput, NN: 60,
					PerYrStatus: types.InOutInput, PerYr: 12,
					PaymentStatus: types.InOutInput, Payment: 150})
			}),
		},
		{
			// UNDER-specified the other way: the payment over-amortizes, so the
			// terminating balloon solves NEGATIVE (a refund). DOS shows it anyway.
			name:  "overpaid_negative",
			flags: []string{"payhard=1200.00", "skip=1,3,5"},
			in: mk(1200, 360, func(in *LoanInput) {
				ms, _ := MonthSetFromString("1,3,5")
				in.SkipMonths = SkipMonths{SkipStatus: types.InOutInput,
					SkipStr: "1,3,5", MonthSet: ms}
			}),
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			flags := append(append([]string(nil), base...), c.flags...)
			dump, ok := runOracleBalloonDump(100000, 0.08, c.in.Loan.NPeriods, 12, flags...)
			if !ok {
				t.Skip("oracle produced no balloon dump")
			}

			// The DOS grid row that is NOT a live balloon is the tacked-on one:
			// index > nballoons, painted outp/outp.
			var want *oracleBalloonRow
			for i := range dump.rows {
				if i >= dump.nballoons {
					want = &dump.rows[i]
					break
				}
			}
			if want == nil {
				t.Fatalf("oracle produced no tacked-on row (nballoons=%d rows=%d)",
					dump.nballoons, len(dump.rows))
			}
			if want.dstatus != int(types.InOutOutput) || want.astatus != int(types.InOutOutput) {
				t.Errorf("oracle tack row status = %d/%d, want outp/outp",
					want.dstatus, want.astatus)
			}

			res := Amortize(c.in)
			if res.Err != nil {
				t.Fatalf("Amortize: %v", res.Err)
			}
			var got *ResolvedBalloon
			for i := range res.Balloons {
				if res.Balloons[i].TackedOn {
					got = &res.Balloons[i]
				}
			}
			if got == nil {
				t.Fatalf("no TackedOn balloon surfaced; DOS shows %s = %.2f",
					want.date.Time.Format("2006-01-02"), want.amount)
			}
			if d := dateutil.DateComp(got.Date, want.date); d != 0 {
				t.Errorf("tack date = %s, want %s (DOS)",
					got.Date.Time.Format("2006-01-02"), want.date.Time.Format("2006-01-02"))
			}
			if math.Abs(got.Amount-want.amount) > 0.02 {
				t.Errorf("tack amount = %.4f, want %.4f (DOS)", got.Amount, want.amount)
			}
			t.Logf("%s: tack %s = %.2f (DOS %.4f), nballoons=%d grid rows=%d",
				c.name, got.Date.Time.Format("2006-01-02"), got.Amount, want.amount,
				dump.nballoons, len(dump.rows))
		})
	}
}

// TestTackOnFinalBalloonExcludedFromTableAndAPR proves the de-activated row is
// display-only: adding it must not move a single figure the schedule reports.
// DOS's dec(nballoons) is exactly this claim, and the oracle's interest/paid/APR
// for these loans are computed with the row already dropped.
func TestTackOnFinalBalloonExcludedFromTableAndAPR(t *testing.T) {
	gateOracle(t)

	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.08,
			NStatus: types.InOutInput, NPeriods: 360,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus: types.InOutInput, PayAmt: 800,
			PointsStatus: types.InOutInput, Points: 0,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2025, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2025, 2, 1),
		},
		Fancy: true,
		Settings: Settings{Basis: types.Basis365360, PerYr: 12, YrDays: 360,
			YrInv: 1.0 / 360, Exact: true, Prepaid: true},
	}
	ms, _ := MonthSetFromString("1,3,5")
	in.SkipMonths = SkipMonths{SkipStatus: types.InOutInput, SkipStr: "1,3,5", MonthSet: ms}

	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("Amortize: %v", res.Err)
	}
	// Oracle (bdump run above shows tack = 217,436.00 at 1/1/2055):
	//   amort_oracle 100000 0.08 360 12 b365_360 exact prepaid pts=0 \
	//     loandmy=1.1.2025 firstdmy=1.2.2025 payhard=800.00 skip=1,3,5
	//   → payment 800.0000 interest 333436.00 paid 433436.00
	const (
		wantInt  = 333436.00
		wantPaid = 433436.00
	)
	if math.Abs(res.TotalInt-wantInt) > 0.02 {
		t.Errorf("totalInterest = %.2f, want %.2f (DOS) — the tacked-on balloon "+
			"leaked into the payment table", res.TotalInt, wantInt)
	}
	if math.Abs(res.TotalPaid-wantPaid) > 0.02 {
		t.Errorf("totalPaid = %.2f, want %.2f (DOS) — the tacked-on balloon "+
			"leaked into the payment table", res.TotalPaid, wantPaid)
	}
	// And the row IS surfaced.
	found := false
	for _, b := range res.Balloons {
		if b.TackedOn {
			found = true
			if math.Abs(b.Amount-217436.00) > 0.02 {
				t.Errorf("tack amount = %.2f, want 217436.00 (DOS bdump)", b.Amount)
			}
		}
	}
	if !found {
		t.Error("no TackedOn balloon surfaced")
	}
}

// TestTackOnFinalBalloonGateClosed pins the DOS gate (Amortize.pas:1386-1394).
// `>= defp` is DEFAULT-or-INPUT and EXCLUDES a solved (outp) value, so a loan
// whose payment the engine computed gets NO terminating balloon — it amortizes
// exactly by construction. Nor does a non-fancy loan: DOS gates on `fancy`
// first, which is why an ordinary loan's grid stays empty.
func TestTackOnFinalBalloonGateClosed(t *testing.T) {
	mk := func(fancy bool, payStatus int8, pay float64) LoanInput {
		return LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: 100000,
				LoanRateStatus: types.InOutInput, LoanRate: 0.08,
				NStatus: types.InOutInput, NPeriods: 360,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PayAmtStatus: payStatus, PayAmt: pay,
				PointsStatus: types.InOutInput, Points: 0,
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2025, 1, 1),
				FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2025, 2, 1),
			},
			Fancy: fancy,
			Settings: Settings{Basis: types.Basis365360, PerYr: 12, YrDays: 360,
				YrInv: 1.0 / 360, Exact: true, Prepaid: true},
		}
	}
	hasTack := func(r AmortResult) bool {
		for _, b := range r.Balloons {
			if b.TackedOn {
				return true
			}
		}
		return false
	}

	// Not fancy: no advanced options on screen — DOS's `fancy` is false.
	if r := Amortize(mk(false, types.InOutInput, 800)); r.Err == nil && hasTack(r) {
		t.Error("non-fancy loan surfaced a terminating balloon; DOS gates on fancy")
	}
	// Payment SOLVED (blank on screen): payamtstatus is not >= defp.
	solved := mk(true, types.InOutEmpty, 0)
	ms, _ := MonthSetFromString("1,3,5")
	solved.SkipMonths = SkipMonths{SkipStatus: types.InOutInput, SkipStr: "1,3,5", MonthSet: ms}
	if r := Amortize(solved); r.Err == nil && hasTack(r) {
		t.Error("solved-payment loan surfaced a terminating balloon; DOS's " +
			"`h^.payamtstatus >= defp` excludes a solved (outp) value")
	}
}
