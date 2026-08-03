package amortization

// RANDOMIZED PAYOFF DIFFERENTIAL (round 18b).
//
// `TestPayoffVsOracleSweep` compares the port's PayoffBalance against DOS's real
// ComputeBalanceFromDate across 10 option modes x 7 dates — 70 comparisons, all
// of them on the SAME loan: 100000 / 8% / 360 / 12. That is a set of golden
// cases, not a measured rate, and until round 18b the backlog described the
// whole surface as having "no differential coverage at all", which was wrong in
// the other direction.
//
// This test varies the loan. Same oracle query, same comparison, same documented
// in-advance frontier — but amount, rate, term, payment frequency, both dates and
// the payoff date are all drawn, so the result is a RATE over a stated envelope
// rather than a pass/fail on one screen.
//
// Discipline carried over from the amortization harness, because every one of
// these rules was bought with a defect:
//
//   - R5: every generated case lands in exactly one counted bucket and the
//     ledger must balance. A case that falls out silently is invisible, and a
//     shrinking denominator makes 5% and 50% look identical.
//   - Rule 9: the rate is reported over ACTUALLY COMPARED, never over generated,
//     and the per-flag BASE RATES are printed so no future session can quote an
//     enrichment without its denominator.
//   - The unquantized-argument trap: every value crosses the process boundary as
//     text, so it is quantized BEFORE both engines see it.
//   - R6: the in-advance non-R78 frontier is a documented approximation in the
//     port, so it is bucketed and reported, not silently tolerated and not
//     failed.

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

