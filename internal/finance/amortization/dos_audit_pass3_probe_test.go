// PASS-3 audit PROBE harness (backward solves) — 2026-07-12.
// NOT a regression file: it drives fresh oracle-vs-Go comparisons across the
// pass-3 probe areas and LOGS divergences (grep for "DIVERGE"). Findings that
// survive a fresh minimal reproduction get promoted to real regression tests.
package amortization

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// ---------- shared probe plumbing ----------

var p3LoanDate = types.NewDateRec(2024, time.January, 1)

func p3MonthsAfter(base types.DateRec, months int) types.DateRec {
	y, m, d := base.Time.Year(), int(base.Time.Month()), base.Time.Day()
	tot := (m - 1) + months
	return types.NewDateRec(y+tot/12, time.Month(tot%12+1), d)
}

// p3FirstDate mirrors the oracle SetupLoan default first-payment date.
func p3FirstDate(perYr int) (types.DateRec, []string) {
	switch perYr {
	case 26:
		return types.NewDateRec(2024, time.January, 15), nil
	case 52:
		return types.NewDateRec(2024, time.January, 8), nil
	case 24:
		// oracle's month arithmetic degenerates at 24/yr; pin the 1/16 grid
		return types.NewDateRec(2024, time.January, 16), []string{"firstdmy=16.1.2024"}
	default:
		return p3MonthsAfter(p3LoanDate, 12/perYr), nil
	}
}

func p3Settings(basis string, perYr int) Settings {
	s := Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360}
	switch basis {
	case "b365":
		s.Basis, s.YrDays, s.YrInv = types.Basis365, 365.25, 1/365.25
	case "b365_360":
		s.Basis, s.YrDays, s.YrInv = types.Basis365360, 360, 1.0/360
	}
	return s
}

func p3BasisTok(basis string) []string {
	if basis == "360" {
		return nil
	}
	return []string{basis}
}

// p3Oracle runs the oracle with a hard 10s timeout and heap-glitch retry.
// Returns every whitespace token of the full output, flattened.
func p3Oracle(t *testing.T, args ...string) ([]string, bool) {
	t.Helper()
	for try := 0; try < 8; try++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		out, err := exec.CommandContext(ctx, oracleBin, args...).Output()
		cancel()
		if err != nil {
			continue
		}
		f := strings.Fields(string(out))
		if len(f) == 0 {
			continue
		}
		// heap glitch: a solved payment of 0 on a should-solve call
		if f[0] == "payment" && f[1] == "0.0000" && !hasTok(args, "payhard=") && !hasTok(args, "pay=") {
			continue
		}
		return f, true
	}
	return nil, false
}

func hasTok(args []string, pfx string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, pfx) {
			return true
		}
	}
	return false
}

// p3Val extracts the token following key, parsed as float.
func p3Val(f []string, key string) (float64, bool) {
	for i, tok := range f {
		if tok == key && i+1 < len(f) {
			v, err := strconv.ParseFloat(f[i+1], 64)
			if err == nil {
				return v, true
			}
		}
	}
	return 0, false
}

func p3Str(f []string, key string) (string, bool) {
	for i, tok := range f {
		if tok == key && i+1 < len(f) {
			return f[i+1], true
		}
	}
	return "", false
}

func p3IsErr(f []string) bool { return len(f) > 0 && f[0] == "ERR" }

// p3Rows parses `rows` output into (label, int, prin, bal) tuples.
type p3Row struct {
	label           string
	intp, prin, bal float64
}

func p3ParseRows(f []string) []p3Row {
	var rows []p3Row
	for i := 0; i+7 < len(f); i++ {
		if f[i] == "row" && f[i+2] == "int" && f[i+4] == "prin" && f[i+6] == "bal" {
			iv, _ := strconv.ParseFloat(f[i+3], 64)
			pv, _ := strconv.ParseFloat(f[i+5], 64)
			bv, _ := strconv.ParseFloat(f[i+7], 64)
			rows = append(rows, p3Row{f[i+1], iv, pv, bv})
		}
	}
	return rows
}

// p3Loan builds the base loan record matching the oracle defaults.
func p3Loan(amount, rate, pay float64, n, perYr int, first types.DateRec) Loan {
	l := Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n,
		PerYrStatus: types.InOutInput, PerYr: perYr,
		LoanDateStatus: types.InOutInput, LoanDate: p3LoanDate,
		FirstStatus: types.InOutInput, FirstDate: first,
	}
	if pay > 0 {
		l.PayAmtStatus, l.PayAmt = types.InOutInput, pay
	} else {
		l.PayAmtStatus = types.StatusEmpty
	}
	return l
}

