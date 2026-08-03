package amortization

// zzsec59_ceiling_test.go — regression gate for discrepancies §59 (round 19).
//
// WHAT BROKE. generateFancyScheduleMode's "past the last regular payment date"
// block (the A2 block) ends the regular grid and then re-enters the off-cycle
// drain loop so every trailing balloon/prepayment is emitted at its own date.
// It did that by setting
//
//	currentDate = AddDays(lastPending, 1)
//
// and AddDays is Julian() + n followed by MDY() — so it inherits DOS's MDY
// range guard, `if (daynumber < 0) or (daynumber > 70000) then ... exit`
// (VIDEODAT.pas:373). Day 70000 is 26 August 2091. When the last pending extra
// fell past that date the +1 FAILED, the block dropped through to its `break`,
// and the walk stopped at the last regular payment date with every trailing
// balloon undrained. TackOnFinalBalloon then read the terminating balloon off a
// schedule that had never reached very_last.
//
// That `break` carried the comment "coverage: excluded — defensive: jump not
// representable". It is reachable on any schedule whose very_last passes 26
// August 2091, which is precisely §59's measured signature: all 27 flips sat 69
// to 121 years past origination on loans dated 2024-2026, i.e. 2093 and later.
// The "69 years" floor in that data is not a property of the generator; it is
// the Julian ceiling.
//
// WHY DOS DOES NOT HAVE IT. DOS never forms this date. The "+1 day" is a
// PORT-ONLY construct for re-entering the drain loop; DOS's ComputeNext
// (AMORTOP.pas:602-613) tests each extra's own date against h^.lastdate and
// never round-trips a date through MDY here. The port applied a faithful DOS
// guard at a site DOS does not have — which is the general shape worth
// remembering, not the specific arithmetic.
//
// CORRECTION TO THE ROUND-18c WRITE-UP. Round 18c root-caused §59 as "the
// probe runs through the PIECEWISE walk whose horizon is loan.LastDate, so it
// stops at the solved last PAYMENT date" and scoped the fix as separating the
// payment-grid horizon from the walk's terminal date — a structural change to
// every fancy schedule. That is not what was wrong. The A2 block already
// computed the correct terminal date (lastPending = 10/12/2126 on the repro,
// confirmed by trace); it simply could not express "one day past it". No
// horizon needed separating, and loan.LastDate was never read for this. The
// round-18c note that the naive `LastDate := veryLast` fix measures wrong
// (-232,046,575 against DOS's -321,878) stands and is unrelated: that fix is
// wrong because it manufactures 80 phantom regular rows, which is a different
// mistake from the one that was actually happening.
//
// THE LADDER. Three rungs, same screen, varying only which balloons are
// present, so very_last moves across the ceiling and nothing else changes:
//
//	very_last 10/12/2126   PAST the ceiling    — was broken, now DOS-exact
//	very_last 10/12/2059   BEFORE the ceiling  — CONTROL, unchanged by the fix
//	very_last 10/12/2046   BEFORE the ceiling  — CONTROL, unchanged by the fix
//
// Measured pre-fix vs post-fix with cmd/goamort built from both trees
// (2026-08-03):
//
//	rung      PRE           POST          DOS
//	2126        6974.06     -321878.17    -321878.17   FIXED
//	2059      -28709.82      -28709.82     -28709.82   unchanged
//	2046        6912.93       6912.93       6912.93    unchanged
//
// The two controls are the load-bearing half of this file: they are what shows
// the change is confined to the region past the ceiling. Standing rule 3 —
// every fix ships a regression test verified BOTH directions, and where a
// component's revert does NOT change the outcome, say so.

