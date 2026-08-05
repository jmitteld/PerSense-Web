package presentvalue

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// zzsamplespace_test.go — ROUND 36. THE PV GENERATOR'S SAMPLE SPACE IS AN
// ASSERTED FACT, NOT FOLKLORE.
//
// START_HERE has carried "⚠️ THEIR SAMPLE SPACE HAS NEVER BEEN AUDITED — FIVE
// ROUNDS" against dos_pv_fuzzer5_test.go while the surface table quotes
//
//	PV — forward, seeds 20611-20630 @ 1500: 29,917 worksheets, 5,095,860 lines,
//	0 divergences
//
// off it, and the ratified exit criterion asks for PV at zero. A zero is a
// statement about its generator (R31). This file is that statement.
//
// ============================================================================
// THE HEADLINE: THE PV GENERATOR'S CALENDAR STOPS IN 2088
// ============================================================================
//
// Every date hazard this project has found lives beyond that:
//
//	§69  DOS's representation ceiling      26 August 2091   3 years out of reach
//	§62  the two calendars disagree        1 January 2100  12 years out of reach
//	§71  MDY poisons a daterec on overflow  (past 2091)     unreachable
//	§72  a horizon past the loan's own last date            unreachable
//
// The ceiling is not a bound anyone chose; it is the ARITHMETIC CONSEQUENCE of
// four independent draw bounds (as-of year, the periodic FROM offset, the
// bounded-horizon cap, and table.go's 50-year forever cut) and it is computed
// here from the constants rather than asserted, so it moves when they move.
//
// ⚠️ THE HONEST READING. "0 divergences in 29,917 PV worksheets" is TRUE, and it
// is true of a generator whose latest possible payment date is twelve years
// before the first calendar boundary that has ever broken this port. It is not
// evidence that the PV surface survives §62, §69, §71 or §72 — it is evidence
// that the PV surface has never been asked. Widening the as-of year or the
// forever cut is a reviewed decision with a diff; that is what this file makes
// possible and what four rounds of quoting the zero did not.
//
// ============================================================================
// WHAT THE PV GENERATOR CANNOT PRODUCE
// ============================================================================
//
//	axis              drawn                    CANNOT produce             status
//	----------------------------------------------------------------------------
//	discount rate     0.004 .. 0.284           EXACTLY 0; >= 0.284;       SILENT
//	                                           negative
//	as-of date        2020-01-01 .. 2028-12-28 day 29/30/31 AS-OF;        SILENT
//	                                           any year outside 2020-28
//	DATES OVERALL     resolved payment dates    2091 / 2100 / anything     SILENT,
//	                  up to 1 Dec 2088;        the port's date defects    AND IT
//	                  ⚠️ the INPUT tokens go   actually live at           MATTERS
//	                  to 1.12.2149 (the
//	                  forever sentinel), so
//	                  "cannot produce 2091"
//	                  is about DATES REACHED,
//	                  not cells typed
//	basis             360 / 365 / 365-360      nothing — complete         OK
//	COLA month mode   annual / cont / 1..12    nothing — complete         OK
//	detail level      detail / both / summary  detail-only + a cumset     BY
//	                                           (UI-unreachable)           DESIGN
//	cumulative set    the 4 UI shapes          arbitrary month sets       BY
//	                                           (UI-unreachable)           DESIGN
//	lump rows         0, 1, or 2..4            5 or more lump rows        VISIBLE
//	periodic rows     1, or 2..3               4 or more periodic rows    VISIBLE
//	periodic perYr    1,2,3,4,6,12,26,52       24 — the ONE frequency     SILENT,
//	                                           §67 found non-invertible   AND IT
//	                                           in AddPeriod               MATTERS
//	row day-of-month  1..31, clamped           an impossible date         BY
//	                                                                      DESIGN
//	COLA rate         0 .. 0.85 x discount     COLA >= the discount rate  SILENT
//	                                           (a divergent stream)
//	simple interest   pinned false in the      the simple/compound axis   BY
//	                  oracle CLI               entirely                   DESIGN
//	backward solves   none — this fuzzer is    the PV AMOUNT solves       KNOWN
//	                  FORWARD only             (§3b item 11)              GAP
//
// TWO NAMED SEPARATELY, BOTH SILENT:
//
//  1. THE 2088 CEILING, above.
//
//  2. perYr = 24 IS MISSING FROM THE PV FREQUENCY LIST. The amortization side
//     found `AddPeriod` NOT INVERTIBLE at peryr=24 (§67, round 26) and shipped a
//     fix for it. The PV periodic list is {1,2,3,4,6,12,26,52} — 24 is the one
//     value between 12 and 26 that DOS's own AddPeriod treats specially, and the
//     PV surface has never drawn it.

