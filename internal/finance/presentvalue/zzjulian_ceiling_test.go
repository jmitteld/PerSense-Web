package presentvalue

// Regression guards for DOS's Julian-day ceiling on the weekly/biweekly payment
// grid (VIDEODAT.pas:373 — `if (daynumber<0) or (daynumber>70000) then begin
// x.m:=errorbyte; exit; end`).
//
// DOS is ASYMMETRIC here, and both halves are reproduced:
//
//   - a FOREVER row (To = the 1/12/2149 sentinel) is TRUNCATED at Julian 70000,
//     because NumberOfInstallments short-circuits on the sentinel year before it
//     can fail (INTSUTIL.pas:1026) and the per-payment walk instead runs until
//     AddPeriod's MDY poisons the cursor, at which point DateComp reports it past
//     the terminal and the loop exits KEEPING its sum;
//   - a row with a REAL To date past the ceiling is REFUSED, because
//     NumberOfInstallments' 26/52 arm calls MDY directly (INTSUTIL.pas:960),
//     hands the poisoned daterec back through its VAR parameter, and the screen
//     dies on the next Julian() with "Bad date passed to Julian function: m=-99".
//
// Before the fix the port allowed Julian day numbers up to 100000, so a
// perpetual biweekly stream ran to 2149 (+1.9% on the first case below) and a
// past-the-ceiling To date was quietly valued instead of refused.
//
// Every expected value here was produced by the REAL DOS engine; the oracle
// command is in each case's `oracle` field and, when the oracle binary is
// present, the test re-runs it and asserts the golden still matches — so these
// numbers cannot rot into "whatever Go printed".

import (
	"errors"
	"math"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// foreverTo is DOS's perpetual sentinel: any To date in the `latest` year
// (2149). NumberOfInstallments returns maxint for it without snapping.
func foreverTo() (int, time.Month, int) { return 2149, time.December, 1 }

// runPVOracleScreen runs `pv_oracle table ...` and reports either the screen
// total or that DOS refused. A refusal reaches us three ways depending on how
// far the engine gets before the poisoned daterec bites — the harness's `ERR`
// line, the EMessage flood on stderr, or a Free Pascal runtime abort when that
// flood unwinds the stack — and all three mean the same thing on a real screen:
// the row is refused and no value is produced.
func runPVOracleScreen(t *testing.T, args []string) (val float64, refused bool, ran bool) {
	t.Helper()
	bin := pvOracleBin()
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("PERSENSE_REQUIRE_ORACLE set but oracle missing at %s", bin)
		}
		return 0, false, false
	}
	cmd := exec.Command(bin, append([]string{"table"}, args...)...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	both := stdout.String() + "\n" + stderr.String()
	if strings.Contains(both, "Bad date passed to Julian function") ||
		strings.Contains(stdout.String(), "ERR") || runErr != nil {
		return 0, true, true
	}
	v, ok := parsePV([]byte(stdout.String()))
	if !ok {
		t.Fatalf("pv_oracle table %v: unparsable output %q", args, stdout.String())
	}
	return v, false, true
}

