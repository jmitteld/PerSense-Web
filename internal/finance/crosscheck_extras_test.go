// crosscheck_extras_test.go — DOS-output cross-checks for the extended
// refdata.json sections (Rule of 78, in-advance annuity, biweekly basis
// coercion).
//
// These cases are emitted by legacy/testharness/refdata.pas in the
// EmitR78Tests / EmitInAdvanceTests / EmitBiweeklyTests procedures.
// Run scripts/regen_refdata.sh --apply locally (requires fpc) to
// produce the matching refdata.json blocks. Until that script has been
// run with the new harness, these tests are skipped via t.Skip below
// rather than failed, so a stale refdata.json doesn't break CI.

package finance

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/persense/persense-port/internal/finance/amortization"
	"github.com/persense/persense-port/internal/types"
)

type r78Case struct {
	Label         string  `json:"label"`
	Amount        float64 `json:"amount"`
	Payment       float64 `json:"payment"`
	NPeriods      int     `json:"nPeriods"`
	TotalInterest float64 `json:"totalInterest"`
	R78Step       float64 `json:"r78Step"`
	IntPmt1       float64 `json:"int_pmt_1"`
	IntPmt2       float64 `json:"int_pmt_2"`
	IntPmt180     float64 `json:"int_pmt_180,omitempty"`
	IntPmt360     float64 `json:"int_pmt_360,omitempty"`
	IntPmt12      float64 `json:"int_pmt_12,omitempty"`
	IntPmt24      float64 `json:"int_pmt_24,omitempty"`
}

type inAdvanceCase struct {
	Label    string  `json:"label"`
	Amount   float64 `json:"amount"`
	Rate     float64 `json:"rate"`
	PerYr    int     `json:"perYr"`
	NPeriods int     `json:"nPeriods"`
	TrueRate float64 `json:"trueRate"`
	F        float64 `json:"f"`
	FF       float64 `json:"ff"`
	Payment  float64 `json:"payment"`
}

type biweeklyCase struct {
	Label        string  `json:"label"`
	Rate         float64 `json:"rate"`
	PerYr        int     `json:"perYr"`
	CoercedBasis string  `json:"coercedBasis"`
	TrueRate     float64 `json:"trueRate"`
}

type amortScheduleCase struct {
	Amount   float64 `json:"amount"`
	Rate     float64 `json:"rate"`
	NPeriods int     `json:"nPeriods"`
	PerYr    int     `json:"perYr"`
	Payment  float64 `json:"payment"`
	MidK     int     `json:"midK"`
	Int1     float64 `json:"int_1"`
	Bal1     float64 `json:"bal_1"`
	Int2     float64 `json:"int_2"`
	Bal2     float64 `json:"bal_2"`
	IntMid   float64 `json:"int_mid"`
	BalMid   float64 `json:"bal_mid"`
	IntLast  float64 `json:"int_last"`
	BalLast  float64 `json:"bal_last"`
}

type extendedRefData struct {
	Rule78              []r78Case           `json:"rule78"`
	InAdvance           []inAdvanceCase     `json:"in_advance"`
	BiweeklyBasisCoerce []biweeklyCase      `json:"biweekly_basis_coercion"`
	AmortSchedule       []amortScheduleCase `json:"amort_schedule"`
}

// amortFirstDate returns the first-payment date one period after
// 2024-01-01, so the loan's first regular period is full (prorate == 1)
// and matches the harness's vanilla schedule assumption.
func amortFirstDate(perYr int) types.DateRec {
	switch perYr {
	case 12:
		return types.NewDateRec(2024, 2, 1)
	case 4:
		return types.NewDateRec(2024, 4, 1)
	case 2:
		return types.NewDateRec(2024, 7, 1)
	default:
		return types.NewDateRec(2025, 1, 1)
	}
}