func p3ApplyFlag(s *Settings, flag string) {
	switch flag {
	case "prepaid":
		s.Prepaid = true
	case "inadv":
		s.InAdvance = true
	case "r78":
		s.R78 = true
	case "usa":
		s.USARule = true
	case "exact":
		s.Exact = true
	}
}

// ---------- AREA 1: TERM solves ----------

func TestPass3A1TermSolves(t *testing.T) {
	gateOracle(t)
	checked, diverged := 0, 0
	flags := []string{"", "prepaid", "inadv", "r78", "usa", "exact"}
	freqs := []int{1, 4, 12, 24, 26, 52}
	for _, perYr := range freqs {
		var bases []string
		if perYr == 24 || perYr == 26 || perYr == 52 {
			bases = []string{"b365", "b365_360"}
		} else {
			bases = []string{"360", "b365", "b365_360"}
		}
		first, xtoks := p3FirstDate(perYr)
		for _, basis := range bases {
			for _, flag := range flags {
				// 2-year seed term (5 for annual)
				n0 := 2 * perYr
				if perYr == 1 {
					n0 = 5
				}
				amount, rate := 10000.0, 0.09
				base := []string{fmt.Sprintf("%.2f", amount), fmt.Sprintf("%.6f", rate)}
				seedArgs := append(append([]string{base[0], base[1], strconv.Itoa(n0), strconv.Itoa(perYr)}, xtoks...), p3BasisTok(basis)...)
				if flag != "" {
					seedArgs = append(seedArgs, flag)
				}
				sf, ok := p3Oracle(t, seedArgs...)
				if !ok || p3IsErr(sf) {
					t.Logf("SKIP seed %v: %v", seedArgs, sf)
					continue
				}
				p0, _ := p3Val(sf, "payment")
				if p0 <= 0 {
					t.Logf("SKIP seed %v: payment %v", seedArgs, p0)
					continue
				}
				// (a) exact-retire-ish payment, (b) fractional-tail payment
				for pi, pay := range []float64{math.Round(p0*100) / 100, math.Round(p0*1.37*100) / 100} {
					args := append(append([]string{base[0], base[1], "0", strconv.Itoa(perYr)}, xtoks...), p3BasisTok(basis)...)
					if flag != "" {
						args = append(args, flag)
					}
					args = append(args, "payhard="+fmt.Sprintf("%.2f", pay), "noterm")
					f, ok := p3Oracle(t, args...)
					if !ok {
						t.Logf("SKIP noterm no-output %v", args)
						continue
					}
					lbl := fmt.Sprintf("py=%d %s %s pay#%d=%.2f", perYr, basis, flag, pi, pay)
					// Go side
					loan := p3Loan(amount, rate, pay, 0, perYr, first)
					loan.NStatus, loan.NPeriods = types.StatusEmpty, 0
					loan.LastStatus = types.StatusEmpty
					res := Amortize(LoanInput{Loan: loan, Settings: func() Settings { s := p3Settings(basis, perYr); p3ApplyFlag(&s, flag); return s }()})
					checked++
					if p3IsErr(f) {
						if res.Err == nil {
							diverged++
							t.Logf("DIVERGE A1 %s: DOS %v | Go solved n=%d int=%.2f", lbl, f, res.NPeriods, res.TotalInt)
						}
						continue
					}
					if res.Err != nil {
						diverged++
						t.Logf("DIVERGE A1 %s: DOS ok %v | Go err %v", lbl, f, res.Err)
						continue
					}
					dn, _ := p3Val(f, "solvedterm")
					dosLast, _ := p3Str(f, "last")
					dInt, _ := p3Val(f, "interest")
					dPaid, _ := p3Val(f, "paid")
					goLast := fmt.Sprintf("%d-%d-%d", res.LastDate.Time.Year(), int(res.LastDate.Time.Month()), res.LastDate.Time.Day())
					if int(dn) != res.NPeriods || dosLast != goLast ||
						math.Abs(dInt-res.TotalInt) > 0.011 || math.Abs(dPaid-res.TotalPaid) > 0.011 {
						diverged++
						t.Logf("DIVERGE A1 %s: DOS n=%d last=%s int=%.2f paid=%.2f | Go n=%d last=%s int=%.2f paid=%.2f",
							lbl, int(dn), dosLast, dInt, dPaid, res.NPeriods, goLast, res.TotalInt, res.TotalPaid)
					}
				}
			}
		}
	}
	t.Logf("A1 term solves: %d checked, %d diverged", checked, diverged)
}

