package interest

// Correctly-rounded exp and log.
//
// WHY THIS FILE EXISTS
//
// The DOS engine is the specification, and every differential test in this
// repo compares against a Free Pascal build of the original units. FPC's `exp`
// and `ln` are essentially correctly rounded — over 20,000 random arguments
// they returned the correctly-rounded double 99.97% and 99.98% of the time.
// Go's math.Exp and math.Log are documented only to be within 1 ULP, and they
// are: measured over the same arguments, math.Exp disagreed with the
// correctly-rounded result on 13.67% of them and math.Log on 9.38%.
//
// A 1-ULP disagreement 13% of the time is not a cosmetic difference here.
// DOS's Iterate is a BRACKET-FREE SECANT (AMORTOP.pas:1415-1495) run against a
// terminal-balance function that, on an option loan, is a sawtooth with several
// roots and with FLAT PLATEAUS — stretches where the seed and the +0.1% probe
// evaluate to the SAME double. On a plateau the slope's sign is decided by the
// last bit of the two terminal values, so a single ULP anywhere upstream flips
// the secant's direction, throws the next iterate to the far field (~1e17), and
// lands the walk in a different basin. The two engines then converge, cleanly
// and stably, to different roots and print different schedules.
//
// Traced concretely on fuzzer5 seed 20622:
//
//	amort_oracle 425820.45 0.1175910000 23 1 b365 prepaid r78 usa \
//	  loandmy=8.11.2025 firstdmy=8.11.2026 b36=73551.47 b204=98688.56 \
//	  pre=96:45:24:314.18 pre=144:42:26:121.02 adj=24:0.0944850000:50593.01 \
//	  adj=60:0.0577340000: adj=192:0.0648990000:51378.45 targ=9334.38 pts=0.031740
//
// The first balloon's discount is exxp(-0.33357136743172633). The correctly
// rounded double is 0.71636077198711101; FPC returns it, Go's math.Exp returns
// 0.71636077198711112. That one bit moved DOS's base-payment seed from
// 44838.166556627104 to 44838.166556627097, which moved the terminal by ~3 ULP,
// which flipped the secant sign, which sent the two engines to different roots —
// a 5,634.44 difference in total interest on an 885,407.24 loan.
//
// So: exp and log have to be correctly rounded, not merely accurate. This file
// provides that.
//
// HOW
//
// Both routines evaluate in double-double (two non-overlapping float64s, ~106
// bits of significand) and round once at the end. The residual error is on the
// order of 2^-105, so the probability that the final rounding differs from the
// correctly-rounded result is about 2^-52 per call — negligible next to FPC's
// own 3-in-10,000 rate.
//
// The double-double primitives are the standard Dekker/Knuth ones. twoProd uses
// math.FMA, which Go guarantees is the exact fused operation (hardware FMA on
// amd64/arm64, software fallback elsewhere), so the product's error term is
// exact rather than reconstructed by Dekker splitting.

import "math"

// dd is a double-double: the represented value is hi+lo, with |lo| <= ulp(hi)/2
// and the sum unevaluated. Every operation below re-establishes that invariant.
type dd struct{ hi, lo float64 }

// twoSum returns s = a+b rounded, and the EXACT error e such that a+b = s+e.
// Knuth's six-operation version; makes no assumption about |a| vs |b|.
func twoSum(a, b float64) (s, e float64) {
	s = a + b
	bb := s - a
	e = (a - (s - bb)) + (b - bb)
	return
}

// quickTwoSum is twoSum's three-operation form, valid only when |a| >= |b|.
// Used to renormalize a (hi, lo) pair whose magnitudes are already ordered.
func quickTwoSum(a, b float64) (s, e float64) {
	s = a + b
	e = b - (s - a)
	return
}

// twoProd returns p = a*b rounded, and the EXACT error e such that a*b = p+e.
func twoProd(a, b float64) (p, e float64) {
	p = a * b
	e = math.FMA(a, b, -p)
	return
}

