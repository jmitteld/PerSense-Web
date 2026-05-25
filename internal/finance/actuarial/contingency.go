package actuarial

import (
	"math"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// Contingency type constants.
// Ported from legacy/src/dos_source/PETYPES.PAS lines 169-171.
const (
	NotContingent = 0 // Payment always made (no life table adjustment)
	Living        = 1 // Payment only if person 1 is alive
	Dead          = 2 // Payment only if person 1 is deceased
	Only1Living   = 3 // Only person 1 is alive (person 2 is dead)
	Only2Living   = 4 // Only person 2 is alive (person 1 is dead)
	EitherLiving  = 5 // At least one person is alive
	BothLiving    = 6 // Both persons are alive
)

// ContingencyLabel returns a human-readable label for a contingency type.
func ContingencyLabel(c byte) string {
	switch c {
	case NotContingent:
		return "None"
	case Living:
		return "Living"
	case Dead:
		return "Deceased"
	case Only1Living:
		return "Only 1 Living"
	case Only2Living:
		return "Only 2 Living"
	case EitherLiving:
		return "Either Living"
	case BothLiving:
		return "Both Living"
	default:
		return "Unknown"
	}
}

// ContingencyFromCode parses a single-character code into a contingency type.
// Ported from legacy/src/dos_source/PEDATA.pas line 144:
// actchar: array[NOT_CONTINGENT..BOTH_LIVING] of char = ('N','L','D','1','2','E','B');
func ContingencyFromCode(code string) byte {
	switch code {
	case "L":
		return Living
	case "D":
		return Dead
	case "1":
		return Only1Living
	case "2":
		return Only2Living
	case "E":
		return EitherLiving
	case "B":
		return BothLiving
	default:
		return NotContingent
	}
}

// ActuarialConfig holds the configuration for life-contingent present value
// calculations.
//
// Reconstructed from PEDATA.pas global variables (lines 288-293) and the
// ActuarialBlock column definitions in PETYPES.PAS (lines 243-255).
type ActuarialConfig struct {
	Table1 *LifeTable    // Life table for person 1
	DOB1   types.DateRec // Date of birth, person 1
	Table2 *LifeTable    // Life table for person 2 (optional, for two-life contingencies)
	DOB2   types.DateRec // Date of birth, person 2
	Now    types.DateRec // Current/reference date (for alive/dead at "now")
	POD    float64       // Payment on Death amount (optional)
	// PODUnknown marks the Payment-on-Death amount as the field to
	// solve for: given a target Sum Value the engine back-solves POD
	// (DOS ComputeUnknownPOD). When set, POD is ignored as an input.
	PODUnknown bool
}

// yearsDif computes years between two dates using 360-day basis.
// Returns positive when 'to' is after 'from'.
// Note: dateutil.YearsDif(z, a, ...) computes years from a to z,
// so we pass (to, from) to get positive when to > from.
func yearsDif(from, to types.DateRec) float64 {
	return dateutil.YearsDif(to, from, types.Basis360, 1.0/360.0, false)
}

// ageAtDate returns the fractional age of a person born on dob at the given date.
func ageAtDate(dob, date types.DateRec) float64 {
	return yearsDif(dob, date)
}

// survivalProb1 returns the survival probability for person 1 at the given date,
// conditional on being alive at the reference date (Now).
func (c *ActuarialConfig) survivalProb1(date types.DateRec) float64 {
	if c.Table1 == nil {
		return 1.0
	}
	ageNow := ageAtDate(c.DOB1, c.Now)
	ageAtPayment := ageAtDate(c.DOB1, date)
	return c.Table1.ConditionalSurvival(ageNow, ageAtPayment)
}

// survivalProb2 returns the survival probability for person 2 at the given date,
// conditional on being alive at the reference date (Now).
func (c *ActuarialConfig) survivalProb2(date types.DateRec) float64 {
	if c.Table2 == nil {
		return 1.0
	}
	ageNow := ageAtDate(c.DOB2, c.Now)
	ageAtPayment := ageAtDate(c.DOB2, date)
	return c.Table2.ConditionalSurvival(ageNow, ageAtPayment)
}

// LifeProb computes the probability that the contingency condition is met
// at the given date.
//
// Reconstructed from the calling patterns in PRESVALU.pas (lines 212, 221,
// 248, 255, 297, 326, 397, 518, 677, 875).
//
// For single-life contingencies (Living, Dead), uses person 1's table.
// For two-life contingencies, combines both tables using independence assumption.
func (c *ActuarialConfig) LifeProb(date types.DateRec, contingency byte) float64 {
	if contingency == NotContingent {
		return 1.0
	}

	s1 := c.survivalProb1(date)
	s2 := c.survivalProb2(date)

	switch contingency {
	case Living:
		return s1
	case Dead:
		return 1 - s1
	case Only1Living:
		return s1 * (1 - s2)
	case Only2Living:
		return (1 - s1) * s2
	case EitherLiving:
		return 1 - (1-s1)*(1-s2)
	case BothLiving:
		return s1 * s2
	default:
		return 1.0
	}
}

// PODValue computes the present value of a Payment on Death amount
// under a single fixed continuous-compounded rate. Thin wrapper over
// PODValueFunc; kept for callers that don't need variable-rate
// discounting.
//
// Reconstructed from PRESVALU.pas references at lines 689, 712, 790, 566.
func (c *ActuarialConfig) PODValue(asOf types.DateRec, rate float64) float64 {
	return c.PODValueFunc(asOf, func(yearsFromAsOf float64) float64 {
		return math.Exp(-rate * yearsFromAsOf)
	})
}

// PODValueFunc computes the expected present value of a Payment on
// Death amount, with discounting expressed as a caller-supplied
// function of "years from the as-of date." This generalisation lets
// the variable-rate Present Value path discount each year's death
// probability through its piecewise rate schedule rather than a
// single constant rate — matching the DOS PRESVALU.pas behavior when
// both ACTU and PVLX compile-time flags were enabled together (the
// Windows port built without ACTU, so this combination was dropped
// from the original Windows release).
//
// The discount callback receives the *years from asOf to the mid-
// point of the death year* (the same convention used by PODValue's
// closed form). For a positive value the callback should return a
// factor < 1 (future deaths discount toward zero); for a date in
// the past the callback should return a factor > 1 (accumulation
// forward to asOf). This sign convention mirrors LumpSumValue and
// VRDiscountFactor.
//
// PODValueFunc = sum over each future year k of:
//   POD × probability(death in year k) × discountForYears(k + 0.5 + offsetToNow)
//
// where probability(death in year k) = P(alive at k) - P(alive at k+1)
// derived from the life table relative to age-now.
func (c *ActuarialConfig) PODValueFunc(asOf types.DateRec,
	discountForYears func(yearsFromAsOf float64) float64) float64 {

	if c.POD == 0 || c.Table1 == nil {
		return 0
	}

	ageNow := ageAtDate(c.DOB1, c.Now)
	maxAge := float64(c.Table1.MaxAge())
	if ageNow >= maxAge {
		return 0
	}

	// Offset from the as-of date to "now" (the reference date used
	// for the survival projection). Typically asOf == Now and this
	// is zero; if they differ, every year of death-probability gets
	// shifted by the same constant.
	offsetToNow := yearsDif(asOf, c.Now)

	sum := 0.0
	for k := 0; float64(k)+ageNow < maxAge; k++ {
		futureAge := ageNow + float64(k)
		pAliveK := c.Table1.ConditionalSurvival(ageNow, futureAge)
		pAliveK1 := c.Table1.ConditionalSurvival(ageNow, futureAge+1)
		pDeathInYear := pAliveK - pAliveK1

		if pDeathInYear <= 0 {
			continue
		}

		yearsFromAsOf := offsetToNow + float64(k) + 0.5
		sum += c.POD * pDeathInYear * discountForYears(yearsFromAsOf)
	}

	return interest.Round2(sum)
}
