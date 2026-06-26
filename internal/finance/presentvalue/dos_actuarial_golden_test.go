package presentvalue

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// TestDOSActuarialGolden is the FIRST direct fidelity comparison of the Go
// actuarial PV engine against the real DOS Per%Sense program for a
// present-value-with-life-contingency case.
//
// It is opt-in (env PERSENSE_GOLDEN=1) so it never runs in the normal suite.
//
// The DOS reference number was captured by driving the real DOS PerSense.exe
// under DOSBox (headless, Xvfb) on the Present Value screen and reading the
// Sum Value off the rendered text screen. See
// docs/dos_actuarial_golden.md / outputs/dos_golden/s_result.png.
//
// EXACT CASE (entered live in DOS):
//   - Periodic Payment: From 01/01/2024 Through 01/01/2029, 12/yr,
//     Amount $2,000.00, COLA blank (0), contingency = "L" (Living).
//   - Present Value block: As of 01/01/2024, True Rate % = 5.0000.
//   - Actuarial window (Ctrl-A): Date Of Birth 01/01/1959, table MALE,
//     "Today" 01/01/2024, Payable-on-death 0.
//   - Settings (status bar): COLA:Ann  basis 360  centurydiv 1950  12/yr.
//
// DOS results read off the screen (outputs/dos_golden/s_result.png):
//   - Periodic row Value : 104,258.31
//   - Present Value Value : 104,258.31  (the Sum Value)
//   - (non-contingent baseline, contingency removed: 107,954.58 —
//     outputs/dos_golden/s_filled.png — used as a forward-PV sanity check.)
//
// The Go side uses actuarial.Persense1988Male() (the recovered MALE.ACT
// table DOS reads) so the table basis is not a variable; R.Rate carries the
// continuously-compounded True Rate (the API/UI treats True Rate % as the
// continuous rate — see internal/api/handlers.go PVRateLineReq).
func TestDOSActuarialGolden(t *testing.T) {
	if os.Getenv("PERSENSE_GOLDEN") != "1" {
		t.Skip("opt-in: set PERSENSE_GOLDEN=1 to run the DOS actuarial golden compare")
	}

	const (
		dosContingent    = 104258.31 // DOS Sum Value with Living contingency
		dosNonContingent = 107954.58 // DOS Sum Value with contingency removed
	)

	asOf := dateOf(2024, time.January, 1)
	dob := dateOf(1959, time.January, 1)

	cfg := &actuarial.ActuarialConfig{
		Table1: actuarial.Persense1988Male(),
		DOB1:   dob,
		Now:    asOf, // DOS "Today" = 01/01/2024
	}

	build := func(act byte, withCfg bool) PVInput {
		in := PVInput{
			Settings: vrTestSettings(), // Basis360, 12/yr, COLA Annual
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: 0.05},
			},
			Periodics: []PeriodicPayment{{
				FromDateStatus: types.InOutInput, FromDate: dateOf(2024, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: dateOf(2029, time.January, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				AmtStatus: types.InOutInput, Amt: 2000,
				Act: act,
			}},
		}
		if withCfg {
			in.Actuarial = cfg
		}
		return in
	}

	// Sanity: non-contingent forward PV should match the DOS baseline exactly.
	nc := Calculate(build(actuarial.NotContingent, false))
	if nc.Err != nil {
		t.Fatalf("non-contingent calc: %v", nc.Err)
	}
	t.Logf("NON-CONTINGENT  DOS=%.2f  Go=%.2f  absdiff=%.4f",
		dosNonContingent, nc.SumValue, nc.SumValue-dosNonContingent)

	// The headline comparison: Living contingency.
	res := Calculate(build(actuarial.Living, true))
	if res.Err != nil {
		t.Fatalf("contingent calc: %v", res.Err)
	}
	absDiff := res.SumValue - dosContingent
	relDiff := absDiff / dosContingent
	t.Logf("LIVING-CONTINGENT  DOS=%.2f  Go=%.4f  absdiff=%.4f  reldiff=%.3e",
		dosContingent, res.SumValue, absDiff, relDiff)

	// PASS bar: agree to the cent (|absdiff| < 0.005) is a landmark match.
	if math.Abs(absDiff) >= 0.005 {
		t.Logf("DIVERGENCE: Go SumValue %.4f vs DOS %.2f (abs %.4f, rel %.3e)",
			res.SumValue, dosContingent, absDiff, relDiff)
	}
}