// TestPVJulianCeilingTruncatesForever pins the truncated value of a perpetual
// weekly/biweekly stream, and pins the monthly control that must NOT move (the
// monthly grid steps month fields, never Julian, so it never reaches MDY).
func TestPVJulianCeilingTruncatesForever(t *testing.T) {
	fy, fm, fd := foreverTo()
	asof := types.NewDateRec(2028, time.January, 1)

	cases := []struct {
		name   string
		input  PVInput
		want   float64
		oracle []string
	}{
		{
			// pv_oracle table 0.10 360 detail none ann asof=1.1.2028 \
			//   per=1.6.2035:1.12.2149:26:1000.00:0.03   ->  pv 170805.731733
			// Pre-fix the port ran the walk to 2149 and returned 174057.625977.
			"forever_biweekly_cola_360",
			tblInput(0.10, types.Basis360, types.COLAAnnual, asof, nil,
				[]PeriodicPayment{tblPer(2035, time.June, 1, fy, fm, fd, 26, 1000.00, 0.03)}),
			170805.731733,
			[]string{"0.10", "360", "detail", "none", "ann", "asof=1.1.2028",
				"per=1.6.2035:1.12.2149:26:1000.00:0.03"},
		},
		{
			// pv_oracle table 0.10 360 detail none ann asof=1.1.2028 \
			//   per=1.6.2035:1.12.2090:26:1000.00:0   ->  pv 122248.284724
			// The control: a biweekly To date INSIDE the ceiling is unaffected.
			"finite_biweekly_under_ceiling_360",
			tblInput(0.10, types.Basis360, types.COLAAnnual, asof, nil,
				[]PeriodicPayment{tblPer(2035, time.June, 1, 2090, time.December, 1, 26, 1000.00, 0)}),
			122248.284724,
			[]string{"0.10", "360", "detail", "none", "ann", "asof=1.1.2028",
				"per=1.6.2035:1.12.2090:26:1000.00:0"},
		},
		{
			// pv_oracle table 0.10 360 detail none ann asof=1.1.2028 \
			//   per=1.6.2035:1.12.2149:12:1000.00:0.03   ->  pv 80278.265609
			// Confinement check: the monthly grid must be untouched by the
			// ceiling, because AddPeriod's 1..12 arm steps the month field and
			// never calls MDY (INTSUTIL.pas:1208 vs :1213).
			"forever_monthly_cola_360_unaffected",
			tblInput(0.10, types.Basis360, types.COLAAnnual, asof, nil,
				[]PeriodicPayment{tblPer(2035, time.June, 1, fy, fm, fd, 12, 1000.00, 0.03)}),
			80278.265609,
			[]string{"0.10", "360", "detail", "none", "ann", "asof=1.1.2028",
				"per=1.6.2035:1.12.2149:12:1000.00:0.03"},
		},
		{
			// pv_oracle table 0.10 365360 detail none 11 asof=1.1.2028 \
			//   per=1.6.2035:1.12.2149:26:1000.00:0.03   ->  pv 168651.578079
			// Month-specific COLA on the 365/360 basis: the arm that had the
			// port +16.2% high before the fix. periodicSumAnnualCOLA routes
			// peryr 26/52 to its exact per-payment loop, so the truncation must
			// land on THAT loop — the whole-years closed form (period II) is
			// unreachable at these frequencies and correctly has no clamp.
			"forever_biweekly_colamonth11_365360",
			tblInput(0.10, types.Basis365360, 11, asof, nil,
				[]PeriodicPayment{tblPer(2035, time.June, 1, fy, fm, fd, 26, 1000.00, 0.03)}),
			168651.578079,
			[]string{"0.10", "365360", "detail", "none", "11", "asof=1.1.2028",
				"per=1.6.2035:1.12.2149:26:1000.00:0.03"},
		},
		{
			// pv_oracle table 0.05 365360 detail none 11 asof=1.1.2045 \
			//   per=1.1.2026:1.12.2149:52:1000.00:0.03   ->  pv 4806557.229011
			// Weekly, as-of well AFTER the from date (the since_from=false
			// anchoring), high absolute value — the region the 2026-07-30
			// rounds measured their -6.4% gap in.
			"forever_weekly_colamonth11_365360_asof_after_from",
			tblInput(0.05, types.Basis365360, 11, types.NewDateRec(2045, time.January, 1), nil,
				[]PeriodicPayment{tblPer(2026, time.January, 1, fy, fm, fd, 52, 1000.00, 0.03)}),
			4806557.229011,
			[]string{"0.05", "365360", "detail", "none", "11", "asof=1.1.2045",
				"per=1.1.2026:1.12.2149:52:1000.00:0.03"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Calculate(tc.input)
			if res.Err != nil {
				t.Fatalf("Calculate: %v", res.Err)
			}
			if math.Abs(res.SumValue-tc.want) > 5e-6 {
				t.Errorf("SumValue = %.6f, want %.6f (DOS)", res.SumValue, tc.want)
			}
			// Provenance is self-checking when the oracle is available.
			if got, refused, ran := runPVOracleScreen(t, tc.oracle); ran {
				if refused {
					t.Fatalf("oracle refused a case the golden says it values")
				}
				if math.Abs(got-tc.want) > 5e-6 {
					t.Errorf("golden %.6f no longer matches the oracle %.6f", tc.want, got)
				}
			}
		})
	}
}

