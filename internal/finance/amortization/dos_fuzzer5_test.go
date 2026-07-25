package amortization

// TestDOSFuzzer5AllAdvancedOptions — the ALL-OPTIONS-AT-ONCE differential fuzzer
// for advanced (fancy) amortization.
//
// Every prior fuzzer varies ONE advanced option at a time (dos_fuzzer3_fancy)
// or a small hand-picked cube of two or three (dos_option_cube_fuzz). Neither
// exercises the screen the client actually fills in: a loan carrying balloons
// AND prepayments AND rate/payment adjustments AND a moratorium AND a principal
// minimum AND skip months AND points, all live simultaneously, with MULTIPLE
// rows in each row-based block. That is where the §45/§46 terminating-balloon
// bug lived, and it is the configuration with the least coverage.
//
// GENERATION RULE (the client's spec): every advanced-option BLOCK is drawn
// independently and included with probability 0.85 — i.e. each option has a 15%
// chance of being absent from a given case. Row-based blocks (balloons,
// prepayments, rate adjustments) get MULTIPLE rows when present. The
// COMPUTATIONAL settings (basis, exact, prepaid, in-advance, plus-regular) are
// not "options" in that sense — they are always-present booleans — so they are
// drawn uniformly instead, which keeps every option combination exercised
// across the whole settings cube rather than concentrating on the defaults.
//
// ROW LIMITS come from the DOS source, not from taste:
//
//	PETYPES.PAS:378-380   maxprepay = 2;  maxadj = maxlines;  maxballoon = maxlines
//
// maxprepay is a HARD 2 and the oracle's SetupPrepayments has no bounds check
// (amort_oracle.pas:296-425 does `inc(k)` then writes `pre[k]^`), so a third
// prepayment token would corrupt the oracle's heap rather than fail cleanly.
// Balloons and adjustments are capped at 3 here — well under maxlines (>= 18) —
// because past three rows the cases stop finding new dispatch paths and just
// cost oracle spawns.
//
// PARITY RULES that keep the two engines seeing byte-identical input:
//
//   - Option dates land on payment dates: every option month is a multiple of
//     mPer = 12/perYr. `pre=`, `b<N>=`, `adj=`, `mor=` and the Go DateRecs all
//     derive from the same addMonths grid off the shared 2024-01-01 loan date.
//   - Option months are DISTINCT across all blocks (a `used` set). Two options
//     landing on the same date is a real DOS behaviour, but the collision-order
//     semantics are a separate open item (§40d), so this fuzzer keeps them apart
//     rather than re-reporting a known gap on every case.
//   - `skip=` is month-of-year based, so it is only drawn at perYr == 12.
//   - `plusreg` maps 1:1 onto Settings.PlusRegular. SetupLoan defaults
//     df.c.plus_regular := false (amort_oracle.pas:91) and SetupBalloons resets
//     it to false (:185), but the `plusreg` token is parsed LATER (:735), so the
//     token — and only the token — decides. There is no hidden default to match.
//   - in.Fancy mirrors the oracle's `fancy := true`, which each Setup* routine
//     sets for balloons/prepayments/adjustments/mor/targ/skip — but which `pts=`
//     does NOT set. A points-only case therefore runs the PLAIN engine on both
//     sides. See the fancyOpt comment in the body for why this matters.
//
// PAYMENT MODE is drawn 50/50 between SOLVED (blank payment; oracle solves it)
// and GIVEN (`payhard=` / types.InOutInput at 0.6x-1.6x the fair payment).
// The GIVEN arm is not decoration: TackOnFinalBalloon's gate requires
// PayAmtStatus >= defp (Amortize.pas:1043), so DOS's terminating balloon can
// NEVER fire under a solved payment. Half the cases exist to reach that code.
//
// SIGNALS, in order of how much they mean:
//
//	total interest / total paid  — the two scalars defined for every option
//	  combination. Per-row or "the regular payment" comparisons are meaningless
//	  here: a moratorium row is interest-only, a prepay row is inflated, an
//	  adjustment row is a different payment entirely, and a balloon row can be
//	  negative. One `bdump` spawn yields both (bdump prints before the totals
//	  line and does not Halt, amort_oracle.pas:910-931).
//	terminating balloon        — the `bdump` grid rows with dstatus/astatus 1
//	  (outp) versus Go's ResolvedBalloon.TackedOn echo. This is the §46 row.
//	last date / nperiods       — DOS's post-FirstPass horizon versus Go's.
//
// One-directional HARD rule, as in fuzzer3-fancy: Go must not produce a
// schedule where DOS REFUSES the screen. Date-horizon breakdowns ("Julian",
// "bad date", "last payment not found") are indeterminate, not refusals.
//
// This is a DISCOVERY fuzzer over a combination space that is known to still
// diverge, so — like TestDOSOptionCubeFuzz — it is opt-in (PERSENSE_FUZZ=1) and
// reports by option SIGNATURE, so a failure names the block combination to walk
// in the Pascal rather than a single anonymous seed.

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// fz5MaxPrepay mirrors PETYPES.PAS:378 `maxprepay = 2`. Exceeding it overruns
// the oracle's pre[] array.
const fz5MaxPrepay = 2

