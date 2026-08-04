package mortgage

// BIT-LEVEL DIFFERENTIAL FOR THE MORTGAGE BACKWARD SOLVE (round 30).
//
// WHY THIS FILE EXISTS. The ratified exit criterion asks for PV and mortgage to
// be "bit-verified on their BACKWARD solves as well as forward". Round 29
// verified at source that neither package had any such harness. This is the
// mortgage half; internal/finance/presentvalue/zzbits_backward_test.go is the
// other, and internal/finance/amortization/zzbits_backward_test.go is the
// template both follow.
//
// ⚠️ FOUR OF THE FIVE UNCONSUMED BACKWARD KEYS CANNOT SERVE AS BIT ORACLES.
// Round 29 listed mtg_oracle's emitted-but-unread RAWBITS keys as
// `howmuch`, `apr`, `apr1`, `apr2` (and `cross`, `time`) and treated them as
// five equivalent openings. They are not. Reading the oracle shows that only
// `howmuch` is taken from the record:
//
//	mtg_oracle.pas:299   RawBitsAdd('howmuch', e[1]^.howmuch)     <- the RECORD field
//	mtg_oracle.pas:92    RawBitsAdd('apr', aprPct / 100)          <- FloatAfter(OracleLastError, 'term')
//	mtg_oracle.pas:201-3 RawBitsAdd('cross'|'apr1'|'apr2'|'time') <- FloatAfter(...) as well
//
// `aprPct` is PARSED BACK OUT OF THE MESSAGE TEXT DOS printed, and DOS prints it
// as `ftoa4(100 * apr, 7)` / `Double2StringFormat(..., DoubleDotFour)`
// (Mortgage.pas:545, :648, :656) — four decimals on a PERCENTAGE, i.e. 1e-6 on
// the fraction. Their low bits are the decimal formatter's, not the solver's. A
// bit differential against them would measure `ftoa4`. R13 — an instrument may
// print only what it has READ — applied to the ORACLE rather than to the port.
//
// So this file covers `howmuch`, the balloon-amount backward solve, and says
// plainly that the APR bit surface is NOT reachable through the present oracle.
// Closing it needs a mtg_oracle change that emits `apr` from the record before
// it is formatted. That is a real remaining gap, and naming it is the point.
//
// WHAT `howmuch` IS. DOS's BalloonCalc (Mortgage.pas:247-257) is CLOSED FORM,
// not a solver:
//
//	balloonval := price * (1 - pct) - (monthly - tax) * Summation(rate, years) - balloonval;
//	howmuch    := balloonval * exxp(rate * when);
//
// That makes the expectation here much sharper than for an iterative solve.
// There is no stopping criterion to absorb a difference: two faithful
// implementations differ only by operation ORDER and by `Summation`/`exxp`'s
// last bits. A wide tail or a systematic lean is therefore a statement about the
// arithmetic itself — which is exactly what §48 was, and what no decimal
// comparison can see. The existing decimal differential over this same mode
// (dos_mtg_balloon_solve_test.go) gates at a RELATIVE 1e-6 with a 5e-3 absolute
// floor; everything below that has never been looked at until now.
//
// R1 COMPLIANCE. The Go side calls Calc, the package's shared entry point, and
// reads the solved field only when the product marked it an OUTPUT. A refusal is
// a product verdict, counted and never bypassed.