// ---------- AREA 2: BALLOON solves ----------

func TestPass3A2BalloonSolves(t *testing.T) {
	gateOracle(t)
	checked, diverged := 0, 0
	type cse struct {
		name  string
		n     int
		bm    int // balloon months after loan date
		pay   float64
		basis string
		flags []string
		adj   string // adj token or ""
		pre   string // pre token or ""
	}
	var cases []cse
	for _, basis := range []string{"360", "b365", "b365_360"} {
		for _, fl := range [][]string{nil, {"exact"}, {"inadv"}, {"prepaid"}} {
			// mid-term and terminating
			cases = append(cases,
				cse{"mid", 120, 37, 1050, basis, fl, "", ""},
				cse{"term", 60, 60, 1500, basis, fl, "", ""},
			)
		}
	}
	// coexisting adjustment / prepay series (360 + b365)
	for _, basis := range []string{"360", "b365"} {
		cases = append(cases,
			cse{"mid+adj", 120, 60, 1050, basis, nil, "adj=24:0.09:", ""},
			cse{"term+adj", 60, 60, 1500, basis, nil, "adj=24:0.09:", ""},
			cse{"mid+pre", 120, 60, 1050, basis, nil, "", "pre=6:12:12:200"},
			cse{"term+pre", 60, 60, 1500, basis, nil, "", "pre=6:12:12:200"},
			cse{"mid+adj+exact", 120, 60, 1050, basis, []string{"exact"}, "adj=24:0.09:", ""},
			cse{"mid+pre+inadv", 120, 60, 1050, basis, []string{"inadv"}, "", "pre=6:12:12:200"},
		)
	}
	amount, rate := 100000.0, 0.08
	for _, c := range cases {
		args := []string{fmt.Sprintf("%.2f", amount), fmt.Sprintf("%.6f", rate), strconv.Itoa(c.n), "12"}
		args = append(args, p3BasisTok(c.basis)...)
		args = append(args, c.flags...)
		if c.adj != "" {
			args = append(args, c.adj)
		}
		if c.pre != "" {
			args = append(args, c.pre)
		}
		args = append(args, "payhard="+fmt.Sprintf("%.2f", c.pay), "solveballoon="+strconv.Itoa(c.bm))
		f, ok := p3Oracle(t, args...)
		lbl := fmt.Sprintf("%s %s %v pay=%.2f", c.name, c.basis, c.flags, c.pay)
		if !ok {
			t.Logf("SKIP A2 %s: no output", lbl)
			continue
		}
		// Go side
		in := LoanInput{
			Loan: p3Loan(amount, rate, c.pay, c.n, 12, p3MonthsAfter(p3LoanDate, 1)),
			Balloons: []BalloonPayment{{
				DateStatus: types.InOutInput, Date: p3MonthsAfter(p3LoanDate, c.bm),
				AmountStatus: types.StatusEmpty,
			}},
			Settings: p3Settings(c.basis, 12),
			Fancy:    true,
		}
		for _, fl := range c.flags {
			p3ApplyFlag(&in.Settings, fl)
		}
		if c.adj != "" {
			in.Adjustments = []RateAdjustment{{
				DateStatus: types.InOutInput, Date: p3MonthsAfter(p3LoanDate, 24),
				LoanRateStatus: types.InOutInput, LoanRate: 0.09,
			}}
		}
		if c.pre != "" {
			in.Prepayments = []Prepayment{{
				StartDateStatus: types.InOutInput, StartDate: p3MonthsAfter(p3LoanDate, 6),
				NNStatus: types.InOutInput, NN: 12,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PaymentStatus: types.InOutInput, Payment: 200,
			}}
		}
		goAmt, goErr := SolveBalloonAmount(in, 0)
		checked++
		if p3IsErr(f) {
			if goErr == nil {
				diverged++
				t.Logf("DIVERGE A2 %s: DOS %v | Go %.4f", lbl, f, goAmt)
			}
			continue
		}
		dosAmt, dok := p3Val(f, "balloon")
		if !dok {
			t.Logf("SKIP A2 %s: unparsed %v", lbl, f)
			continue
		}
		if goErr != nil {
			diverged++
			t.Logf("DIVERGE A2 %s: DOS %.4f | Go err %v", lbl, dosAmt, goErr)
			continue
		}
		tol := 0.01 + 1e-6*math.Abs(dosAmt)
		if math.Abs(goAmt-dosAmt) > tol {
			diverged++
			t.Logf("DIVERGE A2 %s: DOS %.4f | Go %.4f (Δ=%+.4f)", lbl, dosAmt, goAmt, goAmt-dosAmt)
		}
	}
	t.Logf("A2 balloon solves: %d checked, %d diverged", checked, diverged)
}

