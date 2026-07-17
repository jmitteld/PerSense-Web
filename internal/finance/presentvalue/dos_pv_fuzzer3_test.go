package presentvalue

// TestDOSPVFuzzer3 is the ADVERSARIAL edge differential fuzzer for the present-
// value engine — the analogue of the amortization dos_fuzzer3. The existing PV
// sweeps draw a CONSISTENT target (forward-compute a sum value, then solve a
// field back), so they only probe reachable, well-conditioned backward solves.
// fuzzer3 instead draws the target sum value INDEPENDENTLY of the other inputs,
// so a large fraction of draws are unreachable — a sum value above the
// undiscounted amount (implying a negative discount rate or a date before the
// as-of), a near-zero target (a huge rate), etc. — exactly the region where one
// engine's Newton refuses and the other might not.
//
// Hard assertion (one-directional): **Go must not SOLVE a case DOS REFUSES.**
// The PV oracle emits `ERR <message>` on a refused screen. The reverse (DOS
// solves, Go refuses) is Go conservative at the boundary and is LOGGED; value
// divergences on both-solved cases are logged. Inputs are byte-identical (10dp
// to the oracle, 6dp-gridded on the Go side).

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func TestDOSPVFuzzer3(t *testing.T) {
	if _, err := os.Stat(pvOracleBin()); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("PV oracle required but not present (%s)", pvOracleBin())
		}
		t.Skip("PV oracle not present")
	}
	rng := rand.New(rand.NewSource(fuzzSeed(0x70763300))) // "pv3"
	bin := pvOracleBin()

	N := 3000
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, e := strconv.Atoi(s); e == nil && v > 0 {
			N = v
		}
	}
	q6 := func(x float64) float64 { return math.Round(x*1e6) / 1e6 }
	cents := func(x float64) float64 { return math.Round(x*100) / 100 }
	finite := func(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

	const (
		dosSolved = iota
		dosRefused
		dosFlake
	)
	// dosRun execs the oracle, retrying transient failures, and classifies the
	// line: "ERR ..." → refused; a line whose first token is `tok` → solved
	// (fields returned); anything else → refused (non-canonical screen).
	dosRun := func(tok string, args []string) ([]string, int) {
		for try := 0; try < 6; try++ {
			out, err := exec.Command(bin, args...).Output()
			if err != nil {
				continue
			}
			f := strings.Fields(strings.TrimSpace(string(out)))
			if len(f) == 0 {
				continue
			}
			if f[0] == "ERR" {
				return nil, dosRefused
			}
			if f[0] != tok || len(f) < 2 {
				return nil, dosRefused
			}
			return f, dosSolved
		}
		return nil, dosFlake
	}
	f10 := func(x float64) string { return strconv.FormatFloat(x, 'f', 10, 64) }
	parseDate := func(f []string) (types.DateRec, bool) {
		if len(f) < 4 {
			return types.DateRec{}, false
		}
		y, e1 := strconv.Atoi(f[1])
		mo, e2 := strconv.Atoi(f[2])
		d, e3 := strconv.Atoi(f[3])
		if e1 != nil || e2 != nil || e3 != nil || mo < 1 || mo > 12 || d < 1 {
			return types.DateRec{}, false
		}
		return types.NewDateRec(y+1900, time.Month(mo), d), true
	}

	// Fields exercised, each an independent-target backward solve.
	fields := []string{"rate", "lumpamt", "lumpdate", "asof", "peramt"}
	perYrChoices := []int{1, 2, 4, 12}

	checked, bothRefused, flaked := 0, 0, 0
	goSolvesDosRefuses := 0
	dosSolvesGoRefuses := 0
	valDiverge := 0
	worstVal, worstMsg := 0.0, ""

	for c := 0; c < N; c++ {
		amount := cents(1 + rng.Float64()*1_999_999)
		rate := q6(0.0005 + rng.Float64()*0.40)
		months := 1 + rng.Intn(600)
		perYr := perYrChoices[rng.Intn(len(perYrChoices))]
		n := 1 + rng.Intn(perYr*40)
		// Independent target: a wide multiple of the amount so many draws are
		// unreachable (sv > amount ⇒ negative implied rate / pre-as-of date;
		// sv ≪ amount ⇒ very high rate).
		sv := cents(amount * (0.001 + rng.Float64()*2.5))

		field := fields[rng.Intn(len(fields))]
		var (
			dosOK, goOK     bool
			dosVal, goVal   float64
			isDate          bool
			dosDate, goDate types.DateRec
			args            []string
		)

		switch field {
		case "rate":
			args = []string{"bk_rate", f10(sv), f10(amount), strconv.Itoa(months)}
			f, oc := dosRun("rate", args)
			if oc == dosFlake {
				flaked++
				continue
			}
			dosOK = oc == dosSolved
			if dosOK {
				dosVal, _ = strconv.ParseFloat(f[1], 64)
			}
			goVal, goOK = goBkRate(sv, amount, months)

		case "lumpamt":
			args = []string{"bk_lump_amt", f10(sv), f10(rate), strconv.Itoa(months)}
			f, oc := dosRun("amt", args)
			if oc == dosFlake {
				flaked++
				continue
			}
			dosOK = oc == dosSolved
			if dosOK {
				dosVal, _ = strconv.ParseFloat(f[1], 64)
			}
			goVal, goOK = goBkLumpAmount(sv, rate, months)

		case "lumpdate":
			seed := 1 + rng.Intn(240)
			args = []string{"bk_lump_date", f10(sv), f10(amount), f10(rate), strconv.Itoa(seed)}
			f, oc := dosRun("date", args)
			if oc == dosFlake {
				flaked++
				continue
			}
			dosOK = oc == dosSolved
			isDate = true
			if dosOK {
				dosDate, dosOK = parseDate(f)
			}
			goDate, goOK = goBkLumpDate(sv, amount, rate, seed)

		case "asof":
			args = []string{"bk_asof", f10(sv), f10(amount), f10(rate), strconv.Itoa(months)}
			f, oc := dosRun("asof", args)
			if oc == dosFlake {
				flaked++
				continue
			}
			dosOK = oc == dosSolved
			isDate = true
			if dosOK {
				dosDate, dosOK = parseDate(f)
			}
			goDate, goOK = goBkAsOf(sv, amount, rate, months)

		default: // peramt
			args = []string{"bk_per_amt", f10(sv), f10(rate), strconv.Itoa(perYr), strconv.Itoa(n)}
			f, oc := dosRun("amt", args)
			if oc == dosFlake {
				flaked++
				continue
			}
			dosOK = oc == dosSolved
			if dosOK {
				dosVal, _ = strconv.ParseFloat(f[1], 64)
			}
			goVal, goOK = goBkPeriodicAmount(sv, rate, perYr, n)
		}
		if goOK && !isDate && !finite(goVal) {
			goOK = false
		}
		label := strings.Join(args, " ")

		switch {
		case goOK && !dosOK:
			goSolvesDosRefuses++
			if goSolvesDosRefuses <= 20 {
				if isDate {
					t.Errorf("Go SOLVES where DOS REFUSES (%s): Go=%s | [%s]",
						field, goDate.Time.Format("2006-01-02"), label)
				} else {
					t.Errorf("Go SOLVES where DOS REFUSES (%s): Go=%.6f | [%s]", field, goVal, label)
				}
			}
		case !goOK && dosOK:
			dosSolvesGoRefuses++
		case !goOK && !dosOK:
			bothRefused++
		default:
			checked++
			if isDate {
				dd := math.Abs(dosDate.Time.Sub(goDate.Time).Hours() / 24)
				if dd > 2 { // ±1 day rounding slack at the solved boundary
					valDiverge++
					if dd > worstVal {
						worstVal = dd
						worstMsg = fmt.Sprintf("%s Go=%s DOS=%s days=%.0f [%s]", field,
							goDate.Time.Format("2006-01-02"), dosDate.Time.Format("2006-01-02"), dd, label)
					}
				}
			} else {
				tol := math.Max(0.01, 1e-6*math.Abs(dosVal))
				if d := math.Abs(goVal - dosVal); d > tol {
					rel := d / math.Max(1e-6, math.Abs(dosVal))
					valDiverge++
					if rel > worstVal {
						worstVal = rel
						worstMsg = fmt.Sprintf("%s Go=%.6f DOS=%.6f rel=%.2e [%s]", field, goVal, dosVal, rel, label)
					}
				}
			}
		}
	}
	t.Logf("pv fuzzer3: %d both-solved, %d both-refused, %d flake; "+
		"Go-solves-DOS-refuses=%d (HARD); DOS-solves-Go-refuses=%d (conservative, logged); "+
		"value-diverge=%d worst=%.2e [%s]",
		checked, bothRefused, flaked, goSolvesDosRefuses, dosSolvesGoRefuses, valDiverge, worstVal, worstMsg)
	if checked+bothRefused < N/4 {
		t.Fatalf("only %d/%d cases adjudicated — harness/oracle problem", checked+bothRefused, N)
	}
}