import (
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// mtgULPDiff returns the signed distance in representable doubles between a and
// b. Positive means Go is ABOVE DOS. Same sign-magnitude-to-monotone mapping as
// the amortization and PV twins.
func mtgULPDiff(goVal, dosVal float64) int64 {
	gi, di := int64(math.Float64bits(goVal)), int64(math.Float64bits(dosVal))
	if gi < 0 {
		gi = math.MinInt64 - gi
	}
	if di < 0 {
		di = math.MinInt64 - di
	}
	return gi - di
}

// mtgBinomTwoTailed is the exact two-tailed binomial tail for k of n under
// p=1/2. Exact, not a normal approximation: see the note in the PV twin — the
// interesting biases are small-n by nature.
func mtgBinomTwoTailed(k, n int) float64 {
	if n == 0 {
		return 1
	}
	if k < n-k {
		k = n - k
	}
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

// mtgHowMuchBits drives mtg_oracle's solvehowmuch mode and returns DOS's solved
// balloon amount as raw bits. Argument order per mtg_oracle.pas:277-302:
// PRICE PCT YEARS TRUERATE MONTHLY WHEN [POINTS] [TAX].
func mtgHowMuchBits(bin string, price, pct float64, years int, rate, monthly float64,
	when int, points float64) (uint64, bool) {
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 10, 64) }
	cmd := exec.Command(bin, "solvehowmuch", f(price), f(pct), strconv.Itoa(years),
		f(rate), f(monthly), strconv.Itoa(when), f(points))
	cmd.Env = append(os.Environ(), "PERSENSE_ORACLE_RAWBITS=1")
	out, err := cmd.Output()
	if err != nil || strings.Contains(string(out), "ERR") {
		return 0, false
	}
	v, ok := parseRawBits(string(out))["howmuch"]
	return v, ok
}