// ---------- AREA 3: datefrombalance= / dateballoon= ----------

func TestPass3A3DateEvals(t *testing.T) {
	gateOracle(t)
	checked, diverged := 0, 0
	amount, rate, n := 100000.0, 0.10, 120
	pay := 1500.0
	type cfg struct {
		name  string
		basis string
		flags []string
	}
	cfgs := []cfg{
		{"plain", "360", nil},
		{"prepaid", "360", []string{"prepaid"}},
		{"inadv", "360", []string{"inadv"}},
		{"r78", "360", []string{"r78"}},
		{"usa", "360", []string{"usa"}},
		{"exact365", "b365", []string{"exact"}},
		{"b365", "b365", nil},
		{"b365_360", "b365_360", nil},
		{"prepaid365", "b365", []string{"prepaid"}},
		{"inadv365", "b365", []string{"inadv"}},
	}
	for _, c := range cfgs {
		set := p3Settings(c.basis, 12)
		for _, fl := range c.flags {
			p3ApplyFlag(&set, fl)
		}
		in := LoanInput{
			Loan:     p3Loan(amount, rate, pay, n, 12, p3MonthsAfter(p3LoanDate, 1)),
			Settings: set,
		}
		res := Amortize(in)
		if res.Err != nil {
			t.Logf("SKIP A3 %s: Go forward err %v", c.name, res.Err)
			continue
		}
		for _, tgt := range []float64{85000, 55000, 25000, 5000} {
			args := []string{fmt.Sprintf("%.2f", amount), fmt.Sprintf("%.6f", rate), strconv.Itoa(n), "12"}
			args = append(args, p3BasisTok(c.basis)...)
			args = append(args, c.flags...)
			args = append(args, "payhard="+fmt.Sprintf("%.2f", pay), "datefrombalance="+fmt.Sprintf("%.2f", tgt))
			f, ok := p3Oracle(t, args...)
			lbl := fmt.Sprintf("%s tgt=%.0f", c.name, tgt)
			if !ok {
				t.Logf("SKIP A3 %s: no output", lbl)
				continue
			}
			checked++
			gd, _, gok := ComputeDateFromBalanceDOS(res.Schedule, tgt, set.InAdvance)
			if p3IsErr(f) {
				if gok {
					diverged++
					t.Logf("DIVERGE A3 %s: DOS %v | Go %v", lbl, f, gd.Time.Format("2006-01-02"))
				}
				continue
			}
			ds, dok := p3Str(f, "date")
			if !dok {
				t.Logf("SKIP A3 %s: unparsed %v", lbl, f)
				continue
			}
			if !gok {
				diverged++
				t.Logf("DIVERGE A3 %s: DOS %s | Go not-reached", lbl, ds)
				continue
			}
			goDs := fmt.Sprintf("%d/%d/%d", int(gd.Time.Month()), gd.Time.Day(), gd.Time.Year())
			if ds != goDs {
				diverged++
				t.Logf("DIVERGE A3 %s: DOS %s | Go %s", lbl, ds, goDs)
			}
		}
	}
	t.Logf("A3 date-from-balance: %d checked, %d diverged", checked, diverged)
}

// ---------- AREA 4: AMOUNT and RATE solves × fancy options ----------

