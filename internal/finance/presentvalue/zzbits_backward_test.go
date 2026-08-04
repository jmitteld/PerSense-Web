package presentvalue

// BIT-LEVEL DIFFERENTIAL FOR THE PRESENT-VALUE BACKWARD SOLVER (round 30).
//
// WHY THIS FILE EXISTS. The ratified exit criterion asks for PV and mortgage to
// be "bit-verified on their BACKWARD solves as well as forward". Round 29
// verified at source that neither package had any such harness at all:
//
//	                forward bit harness              backward bit harness
//	amortization    zzbits_fidelity_test.go          zzbits_backward_test.go + _long_
//	present value   zzbits_fidelity_test.go (`pv`)   NONE   <- this file
//	mortgage        zzbits_fidelity_test.go (4 keys) NONE   <- zzbits_backward_test.go there
//
// `pv_oracle.pas` has emitted `RawBitsAdd('rate', c[1]^.r.rate)` on its `bk_rate`
// mode since round 10 and nothing has ever read it. An emitted RAWBITS key that
// nothing reads is not coverage — it is a harness that reports green because it
// compares nothing, which is the failure mode this project has now hit twice.
//
// WHY DECIMAL RESOLUTION IS NOT A SUBSTITUTE. The identical argument round 18b
// made for the amortization backward solvers applies verbatim here, and this
// solver is if anything more exposed: DOS's PV rate solve (PRESVALU.pas:690-735)
// is a SECANT WITH A NUMERICAL DERIVATIVE and a hard ±0.04 step clamp
//
//	denom := (sum - oldsum);  if abs(denom) < teeny then denom := teeny;
//	if count = 1 then diff := 0.001 else diff := (sumvalue - sum) * diff / denom;
//	if diff < -0.04 then diff := -0.04 else if diff > 0.04 then diff := 0.04;
//	r.rate := r.rate - diff;
//	until (abs(diff) < teeny) or (count = 30);
//
// — so it stops on the SIZE OF ITS LAST STEP, not on a residual, and it returns
// whatever point that step landed on. Two implementations of the same recurrence
// will differ in the last bits essentially always. What distinguishes a defect
// from that arithmetic is a systematic LEAN, and no decimal comparison at any
// tolerance can see one. §48 was exactly that shape.
//
// WHAT THIS TEST ASSERTS, AND WHAT IT DELIBERATELY DOES NOT. It does not demand
// bit-identical solves. It asserts (1) a bounded ULP tail, so no case is wildly
// off while a decimal comparison calls it agreeing, and (2) NO SIGN BIAS, by an
// exact two-tailed binomial — the assertion the decimal harness cannot make.
// Both mirror internal/finance/amortization/zzbits_backward_test.go, which is
// this file's template by design: a second shape would be a second thing to
// audit.
//
// R14 (a verdict must be expressed in the units the solver accepts in). DOS's
// acceptance here is stated directly in RATE units — `abs(diff) < teeny`, teeny
// = 1E-10 (PETYPES.PAS:148) — so unlike the amortization case no repricing is
// needed to reach DOS's own band. The ratio |goRate − dosRate| / teeny is
// reported, and a ULP tail that is large in ULP but small against teeny is
// arithmetic, not a defect. That is why the tail assertion is scaled to teeny
// and the raw ULP count is reported alongside rather than gating alone.
//
// R1 COMPLIANCE. The Go side calls Calculate, the package's shared entry point —
// not the internal Newton routine. A harness that reassembles the solve and
// drops the convergence gate is defect #7's exact shape; here a refusal is a
// product verdict, counted and never bypassed.
//
// ⚠️ SCOPE, STATED SO NOBODY READS MORE INTO A GREEN RUN THAN IS THERE.
// `bk_rate` is the ONLY backward PV solve that is drivable headlessly and emits
// bits. pv_oracle.pas:950-956 says why: the lump and periodic AMOUNT solves go
// through BackwardCalc's backup frame, which depends on the full screen-column
// layout. `bk_asof` is drivable and emits NO bits. So this file covers the RATE
// solve on a single-lump frame and nothing else. The remaining backward PV
// surface is still uncovered at bit resolution, and that is a real gap, not an
// omission — see the round-30 write-up.