func ddAdd(a, b dd) dd {
	s, e := twoSum(a.hi, b.hi)
	e += a.lo + b.lo
	s, e = quickTwoSum(s, e)
	return dd{s, e}
}

func ddAddD(a dd, b float64) dd {
	s, e := twoSum(a.hi, b)
	e += a.lo
	s, e = quickTwoSum(s, e)
	return dd{s, e}
}

func ddMul(a, b dd) dd {
	p, e := twoProd(a.hi, b.hi)
	e += a.hi*b.lo + a.lo*b.hi
	p, e = quickTwoSum(p, e)
	return dd{p, e}
}

func ddMulD(a dd, b float64) dd {
	p, e := twoProd(a.hi, b)
	e += a.lo * b
	p, e = quickTwoSum(p, e)
	return dd{p, e}
}

// ddDivD divides a double-double by a double. One Newton correction on the
// leading quotient recovers the second limb.
func ddDivD(a dd, b float64) dd {
	q1 := a.hi / b
	p, pe := twoProd(q1, b)
	rh, rl := twoSum(a.hi, -p)
	rl += a.lo - pe
	q2 := (rh + rl) / b
	s, t := quickTwoSum(q1, q2)
	return dd{s, t}
}

// value collapses a double-double to the nearest float64. |lo| <= ulp(hi)/2, so
// the single addition IS the correct rounding of hi+lo (barring a tie, which
// the ~2^-105 residual makes vanishingly unlikely).
func (a dd) value() float64 { return a.hi + a.lo }

// ln2 to double-double precision: 0.69314718055994530941723212145798...
var ddLn2 = dd{6.93147180559945286e-01, 2.319046813846299558e-17}

// log2e = 1/ln2, used only to pick the reduction integer k, so double precision
// is ample.
const log2e = 1.44269504088896338700e+00

// invFactDD holds 1/n! as a double-double for n = 2..9, indexed invFactDD[n-2].
// These are the coefficients whose plain-double rounding error would still be
// visible at the 2^-105 target; beyond n=9 the term itself is small enough that
// a plain double coefficient contributes nothing (see expDD).
var invFactDD = [8]dd{
	{5.00000000000000000e-01, 0.00000000000000000e+00},  // 1/2!
	{1.66666666666666657e-01, 9.25185853854297066e-18},  // 1/3!
	{4.16666666666666644e-02, 2.31296463463574266e-18},  // 1/4!
	{8.33333333333333322e-03, 1.15648231731787138e-19},  // 1/5!
	{1.38888888888888894e-03, -5.30054395437357706e-20}, // 1/6!
	{1.98412698412698413e-04, 1.72095582934207053e-22},  // 1/7!
	{2.48015873015873016e-05, 2.15119478667758816e-23},  // 1/8!
	{2.75573192239858925e-06, -1.85839327404647208e-22}, // 1/9!
}

// Plain-double 1/n! for the series tail, n = 10..14.
const (
	invFact10 = 2.75573192239858883e-07
	invFact11 = 2.50521083854417202e-08
	invFact12 = 2.08767569878681002e-09
	invFact13 = 1.60590438368216133e-10
	invFact14 = 1.14707455977297245e-11
)