import (
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// sec59Base is the §59 repro from docs/discrepancies.md, minus its balloons.
// Provenance: fuzzer5, round 18, `noterm` arm. DOS solves the term to 22
// periods ending 10/12/2046 on every rung, which is what makes the rungs
// comparable — only very_last moves.
const sec59Base = "49726.63 0.1048540000 249 1 b365_360 exact prepaid inadv plusreg r78 usa " +
	"loandmy=12.10.2024 firstdmy=12.10.2025"

const sec59Tail = "pre=1536:376:6:61.13 targ=135.78 payhard=5994.80 noterm"

func TestSec59TerminatingBalloonPastJulianCeiling(t *testing.T) {
	cases := []struct {
		name     string
		balloons string
		// wantDate/wantAmount are DOS's, captured from amort_oracle `bdump`.
		// Provenance for each is the full command printed on failure.
		wantDate   string
		wantAmount float64
		// pastCeiling records whether very_last is beyond DOS's MDY day-70000
		// limit — i.e. whether this rung exercises the defect or controls it.
		pastCeiling bool
		// preFix is what the port produced BEFORE the round-19 fix, measured.
		// Kept so a future reader can see the size of the move, and so that a
		// regression back to the old behaviour is recognisable by value.
		preFix float64
	}{
		{
			name:        "very_last 2126, past the ceiling — the defect",
			balloons:    "b264=13027.49 b420=10892.59 b1224=5915.90",
			wantDate:    "10/12/2126",
			wantAmount:  -321878.17,
			pastCeiling: true,
			preFix:      6974.06,
		},
		{
			name:        "very_last 2059, before the ceiling — CONTROL",
			balloons:    "b264=13027.49 b420=10892.59",
			wantDate:    "10/12/2059",
			wantAmount:  -28709.82,
			pastCeiling: false,
			preFix:      -28709.82,
		},
		{
			name:        "very_last 2046, before the ceiling — CONTROL",
			balloons:    "b264=13027.49",
			wantDate:    "10/12/2046",
			wantAmount:  6912.93,
			pastCeiling: false,
			preFix:      6912.93,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			line := sec59Base + " " + tc.balloons + " " + sec59Tail
			in, _ := m5Parse(t, line)

			res := Amortize(in)
			if res.Err != nil {
				t.Fatalf("Amortize: %v\n  repro: amort_oracle %s bdump", res.Err, line)
			}

			var tack ResolvedBalloon
			found := false
			for _, b := range res.Balloons {
				if b.TackedOn {
					tack, found = b, true
				}
			}
			if !found {
				t.Fatalf("no terminating balloon produced\n  repro: amort_oracle %s bdump", line)
			}

			gotDate := fmt.Sprintf("%d/%d/%d", int(tack.Date.Time.Month()),
				tack.Date.Time.Day(), tack.Date.Time.Year())
			if gotDate != tc.wantDate {
				t.Errorf("tack DATE = %s, DOS says %s\n  repro: amort_oracle %s bdump",
					gotDate, tc.wantDate, line)
			}
			// Cent-level: these are displayed currency cells and DOS prints them
			// rounded to 4dp from a Round2'd value. 0.005 is half a cent.
			if math.Abs(tack.Amount-tc.wantAmount) > 0.005 {
				t.Errorf("tack AMOUNT = %.4f, DOS says %.4f (delta %.4f)\n"+
					"  pre-round-19 the port produced %.4f here\n"+
					"  repro: amort_oracle %s bdump",
					tack.Amount, tc.wantAmount, tack.Amount-tc.wantAmount, tc.preFix, line)
			}

			// The mechanism itself, asserted rather than described. If someone
			// raises or removes the MDY ceiling, this is the line that tells them
			// this test's ladder no longer straddles anything.
			_, err := dateutil.AddDays(tack.Date, 1)
			if tc.pastCeiling && err == nil {
				t.Errorf("this rung is supposed to sit PAST DOS's MDY day-70000 ceiling, "+
					"but AddDays(%s, 1) now succeeds — the ceiling moved, and this "+
					"ladder no longer tests what it claims to", gotDate)
			}
			if !tc.pastCeiling && err != nil {
				t.Errorf("this rung is supposed to be a CONTROL below the ceiling, "+
					"but AddDays(%s, 1) fails: %v", gotDate, err)
			}
		})
	}
}

// TestSec59AgainstLiveOracle re-derives the goldens above from the real DOS
// engine whenever it is present, so they cannot rot silently. Standing rule 6:
// goldens carry provenance, and the cheapest provenance is the oracle itself.
func TestSec59AgainstLiveOracle(t *testing.T) {
	gateOracle(t)

	rungs := []string{
		"b264=13027.49 b420=10892.59 b1224=5915.90",
		"b264=13027.49 b420=10892.59",
		"b264=13027.49",
	}
	for _, b := range rungs {
		line := sec59Base + " " + b + " " + sec59Tail
		t.Run(b, func(t *testing.T) {
			dosDate, dosAmt, ok := sec59OracleTack(t, line)
			if !ok {
				t.Fatalf("oracle produced no solved balloon row\n  repro: amort_oracle %s bdump", line)
			}
			in, _ := m5Parse(t, line)
			res := Amortize(in)
			if res.Err != nil {
				t.Fatalf("Amortize: %v", res.Err)
			}
			var tack ResolvedBalloon
			found := false
			for _, rb := range res.Balloons {
				if rb.TackedOn {
					tack, found = rb, true
				}
			}
			if !found {
				t.Fatalf("port produced no terminating balloon\n  repro: amort_oracle %s bdump", line)
			}
			gotDate := fmt.Sprintf("%d/%d/%d", int(tack.Date.Time.Month()),
				tack.Date.Time.Day(), tack.Date.Time.Year())
			if gotDate != dosDate || math.Abs(tack.Amount-dosAmt) > 0.005 {
				t.Errorf("port %s %.4f vs DOS %s %.4f\n  repro: amort_oracle %s bdump",
					gotDate, tack.Amount, dosDate, dosAmt, line)
			}
		})
	}
}