func TestPass3A4AmountRateFancy(t *testing.T) {
	gateOracle(t)
	checked, diverged := 0, 0
	type cse struct {
		name  string
		basis string
		flags []string
		toks  []string
		apply func(in *LoanInput)
	}
	adj1 := func(m int, r float64) func(*LoanInput) {
		return func(in *LoanInput) {
			in.Fancy = true
			in.Adjustments = append(in.Adjustments, RateAdjustment{
				DateStatus: types.InOutInput, Date: p3MonthsAfter(p3LoanDate, m),
				LoanRateStatus: types.InOutInput, LoanRate: r,
			})
		}
	}
	preS := func(start, nn, py int, amt float64) func(*LoanInput) {
		return func(in *LoanInput) {
			in.Fancy = true
			in.Prepayments = append(in.Prepayments, Prepayment{
				StartDateStatus: types.InOutInput, StartDate: p3MonthsAfter(p3LoanDate, start),
				NNStatus: types.InOutInput, NN: nn,
				PerYrStatus: types.InOutInput, PerYr: py,
				PaymentStatus: types.InOutInput, Payment: amt,
			})
		}
	}
	skipO := func(str string) func(*LoanInput) {
		return func(in *LoanInput) {
			in.Fancy = true
			ms, _ := MonthSetFromString(str)
			in.SkipMonths = SkipMonths{SkipStatus: types.InOutInput, SkipStr: str, MonthSet: ms}
		}
	}
	targO := func(x float64) func(*LoanInput) {
		return func(in *LoanInput) {
			in.Fancy = true
			in.Target = Target{TargetStatus: types.InOutInput, TargetValue: x}
		}
	}
	morO := func(m int) func(*LoanInput) {
		return func(in *LoanInput) {
			in.Fancy = true
			in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: p3MonthsAfter(p3LoanDate, m)}
		}
	}
	ballO := func(m int, amt float64) func(*LoanInput) {
		return func(in *LoanInput) {
			in.Fancy = true
			in.Balloons = append(in.Balloons, BalloonPayment{
				DateStatus: types.InOutInput, Date: p3MonthsAfter(p3LoanDate, m),
				AmountStatus: types.InOutInput, Amount: amt,
			})
		}
	}
	compose := func(fs ...func(*LoanInput)) func(*LoanInput) {
		return func(in *LoanInput) {
			for _, f := range fs {
				f(in)
			}
		}
	}
	var cases []cse
	for _, basis := range []string{"360", "b365", "b365_360"} {
		for _, fl := range [][]string{nil, {"inadv"}, {"exact"}} {
			cases = append(cases,
				cse{"skip", basis, fl, []string{"skip=6-8"}, skipO("6-8")},
				cse{"targ", basis, fl, []string{"targ=400"}, targO(400)},
				cse{"adj", basis, fl, []string{"adj=24:0.10:"}, adj1(24, 0.10)},
				cse{"pre", basis, fl, []string{"pre=6:12:12:300"}, preS(6, 12, 12, 300)},
			)
		}
		cases = append(cases,
			cse{"twoARMs", basis, nil, []string{"adj=24:0.10:", "adj=48:0.06:"}, compose(adj1(24, 0.10), adj1(48, 0.06))},
			cse{"balloon+mor", basis, nil, []string{"mor=12", "b60=15000"}, compose(morO(12), ballO(60, 15000))},
		)
	}
	amount, rate, n := 100000.0, 0.08, 120
	for _, c := range cases {
		// One canonical payment per case: DOS's own forward solve.
		fargs := []string{fmt.Sprintf("%.2f", amount), fmt.Sprintf("%.6f", rate), strconv.Itoa(n), "12"}
		fargs = append(fargs, p3BasisTok(c.basis)...)
		fargs = append(fargs, c.flags...)
		fargs = append(fargs, c.toks...)
		sf, ok := p3Oracle(t, fargs...)
		if !ok || p3IsErr(sf) {
			t.Logf("SKIP A4 seed %s %s %v: %v", c.name, c.basis, c.flags, sf)
			continue
		}
		p0, _ := p3Val(sf, "payment")
		if p0 <= 0 {
			t.Logf("SKIP A4 seed %s %s %v: pay %v", c.name, c.basis, c.flags, p0)
			continue
		}
		pay := math.Round(p0*10000) / 10000
		for _, solve := range []string{"noamt", "norate"} {
			args := make([]string, 0, 16)
			if solve == "noamt" {
				args = append(args, "0", fmt.Sprintf("%.6f", rate))
			} else {
				args = append(args, fmt.Sprintf("%.2f", amount), "0")
			}
			args = append(args, strconv.Itoa(n), "12")
			args = append(args, p3BasisTok(c.basis)...)
			args = append(args, c.flags...)
			args = append(args, c.toks...)
			args = append(args, "pay="+fmt.Sprintf("%.4f", pay), solve)
			f, ok := p3Oracle(t, args...)
			lbl := fmt.Sprintf("%s %s %v %s", c.name, c.basis, c.flags, solve)
			if !ok {
				t.Logf("SKIP A4 %s: no output", lbl)
				continue
			}
			in := LoanInput{
				Loan:     p3Loan(amount, rate, 0, n, 12, p3MonthsAfter(p3LoanDate, 1)),
				Settings: p3Settings(c.basis, 12),
			}
			for _, fl := range c.flags {
				p3ApplyFlag(&in.Settings, fl)
			}
			c.apply(&in)
			in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutDefault, pay
			checked++
			if solve == "noamt" {
				in.Loan.AmountStatus, in.Loan.Amount = types.StatusEmpty, 0
				goV, _, goErr := SolveLoanAmount(in)
				if p3IsErr(f) {
					if goErr == nil {
						diverged++
						t.Logf("DIVERGE A4 %s: DOS %v | Go %.4f", lbl, f, goV)
					}
					continue
				}
				dosV, dok := p3Val(f, "solvedamount")
				if !dok {
					t.Logf("SKIP A4 %s: unparsed %v", lbl, f)
					continue
				}
				if goErr != nil {
					diverged++
					t.Logf("DIVERGE A4 %s: DOS %.4f | Go err %v", lbl, dosV, goErr)
					continue
				}
				tol := 0.01 + 2e-6*math.Abs(dosV)
				if math.Abs(goV-dosV) > tol {
					diverged++
					t.Logf("DIVERGE A4 %s: DOS %.4f | Go %.4f (Δ=%+.4f)", lbl, dosV, goV, goV-dosV)
				}
			} else {
				in.Loan.LoanRateStatus, in.Loan.LoanRate = types.StatusEmpty, 0
				goV, _, goErr := SolveRate(in)
				if p3IsErr(f) {
					if goErr == nil {
						diverged++
						t.Logf("DIVERGE A4 %s: DOS %v | Go %.6f", lbl, f, goV)
					}
					continue
				}
				dosV, dok := p3Val(f, "solvedrate")
				if !dok {
					t.Logf("SKIP A4 %s: unparsed %v", lbl, f)
					continue
				}
				if goErr != nil {
					diverged++
					t.Logf("DIVERGE A4 %s: DOS %.6f | Go err %v", lbl, dosV, goErr)
					continue
				}
				if math.Abs(goV-dosV) > 5e-6*math.Max(1, math.Abs(dosV)/0.01) {
					// 5e-6 relative on rates (rates ~0.08 → tol ~4e-7 abs is too tight for
					// bisection stop; use 5e-6 absolute, ~6e-5 relative)
					if math.Abs(goV-dosV) > 5e-6 {
						diverged++
						t.Logf("DIVERGE A4 %s: DOS %.8f | Go %.8f (Δ=%+.2e)", lbl, dosV, goV, goV-dosV)
					}
				}
			}
		}
	}
	t.Logf("A4 amount/rate fancy solves: %d checked, %d diverged", checked, diverged)
}