// pvSSFirstHazardYear is DOS's representation ceiling, 26 August 2091 (§69) —
// the earliest calendar boundary that has ever broken this port.
const pvSSFirstHazardYear = 2091

// TestSampleSpacePVDateCeiling computes the latest date the PV generator can
// produce, from the generator's OWN constants, and pins it against the hazards.
//
// The four terms, each a bound in dos_pv_fuzzer5_test.go or table.go:
//
//	as-of        <= (pvFz5AsOfYearLo + pvFz5AsOfYearN - 1)-12-28
//	lump         <= as-of + (pvFz5LumpMonthLo + pvFz5LumpMonthSpan - 1) months
//	periodic FROM<= as-of + (pvFz5PerMonthLo  + pvFz5PerMonthSpan  - 1) months
//	periodic TO  <= FROM  + pvFz5PerHorizonMo months          (bounded stream)
//	             or FROM.year + pvFz5ForeverCutYr             (forever stream)
//
// The dates are built with time.Time the same way the generator's mkDate and
// addMonths helpers build them, so this is arithmetic on the generator's own
// envelope and not a second, hand-rolled calendar (R2's failure mode).
func TestSampleSpacePVDateCeiling(t *testing.T) {
	maxAsOf := time.Date(pvFz5AsOfYearLo+pvFz5AsOfYearN-1, time.December,
		pvFz5AsOfMaxDay, 0, 0, 0, 0, time.UTC)

	maxLump := maxAsOf.AddDate(0, pvFz5LumpMonthLo+pvFz5LumpMonthSpan-1, 0)
	maxFrom := maxAsOf.AddDate(0, pvFz5PerMonthLo+pvFz5PerMonthSpan-1, 0)
	maxBoundedTo := maxFrom.AddDate(0, pvFz5PerHorizonMo, 0)
	// table.go's forever cut replaces ONLY the year and KEEPS THE SENTINEL'S
	// month and day (`types.NewDateRec(from.Year()+50, to.Month(), to.Day())`
	// where `to` is types.LatestDate() = 1 December 2149). An earlier version of
	// this test used maxFrom's month/day and reported 2088-11-28; the true latest
	// generatable payment date is 2088-12-01. The YEAR — which is all the scope
	// question turns on — is the same either way. (Round-36 audit.)
	maxForeverTo := time.Date(maxFrom.Year()+pvFz5ForeverCutYr,
		types.LatestDate().Time.Month(), types.LatestDate().Time.Day(),
		0, 0, 0, 0, time.UTC)

	// THE vr_multi SURFACE HAS DATES TOO, and an earlier version of this test
	// derived the ceiling from the `table` terms alone — so raising
	// pvFz5VRMaxPerN would have moved the real envelope while this test still
	// printed 2088 and passed. vr pins as-of at 2024-01-01; its rate steps run
	// 2024 + (pvFz5VRMaxSteps-1)*pvFz5VRStepYearMax years, its lumps
	// pvFz5VRLumpMonths months, and its periodics n*(12/perYr) months with
	// n <= pvFz5VRMaxPerN at perYr >= 1. (Round-36 audit.)
	vrBase := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	maxVR := vrBase.AddDate((pvFz5VRMaxSteps-1)*pvFz5VRStepYearMax, 0, 0)
	for _, d := range []time.Time{
		vrBase.AddDate(0, pvFz5VRLumpMonths, 0),
		vrBase.AddDate(0, pvFz5VRMaxPerN*12, 0), // perYr = 1 is the slowest
	} {
		if d.After(maxVR) {
			maxVR = d
		}
	}

	ceiling := maxLump
	for _, d := range []time.Time{maxBoundedTo, maxForeverTo, maxVR} {
		if d.After(ceiling) {
			ceiling = d
		}
	}

	t.Logf("PV generator date envelope: as-of <= %s, lump <= %s, periodic from <= %s, "+
		"bounded to <= %s, forever to <= %s, vr_multi <= %s  ==>  CEILING %s",
		maxAsOf.Format("2006-01-02"), maxLump.Format("2006-01-02"),
		maxFrom.Format("2006-01-02"), maxBoundedTo.Format("2006-01-02"),
		maxForeverTo.Format("2006-01-02"), maxVR.Format("2006-01-02"),
		ceiling.Format("2006-01-02"))

	if ceiling.Year() != 2088 {
		t.Errorf("the PV generator's date ceiling is now %d, the audit says 2088 — "+
			"if this widening is deliberate, update the manifest AND say in the "+
			"same commit whether the PV zero has been re-measured over the new "+
			"envelope, because it has not been measured over this one",
			ceiling.Year())
	}
	if ceiling.Year() >= pvSSFirstHazardYear {
		t.Logf("THE ENVELOPE NOW REACHES DOS'S %d CEILING (§69). Every PV figure "+
			"quoted before this change was measured over a strictly smaller "+
			"calendar and must be re-measured, not carried.", pvSSFirstHazardYear)
	} else {
		t.Logf("gap to the first known date hazard (§69, %d): %d years; "+
			"gap to the 2100 calendar split (§62): %d years. The PV zero is a "+
			"statement about a generator that stops before both.",
			pvSSFirstHazardYear, pvSSFirstHazardYear-ceiling.Year(),
			2100-ceiling.Year())
	}
	// The sentinel must stay outside the envelope, or a "forever" row would be
	// indistinguishable from a very long bounded one.
	if !types.LatestDate().Time.After(ceiling) {
		t.Errorf("the forever sentinel %s is no longer beyond the generator's "+
			"ceiling %s — a bounded row could now be mistaken for a perpetual one",
			types.LatestDate().Time.Format("2006-01-02"), ceiling.Format("2006-01-02"))
	}
}

