package mortgage

import (
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// These tests drive the interest.Exxp overflow-propagation guards that the
// mortgage math functions carry. Exxp errors only for arguments > 70, which in
// practice requires pathological (huge or negative) rates that never occur in a
// real mortgage — but the guards are real error paths, so we exercise them with
// extreme rates rather than leave them untested.

func TestSummation_OverflowGuards(t *testing.T) {
	// Negative large rate: Exxp(-r*t) = Exxp(+large) overflows ('last').
	if _, err := Summation(-10, 30); err == nil {
		t.Error("Summation(-10,30) should overflow")
	}
	// last ok but Exxp(-r/12) overflows ('f'): r very negative, t tiny.
	if _, err := Summation(-1000, 0.05); err == nil {
		t.Error("Summation(-1000,0.05) should overflow on f")
	}
}

func TestTerminalBalloon_OverflowGuards(t *testing.T) {
	b := computedBalloon(0.06, 30, 10, 50000)
	// Positive huge rate -> expRt = Exxp(r*t) overflows.
	hot := b
	hot.Rate = 10
	if _, err := TerminalBalloon(&hot, 30); err == nil {
		t.Error("TerminalBalloon huge +rate should overflow")
	}
	// Negative huge rate -> Summation overflows first.
	cold := b
	cold.Rate = -10
	if _, err := TerminalBalloon(&cold, 30); err == nil {
		t.Error("TerminalBalloon huge -rate should error in Summation")
	}
}

func TestVPFTL_OverflowGuards(t *testing.T) {
	b := computedBalloon(0.06, 30, 10, 50000)
	// Negative huge rate -> Summation overflows.
	cold := b
	cold.Rate = -10
	if _, err := ValueOfPaymentsForTerminatedLoan(&cold, -10, 30); err == nil {
		t.Error("VPFTL -rate should error")
	}
	// Positive huge line rate -> TerminalBalloon (which uses ei.Rate) overflows.
	hot := b
	hot.Rate = 10
	if _, err := ValueOfPaymentsForTerminatedLoan(&hot, 10, 15); err == nil {
		t.Error("VPFTL +rate should error in TerminalBalloon")
	}
}

func TestOneMonthAPR_OverflowGuard(t *testing.T) {
	b := computedBasic(0.06, 30)
	b.Rate = 1000
	if _, err := OneMonthAPR(&b, yrdays360); err == nil {
		t.Error("OneMonthAPR huge rate should overflow")
	}
}

func TestBalloonCalc_OverflowGuards(t *testing.T) {
	base := computedBalloon(0.06, 30, 10, 50000)
	// Negative huge rate -> Summation overflows.
	cold := base
	cold.Rate = -10
	if err := balloonCalc(&cold, 0); err == nil {
		t.Error("balloonCalc -rate should error in Summation")
	}
	// Positive huge rate -> Exxp(rate*when) overflows.
	hot := base
	hot.Rate = 10
	if err := balloonCalc(&hot, 0); err == nil {
		t.Error("balloonCalc +rate should error in Exxp")
	}
}

func TestIterateToFindAPR_OverflowGuard(t *testing.T) {
	b := computedBasic(0.06, 30)
	b.Rate = 100 // huge -> ValueOfPaymentsForTerminatedLoan overflows
	if _, _, err := IterateToFindAPR(b, 30, yrdays360); err == nil {
		t.Error("IterateToFindAPR huge rate should error")
	}
}

func TestCalc_InternalOverflowGuards(t *testing.T) {
	// Balloon-known with a hugely negative rate forces the balloonval Exxp
	// guard (Exxp(-rate*when) = Exxp(+large)) inside Calc.
	res := Calc(MtgLine{
		PriceStatus: inp(), Price: 200000, PctStatus: inp(), Pct: 0.2,
		YearsStatus: inp(), Years: 30, RateStatus: inp(), Rate: -10,
		TaxStatus: inp(), WhenStatus: inp(), When: 30,
		HowMuchStatus: inp(), HowMuch: 50000,
	})
	if res.Err == nil {
		t.Error("Calc with balloon and huge -rate should error on balloonval Exxp")
	}
	// Monthly-from-price with a hugely negative rate forces the Summation
	// guard on the monthly branch.
	res2 := Calc(MtgLine{
		PriceStatus: inp(), Price: 200000, PctStatus: inp(), Pct: 0.2,
		YearsStatus: inp(), Years: 30, RateStatus: inp(), Rate: -10,
		TaxStatus: inp(), BalloonStat: types.BalloonBlank,
	})
	if res2.Err == nil {
		t.Error("Calc monthly-from-price with huge -rate should error in Summation")
	}
}