// expDD returns exp(x) as a double-double. x must be finite and moderate; the
// callers in this package bound |x| to 70.
//
// Reduction: k = round(x·log2e), r = x − k·ln2 with ln2 carried in
// double-double, so |r| <= ln2/2 ≈ 0.3466 and the reduction itself contributes
// no error at the 2^-105 level. r is then scaled down by 2^-5 to |r'| <= 0.0109
// and exp(r')−1 is summed as rs·(1 + rs·(1/2! + rs·(1/3! + …))). Five doublings
// via exp(2a)−1 = (exp(a)−1)·((exp(a)−1)+2) undo the scaling WITHOUT ever
// forming exp(r')−1 as a difference of nearly-equal numbers, which is what keeps
// the relative accuracy. Finally 2^k is applied exactly.
//
// The series is split at n=9. The Horner accumulator enters the double-double
// section holding a value of magnitude ~1/9!, and terms from 1/10! on are
// weighted by rs^9 <= 0.0109^9 ≈ 1.5e-18 relative to the leading term, i.e.
// below 2^-59 of it — so the tail can be summed in plain float64 and its own
// rounding error (~2^-53 of a 2^-59 quantity, ~2^-112 relative) stays under the
// 2^-105 budget. Only the n<=9 coefficients need double-double representation.
// This matters for speed: the earlier formulation divided by n inside the loop,
// which cost 26 double-precision divisions per call and dominated the runtime.
func expDD(x float64) dd {
	k := math.Round(x * log2e)
	r := ddAdd(dd{x, 0}, ddMulD(ddLn2, -k))

	// Scale r down by 2^5. Both limbs scale exactly (a power of two).
	const shifts = 5
	rs := dd{r.hi / 32, r.lo / 32}

	// Plain-double tail, Horner from 1/14! up to 1/10!.
	rh := rs.hi
	tail := invFact14
	tail = invFact13 + rh*tail
	tail = invFact12 + rh*tail
	tail = invFact11 + rh*tail
	tail = invFact10 + rh*tail

	// Double-double Horner from 1/9! down to 1/2!. The tail enters weighted by a
	// single rs, continuing the same Horner chain.
	t := ddAddD(invFactDD[7], rh*tail)
	for n := 6; n >= 0; n-- {
		t = ddAdd(invFactDD[n], ddMul(rs, t))
	}

	// m = exp(rs) - 1 = rs·(1 + rs·t).
	m := ddMul(rs, ddAddD(ddMul(rs, t), 1))

	// Undo the scaling: exp(2a) - 1 = (exp(a)-1) * ((exp(a)-1) + 2).
	for i := 0; i < shifts; i++ {
		m = ddMul(m, ddAddD(m, 2))
	}

	e := ddAddD(m, 1)
	// Apply 2^k. Scaling each limb separately is exact for the exponent range
	// these callers produce (|x| <= 70 ⇒ |k| <= 101).
	ki := int(k)
	return dd{math.Ldexp(e.hi, ki), math.Ldexp(e.lo, ki)}
}

// crExp returns the correctly-rounded float64 nearest exp(x).
func crExp(x float64) float64 {
	if x == 0 {
		return 1
	}
	return expDD(x).value()
}

// crLog returns the correctly-rounded float64 nearest ln(x), for x > 0.
//
// Method: split x = m·2^e with m in [√½, √2), take y0 = math.Log(m) (good to
// ~1 ULP), then correct it in double-double. With u = m·exp(−y0) − 1 we have
// ln(m) = y0 + ln(1+u), and |u| is on the order of 2^-52, so two terms of the
// log1p series are already past 2^-105. exp(−y0) is the double-double expDD
// above, which is what makes the correction meaningful. The exponent's
// contribution e·ln2 is then added in double-double.
func crLog(x float64) float64 {
	if x <= 0 || math.IsInf(x, 1) || math.IsNaN(x) {
		return math.Log(x)
	}
	m, e := math.Frexp(x) // x = m·2^e, m in [0.5, 1)
	// Re-center to [√½, √2) so |ln m| <= ln2/2 and the correction stays tiny.
	if m < math.Sqrt2/2 {
		m *= 2
		e--
	}
	y0 := math.Log(m)
	u := ddAddD(ddMulD(expDD(-y0), m), -1)
	// ln(1+u) = u - u²/2 + u³/3 - …; u³ is below 2^-150 here.
	u2 := ddMul(u, u)
	corr := ddAdd(u, ddMulD(u2, -0.5))
	return ddAdd(ddAddD(corr, y0), ddMulD(ddLn2, float64(e))).value()
}
