package amortization

import (
	"math"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// TestMorTargetMoratoriumPrecedence guards the fix for a target combined with a
// moratorium. DOS gives the moratorium PRECEDENCE: in ComputeNext the
// interest-only branch comes before the target branch (AMORTOP.pas:641-648, an
// else-if), so a target does NOT force principal during the interest-only
// window — the balance holds until FirstRepay.
//
// Go applied the target unconditionally, so during the moratorium it paid down
// ~TargetValue per period, lowering the balance before amortization began and
// under-reporting interest. For mor=74 + targ=61 on $261k/240 the balance fell
// to ~256,547 by m73 (DOS holds 261,000), the post-moratorium payment dropped to
// 2258.53 (DOS 2297.73), and total interest came out ~$2,885 light.
//
// Golden (real DOS): amort_oracle 261000 0.0592 240 12 mor=74 targ=61
//
//	→ balance holds at 261,000 through m73, payment 2297.73 from m74, total
//	  interest 216,716.17 (identical to the same loan with NO target, since the
//	  $61 target never binds against ~$1,010 natural principal).
//
// The target must still apply OUTSIDE the moratorium (TestAdvancedTarget... and
// TestTargetAndMoratoriumAndSkip guard that), and still override skip months
// (TestHelpAM_TargetOverridesSkipInteraction) — only the interest-only window is
// exempt.
func TestMorTargetMoratoriumPrecedence(t *testing.T) {
	mk := func(withTarget bool) LoanInput {
		in := LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: 261000,
				LoanRateStatus: types.InOutInput, LoanRate: 0.0592,
				NStatus: types.InOutInput, NPeriods: 240,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
				FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, 2, 1),
			},
			Settings:   Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
			Moratorium: Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: dateMonthsAfterLoan(74)},
			Fancy:      true,
		}
		if withTarget {
			in.Target = Target{TargetStatus: types.InOutInput, TargetValue: 61}
		}
		return in
	}

	rTarget := Amortize(mk(true))
	rPlain := Amortize(mk(false))
	if rTarget.Err != nil || rPlain.Err != nil {
		t.Fatalf("err: %v / %v", rTarget.Err, rPlain.Err)
	}

	const dosInterest = 216716.17
	if d := math.Abs(rTarget.TotalInt - dosInterest); d > 5 {
		t.Errorf("total interest = %.2f, want DOS %.2f (±5). ~$213,832 means the target paid "+
			"principal during the interest-only moratorium.", rTarget.TotalInt, dosInterest)
	}
	// An inactive target must not change the schedule at all vs plain moratorium.
	if d := math.Abs(rTarget.TotalInt - rPlain.TotalInt); d > 0.01 {
		t.Errorf("mor+target interest %.2f != plain mor interest %.2f — an inactive target "+
			"changed the schedule (moratorium precedence broken).", rTarget.TotalInt, rPlain.TotalInt)
	}
	// Balance must hold through the moratorium (m73 is the last interest-only row).
	for _, row := range rTarget.Schedule {
		if row.PayNum == 73 {
			if math.Abs(row.Principal-261000) > 0.01 {
				t.Errorf("balance at m73 = %.2f, want 261000 (interest-only). The target paid "+
					"down principal during the moratorium.", row.Principal)
			}
		}
	}
}
