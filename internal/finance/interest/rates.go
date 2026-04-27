package interest

import (
	"strings"

	"github.com/persense/persense-port/internal/types"
)

// CalcContext holds the settings needed for interest calculations.
// It replaces the global variables df, yrdays, yrinv from the Pascal code.
type CalcContext struct {
	Basis  types.BasisType  // day-count convention
	PerYr  byte             // default compounding frequency (from settings)
	YrDays float64          // days per year (360 or 365.25)
	YrInv  float64          // 1 / YrDays
}

// NewCalcContext creates a CalcContext with yrdays/yrinv computed from the basis.
//
// Ported from legacy/source/INTSUTIL.pas: procedure SetYrDays
func NewCalcContext(basis types.BasisType, peryr byte) CalcContext {
	var yrdays float64
	if basis == types.Basis365 {
		yrdays = 365.25
	} else {
		yrdays = 360
	}
	return CalcContext{
		Basis:  basis,
		PerYr:  peryr,
		YrDays: yrdays,
		YrInv:  1.0 / yrdays,
	}
}

// RealPerYr returns the effective number of compounding periods per year.
// For special compounding modes (daily, weekly, biweekly), it converts
// to the actual number of periods. For Canadian compounding, it strips
// the Canadian flag to get the underlying frequency.
//
// Ported from legacy/source/INTSUTIL.pas: function RealPerYr
func RealPerYr(n byte, yrdays float64) float64 {
	switch n {
	case types.CompoundingDaily:
		return yrdays
	case 52:
		return yrdays / 7
	case 26:
		return yrdays / 14
	default:
		// Strip Canadian flag if present: n AND (NOT canadian)
		return float64(n & ^types.CompoundingCanadian)
	}
}

// YieldFromRate converts a true (continuously compounded) rate to a yield
// (effective rate) for a given compounding frequency n.
//
// Formula: yield = nn * (exp(rate/nn) - 1)
// where nn = RealPerYr(n)
//
// Ported from legacy/source/INTSUTIL.pas: function YieldFromRate
func YieldFromRate(rr float64, n byte, yrdays float64) (float64, error) {
	nn := RealPerYr(n, yrdays)
	expVal, err := Exxp(rr / nn)
	if err != nil {
		return 0, err
	}
	return nn * (expVal - 1), nil
}

// RateFromYield converts a yield (effective rate) to a true
// (continuously compounded) rate for a given compounding frequency n.
//
// Formula: rate = nn * ln(1 + yield/nn)
// where nn = RealPerYr(n)
//
// Ported from legacy/source/INTSUTIL.pas: function RateFromYield
func RateFromYield(yy float64, n byte, yrdays float64) (float64, error) {
	nn := RealPerYr(n, yrdays)
	lnVal, err := Lnn(1 + yy/nn)
	if err != nil {
		return 0, err
	}
	return nn * lnVal, nil
}

// ReportedRate converts an internal APR to the rate as displayed to the user,
// accounting for Canadian or daily compounding modes.
//
// When Canadian or daily compounding is in effect, the rate is converted
// from the loan's payment frequency to the display compounding frequency.
// Otherwise, the rate passes through unchanged.
//
// Parameters:
//   - apr: the internal APR value
//   - loanPerYr: the loan's payment frequency (e.g. h^.peryr)
//   - settingsPerYr: the global settings compounding frequency (df.c.peryr)
//   - yrdays: days per year for RealPerYr
//
// Ported from legacy/source/INTSUTIL.pas: function ReportedRate
func ReportedRate(apr float64, loanPerYr, settingsPerYr byte, yrdays float64) (float64, error) {
	if settingsPerYr&(types.CompoundingCanadian|types.CompoundingDaily) > 0 {
		// Convert: APR → yield at loan freq → rate at settings freq → yield at settings freq
		rfy, err := RateFromYield(apr, loanPerYr, yrdays)
		if err != nil {
			return 0, err
		}
		return YieldFromRate(rfy, settingsPerYr, yrdays)
	}
	return apr, nil
}

// InterpretedRate converts a user-input rate to the internal rate,
// accounting for Canadian or daily compounding. This is the inverse
// of ReportedRate.
//
// Ported from legacy/source/INTSUTIL.pas: function InterpretedRate
func InterpretedRate(inputrate float64, loanPerYr, settingsPerYr byte, yrdays float64) (float64, error) {
	rfy, err := RateFromYield(inputrate, settingsPerYr, yrdays)
	if err != nil {
		return 0, err
	}
	return YieldFromRate(rfy, loanPerYr, yrdays)
}

// PerYrString returns a human-readable description of a compounding frequency.
//
// The capitalization parameter controls casing:
//
//	0 = lowercase ("monthly")
//	1 = capitalize first letter ("Monthly")
//	2 = all uppercase ("MONTHLY")
//
// Ported from legacy/source/INTSUTIL.pas: function PerYrString
func PerYrString(peryr byte, capitalization byte) string {
	var ws string
	switch peryr {
	case 1:
		ws = "yearly"
	case 2:
		ws = "semi-annually"
	case 3:
		ws = "thrice-annually"
	case 4:
		ws = "quarterly"
	case 6:
		ws = "bi-monthly"
	case 12:
		ws = "monthly"
	case 24:
		ws = "twice-monthly"
	case 26:
		ws = "biweekly"
	case 52:
		ws = "weekly"
	default:
		ws = "unknown"
	}

	switch capitalization {
	case 1:
		if len(ws) > 0 {
			ws = strings.ToUpper(ws[:1]) + ws[1:]
		}
	case 2:
		ws = strings.ToUpper(ws)
	}
	return ws
}
