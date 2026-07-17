package mortgage

// TestDOSMtgFuzzer3 is the ADVERSARIAL edge differential fuzzer for the mortgage
// engine — the analogue of the amortization dos_fuzzer3. Where the existing
// mortgage sweeps draw well-conditioned, self-consistent inputs (a solvable
// price/pct/rate and compare the solved value), fuzzer3 draws every input
// INDEPENDENTLY and widely — percent-down at or past 100%, points past the
// down payment, monthly payments that imply a degenerate or negative price,
// balloons larger than the loan — deliberately pushing into the region where a
// screen is over-determined, non-amortizing, or otherwise refused.
//
// Its hard assertion is one-directional and targets the class the value sweeps
// are structurally blind to: **Go must not SOLVE a case DOS REFUSES.** DOS
// emits `ERR <message>` on a refused/over-determined screen; the port must not
// return a (finite) computed value where DOS declines. The reverse (DOS solves,
// Go refuses) is Go being conservative at the boundary and is LOGGED, not
// failed; likewise value divergences on both-solved cases are logged.
//
// Byte-identical inputs: every value is fed to the oracle at 10dp and quantized
// on the Go side to a coarser grid, so both engines receive the same number.

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

func TestDOSMtgFuzzer3(t *testing.T) {
	bin := mtgOracleBin()
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("mortgage oracle required but not present (%s)", bin)
		}
		t.Skipf("mortgage oracle not present (%s)", bin)
	}
	rng := rand.New(rand.NewSource(fuzzSeed(0x6d746733))) // "mtg3"

	N := 3000
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, e := strconv.Atoi(s); e == nil && v > 0 {
			N = v
		}
	}
	q6 := func(x float64) float64 { return math.Round(x*1e6) / 1e6 }
	finite := func(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

	const (
		dosSolved = iota
		dosRefused
		dosFlake
	)
	// runOracle returns the fields of the (retried) output line and whether it ran.
	runOracle := func(args []string) ([]string, bool) {
		for try := 0; try < 6; try++ {
			out, err := exec.Command(bin, args...).Output()
			if err != nil {
				continue
			}
			f := strings.Fields(strings.TrimSpace(string(out)))
			if len(f) >= 1 {
				return f, true
			}
		}
		return nil, false
	}
	// dosMtg runs a solve mode and extracts the requested solved field. A refusal
	// is "ERR ..."; a success line is "monthly M price P cash C financed F".
	fieldIdx := map[string]int{"monthly": 1, "price": 3, "cash": 5, "financed": 7}
	dosMtg := func(want string, args []string) (float64, int) {
		f, ran := runOracle(args)
		if !ran {
			return 0, dosFlake
		}
		if f[0] == "ERR" {
			return 0, dosRefused
		}
		if f[0] != "monthly" || len(f) < 8 {
			return 0, dosRefused // any non-canonical line is a refusal/insufficient screen
		}
		v, err := strconv.ParseFloat(f[fieldIdx[want]], 64)
		if err != nil {
			return 0, dosFlake
		}
		return v, dosSolved
	}

	// The four solve directions we exercise, each mapping to an oracle mode + the
	// Go MtgLine blank field. `want` is the solved field compared across engines.
	fields := []string{"monthly", "price", "mcash", "mfin"}

	checked, bothRefused, flaked := 0, 0, 0
	goSolvesDosRefuses := 0
	dosSolvesGoRefuses := 0
	valDiverge := 0
	worstVal, worstMsg := 0.0, ""

	for c := 0; c < N; c++ {
		price := math.Round(1000 + rng.Float64()*1_999_000)
		pct := q6(rng.Float64() * 1.25)                 // includes ≥100% down (degenerate)
		years := 1 + rng.Intn(45)                       // 1..45
		rate := q6(rng.Float64() * 0.60)                // wide true-rate range incl. 0
		points := q6(rng.Float64() * 0.60)              // can exceed a small down payment
		monthly := math.Round(1 + rng.Float64()*49_999) // for the price solve (independent)
		field := fields[rng.Intn(len(fields))]

		// Occasional balloon on the monthly-family solves (huge/early → refusal-prone).
		hasBalloon := field != "price" && rng.Intn(4) == 0
		bWhen, bHow := 0, 0.0
		if hasBalloon {
			bWhen = 1 + rng.Intn(years) // may be == years (at/after term)
			bHow = math.Round(price * (0.05 + rng.Float64()*2.5))
		}

		var args []string
		var want string
		m := MtgLine{
			YearsStatus: types.InOutInput, Years: years,
			RateStatus: types.InOutInput, Rate: rate,
			PointsStatus: types.InOutInput, Points: points,
			TaxStatus: types.InOutInput, Tax: 0,
		}
		switch field {
		case "monthly":
			want = "monthly"
			m.PriceStatus, m.Price = types.InOutInput, price
			m.PctStatus, m.Pct = types.InOutInput, pct
			m.MonthlyStatus = types.StatusEmpty
			args = []string{"monthly", ff(price), ff(pct), strconv.Itoa(years), ff(rate), ff(points)}
			if hasBalloon {
				m.WhenStatus, m.When = types.InOutInput, bWhen
				m.HowMuchStatus, m.HowMuch = types.InOutInput, bHow
				args = append(args, strconv.Itoa(bWhen), ff(bHow))
			}
		case "price":
			want = "price"
			m.PctStatus, m.Pct = types.InOutInput, pct
			m.MonthlyStatus, m.Monthly = types.InOutInput, monthly
			m.PriceStatus = types.StatusEmpty
			args = []string{"price", ff(pct), strconv.Itoa(years), ff(rate), ff(monthly), ff(points)}
		case "mcash":
			want = "monthly"
			cash := math.Round(price * (pct + (1-pct)*points))
			m.PriceStatus, m.Price = types.InOutInput, price
			m.CashStatus, m.Cash = types.InOutInput, cash
			m.MonthlyStatus = types.StatusEmpty
			args = []string{"mcash", ff(price), ff(cash), strconv.Itoa(years), ff(rate), ff(points)}
		default: // mfin
			want = "monthly"
			financed := math.Round(price * (1 - pct))
			m.PriceStatus, m.Price = types.InOutInput, price
			m.FinancedStatus, m.Financed = types.InOutInput, financed
			m.MonthlyStatus = types.StatusEmpty
			args = []string{"mfin", ff(price), ff(financed), strconv.Itoa(years), ff(rate), ff(points)}
		}

		dosVal, outcome := dosMtg(want, args)
		if outcome == dosFlake {
			flaked++
			continue
		}
		dosOK := outcome == dosSolved

		r := Calc(m)
		var goVal float64
		goOK := r.Err == nil
		if goOK {
			switch want {
			case "price":
				goVal = r.Line.Price
			default:
				goVal = r.Line.Monthly
			}
			if !finite(goVal) {
				goOK = false // a non-finite result is not a solve
			}
		}

		switch {
		case goOK && !dosOK:
			goSolvesDosRefuses++
			if goSolvesDosRefuses <= 20 {
				t.Errorf("Go SOLVES where DOS REFUSES (%s): Go=%.6f | price=%.0f pct=%.6f yrs=%d rate=%.6f pts=%.6f monthly=%.0f balloon=%v | [%s]",
					field, goVal, price, pct, years, rate, points, monthly, hasBalloon, strings.Join(args, " "))
			}
		case !goOK && dosOK:
			dosSolvesGoRefuses++
		case !goOK && !dosOK:
			bothRefused++
		default:
			checked++
			tol := math.Max(0.01, 1e-6*math.Abs(dosVal))
			if d := math.Abs(goVal - dosVal); d > tol {
				rel := d / math.Max(1, math.Abs(dosVal))
				valDiverge++
				if rel > worstVal {
					worstVal = rel
					worstMsg = fmt.Sprintf("%s Go=%.4f DOS=%.4f rel=%.2e [%s]", field, goVal, dosVal, rel, strings.Join(args, " "))
				}
			}
		}
	}
	t.Logf("mtg fuzzer3: %d both-solved, %d both-refused, %d flake; "+
		"Go-solves-DOS-refuses=%d (HARD); DOS-solves-Go-refuses=%d (conservative, logged); "+
		"value-diverge=%d worst rel=%.2e [%s]",
		checked, bothRefused, flaked, goSolvesDosRefuses, dosSolvesGoRefuses, valDiverge, worstVal, worstMsg)
	if checked+bothRefused < N/4 {
		t.Fatalf("only %d/%d cases adjudicated — harness/oracle problem", checked+bothRefused, N)
	}
}
