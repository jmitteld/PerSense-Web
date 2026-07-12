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
	return types.NewDateRec(y, time.Month(m), d), true
}

// monthsAfter replicates the oracle's raw month arithmetic:
// tot := (loan.m-1)+months; date.m=(tot mod 12)+1; date.y=loan.y+(tot div 12); date.d=loan.d
func monthsAfter(loan types.DateRec, months int) types.DateRec {
	y, mo, d := loan.Time.Year(), int(loan.Time.Month()), loan.Time.Day()
	tot := (mo - 1) + months
	return types.NewDateRec(y+tot/12, time.Month(tot%12+1), d)
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
	wantAPR := false
	var payoffDate types.DateRec
	havePayoff := false

	// two passes like the oracle: loandmy/firstdmy applied after balloons etc.?
	// Oracle order: SetupLoan, balloons/pre/adj (anchored on DEFAULT loan date
	// 1.1.2024), then pay=/payhard=/pts=/flags, then loandmy=/firstdmy= (override),
	// then first=/mor=/targ=/skip= (anchored on the OVERRIDDEN loan date).
	// Replicate exactly.
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

	if wantRows {
		fmt.Printf("payment %.4f\n", payment)
		for _, r := range res.Schedule {
			if r.PayNum < 1 {
				continue // settlement row: DOS rows-mode excludes paynum 0/-1
			}
			fmt.Printf("row %d int %.4f prin %.4f bal %.4f\n", r.PayNum, r.Interest, r.PayAmt-r.Interest, r.Principal)
		}
		fmt.Println("end")
		return
	}
	fmt.Printf("payment %.4f interest %.2f paid %.2f\n", payment, res.TotalInt, res.TotalPaid)
}
