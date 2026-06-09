package presentvalue

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// These tests cover the guard that rejects a two-life contingency when
// no usable second life is configured — previously a silent degeneration
// (person 2 treated as immortal). See checkSecondLifeProvided.

func twoLifeLumpInput(cfg *actuarial.ActuarialConfig, act byte) PVInput {
	return PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: dateOf(2026, time.January, 1),
			R: RateEntry{Status: types.InOutInput, Rate: 0.06},
		},
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: dateOf(2040, time.January, 1),
			AmtStatus: types.InOutInput, Amt: 100000,
			Act: act,
		}},
		Actuarial: cfg,
	}
}

// TestTwoLifeGuardRejectsMissingSecondTable: every two-life contingency
// is refused with an actionable, row-named error when only one life
// table is supplied.
func TestTwoLifeGuardRejectsMissingSecondTable(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	for _, act := range []byte{actuarial.Only1Living, actuarial.Only2Living, actuarial.EitherLiving, actuarial.BothLiving} {
		cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1)) // Table1 only
		res := Calculate(twoLifeLumpInput(cfg, act))
		if res.Err == nil {
			t.Errorf("%s without a second table: expected an error, got SumValue %.2f",
				actuarial.ContingencyLabel(act), res.SumValue)
			continue
		}
		if !strings.Contains(res.Err.Error(), "second life table") {
			t.Errorf("%s: error should mention the second life table, got: %v",
				actuarial.ContingencyLabel(act), res.Err)
		}
	}
}

// TestTwoLifeGuardAllowsWithSecondTable: with a real second life table
// and DOB the same contingencies compute normally.
func TestTwoLifeGuardAllowsWithSecondTable(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	for _, act := range []byte{actuarial.Only1Living, actuarial.Only2Living, actuarial.EitherLiving, actuarial.BothLiving} {
		cfg := twoLifeCfg(asOf, dateOf(1956, time.January, 1), dateOf(1961, time.January, 1))
		res := Calculate(twoLifeLumpInput(cfg, act))
		if res.Err != nil {
			t.Errorf("%s with a second table: unexpected error: %v",
				actuarial.ContingencyLabel(act), res.Err)
		}
	}
}

// TestTwoLifeGuardSingleLifeUnaffected: single-life contingencies (and
// non-contingent rows) are never blocked by the guard.
func TestTwoLifeGuardSingleLifeUnaffected(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	for _, act := range []byte{actuarial.NotContingent, actuarial.Living, actuarial.Dead} {
		cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1))
		res := Calculate(twoLifeLumpInput(cfg, act))
		if res.Err != nil {
			t.Errorf("%s with one table: unexpected error: %v",
				actuarial.ContingencyLabel(act), res.Err)
		}
	}
}

// TestTwoLifeGuardMissingDOB2: a second table with no valid second date
// of birth is also rejected (the age projection would be garbage).
func TestTwoLifeGuardMissingDOB2(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := twoLifeCfg(asOf, dateOf(1956, time.January, 1), dateOf(1961, time.January, 1))
	cfg.DOB2 = types.DateRec{} // blank second DOB
	res := Calculate(twoLifeLumpInput(cfg, actuarial.BothLiving))
	if res.Err == nil {
		t.Errorf("BothLiving with a blank second DOB: expected an error, got SumValue %.2f", res.SumValue)
	}
}

// TestTwoLifeGuardVRPath: the guard fires on the variable-rate path too.
func TestTwoLifeGuardVRPath(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1)) // one table
	res := Calculate(PVInput{
		Settings: vrTestSettings(),
		PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf},
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: dateOf(2040, time.January, 1),
			AmtStatus: types.InOutInput, Amt: 100000,
			Act: actuarial.EitherLiving,
		}},
		RateSchedule: vrAuditSchedule(),
		Actuarial:    cfg,
	})
	if res.Err == nil {
		t.Errorf("VR two-life without a second table: expected an error, got SumValue %.2f", res.SumValue)
	}
}

// TestTwoLifeGuardNamesPeriodicRow: a two-life periodic row produces an
// error that names the periodic row (1-based).
func TestTwoLifeGuardNamesPeriodicRow(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1))
	res := Calculate(PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R: RateEntry{Status: types.InOutInput, Rate: 0.06},
		},
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: dateOf(2030, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: dateOf(2040, time.January, 1),
			PerYrStatus: types.InOutInput, PerYr: 12,
			AmtStatus: types.InOutInput, Amt: 1000,
			Act: actuarial.BothLiving,
		}},
		Actuarial: cfg,
	})
	if res.Err == nil {
		t.Fatalf("expected an error for a two-life periodic row without a second table")
	}
	if !strings.Contains(res.Err.Error(), "periodic payment line 1") {
		t.Errorf("error should name the periodic row, got: %v", res.Err)
	}
}
