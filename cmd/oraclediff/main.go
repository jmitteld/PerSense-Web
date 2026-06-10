// Command oraclediff is a differential-testing harness for the Per%Sense
// amortization engine. It generates random loan worksheets, runs the Go
// engine, and compares the results against a pluggable "oracle" — an
// independent reference implementation — reporting and shrinking any
// disagreement to a minimal reproducer.
//
// The point of a differential/binary oracle (docs/fidelity_validation_roadmap.md
// §3) is to escape the "shared transcription error" ceiling of a
// hand-written Pascal reimplementation: the oracle is an *independent*
// authority, ideally the original product itself.
//
// Oracle modes:
//
//	-oracle=self      Go engine vs Go engine. A plumbing sanity check —
//	                  must report zero mismatches.
//	-oracle=mutant    Go engine vs a deliberately-broken variant. Proves
//	                  the harness actually catches and shrinks a
//	                  discrepancy (the broken variant drops one period).
//	-oracle=cmd       Go engine vs an EXTERNAL program named by -cmd. The
//	                  worksheet is written to the program's stdin as JSON;
//	                  it must print the Result JSON to stdout. This is how
//	                  the real authority is wired in: a small wrapper that
//	                  drives legacy Persense.exe (Windows/Wine host) or a
//	                  headless Free Pascal driver linking the legacy
//	                  computational units. See README in this directory.
//
// Example (real oracle on a Windows/Wine host):
//
//	oraclediff -n 5000 -oracle=cmd -cmd "wine persense_oracle.exe" -screen amort
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"time"

	"github.com/persense/persense-port/internal/finance/amortization"
	"github.com/persense/persense-port/internal/types"
)

// Worksheet is a self-contained, serializable amortization scenario —
// the unit of input the generator produces and an oracle consumes.
type Worksheet struct {
	Amount        float64 `json:"amount"`
	Rate          float64 `json:"rate"`
	NPeriods      int     `json:"nPeriods"`
	PerYr         int     `json:"perYr"`
	Payment       float64 `json:"payment"`
	BalloonPeriod int     `json:"balloonPeriod,omitempty"` // 0 = none
	BalloonAmount float64 `json:"balloonAmount,omitempty"`
	PlusRegular   bool    `json:"plusRegular,omitempty"`
}

// Result is the comparable output: a few load-bearing aggregates plus
// the interest/balance at the midpoint, which together pin the schedule
// shape without requiring the whole row-by-row table to match formats.
type Result struct {
	TotalInterest float64 `json:"totalInterest"`
	FinalPrinc    float64 `json:"finalPrinc"`
	Payment       float64 `json:"payment"` // row-1 regular payment
	MidInterest   float64 `json:"midInterest"`
	MidBalance    float64 `json:"midBalance"`
	NRows         int     `json:"nRows"`
}

func (w Worksheet) firstDate(perYr int) types.DateRec {
	switch perYr {
	case 12:
		return types.NewDateRec(2024, 2, 1)
	case 4:
		return types.NewDateRec(2024, 4, 1)
	case 2:
		return types.NewDateRec(2024, 7, 1)
	default:
		return types.NewDateRec(2025, 1, 1)
	}
}

func (w Worksheet) toInput() amortization.LoanInput {
	in := amortization.LoanInput{
		Loan: amortization.Loan{
			AmountStatus:   types.InOutInput, Amount: w.Amount,
			LoanRateStatus: types.InOutInput, LoanRate: w.Rate,
			NStatus:        types.InOutInput, NPeriods: w.NPeriods,
			PerYrStatus:    types.InOutInput, PerYr: w.PerYr,
			PayAmtStatus:   types.InOutInput, PayAmt: w.Payment,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus:    types.InOutInput, FirstDate: w.firstDate(w.PerYr),
		},
		Settings: amortization.Settings{
			Basis: types.Basis360, PerYr: byte(w.PerYr), YrDays: 360, YrInv: 1.0 / 360,
			PlusRegular: w.PlusRegular,
		},
	}
	if w.BalloonPeriod > 0 && w.PerYr == 12 {
		// Balloon on a real monthly payment date (period BalloonPeriod).
		in.Balloons = []amortization.BalloonPayment{{
			DateStatus: types.InOutInput, Date: monthlyDate(w.BalloonPeriod),
			AmountStatus: types.InOutInput, Amount: w.BalloonAmount,
		}}
		in.Fancy = true
	}
	return in
}

