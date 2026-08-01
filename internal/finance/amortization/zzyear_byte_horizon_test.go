package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Engine-level regression guard for docs/discrepancies.md §55 — DOS's date
// record stores the YEAR in a BYTE, so a schedule whose nominal horizon runs
// past 2155 is amortized by DOS against a WRAPPED last-payment date, and the
// payment solve, the row count and the totals all follow that wrapped horizon.
//
// The mechanism lives in internal/dateutil (AddNPeriods' year jump and
// AddPeriod's inc/dec of the year field — see zzyearbyte_test.go for the
// site-by-site goldens). This file guards the thing that actually matters: that
// the AMORTIZATION ENGINE, reading a wrapped Last Pmt Date, produces DOS's
// schedule rather than one 14x too long.
//
// Case A is the worst screen in claude/convergence_assessment_2026-07-31c.md
// §3 — a 300-payment ANNUAL loan, which DOS does not refuse:
//
//	amort_oracle 391495.35 0.029252 300 1 loandmy=29.5.2023 firstdmy=29.7.2023 \
//	    adj=188:0.0808: adj=90:0.0227: pre=28:25:12:74.69
//	  -> payment 16693.3528 interest 623111.96 paid 1014607.31   (67 rows,
//	     last regular payment 7/29/2066)
//
// 123 + 299 = 422, truncated to 166, so DOS's last payment date is 7/29/2066 and
// NumberOfInstallments then counts 44 annual payments against it, not 300. The
// port ran all 300 (to the year 2322) and reported payment 11747.0979 with
// 8,964,450.80 of interest.
//
// Case C pins the ONE-SHORT boundary: a term that lands on py=255 exactly must
// NOT wrap, so the rule cannot be implemented as an over-eager clamp. The
// mirror direction — a wrap that puts the horizon BEHIND the first payment, so
// DOS refuses — is TestDOSYearByteWrappedScreenIsRefusedLikeDOS below.
func TestDOSYearByteScheduleHorizon(t *testing.T) {
	type adjSpec struct {
		months int
		rate   float64
	}
	type preSpec struct {
		startMonths, nn, perYr int
		amount                 float64
	}
	type screen struct {
		amount   float64
		rate     float64
		n, perYr int
		loanY    int
		loanM    time.Month
		loanD    int
		firstY   int
		firstM   time.Month
		firstD   int
		adjs     []adjSpec
		pres     []preSpec
		fancy    bool
		exact    bool
	}

	monthsAfter := func(loan types.DateRec, months int) types.DateRec {
		tot := (int(loan.Time.Month()) - 1) + months
		y := loan.Time.Year() + tot/12
		m := time.Month(tot%12 + 1)
		d := loan.Time.Day()
		if dim := types.NewDateRec(y, m, 1).Time.AddDate(0, 1, -1).Day(); d > dim {
			d = dim
		}
		return types.NewDateRec(y, m, d)
	}

	run := func(t *testing.T, s screen) AmortResult {
		t.Helper()
		loanD := types.NewDateRec(s.loanY, s.loanM, s.loanD)
		firstD := types.NewDateRec(s.firstY, s.firstM, s.firstD)
		in := LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: s.amount,
				LoanRateStatus: types.InOutInput, LoanRate: s.rate,
				NStatus: types.InOutInput, NPeriods: s.n,
				PerYrStatus: types.InOutInput, PerYr: s.perYr,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput, LoanDate: loanD,
				FirstStatus: types.InOutInput, FirstDate: firstD,
			},
			Settings: Settings{Basis: types.Basis360, PerYr: byte(s.perYr),
				YrDays: 360, YrInv: 1.0 / 360, Exact: s.exact},
			Fancy: s.fancy,
		}
		for _, a := range s.adjs {
			in.Adjustments = append(in.Adjustments, RateAdjustment{
				DateStatus: types.InOutInput, Date: monthsAfter(loanD, a.months),
				LoanRateStatus: types.InOutInput, LoanRate: a.rate,
			})
		}
		for _, p := range s.pres {
			in.Prepayments = append(in.Prepayments, Prepayment{
				StartDateStatus: types.InOutInput, StartDate: monthsAfter(loanD, p.startMonths),
				NNStatus: types.InOutInput, NN: p.nn,
				PerYrStatus: types.InOutInput, PerYr: p.perYr,
				PaymentStatus: types.InOutInput, Payment: p.amount,
			})
		}
		res := Amortize(in)
		if res.Err != nil {
			t.Fatalf("Amortize: %v", res.Err)
		}
		return res
	}

	cases := []struct {
		name      string
		s         screen
		wantInt   float64
		wantPaid  float64
		wantLastY int
		wantRows  int
		oracle    string
	}{
		{
			name: "A/annual-300-with-options (the 14x case)",
			s: screen{
				amount: 391495.35, rate: 0.029252, n: 300, perYr: 1,
				loanY: 2023, loanM: time.May, loanD: 29,
				firstY: 2023, firstM: time.July, firstD: 29,
				adjs:  []adjSpec{{188, 0.0808}, {90, 0.0227}},
				pres:  []preSpec{{28, 25, 12, 74.69}},
				fancy: true,
			},
			wantInt: 623111.96, wantPaid: 1014607.31, wantLastY: 2066,
			oracle: "amort_oracle 391495.35 0.029252 300 1 loandmy=29.5.2023 " +
				"firstdmy=29.7.2023 adj=188:0.0808: adj=90:0.0227: pre=28:25:12:74.69 " +
				"-> payment 16693.3528 interest 623111.96 paid 1014607.31",
		},
		{
			// The SECOND reader of the same rule, and the one that needs no
			// options at all: the plain walk must stop at the wrapped horizon
			// rather than run out the entered period count. DOS emits 1056 rows
			// and clears the balance on the last of them; the port ran all 7200,
			// cycling the calendar 28 times.
			name: "D/semimonthly-7200-plain-walk-stops-at-horizon",
			s: screen{
				amount: 421052.18, rate: 0.047119, n: 7200, perYr: 24,
				loanY: 2029, loanM: time.April, loanD: 15,
				firstY: 2029, firstM: time.May, firstD: 15,
			},
			wantInt: 875474.53, wantPaid: 1296526.71, wantLastY: 2073,
			wantRows: 1056,
			oracle: "amort_oracle 421052.18 0.047119 7200 24 loandmy=15.4.2029 " +
				"firstdmy=15.5.2029 -> payment 828.2686 interest 875474.53 paid 1296526.71",
		},
		{
			// THE CLAMP MUST STAY INERT HERE. This screen's horizon does NOT
			// wrap (2119 is inside the year byte) and its 1080 periods are
			// exactly the 1080 payment dates between first and last — but the
			// port's WALK dates drift a month at February 2100, because §54
			// (DOS's leap rule has no century correction) is deferred. A
			// row-by-row `date >= lastDate` transcription of DOS's `until`
			// therefore folded one row early and lost the final 0.56 of
			// interest. It was the single NEW divergence in the seed-77 sweep,
			// and it is why walkPeriods counts off the ENDPOINTS instead.
			name: "E/inert-when-the-horizon-does-not-wrap",
			s: screen{
				amount: 114948.20, rate: 0.025189, n: 1080, perYr: 12,
				loanY: 2029, loanM: time.April, loanD: 29,
				firstY: 2029, firstM: time.May, firstD: 29,
				exact: true,
			},
			wantInt: 175844.60, wantPaid: 290792.80, wantLastY: 2119,
			wantRows: 1080,
			oracle: "amort_oracle 114948.20 0.025189 1080 12 loandmy=29.4.2029 " +
				"firstdmy=29.5.2029 exact -> payment 269.2526 interest 175844.60 " +
				"paid 290792.80 (1080 rows)",
		},
		{
			// One short of the wrap: 123 + 132 = 255 exactly, so the year byte
			// does NOT roll and DOS solves the screen normally. A clamp
			// implemented one year early would break this and nothing else.
			name: "C/annual-133-one-short-of-the-wrap",
			s: screen{
				amount: 391495.35, rate: 0.029252, n: 133, perYr: 1,
				loanY: 2023, loanM: time.May, loanD: 29,
				firstY: 2023, firstM: time.July, firstD: 29,
			},
			wantInt: 1128391.70, wantPaid: 1519887.05, wantLastY: 2155,
			oracle: "amort_oracle 391495.35 0.029252 133 1 loandmy=29.5.2023 " +
				"firstdmy=29.7.2023 -> payment 11427.7222 interest 1128391.70 paid 1519887.05",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := run(t, c.s)
			if math.Abs(res.TotalInt-c.wantInt) > 0.005 {
				t.Errorf("interest = %.2f, want %.2f (DOS)\n  oracle: %s",
					res.TotalInt, c.wantInt, c.oracle)
			}
			if math.Abs(res.TotalPaid-c.wantPaid) > 0.005 {
				t.Errorf("total paid = %.2f, want %.2f (DOS)\n  oracle: %s",
					res.TotalPaid, c.wantPaid, c.oracle)
			}
			if c.wantRows > 0 && len(res.Schedule) != c.wantRows {
				t.Errorf("rows = %d, want %d (DOS)\n  oracle: %s",
					len(res.Schedule), c.wantRows, c.oracle)
			}
			if res.LastDate.Time.Year() != c.wantLastY {
				t.Errorf("derived Last Pmt Date year = %d, want %d (DOS's wrapped byte)\n  oracle: %s",
					res.LastDate.Time.Year(), c.wantLastY, c.oracle)
			}
		})
	}
}

