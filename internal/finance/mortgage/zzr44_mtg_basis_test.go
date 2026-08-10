package mortgage

import (
	"os"
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// ROUND 44, item 0-13b — THE MORTGAGE APR DAY-COUNT IS A DIVERGENCE ON THE
// SHIPPED DEFAULT PATH, NOT A COVERAGE GAP.
//
// Nate answered decision 3a.13 on 2026-08-09: MEASURE FIRST, DO NOT CHANGE THE
// ARITHMETIC. Item 0-13b was scoped as "give mtg_oracle a basis token so the
// surface can be measured before it is changed." The token was added
// (`legacy/oracle/mtg_oracle.pas`, `b360`/`b365`/`b365_360`, rule 7: a NEW
// token, default untouched, smoke test byte-identical). THE MEASUREMENT THEN
// INVERTED THE ITEM'S PREMISE.
//
// WHAT THE TOKEN MEASURED (round 44, /tmp/oraclebuild/mtg_oracle,
// `-dV_3 -dSCROLLS -dPVLX`):
//
//	mtg_oracle apr 250000 0.20 30 0.0725 0.015 0 0 [token]
//	  <none>    apr 0.0742490000     b360      apr 0.0742490000
//	  b365      apr 0.0742490000     b365_360  apr 0.0742490000
//
// IDENTICAL ON EVERY BASIS, on two independent cases, and deterministic across
// 10 repeats each (§74's repeat-sampling requirement — the ParseDMY
// nondeterminism lesson).
//
// R51 says an inert change is a statement about the change until you show it
// reaches. IT REACHES, AND THE INVARIANCE IS REAL — read from the Pascal
// (rule 5), not inferred from the null result:
//
//   - `SetYrDays` (INTSUTIL.pas:333, the active `{3/94}` variant) sets
//     yrdays := 365.25 ONLY for x365; EVERYTHING ELSE, INCLUDING x365_360,
//     gets 360.
//   - **`Mortgage.pas` NEVER READS `yrdays` AT ALL** — zero occurrences in the
//     mortgage unit. DOS's mortgage screen is basis-invariant BY CONSTRUCTION.
//
// `docs/frontend_qa_report.md:167-169` already asserted exactly this ("the
// Mortgage screen is always 360-day"). It is now MEASURED rather than asserted.
//
// 🚨 THE ROUND'S OWN CONFIDENT FINDING, AND HOW ITS POSITIVE CONTROL KILLED IT.
//
// The port's `internal/api/handlers.go:78-83` (`mtgAPRYrDays`) returns 365.25
// for every basis except the literal "360" — INCLUDING "", THE SHIPPED DEFAULT
// — and hands it to FullTermAPR at :668 and :767. Round 44 wrote the obvious
// conclusion: *"DOS computes every mortgage APR at 360, the port computes it at
// 365.25 by default, therefore the shipped default path diverges on every
// mortgage."* That is what audit F7, restatement §1e, START_HERE item 0-13b and
// Nate's 3a.13 framing all point at, and it is what this test was written to
// pin.
//
// **THE FIRST FORM OF THIS TEST FAILED ON ITS OWN VACUITY GUARD (R24): the port
// returns the IDENTICAL APR at 360 and at 365.25.** Reading the source rather
// than the null result (rule 5, and R51 — an inert change is a statement about
// the change until you show it reaches):
//
//	interest.YieldFromRate(rr, n, yrdays) -> nn := RealPerYr(n, yrdays)
//	RealPerYr consults yrdays for `daily`, 52 and 26 ONLY; every other n
//	returns n itself. (rates.go:42-54, and intsutil.pas:1255-1261 VERBATIM.)
//
// **THE MORTGAGE SCREEN COMPOUNDS MONTHLY. n IS 12 AT EVERY APR CALL SITE. SO
// `yrdays` IS STRUCTURALLY INERT IN THE MORTGAGE APR — IN DOS AND IN THE PORT
// EQUALLY, BY THE SAME LINE OF THE SAME FUNCTION.**
//
// ⟹ **THERE IS NO DIVERGENCE, AND ITEM 0-13b's PREMISE IS RETIRED.** The
// published mortgage zero (30,000 cases / 135,853 APR verdicts) IS a 360-only
// statement — that part of F7 is correct — but the scope restriction is
// IMMATERIAL, because the axis it excludes cannot move the answer. `mtgAPRYrDays`
// is dead code on the mortgage APR path, not a defect.
//
// ⚠️ WHAT IS **NOT** RETIRED: this argument covers the APR only. It says nothing
// about any other mortgage cell, and nothing about a screen compounded at
// daily/52/26 — which the mortgage screen does not offer, but which the
// invariance below depends on. If a frequency selector is ever added to the
// mortgage screen, THIS ENTIRE RESULT LAPSES. That is what the positive control
// in the second test exists to make visible.
//
// Filed as §87. Nate's 3a.13 answer ("do not change the arithmetic") is
// unaffected and needs no revisiting: there is now nothing to change.
func TestR44MortgageAPRIsDayCountInvariantLikeDOS(t *testing.T) {
	line := MtgLine{
		PriceStatus: types.InOutInput, Price: 250000,
		PctStatus: types.InOutInput, Pct: 0.20,
		YearsStatus: types.InOutInput, Years: 30,
		RateStatus: types.InOutInput, Rate: 0.0725,
		PointsStatus: types.InOutInput, Points: 0.015,
	}
	// Solve the screen first — FullTermAPR needs the DERIVED fields (financed,
	// monthly) that Calc produces, exactly as handlers.go:668 passes result.Line.
	line = Calc(line).Line

	var got []float64
	for _, yd := range []float64{360, 365, 365.25} {
		apr, conv, err := FullTermAPR(line, yd)
		if err != nil || !conv {
			t.Fatalf("FullTermAPR(%.2f) did not answer: conv=%v err=%v", yd, conv, err)
		}
		got = append(got, apr)
	}
	t.Logf("MORTGAGE APR vs DAY-COUNT (item 0-13b): 360 -> %.10f | 365 -> %.10f | "+
		"365.25 -> %.10f. DOS: Mortgage.pas never reads yrdays. PORT: RealPerYr "+
		"ignores yrdays at n=12. The oracle, asked through round 44's new b365/"+
		"b365_360 tokens, returns 0.0742490000 on all three. §87.",
		got[0], got[1], got[2])

	for i := 1; i < len(got); i++ {
		if got[i] != got[0] {
			t.Errorf("THE MORTGAGE APR MOVED WITH THE DAY-COUNT: %.12f vs %.12f. "+
				"DOS's mortgage screen is day-count invariant by construction "+
				"(Mortgage.pas never reads yrdays; RealPerYr ignores it at n=12). "+
				"If this fires, either the compounding frequency is no longer 12 or "+
				"RealPerYr changed — and the published mortgage zero, which was "+
				"measured entirely at 360, must be re-derived across bases before "+
				"it is quoted again (R31/R36). §87.", got[i], got[0])
		}
	}

	// 🚨 THE POSITIVE CONTROL, INSIDE THE GUARD (R24, r42's §76 pattern).
	// The invariance above is only meaningful if yrdays is capable of moving
	// this arithmetic AT ALL. If RealPerYr ever stopped consulting yrdays
	// entirely, the assertion above would pass vacuously forever and the
	// mortgage zero's scope statement would silently become unfalsifiable.
	// Assert the parameter IS load-bearing at a frequency that uses it.
	dailyAt360, err1 := interest.YieldFromRate(0.0725, types.CompoundingDaily, 360)
	dailyAt36525, err2 := interest.YieldFromRate(0.0725, types.CompoundingDaily, 365.25)
	if err1 != nil || err2 != nil {
		t.Fatalf("the positive control could not run: %v / %v", err1, err2)
	}
	if dailyAt360 == dailyAt36525 {
		t.Fatal("VACUOUS: yrdays no longer changes YieldFromRate even at DAILY " +
			"compounding, so the monthly invariance asserted above is untestable " +
			"and this whole test is measuring nothing. RealPerYr (rates.go:42) " +
			"must consult yrdays for daily/52/26 — intsutil.pas:1255-1261. R24/R16.")
	}
	t.Logf("  positive control: at DAILY compounding yrdays IS load-bearing "+
		"(%.12f vs %.12f), so the monthly invariance above is a real result and "+
		"not an artefact of a dead parameter.", dailyAt360, dailyAt36525)
}

// TestR44MortgageFuzzerHardCodes360 pins the REASON the divergence was invisible
// for twenty-nine rounds: every APR call site in the mortgage differential
// passes the literal 360, which is the one day-count at which the port and DOS
// agree. If a later round parameterises those sites, this test should be
// updated deliberately — and the mortgage zero re-derived (R36), because it
// would then be measuring a different population.
//
// ⚠️ THIS IS A SOURCE-LAYOUT PIN ON A HARNESS PROPERTY, NOT A BEHAVIOUR TEST.
// It exists so that "the mortgage zero is 360-only" cannot quietly stop being
// true without someone noticing. CAUTION 8 / R31.
func TestR44MortgageFuzzerHardCodes360(t *testing.T) {
	raw, err := os.ReadFile("dos_mtg_fuzzer5_test.go")
	if err != nil {
		t.Fatalf("cannot read the mortgage fuzzer: %v", err)
	}
	src := string(raw)
	n := strings.Count(src, "FullTermAPR(")
	// ⚠️ COUNT THE CO-OCCURRENCE, NOT THE SUBSTRING. Mutation testing killed an
	// earlier form that counted ", 360)" anywhere in the file: the fuzzer has
	// other ", 360)" arguments, so rewriting every FullTermAPR call to 365.25
	// left the count non-zero and the pin green. Same defect class as a needle
	// that is a prefix of a surviving neighbour. R38.
	n360 := 0
	for _, ln := range strings.Split(src, "\n") {
		if strings.Contains(ln, "FullTermAPR(") && strings.Contains(ln, ", 360)") {
			n360++
		}
	}
	if n == 0 {
		t.Fatal("no FullTermAPR call sites found in the mortgage fuzzer — the pin " +
			"is measuring nothing. R16.")
	}
	t.Logf("mortgage fuzzer: %d FullTermAPR call sites, %d literal-360 arguments. "+
		"The published mortgage zero (30,000 cases / 135,853 APR verdicts) is a "+
		"360-ONLY statement. §87.", n, n360)
	if n360 == 0 {
		t.Error("the mortgage fuzzer no longer passes a literal 360 to FullTermAPR. " +
			"If the APR day-count has been parameterised, the published mortgage " +
			"zero was measured over a DIFFERENT population and must be re-derived " +
			"before it is quoted again (R36/R31). Update this pin deliberately.")
	}
}