// TestPVJulianCeilingRefusesRealToDate covers the other half of the asymmetry:
// a real To date past the ceiling must be REFUSED, not valued. Pre-fix the port
// returned 122290.413102 for the first case — a plausible-looking number that a
// reviewer had no reason to question, which is precisely why this needs a guard.
func TestPVJulianCeilingRefusesRealToDate(t *testing.T) {
	asof := types.NewDateRec(2028, time.January, 1)
	// Julian 70000 is 26 Aug 2091: the last day DOS's MDY will return.
	lastOK := dateutil.LastRepresentableDate()
	if got := dateutil.Julian(lastOK); got != 70000 {
		t.Fatalf("LastRepresentableDate is Julian %d, want 70000", got)
	}

	cases := []struct {
		name        string
		to          types.DateRec
		wantRefusal bool
		oracle      []string
	}{
		{
			// pv_oracle table 0.10 360 detail none ann asof=1.1.2028 \
			//   per=1.6.2035:1.12.2091:26:1000.00:0
			//   -> ERR Bad date passed to Julian function: m=-99
			"to_past_ceiling_refuses",
			types.NewDateRec(2091, time.December, 1), true,
			[]string{"0.10", "360", "detail", "none", "ann", "asof=1.1.2028",
				"per=1.6.2035:1.12.2091:26:1000.00:0"},
		},
		// The adjacent pair below is the sharp edge, and it is NOT at Julian
		// 70000 itself. NumberOfInstallments snaps the To date BACKWARD onto
		// the biweekly grid before MDY sees it, so what matters is the last
		// grid payment on or before the To date. For a 1/6/2035 anchor the
		// grid straddles the ceiling: the payment on 24 Aug 2091 is the last
		// representable one, so every To date up to 6 Sep 2091 snaps back onto
		// it and computes, and 7 Sep 2091 is the first To date whose snap lands
		// on the next (unrepresentable) grid payment. Both engines agree day by
		// day across the whole of Aug-Sep 2091; a guard keyed on the ENTERED
		// date rather than the snapped one would refuse eleven days too early.
		{
			// pv_oracle table 0.10 360 detail none ann asof=1.1.2028 \
			//   per=1.6.2035:6.9.2091:26:1000.00:0   ->  pv 122279.486551
			"to_last_snappable_computes",
			types.NewDateRec(2091, time.September, 6), false,
			[]string{"0.10", "360", "detail", "none", "ann", "asof=1.1.2028",
				"per=1.6.2035:6.9.2091:26:1000.00:0"},
		},
		{
			// pv_oracle table 0.10 360 detail none ann asof=1.1.2028 \
			//   per=1.6.2035:7.9.2091:26:1000.00:0
			//   -> ERR Bad date passed to Julian function: m=-99
			"to_first_unsnappable_refuses",
			types.NewDateRec(2091, time.September, 7), true,
			[]string{"0.10", "360", "detail", "none", "ann", "asof=1.1.2028",
				"per=1.6.2035:7.9.2091:26:1000.00:0"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := tblInput(0.10, types.Basis360, types.COLAAnnual, asof, nil,
				[]PeriodicPayment{tblPer(2035, time.June, 1,
					tc.to.Time.Year(), tc.to.Time.Month(), tc.to.Time.Day(), 26, 1000.00, 0)})
			res := Calculate(in)
			if tc.wantRefusal {
				if res.Err == nil {
					t.Fatalf("expected a refusal, got SumValue = %.6f", res.SumValue)
				}
				// The message has to name the boundary — "bad date" tells the
				// user nothing they can act on.
				if !strings.Contains(res.Err.Error(), "August 26, 2091") {
					t.Errorf("refusal does not name the boundary date: %v", res.Err)
				}
			} else if res.Err != nil {
				t.Fatalf("unexpected refusal: %v", res.Err)
			}
			if _, refused, ran := runPVOracleScreen(t, tc.oracle); ran {
				if refused != tc.wantRefusal {
					t.Errorf("DOS refused = %v, port refused = %v", refused, tc.wantRefusal)
				}
			}
		})
	}
}

// TestPVJulianCeilingForeverIsNotRefused nails down the asymmetry itself: the
// SAME frequency and from-date that gets refused with a real 2091 To date must
// be VALUED when the row runs forever, because DOS's NumberOfInstallments never
// reaches MDY on the sentinel year. Getting this backwards would refuse every
// perpetual biweekly row on the screen.
func TestPVJulianCeilingForeverIsNotRefused(t *testing.T) {
	fy, fm, fd := foreverTo()
	in := tblInput(0.10, types.Basis360, types.COLAAnnual,
		types.NewDateRec(2028, time.January, 1), nil,
		[]PeriodicPayment{tblPer(2035, time.June, 1, fy, fm, fd, 26, 1000.00, 0.03)})
	res := Calculate(in)
	if res.Err != nil {
		t.Fatalf("a forever biweekly row must be valued (truncated), not refused: %v", res.Err)
	}
	if res.SumValue <= 0 {
		t.Fatalf("SumValue = %.6f, want the truncated perpetuity value", res.SumValue)
	}
}

