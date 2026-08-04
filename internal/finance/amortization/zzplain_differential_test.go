package amortization

// RANDOMIZED PLAIN-LOAN DIFFERENTIAL (round 21, 2026-08-03).
//
// WHY THIS EXISTS — THE COVERAGE HOLE §62 CAME OUT OF.
//
// `dos_fuzzer5_test.go` ABANDONS a case that draws no advanced option
// (`skippedPlain`, :1557) before the oracle is ever spawned. Its own comment
// says so and says what is supposed to cover the gap:
//
//	"this fuzzer can never report a divergence on a PLAIN loan … Plain-loan
//	 fidelity is covered by zzmetafuzz_test.go's forward corpus and by the
//	 committed unit suite — NOT by any figure derived from this fuzzer."
//
// That second half was a COVERAGE CLAIM AND IT WAS NOT BACKED. zzmetafuzz's
// forward corpus is five hand-written screens on the days 1, 8, 15 and 29, and
// no committed case put a day-29 anchor's LAST payment on a February. So the
// port's plain path — the simplest thing the product does, and the shape most
// real screens have — had NO randomized differential at all, and §62 (a dropped
// final row on any 29th/30th/31st-anchored loan ending in February) sat there
// unmeasured while three rounds quoted a residual rate that structurally
// excluded it.
//
// Every headline amortization figure this project has published came from
// fuzzer5. None of them said anything about a plain loan. This test is the
// instrument that lets one be quoted.
//
// WHAT IT DRAWS, AND THE TWO THINGS IT ASSERTS ABOUT ITS OWN DRAW (R7).
//
// Plain means no advanced option — no balloon, prepayment, adjustment, target,
// moratorium or skip — which is exactly fuzzer5's skip condition. It still
// varies the things a user varies: amount, rate, all eight payment frequencies,
// term, day-of-month INCLUDING 29/30/31, an odd first period, and the 360 /
// 365 / 365-360 bases.
//
// A stratification label is a coverage claim (round 17), so this test proves
// two properties of its own sample rather than asserting them in prose:
//   - anchor days 29-31 are actually drawn, and
//   - last payment dates actually land on a clamped February — the §62 shape.
// If a future edit narrows the draw so either stops happening, this fails HERE
// rather than quietly reporting a clean rate over a population that no longer
// contains the interesting case.
//
// R2: THE HARNESS COMPUTES NO INPUT DATE. Both engines get the same
// `loandmy=` / `firstdmy=` tokens and DERIVE everything else. The last date is
// read back OUT of each engine (`bdump`), never computed here and fed in.

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

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