// ---------- AREA 5: payment solves with unusual first periods ----------

func TestPass3A5OddFirstPaymentSolves(t *testing.T) {
	gateOracle(t)
	checked, diverged := 0, 0
	type stub struct {
		name          string
		loanD, firstD types.DateRec
	}
	stubs := []stub{
		{"3yrOut", types.NewDateRec(2024, time.January, 1), types.NewDateRec(2027, time.January, 1)},
		{"loan+1d", types.NewDateRec(2024, time.January, 1), types.NewDateRec(2024, time.January, 2)},
		{"oddday", types.NewDateRec(2024, time.January, 5), types.NewDateRec(2024, time.January, 20)},
	}
	for _, st := range stubs {
		for _, perYr := range []int{12, 24, 26, 52} {
			var bases []string
			if perYr == 12 {
				bases = []string{"360", "b365"}
			} else {
				bases = []string{"b365", "b365_360"}
			}
			for _, basis := range bases {
				for _, flag := range []string{"", "prepaid", "exact", "inadv"} {
					n := 2 * perYr
					amount, rate := 50000.0, 0.11
					args := []string{fmt.Sprintf("%.2f", amount), fmt.Sprintf("%.6f", rate), strconv.Itoa(n), strconv.Itoa(perYr)}
					args = append(args, p3BasisTok(basis)...)
					if flag != "" {
						args = append(args, flag)
					}
					args = append(args,
						fmt.Sprintf("loandmy=%d.%d.%d", st.loanD.Time.Day(), int(st.loanD.Time.Month()), st.loanD.Time.Year()),
						fmt.Sprintf("firstdmy=%d.%d.%d", st.firstD.Time.Day(), int(st.firstD.Time.Month()), st.firstD.Time.Year()))
					f, ok := p3Oracle(t, args...)
					lbl := fmt.Sprintf("%s py=%d %s %s", st.name, perYr, basis, flag)
					if !ok {
						t.Logf("SKIP A5 %s: no output", lbl)
						continue
					}
					set := p3Settings(basis, perYr)
					if flag != "" {
						p3ApplyFlag(&set, flag)
					}
					loan := p3Loan(amount, rate, 0, n, perYr, st.firstD)
					loan.LoanDate = st.loanD
					res := Amortize(LoanInput{Loan: loan, Settings: set})
					checked++
					if p3IsErr(f) {
						if res.Err == nil {
							diverged++
							t.Logf("DIVERGE A5 %s: DOS %v | Go pay ok int=%.2f", lbl, f, res.TotalInt)
						}
						continue
					}
					dp, _ := p3Val(f, "payment")
					dInt, _ := p3Val(f, "interest")
					if res.Err != nil {
						diverged++
						t.Logf("DIVERGE A5 %s: DOS pay=%.4f | Go err %v", lbl, dp, res.Err)
						continue
					}
					gp := modalReg(res.Schedule)
					if math.Abs(gp-dp) > 0.005 || math.Abs(res.TotalInt-dInt) > 0.011 {
						diverged++
						t.Logf("DIVERGE A5 %s: DOS pay=%.4f int=%.2f | Go pay=%.4f int=%.2f (Δpay=%+.4f)",
							lbl, dp, dInt, gp, res.TotalInt, gp-dp)
					}
				}
			}
		}
	}
	t.Logf("A5 odd-first payment solves: %d checked, %d diverged", checked, diverged)
}