// TestDOSMtgBackwardHowMuchBitFidelity is the standing bit-level differential
// for the balloon-amount backward solve.
func TestDOSMtgBackwardHowMuchBitFidelity(t *testing.T) {
	bin := mtgOracleBin()
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("mortgage oracle required but not present (%s)", bin)
		}
		t.Skipf("mortgage oracle not present (%s); build via legacy/oracle/build_linux.sh TARGET=mtg_oracle", bin)
	}

	cases := 300
	if v := os.Getenv("PERSENSE_BITS_N"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cases = n
		}
	}
	rng := rand.New(rand.NewSource(30069))

	var (
		checked, refused, dosNoSolve int
		above, below, exact          int
		maxAbs, sumAbs               int64
		worst                        string
		maxCents                     float64
		worstCents                   string
	)

	for i := 0; i < cases; i++ {
		price := float64(80000 + rng.Intn(600000))
		pct := 0.05 + rng.Float64()*0.4
		years := 5 + rng.Intn(35)
		rate := 0.02 + rng.Float64()*0.13
		points := rng.Float64() * 0.05
		when := 1 + rng.Intn(years-1)
		monthly := 1000.0 + rng.Float64()*3000

		// Every argument is quantized to the SAME decimal text the oracle's argv
		// carries, so the two engines start from the identical double. The
		// unquantized-argument trap once made 400 of 400 comparisons unequal for
		// this exact reason; here it would be worse, because at bit resolution a
		// one-ULP argument difference is indistinguishable from a defect.
		q := func(v float64) float64 {
			r, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'f', 10, 64), 64)
			return r
		}
		price, pct, rate, points, monthly = q(price), q(pct), q(rate), q(points), q(monthly)

		repro := "mtg_oracle solvehowmuch " +
			strconv.FormatFloat(price, 'f', 10, 64) + " " +
			strconv.FormatFloat(pct, 'f', 10, 64) + " " + strconv.Itoa(years) + " " +
			strconv.FormatFloat(rate, 'f', 10, 64) + " " +
			strconv.FormatFloat(monthly, 'f', 10, 64) + " " + strconv.Itoa(when) + " " +
			strconv.FormatFloat(points, 'f', 10, 64)

		bits, ok := mtgHowMuchBits(bin, price, pct, years, rate, monthly, when, points)
		if !ok {
			dosNoSolve++
			continue
		}
		r := Calc(MtgLine{
			PriceStatus: types.InOutInput, Price: price,
			PctStatus: types.InOutInput, Pct: pct,
			YearsStatus: types.InOutInput, Years: years,
			RateStatus: types.InOutInput, Rate: rate,
			MonthlyStatus: types.InOutInput, Monthly: monthly,
			WhenStatus: types.InOutInput, When: when,
			PointsStatus: types.InOutInput, Points: points,
			TaxStatus: types.InOutInput, Tax: 0,
			HowMuchStatus: types.StatusEmpty,
		})
		if r.Err != nil || r.Line.HowMuchStatus != types.InOutOutput {
			refused++
			continue
		}

		dosVal := math.Float64frombits(bits)
		checked++
		d := mtgULPDiff(r.Line.HowMuch, dosVal)
		switch {
		case d > 0:
			above++
		case d < 0:
			below++
		default:
			exact++
		}
		a := d
		if a < 0 {
			a = -a
		}
		sumAbs += a
		if a > maxAbs {
			maxAbs, worst = a, repro
		}
		// R14: the difference in the units this VALUE lives in. `howmuch` is a
		// currency amount DOS displays to the cent, so cents are the units any
		// verdict about it has to be stated in — a ULP count is not a unit either
		// engine promises anything in.
		if c := math.Abs(r.Line.HowMuch-dosVal) * 100; c > maxCents {
			maxCents, worstCents = c, repro
		}
	}

	if checked == 0 {
		t.Fatalf("solvehowmuch: NOTHING was compared (DOS no-solve %d, product refused %d). "+
			"A backward bit harness that compares nothing reports green.",
			dosNoSolve, refused)
	}

	mean := float64(sumAbs) / float64(checked)
	t.Logf("mortgage solved balloon amount (solvehowmuch): compared %d "+
		"(DOS no-solve %d, product refused %d) | exact %d, Go above %d, Go below %d | "+
		"mean %.1f ULP, max %d ULP | max |Δ| %.3g cents",
		checked, dosNoSolve, refused, exact, above, below, mean, maxAbs, maxCents)
	if worst != "" {
		t.Logf("   worst ULP:   %s", worst)
	}
	if worstCents != "" {
		t.Logf("   worst cents: %s", worstCents)
	}

	// ---- Assertion 1: bounded tail ----
	// BalloonCalc is closed form, so there is no stopping criterion to absorb a
	// difference and the bar is much tighter than the amortization twin's 2^20.
	// 4096 ULP on a six-figure currency value is ~1e-9 dollars — still far below
	// anything the decimal differential over this same mode can resolve (it gates
	// at relative 1e-6 with a 5e-3 absolute floor), and far above what a
	// difference in operation ORDER through Summation and exxp can produce.
	const maxULP = 4096
	if maxAbs > maxULP {
		t.Errorf("solvehowmuch: worst case is %d ULP (limit %d). BalloonCalc is CLOSED "+
			"FORM — a tail this wide is not two solvers stopping differently, it is "+
			"the arithmetic.\n  %s", maxAbs, maxULP, worst)
	}

	// ---- Assertion 2: no sign bias. THE POINT OF THIS FILE. ----
	// A closed form makes this sharper still: with no iteration in the way, a
	// consistent lean IS an ordering or conversion difference. Reported always;
	// fails only when it is also materially sized, which for a displayed currency
	// value means at the scale of a hundredth of a cent.
	nz := above + below
	if nz > 0 {
		lean, dir := above, "above"
		if below > above {
			lean, dir = below, "below"
		}
		p := mtgBinomTwoTailed(lean, nz)
		t.Logf("   sign balance: %d of %d non-exact differences have Go %s DOS "+
			"(two-tailed p=%.2g)", lean, nz, dir, p)
		if p < 0.01 && maxCents > 0.01 {
			t.Errorf("solvehowmuch: SIGN-BIASED. %d of %d non-exact differences lean %s "+
				"(two-tailed p=%.2g) and the worst is %.3g cents. Independent rounding "+
				"does not do this; a difference in the order of BalloonCalc's terms, or "+
				"in Summation/exxp, does.\n  %s", lean, nz, dir, p, maxCents, worstCents)
		}
	}
}