type plainDiffLedger struct {
	generated    int
	compared     int
	oracleRefuse int
	goRefuse     int
	noTotals     int
	unparsed     int

	rowDiff, intDiff, paidDiff, payDiff int
	// ROUND 22 — the four comparisons this instrument did not make.
	// Round 21 shipped it comparing a ROW COUNT and two TOTALS. Everything
	// else the oracle already prints was read and thrown away: `payment` was
	// parsed into dPay and discarded with `_ = dPay`, and the last date was
	// parsed only to pick an era bucket. A schedule can agree on its count and
	// its two totals while every row inside it is wrong in offsetting pairs,
	// and §62 itself was a ROW-LEVEL defect that happened to move a total.
	lastDiff, nperDiff      int
	rowIntDiff, rowPrinDiff int
	rowBalDiff              int
	rowsCompared            int
	worst                   []string
	// ROUND 22 — THE NUMERATOR IS ADJUDICATED, NOT JUST COUNTED (rule 9).
	//
	// Two mechanisms account for essentially every row-level signal, and they
	// have completely different meanings. Counting them together produces a
	// per-row "divergence rate" that is mostly an artefact of the oracle's own
	// print step, and that is precisely the mistake round 18 made with the tack
	// tolerance — a rate whose numerator had not been read.
	//
	//  1. A HALF-CENT PRINT TIE. The oracle's row figures are DOS's RENDERED
	//     CENTS (amort_oracle.pas:1186-1191). When the port's raw value lands on
	//     a .xx5 boundary the two formatters can round opposite ways over a
	//     difference of 1e-8, and the comparison reports a full cent. Verified
	//     on three sampled rows: the port's raw values sat 8.4e-06, 2.2e-05 and
	//     2.6e-03 from the boundary. This is NOT scored away — the tolerance
	//     stays at a cent and the signal is still counted — it is CLASSIFIED, so
	//     the residual can be quoted with the ties separated out instead of
	//     silently folded in or silently tolerated.
	//
	//  2. THE TERMINATING-BALLOON FINAL ROW — §59's mechanism, on the plain
	//     surface. Where the regular payment does not quite amortize the loan,
	//     DOS computes a terminating balloon and then DE-ACTIVATES it
	//     (Amortize.pas:1040-1088, `dec(nballoons)`; the oracle duly reports
	//     `nballoons 0`). DOS's last GRID row therefore shows the regular
	//     payment's own split and leaves the balloon balance standing; the port
	//     folds the balloon into that row and shows a zero balance. Everything
	//     computed agrees — payment, both totals, nperiods and the last date, to
	//     the cent — and the divergence is confined to what the final row
	//     DISPLAYS. §59's rule is the governing one: establish whether a
	//     divergent cell is USED before assigning severity. It is used, so it is
	//     named and reported; it is not a computation defect, so it does not
	//     enter the row-value counters.
	rowTieInt, rowTiePrin, rowTieBal int
	balloonFinalRow                  int
	balloonFinalRowInScope           int
	// The IN-SCOPE samples, kept separately. The general `worst` list is capped
	// at twelve and fills in draw order, so on a seed with many out-of-scope
	// signals the IN-SCOPE ones — the only signals that fail this test — could
	// be crowded out of the report entirely. An instrument whose failure message
	// may omit every reproducing command for the failure is not usable.
	worstInScope []string

	// coverage counters — printed at every level including zero (R13/R8)
	anchorHigh      int // loan/first anchored on the 29th, 30th or 31st
	febLast         int // last payment date lands in February
	febClamped      int // …and that February is short of the anchor day
	byPerYr         map[int]int
	byBasis         map[string]int
	oddFirst        int
	trailingZeroRow int // DOS's all-zero row past the last payment; counted, not scored
	inScope         int
	outScope        int
	inScopeSig      int
	curOutOfScope   bool
	outScopeSig     int
	maxLastYear     int
	minLastYear     int

	// ROUND 22 — THE REFUSAL BUCKET, WHICH WAS 14% OF THE POPULATION AND HAD
	// NO CONTENT AT ALL.
	//
	// Round 21 counted every oracle refusal into one `oracleRefuse` integer and
	// `continue`d before the port was ever run. Two things were lost by that.
	//
	// First, the bucket is not one class. Measured over 4,800 draws it is
	// exactly two DOS messages: "There must be at least two regular payments"
	// and "Computation of payment amount or interest rate did not converge",
	// and they mean completely different things — the first is an input
	// rejection, the second is DOS's Iterate giving up. fuzzer5 has kept them
	// in separate buckets (`refused` vs `non-converged`) since round 16 and
	// treats them differently; the plain instrument merged them.
	//
	// Second, and worse: fuzzer5 makes "the port produced a schedule for a
	// screen DOS REFUSED" a HARD failure (`go_solved_dos_refused`). The plain
	// differential never asked that question, because it returned before
	// calling Amortize. The single most dangerous thing a port can do on a
	// rejected screen — answer it — was unmeasured on the plain surface for
	// the whole of the instrument's life, which is one round.
	//
	// This is R15 landing on the instrument R15 produced: a bucket label is a
	// coverage claim, and "oracle-refused" claimed those 14% were unanswerable.
	refuseTwoPay      int // DOS: fewer than two regular payments
	refuseNonConv     int // DOS: Iterate did not converge
	refuseOther       int // anything else, printed even at zero (R13)
	goSolvedDosRefuse int // HARD — the port answered a screen DOS refused
	nonConvGoRetires  int // benign: a root DOS's bisection missed
	nonConvGoSpurious int // ADVISORY — a schedule out of a screen with no answer
	refusePaired      int // DOS refused AND the port refused
	// Refusal MESSAGE classes. Both engines rejecting a screen is fidelity;
	// rejecting it for a different stated reason is a user-visible difference
	// on a cell nobody has compared. Counted, split by era, never scored as a
	// schedule divergence.
	refuseMsgSame    int
	refuseMsgDiffer  map[string]int
	refuseHzPast2155 int
	refuseHzInScope  int
	// The severity question for a differing refusal message: is the class
	// BOUNDED to screens the client does not compare? Counted, not assumed —
	// "it only happens past 2155" is a claim, and this project retires one of
	// those most rounds.
	refuseMsgDifferInScope int
	// Cases whose horizon only the ARITHMETIC bound can see, because both
	// engines' reported last years wrapped past DOS's 2155 byte ceiling. These
	// were counted IN SCOPE by round 21's era split.
	eraWrapRescued int
}

// plainDOSRow is one line of the oracle's `rows` dump. The three figures are
// DOS's own rendered cents (amort_oracle.pas:1186-1191), not engine doubles.
type plainDOSRow struct {
	num                         int
	interest, prinPaid, balance float64
}

