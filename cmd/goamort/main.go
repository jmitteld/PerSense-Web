// goamort mirrors the amort_oracle CLI token language and drives the Go
// amortization engine, printing output in the same machine format so a
// runner can diff the two. Differential-audit tool only; not shipped.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/persense/persense-port/internal/finance/amortization"
	"github.com/persense/persense-port/internal/types"
)

// parseDMY reads a `D.M.Y` token exactly as the oracle's ParseDMY does
// (amort_oracle.pas:147-160) — EXCEPT for one thing it structurally cannot do.
//
// The oracle stores d, m and y VERBATIM into a Pascal daterec: three bytes, no
// validation, no clamp, no roll. `loandmy=31.6.2023` leaves DOS holding an
// impossible 31 June 2023 and the DOS date primitives resolve it their own way.
// types.DateRec is time.Time-backed, so it cannot represent that date at all;
// time.Date NORMALISES it to 1 July 2023 and the two engines then run different
// screens while the runner believes it is comparing one.
//
// Measured 2026-07-31 (round 9): `100000 0.08 144 12 loandmy=31.6.2023
// firstdmy=31.8.2023 adj=67:0.04: adj=77:0.11: pre=20:24:12:150` gives oracle
// 60638.01 vs goamort 59673.04 — and the oracle run with loandmy=1.7.2023 gives
// 59673.04 exactly, i.e. goamort was answering the rolled screen correctly. A
// sweep over day-of-month 1/15/28/29/30/31 reported 21 "engine divergences",
// every one of them at day 31 in a 30-day month, all of them this.
//
// This is the FOURTH harness date bug in the same family (see START_HERE §5).
// Rather than fake a date the port cannot hold, refuse the token loudly: an
// unrepresentable input must never be silently turned into a different one.
func parseDMY(s string) (types.DateRec, bool) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return types.DateRec{}, false
	}
	d, e1 := strconv.Atoi(parts[0])
	m, e2 := strconv.Atoi(parts[1])
	y, e3 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || e3 != nil {
		return types.DateRec{}, false
	}
	dr := types.NewDateRec(y, time.Month(m), d)
	if dr.Time.Day() != d || int(dr.Time.Month()) != m || dr.Time.Year() != y {
		fmt.Fprintf(os.Stderr,
			"goamort: %q is not a real calendar date. The DOS oracle stores it "+
				"verbatim (amort_oracle.pas:147-160); types.DateRec cannot, and would "+
				"silently roll it to %s — a DIFFERENT screen. Refusing rather than "+
				"reporting a fake divergence.\n",
			s, dr.Time.Format("2.1.2006"))
		return types.DateRec{}, false
	}
	return dr, true
}

// monthsAfter replicates the oracle's raw month arithmetic:
// tot := (loan.m-1)+months; date.m=(tot mod 12)+1; date.y=loan.y+(tot div 12); date.d=loan.d
// monthsAfter mirrors how amort_oracle anchors every option date on the loan
// date — b<N>=, pre= and adj= all use the same three lines
// (amort_oracle.pas:172-176, :254-258, :310-314):
//
//	tot := (h^.loandate.m - 1) + monthsVal;
//	date.d := h^.loandate.d;
//	date.m := (tot mod 12) + 1;
//	date.y := h^.loandate.y + (tot div 12);
//	CheckForDaysTooLarge(date);
//
// THE CLAMP IS LOAD-BEARING. `CheckForDaysTooLarge` pins the day DOWN to the
// last day of the target month; types.NewDateRec, being time.Time-backed,
// NORMALISES instead and rolls forward — 30 Feb becomes 2 March rather than
// 28 Feb. A day-30 loan date with an offset landing in February therefore put
// the option on a different date in each engine, and the differential reported
// an "engine divergence" of up to 8,500 in total interest that was entirely
// this. Third harness bug of this shape; see docs/testing_policy.md §7b.
func monthsAfter(loan types.DateRec, months int) types.DateRec {
	y, mo, d := loan.Time.Year(), int(loan.Time.Month()), loan.Time.Day()
	tot := (mo - 1) + months
	ty, tm := y+tot/12, time.Month(tot%12+1)
	if dim := daysInMonth(ty, tm); d > dim {
		d = dim
	}
	return types.NewDateRec(ty, tm, d)
}