// ---------- AREA 6: fresh-seed fuzz (seed 20260712) ----------

func TestPass3A6Fuzz(t *testing.T) {
	gateOracle(t)
	rng := rand.New(rand.NewSource(20260712))
	checked, diverged := 0, 0
	for c := 0; c < 150; c++ {
		amount := 1000 + rng.Float64()*499000
		rate := 0.005 + rng.Float64()*0.295
		perYr := []int{12, 24, 26, 52}[rng.Intn(4)]
		n := 6 + rng.Intn(355)
		if n > 30*perYr {
			n = 30 * perYr
		}
		var basis string
		if perYr == 26 || perYr == 52 {
			basis = []string{"b365", "b365_360"}[rng.Intn(2)]
		} else {
			basis = []string{"360", "b365", "b365_360"}[rng.Intn(3)]
		}
		// one random option
		optNames := []string{"none", "prepaid", "inadv", "r78", "usa", "exact", "balloon", "adj", "pre", "skip", "mor", "targ"}
		opt := optNames[rng.Intn(len(optNames))]
		if perYr != 12 && (opt == "adj" || opt == "pre" || opt == "skip" || opt == "mor" || opt == "targ" || opt == "balloon") {
			opt = []string{"none", "prepaid", "inadv", "r78", "usa", "exact"}[rng.Intn(6)]
		}
		first, xtoks := p3FirstDate(perYr)
		args := []string{fmt.Sprintf("%.2f", amount), fmt.Sprintf("%.8f", rate), strconv.Itoa(n), strconv.Itoa(perYr)}
		args = append(args, xtoks...)
		args = append(args, p3BasisTok(basis)...)
		set := p3Settings(basis, perYr)
		in := LoanInput{Loan: p3Loan(amount, rate, 0, n, perYr, first), Settings: set}
		switch opt {
		case "none":
		case "prepaid", "inadv", "r78", "usa", "exact":
			args = append(args, opt)
			p3ApplyFlag(&in.Settings, opt)
		case "balloon":
			bm := 6 + rng.Intn(n-6+1)
			amt := math.Round(amount * (0.05 + rng.Float64()*0.3))
			args = append(args, fmt.Sprintf("b%d=%.0f", bm, amt))
			in.Fancy = true
			in.Balloons = []BalloonPayment{{DateStatus: types.InOutInput, Date: p3MonthsAfter(p3LoanDate, bm),
				AmountStatus: types.InOutInput, Amount: amt}}
		case "adj":
			am := 3 + rng.Intn(n-3)
			r2 := 0.01 + rng.Float64()*0.2
			args = append(args, fmt.Sprintf("adj=%d:%.6f:", am, r2))
			in.Fancy = true
			in.Adjustments = []RateAdjustment{{DateStatus: types.InOutInput, Date: p3MonthsAfter(p3LoanDate, am),
				LoanRateStatus: types.InOutInput, LoanRate: r2}}
		case "pre":
			sm := 3 + rng.Intn(n/2)
			nn := 1 + rng.Intn(n/2)
			amt := math.Round(amount * 0.005 * (1 + rng.Float64()))
			args = append(args, fmt.Sprintf("pre=%d:%d:12:%.0f", sm, nn, amt))
			in.Fancy = true
			in.Prepayments = []Prepayment{{
				StartDateStatus: types.InOutInput, StartDate: p3MonthsAfter(p3LoanDate, sm),
				NNStatus: types.InOutInput, NN: nn,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PaymentStatus: types.InOutInput, Payment: amt}}
		case "skip":
			args = append(args, "skip=6-8")
			ms, _ := MonthSetFromString("6-8")
			in.Fancy = true
			in.SkipMonths = SkipMonths{SkipStatus: types.InOutInput, SkipStr: "6-8", MonthSet: ms}
		case "mor":
			mm := (2 + rng.Intn(3)) * (12 / 12)
			args = append(args, fmt.Sprintf("mor=%d", mm))
			in.Fancy = true
			in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: p3MonthsAfter(p3LoanDate, mm)}
		case "targ":
			tv := math.Round(amount / float64(n) * 0.5)
			args = append(args, fmt.Sprintf("targ=%.0f", tv))
			in.Fancy = true
			in.Target = Target{TargetStatus: types.InOutInput, TargetValue: tv}
		}
		rowArgs := append(append([]string{}, args...), "rows")
		f, ok := p3Oracle(t, rowArgs...)
		lbl := fmt.Sprintf("fuzz#%d amt=%.2f r=%.6f n=%d py=%d %s %s", c, amount, rate, n, perYr, basis, opt)
		if !ok {
			t.Logf("SKIP A6 %s: no output", lbl)
			continue
		}
		res := Amortize(in)
		checked++
		if p3IsErr(f) {
			if res.Err == nil {
				diverged++
				t.Logf("DIVERGE A6 %s: DOS %v | Go ok", lbl, strings.Join(f, " "))
			}
			continue
		}
		dp, dok := p3Val(f, "payment")
		if !dok || dp == 0 {
			t.Logf("SKIP A6 %s: degenerate %v", lbl, f[:minInt(8, len(f))])
			continue
		}
		if res.Err != nil {
			diverged++
			t.Logf("DIVERGE A6 %s: DOS pay=%.4f | Go err %v", lbl, dp, res.Err)
			continue
		}
		gp := modalReg(res.Schedule)
		if math.Abs(gp-dp) > 0.005 {
			diverged++
			t.Logf("DIVERGE A6 %s: DOS pay=%.4f | Go pay=%.4f (Δ=%+.4f) args: %s", lbl, dp, gp, gp-dp, strings.Join(args, " "))
			continue
		}
		// 3 sampled rows: DOS rows include settlement rows in fancy mode? p3ParseRows
		// keeps only real payment rows (oracle IsDetailLine). Compare Go schedule rows
		// by position when counts match.
		rows := p3ParseRows(f)
		var goRows []PaymentRecord
		for _, r := range res.Schedule {
			if r.PayNum >= 1 {
				goRows = append(goRows, r)
			}
		}
		if len(rows) != len(goRows) {
			diverged++
			t.Logf("DIVERGE A6 %s: DOS %d rows | Go %d rows", lbl, len(rows), len(goRows))
			continue
		}
		bad := false
		for _, ri := range []int{0, len(rows) / 2, len(rows) - 1} {
			dr, gr := rows[ri], goRows[ri]
			if math.Abs(dr.intp-gr.Interest) > 0.011 || math.Abs(dr.bal-gr.Principal) > 0.011 {
				bad = true
				t.Logf("DIVERGE A6 %s row %d: DOS int=%.4f bal=%.4f | Go int=%.4f bal=%.4f",
					lbl, ri, dr.intp, dr.bal, gr.Interest, gr.Principal)
			}
		}
		if bad {
			diverged++
		}
	}
	t.Logf("A6 fuzz: %d checked, %d diverged", checked, diverged)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