func TestPayoffRandomizedSweep(t *testing.T) {
	// OPT-IN, like every other differential fuzzer in this package
	// (PERSENSE_FUZZ=1), and for the same reason: a randomized differential that
	// finds a REAL divergence must fail, and a permanently-red default suite
	// destroys the NEW=0 discipline that standing rule 4 depends on. The committed
	// corpus tests are the always-green gate; the fuzzers are the hunt.
	//
	// It found one on its first run — see the §-note below — so this is not a
	// hypothetical distinction.
	if os.Getenv("PERSENSE_FUZZ") == "" {
		t.Skip("opt-in: set PERSENSE_FUZZ=1 to run the randomized payoff differential")
	}
	gateOracle(t)

	ask := func(args ...string) (float64, bool) {
		out, err := exec.Command(payoffOracleBin, args...).CombinedOutput()
		if err != nil {
			return 0, false
		}
		f := strings.Fields(strings.TrimSpace(string(out)))
		if len(f) >= 2 && f[0] == "payoff" {
			v, e := strconv.ParseFloat(f[1], 64)
			return v, e == nil
		}
		return 0, false
	}

	rng := rand.New(rand.NewSource(1802))
	const cases = 600
	const cents = 0.01

	// Ledger buckets (R5).
	var generated, compared, oracleRefused, goRefused, frontier, dosZero int
	var dosZeroRepro []string
	var diverged int
	worst := 0.0
	worstCmd := ""
	base := map[string]int{}
	sig := map[string]int{}

	for i := 0; i < cases; i++ {
		generated++

		amount := quantize(float64(5000+rng.Intn(495000)), 2)
		rate := quantize(0.02+rng.Float64()*0.16, 6)
		// MONTHLY ONLY, and the restriction is R2, not laziness. The first payment
		// date below is drawn one month after the loan date; that is exactly one
		// period only at perYr=12. At every other frequency it is an OFF-GRID first
		// period, and a harness-computed off-grid date is the single family that
		// has produced eight defects in this project. The first run of this sweep
		// with all eight frequencies measured 27 divergences in 433 with
		// sub-monthly enriched x2.56 (25 of 27, base rate 36%) — which is exactly
		// what an off-grid first period would ALSO produce, so that population is
		// unadjudicable as built and is not evidence about payoff.
		//
		// Widening this axis needs the first date to come from the engine's own
		// derivation rather than from here (drop `firstdmy=` and let both sides
		// default it), which is a separate change with its own gate. Until then the
		// envelope says monthly and the rate means monthly.
		perYr := 12
		years := 2 + rng.Intn(28)
		n := years * perYr
		if n < 2 {
			n = 2
		}

		basis := []types.BasisType{types.Basis360, types.Basis365, types.Basis365360}[rng.Intn(3)]
		inadv := rng.Intn(2) == 0
		r78 := rng.Intn(2) == 0
		exact := rng.Intn(2) == 0
		prepaid := rng.Intn(2) == 0

		var tok []string
		switch basis {
		case types.Basis365:
			tok = append(tok, "b365")
		case types.Basis365360:
			tok = append(tok, "b365_360")
		}
		if exact {
			tok = append(tok, "exact")
		}
		if prepaid {
			tok = append(tok, "prepaid")
		}
		if inadv {
			tok = append(tok, "inadv")
		}
		if r78 {
			tok = append(tok, "r78")
		}
		for _, k := range tok {
			base[k]++
		}
		base["all"]++
		// perYr is a CONFOUNDER by construction: the first payment date is drawn
		// one month after the loan date regardless of frequency, so a sub-monthly
		// loan gets an off-grid first period. Any basis/option enrichment has to be
		// read against this before it means anything (rule 9, and round 18b's
		// addition — adjudicate before naming a frontier).
		base[fmt.Sprintf("perYr=%02d", perYr)]++
		if perYr > 12 {
			base["submonthly"]++
		}

		loanY := 2015 + rng.Intn(10)
		loanM := 1 + rng.Intn(12)
		loanD := []int{1, 15, 28}[rng.Intn(3)]
		loanDate := types.NewDateRec(loanY, time.Month(loanM), loanD)
		// One month after the loan date — what the existing payoff sweep uses and
		// what DOS defaults to. Deliberately NOT computed per-frequency: R2 says
		// the harness does not invent dates, and a hand-rolled sub-monthly step is
		// exactly the family that has produced eight defects.
		firstDate := loanDate
		firstDate.Time = firstDate.Time.AddDate(0, 1, 0)

		// A payoff date somewhere in [loan date, a little past the last payment].
		spanMonths := int(float64(n) / float64(perYr) * 12.0)
		off := rng.Intn(spanMonths + 13)
		payoffDate := loanDate
		payoffDate.Time = payoffDate.Time.AddDate(0, off, 0)
		if d := rng.Intn(3); d == 1 {
			payoffDate.Time = payoffDate.Time.AddDate(0, 0, 14) // mid-period
		}

		yd := 365.0
		if basis == types.Basis360 {
			yd = 360
		}
		s := Settings{Basis: basis, PerYr: byte(perYr), YrDays: yd, YrInv: 1.0 / yd,
			Prepaid: prepaid, InAdvance: inadv, R78: r78, Exact: exact}
		if basis == types.Basis365360 {
			s.YrInv = 1.0 / 360
		}

		in := LoanInput{Settings: s, Loan: Loan{
			NStatus: types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: perYr,
			LoanDateStatus: types.InOutInput, LoanDate: loanDate,
			FirstStatus: types.InOutInput, FirstDate: firstDate,
			AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			PayAmtStatus: types.StatusEmpty,
		}}

		oargs := []string{
			strconv.FormatFloat(amount, 'f', 2, 64),
			strconv.FormatFloat(rate, 'f', 6, 64),
			strconv.Itoa(n), strconv.Itoa(perYr),
			fmt.Sprintf("loandmy=%d.%d.%d", loanDate.Time.Day(), int(loanDate.Time.Month()), loanDate.Time.Year()),
			fmt.Sprintf("firstdmy=%d.%d.%d", firstDate.Time.Day(), int(firstDate.Time.Month()), firstDate.Time.Year()),
		}
		oargs = append(oargs, tok...)
		oargs = append(oargs, fmt.Sprintf("payoff=%d.%d.%d",
			payoffDate.Time.Day(), int(payoffDate.Time.Month()), payoffDate.Time.Year()))
		cmd := "amort_oracle " + strings.Join(oargs, " ")

		want, ok := ask(oargs...)
		if !ok {
			oracleRefused++
			continue
		}
		got, err := PayoffBalance(in, payoffDate)
		if err != nil {
			goRefused++
			continue
		}

		// The documented in-advance (non-R78) approximation: DOS discounts against
		// the in-advance walk's internal settlement-shifted state; the port derives
		// it from the displayed schedule. Bucketed, reported, NOT failed — and NOT
		// folded into the agreeing population either.
		if inadv && !r78 {
			frontier++
			continue
		}

		// DOS's `payoff 0.0000` IS AMBIGUOUS and must not be scored either way.
		// It is the genuine answer for a payoff date past full repayment —
		//	amort_oracle 100000 0.08 36 12 loandmy=1.1.2015 firstdmy=1.2.2015 \
		//	  payoff=1.1.2030   ->  payoff 0.0000
		// — and it is ALSO what DOS emits on screens where it plainly has a balance:
		//	amort_oracle 415792.00 0.089484 36 12 loandmy=28.7.2023 \
		//	  firstdmy=28.8.2023 payoff=28.12.2023  ->  payoff 0.0000
		//	  (the port says 377686.78; the SAME screen with `b365` added gives DOS
		//	   377772.78, and with n=120 instead of 36 it gives 410111.02)
		// so a bare 0 cannot be distinguished from a bail without more work.
		//
		// Scoring it as agreement would hide a divergence; scoring it as a
		// divergence would fail the suite on an unadjudicated DOS response. It gets
		// its own counted bucket and a loud report instead — the same treatment
		// `dos_solved_go_refused` gets in fuzzer5, and for the same reason.
		if want == 0 && math.Abs(got) > cents {
			dosZero++
			if len(dosZeroRepro) < 5 {
				dosZeroRepro = append(dosZeroRepro,
					fmt.Sprintf("%s\n    Go=%.4f DOS=0.0000", cmd, got))
			}
			continue
		}

		compared++
		d := math.Abs(got - want)
		if d > cents {
			diverged++
			for _, k := range tok {
				sig[k]++
			}
			sig["all"]++
			sig[fmt.Sprintf("perYr=%02d", perYr)]++
			if perYr > 12 {
				sig["submonthly"]++
			}
			if d > worst {
				worst, worstCmd = d, fmt.Sprintf("%s\n    Go=%.4f DOS=%.4f diff=%.4f", cmd, got, want, got-want)
			}
		}
	}

	// ---- R5: the ledger must balance ----
	acct := compared + oracleRefused + goRefused + frontier + dosZero
	t.Logf("ledger: generated %d = compared %d + oracle-refused %d + go-refused %d "+
		"+ in-advance-frontier %d + dos-zero-port-nonzero %d | UNACCOUNTED %d",
		generated, compared, oracleRefused, goRefused, frontier, dosZero, generated-acct)
	if dosZero > 0 {
		t.Logf("SIG=ADVISORY:dos_zero_payoff_port_nonzero — %d of %d generated. DOS "+
			"returned exactly 0 where the port computed a balance. UNADJUDICATED: a "+
			"bare 0 is also DOS's genuine answer for a repaid loan, so these are "+
			"neither scored as agreement nor failed. Round-19 item.", dosZero, generated)
		for _, r := range dosZeroRepro {
			t.Logf("    %s", r)
		}
	}
	if generated != acct {
		t.Errorf("PAYOFF LEDGER DOES NOT BALANCE: %d generated but %d accounted. A case "+
			"that falls out silently makes the rate meaningless.", generated, acct)
	}
	if compared == 0 {
		t.Fatalf("nothing was compared — the sweep is not measuring anything")
	}

	// ---- Rule 9: the rate, over ACTUALLY COMPARED ----
	t.Logf("ACTUALLY COMPARED %d — the denominator for any payoff rate", compared)
	if diverged == 0 {
		t.Logf("payoff randomized sweep: 0 divergences in %d compared cases "+
			"(1 in >%d at this sample size)", compared, compared)
	} else {
		t.Errorf("payoff differs from DOS in %d of %d compared cases (1 in %d), worst %.4f\n  %s",
			diverged, compared, compared/diverged, worst, worstCmd)
	}

	// ---- Rule 9: base rates, so nobody can quote an enrichment without them ----
	keys := make([]string, 0, len(base))
	for k := range base {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%d ", k, base[k])
	}
	t.Logf("base rates (generated): %s", strings.TrimSpace(b.String()))
	if diverged > 0 {
		b.Reset()
		for _, k := range keys {
			fmt.Fprintf(&b, "%s=%d ", k, sig[k])
		}
		t.Logf("in divergences: %s", strings.TrimSpace(b.String()))
	}

	// ---- The envelope, stated so a future session knows what this rate covers ----
	t.Logf("ENVELOPE: plain loans (no balloons/prepayments/adjustments); "+
		"$5k-500k; 2-18%%; 2-30 years; perYr=12 ONLY (see the R2 note in the source); "+
		"origins 2015-2024; payoff dates from the loan date to ~1 year past the "+
		"last payment, a third of them mid-period. %d in-advance non-R78 cases "+
		"were bucketed to the documented frontier and are NOT in the denominator.",
		frontier)
}