import (
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// pvTeeny is DOS's convergence constant, PETYPES.PAS:148. The PV rate solve
// stops when its own step falls below this, so it IS the acceptance band.
const pvTeeny = 1e-10

// pvULPDiff returns the signed distance in representable doubles between a and
// b. Positive means the Go value is ABOVE DOS's. Same mapping as the
// amortization twin: sign-magnitude onto a monotone integer ordering, so
// adjacent doubles are one apart across zero.
func pvULPDiff(goVal, dosVal float64) int64 {
	gi, di := int64(math.Float64bits(goVal)), int64(math.Float64bits(dosVal))
	if gi < 0 {
		gi = math.MinInt64 - gi
	}
	if di < 0 {
		di = math.MinInt64 - di
	}
	return gi - di
}

// pvBinomTwoTailed is the exact two-tailed binomial tail for k of n under
// p=1/2. Exact rather than a normal approximation because the interesting
// biases are small-n by nature — a solver that is nearly always bit-exact
// produces few non-exact cases, and those are precisely the ones worth reading.
// The amortization twin found 12 of 12 leaning one way (p=4.9e-4) on its first
// run; an n>=30 gate would have said nothing.
func pvBinomTwoTailed(k, n int) float64 {
	if n == 0 {
		return 1
	}
	if k < n-k {
		k = n - k
	}
	// Sum the upper tail P(X >= k) and double it, capped at 1.
	logFact := func(m int) float64 {
		s := 0.0
		for i := 2; i <= m; i++ {
			s += math.Log(float64(i))
		}
		return s
	}
	lf := logFact(n)
	tail := 0.0
	for i := k; i <= n; i++ {
		tail += math.Exp(lf - logFact(i) - logFact(n-i) - float64(n)*math.Ln2)
	}
	if p := 2 * tail; p < 1 {
		return p
	}
	return 1
}

type pvBackwardBitStat struct {
	name       string
	checked    int
	refused    int // the PRODUCT refused — a verdict, not a failure
	dosNoSolve int
	above      int
	below      int
	exact      int
	maxAbs     int64
	sumAbs     int64
	worst      string

	// R14: the same differences in DOS's own acceptance units.
	maxTeenyRatio float64
	worstTeeny    string
}

func (s *pvBackwardBitStat) note(goVal, dosVal float64, repro string) {
	s.checked++
	d := pvULPDiff(goVal, dosVal)
	switch {
	case d > 0:
		s.above++
	case d < 0:
		s.below++
	default:
		s.exact++
	}
	a := d
	if a < 0 {
		a = -a
	}
	s.sumAbs += a
	if a > s.maxAbs {
		s.maxAbs = a
		s.worst = repro
	}
	if r := math.Abs(goVal-dosVal) / pvTeeny; r > s.maxTeenyRatio {
		s.maxTeenyRatio = r
		s.worstTeeny = repro
	}
}

// pvBkRateBits drives pv_oracle's bk_rate mode and returns DOS's solved rate as
// raw bits. Argument order is SUMVALUE, LUMP AMOUNT, MONTHS-AFTER-AS-OF, per
// pv_oracle.pas:951-971.
func pvBkRateBits(bin string, sumValue, amount float64, months int) (uint64, bool) {
	cmd := exec.Command(bin, "bk_rate",
		strconv.FormatFloat(sumValue, 'f', 2, 64),
		strconv.FormatFloat(amount, 'f', 2, 64),
		strconv.Itoa(months))
	cmd.Env = append(os.Environ(), "PERSENSE_ORACLE_RAWBITS=1")
	out, err := cmd.Output()
	if err != nil {
		return 0, false
	}
	v, ok := parseRawBits(string(out))["rate"]
	return v, ok
}

// pvBkRateFrame builds the Go input that mirrors pv_oracle's SetupLumpFrame plus
// bk_rate exactly: AllocAll's defaults are as-of 1/1/2024, r.peryr = 1, basis
// x360, colamonth ANN (pv_oracle.pas:94-110), and SetupLumpFrame puts a single
// lump `months` months later with its VALUE blank (:606-617). bk_rate then makes
// the RATE the unknown and supplies the target sum value and the lump amount.
func pvBkRateFrame(sumValue, amount float64, months int) PVInput {
	y := 2024 + months/12
	m := time.Month(months%12 + 1)
	return PVInput{
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput,
			Date:       types.NewDateRec(y, m, 1),
			AmtStatus:  types.InOutInput,
			Amt:        amount,
		}},
		PresVal: PresValLine{
			AsOfStatus:     types.InOutInput,
			AsOf:           types.NewDateRec(2024, time.January, 1),
			R:              RateEntry{Status: types.StatusEmpty, PerYr: 1},
			SumValueStatus: types.InOutInput,
			SumValue:       sumValue,
		},
		Settings: PVSettings{
			Basis: types.Basis360, PerYr: 1, COLAMonth: types.COLAAnnual,
			YrDays: 360, YrInv: 1.0 / 360,
		},
	}
}

