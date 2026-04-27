// Package interest provides core financial math functions ported from the
// legacy Delphi/Pascal INTSUTIL.pas module. These include safe exponential,
// logarithm, and square root functions with Taylor series approximations
// for values near singularities, as well as rate/yield conversions.
//
// All functions preserve the exact numerical behavior of the original Pascal
// code to ensure financial calculation fidelity.
//
// Ported from legacy/source/INTSUTIL.pas
package interest

import (
	"fmt"
	"math"

	"github.com/persense/persense-port/internal/types"
)

// ErrOverflow is returned when a calculation overflows.
var ErrOverflow = fmt.Errorf("overflow: answer too large")

// ErrInconsistent is returned when input data is internally inconsistent.
var ErrInconsistent = fmt.Errorf("inconsistent data")

// ErrTimeTooLong is returned when a time period exceeds the allowed maximum.
var ErrTimeTooLong = fmt.Errorf("time period too long")

// --- Safe math functions ---

// Exxp computes e^x with overflow protection and a Taylor series
// approximation for |x| < 1e-4 (the "small" constant).
//
// The Taylor series avoids precision loss that occurred in the original
// Turbo Pascal compiler for values very close to zero.
//
// Ported from legacy/source/INTSUTIL.pas: function exxp
func Exxp(x float64) (float64, error) {
	const sixth = 1.0 / 6.0

	if x > 70 {
		return 0, ErrOverflow
	}
	if x < -70 {
		return 1e-32, nil
	}
	if math.Abs(x) < types.Small {
		// Taylor series: 1 + x + x²/2 + x³/6
		x2 := x * x
		return 1 + x + types.Half*x2 + sixth*x*x2, nil
	}
	return math.Exp(x), nil
}

// Lnn computes ln(x) with error protection and a Taylor series
// approximation for |x-1| < 1e-4.
//
// The Taylor series compensates for a known Turbo Pascal compiler bug
// where ln(x) lost precision for x very close to 1.
//
// Ported from legacy/source/INTSUTIL.pas: function lnn
func Lnn(x float64) (float64, error) {
	const third = 1.0 / 3.0

	if x <= 0 {
		return 0, ErrInconsistent
	}
	if math.Abs(x-1) < types.Small {
		// Taylor series: t - t²/2 + t³/3
		t := x - 1
		t2 := t * t
		return t - types.Half*t2 + third*t*t2, nil
	}
	return math.Log(x), nil
}

// Sqrrt computes sqrt(x) with error protection for negative values.
// Small negative values (> -1e-10) are treated as zero.
//
// Ported from legacy/source/INTSUTIL.pas: function sqrrt
func Sqrrt(x float64) (float64, error) {
	if x < 0 {
		if x < -types.Teeny {
			return 0, ErrInconsistent
		}
		return 0, nil
	}
	return math.Sqrt(x), nil
}

// Power computes x^n using logarithms: exp(n * ln(x)).
// Returns 0 for x <= 0.
//
// Ported from legacy/source/INTSUTIL.pas: function Power
func Power(x, n float64) (float64, error) {
	if x <= 0 {
		return 0, nil
	}
	lnx, err := Lnn(x)
	if err != nil {
		return 0, err
	}
	return Exxp(n * lnx)
}

// QuadraticFormula solves Ax² + Bx + C = 0, returning the more negative root:
//
//	(-B - sqrt(B² - 4AC)) / 2A
//
// Ported from legacy/source/INTSUTIL.pas: function QuadraticFormula
func QuadraticFormula(a, b, c float64) (float64, error) {
	discriminant := b*b - 4*a*c
	sq, err := Sqrrt(discriminant)
	if err != nil {
		return 0, err
	}
	return (-b - sq) / (2 * a), nil
}

// Round2 rounds a monetary value to 2 decimal places using the
// original Pascal rounding convention (add halfpenny then truncate).
//
// Note: The original uses 0.005 - teeny as the halfpenny to avoid
// the exact-half ambiguity. This produces round-half-down behavior.
//
// Ported from legacy/source/INTSUTIL.pas: procedure Round2
func Round2(x float64) float64 {
	const halfpenny = 0.005 - types.Teeny
	if x > 0 {
		x += halfpenny
	} else {
		x -= halfpenny
	}
	return math.Trunc(x*100) / 100
}

// OK returns true if v is a valid numeric value (not a sentinel).
// Sentinel values are: unk (-8888), blank (-7777), error (-8888).
//
// Ported from legacy/source/INTSUTIL.pas: function ok
func OK(v float64) bool {
	return v != float64(types.Unk) && v != float64(types.Blank) && v != float64(types.ErrorVal)
}

// Floor returns the largest integer <= x, matching the original Pascal
// implementation which uses trunc() with special handling for negatives.
//
// Ported from legacy/source/INTSUTIL.pas: function floor
func Floor(x float64) int64 {
	if x > 0 {
		return int64(x)
	}
	tr := int64(x)
	if float64(tr) == x {
		return tr
	}
	return tr - 1
}