// sec59OracleTack runs amort_oracle with bdump and returns the LAST balloon row
// whose amount is an OUTPUT cell (astatus 1) — the terminating balloon. Keying
// on the amount status rather than requiring BOTH statuses to be outp is
// deliberate: the merge path leaves the DATE at the user's inp status, and
// requiring both is exactly the bug that made round 18 see only the append
// path (see the `tack` helper in dos_fuzzer5_test.go).
func sec59OracleTack(t *testing.T, line string) (string, float64, bool) {
	t.Helper()
	out, err := exec.Command(oracleBin, append(strings.Fields(line), "bdump")...).Output()
	if err != nil {
		t.Fatalf("oracle: %v", err)
	}
	date, amt, ok := "", 0.0, false
	for _, ln := range strings.Split(string(out), "\n") {
		f := strings.Fields(ln)
		//  0          1 2    3          4       5 6      7          8       9
		// balloonrow N date M/D/YYYY dstatus S amount A          astatus S
		if len(f) < 10 || f[0] != "balloonrow" {
			continue
		}
		if f[9] != "1" { // astatus outp — a solved cell
			continue
		}
		v, perr := strconv.ParseFloat(f[7], 64)
		if perr != nil {
			t.Fatalf("parsing %q: %v", ln, perr)
		}
		date, amt, ok = f[3], v, true
	}
	return date, amt, ok
}

// TestSec59DrainAllEquivalence pins the claim the fix rests on: the new
// "currentDate = lastPending, drain inclusively" form is equivalent to the old
// "currentDate = lastPending + 1, drain strictly" form everywhere AddDays
// succeeded. Both reduce to "admit every extra dated on or before lastPending,
// and only enter the block when lastPending >= currentDate".
//
// This is an internal-consistency test and by standing rule 10 it never drives
// a behaviour change; it exists so that a future edit to either predicate has
// to confront the equivalence deliberately.
func TestSec59DrainAllEquivalence(t *testing.T) {
	mk := func(y, m, d int) types.DateRec {
		return types.NewDateRec(y, time.Month(m), d)
	}
	for _, tc := range []struct{ ly, lm, ld, cy, cm, cd int }{
		{2046, 10, 12, 2046, 10, 12},
		{2059, 10, 12, 2047, 10, 12},
		{2091, 8, 26, 2047, 10, 12},
		{2091, 8, 27, 2047, 10, 12}, // first date past the ceiling
		{2126, 10, 12, 2047, 10, 12},
		{2040, 1, 1, 2047, 10, 12}, // lastPending BEFORE currentDate: block must not fire
	} {
		lastPending := mk(tc.ly, tc.lm, tc.ld)
		currentDate := mk(tc.cy, tc.cm, tc.cd)

		newFires := dateutil.DateComp(lastPending, currentDate) >= 0
		oldFires := false
		if jump, err := dateutil.AddDays(lastPending, 1); err == nil {
			oldFires = dateutil.DateComp(jump, currentDate) > 0
		}
		pastCeiling := false
		if _, err := dateutil.AddDays(lastPending, 1); err != nil {
			pastCeiling = true
		}
		if pastCeiling {
			// This is the whole defect: the old form declined to fire.
			if oldFires {
				t.Errorf("lastPending %v: expected the OLD form to fail past the ceiling", lastPending.Time)
			}
			if !newFires {
				t.Errorf("lastPending %v: the NEW form must still fire past the ceiling", lastPending.Time)
			}
			continue
		}
		if oldFires != newFires {
			t.Errorf("lastPending %v vs currentDate %v: old form fires=%v, new form fires=%v — "+
				"the two are supposed to be equivalent below the ceiling",
				lastPending.Time, currentDate.Time, oldFires, newFires)
		}
	}
}
