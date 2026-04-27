// Package amortization implements loan amortization calculations ported from
// the legacy Delphi/Pascal Amortize.pas and AMORTOP.pas modules.
//
// The amortization engine supports:
//   - Standard and Rule-of-78 amortization
//   - Prepaid interest
//   - Multiple balloon payments
//   - Rate/payment adjustments (ARMs)
//   - Extra/skipped prepayments
//   - Moratorium (interest-only) periods
//   - Targeted principal reduction
//   - Skip-month schedules
//   - Daily, Canadian, and standard compounding
//   - US Rule interest calculations
//   - 360/365/365-360 day count conventions
//
// All monetary values use float64 internally to match the original Pascal real
// type and preserve exact numerical behavior of iterative algorithms.
//
// Ported from legacy/source/Amortize.pas and legacy/source/AMORTOP.pas
package amortization

import (
	"github.com/persense/persense-port/internal/types"
)

const (
	minPmt = 1.0 // minimum meaningful payment amount
	teeny  = types.Teeny
	tiny   = types.Tiny
	small  = types.Small
)

// Loan holds the top-level amortization loan parameters.
// Mirrors the Pascal AMZLoan record + supporting global state.
//
// Ported from legacy/source/PETYPES.PAS: AMZLoan record
type Loan struct {
	AmountStatus   int8
	Amount         float64 // loan principal
	LoanDateStatus int8
	LoanDate       types.DateRec
	LoanRateStatus int8
	LoanRate       float64 // nominal interest rate (as yield at peryr)
	FirstStatus    int8
	FirstDate      types.DateRec // date of first regular payment
	NStatus        int8
	NPeriods       int // number of regular payment periods
	LastStatus     int8
	LastDate       types.DateRec // date of last regular payment
	PerYrStatus    int8
	PerYr          int // payments per year
	PayAmtStatus   int8
	PayAmt         float64 // regular payment amount
	PointsStatus   int8
	Points         float64 // points charge for APR
	APRStatus      int8
	APR            float64 // computed APR

	LastOK bool // whether last date is valid/computed
}

// BalloonPayment represents a lump-sum payment at a specific date.
// Ported from legacy/source/PETYPES.PAS: balloonrec
type BalloonPayment struct {
	DateStatus   int8
	Date         types.DateRec
	AmountStatus int8
	Amount       float64
}

// RateAdjustment represents a rate or payment change on a specific date.
// Ported from legacy/source/PETYPES.PAS: adjrec
type RateAdjustment struct {
	DateStatus     int8
	Date           types.DateRec
	LoanRateStatus int8
	LoanRate       float64
	AmountStatus   int8
	Amount         float64
	AmtOK          bool // whether amount was user-specified
}

// Prepayment represents a series of extra (or skipped) payments.
// Ported from legacy/source/PETYPES.PAS: prepaymentrec
type Prepayment struct {
	StartDateStatus int8
	StartDate       types.DateRec
	NNStatus        int8
	NN              int // number of extra payments
	StopDateStatus  int8
	StopDate        types.DateRec
	PerYrStatus     int8
	PerYr           int
	PaymentStatus   int8
	Payment         float64 // amount per extra payment (0 = skip)
	NextDate        types.DateRec
}

// Moratorium represents an interest-only deferment period.
// Ported from legacy/source/PETYPES.PAS: moratoriumrec
type Moratorium struct {
	FirstRepayStatus int8
	FirstRepay       types.DateRec
}

// Target represents a minimum principal reduction per payment.
// Ported from legacy/source/PETYPES.PAS: targetrec
type Target struct {
	TargetStatus int8
	TargetValue  float64
}

// SkipMonths represents months in which payments are skipped.
// Ported from legacy/source/PETYPES.PAS: skiprec
type SkipMonths struct {
	SkipStatus int8
	SkipStr    string    // e.g. "6-8" or "1,6,12"
	MonthSet   [13]bool  // parsed: MonthSet[m] = true if month m is skipped
}

// Settings holds the computational settings that affect amortization.
// These replace the global df.c and related variables from Pascal.
type Settings struct {
	Basis       types.BasisType
	PerYr       byte    // default compounding frequency from settings
	Prepaid     bool    // prepaid interest
	InAdvance   bool    // payments in advance
	PlusRegular bool    // balloon includes regular payment
	Exact       bool    // exact interest calculations
	R78         bool    // Rule of 78 amortization
	USARule     bool    // US Rule for interest
	YrDays      float64 // days per year
	YrInv       float64 // 1/yrdays
	CenturyDiv  int
	Daily       bool    // daily compounding mode
}