// pairRefusal is ROUND 22's answer to "what is in the 14% the round-21
// instrument threw away?".
//
// It runs on every case the ORACLE refused, and it asks fuzzer5's question,
// which the plain differential had never asked: given that DOS said no, what
// did the PORT say?
//
// Three outcomes, and only one of them is a defect:
//
//   - the port also refused → refusal is PAIRED. Fidelity. Counted, and its
//     MESSAGE class recorded, because two engines rejecting the same screen for
//     different stated reasons is a real user-visible difference even though no
//     schedule diverges.
//   - the port produced a schedule and DOS's refusal was an INPUT rejection →
//     HARD. This is `go_solved_dos_refused`, the one thing a port must never do.
//   - the port produced a schedule and DOS's refusal was NON-CONVERGENCE →
//     adjudicated, not scored: if the schedule genuinely retires the loan the
//     port found a root DOS's bisection missed (the known, benign solver gap);
//     if it does NOT retire, the port invented a schedule out of a screen with
//     no answer, which is the same class of bug as the HARD case. fuzzer5 has
//     drawn exactly this distinction since round 16; it is reproduced rather
//     than reinvented.
//
// hzYear is an ARITHMETIC bound, not a date: n payments at perYr per year span
// n/perYr years from the loan year. R2 forbids the harness computing a date to
// COMPARE; this one is used only to bucket a refusal for the report, and saying
// so is the difference between the two.
func (l *plainDiffLedger) pairRefusal(t *testing.T, oracleOut, repro string,
	loanYear, n, perYr int, r AmortResult, amount float64) {

	class := "other"
	switch {
	case strings.Contains(oracleOut, "at least two regular payments"):
		class = "twopay"
		l.refuseTwoPay++
	case strings.Contains(oracleOut, "did not converge"):
		class = "nonconv"
		l.refuseNonConv++
	default:
		l.refuseOther++
	}

	hzYear := loanYear + (n+perYr-1)/perYr
	if hzYear > 2155 {
		l.refuseHzPast2155++
	} else if hzYear <= 2099 {
		l.refuseHzInScope++
	}

	goOK := r.Err == nil && len(r.Schedule) > 0
	if !goOK {
		l.refusePaired++
		// Same screen, same verdict — but is it the same REASON? DOS's text is
		// the thing being ported, so a differing message is a divergence in a
		// displayed cell. §59's rule applies: establish whether it is USED
		// before assigning severity. It IS used — the user reads it — so it is
		// counted and reported, and it is not scored as a schedule divergence
		// because no schedule exists on either side.
		portMsg := "port: nil error, empty schedule"
		if r.Err != nil {
			portMsg = r.Err.Error()
		}
		// Classify the PORT's message the same way DOS's was classified, then
		// compare the two CLASSES. An earlier version of this asked only
		// "do both say non-converged?", which scores "neither says
		// non-converged" as agreement — and that silently reported 173 of 173
		// refusals as same-message when 98 of them carried two different
		// sentences. A comparison whose negative branch cannot distinguish
		// "same" from "both unlike a third thing" is not a comparison.
		portClass := "other"
		switch {
		case strings.Contains(portMsg, "at least two regular payments"):
			portClass = "twopay"
		case strings.Contains(portMsg, "did not converge"):
			portClass = "nonconv"
		}
		if portClass == class {
			l.refuseMsgSame++
		} else {
			l.refuseMsgDiffer[class+" -> port: "+firstSentence(portMsg)]++
			if hzYear <= 2099 {
				l.refuseMsgDifferInScope++
				t.Logf("SIG=ADVISORY:plain_refusal_message_differs_in_scope — both engines "+
					"reject this screen, DOS calling it %q and the port %q, on a loan whose "+
					"arithmetic horizon is %d. No schedule diverges; the sentence the user "+
					"reads does.\n  %s", class, firstSentence(portMsg), hzYear, repro)
			}
		}
		return
	}

	// The port answered a screen DOS refused.
	if class == "nonconv" {
		bal := r.Schedule[len(r.Schedule)-1].Principal
		if math.Abs(bal) <= math.Max(1.0, 1e-4*math.Abs(amount)) {
			l.nonConvGoRetires++
		} else {
			l.nonConvGoSpurious++
			t.Logf("SIG=ADVISORY:plain_dos_nonconverge_go_nonretiring — the port built a "+
				"schedule from a screen DOS could not solve, and it does NOT retire "+
				"(final balance %.2f on %d rows)\n  %s",
				bal, len(r.Schedule), repro)
		}
		return
	}
	l.goSolvedDosRefuse++
	t.Errorf("SIG=HARD:plain_go_solved_dos_refused — DOS REFUSED this screen (%s) and the "+
		"port produced a %d-row schedule anyway (int=%.2f paid=%.2f). A port must not "+
		"answer a screen the original rejects.\n  %s",
		class, len(r.Schedule), r.TotalInt, r.TotalPaid, repro)
}

// firstSentence keeps a refusal message groupable without letting a long
// user-facing string dominate the report.
func firstSentence(s string) string {
	if i := strings.Index(s, "."); i > 0 {
		return s[:i+1]
	}
	if len(s) > 60 {
		return s[:60]
	}
	return s
}

func (l *plainDiffLedger) note(kind, repro string) {
	if l.curOutOfScope {
		l.outScopeSig++
	} else {
		l.inScopeSig++
		if len(l.worstInScope) < 20 {
			l.worstInScope = append(l.worstInScope, kind+": "+repro)
		}
	}
	switch kind {
	case "rows":
		l.rowDiff++
	case "int":
		l.intDiff++
	case "paid":
		l.paidDiff++
	case "pay":
		l.payDiff++
	}
	if len(l.worst) < 12 {
		l.worst = append(l.worst, kind+": "+repro)
	}
}

// totalsTol is the tolerance the project's other totals comparisons use:
// absolute a penny, relative 1e-7. Scaled to the value it guards (R10) — these
// figures are DOS's own 2-decimal totals line, re-parsed, so a penny is the
// resolution of the quantity itself and not a guess.
func totalsTol(x float64) float64 {
	t := 1e-7 * math.Abs(x)
	if t < 0.01 {
		t = 0.01
	}
	return t
}