func loadExtendedRefData(t *testing.T) *extendedRefData {
	t.Helper()
	data, err := os.ReadFile("../../legacy/reference-output/refdata.json")
	if err != nil {
		t.Fatalf("read refdata: %v", err)
	}
	var rd extendedRefData
	if err := json.Unmarshal(data, &rd); err != nil {
		t.Fatalf("parse refdata: %v", err)
	}
	return &rd
}

// TestCrossCheckRule78 checks the Go port's Rule-of-78 interest split
// against the closed-form reference values emitted by refdata.pas.
//
// The reference computes interest_k = T * (n+1-k) / (n(n+1)/2) where
// T = n*payment - amount. The Go engine uses the equivalent
// step-decrement form (r78step seeded at r78int_initial). Both should
// agree to within rounding.
func TestCrossCheckRule78(t *testing.T) {
	rd := loadExtendedRefData(t)
	if len(rd.Rule78) == 0 {
		t.Skip("rule78 fixtures not present in refdata.json — regenerate with scripts/regen_refdata.sh --apply")
	}
	for _, c := range rd.Rule78 {
		t.Run(c.Label, func(t *testing.T) {
			// Drive the REAL amortization engine in Rule-of-78 mode and
			// read the per-period interest off the generated schedule —
			// this validates engine.go's R78 allocation against the
			// independent Pascal, not a re-derivation of the same formula.
			input := amortization.LoanInput{
				Loan: amortization.Loan{
					AmountStatus: types.InOutInput, Amount: c.Amount,
					PayAmtStatus: types.InOutInput, PayAmt: c.Payment,
					NStatus: types.InOutInput, NPeriods: c.NPeriods,
					PerYrStatus: types.InOutInput, PerYr: 12,
					LoanRateStatus: types.InOutInput, LoanRate: 0.06,
					LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
					FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, 2, 1),
				},
				Settings: amortization.Settings{
					Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, R78: true,
				},
			}
			res := amortization.Amortize(input)
			if res.Err != nil {
				t.Fatalf("Amortize(R78): %v", res.Err)
			}
			if len(res.Schedule) < c.NPeriods {
				t.Fatalf("schedule has %d rows, want >= %d", len(res.Schedule), c.NPeriods)
			}
			intAt := func(k int) float64 { return res.Schedule[k-1].Interest }

			// The engine rounds per-row interest to cents when the payment
			// is user-supplied (hard payment); the harness emits unrounded
			// values, so compare within a cent.
			const tol = 0.01
			check := func(k int, want float64) {
				if want == 0 {
					return
				}
				if d := math.Abs(intAt(k) - want); d > tol {
					t.Errorf("period %d interest: engine=%.6f harness=%.6f (diff %.4f)", k, intAt(k), want, d)
				}
			}
			check(1, c.IntPmt1)
			check(2, c.IntPmt2)
			check(180, c.IntPmt180)
			check(12, c.IntPmt12)
			check(c.NPeriods, c.IntPmt24)
		})
	}
}

// TestCrossCheckInAdvance checks that the Go port produces the same
// in-advance (annuity-due) payment as the closed form in refdata.pas
// for representative cases.
func TestCrossCheckInAdvance(t *testing.T) {
	rd := loadExtendedRefData(t)
	if len(rd.InAdvance) == 0 {
		t.Skip("in_advance fixtures not present in refdata.json — regenerate with scripts/regen_refdata.sh --apply")
	}
	for _, c := range rd.InAdvance {
		t.Run(c.Label, func(t *testing.T) {
			// Drive the REAL engine: solve the in-advance (annuity-due)
			// payment with PayAmt blank and InAdvance set, and compare it
			// to the independent Pascal closed form.
			input := amortization.LoanInput{
				Loan: amortization.Loan{
					AmountStatus: types.InOutInput, Amount: c.Amount,
					LoanRateStatus: types.InOutInput, LoanRate: c.Rate,
					NStatus: types.InOutInput, NPeriods: c.NPeriods,
					PerYrStatus: types.InOutInput, PerYr: c.PerYr,
					LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
					FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, 2, 1),
					PayAmtStatus: types.StatusEmpty,
				},
				Settings: amortization.Settings{
					Basis: types.Basis360, PerYr: byte(c.PerYr), YrDays: 360, YrInv: 1.0 / 360,
					InAdvance: true,
				},
			}
			got, err := amortization.SolvePaymentClosedForm(input)
			if err != nil {
				t.Fatalf("SolvePaymentClosedForm(in-advance): %v", err)
			}
			if d := math.Abs(got - c.Payment); d > 0.01 {
				t.Errorf("in-advance payment: engine=%.4f harness=%.4f (diff %.4f)", got, c.Payment, d)
			}
		})
	}
}