// LoanInput bundles all the data needed to compute an amortization.
type LoanInput struct {
	Loan       Loan
	Balloons   []BalloonPayment
	Adjustments []RateAdjustment
	Prepayments []Prepayment
	Moratorium  Moratorium
	Target      Target
	SkipMonths  SkipMonths
	Settings    Settings
	Fancy       bool // whether advanced (fancy) mode is active
}

// PaymentRecord represents one line of an amortization schedule.
type PaymentRecord struct {
	PayNum    int
	Date      types.DateRec
	PayAmt    float64 // total payment this period (incl. extras)
	Interest  float64 // interest portion
	Principal float64 // remaining principal after this payment
	IntToDate float64 // cumulative interest to date
}

// AmortResult holds the full output of an amortization calculation.
type AmortResult struct {
	Schedule     []PaymentRecord
	FinalPrinc   float64 // final remaining principal (should be ~0)
	TotalPaid    float64 // sum of all payments
	TotalInt     float64 // sum of all interest
	APR          float64 // computed APR (if points specified)
	APRConverged bool
	Err          error
}

// --- Zero/Empty functions ---

// ZeroLoan initializes a Loan to empty/zero.
// Ported from legacy/source/Amortize.pas: procedure ZeroAMZLoan
func ZeroLoan(l *Loan) {
	*l = Loan{
		LoanDate:  types.UnknownDate(),
		FirstDate: types.UnknownDate(),
		LastDate:  types.UnknownDate(),
	}
}

// ZeroBalloon initializes a BalloonPayment to empty.
// Ported from legacy/source/Amortize.pas: procedure ZeroBalloon
func ZeroBalloon(b *BalloonPayment) {
	*b = BalloonPayment{Date: types.UnknownDate()}
}

// BalloonIsEmpty returns true if the balloon has no data.
// Ported from legacy/source/Amortize.pas: function BalloonIsEmpty
func BalloonIsEmpty(b *BalloonPayment) bool {
	return b.DateStatus == types.StatusEmpty && b.AmountStatus == types.StatusEmpty
}

// ZeroAdjustment initializes a RateAdjustment to empty.
// Ported from legacy/source/Amortize.pas: procedure ZeroAdjustment
func ZeroAdjustment(a *RateAdjustment) {
	*a = RateAdjustment{Date: types.UnknownDate()}
}

// AdjustmentIsEmpty returns true if the adjustment has no data.
// Ported from legacy/source/Amortize.pas: function AdjustmentIsEmpty
func AdjustmentIsEmpty(a *RateAdjustment) bool {
	return a.DateStatus == types.StatusEmpty &&
		a.LoanRateStatus == types.StatusEmpty &&
		a.AmountStatus == types.StatusEmpty
}

// ZeroPrepayment initializes a Prepayment to empty.
// Ported from legacy/source/Amortize.pas: procedure ZeroPrepayment
func ZeroPrepayment(p *Prepayment) {
	*p = Prepayment{
		StartDate: types.UnknownDate(),
		StopDate:  types.UnknownDate(),
		NextDate:  types.UnknownDate(),
	}
}

// PrepaymentIsEmpty returns true if the prepayment has no data.
// Ported from legacy/source/Amortize.pas: function PrepaymentIsEmpty
func PrepaymentIsEmpty(p *Prepayment) bool {
	return p.StartDateStatus == types.StatusEmpty &&
		p.NNStatus == types.StatusEmpty &&
		p.StopDateStatus == types.StatusEmpty &&
		p.PerYrStatus == types.StatusEmpty &&
		p.PaymentStatus == types.StatusEmpty
}

// ZeroMoratorium initializes a Moratorium to empty.
// Ported from legacy/source/Amortize.pas: procedure ZeroMoratorium
func ZeroMoratorium(m *Moratorium) {
	*m = Moratorium{FirstRepay: types.UnknownDate()}
}

// ZeroTarget initializes a Target to empty.
// Ported from legacy/source/Amortize.pas: procedure ZeroTarget
func ZeroTarget(t *Target) {
	*t = Target{}
}

// ZeroSkipMonths initializes a SkipMonths to empty.
// Ported from legacy/source/Amortize.pas: procedure ZeroSkip
func ZeroSkipMonths(s *SkipMonths) {
	*s = SkipMonths{}
}