func monthlyDate(period int) types.DateRec {
	// First payment 2024-02-01; period k is +(k-1) months.
	y, m := 2024, 2+(period-1)
	y += (m - 1) / 12
	m = (m-1)%12 + 1
	return types.NewDateRec(y, time.Month(m), 1)
}

// goEval is the system under test: the real Go amortization engine.
func goEval(w Worksheet) (Result, error) {
	res := amortization.Amortize(w.toInput())
	if res.Err != nil {
		return Result{}, res.Err
	}
	mid := len(res.Schedule) / 2
	r := Result{
		TotalInterest: res.TotalInt,
		FinalPrinc:    res.FinalPrinc,
		NRows:         len(res.Schedule),
	}
	if len(res.Schedule) > 0 {
		r.Payment = res.Schedule[0].PayAmt
		r.MidInterest = res.Schedule[mid].Interest
		r.MidBalance = res.Schedule[mid].Principal
	}
	return r, nil
}

// Oracle is a reference implementation the engine is compared against.
type Oracle func(Worksheet) (Result, error)

// mutantOracle is a deliberately-broken reference: it amortizes one
// period short. Used to prove the harness detects and shrinks a real
// discrepancy.
func mutantOracle(w Worksheet) (Result, error) {
	w.NPeriods--
	return goEval(w)
}

// cmdOracle runs an external program: worksheet JSON on stdin, Result
// JSON on stdout. This is the slot for legacy Persense.exe (via a
// wrapper) or a headless Free Pascal driver.
func cmdOracle(command string) Oracle {
	return func(w Worksheet) (Result, error) {
		cmd := exec.Command("sh", "-c", command)
		in, _ := json.Marshal(w)
		cmd.Stdin = bytes.NewReader(in)
		out, err := cmd.Output()
		if err != nil {
			return Result{}, fmt.Errorf("oracle command failed: %w", err)
		}
		var r Result
		if err := json.Unmarshal(out, &r); err != nil {
			return Result{}, fmt.Errorf("oracle output not JSON Result: %w", err)
		}
		return r, nil
	}
}

func genWorksheet(rng *rand.Rand) Worksheet {
	perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
	amount := float64(10000 + rng.Intn(990000))
	rate := 0.01 + rng.Float64()*0.14
	years := 3 + rng.Intn(27)
	n := years * perYr
	f := 1 + rate/float64(perYr)
	// A roughly amortizing payment.
	pay := amount * (f - 1) / (1 - math.Pow(f, -float64(n)))
	w := Worksheet{Amount: amount, Rate: rate, NPeriods: n, PerYr: perYr, Payment: pay}
	if perYr == 12 && n >= 24 && rng.Intn(2) == 0 {
		w.BalloonPeriod = n/4 + rng.Intn(n/2)
		w.BalloonAmount = amount * (0.05 + 0.2*rng.Float64())
		w.PlusRegular = rng.Intn(2) == 0
	}
	return w
}

// mismatches lists the fields where got and want differ beyond tolerance.
func mismatches(got, want Result) []string {
	var ms []string
	cmp := func(name string, a, b, tol float64) {
		if math.Abs(a-b) > tol+1e-7*math.Abs(b) {
			ms = append(ms, fmt.Sprintf("%s: go=%.6f oracle=%.6f Δ=%.6f", name, a, b, a-b))
		}
	}
	cmp("totalInterest", got.TotalInterest, want.TotalInterest, 0.02)
	cmp("finalPrinc", got.FinalPrinc, want.FinalPrinc, 0.02)
	cmp("payment", got.Payment, want.Payment, 0.01)
	cmp("midInterest", got.MidInterest, want.MidInterest, 0.01)
	cmp("midBalance", got.MidBalance, want.MidBalance, 0.02)
	if got.NRows != want.NRows {
		ms = append(ms, fmt.Sprintf("nRows: go=%d oracle=%d", got.NRows, want.NRows))
	}
	return ms
}

