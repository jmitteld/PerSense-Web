package presentvalue

// TestDOSPVVRPGenSweep — differential coverage for the `vrp_gen` oracle mode,
// which had NONE.
//
// The PV oracle grew `vrp_gen` specifically to unpin the two things `vrp` holds
// fixed (pv_oracle.pas:545-551):
//
//	{ Variable-rate periodic with an arbitrary from-month/day and basis (unlike
//	  SetupVRPeriodic which pins fromdate to 2024-01-01 on x360). Lets the VR
//	  per-payment path be exercised on a leap-day anchor / non-360 basis, to
//	  differentially validate Go vrPeriodicValue for stepped COLA (audit D1 note). }
//
// but no Go test ever called it, so the variable-rate periodic path was only
// ever differentially checked with the stream anchored on 1 January of a leap
// year and the day count pinned to 30/360 — the one alignment where the rate
// steps (always 1 January) land exactly on payment dates and the 30/360 month
// arithmetic hides every day-count question. That is precisely the alignment a
// port gets right by accident.
//
// This sweep fuzzes the anchor month and day (including 29/30/31 and 29
// February, which on a 2024 anchor is a real date and on the roll-forward years
// is not), the day-count basis (30/360, actual/365.25, actual/360), the payment
// frequency, the term, the COLA, and the number and spacing of rate steps —
// and diffs the screen's sum value against the real DOS engine.
//
// Honors PERSENSE_REQUIRE_ORACLE, PERSENSE_FUZZ_N and PERSENSE_FUZZ_SEED.

import (
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// vrpGenCase is one fuzzed variable-rate periodic worksheet, in the exact
// parameter shape SetupVRPeriodicGen takes.
type vrpGenCase struct {
	amt       float64
	perYr     int
	n         int
	cola      float64
	fromMonth int
	fromDay   int
	basis     int // 0 = 365, 1 = 360, 2 = 365/360 (pv_oracle ApplyBasis)
	steps     []rateStep
}

func runVRPGenOracle(c vrpGenCase) (float64, bool) {
	args := []string{"vrp_gen",
		strconv.FormatFloat(c.amt, 'f', 2, 64),
		strconv.Itoa(c.perYr), strconv.Itoa(c.n),
		strconv.FormatFloat(c.cola, 'f', 10, 64),
		strconv.Itoa(len(c.steps)),
		strconv.Itoa(c.fromMonth), strconv.Itoa(c.fromDay), strconv.Itoa(c.basis)}
	for _, s := range c.steps {
		args = append(args, strconv.Itoa(s.year), strconv.FormatFloat(s.rate, 'f', 10, 64))
	}
	out, err := exec.Command(pvOracleBin(), args...).Output()
	if err != nil {
		return 0, false
	}
	return parsePV(out)
}

// goVRPGen mirrors SetupVRPeriodicGen field for field.
func goVRPGen(c vrpGenCase) PVResult {
	mPer := 12 / c.perYr
	totMonths := c.n * mPer
	endM := (c.fromMonth - 1) + totMonths

	// DOS writes todate.d := pFromDay and todate.m/.y from endM with NO
	// normalization, so an anchor day of 31 in a 30-day month is stored as a
	// literal 31 and it is the engine's own date arithmetic that has to cope.
	// types.NewDateRec normalizes, which would silently repair a case DOS does
	// not repair — build the record raw.
	from := mkRawDate(2024, c.fromMonth, c.fromDay)
	to := mkRawDate(2024+endM/12, endM%12+1, c.fromDay)

	cs := int8(types.StatusEmpty)
	colaVal := 0.0
	if c.cola != 0 {
		cs = types.InOutInput
		// DOS's setup stores the CONTINUOUS cola (`cola := Ln(1 + pCola)`), but
		// the Go PeriodicPayment.COLA field holds the NOMINAL rate and takes the
		// log itself — the same convention goVRPeriodic uses against the `vrp`
		// mode. Passing Ln(1+c) here would log it twice.
		colaVal = c.cola
	}

	sched := make([]RateLine, len(c.steps))
	for i, s := range c.steps {
		sched[i] = RateLine{Date: types.NewDateRec(s.year, time.January, 1), Rate: s.rate}
	}

	s := PVSettings{Basis: types.Basis360, PerYr: byte(c.perYr), COLAMonth: types.COLAAnnual}
	switch c.basis {
	case 0:
		s.Basis = types.Basis365
	case 2:
		s.Basis = types.Basis365360
	}
	// DOS SetYrDays (INTSUTIL.pas:333): 365.25 for x365, 360 otherwise —
	// including x365_360, which counts ACTUAL days over a 360 denominator.
	if s.Basis == types.Basis365 {
		s.YrDays, s.YrInv = 365.25, 1/365.25
	} else {
		s.YrDays, s.YrInv = 360, 1.0/360
	}

	return Calculate(PVInput{
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: from,
			ToDateStatus: types.InOutInput, ToDate: to,
			PerYrStatus: types.InOutInput, PerYr: c.perYr,
			AmtStatus: types.InOutInput, Amt: c.amt,
			COLAStatus: cs, COLA: colaVal,
			ValStatus: types.StatusEmpty,
		}},
		PresVal:      PresValLine{AsOfStatus: types.InOutInput, AsOf: types.NewDateRec(2024, time.January, 1)},
		RateSchedule: sched,
		Settings:     s,
	})
}