// fz5Outcome classifies one oracle spawn.
//
// fz5NonConverge is kept apart from fz5Refused on purpose. "Computation of
// payment amount or interest rate did not converge" is DOS's Iterate giving up
// (AMORTOP.pas), NOT the screen being rejected as ill-formed — the port
// legitimately finds roots DOS's bisection misses, which is why fuzzer3-fancy
// downgrades that same case to a log when Go's schedule actually retires the
// loan. Folding it into "refused" would turn a known solver-fidelity gap into a
// hard failure on nearly every stacked-option case.
const (
	fz5Solved = iota
	fz5Refused
	fz5Flake
	fz5DateHorizon
	fz5NonConverge
)

// fz5Dump is everything one `bdump` oracle run tells us.
type fz5Dump struct {
	payment   float64
	interest  float64
	paid      float64
	nballoons int
	nlines    int
	rows      []fz5BalloonRow
	lastDate  string // "M/D/YYYY" as DOS prints it
	nPeriods  int
}

type fz5BalloonRow struct {
	idx      int
	date     string // "M/D/YYYY"
	dstatus  int
	amount   float64
	astatus  int
}

// tack returns the terminating-balloon row DOS computed and de-activated: the
// LAST row whose date and amount are both OUTPUT cells (status 1 = outp). A row
// the user typed carries status 3 (inp), and a solved target balloon carries an
// inp date with an outp amount, so requiring BOTH to be outp isolates
// TackOnFinalBalloon's row exactly.
func (d fz5Dump) tack() (fz5BalloonRow, bool) {
	for i := len(d.rows) - 1; i >= 0; i-- {
		if d.rows[i].dstatus == 1 && d.rows[i].astatus == 1 {
			return d.rows[i], true
		}
	}
	return fz5BalloonRow{}, false
}

// aprRetryWithoutPoints re-runs an oracle line with its `pts=` cell removed and
// folds the points fee back into the totals by hand. See the call site for why
// an APR non-convergence is not a refusal (Amortize.pas:1419-1422 warns without
// exiting, so DOS still draws the table).
//
// Returns ok=false when there is no `pts=` cell to strip (then the APR message
// came from somewhere the reconstruction does not model, and the caller should
// keep treating it as a refusal) or when the amount cannot be read back.
func aprRetryWithoutPoints(run func([]string) (fz5Dump, int), args []string) (fz5Dump, int, bool) {
	pts, stripped := 0.0, make([]string, 0, len(args))
	found := false
	for _, a := range args {
		if v, ok := strings.CutPrefix(a, "pts="); ok {
			f, e := strconv.ParseFloat(v, 64)
			if e != nil {
				return fz5Dump{}, 0, false
			}
			pts, found = f, true
			continue
		}
		stripped = append(stripped, a)
	}
	if !found || len(stripped) == 0 {
		return fz5Dump{}, 0, false
	}
	amount, e := strconv.ParseFloat(stripped[0], 64)
	if e != nil {
		return fz5Dump{}, 0, false
	}
	d, oc := run(stripped)
	if oc != fz5Solved {
		return d, oc, true
	}
	fee := pts * amount
	d.interest += fee
	d.paid += fee
	return d, fz5Solved, true
}