func TestDOSPlainLoanDifferential(t *testing.T) {
	if os.Getenv("PERSENSE_FUZZ") == "" && os.Getenv("PERSENSE_PLAINDIFF") == "" {
		// Opt-in like the project's other randomized differentials, so a real
		// finding fails loudly for whoever is hunting rather than reddening the
		// default gate for everyone. The committed §62 cases
		// (zzsec62_feb_grid_test.go) run unconditionally and are the standing
		// guard; this is the sweep.
		t.Skip("randomized plain-loan differential; set PERSENSE_PLAINDIFF=1 (or PERSENSE_FUZZ=1)")
	}
	bin := oracleBin
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("oracle required but not present (%s)", bin)
		}
		t.Skipf("amort oracle not present (%s)", bin)
	}

	cases := 1200
	if v := os.Getenv("PERSENSE_PLAINDIFF_N"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cases = n
		}
	}
	seed := int64(21000)
	if v := os.Getenv("PERSENSE_PLAINDIFF_SEED"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			seed = n
		}
	}
	rng := rand.New(rand.NewSource(seed))
	L := &plainDiffLedger{byPerYr: map[int]int{}, byBasis: map[string]int{},
		refuseMsgDiffer: map[string]int{}}

	for i := 0; i < cases; i++ {
		L.generated++

		amount := quantize(float64(25000+rng.Intn(475000)), 2)
		rate := quantize(0.03+rng.Float64()*0.11, 6)
		perYr := []int{1, 2, 3, 4, 6, 12, 24, 26}[rng.Intn(8)]
		// Terms in whole years would put the last payment in the SAME month as
		// the first for perYr=1 only, and one month earlier otherwise — which is
		// how the February shape stayed rare. Draw the period count directly so
		// the last month walks the whole calendar.
		n := 2 + rng.Intn(360)

		ldY := 2023 + rng.Intn(3)
		ldM := time.Month(1 + rng.Intn(12))
		ldD := 1 + rng.Intn(31)
		// Clamp to a real day BEFORE building the record: types.NewDateRec
		// normalises, so an unclamped 31 February would silently become 3 March
		// and change the month the whole series is anchored on.
		if last := int(dateutil.DaysInM(types.NewDateRec(ldY, ldM, 1))); ldD > last {
			ldD = last
		}
		loanDate := types.NewDateRec(ldY, ldM, ldD)

		mPer := 1
		if perYr >= 1 && perYr <= 12 && 12%perYr == 0 {
			mPer = 12 / perYr
		}
		firstMonths := mPer
		odd := false
		if mPer > 1 && rng.Intn(4) == 0 {
			firstMonths = 1 + rng.Intn(2*mPer)
			odd = firstMonths != mPer
		}
		firstDate := fz5AddMonths(loanDate, firstMonths)

		basisTok, basisName := "", "360"
		basis := types.Basis360
		switch rng.Intn(4) {
		case 1:
			basisTok, basisName, basis = "b365", "365", types.Basis365
		case 2:
			basisTok, basisName, basis = "b365_360", "365/360", types.Basis365360
		}

		args := []string{
			strconv.FormatFloat(amount, 'f', 2, 64),
			strconv.FormatFloat(rate, 'f', 6, 64),
			strconv.Itoa(n), strconv.Itoa(perYr),
			fmt.Sprintf("loandmy=%d.%d.%d", loanDate.Time.Day(),
				int(loanDate.Time.Month()), loanDate.Time.Year()),
			fmt.Sprintf("firstdmy=%d.%d.%d", firstDate.Time.Day(),
				int(firstDate.Time.Month()), firstDate.Time.Year()),
		}
		if basisTok != "" {
			args = append(args, basisTok)
		}
		repro := "amort_oracle " + joinArgs(args)

		// The port's input, built once so the refusal-pairing branch below can
		// use it too. Round 21 built this only AFTER the oracle had answered,
		// which is precisely why no refused case was ever paired.
		yrDays, yrInv := 360.0, 1.0/360
		if basis == types.Basis365 {
			yrDays, yrInv = 365.25, 1/365.25
		}
		in := gzLoanInput(amount, rate, n, perYr, Settings{
			Basis: basis, PerYr: byte(perYr), YrDays: yrDays, YrInv: yrInv})
		in.Loan.LoanDate, in.Loan.FirstDate = loanDate, firstDate

		// ---- DOS ----
		out, err := exec.Command(bin, append(append([]string{}, args...), "bdump")...).Output()
		txt := string(out)
		if err != nil || strings.Contains(txt, "ERR ") {
			L.oracleRefuse++
			L.pairRefusal(t, txt, repro, ldY, n, perYr, Amortize(in), amount)
			continue
		}
		var dPay, dInt, dPaid float64
		gotQuiet := false
		for _, ln := range strings.Split(txt, "\n") {
			f := strings.Fields(ln)
			if len(f) >= 6 && f[0] == "payment" && f[2] == "interest" && f[4] == "paid" {
				dPay, _ = strconv.ParseFloat(f[1], 64)
				dInt, _ = strconv.ParseFloat(f[3], 64)
				dPaid, _ = strconv.ParseFloat(f[5], 64)
				gotQuiet = true
			}
		}
		if !gotQuiet {
			L.unparsed++
			continue
		}
		// DOS's -1/-1 no-totals sentinel is NOT a zero and must not be scored
		// either way (the standing rule about an ambiguous oracle response).
		if dInt <= -1 && dPaid <= -1 {
			L.noTotals++
			continue
		}
		dNPer, dLastY, dLastM, dLastD, okLast := parseBdumpLastLine(txt)
		dRows, dTrailingZero := 0, false
		var dRowVals []plainDOSRow
		if rowsOut, rerr := exec.Command(bin, append(append([]string{}, args...), "rows")...).Output(); rerr == nil {
			var lastRow string
			for _, ln := range strings.Split(string(rowsOut), "\n") {
				if strings.HasPrefix(ln, "row ") {
					dRows++
					lastRow = ln
					// ROUND 22: keep the VALUES, not just the count. They cost
					// nothing — this exec already ran.
					if f := strings.Fields(ln); len(f) >= 8 {
						var rv plainDOSRow
						rv.num, _ = strconv.Atoi(f[1])
						rv.interest, _ = strconv.ParseFloat(f[3], 64)
						rv.prinPaid, _ = strconv.ParseFloat(f[5], 64)
						rv.balance, _ = strconv.ParseFloat(f[7], 64)
						dRowVals = append(dRowVals, rv)
					}
				}
			}
			// DOS PRINTS ONE ROW PAST THE LAST PAYMENT ON SOME SCREENS, AND IT
			// IS ALL ZEROS. AMORTOP.pas:1136-1139 says why in DOS's own words:
			// when printing the entire table it advances WhenToStop one further
			// "in order to print the last line". At the far end of the calendar
			// that surfaces as a trailing `int 0.00 prin 0.00 bal 0.00` row the
			// port does not emit — carrying no money, and the totals agree to the
			// cent on both sides.
			//
			// This is §59's lesson as a rule rather than an anecdote: establish
			// whether a divergent row is USED before assigning severity. It is
			// COUNTED and printed, never silently dropped — an uncounted
			// exclusion is the silent bucket R5 and R8 exist to prevent.
			f := strings.Fields(lastRow)
			if len(f) >= 8 && f[3] == "0.0000" && f[5] == "0.0000" && f[7] == "0.0000" {
				dTrailingZero = true
			}
		}

		// ---- the port, identical inputs ----
		r := Amortize(in)
		if r.Err != nil || len(r.Schedule) == 0 {
			L.goRefuse++
			continue
		}

		L.compared++
		L.byPerYr[perYr]++
		L.byBasis[basisName]++
		if odd {
			L.oddFirst++
		}
		if ldD >= 29 || firstDate.Time.Day() >= 29 {
			L.anchorHigh++
		}
		// ERA SPLIT — the client's 2099 boundary (2026-08-03). Generation still
		// reaches DOS's whole range; only the REPORT splits.
		//
		// ROUND 22 — THE LABEL WAS WRONG, AND IT WAS WRONG IN THE DIRECTION THAT
		// FLATTERS THE HEADLINE.
		//
		// Round 21 keyed this on DOS's REPORTED last year alone. DOS's `daterec`
		// year is a byte based at 1900 (§55), so on any series long enough to run
		// past 2155 DOS's reported year WRAPS — and a wrapped year is small. The
		// measured consequence, from arm B of this round's standing ranges:
		//
		//   amort_oracle 428108.00 0.059685 296 1 loandmy=25.3.2024 firstdmy=25.3.2025
		//   DOS: lastdate 3/25/2064 nperiods 296
		//
		// 296 annual payments from 2024 last land in 2320. DOS says 2064
		// (2320-256), so the old test filed a 296-YEAR loan as IN SCOPE — and
		// that single case contributed 20 of arm B's 52 "in-scope" signals. The
		// in-scope rate this instrument published was contaminated by exactly the
		// long-horizon cases the client's boundary exists to exclude.
		//
		// A STRATIFICATION LABEL IS A COVERAGE CLAIM, and this one was asserted
		// against a field that cannot express the quantity it was being read for.
		//
		// The fix uses three sources and takes the WORST, because each covers a
		// different failure of the others:
		//   - DOS's reported year: right until it wraps.
		//   - the PORT's own resolved last year: the engine's answer, not a
		//     harness derivation (R2), and it is what fuzzer5 has keyed on since
		//     round 19 — but it can also be short if the loan retires early.
		//   - an ARITHMETIC bound, loanYear + ceil(n/perYr): not a date and not
		//     subject to any calendar's representation limits, so it is the one
		//     that survives the wrap.
		// Any of the three saying "past 2099" puts the case out of scope. That is
		// deliberately conservative: it can move a borderline case OUT of the
		// in-scope population, never into it, so the in-scope rate can only be
		// pessimistic and never flattering.
		portLastYear := 0
		if nS := len(r.Schedule); nS > 0 {
			portLastYear = r.Schedule[nS-1].Date.Time.Year()
		}
		if dateutil.DateOK(r.LastDate) && r.LastDate.Time.Year() > portLastYear {
			portLastYear = r.LastDate.Time.Year()
		}
		arithHorizon := ldY + (n+perYr-1)/perYr
		L.curOutOfScope = (okLast && dLastY > 2099) || portLastYear > 2099 ||
			arithHorizon > 2099
		if arithHorizon > 2099 && okLast && dLastY <= 2099 && portLastYear <= 2099 {
			// Both engines' reported years wrapped. Counted so the size of the
			// class that used to be mislabelled is visible rather than inferred.
			L.eraWrapRescued++
		}
		if L.curOutOfScope {
			L.outScope++
		} else {
			L.inScope++
		}
		if okLast {
			if L.minLastYear == 0 || dLastY < L.minLastYear {
				L.minLastYear = dLastY
			}
			if dLastY > L.maxLastYear {
				L.maxLastYear = dLastY
			}
			if dLastM == 2 {
				L.febLast++
				if firstDate.Time.Day() > dLastD {
					L.febClamped++
				}
			}
		}

		gRows := 0
		for _, row := range r.Schedule {
			if row.PayNum >= 1 {
				gRows++
			}
		}
		if dTrailingZero && dRows == gRows+1 {
			L.trailingZeroRow++
		} else if dRows > 0 && gRows != dRows {
			L.note("rows", fmt.Sprintf("DOS %d rows, port %d | %s", dRows, gRows, repro))
		}
		if math.Abs(r.TotalInt-dInt) > totalsTol(dInt) {
			L.note("int", fmt.Sprintf("DOS %.2f, port %.2f | %s", dInt, r.TotalInt, repro))
		}
		if math.Abs(r.TotalPaid-dPaid) > totalsTol(dPaid) {
			L.note("paid", fmt.Sprintf("DOS %.2f, port %.2f | %s", dPaid, r.TotalPaid, repro))
		}

		// ---- ROUND 22: the payment amount ----
		// `payment` is h^.payamt, a REAL ENGINE DOUBLE — the oracle says so at
		// its own emit site (amort_oracle.pas:1197-1203) and gives it raw bits,
		// unlike the two totals which are re-parsed out of a formatted line.
		// Round 21 parsed it and threw it away. It is the single number the user
		// looks at first.
		gPay := 0.0
		for _, row := range r.Schedule {
			if row.PayNum >= 1 && row.PayAmt != 0 {
				gPay = row.PayAmt
				break
			}
		}
		if math.Abs(gPay-dPay) > totalsTol(dPay) {
			L.note("pay", fmt.Sprintf("DOS %.4f, port %.4f | %s", dPay, gPay, repro))
		}

		// ---- ROUND 22: the horizon the two engines actually resolved ----
		// The last date was already being read; it was used to pick an era
		// bucket and never compared. A schedule whose row count and totals agree
		// can still end on a different DAY — §62's neighbourhood exactly, and
		// the class §59 lived in.
		if okLast {
			gl := r.LastDate
			if gl.Time.Year() != dLastY || int(gl.Time.Month()) != dLastM || gl.Time.Day() != dLastD {
				L.lastDiff++
				L.note("lastdate", fmt.Sprintf("DOS %d/%d/%d, port %d/%d/%d | %s",
					dLastM, dLastD, dLastY, int(gl.Time.Month()), gl.Time.Day(),
					gl.Time.Year(), repro))
			}
			if dNPer != r.NPeriods {
				L.nperDiff++
				L.note("nperiods", fmt.Sprintf("DOS %d, port %d | %s", dNPer, r.NPeriods, repro))
			}
		}

		// ---- ROUND 22: THE ROWS THEMSELVES ----
		//
		// Until now this instrument compared a row COUNT and two TOTALS. Those
		// three numbers are compatible with every row inside the schedule being
		// wrong in offsetting pairs, and the totals tolerance (a penny) is looser
		// than a single row's own resolution.
		//
		// RESOLUTION IS THE ORACLE'S, NOT A GUESS (R10). The oracle's `row` line
		// does not carry engine doubles: amort_oracle.pas:1186-1191 `Val`s the
		// three figures back OUT of DOS's own rendered screen line, so they are
		// DOS's displayed CENTS printed with four decimals. The right comparison
		// is therefore cent-to-cent — and it cannot be tightened, because there
		// is no more information in the source. Measured: the largest raw gap
		// over 173,246 rows is 0.0050, exactly the half-cent print boundary and
		// not an engine difference.
		//
		// FIELD SEMANTICS, WHICH NEARLY MANUFACTURED 173,246 DIVERGENCES. The
		// oracle's `prin` is the principal PAID THIS PERIOD; the port's
		// PaymentRecord.Principal is the balance REMAINING AFTER IT
		// (types.go:640). The oracle's `bal` is the field that corresponds to it.
		// Comparing the two like-named fields reports every row in every
		// schedule as divergent. Written down because the harness is a suspect
		// before the engine is, and this one was caught by reading the Pascal.
		if len(dRowVals) > 0 {
			var gRowVals []PaymentRecord
			for _, row := range r.Schedule {
				if row.PayNum >= 1 {
					gRowVals = append(gRowVals, row)
				}
			}
			if len(dRowVals) == len(gRowVals) {
				// THE TERMINATING-BALLOON FINAL ROW, identified before the row
				// loop so it is never counted as three ordinary row-value
				// divergences. Signature, all four parts required: it is the LAST
				// row; DOS still shows a balance there; the port shows none; and
				// the gap is exactly the extra money the port's final payment
				// carries over the regular one. That last clause is what makes it
				// a folded balloon rather than a coincidence — it ties DOS's
				// leftover balance to the port's oversized final payment.
				balloonRow := -1
				if nR := len(dRowVals); nR >= 2 {
					dLeft := dRowVals[nR-1].balance
					gLeft := quantize(gRowVals[nR-1].Principal, 2)
					extra := gRowVals[nR-1].PayAmt - gRowVals[nR-2].PayAmt
					if dLeft > 0.005 && math.Abs(gLeft) <= 0.005 &&
						math.Abs(extra-dLeft) <= 0.02 {
						balloonRow = nR - 1
						L.balloonFinalRow++
						if !L.curOutOfScope {
							L.balloonFinalRowInScope++
							t.Logf("SIG=ADVISORY:plain_terminating_balloon_final_row — DOS leaves "+
								"%.2f standing on its last grid row (its terminating balloon, "+
								"de-activated per Amortize.pas:1040-1088) and the port folds the "+
								"same %.2f into that row's payment. Payment, both totals, "+
								"nperiods and the last date all agree to the cent.\n  %s",
								dLeft, extra, repro)
						}
					}
				}
				prevBal := amount
				for k := range dRowVals {
					L.rowsCompared++
					if k == balloonRow {
						prevBal = gRowVals[k].Principal
						continue
					}
					// tie reports whether the port's raw value sits close enough
					// to a half-cent boundary that the CENT it rounds to is a
					// property of the two formatters rather than of the two
					// engines. The window is one hundredth of a cent: §48/§49
					// measured FPC's formatter disagreeing with Go's on ties at
					// the sixth decimal, so a raw value this close to .xx5 has no
					// determinate cent on either side.
					tie := func(raw float64) bool {
						f := math.Abs(math.Abs(raw)*100 - math.Floor(math.Abs(raw)*100))
						return math.Abs(f-0.5) < 1e-3
					}
					if math.Abs(quantize(gRowVals[k].Interest, 2)-dRowVals[k].interest) > 0.0051 {
						L.rowIntDiff++
						if tie(gRowVals[k].Interest) {
							L.rowTieInt++
						}
						L.note("rowint", fmt.Sprintf("row %d DOS int %.2f, port %.4f (tie=%v) | %s",
							dRowVals[k].num, dRowVals[k].interest, gRowVals[k].Interest,
							tie(gRowVals[k].Interest), repro))
					}
					if math.Abs(quantize(gRowVals[k].Principal, 2)-dRowVals[k].balance) > 0.0051 {
						L.rowBalDiff++
						if tie(gRowVals[k].Principal) {
							L.rowTieBal++
						}
						L.note("rowbal", fmt.Sprintf("row %d DOS bal %.2f, port %.4f (tie=%v) | %s",
							dRowVals[k].num, dRowVals[k].balance, gRowVals[k].Principal,
							tie(gRowVals[k].Principal), repro))
					}
					if math.Abs(quantize(prevBal-gRowVals[k].Principal, 2)-dRowVals[k].prinPaid) > 0.0051 {
						L.rowPrinDiff++
						if tie(prevBal - gRowVals[k].Principal) {
							L.rowTiePrin++
						}
						L.note("rowprin", fmt.Sprintf("row %d DOS prin %.2f, port %.4f (tie=%v) | %s",
							dRowVals[k].num, dRowVals[k].prinPaid,
							prevBal-gRowVals[k].Principal, tie(prevBal-gRowVals[k].Principal), repro))
					}
					prevBal = gRowVals[k].Principal
				}
			}
		}
	}

	// ---- the ledger (R5): every generated case lands in exactly one bucket ----
	accounted := L.compared + L.oracleRefuse + L.goRefuse + L.noTotals + L.unparsed
	t.Logf("ledger: generated %d = compared %d + oracle-refused %d + port-refused %d + "+
		"no-totals %d + unparsed %d | UNACCOUNTED %d",
		L.generated, L.compared, L.oracleRefuse, L.goRefuse, L.noTotals, L.unparsed,
		L.generated-accounted)
	if L.generated-accounted != 0 {
		t.Errorf("ledger does not balance: %d generated cases are unaccounted for. "+
			"An invisible denominator is how a 5%% rate and a 50%% rate come to "+
			"look identical (R5).", L.generated-accounted)
	}
	t.Logf("coverage: anchor day>=29 %d | February last date %d (of which CLAMPED %d) | "+
		"odd first period %d | DOS last years %d-%d",
		L.anchorHigh, L.febLast, L.febClamped, L.oddFirst, L.minLastYear, L.maxLastYear)
	t.Logf("coverage: perYr %v | basis %v", L.byPerYr, L.byBasis)
	t.Logf("signals: row-count %d, total-interest %d, total-paid %d, PAYMENT %d, "+
		"LAST-DATE %d, NPERIODS %d (of %d compared) | DOS trailing all-zero rows excluded %d",
		L.rowDiff, L.intDiff, L.paidDiff, L.payDiff, L.lastDiff, L.nperDiff,
		L.compared, L.trailingZeroRow)
	t.Logf("signals PER ROW: interest %d, principal-paid %d, balance %d (of %d rows "+
		"compared, at the oracle's own CENT resolution)",
		L.rowIntDiff, L.rowPrinDiff, L.rowBalDiff, L.rowsCompared)
	t.Logf("PER-ROW numerator adjudicated: half-cent PRINT TIES — interest %d, "+
		"principal-paid %d, balance %d. A tie is a row whose port raw value lies "+
		"within 1e-4 of a .xx5 boundary, where the cent is a property of the two "+
		"FORMATTERS (§48/§49) and not of the two engines. Counted, never silenced.",
		L.rowTieInt, L.rowTiePrin, L.rowTieBal)
	t.Logf("terminating-balloon final rows (§59's mechanism on the plain surface): "+
		"%d cases, of which IN SCOPE %d. DOS leaves the de-activated balloon "+
		"standing on its last grid row; the port folds it in. Every computed value "+
		"agrees; excluded from the row-value counters and reported as its own class.",
		L.balloonFinalRow, L.balloonFinalRowInScope)
	// THE REFUSAL BUCKET, WITH CONTENTS. Printed at every level including zero
	// (R13/R8): a bucket that prints nothing when empty is how a bucket stops
	// being looked at.
	t.Logf("refusals: oracle refused %d = twopay %d + nonconverge %d + other %d | "+
		"PAIRED (port also refused) %d | go-solved-dos-refused %d (HARD) | "+
		"dos-nonconverge port retires %d / port spurious %d",
		L.oracleRefuse, L.refuseTwoPay, L.refuseNonConv, L.refuseOther,
		L.refusePaired, L.goSolvedDosRefuse, L.nonConvGoRetires, L.nonConvGoSpurious)
	t.Logf("refusal messages: same class %d | differing %v | of the differing, IN SCOPE "+
		"(horizon <=2099) %d", L.refuseMsgSame, L.refuseMsgDiffer, L.refuseMsgDifferInScope)
	t.Logf("refusal horizon (arithmetic bound, not a date): past DOS's 2155 ceiling %d | "+
		"in scope <=2099 %d", L.refuseHzPast2155, L.refuseHzInScope)
	t.Logf("era split: in-scope<=2099 compared %d signals %d | out-of-scope>2099 "+
		"compared %d signals %d | of the out-of-scope, %d were visible ONLY to the "+
		"arithmetic bound because both engines' reported years had wrapped (§55) — "+
		"round 21 counted those as IN SCOPE",
		L.inScope, L.inScopeSig, L.outScope, L.outScopeSig, L.eraWrapRescued)
	// IN-SCOPE FIRST AND ALWAYS. These are the signals that fail the test; the
	// general list below is capped and can fill up with out-of-scope draws.
	for _, w := range L.worstInScope {
		t.Logf("   IN-SCOPE %s", w)
	}
	for _, w := range L.worst {
		t.Logf("   %s", w)
	}

	if L.compared == 0 {
		t.Fatalf("NOTHING was compared — a differential that compares nothing reports green (R5).")
	}
	// A row-level assertion that never reached a row is the same silent pass as
	// a differential that compared nothing, one level down (R12).
	if L.rowsCompared == 0 {
		t.Errorf("NO ROW was compared. The per-row assertions are the ones that can see " +
			"a schedule wrong in offsetting pairs; if they reach nothing, the totals " +
			"comparison is back to being the only real check and this test is round 21's.")
	}
	// Likewise the refusal bucket: it was 14%% of the population when it was
	// first opened, and an instrument that stops reaching it has stopped
	// answering the question this round added.
	if L.oracleRefuse > 0 && L.refusePaired+L.goSolvedDosRefuse+L.nonConvGoRetires+
		L.nonConvGoSpurious != L.oracleRefuse {
		t.Errorf("the refusal bucket does not balance: %d refused but %d adjudicated. "+
			"R5 applies inside a bucket as well as across them.", L.oracleRefuse,
			L.refusePaired+L.goSolvedDosRefuse+L.nonConvGoRetires+L.nonConvGoSpurious)
	}

	// ---- the draw is asserted, not described (R7) ----
	if L.anchorHigh == 0 {
		t.Errorf("the draw produced NO loan anchored on the 29th-31st. That is the " +
			"anchor §62 lives on; without it this test's clean rate would describe a " +
			"population that cannot contain the defect it was written for.")
	}
	if L.febClamped == 0 {
		t.Errorf("the draw produced NO schedule whose last payment lands on a CLAMPED "+
			"February (%d landed in February at all). That is §62's exact shape and "+
			"the reason this file exists; a clean rate over a draw without it means "+
			"nothing.", L.febLast)
	}

	// ---- the differential itself ----
	// SEVERITY FOLLOWS THE ERA SPLIT (round 19's decision, client 2026-08-03):
	// keep GENERATING past 2099, split the REPORT. An in-scope plain-loan
	// divergence fails; an out-of-scope one is reported and travels in the
	// record. §62 was in-scope, which is why it mattered.
	if L.inScopeSig > 0 {
		t.Errorf("IN-SCOPE PLAIN-LOAN DIVERGENCES: %d signals over %d in-scope cases "+
			"(horizon <=2099). The plain path is the simplest thing the product does "+
			"and the shape most real screens have; a divergence here is not a corner "+
			"case.", L.inScopeSig, L.inScope)
	}
	if L.outScopeSig > 0 {
		t.Logf("SIG=ADVISORY:plain_out_of_scope — %d signals over %d cases whose horizon "+
			"is past 2099, outside the client's comparison boundary. Reported, not "+
			"failed; the reproducing commands are above.", L.outScopeSig, L.outScope)
	}
}