// daysInMonth is DOS's daysinm (VIDEODAT.pas) — the length of the given month,
// leap-aware.
func daysInMonth(y int, m time.Month) int {
	return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, -1).Day()
}

func main() {
	args := os.Args[1:]
	if len(args) < 4 {
		fmt.Println("usage: goamort AMOUNT RATE N PERYR [tokens]")
		os.Exit(1)
	}
	amount, _ := strconv.ParseFloat(args[0], 64)
	rate, _ := strconv.ParseFloat(args[1], 64)
	n, _ := strconv.Atoi(args[2])
	peryr, _ := strconv.Atoi(args[3])
	toks := args[4:]

	loanDate := types.NewDateRec(2024, time.January, 1)
	var firstDate types.DateRec
	if peryr == 26 || peryr == 52 {
		firstDate = types.DateRec{Time: loanDate.Time.AddDate(0, 0, 364/peryr)}
	} else if peryr >= 1 && 12/peryr >= 1 {
		firstDate = monthsAfter(loanDate, 12/peryr)
	} else {
		firstDate = loanDate
	}

	loan := amortization.Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n,
		PerYrStatus: types.InOutInput, PerYr: peryr,
		PayAmtStatus:   types.StatusEmpty,
		LoanDateStatus: types.InOutInput, LoanDate: loanDate,
		FirstStatus: types.InOutInput, FirstDate: firstDate,
	}
	s := amortization.Settings{Basis: types.Basis360, PerYr: byte(peryr), YrDays: 360, YrInv: 1.0 / 360}

	var balloons []amortization.BalloonPayment
	var adjs []amortization.RateAdjustment
	var pres []amortization.Prepayment
	var mor amortization.Moratorium
	var targ amortization.Target
	var skip amortization.SkipMonths
	fancy := false
	wantRows := false
	wantBDump := false
	wantAPR := false
	var payoffDate types.DateRec
	havePayoff := false

	// ORDER IS LOAD-BEARING. amort_oracle.pas:759-779 applies loandmy=/firstdmy=
	// BETWEEN SetupLoan and SetupBalloons/SetupPrepayments/SetupAdjustments,
	// because those three anchor every option date on h^.loandate
	// (`tot := (h^.loandate.m - 1) + monthsVal; date.d := h^.loandate.d`, at
	// :172-176, :254-258 and :310-314). Override the loan date AFTER them and the
	// options stay pinned to the default while the loan itself moves, so the two
	// engines get option ROWS on different dates.
	//
	// The oracle carries a warning about this: when fuzzer5 grew a loan-date axis
	// on 2026-07-25 the same ordering bug there "turned 85 of 95 compared cases
	// divergent in a single run — all of them the same harness artifact, none a
	// port bug". This CLI had the mirror-image defect (its own comment claimed the
	// oracle anchored options on the DEFAULT loan date, which is the opposite of
	// what amort_oracle.pas does) and it produced exactly the same false finding
	// on 2026-07-31 before being caught. Hence this pre-pass.
	for _, t := range toks {
		switch {
		case strings.HasPrefix(t, "loandmy="):
			if d, ok := parseDMY(t[8:]); ok {
				loan.LoanDate = d
				loanDate = d
			}
		case strings.HasPrefix(t, "firstdmy="):
			if d, ok := parseDMY(t[9:]); ok {
				loan.FirstDate = d
				firstDate = d
			}
		}
	}

	for _, t := range toks {
		switch {
		case strings.HasPrefix(t, "b") && strings.Contains(t, "=") && len(t) > 2 && t[1] >= '0' && t[1] <= '9':
			eq := strings.Index(t, "=")
			mo, e1 := strconv.Atoi(t[1:eq])
			amt, e2 := strconv.ParseFloat(t[eq+1:], 64)
			if e1 != nil || e2 != nil || mo < 0 {
				continue
			}
			balloons = append(balloons, amortization.BalloonPayment{
				DateStatus: types.InOutInput, Date: monthsAfter(loanDate, mo),
				AmountStatus: types.InOutInput, Amount: amt,
			})
			fancy = true
		case strings.HasPrefix(t, "adj="):
			body := strings.SplitN(t[4:], ":", 3)
			if len(body) != 3 {
				continue
			}
			mo, _ := strconv.Atoi(body[0])
			a := amortization.RateAdjustment{DateStatus: types.InOutInput, Date: monthsAfter(loanDate, mo)}
			if body[1] != "" {
				r, err := strconv.ParseFloat(body[1], 64)
				if err == nil {
					a.LoanRateStatus, a.LoanRate = types.InOutInput, r
				}
			}
			if body[2] != "" {
				v, err := strconv.ParseFloat(body[2], 64)
				if err == nil {
					a.AmountStatus, a.Amount, a.AmtOK = types.InOutInput, v, true
				}
			}
			adjs = append(adjs, a)
			fancy = true
		case strings.HasPrefix(t, "pre="):
			body := strings.SplitN(t[4:], ":", 4)
			if len(body) != 4 {
				continue
			}
			st, _ := strconv.Atoi(body[0])
			nn, _ := strconv.Atoi(body[1])
			py, _ := strconv.Atoi(body[2])
			amt, _ := strconv.ParseFloat(body[3], 64)
			pres = append(pres, amortization.Prepayment{
				StartDateStatus: types.InOutInput, StartDate: monthsAfter(loanDate, st),
				NNStatus: types.InOutInput, NN: nn,
				PerYrStatus: types.InOutInput, PerYr: py,
				PaymentStatus: types.InOutInput, Payment: amt,
			})
			fancy = true
		}
	}
	for _, t := range toks {
		switch {
		case strings.HasPrefix(t, "pay="):
			v, _ := strconv.ParseFloat(t[4:], 64)
			loan.PayAmtStatus, loan.PayAmt = types.InOutDefault, v
		case strings.HasPrefix(t, "payhard="):
			v, _ := strconv.ParseFloat(t[8:], 64)
			loan.PayAmtStatus, loan.PayAmt = types.InOutInput, v
		case strings.HasPrefix(t, "pts="):
			v, _ := strconv.ParseFloat(t[4:], 64)
			loan.PointsStatus, loan.Points = types.InOutInput, v
		case t == "inadv":
			s.InAdvance = true
		case t == "r78":
			s.R78 = true
		case t == "usa":
			s.USARule = true
		case t == "prepaid":
			s.Prepaid = true
		case t == "exact":
			s.Exact = true
		case t == "plusreg":
			s.PlusRegular = true
		case t == "b365":
			s.Basis, s.YrDays, s.YrInv = types.Basis365, 365.25, 1.0/365.25
		case t == "b365_360":
			s.Basis, s.YrDays, s.YrInv = types.Basis365360, 360, 1.0/360
		case t == "rows":
			wantRows = true
		case t == "apr":
			wantAPR = true
		case strings.HasPrefix(t, "loandmy="):
			if d, ok := parseDMY(t[8:]); ok {
				loan.LoanDate = d
				loanDate = d
			}
		case strings.HasPrefix(t, "firstdmy="):
			if d, ok := parseDMY(t[9:]); ok {
				loan.FirstDate = d
			}
		}
	}
	for _, t := range toks {
		switch {
		case strings.HasPrefix(t, "first="):
			mo, _ := strconv.Atoi(t[6:])
			loan.FirstDate = monthsAfter(loanDate, mo)
		case strings.HasPrefix(t, "mor="):
			mo, err := strconv.Atoi(t[4:])
			if err == nil && mo >= 0 {
				mor = amortization.Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: monthsAfter(loanDate, mo)}
				fancy = true
			}
		case strings.HasPrefix(t, "targ="):
			v, err := strconv.ParseFloat(t[5:], 64)
			if err == nil {
				targ = amortization.Target{TargetStatus: types.InOutInput, TargetValue: v}
				fancy = true
			}
		case strings.HasPrefix(t, "skip="):
			ms, err := amortization.MonthSetFromString(t[5:])
			if err == nil {
				skip = amortization.SkipMonths{SkipStatus: types.InOutInput, SkipStr: t[5:], MonthSet: ms}
				fancy = true
			}
		case t == "noterm":
			// amort_oracle.pas:763-764 blanks BOTH the term and the last-date
			// cells and lets the walk run until the loan retires.
			loan.NStatus, loan.NPeriods = types.StatusEmpty, 0
			loan.LastStatus, loan.LastOK = types.StatusEmpty, false
		case t == "non":
			// amort_oracle.pas:769 blanks ONLY n, leaving the typed last date in
			// force, so FirstPass derives the term through NumberOfInstallments
			// (INTSUTIL.pas:936). Note the oracle sets laststatus but NOT lastok.
			loan.NStatus, loan.NPeriods = types.StatusEmpty, 0
		case strings.HasPrefix(t, "lastdmy="):
			if d, ok := parseDMY(strings.TrimPrefix(t, "lastdmy=")); ok {
				loan.LastStatus, loan.LastDate = types.InOutInput, d
			}
		case t == "bdump":
			wantBDump = true
		case strings.HasPrefix(t, "payoff="):
			if d, ok := parseDMY(t[7:]); ok {
				payoffDate = d
				havePayoff = true
			}
		}
	}

	// API-layer coercion (handlers.go:850): weekly/biweekly on 360 -> 365
	if (peryr == 26 || peryr == 52) && s.Basis == types.Basis360 {
		s.Basis, s.YrDays, s.YrInv = types.Basis365, 365.25, 1.0/365.25
	}

	input := amortization.LoanInput{
		Loan: loan, Balloons: balloons, Adjustments: adjs, Prepayments: pres,
		Moratorium: mor, Target: targ, SkipMonths: skip, Settings: s, Fancy: fancy,
	}

	if havePayoff {
		v, err := amortization.PayoffBalance(input, payoffDate)
		if err != nil {
			fmt.Println("ERR", err)
			return
		}
		fmt.Printf("payoff %.4f\n", v)
		return
	}

	res := amortization.Amortize(input)
	if res.Err != nil {
		fmt.Println("ERR", res.Err)
		return
	}

	if wantAPR {
		fmt.Printf("apr %.6f\n", res.APR)
		return
	}

	// payment: user-supplied echoed; else the first regular (PayNum>=1) row's
	// nonzero payment (matches DOS h^.payamt for the solved case in the
	// scenarios probed; row-level diffs are the authoritative signal).
	payment := loan.PayAmt
	if loan.PayAmtStatus < types.InOutDefault {
		for _, r := range res.Schedule {
			if r.PayNum < 1 || r.PayAmt <= 0 {
				continue
			}
			// during a moratorium rows are interest-only, not h^.payamt
			if mor.FirstRepayStatus >= types.InOutDefault &&
				r.Date.Time.Before(mor.FirstRepay.Time) {
				continue
			}
			// a balloon-dated row carries the balloon, not h^.payamt
			onBalloon := false
			for _, b := range balloons {
				if b.Date.Time.Equal(r.Date.Time) {
					onBalloon = true
				}
			}
			if onBalloon {
				continue
			}
			// a target-boosted row carries targ+interest, not h^.payamt
			if targ.TargetStatus >= types.InOutDefault &&
				r.PayAmt-r.Interest <= targ.TargetValue+0.005 {
				continue
			}
			payment = r.PayAmt
			break
		}
	}

	if wantBDump {
		// Mirrors amort_oracle's bdump block: the post-FirstPass last regular
		// payment date and term, in DOS's M/D/YYYY shape, so the two CLIs can be
		// diffed directly. Emitted before the payment line, as the oracle does.
		fmt.Printf("lastdate %d/%d/%d nperiods %d\n",
			int(res.LastDate.Time.Month()), res.LastDate.Time.Day(),
			res.LastDate.Time.Year(), res.NPeriods)
	}
	if wantRows {
		fmt.Printf("payment %.4f\n", payment)
		for _, r := range res.Schedule {
			if r.PayNum < 1 {
				continue // settlement row: DOS rows-mode excludes paynum 0/-1
			}
			if os.Getenv("GOAMORT_ROWDATES") != "" {
				fmt.Printf("row %d %d/%d/%d pay %.2f int %.4f prin %.4f bal %.4f\n",
					r.PayNum, int(r.Date.Time.Month()), r.Date.Time.Day(),
					r.Date.Time.Year()%100, r.PayAmt, r.Interest, r.PayAmt-r.Interest, r.Principal)
				continue
			}
			fmt.Printf("row %d int %.4f prin %.4f bal %.4f\n", r.PayNum, r.Interest, r.PayAmt-r.Interest, r.Principal)
		}
		fmt.Println("end")
		return
	}
	fmt.Printf("payment %.4f interest %.2f paid %.2f\n", payment, res.TotalInt, res.TotalPaid)
}