func TestDOSFuzzer5AllAdvancedOptions(t *testing.T) {
	// Opt-in: the all-options space still diverges in places (the moratorium +
	// Exact gap of docs/discrepancies.md §45 among them), so this stays off the
	// default `go test ./...` run and is used for hunting.
	if os.Getenv("PERSENSE_FUZZ") == "" {
		t.Skip("opt-in: set PERSENSE_FUZZ=1 to run the all-advanced-options fuzzer")
	}
	gateOracle(t)

	rng := rand.New(rand.NewSource(fuzzSeed(0x667a3520))) // "fz5 "

	N := 300
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, e := strconv.Atoi(s); e == nil && v > 0 {
			N = v
		}
	}

	q6 := func(x float64) float64 { return math.Round(x*1e6) / 1e6 }
	cents := func(x float64) float64 { return math.Round(x*100) / 100 }
	// addMonths mirrors the oracle's month arithmetic off the SetupLoan default
	// loan date (1.1.2024): tot := (loandate.m-1) + months, day = loandate.d.
	addMonths := func(months int) types.DateRec {
		return types.NewDateRec(2024+months/12, time.Month(months%12+1), 1)
	}
	// present implements the client's rule: each option block is used unless a
	// 15% coin says otherwise.
	present := func() bool { return rng.Float64() >= 0.15 }

	dosLine := func(f []string, name string) (float64, bool) {
		for i := 0; i+1 < len(f); i++ {
			if f[i] == name {
				v, e := strconv.ParseFloat(f[i+1], 64)
				return v, e == nil
			}
		}
		return 0, false
	}

	// runDump execs the oracle in `bdump` mode and parses the whole output:
	// the balloon grid AND the quiet totals line. Retries the known heap flakes
	// (payment 0 / interest -1.00 / paid <= 0) on a fresh process, as every
	// other oracle helper in this package does.
	errBucket := map[string]int{}
	var runDump func(args []string) (fz5Dump, int)
	runDump = func(args []string) (fz5Dump, int) {
		for try := 0; try < 8; try++ {
			out, err := exec.Command(oracleBin, args...).Output()
			if err != nil {
				continue
			}
			text := strings.TrimSpace(string(out))
			if text == "" {
				continue
			}
			if strings.HasPrefix(text, "ERR") {
				first := strings.SplitN(text, "\n", 2)[0]
				errBucket[strings.TrimSpace(strings.TrimPrefix(first, "ERR"))]++
				msg := strings.ToLower(text)
				if strings.Contains(msg, "julian") || strings.Contains(msg, "bad date") ||
					strings.Contains(msg, "last payment not found") {
					return fz5Dump{}, fz5DateHorizon
				}
				if strings.Contains(msg, "did not converge") {
					return fz5Dump{}, fz5NonConverge
				}
				// "Computation of APR failed to converge." is NOT a refusal.
				// DOS's dispatch is (Amortize.pas:1419-1422)
				//
				//	if (h^.pointsstatus > defp) then
				//	  if (not EstimateAndRefineAPRwithPoints) then
				//	    MessageBox('Computation of APR failed to converge.',
				//	               DA_APRNoConverge);
				//
				// with NO `exit` after the MessageBox — unlike every other
				// failure arm in that routine. The user gets a warning box and
				// the amortization table is still drawn, with a meaningless APR.
				// Only the headless harness turns it fatal: the oracle's Globals
				// stub routes every MessageBox to noteError (legacy/oracle/
				// Globals.pas:99-106), which aborts the run before any output.
				//
				// The points cell does not shape the schedule at all — it is a
				// flat fee folded into the totals, `interest` and `paid` both
				// gaining exactly points x amount (measured:
				// `100000 0.10 120 12 payhard=1500` -> interest 46576.09; the
				// same line + `pts=0.03` -> 49576.09, + `pts=0.027009` ->
				// 49276.99). So re-run without the points cell, which is also
				// what switches the APR solve off ("if =defp, then it's zero by
				// default and we skip the APR computation", Amortize.pas:1419),
				// and add the fee back. That recovers DOS's real answer instead
				// of scoring the case as a refusal.
				//
				// 2026-07-25 seed 11001: both hard failures were this. e.g.
				//	325717.65 0.1332850000 25 1 exact mor=132 b168=32482.08 \
				//	  b204=77376.37 pre=48:177:12:770.15 adj=12::57986.62 \
				//	  adj=120:0.1027860000: targ=3474.07 pts=0.027009 \
				//	  payhard=40676.18
				// reconstructs to 178070.36 + 0.027009*325717.65 = 186867.61,
				// which is what the port already produced.
				if strings.Contains(msg, "apr failed to converge") {
					if d2, oc2, ok := aprRetryWithoutPoints(runDump, args); ok {
						return d2, oc2
					}
				}
				return fz5Dump{}, fz5Refused
			}
			var d fz5Dump
			gotTotals := false
			for _, ln := range strings.Split(text, "\n") {
				f := strings.Fields(strings.TrimSpace(ln))
				if len(f) == 0 {
					continue
				}
				switch f[0] {
				case "nballoons":
					if v, ok := dosLine(f, "nballoons"); ok {
						d.nballoons = int(v)
					}
					if v, ok := dosLine(f, "nlines"); ok {
						d.nlines = int(v)
					}
				case "balloonrow":
					// balloonrow I date M/D/YYYY dstatus S amount A astatus S
					var r fz5BalloonRow
					if len(f) > 1 {
						r.idx, _ = strconv.Atoi(f[1])
					}
					for i := 0; i+1 < len(f); i++ {
						switch f[i] {
						case "date":
							r.date = f[i+1]
						case "dstatus":
							r.dstatus, _ = strconv.Atoi(f[i+1])
						case "amount":
							r.amount, _ = strconv.ParseFloat(f[i+1], 64)
						case "astatus":
							r.astatus, _ = strconv.Atoi(f[i+1])
						}
					}
					d.rows = append(d.rows, r)
				case "lastdate":
					if len(f) > 1 {
						d.lastDate = f[1]
					}
					if v, ok := dosLine(f, "nperiods"); ok {
						d.nPeriods = int(v)
					}
				case "payment":
					p, pok := dosLine(f, "payment")
					in, iok := dosLine(f, "interest")
					pd, dok := dosLine(f, "paid")
					if !pok || !iok || !dok {
						continue
					}
					d.payment, d.interest, d.paid = p, in, pd
					gotTotals = true
				}
			}
			if !gotTotals {
				continue // no totals line at all — flake, respawn
			}
			// Heap-flake sentinels: a real schedule never has non-positive paid,
			// and NumAfter returns -1 when the totals line is malformed.
			if d.paid <= 0 || d.interest == -1 || d.payment == 0 {
				continue
			}
			return d, fz5Solved
		}
		return fz5Dump{}, fz5Flake
	}

	perYrs := []int{1, 2, 4, 12}
	bases := []types.BasisType{types.Basis360, types.Basis365, types.Basis365360}

	// retires reports whether a Go schedule genuinely amortizes (its last
	// remaining balance is ~0 relative to the loan). Same definition as
	// fuzzer3-fancy's: it is what separates a VALID root DOS's less-robust
	// Iterate simply missed from a DEGENERATE spurious solve the port invented.
	retires := func(r AmortResult, principal float64) bool {
		if r.Err != nil || len(r.Schedule) == 0 {
			return false
		}
		bal := r.Schedule[len(r.Schedule)-1].Principal
		return math.Abs(bal) <= math.Max(1.0, 1e-4*math.Abs(principal))
	}

	// Per-case accounting.
	checked, refused, flaked, horizon := 0, 0, 0, 0
	nonConv, nonConvGoRetires, nonConvGoSpurious := 0, 0, 0
	goRefusedDosSolved, goSolvedDosRefused := 0, 0
	tackAgree, tackGoOnly, tackDosOnly, tackValueDiff := 0, 0, 0, 0
	blockCover := map[string]int{}

	type classStat struct {
		n         int
		diverge   int
		worstInt  float64
		worstPaid float64
		worstCmd  string
	}
	classes := map[string]*classStat{}

	for c := 0; c < N; c++ {
		perYr := perYrs[rng.Intn(len(perYrs))]
		mPer := 12 / perYr
		years := 8 + rng.Intn(18) // 8..25y — long enough to seat 3 balloons + 3 ARMs
		n := years * perYr
		amount := cents(25000 + rng.Float64()*475000)
		rate := q6(0.03 + rng.Float64()*0.11)

		// fair is the closed-form level payment that would retire this loan with
		// no options at all. Every option amount below is scaled off it rather
		// than off a flat dollar range. That is not cosmetic: a flat "$25..$625
		// per prepayment" is rounding noise on a $500k loan and instantly
		// over-retires a $25k one, and the over-retiring end is exactly where
		// DOS's Iterate gives up with "did not converge" — which is why the first
		// smoke run left only 20% of cases comparable. Scaling keeps the stacked
		// options inside the region where DOS will actually produce a schedule to
		// compare against, without narrowing WHICH options get stacked.
		fair := amount * (rate / float64(perYr)) / (1 - math.Pow(1+rate/float64(perYr), -float64(n)))

		basis := bases[rng.Intn(len(bases))]
		exact := rng.Intn(2) == 0
		prepaid := rng.Intn(2) == 0
		inadv := rng.Intn(2) == 0
		plusreg := rng.Intn(2) == 0

		s := gzSettings(perYr, basis, exact, prepaid, inadv, false, false)
		s.PlusRegular = plusreg

		var flags []string
		if bf, ok := basisFlag(basis); ok {
			flags = append(flags, bf)
		}
		if exact {
			flags = append(flags, "exact")
		}
		if prepaid {
			flags = append(flags, "prepaid")
		}
		if inadv {
			flags = append(flags, "inadv")
		}
		if plusreg {
			flags = append(flags, "plusreg")
		}

		// Distinct option months on the payment grid, so no two option blocks
		// can land on the same date and expose §40d's collision ordering.
		used := map[int]bool{}
		pickMonth := func(loK, hiK int) (int, bool) {
			if hiK < loK {
				return 0, false
			}
			for try := 0; try < 40; try++ {
				k := loK + rng.Intn(hiK-loK+1)
				if !used[k] {
					used[k] = true
					return k * mPer, true
				}
			}
			return 0, false
		}

		var blocks []string
		var mutators []func(*LoanInput)
		anyOpt := false
		// fancyOpt mirrors the ORACLE's `fancy` flag, which is NOT "any option is
		// present" -- it is the user-facing screen toggle
		// (AmortizationScreenUnit.pas:306 `fancy := false`, :382 `fancy := not
		// fancy`; PEDATA.pas:714 `default_fancy := false`).  amort_oracle.pas sets
		// `fancy := true` only inside the Setup routines for balloons (:183),
		// prepayments (:276), adjustments (:423), mor= (:788), targ= (:802),
		// skip= (:813), solveballoon= (:859) and dateballoon= (:879).  `pts=` does
		// NOT set it -- points are an ordinary loan field, not an advanced option.
		//
		// This distinction is load-bearing, because DOS genuinely answers
		// DIFFERENTLY for the same loan under the two engines: Iterate's terminal
		// (AMORTOP.pas:1437) is
		//	if (fancy) or ((df.c.exact) and (df.c.basis<>x360))
		//	   then RepayFancyLoan else RepayLoan(p)
		// so a plain in-advance loan is repaid by RepayLoan's annuity-due branch
		// (AMORTOP.pas:1275-1281) instead of the prorated fancy walk.  Measured:
		//	amort_oracle 271486.77 0.0926380000 50 2 b365_360 prepaid inadv \
		//	  pts=0.012583
		//	  -> payment 13869.8743 interest 466664.05 paid 738150.82   (plain)
		//	same line + targ=0.01 (a no-op target, but it sets fancy)
		//	  -> payment 14109.5081 interest 436009.96 paid 707496.73   (fancy)
		// Go's Amortize produced 14109.51 / 436009.96 -- i.e. it matched DOS's
		// FANCY answer exactly and was correct; the harness was simply forcing
		// in.Fancy = true while the oracle ran the plain engine.  So mirror the
		// oracle: Fancy iff a fancy-setting option block was actually emitted.
		fancyOpt := false
		note := func(name string) {
			anyOpt = true
			if name != "pts" {
				fancyOpt = true
			}
			blocks = append(blocks, name)
			blockCover[name]++
		}

		// ---- Moratorium (interest-only until first repayment of principal) ----
		// Drawn FIRST even though it is written to the screen last, because it
		// constrains where a balloon may go. AMORTOP.pas rejects the whole screen
		// with "No balloon can precede first repayment of principal", and with
		// mor present 85% of the time and balloons scattered over the first 80%
		// of the term that single rule ate 16 of 40 cases on the previous run.
		// Honouring it is not a weakening of the fuzz: an illegal screen teaches
		// nothing about the port, it just costs a case.
		morK := 0
		if present() {
			if m, ok := pickMonth(1, maxInt(1, n/2)); ok {
				morK = m / mPer
				mor := Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: addMonths(m)}
				flags = append(flags, "mor="+strconv.Itoa(m))
				note("mor")
				mutators = append(mutators, func(in *LoanInput) { in.Moratorium = mor })
			}
		}

		// ---- Balloons: 1..3 rows, after any moratorium, inside the first ~80% ----
		if present() {
			k := 1 + rng.Intn(3)
			var months []int
			for i := 0; i < k; i++ {
				if m, ok := pickMonth(morK+1, maxInt(morK+1, n*8/10)); ok {
					months = append(months, m)
				}
			}
			sort.Ints(months)
			var bs []BalloonPayment
			// Cap the SUM of the balloons at 60% of principal. Individually a 40%
			// balloon is fine; three of them stacked retire the loan outright
			// mid-term, and DOS answers that with a non-converge rather than a
			// schedule, so the case is silently lost instead of compared.
			budget := amount * 0.60
			for _, m := range months {
				if budget <= 0 {
					break
				}
				amt := cents(math.Min(budget, amount*(0.02+rng.Float64()*0.28)))
				if amt < 1 {
					break
				}
				budget -= amt
				flags = append(flags, fmt.Sprintf("b%d=%.2f", m, amt))
				bs = append(bs, balloonAt(m, amt))
			}
			if len(bs) > 0 {
				note(fmt.Sprintf("balloon%d", len(bs)))
				mutators = append(mutators, func(in *LoanInput) { in.Balloons = bs })
			}
		}

		// ---- Prepayments: 1..maxprepay(2) series ----
		if present() {
			k := 1 + rng.Intn(fz5MaxPrepay)
			var ps []Prepayment
			for i := 0; i < k; i++ {
				startK := 1 + rng.Intn(maxInt(1, n*7/10))
				m, ok := pickMonth(startK, startK)
				if !ok {
					continue
				}
				// nn extra payments at ppy/yr. Keep the series inside the term so
				// CheckPrepayments' derived stop date does not run off the horizon.
				ppy := []int{12, 24, 26, 52}[rng.Intn(4)]
				remMonths := (n * mPer) - m
				maxNN := (remMonths * ppy) / 12
				if maxNN < 1 {
					maxNN = 1
				}
				if maxNN > 400 {
					maxNN = 400
				}
				nn := 1 + rng.Intn(maxNN)
				// Size the series as a FRACTION OF THE REGULAR PAYMENT FLOW: a
				// prepayment arrives ppy times a year against perYr regular
				// payments, so perYr/ppy converts "fraction of one regular
				// payment's worth of extra money per year" into a per-prepayment
				// amount. 3%-25% of the flow keeps the loan alive to term.
				amt := cents(fair * (0.03 + rng.Float64()*0.22) * float64(perYr) / float64(ppy))
				if amt < 1 {
					amt = 1
				}
				flags = append(flags, fmt.Sprintf("pre=%d:%d:%d:%.2f", m, nn, ppy, amt))
				ps = append(ps, Prepayment{
					StartDateStatus: types.InOutInput, StartDate: addMonths(m),
					NNStatus: types.InOutInput, NN: nn,
					PerYrStatus: types.InOutInput, PerYr: ppy,
					PaymentStatus: types.InOutInput, Payment: amt,
				})
			}
			if len(ps) > 0 {
				note(fmt.Sprintf("prepay%d", len(ps)))
				mutators = append(mutators, func(in *LoanInput) { in.Prepayments = ps })
			}
		}

		// ---- Rate / payment adjustments: 1..3 rows ----
		// Amortize.pas:1294 rejects the screen on `(df.c.in_advance) and (nadj >
		// 0)` — ANY adjustment row, not merely a rate change, despite the message
		// text saying "change rates". So the block is suppressed under inadv.
		// This is not an untested corner: an earlier sweep of this fuzzer
		// generated 20 inadv+adj screens and the port refused all 20 alongside
		// DOS (Go-solved-DOS-refused = 0), so the gate itself is verified. Left
		// in, it would only burn a third of the run on a known-agreeing refusal.
		if !inadv && present() {
			k := 1 + rng.Intn(3)
			var months []int
			for i := 0; i < k; i++ {
				if m, ok := pickMonth(1, maxInt(1, n*8/10)); ok {
					months = append(months, m)
				}
			}
			sort.Ints(months)
			if len(months) > 0 {
				var as []RateAdjustment
				kinds := make([]int, 0, len(months))
				for _, m := range months {
					// 0 = rate only, 1 = payment only, 2 = both. All three are real
					// DOS screens (adj= accepts a blank rate OR a blank amount) —
					kind := rng.Intn(3)
					kinds = append(kinds, kind)
					a := RateAdjustment{DateStatus: types.InOutInput, Date: addMonths(m)}
					rateStr, amtStr := "", ""
					if kind != 1 {
						nr := q6(0.02 + rng.Float64()*0.13)
						a.LoanRateStatus, a.LoanRate = types.InOutInput, nr
						rateStr = strconv.FormatFloat(nr, 'f', 10, 64)
					}
					if kind != 0 {
						// A payment adjustment is a NEW regular payment, so it has to
						// live near the fair payment. amount/n (straight-line
						// principal, no interest) is far below it on a long loan and
						// starves the schedule into a non-converge.
						na := cents(fair * (0.75 + rng.Float64()*0.7))
						a.AmountStatus, a.Amount, a.AmtOK = types.InOutInput, na, true
						amtStr = strconv.FormatFloat(na, 'f', 2, 64)
					}
					flags = append(flags, fmt.Sprintf("adj=%d:%s:%s", m, rateStr, amtStr))
					as = append(as, a)
				}
				note(fmt.Sprintf("adj%d", len(as)))
				_ = kinds
				mutators = append(mutators, func(in *LoanInput) { in.Adjustments = as })
			}
		}

		// ---- Target: minimum principal reduction per payment ----
		if present() {
			// The target is a MINIMUM principal reduction per payment, so it only
			// binds when it is a plausible slice of the payment. A flat $50-$500
			// is unreachable on a small loan (DOS then has nothing to converge to)
			// and inert on a large one.
			tv := cents(fair * (0.02 + rng.Float64()*0.23))
			flags = append(flags, fmt.Sprintf("targ=%.2f", tv))
			note("targ")
			mutators = append(mutators, func(in *LoanInput) {
				in.Target = Target{TargetStatus: types.InOutInput, TargetValue: tv}
			})
		}

		// ---- Skip months (month-of-year; monthly schedules only) ----
		if perYr == 12 && present() {
			skipStr := []string{"6", "1,7", "5-7", "2,8,11", "11-12", "1,3,5"}[rng.Intn(6)]
			sm := skipSetRaw(skipStr)
			if sm.SkipStatus != 0 {
				flags = append(flags, "skip="+skipStr)
				note("skip")
				mutators = append(mutators, func(in *LoanInput) { in.SkipMonths = sm })
			}
		}

		// ---- Points (turns the APR solver on in both engines) ----
		points := -1.0
		if present() {
			points = q6(rng.Float64() * 0.04)
			flags = append(flags, fmt.Sprintf("pts=%.6f", points))
			note("pts")
		}

		if !anyOpt {
			continue // the 15% coins came up empty for every block — plain loan
		}

		// ---- Payment mode ----
		// The fair payment scaled 0.85x-1.35x, so residuals of BOTH signs occur
		// and TackOnFinalBalloon's over- and under-funded arms both get hit —
		// but the schedule still terminates. Below ~0.8x a stacked-option loan
		// negatively amortizes past DOS's horizon; above ~1.4x it retires far
		// early and DOS refuses rather than truncating.
		givenPay := rng.Intn(2) == 0
		pay := cents(fair * (0.85 + rng.Float64()*0.5))
		if givenPay {
			flags = append(flags, fmt.Sprintf("payhard=%.2f", pay))
		}
		flags = append(flags, "bdump")

		args := []string{
			strconv.FormatFloat(amount, 'f', 2, 64),
			strconv.FormatFloat(rate, 'f', 10, 64),
			strconv.Itoa(n), strconv.Itoa(perYr),
		}
		args = append(args, flags...)

		dos, outcome := runDump(args)

		// ---- The Go side, byte-identical inputs ----
		in := gzLoanInput(amount, rate, n, perYr, s)
		in.Fancy = fancyOpt // see the fancyOpt comment above: mirrors the oracle
		if givenPay {
			in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, pay
		}
		if points >= 0 {
			in.Loan.PointsStatus, in.Loan.Points = types.InOutInput, points
		}
		for _, m := range mutators {
			m(&in)
		}
		gr := Amortize(in)
		goOK := gr.Err == nil && len(gr.Schedule) > 0

		sort.Strings(blocks)
		sig := strings.Join(blocks, "+")
		if givenPay {
			sig += "|pay"
		} else {
			sig += "|solve"
		}
		cmd := "amort_oracle " + strings.Join(args, " ")

		switch outcome {
		case fz5Flake:
			flaked++
			continue
		case fz5DateHorizon:
			horizon++
			continue
		case fz5NonConverge:
			// DOS's Iterate gave up. There is no oracle answer to compare against,
			// so this can never be a hard failure — but it is not nothing either.
			// If Go's schedule genuinely retires the loan, the port found a root
			// DOS's bisection missed (the known solver-fidelity gap, benign). If
			// Go "solved" and the loan does NOT retire, the port invented a
			// schedule out of a screen with no answer, which is the same class of
			// bug as Go-solved-DOS-refused and is worth naming in the report.
			nonConv++
			if goOK {
				if retires(gr, amount) {
					nonConvGoRetires++
				} else {
					nonConvGoSpurious++
					t.Logf("DOS non-converge, Go returned a NON-RETIRING schedule [%s]\n  %s\n"+
						"  Go: final balance %.2f on %d rows (int=%.2f paid=%.2f)",
						sig, cmd, gr.Schedule[len(gr.Schedule)-1].Principal, len(gr.Schedule),
						gr.TotalInt, gr.TotalPaid)
				}
			}
			continue
		case fz5Refused:
			refused++
			// HARD, one-directional: Go must not produce a schedule for a screen
			// DOS refuses outright.
			if goOK {
				goSolvedDosRefused++
				t.Errorf("Go produced a schedule where DOS REFUSED the screen [%s]\n  %s\n"+
					"  Go: int=%.2f paid=%.2f rows=%d", sig, cmd, gr.TotalInt, gr.TotalPaid,
					len(gr.Schedule))
			}
			continue
		}

		checked++
		cs := classes[sig]
		if cs == nil {
			cs = &classStat{}
			classes[sig] = cs
		}
		cs.n++

		if !goOK {
			goRefusedDosSolved++
			errText := "nil"
			if gr.Err != nil {
				errText = gr.Err.Error()
			}
			t.Logf("DOS solved, Go refused [%s]: %v\n  %s", sig, errText, cmd)
			continue
		}

		// ---- Signal 1: totals ----
		// Scaled tolerance: the DOS-vs-port rounding tail grows with the number
		// of rows and with the size of the schedule, and a stacked-option
		// schedule can run tens of thousands of rows once weekly prepayments are
		// in play. The floor is a dollar; the slope is 5 basis points of a
		// basis point of DOS's own total.
		intTol := math.Max(1.0, 5e-4*math.Abs(dos.interest))
		paidTol := math.Max(1.0, 5e-4*math.Abs(dos.paid))
		dInt := math.Abs(gr.TotalInt - dos.interest)
		dPaid := math.Abs(gr.TotalPaid - dos.paid)
		if dInt > intTol || dPaid > paidTol {
			cs.diverge++
			if dInt > cs.worstInt || dPaid > cs.worstPaid {
				if dInt > cs.worstInt {
					cs.worstInt = dInt
				}
				if dPaid > cs.worstPaid {
					cs.worstPaid = dPaid
				}
				cs.worstCmd = fmt.Sprintf("%s\n    DOS int=%.2f paid=%.2f | Go int=%.2f paid=%.2f "+
					"(dInt=%.2f dPaid=%.2f)", cmd, dos.interest, dos.paid, gr.TotalInt, gr.TotalPaid,
					dInt, dPaid)
			}
		}

		// ---- Signal 2: DOS's terminating balloon (§46) ----
		dosTack, dosHasTack := dos.tack()
		var goTack ResolvedBalloon
		goHasTack := false
		for _, b := range gr.Balloons {
			if b.TackedOn {
				goTack, goHasTack = b, true
			}
		}
		switch {
		case dosHasTack && goHasTack:
			tackAgree++
			wantDate := fmt.Sprintf("%d/%d/%d", int(goTack.Date.Time.Month()),
				goTack.Date.Time.Day(), goTack.Date.Time.Year())
			// The tack amount is a balance, so scale the tolerance to the loan.
			tackTol := math.Max(0.05, 1e-5*math.Abs(amount))
			if math.Abs(goTack.Amount-dosTack.amount) > tackTol || wantDate != dosTack.date {
				tackValueDiff++
				t.Errorf("terminating balloon differs [%s]\n  %s\n"+
					"  DOS: %s %.4f (row %d, dstatus/astatus outp, nballoons=%d nlines=%d)\n"+
					"  Go : %s %.4f", sig, cmd, dosTack.date, dosTack.amount, dosTack.idx,
					dos.nballoons, dos.nlines, wantDate, goTack.Amount)
			}
		case dosHasTack && !goHasTack:
			tackDosOnly++
			t.Errorf("DOS tacked a terminating balloon the port did not [%s]\n  %s\n"+
				"  DOS row %d: %s %.4f (dstatus %d astatus %d), nballoons=%d nlines=%d\n"+
				"  This is the §46 gate: Amortize.pas:1043 requires fancy AND "+
				"PayAmtStatus >= defp AND an over-specified schedule.",
				sig, cmd, dosTack.idx, dosTack.date, dosTack.amount,
				dosTack.dstatus, dosTack.astatus, dos.nballoons, dos.nlines)
		case !dosHasTack && goHasTack:
			tackGoOnly++
			t.Errorf("the port tacked a terminating balloon DOS did not [%s]\n  %s\n"+
				"  Go: %s %.4f — DOS's grid has no outp/outp row",
				sig, cmd, goTack.Date.Time.Format("1/2/2006"), goTack.Amount)
		}

		// ---- Signal 3: the post-FirstPass horizon ----
		if dos.nPeriods > 0 && gr.NPeriods > 0 && dos.nPeriods != gr.NPeriods {
			t.Logf("nperiods differ [%s]: DOS %d, Go %d\n  %s", sig, dos.nPeriods, gr.NPeriods, cmd)
		}
	}

	// ---- Report ----
	t.Logf("cases: %d generated, %d compared | DOS refused %d, non-converged %d, date-horizon %d, flaked %d",
		N, checked, refused, nonConv, horizon, flaked)
	t.Logf("dispatch: Go-solved-DOS-refused %d (hard fail), DOS-solved-Go-refused %d (logged)",
		goSolvedDosRefused, goRefusedDosSolved)
	t.Logf("DOS non-converge: Go retired the loan %d (benign solver gap), Go non-retiring %d (suspect)",
		nonConvGoRetires, nonConvGoSpurious)

	if len(errBucket) > 0 {
		ekeys := make([]string, 0, len(errBucket))
		for k := range errBucket {
			ekeys = append(ekeys, k)
		}
		sort.Slice(ekeys, func(i, j int) bool { return errBucket[ekeys[i]] > errBucket[ekeys[j]] })
		for _, k := range ekeys {
			t.Logf("oracle ERR x%d: %s", errBucket[k], k)
		}
	}
	t.Logf("terminating balloon: agree %d, value-differs %d, DOS-only %d, Go-only %d",
		tackAgree, tackValueDiff, tackDosOnly, tackGoOnly)

	covKeys := make([]string, 0, len(blockCover))
	for k := range blockCover {
		covKeys = append(covKeys, k)
	}
	sort.Strings(covKeys)
	var cov []string
	for _, k := range covKeys {
		cov = append(cov, fmt.Sprintf("%s=%d", k, blockCover[k]))
	}
	t.Logf("block coverage: %s", strings.Join(cov, " "))

	keys := make([]string, 0, len(classes))
	for k := range classes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := classes[keys[i]], classes[keys[j]]
		if a.diverge != b.diverge {
			return a.diverge > b.diverge
		}
		return keys[i] < keys[j]
	})
	totalDiverge := 0
	for _, k := range keys {
		cs := classes[k]
		totalDiverge += cs.diverge
		if cs.diverge == 0 {
			continue
		}
		t.Errorf("DIVERGENT CLASS %s: %d/%d cases (worst dInt=%.2f dPaid=%.2f)\n    %s",
			k, cs.diverge, cs.n, cs.worstInt, cs.worstPaid, cs.worstCmd)
	}
	t.Logf("divergent option classes: %d of %d compared cases", totalDiverge, checked)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