// fails reports whether the engine and oracle disagree on w.
func fails(w Worksheet, oracle Oracle) bool {
	g, gerr := goEval(w)
	o, oerr := oracle(w)
	if (gerr == nil) != (oerr == nil) {
		return true
	}
	if gerr != nil {
		return false // both errored — not a value mismatch
	}
	return len(mismatches(g, o)) > 0
}

// shrink greedily simplifies a failing worksheet while it keeps failing,
// to produce a minimal reproducer.
func shrink(w Worksheet, oracle Oracle) Worksheet {
	changed := true
	for changed {
		changed = false
		cands := []Worksheet{}
		if w.BalloonPeriod > 0 {
			c := w
			c.BalloonPeriod, c.BalloonAmount = 0, 0
			cands = append(cands, c)
		}
		if w.NPeriods > w.PerYr*2 {
			c := w
			c.NPeriods = w.NPeriods - w.PerYr
			f := 1 + c.Rate/float64(c.PerYr)
			c.Payment = c.Amount * (f - 1) / (1 - math.Pow(f, -float64(c.NPeriods)))
			cands = append(cands, c)
		}
		if w.Amount > 20000 {
			c := w
			c.Amount = math.Round(w.Amount/2/1000) * 1000
			f := 1 + c.Rate/float64(c.PerYr)
			c.Payment = c.Amount * (f - 1) / (1 - math.Pow(f, -float64(c.NPeriods)))
			cands = append(cands, c)
		}
		for _, c := range cands {
			if fails(c, oracle) {
				w, changed = c, true
				break
			}
		}
	}
	return w
}

func main() {
	n := flag.Int("n", 2000, "number of random worksheets")
	seed := flag.Int64("seed", 1, "PRNG seed")
	mode := flag.String("oracle", "self", "oracle: self | mutant | cmd")
	command := flag.String("cmd", "", "external oracle command (for -oracle=cmd)")
	flag.Parse()

	var oracle Oracle
	switch *mode {
	case "self":
		oracle = goEval
	case "mutant":
		oracle = mutantOracle
	case "cmd":
		if *command == "" {
			fmt.Fprintln(os.Stderr, "-oracle=cmd requires -cmd")
			os.Exit(2)
		}
		oracle = cmdOracle(*command)
	default:
		fmt.Fprintf(os.Stderr, "unknown oracle mode %q\n", *mode)
		os.Exit(2)
	}

	rng := rand.New(rand.NewSource(*seed))
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	fmt.Fprintf(w, "oraclediff: %d worksheets, oracle=%s, seed=%d\n", *n, *mode, *seed)
	checked, failed := 0, 0
	for i := 0; i < *n; i++ {
		ws := genWorksheet(rng)
		g, gerr := goEval(ws)
		o, oerr := oracle(ws)
		if gerr != nil || oerr != nil {
			continue // skip cases either side legitimately rejects
		}
		checked++
		ms := mismatches(g, o)
		if len(ms) == 0 {
			continue
		}
		failed++
		min := shrink(ws, oracle)
		fmt.Fprintf(w, "\nMISMATCH (worksheet %d):\n  generated: %+v\n  shrunk:    %+v\n", i, ws, min)
		for _, m := range ms {
			fmt.Fprintf(w, "    %s\n", m)
		}
		if failed >= 5 {
			fmt.Fprintf(w, "\n(stopping after 5 reported mismatches)\n")
			break
		}
	}
	fmt.Fprintf(w, "\nchecked %d comparable worksheets, %d mismatches\n", checked, failed)
	if failed > 0 {
		w.Flush()
		os.Exit(1)
	}
}
