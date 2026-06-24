// Package presentvalue implements present value calculations ported from the
// legacy Delphi/Pascal PRESVALU.pas and PVLUTIL.pas modules.
//
// The present value screen computes the discounted value of a stream of
// payments (both lump-sum and periodic) at a given interest rate, as of a
// specified date. It supports:
//   - Multiple lump-sum (one-time) payments
//   - Multiple series of periodic payments with COLA adjustments
//   - Forward calculation (rate+date → value)
//   - Backward calculation (value → unknown rate, date, or amount)
//   - Simple and compound interest modes
//   - Multiple rate entries with date ranges (fancy/extended mode)
//
// Ported from legacy/source/PRESVALU.pas and legacy/source/PVLUTIL.pas
package presentvalue

import (
	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

const (
	teeny = types.Teeny
	tiny  = types.Tiny
	half  = types.Half
)

// RateEntry holds an interest rate with its effective date.
// Ported from legacy/source/PETYPES.PAS: raterec
type RateEntry struct {
	Status int8
	Rate   float64 // continuously compounded rate
	PerYr  byte    // compounding frequency when entered
}

// LumpSumPayment represents a single one-time payment.
// Ported from legacy/source/PETYPES.PAS: lumpsum record
type LumpSumPayment struct {
	DateStatus int8
	Date       types.DateRec
	AmtStatus  int8
	Amt        float64 // payment amount
	ValStatus  int8
	Val        float64 // present value
	Status     int     // composite status code
	Act        byte    // actuarial contingency type (0=NotContingent)
	Prob       float64 // computed survival probability (output, 0-1)
}

// PeriodicPayment represents a series of periodic payments.
// Ported from legacy/source/PETYPES.PAS: periodic record
type PeriodicPayment struct {
	FromDateStatus int8
	FromDate       types.DateRec
	ToDateStatus   int8
	ToDate         types.DateRec
	PerYrStatus    int8
	PerYr          int
	AmtStatus      int8
	Amt            float64 // payment amount per period
	COLAStatus     int8
	COLA           float64 // cost of living adjustment (continuous rate)
	ValStatus      int8
	Val            float64 // present value of the series
	Status         int
	NInstallments  int     // computed number of installments
	Act            byte    // actuarial contingency type (0=NotContingent)
	Prob           float64 // average survival probability (output)
	// Installments carries the per-payment-date breakdown the DOS table
	// prints under a life contingency (pvltable.pas PrintNextPayment): one
	// entry per scheduled payment with its own survival probability. It is
	// populated only when the row is life-contingent (Act != NotContingent),
	// matching DOS, which walks per-payment only when fold_in_life forces the
	// exact summation method. Prob above remains the stream average.
	Installments []PeriodicInstallment
}

// PeriodicInstallment is one scheduled payment of a life-contingent periodic
// stream. It mirrors the per-row figures the DOS PVL table prints
// (pvltable.pas:514-533): the payment date, the "if paid" value (discounted to
// the as-of date but NOT survival-weighted — DOS `ifpd`), the survival
// probability at that date (DOS `prob`, equal to LifeProb(t) and recovered in
// DOS as v/ifpd), and the survival-weighted present value (DOS `v` = ifpd*prob).
type PeriodicInstallment struct {
	Date   types.DateRec
	IfPaid float64 // discounted value ignoring survival (DOS ifpd)
	Prob   float64 // survival probability at this date (DOS prob = LifeProb(t))
	Value  float64 // survival-weighted present value = IfPaid * Prob (DOS v)
}

// PresValLine represents a present value summary/rate line.
// Ported from legacy/source/PETYPES.PAS: presval record
type PresValLine struct {
	AsOfStatus     int8
	AsOf           types.DateRec
	R              RateEntry
	SumValueStatus int8
	SumValue       float64 // total present value
	Status         int
	DurationStatus int8
	Duration       float64
}

// PVSettings holds computational settings for present value calculations.
type PVSettings struct {
	Basis     types.BasisType
	PerYr     byte // default compounding frequency
	COLAMonth byte // ANN(99), CNT(98), or 1-12
	Exact     bool // exact calculation mode
	YrDays    float64
	YrInv     float64
}

// RateLine is one entry in a variable-rate schedule. Each entry marks
// the date a new rate takes effect; that rate stays in force until the
// next entry's Date (or forever, for the last entry). The first
// entry's Date is conceptually "from the beginning of time" — its
// rate is the starting rate regardless of what's stored there.
//
// Ported from legacy/src/dos_source/PETYPES.PAS line 622: rateline.
// The DOS app stored a tri-rate record (true/loan/yield) but they're
// all derivable from each other; we keep only the continuous true
// rate here and let the API/UI translate at the boundary.
type RateLine struct {
	Date types.DateRec
	Rate float64 // continuously-compounded true rate
}

// PVInput bundles all inputs for a present value calculation.
type PVInput struct {
	LumpSums  []LumpSumPayment
	Periodics []PeriodicPayment
	PresVal   PresValLine // rate and as-of date
	Settings  PVSettings
	Actuarial *actuarial.ActuarialConfig // nil = no life contingency

	// RateSchedule, when non-empty, switches the engine into
	// variable-rate (DOS "PVL fancy") mode. PresVal.Rate is ignored
	// and the schedule is used for all discounting. Backward calc is
	// not supported in this mode (matches DOS: "rates cannot be the
	// target of a computation"). See docs/requirements.md §2.3.5.
	RateSchedule []RateLine
}

// PVResult holds the output of a present value calculation.
type PVResult struct {
	LumpSums  []LumpSumPayment  // with computed values filled in
	Periodics []PeriodicPayment // with computed values filled in
	SumValue  float64           // total present value
	PODValue  float64           // payment on death value (actuarial only)
	Err       error
	// Warnings carries non-fatal advisories surfaced during the
	// calculation (e.g. an over-specified row). Empty on a clean run.
	Warnings []string
	// Rate and AsOf echo the discount rate and as-of date the
	// calculation actually used. They matter for the backward solves
	// PV-8 (rate unknown) and PV-9 (as-of date unknown): the solved
	// value is carried back here so the caller can display it.
	Rate float64
	AsOf types.DateRec
	// POD carries the solved Payment-on-Death amount when the
	// actuarial config left it unknown (DOS ComputeUnknownPOD).
	POD float64
}

// --- Zero/Empty functions ---

// ZeroLumpSum initializes a LumpSumPayment to empty.
// Ported from legacy/source/PRESVALU.pas: procedure ZeroLumpSum
func ZeroLumpSum(l *LumpSumPayment) {
	*l = LumpSumPayment{Date: types.UnknownDate()}
}

// LumpSumIsEmpty returns true if the lump sum has no data.
// Ported from legacy/source/PRESVALU.pas: function LumpSumIsEmpty
func LumpSumIsEmpty(l *LumpSumPayment) bool {
	return l.DateStatus == types.StatusEmpty &&
		l.AmtStatus == types.StatusEmpty &&
		l.ValStatus == types.StatusEmpty
}

// ZeroPeriodic initializes a PeriodicPayment to empty.
// Ported from legacy/source/PRESVALU.pas: procedure ZeroPeriodic
func ZeroPeriodic(p *PeriodicPayment) {
	*p = PeriodicPayment{
		FromDate: types.UnknownDate(),
		ToDate:   types.UnknownDate(),
	}
}

// PeriodicIsEmpty returns true if the periodic payment has no data.
// Ported from legacy/source/PRESVALU.pas: function PeriodicIsEmpty
func PeriodicIsEmpty(p *PeriodicPayment) bool {
	return p.FromDateStatus == types.StatusEmpty &&
		p.ToDateStatus == types.StatusEmpty &&
		p.PerYrStatus == types.StatusEmpty &&
		p.AmtStatus == types.StatusEmpty &&
		p.COLAStatus == types.StatusEmpty &&
		p.ValStatus == types.StatusEmpty
}

// ZeroPresValLine initializes a PresValLine to empty.
// Ported from legacy/source/PRESVALU.pas: procedure ZeroPresVal
func ZeroPresValLine(p *PresValLine) {
	*p = PresValLine{AsOf: types.UnknownDate()}
}

// PresValLineIsEmpty returns true if the present value line has no data.
// Ported from legacy/source/PRESVALU.pas: function PresValIsEmpty
func PresValLineIsEmpty(p *PresValLine) bool {
	return p.AsOfStatus == types.StatusEmpty &&
		p.R.Status == types.StatusEmpty &&
		p.SumValueStatus == types.StatusEmpty &&
		p.DurationStatus == types.StatusEmpty
}