// TestDOSPVBackwardRateBitFidelity is the standing bit-level differential for
// the PV rate solve. It is the backward twin of TestDOSPVScreenTotalBitFidelity
// and closes the PV half of the exit criterion's backward-bit clause.
func TestDOSPVBackwardRateBitFidelity(t *testing.T) {
	bin := pvOracleBin()
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("PV oracle required but not present (%s)", bin)
		}
		t.Skipf("PV oracle not present (%s); build via legacy/oracle/build_linux.sh TARGET=pv_oracle", bin)
	}

	cases := 300
	if v := os.Getenv("PERSENSE_BITS_N"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cases = n
		}
	}
	// Seeded, so a failure is reproducible from the printed repro line alone.
	rng := rand.New(rand.NewSource(30068))
	stat := &pvBackwardBitStat{name: "PV solved rate (bk_rate)"}

	for i := 0; i < cases; i++ {
		// A single lump, discounted over 1..40 years. The horizon is drawn wide
		// on purpose: DOS's ±0.04 step clamp means the number of secant steps —
		// and therefore where it stops — depends on how far the seed is from the
		// root, and a short-horizon-only sample would never exercise the clamp.
		months := 12 + rng.Intn(468)
		amount := quantize(float64(10000+rng.Intn(990000)), 2)
		// The target sum value is drawn as a discount FRACTION of the lump rather
		// than as an absolute, so every case has a root in a plausible rate band
		// instead of most of them refusing. 0.08..0.97 spans roughly 0.1%..40%
		// annualised across the horizon range above.
		frac := 0.08 + rng.Float64()*0.89
		sumValue := quantize(amount*frac, 2)
		if sumValue <= 0 {
			continue
		}
		repro := "pv_oracle bk_rate " +
			strconv.FormatFloat(sumValue, 'f', 2, 64) + " " +
			strconv.FormatFloat(amount, 'f', 2, 64) + " " + strconv.Itoa(months)

		bits, ok := pvBkRateBits(bin, sumValue, amount, months)
		if !ok {
			// DOS refused (rate not determined / did not converge) or errored.
			// Not a failure — but counted, because a run where DOS answers
			// nothing must not look like a run where the two engines agreed.
			stat.dosNoSolve++
			continue
		}
		res := Calculate(pvBkRateFrame(sumValue, amount, months))
		if res.Err != nil {
			stat.refused++
			continue
		}
		stat.note(res.Rate, math.Float64frombits(bits), repro)
	}

	if stat.checked == 0 {
		t.Fatalf("%s: NOTHING was compared (DOS no-solve %d, product refused %d). "+
			"A backward bit harness that compares nothing reports green — that is "+
			"exactly the state this file was written to end.",
			stat.name, stat.dosNoSolve, stat.refused)
	}

	mean := float64(stat.sumAbs) / float64(stat.checked)
	t.Logf("%s: compared %d (DOS no-solve %d, product refused %d) | "+
		"exact %d, Go above %d, Go below %d | mean %.1f ULP, max %d ULP | "+
		"max |Δrate|/teeny %.3g",
		stat.name, stat.checked, stat.dosNoSolve, stat.refused,
		stat.exact, stat.above, stat.below, mean, stat.maxAbs, stat.maxTeenyRatio)
	if stat.worst != "" {
		t.Logf("   worst ULP:   %s", stat.worst)
	}
	if stat.worstTeeny != "" {
		t.Logf("   worst teeny: %s", stat.worstTeeny)
	}

	// ---- Assertion 1: bounded tail, IN DOS'S OWN UNITS (R14) ----
	// The gate is |Δrate| against DOS's stopping constant, not a bare ULP count:
	// a ULP count is not a unit either engine promises anything in, and defect
	// #10 / R10 is the standing reminder that an unscaled constant in an
	// instrument is a defect waiting to be measured. 10x teeny allows for two
	// implementations stopping on adjacent steps of the same recurrence; 1e-9 on
	// a rate is a tenth of a basis point of a basis point, orders below anything
	// any other instrument in this project can resolve.
	const maxTeenyRatio = 10.0
	if stat.maxTeenyRatio > maxTeenyRatio {
		t.Errorf("%s: worst case is %.3g x DOS's own acceptance step (teeny=%g, limit %gx). "+
			"At this size the two engines are not stopping on adjacent steps of the "+
			"same recurrence.\n  %s",
			stat.name, stat.maxTeenyRatio, pvTeeny, maxTeenyRatio, stat.worstTeeny)
	}

	// ---- Assertion 2: no sign bias. THE POINT OF THIS FILE. ----
	// Independent rounding splits evenly. A systematic conversion or ordering
	// defect leans — §48's signature, invisible to every decimal instrument.
	//
	// Severity is split deliberately (R6, and R10's lesson about bare constants):
	// a lean is a real statement about the arithmetic at ANY magnitude, so it is
	// always REPORTED; it FAILS only when it is both significant and materially
	// sized in DOS's units. Failing the suite over a systematic 3-ULP offset —
	// 1e-16 relative — would be crying wolf, and a red suite everyone learns to
	// ignore is worse than no test.
	nz := stat.above + stat.below
	if nz > 0 {
		lean, dir := stat.above, "above"
		if stat.below > stat.above {
			lean, dir = stat.below, "below"
		}
		p := pvBinomTwoTailed(lean, nz)
		t.Logf("   sign balance: %d of %d non-exact differences have Go %s DOS "+
			"(two-tailed p=%.2g)", lean, nz, dir, p)
		if p < 0.01 && stat.maxTeenyRatio > 1.0 {
			t.Errorf("%s: SIGN-BIASED. %d of %d non-exact differences lean %s "+
				"(two-tailed p=%.2g) and the worst is %.3g x DOS's acceptance step. "+
				"Independent rounding does not do this; a systematic difference in "+
				"the discount arithmetic or in the secant's ordering does.\n  %s",
				stat.name, lean, nz, dir, p, stat.maxTeenyRatio, stat.worstTeeny)
		}
	}
}