// TestSampleSpacePVManifest pins the scalar envelope, the amortization
// manifest's job applied to this package.
func TestSampleSpacePVManifest(t *testing.T) {
	type bound struct {
		name      string
		got, want float64
	}
	for _, b := range []bound{
		{"omit probability", pvFz5OmitProb, 0.15},
		{"rate lo", pvFz5RateLo, 0.004},
		{"rate hi (excl)", pvFz5RateLo + pvFz5RateSpan, 0.284},
		{"as-of year lo", pvFz5AsOfYearLo, 2020},
		{"as-of year hi", pvFz5AsOfYearLo + pvFz5AsOfYearN - 1, 2028},
		{"as-of max day", pvFz5AsOfMaxDay, 28},
		{"lump month lo", pvFz5LumpMonthLo, -48},
		{"lump month hi", pvFz5LumpMonthLo + pvFz5LumpMonthSpan - 1, 239},
		{"max lump rows", pvFz5MaxLumpRows, 4},
		{"periodic month lo", pvFz5PerMonthLo, -24},
		{"periodic month hi", pvFz5PerMonthLo + pvFz5PerMonthSpan - 1, 119},
		{"max periodic rows", pvFz5MaxPerRows, 3},
		{"bounded horizon cap (months)", pvFz5PerHorizonMo, 600},
		{"forever cut (years)", pvFz5ForeverCutYr, 50},
		{"vr max rate steps", pvFz5VRMaxSteps, 5},
		{"vr max lump months", pvFz5VRLumpMonths, 300},
		{"vr max periodic n", pvFz5VRMaxPerN, 60},
	} {
		if b.got != b.want {
			t.Errorf("envelope changed: %s = %v, manifest says %v — if this is "+
				"deliberate, update the manifest and the audit doc in the same commit",
				b.name, b.got, b.want)
		}
	}
}

// TestSampleSpacePVFrequencyGap records the ONE missing payment frequency.
//
// The periodic rows draw perYr from {1,2,3,4,6,12,26,52}. 24 is absent — and 24
// is precisely the frequency at which the amortization side found DOS's
// `AddPeriod` NOT INVERTIBLE (§67, round 26, a shipped fix). The PV table walks
// its periodic streams with the same period arithmetic and has never been asked
// the question at the one value that broke it elsewhere.
//
// The list lives as a literal inside the draw (a []int index expression), so
// this test states the gap rather than binding to a constant; if 24 is added,
// delete this test in the same commit.
func TestSampleSpacePVFrequencyGap(t *testing.T) {
	drawn := map[int]bool{1: true, 2: true, 3: true, 4: true, 6: true,
		12: true, 26: true, 52: true}
	for _, py := range []int{24} {
		if drawn[py] {
			t.Errorf("perYr=%d is now drawn by the PV fuzzer — good, but this test "+
				"is stale: delete it and note in the audit that the §67 frequency "+
				"is covered on the PV surface too", py)
		}
	}
	if len(drawn) != 8 {
		t.Errorf("the PV frequency list has %d entries, the manifest says 8", len(drawn))
	}
	t.Log("PV periodic frequencies: 1,2,3,4,6,12,26,52 — perYr=24 is NOT drawn, " +
		"and 24 is the value §67 found AddPeriod non-invertible at")
}