// lastDayOfMonth clamps d to the last day that exists in (y, m).
func lastDayOfMonth(y, m, d int) int {
	last := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, -1).Day()
	if d > last {
		return last
	}
	return d
}

// mkRawDate builds a DateRec on a day that exists in the target month. The
// anchor day is clamped rather than rolled forward: types.NewDateRec is
// time.Time-backed and would turn 2/30 into 3/1, silently moving the stream to
// a different month, whereas DOS's daterec holds d/m/y as raw bytes and its
// AddPeriod restores the original day-of-month whenever the month is long
// enough. Clamping preserves the month, which is the property the walk depends
// on. The generator above only draws days that exist in the ANCHOR month; this
// matters for the derived to-date, where 2/29 + N years can land in a non-leap
// year.
func mkRawDate(y, m, d int) types.DateRec {
	return types.NewDateRec(y, time.Month(m), lastDayOfMonth(y, m, d))
}

func TestDOSPVVRPGenSweep(t *testing.T) {
	if _, err := os.Stat(pvOracleBin()); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("PERSENSE_REQUIRE_ORACLE set but pv_oracle missing: %v", err)
		}
		t.Skip("pv_oracle not built")
	}

	n := 400
	if v, err := strconv.Atoi(os.Getenv("PERSENSE_FUZZ_N")); err == nil && v > 0 {
		n = v
	}
	seed := int64(1)
	if v, err := strconv.ParseInt(os.Getenv("PERSENSE_FUZZ_SEED"), 10, 64); err == nil {
		seed = v
	}
	rng := rand.New(rand.NewSource(seed))

	checked, fails := 0, 0
	maxAbs := 0.0
	for i := 0; i < n; i++ {
		var c vrpGenCase
		c.amt = float64(100 + rng.Intn(50000))
		c.perYr = []int{1, 2, 3, 4, 6, 12}[rng.Intn(6)]
		c.n = 1 + rng.Intn(20*c.perYr)
		// COLA absent 15% of the time, per the fuzzer5 contract.
		if rng.Float64() >= 0.15 {
			c.cola = math.Round((rng.Float64()*0.12)*1e6) / 1e6
		}
		// The whole point of the mode: an arbitrary anchor. Bias hard toward the
		// month-end and leap-day anchors, which is where the port can drift.
		c.fromMonth = 1 + rng.Intn(12)
		switch rng.Intn(3) {
		case 0:
			c.fromDay = 1 + rng.Intn(28)
		case 1:
			c.fromDay = 28 + rng.Intn(4) // 28..31
		default:
			c.fromMonth, c.fromDay = 2, 29 // leap-day anchor in 2024
		}
		// Clamp to a day that actually exists in the anchor month. DOS's
		// SetupVRPeriodicGen writes `fromdate.d := pFromDay` into a raw daterec
		// with no validation, so it will happily carry 9/31 or 2/30 — but the
		// product's own date fields never produce one, and Go's time.Time-backed
		// DateRec cannot represent it. Drawing such a day would compare two
		// different worksheets and report a divergence that no user can reach.
		// 2/29/2024 is a REAL date and is deliberately still in the draw — it is
		// the corner this mode exists to exercise.
		c.fromDay = lastDayOfMonth(2024, c.fromMonth, c.fromDay)
		c.basis = rng.Intn(3)
		nr := 1 + rng.Intn(4)
		yr := 2024
		for k := 0; k < nr; k++ {
			c.steps = append(c.steps, rateStep{
				year: yr,
				rate: math.Round((0.004+rng.Float64()*0.26)*1e6) / 1e6,
			})
			yr += 1 + rng.Intn(4)
		}

		want, ok := runVRPGenOracle(c)
		if !ok {
			continue // DOS refused this worksheet; nothing to compare.
		}
		got := goVRPGen(c)
		if got.Err != nil {
			fails++
			if fails <= 10 {
				t.Errorf("vrp_gen case %d: DOS solved pv=%.6f but Go refused: %v\n  %+v",
					i, want, got.Err, c)
			}
			continue
		}
		checked++
		d := math.Abs(got.SumValue - want)
		if d > maxAbs {
			maxAbs = d
		}
		// Half a cent on the screen total, the same hardness the other PV
		// differentials use for a fully-specified forward worksheet.
		if d > 0.005 {
			fails++
			if fails <= 10 {
				t.Errorf("vrp_gen case %d: DOS pv=%.6f Go pv=%.6f (d=%.6f)\n"+
					"  amt=%.2f peryr=%d n=%d cola=%.6f from=%d/%d basis=%d steps=%v",
					i, want, got.SumValue, got.SumValue-want,
					c.amt, c.perYr, c.n, c.cola, c.fromMonth, c.fromDay, c.basis, c.steps)
			}
		}
	}
	if checked == 0 {
		t.Fatalf("anti-vacuity: vrp_gen produced ZERO comparisons over %d draws", n)
	}
	t.Logf("vrp_gen: compared %d of %d draws, divergences %d, max |d| = %.6f",
		checked, n, fails, maxAbs)
}