// TestCrossCheckAmortSchedule drives the REAL amortization engine on a
// vanilla loan and cross-checks the per-period interest and remaining
// balance (head, period 2, midpoint, tail) plus the solved payment
// against the independent FreePascal schedule (section "amort_schedule").
// Skips until refdata.json is regenerated.
func TestCrossCheckAmortSchedule(t *testing.T) {
	rd := loadExtendedRefData(t)
	if len(rd.AmortSchedule) == 0 {
		t.Skip("amort_schedule fixtures not present in refdata.json — regenerate with scripts/regen_refdata.sh --apply")
	}
	for _, c := range rd.AmortSchedule {
		c := c
		t.Run("", func(t *testing.T) {
			in := amortization.LoanInput{
				Loan: amortization.Loan{
					AmountStatus: types.InOutInput, Amount: c.Amount,
					LoanRateStatus: types.InOutInput, LoanRate: c.Rate,
					NStatus: types.InOutInput, NPeriods: c.NPeriods,
					PerYrStatus: types.InOutInput, PerYr: c.PerYr,
					LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
					FirstStatus: types.InOutInput, FirstDate: amortFirstDate(c.PerYr),
					PayAmtStatus: types.StatusEmpty, // solved → no penny rounding
				},
				Settings: amortization.Settings{
					Basis: types.Basis360, PerYr: byte(c.PerYr), YrDays: 360, YrInv: 1.0 / 360,
				},
			}
			res := amortization.Amortize(in)
			if res.Err != nil {
				t.Fatalf("Amortize: %v", res.Err)
			}
			if len(res.Schedule) != c.NPeriods {
				t.Fatalf("schedule has %d rows, want %d", len(res.Schedule), c.NPeriods)
			}
			// Solved payment (row 1's PayAmt is the regular payment).
			assertClose(t, "amort.payment", res.Schedule[0].PayAmt, c.Payment, 1e-7)

			check := func(k int, wantInt, wantBal float64) {
				assertClose(t, "amort.int", res.Schedule[k-1].Interest, wantInt, 1e-7)
				assertClose(t, "amort.bal", res.Schedule[k-1].Principal, wantBal, 1e-7)
			}
			check(1, c.Int1, c.Bal1)
			check(2, c.Int2, c.Bal2)
			check(c.MidK, c.IntMid, c.BalMid)
			check(c.NPeriods, c.IntLast, c.BalLast)
		})
	}
}

// TestCrossCheckBiweeklyBasisCoercion checks that the truerate derived
// from a biweekly / weekly loan matches the refdata reference. The
// engine separately enforces the basis coercion (peryr 26/52 + Basis360
// → Basis365); this test only validates the rate-conversion math.
func TestCrossCheckBiweeklyBasisCoercion(t *testing.T) {
	rd := loadExtendedRefData(t)
	if len(rd.BiweeklyBasisCoerce) == 0 {
		t.Skip("biweekly fixtures not present in refdata.json — regenerate with scripts/regen_refdata.sh --apply")
	}
	for _, c := range rd.BiweeklyBasisCoerce {
		t.Run(c.Label, func(t *testing.T) {
			truerate := float64(c.PerYr) * math.Log(1+c.Rate/float64(c.PerYr))
			if math.Abs(truerate-c.TrueRate) > 1e-10 {
				t.Errorf("truerate: got %.15g, want %.15g", truerate, c.TrueRate)
			}
		})
	}
}