// TestPVJulianCeilingTableWalkKeepsPayments guards the table walk specifically.
// nextTablePayment's stop-and-keep must retire ONLY the stream that hit the
// ceiling, keep the payment it just emitted, and leave the other streams
// running; an abort there would drop the perpetual row's payments entirely and
// silently shorten the table.
func TestPVJulianCeilingTableWalkKeepsPayments(t *testing.T) {
	fy, fm, fd := foreverTo()
	in := tblInput(0.10, types.Basis360, types.COLAAnnual,
		types.NewDateRec(2028, time.January, 1), nil,
		[]PeriodicPayment{
			tblPer(2035, time.June, 1, fy, fm, fd, 26, 1000.00, 0.03),
			tblPer(2030, time.January, 1, 2050, time.January, 1, 12, 250.00, 0),
		})
	res := MakeTable(in, TableRequest{Detail: TableDetailOnly})
	if res.Err != nil {
		t.Fatalf("MakeTable: %v", res.Err)
	}
	// The biweekly row alone contributes >1000 payments inside the table's own
	// 50-year forever cutoff; a walk that aborted would produce a handful.
	if res.PaymentN < 1000 {
		t.Errorf("table produced %d payment lines; the biweekly stream was cut short", res.PaymentN)
	}
	if res.GrandValue <= 0 {
		t.Errorf("GrandValue = %.6f", res.GrandValue)
	}
}

// TestJulianCeilingErrorPlumbing checks the propagation contract the PV refusal
// depends on, at the dateutil boundary rather than through the engine.
func TestJulianCeilingErrorPlumbing(t *testing.T) {
	if _, err := dateutil.MDY(70000); err != nil {
		t.Fatalf("MDY(70000) must succeed: %v", err)
	}
	_, err := dateutil.MDY(70001)
	if !errors.Is(err, dateutil.ErrJulianCeiling) {
		t.Fatalf("MDY(70001) error = %v, want ErrJulianCeiling", err)
	}
	if _, err := dateutil.MDY(-1); !errors.Is(err, dateutil.ErrJulianCeiling) {
		t.Fatalf("MDY(-1) error = %v, want ErrJulianCeiling (DOS uses one arm for both)", err)
	}

	from := types.NewDateRec(2035, time.June, 1)
	past := types.NewDateRec(2091, time.December, 1) // Julian 70097

	// The 26/52 arm propagates...
	_, _, _, _, err = dateutil.NumberOfInstallmentsRawE(from, past, 26, types.OnOrBefore)
	if !errors.Is(err, dateutil.ErrJulianCeiling) {
		t.Fatalf("NumberOfInstallmentsRawE(26) error = %v, want ErrJulianCeiling", err)
	}
	// ...the monthly arm never touches MDY, so it must NOT.
	if _, _, _, _, err := dateutil.NumberOfInstallmentsRawE(from, past, 12, types.OnOrBefore); err != nil {
		t.Fatalf("NumberOfInstallmentsRawE(12) must not fail: %v", err)
	}
	// A forever terminal short-circuits before ChoosePaymentDate, so no error
	// even at peryr 26 — this is what makes the truncate/refuse split work.
	forever := types.NewDateRec(2149, time.December, 1)
	if _, _, _, _, err := dateutil.NumberOfInstallmentsRawE(from, forever, 26, types.OnOrBefore); err != nil {
		t.Fatalf("a forever terminal must not error: %v", err)
	}

	// The non-E wrapper still swallows, for the many callers that only need a
	// date to COMPARE (an unknown date sorts last under DateComp, exactly as
	// DOS's m=-99 record does).
	_, snapped := dateutil.NumberOfInstallments(from, past, 26, types.OnOrBefore)
	if dateutil.DateOK(snapped) {
		t.Errorf("the swallowing wrapper should hand back the unusable date, got %v", snapped)
	}
	if dateutil.DateComp(snapped, past) <= 0 {
		t.Errorf("an unusable date must order after every real date (DOS DateComp)")
	}
}