// TestDOSYearByteWrappedScreenIsRefusedLikeDOS is the MIRROR direction of
// TestDOSYearByteScheduleHorizon: a wrapped horizon that lands BEHIND the first
// payment date must be refused, not silently solved. Before §55 the port
// happily produced a 200-year annual schedule here; DOS refuses it, and a port
// that solves a screen DOS refuses is the failure mode paired_regression.sh
// exists to catch.
//
//	amort_oracle 391495.35 0.029252 200 1 loandmy=29.5.2023 firstdmy=29.7.2023
//	  -> ERR There must be at least two regular payments.
//
// (123 + 199 = 322 -> 66 -> the year 1966.) DOS's wording is its own; the port
// gives C-A-5's message for the same condition — see validate.go.
func TestDOSYearByteWrappedScreenIsRefusedLikeDOS(t *testing.T) {
	loanD := types.NewDateRec(2023, time.May, 29)
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 391495.35,
			LoanRateStatus: types.InOutInput, LoanRate: 0.029252,
			NStatus: types.InOutInput, NPeriods: 200,
			PerYrStatus: types.InOutInput, PerYr: 1,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: loanD,
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2023, time.July, 29),
		},
		Settings: Settings{Basis: types.Basis360, PerYr: 1, YrDays: 360, YrInv: 1.0 / 360},
	}
	res := Amortize(in)
	if res.Err == nil {
		t.Fatalf("expected the wrapped screen to be refused as DOS refuses it; "+
			"got a schedule of %d rows, interest %.2f", len(res.Schedule), res.TotalInt)
	}
}
