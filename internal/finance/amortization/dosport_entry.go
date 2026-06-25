package amortization

// dosport_entry.go — state construction (buildDosEng), the payment-solve seed
// (EstimateAndRefinePayment, Amortize.pas:377-430), and the AmortizeDOS entry
// that mirrors the MakeTable dispatch for the ordinary engine. Parallel to the
// production Amortize(); selected by tests/fuzzer behind a flag.

import (
	"errors"
	"sort"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// errNoConverge is returned when the Newton payment solve fails to converge,
// mirroring DOS's "Computation of payment ... did not converge" (AMORTOP.pas:1489).
var errNoConverge = errors.New("amortization: payment solve did not converge")

// buildDosEng translates a LoanInput into the Pascal-style engine state.
func buildDosEng(input LoanInput) *dosEng {
	e := &dosEng{set: input.Settings, loan: input.Loan}

	// Derive the last regular payment date if absent: firstDate + (n-1) periods.
	if !e.loan.LastOK || e.loan.LastStatus < types.InOutDefault {
		last := e.loan.FirstDate
		for i := 1; i < e.loan.NPeriods; i++ {
			last, _ = dateutil.AddPeriod(last, e.loan.PerYr, e.loan.FirstDate.Time.Day(), false)
		}
		e.loan.LastDate = last
		e.loan.LastOK = true
	}
	e.veryLast = e.loan.LastDate

	// Balloons (known amount), sorted by date, 1-based with index 0 unused.
	var bs []dpBalloon
	for i := range input.Balloons {
		b := &input.Balloons[i]
		if b.AmountStatus >= types.InOutDefault && b.Amount != 0 {
			bs = append(bs, dpBalloon{date: b.Date, amount: b.Amount})
		}
	}
	sort.Slice(bs, func(i, j int) bool { return dateutil.DateComp(bs[i].date, bs[j].date) < 0 })
	e.balloons = make([]dpBalloon, len(bs)+1)
	copy(e.balloons[1:], bs)
	e.nballoons = len(bs)
	e.userNballoons = len(bs)
	e.nextBalloon = 1
	for _, b := range bs {
		if dateutil.DateComp(b.date, e.veryLast) > 0 {
			e.veryLast = b.date
		}
	}

	// Prepayment series, 1-based.
	var ps []dpPrepay
	for i := range input.Prepayments {
		pp := &input.Prepayments[i]
		if pp.PaymentStatus < types.InOutDefault || pp.StartDateStatus < types.InOutDefault {
			continue
		}
		dp := dpPrepay{nextdate: pp.StartDate, startdate: pp.StartDate, peryr: pp.PerYr, payment: pp.Payment}
		if pp.StopDateStatus >= types.InOutDefault {
			dp.stopdate, dp.stopOK = pp.StopDate, true
			if dateutil.DateComp(pp.StopDate, e.veryLast) > 0 {
				e.veryLast = pp.StopDate
			}
		}
		if pp.NNStatus >= types.InOutDefault {
			dp.nn, dp.nnOK = pp.NN, true
		}
		ps = append(ps, dp)
	}
	e.pres = make([]dpPrepay, len(ps)+1)
	copy(e.pres[1:], ps)
	e.npre = len(ps)

	// Adjustments (rate and/or amount), sorted by date, 1-based.
	var as []dpAdj
	for i := range input.Adjustments {
		a := &input.Adjustments[i]
		if a.DateStatus < types.InOutDefault {
			continue
		}
		da := dpAdj{date: a.Date}
		if a.LoanRateStatus >= types.InOutDefault {
			da.loanrate, da.rateOK = a.LoanRate, true
		}
		if a.AmtOK {
			da.amount, da.amtok = a.Amount, true
		}
		as = append(as, da)
	}
	sort.Slice(as, func(i, j int) bool { return dateutil.DateComp(as[i].date, as[j].date) < 0 })
	e.adjs = make([]dpAdj, len(as)+1)
	copy(e.adjs[1:], as)
	e.nadj = len(as)
	e.nextAdj = 1

	if input.Moratorium.FirstRepayStatus >= types.InOutDefault {
		e.morPresent = true
		e.morFirstRepay = input.Moratorium.FirstRepay
	}
	// targ.target: when a target is SET, the per-period payment is floored at
	// (target + interest). When NO target is set, the floor must be INERT — the
	// oracle negative-amortizes a low-payment balloon loan (balance grows, prin<0)
	// rather than flooring to interest-only, so the effective no-target value is
	// -infinity, NOT the literal 0 that ZeroTarget writes (Amortize.pas:82). Using
	// -inf makes `payamt-interest < targValue` never fire, matching DOS.
	if input.Target.TargetStatus >= types.InOutDefault {
		e.targValue = input.Target.TargetValue
	} else {
		e.targValue = -1e300
	}
	e.skipSet = input.SkipMonths.MonthSet

	e.f = GrowthPerPeriod(&e.loan, e.set.YrInv)
	e.truerate, _ = ComputeTrueRate(&e.loan, &e.set)
	return e
}

// estimateAndRefinePayment mirrors Amortize.pas:377-430: analytic annuity seed
// over the balloon-netted balance, then Iterate(til_adj) to refine. Sets e.d.
func (e *dosEng) estimateAndRefinePayment() bool {
	p := e.loan.Amount
	usap := 0.0
	adjp := e.loan.Amount
	rate, _ := interest.RateFromYield(e.loan.LoanRate, byte(e.loan.PerYr), e.set.YrDays)
	for i := 1; i <= e.userNballoons; i++ {
		yd := dateutil.YearsDif(e.balloons[i].date, e.loan.LoanDate, e.set.Basis, e.set.YrInv, false)
		disc, _ := interest.Exxp(-rate * yd)
		adjp -= e.balloons[i].amount * disc
	}
	// (Prepayment seed terms omitted — Iterate refines; the fuzzer does not
	// generate prepayment series. TODO: port FirstLastAndFF for completeness.)
	e.d = annuityPayment(adjp, e.f, e.loan.NPeriods)
	return e.iterate(p, usap, e.loan.LoanDate, e.loan.FirstDate, &e.d, false, false)
}

// AmortizeDOS is the faithful-port entry: it mirrors the MakeTable flow — solve
// the blank payment (EstimateAndRefinePayment) when one is unknown, then build
// the schedule with RepayFancyLoan(entire). It is the parallel engine validated
// against the oracle; the production Amortize remains the default.
func AmortizeDOS(input LoanInput) AmortResult {
	e := buildDosEng(input)

	// hard_payment is true only for a USER-GIVEN payment (per-period interest
	// rounding); a solved payment runs unrounded (Iterate sets hard_payment=false,
	// AMORTIZE.pas:1496).
	if input.Loan.PayAmtStatus >= types.InOutDefault {
		e.d = input.Loan.PayAmt
		e.hardPay = true
	} else {
		e.hardPay = false
		if !e.estimateAndRefinePayment() {
			return AmortResult{Err: errNoConverge}
		}
	}

	p := e.loan.Amount
	usap := 0.0
	e.hardPay = input.Loan.PayAmtStatus >= types.InOutDefault
	rows := e.repayFancyLoan(&p, &usap, e.loan.LoanDate, e.loan.FirstDate, true, true, 0)

	var res AmortResult
	cumInt := 0.0
	for i, r := range rows {
		cumInt += r.interest
		res.Schedule = append(res.Schedule, PaymentRecord{
			PayNum:    i + 1,
			Date:      r.date,
			PayAmt:    r.payamt,
			Interest:  r.interest,
			Principal: r.principal,
			IntToDate: cumInt,
		})
		res.TotalPaid += r.payamt
		res.TotalInt += r.interest
	}
	if len(res.Schedule) > 0 {
		res.FinalPrinc = res.Schedule[len(res.Schedule)-1].Principal
	}
	res.NPeriods = e.loan.NPeriods
	res.FirstDate = e.loan.FirstDate
	res.LastDate = e.loan.LastDate
	return res
}
