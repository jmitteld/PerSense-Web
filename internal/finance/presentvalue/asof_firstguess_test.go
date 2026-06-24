package presentvalue

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestSolveAsOfFirstGuessRegression guards the off-by-100 fix to the As-of-Date
// solver's first guess in backward.go (now NewDateRec(2000,1,1); previously
// NewDateRec(1900,1,1)).
//
// The legacy daterec year byte holds (calendar year - 1900), so the DOS source's
// As-of search seed `asof.y := 100` (PRESVALU.pas:761) is the year 2000 — NOT
// 1900. The original port mistranslated that to 1900, which made the solver's
// first Newton step ~(answer - 1900) years. dateutil.AddYears (faithfully porting
// INTSUTIL.pas:894, `if abs(yrs) > 128 then TimeTooLong`) rejects any step over
// 128 years, so any As-of answer at or after ~2029 aborted with "time period too
// long" — even though DOS, starting from 2000, solves it in one small step.
//
// Each case below errored before the fix; the 2024 case (which squeaked under the
// 128-year cap) worked before and must keep working, proving the fix changes no
// converged result.
func TestSolveAsOfFirstGuessRegression(t *testing.T) {
	// rate 5% continuous true rate; a $10,000 lump discounted to the solved
	// As-of date must equal the target, i.e. As-of = lumpYear - ln(10000/target)/0.05.
	cases := []struct {
		name     string
		lumpYear int
		target   float64
		wantYear int // expected solved As-of year (Jan 1)
	}{
		{"answer_2029_regressed_before_fix", 2034, 7788.01, 2029}, // 5 yrs: 10000*e^-0.25
		{"answer_2024_worked_before_fix", 2026, 9048.37, 2024},    // 2 yrs: 10000*e^-0.10
		{"answer_2040_regressed_before_fix", 2060, 3678.79, 2040}, // 20 yrs: 10000*e^-1.00
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input := PVInput{
				LumpSums: []LumpSumPayment{{
					DateStatus: types.InOutInput,
					Date:       newDate(c.lumpYear, time.January, 1),
					AmtStatus:  types.InOutInput,
					Amt:        10000,
				}},
				PresVal: PresValLine{
					// As-of Date left blank (its status stays empty) so the
					// engine solves for it; Rate and target Sum Value supplied.
					R:              RateEntry{Status: types.StatusFromRate, Rate: 0.05},
					SumValueStatus: types.InOutInput,
					SumValue:       c.target,
				},
				Settings: defaultSettings(),
			}

			result := Calculate(input)
			if result.Err != nil {
				t.Fatalf("As-of solve returned an error (off-by-100 regression — the "+
					"first guess must be year 2000, not 1900): %v", result.Err)
			}
			if result.AsOf.Time.IsZero() {
				t.Fatal("As-of date was not solved (got the zero/unknown date)")
			}

			want := newDate(c.wantYear, time.January, 1)
			gotDays := result.AsOf.Time.Sub(want.Time).Hours() / 24
			if gotDays < -2 || gotDays > 2 {
				t.Errorf("solved As-of = %s, want ~%s (within 2 days)",
					result.AsOf.Time.Format("2006-01-02"),
					want.Time.Format("2006-01-02"))
			}
		})
	}
}
