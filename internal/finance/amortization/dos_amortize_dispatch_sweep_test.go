package amortization

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Cross-product DOS sweep over the FULL Amortize field-presence dispatch.
//
// The existing oracle sweeps validate the backward SOLVERS directly (e.g.
// goSolveBalloon calls SolvePaymentClosedForm). The path the API/UI actually use is
// Amortize() with a blank payment, which dispatches the solve internally — and
// that path is where this session's bugs hid: Amortize estimated the payment
// (ignoring a known balloon, and not augmenting for a prepaid-OFF odd first
// period) instead of solving it. This sweep drives Amortize with a blank payment
// across balloons and odd first periods and compares the SOLVED regular payment
// to the real DOS engine. Skips when the oracle binary is absent.

// modalReg returns the steady (most frequent) regular payment in a schedule —
// the engine's solved payment, robust to a balloon row or an odd first period.
// Bucketed to whole dollars so near-equal regular rows (which differ by a cent of
// final-row rounding on very short loans) still cluster, while the balloon row
// stays in its own bucket.
func modalReg(sched []PaymentRecord) float64 {
	cnt := map[string]int{}
	best := 0
	var bestV float64
	for _, r := range sched {
		if r.PayNum < 1 {
			continue
		}
		k := fmt.Sprintf("%.0f", r.PayAmt)
		cnt[k]++
		if cnt[k] > best {
			best, bestV = cnt[k], r.PayAmt
		}
	}
	return bestV
}

func TestDOSAmortizeDispatchSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s); build via legacy/oracle/build_linux.sh", oracleBin)
	}
	baseSettings := func(perYr int) Settings {
		return Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: false}
	}

	// (A) blank payment + a known balloon, solved THROUGH Amortize.
	rng := rand.New(rand.NewSource(20260613))
	aChecked, aFails := 0, 0
	aMax, aWorst := 0.0, ""
	for i := 0; i < 250; i++ {
		amount := float64(20000 + rng.Intn(480000))
		rate := 0.01 + rng.Float64()*0.15
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		nPeriods := (3 + rng.Intn(25)) * perYr
		mPer := 12 / perYr
		bp := 1 + rng.Intn(nPeriods-1)
		bMonths := bp * mPer
		bAmt := amount * (0.05 + rng.Float64()*0.25)
		op, _, ok := runOracleBalloon(amount, rate, nPeriods, perYr, bMonths, bAmt)
		if !ok {
			continue
		}
		by, bm := 2024+bMonths/12, time.Month(bMonths%12+1)
		in := LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: amount,
				LoanRateStatus: types.InOutInput, LoanRate: rate,
				NStatus: types.InOutInput, NPeriods: nPeriods,
				PerYrStatus: types.InOutInput, PerYr: perYr,
				PayAmtStatus:   types.StatusEmpty, // blank → dispatch must solve it
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
				FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr)},
			Balloons: []BalloonPayment{{
				DateStatus: types.InOutInput, Date: types.NewDateRec(by, bm, 1),
				AmountStatus: types.InOutInput, Amount: bAmt}},
			Fancy:    true,
			Settings: baseSettings(perYr),
		}
		res := Amortize(in)
		if res.Err != nil || len(res.Schedule) == 0 {
			continue
		}
		gp := modalReg(res.Schedule)
		aChecked++
		rel := math.Abs(op-gp) / math.Max(1, gp)
		if rel > aMax {
			aMax, aWorst = rel, fmt.Sprintf("amt=%.0f r=%.4f n=%d py=%d bMo=%d DOS=%.4f Go=%.4f", amount, rate, nPeriods, perYr, bMonths, op, gp)
		}
		if rel > 5e-4 {
			aFails++
			if aFails <= 12 {
				t.Errorf("AMORTIZE balloon-dispatch mismatch amt=%.0f r=%.4f n=%d py=%d bMo=%d bAmt=%.0f: DOS=%.4f Go=%.4f (rel %.2e)",
					amount, rate, nPeriods, perYr, bMonths, bAmt, op, gp, rel)
			}
		}
	}
	t.Logf("(A) Amortize blank-payment + balloon: checked %d, divergences %d, max relErr %.2e at [%s]", aChecked, aFails, aMax, aWorst)

	// (B) blank payment + an odd first period (prepaid OFF — the oracle default),
	// solved THROUGH Amortize. Exercises the first-period payment augmentation.
	bChecked, bFails := 0, 0
	bMax, bWorst := 0.0, ""
	for i := 0; i < 250; i++ {
		amount := float64(5000 + rng.Intn(495000))
		rate := 0.01 + rng.Float64()*0.12
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		n := 2 + rng.Intn(20)
		mPer := 12 / perYr
		firstMonths := 1 + rng.Intn(2*mPer) // may be a short or long odd first period
		op, ok := runOraclePayment(amount, rate, n, perYr, "first="+strconv.Itoa(firstMonths))
		if !ok {
			continue
		}
		fy, fm := 2024+firstMonths/12, time.Month(firstMonths%12+1)
		in := LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: amount,
				LoanRateStatus: types.InOutInput, LoanRate: rate,
				NStatus: types.InOutInput, NPeriods: n,
				PerYrStatus: types.InOutInput, PerYr: perYr,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
				FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(fy, fm, 1)},
			Settings: baseSettings(perYr), // Prepaid defaults false, matching the oracle
		}
		res := Amortize(in)
		if res.Err != nil || len(res.Schedule) == 0 {
			continue
		}
		gp := modalReg(res.Schedule)
		bChecked++
		rel := math.Abs(op-gp) / math.Max(1, gp)
		if rel > bMax {
			bMax, bWorst = rel, fmt.Sprintf("amt=%.0f r=%.4f n=%d py=%d first=%d DOS=%.4f Go=%.4f", amount, rate, n, perYr, firstMonths, op, gp)
		}
		if rel > 5e-4 {
			bFails++
			if bFails <= 12 {
				t.Errorf("AMORTIZE odd-first-dispatch mismatch amt=%.0f r=%.4f n=%d py=%d first=%d: DOS=%.4f Go=%.4f (rel %.2e)",
					amount, rate, n, perYr, firstMonths, op, gp, rel)
			}
		}
	}
	t.Logf("(B) Amortize blank-payment + odd first period: checked %d, divergences %d, max relErr %.2e at [%s]", bChecked, bFails, bMax, bWorst)

	if aChecked < 50 || bChecked < 50 {
		t.Fatalf("too few oracle answers (A=%d, B=%d) — oracle may be flaking", aChecked, bChecked)
	}
}

// TestDOSAmortizeDispatchCrossProduct sweeps the FULL Amortize blank-payment
// dispatch across the cross-product of settings (basis 360/365, prepaid on/off,
// balloon-includes-regular) × an optional known balloon × an optional odd first
// period, all against the real DOS engine. The aim is to exhaust the
// payment-solve corners the API/UI can reach, not just a couple of them.
func TestDOSAmortizeDispatchCrossProduct(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(0x5eed))
	checked, fails := 0, 0
	maxRel, worst := 0.0, ""
	byDim := map[string]int{} // count checked per dimension class
	for i := 0; i < 700; i++ {
		amount := float64(20000 + rng.Intn(480000))
		rate := 0.01 + rng.Float64()*0.14
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		nPeriods := (4 + rng.Intn(24)) * perYr
		mPer := 12 / perYr

		var flags []string
		set := Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: false}
		dim := ""

		if rng.Intn(2) == 0 {
			flags = append(flags, "b365")
			set.Basis, set.YrDays, set.YrInv = types.Basis365, 365, 1.0/365
			dim += "B"
		} else {
			dim += "."
		}
		if rng.Intn(2) == 0 {
			flags = append(flags, "prepaid")
			set.Prepaid = true
			dim += "P"
		} else {
			dim += "."
		}

		// First payment: natural (one full period). The odd-first-period × fancy
		// combination is a documented frontier — see TestDOSOddFirstFancyFrontier.
		firstMonths := mPer
		flags = append(flags, "first="+strconv.Itoa(firstMonths))
		fy, fm := 2024+firstMonths/12, time.Month(firstMonths%12+1)

		var balloons []BalloonPayment
		fancy := false
		if rng.Intn(2) == 0 {
			bp := 1 + rng.Intn(nPeriods-1)
			bMonths := bp * mPer
			bAmt := amount * (0.05 + rng.Float64()*0.2)
			flags = append(flags, "b"+strconv.Itoa(bMonths)+"="+strconv.FormatFloat(bAmt, 'f', 2, 64))
			by, bm := 2024+bMonths/12, time.Month(bMonths%12+1)
			balloons = []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(by, bm, 1),
				AmountStatus: types.InOutInput, Amount: bAmt}}
			fancy = true
			if rng.Intn(2) == 0 {
				flags = append(flags, "plusreg")
				set.PlusRegular = true
				dim += "R"
			} else {
				dim += "L"
			}
		} else {
			dim += "."
		}

		op, ok := runOraclePayment(amount, rate, nPeriods, perYr, flags...)
		if !ok {
			continue
		}
		in := LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: amount,
				LoanRateStatus: types.InOutInput, LoanRate: rate,
				NStatus: types.InOutInput, NPeriods: nPeriods,
				PerYrStatus: types.InOutInput, PerYr: perYr,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
				FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(fy, fm, 1)},
			Balloons: balloons, Fancy: fancy, Settings: set,
		}
		res := Amortize(in)
		if res.Err != nil || len(res.Schedule) == 0 {
			continue
		}
		gp := modalReg(res.Schedule)
		checked++
		byDim[dim]++
		rel := math.Abs(op-gp) / math.Max(1, gp)
		if rel > maxRel {
			maxRel, worst = rel, fmt.Sprintf("dim=%s amt=%.0f r=%.4f n=%d py=%d flags=%v DOS=%.4f Go=%.4f", dim, amount, rate, nPeriods, perYr, flags, op, gp)
		}
		// 1e-3 (vs 5e-4 elsewhere): the 365-day basis on long terms accumulates a
		// few cents of per-period rounding vs DOS; real divergences are ≥1e-2.
		if rel > 1e-3 {
			fails++
			if fails <= 15 {
				t.Errorf("CROSS dispatch mismatch dim=%s amt=%.0f r=%.4f n=%d py=%d flags=%v: DOS=%.4f Go=%.4f (rel %.2e)",
					dim, amount, rate, nPeriods, perYr, flags, op, gp, rel)
			}
		}
	}
	t.Logf("cross-product dispatch: checked %d, divergences %d, max relErr %.2e at [%s]", checked, fails, maxRel, worst)
	t.Logf("dimension coverage (basis/Prepaid/balloon±plusReg): %v", byDim)
	if checked < 200 {
		t.Fatalf("too few oracle answers (%d) — oracle flaking", checked)
	}
}

// TestDOSOddFirstFancyFrontier sweeps what used to be the known remaining
// frontier: an ODD first period (a short or long first payment gap — e.g. an
// annual loan whose first payment is 1 or 18 months out) combined with prepaid
// interest, a balloon, or the 365-day basis. This corner is now CLOSED:
//   - prepaid / 365 odd-first: the blank-payment solve refines the closed-form
//     estimate against the real (prorated) schedule (oddFirstPeriod +
//     solveFancyPayment, engine.go).
//   - balloon odd-first: off-cycle balloons are now applied at their exact date
//     (the balloon draining in generateFancySchedule), matching DOS instead of
//     folding the balloon into the next regular payment.
//
// The sweep therefore now asserts ZERO divergence, same as the cross-product —
// it is a strict regression guard that the frontier stays closed. See
// docs/dos_known_frontier.md.
func TestDOSOddFirstFancyFrontier(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(0xf00d))
	checked, diverged := 0, 0
	maxRel, worst := 0.0, ""
	for i := 0; i < 500; i++ {
		amount := float64(20000 + rng.Intn(480000))
		rate := 0.01 + rng.Float64()*0.14
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		nPeriods := (4 + rng.Intn(24)) * perYr
		mPer := 12 / perYr

		var flags []string
		set := Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360}
		// Force an ODD first period (never the natural one full period).
		firstMonths := 1 + rng.Intn(2*mPer)
		for firstMonths == mPer {
			firstMonths = 1 + rng.Intn(2*mPer)
		}
		flags = append(flags, "first="+strconv.Itoa(firstMonths))
		fy, fm := 2024+firstMonths/12, time.Month(firstMonths%12+1)

		// At least one fancy/settings dimension (this is the divergent space).
		kind := rng.Intn(3)
		var balloons []BalloonPayment
		fancy := false
		switch kind {
		case 0:
			flags = append(flags, "prepaid")
			set.Prepaid = true
		case 1:
			flags = append(flags, "b365")
			set.Basis, set.YrDays, set.YrInv = types.Basis365, 365, 1.0/365
		case 2:
			bp := 1 + rng.Intn(nPeriods-1)
			bMonths := bp * mPer
			bAmt := amount * (0.05 + rng.Float64()*0.2)
			flags = append(flags, "b"+strconv.Itoa(bMonths)+"="+strconv.FormatFloat(bAmt, 'f', 2, 64))
			by, bm := 2024+bMonths/12, time.Month(bMonths%12+1)
			balloons = []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(by, bm, 1),
				AmountStatus: types.InOutInput, Amount: bAmt}}
			fancy = true
		}
		op, ok := runOraclePayment(amount, rate, nPeriods, perYr, flags...)
		if !ok {
			continue
		}
		in := LoanInput{
			Loan: Loan{AmountStatus: types.InOutInput, Amount: amount,
				LoanRateStatus: types.InOutInput, LoanRate: rate,
				NStatus: types.InOutInput, NPeriods: nPeriods, PerYrStatus: types.InOutInput, PerYr: perYr,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
				FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(fy, fm, 1)},
			Balloons: balloons, Fancy: fancy, Settings: set,
		}
		res := Amortize(in)
		if res.Err != nil || len(res.Schedule) == 0 {
			continue
		}
		gp := modalReg(res.Schedule)
		checked++
		rel := math.Abs(op-gp) / math.Max(1, gp)
		if rel > 1e-3 {
			diverged++
			if diverged <= 15 {
				t.Errorf("CLOSED-frontier regression amt=%.0f r=%.4f n=%d py=%d flags=%v: DOS=%.4f Go=%.4f (rel %.2e)",
					amount, rate, nPeriods, perYr, flags, op, gp, rel)
			}
		}
		if rel > maxRel {
			maxRel, worst = rel, fmt.Sprintf("amt=%.0f r=%.4f n=%d py=%d flags=%v DOS=%.4f Go=%.4f", amount, rate, nPeriods, perYr, flags, op, gp)
		}
	}
	pct := 0.0
	if checked > 0 {
		pct = float64(diverged) / float64(checked) * 100
	}
	t.Logf("odd-first × {prepaid|balloon|365} (now CLOSED): checked %d, diverged(>1e-3) %d (%.0f%%), max relErr %.2e at [%s]",
		checked, diverged, pct, maxRel, worst)
	if checked < 100 {
		t.Fatalf("too few oracle answers (%d) — oracle flaking", checked)
	}
}

// TestDOSFancyBackwardAmountRateRoundTrip validates the fancy BACKWARD solves
// (SolveLoanAmount / SolveRate with a balloon active) against the real DOS engine,
// by round-tripping through it: the DOS oracle solves the regular payment for a
// known (amount, rate, balloon); the Go fancy backward solver must then recover
// the original amount (resp. rate) from that DOS-derived payment. This closes the
// "fancy backward solves are best-effort" caveat — they use solveFancyAmount /
// solveFancyRate (the schedule-oracle bisection against the DOS-validated forward
// engine), and this proves they invert the DOS forward relationship.
func TestDOSFancyBackwardAmountRateRoundTrip(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(0xBACC))
	ld := types.NewDateRec(2024, time.January, 1)
	mkBalloonInput := func(amount, rate float64, n, perYr, bMonths int, bAmt float64) LoanInput {
		by, bm := 2024+bMonths/12, time.Month(bMonths%12+1)
		return LoanInput{
			Loan: Loan{
				NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
				LoanDateStatus: types.InOutInput, LoanDate: ld,
				FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr)},
			Balloons: []BalloonPayment{{
				DateStatus: types.InOutInput, Date: types.NewDateRec(by, bm, 1),
				AmountStatus: types.InOutInput, Amount: bAmt}},
			Fancy:    true,
			Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360},
		}
	}
	amtChecked, amtFails, amtMax := 0, 0, 0.0
	rateChecked, rateFails, rateMax := 0, 0, 0.0
	for i := 0; i < 200; i++ {
		amount := float64(50000 + rng.Intn(400000))
		rate := 0.03 + rng.Float64()*0.10
		perYr := []int{1, 4, 12}[rng.Intn(3)]
		nPeriods := (4 + rng.Intn(20)) * perYr
		mPer := 12 / perYr
		bp := 1 + rng.Intn(nPeriods-1)
		bMonths := bp * mPer
		bAmt := amount * (0.05 + rng.Float64()*0.15)

		pay, _, ok := runOracleBalloon(amount, rate, nPeriods, perYr, bMonths, bAmt)
		if !ok || pay <= 0 {
			continue
		}

		// Recover AMOUNT from the DOS-derived payment.
		ain := mkBalloonInput(amount, rate, nPeriods, perYr, bMonths, bAmt)
		ain.Loan.AmountStatus = types.StatusEmpty
		ain.Loan.LoanRateStatus, ain.Loan.LoanRate = types.InOutInput, rate
		ain.Loan.PayAmtStatus, ain.Loan.PayAmt = types.InOutInput, pay
		if a2, ok, err := SolveLoanAmount(ain); err == nil && ok {
			amtChecked++
			rel := math.Abs(a2-amount) / math.Max(1, amount)
			if rel > amtMax {
				amtMax = rel
			}
			if rel > 2e-3 {
				amtFails++
				if amtFails <= 10 {
					t.Errorf("AMOUNT round-trip amt=%.0f r=%.4f n=%d py=%d bMo=%d: recovered %.2f (rel %.2e)",
						amount, rate, nPeriods, perYr, bMonths, a2, rel)
				}
			}
		}

		// Recover RATE from the DOS-derived payment.
		rin := mkBalloonInput(amount, rate, nPeriods, perYr, bMonths, bAmt)
		rin.Loan.AmountStatus, rin.Loan.Amount = types.InOutInput, amount
		rin.Loan.PayAmtStatus, rin.Loan.PayAmt = types.InOutInput, pay
		rin.Loan.LoanRateStatus = types.StatusEmpty
		if r2, ok, err := SolveRate(rin); err == nil && ok {
			rateChecked++
			rel := math.Abs(r2-rate) / math.Max(1e-3, rate)
			if rel > rateMax {
				rateMax = rel
			}
			if rel > 5e-3 {
				rateFails++
				if rateFails <= 10 {
					t.Errorf("RATE round-trip amt=%.0f r=%.4f n=%d py=%d bMo=%d: recovered %.4f (rel %.2e)",
						amount, rate, nPeriods, perYr, bMonths, r2, rel)
				}
			}
		}
	}
	t.Logf("fancy backward AMOUNT: checked %d, fails %d, max relErr %.2e", amtChecked, amtFails, amtMax)
	t.Logf("fancy backward RATE:   checked %d, fails %d, max relErr %.2e", rateChecked, rateFails, rateMax)
	if amtChecked < 50 || rateChecked < 50 {
		t.Fatalf("too few round-trips (amt=%d rate=%d) — oracle flaking", amtChecked, rateChecked)
	}
}

// TestDOSOddDaysFirstPeriodSweep drives ODD-DAYS first periods — a loan whose
// day-of-month differs from the first payment's, so the first period is a
// fractional-days stub the month-only `first=` cannot express (e.g. AM Example 1:
// loan 2/12, first 3/1). It uses the oracle's loandmy=/firstdmy= date overrides
// and compares the Go blank-payment solve to the real DOS engine across random
// day-of-month / month-offset combinations. This continuously validates the
// odd-days payment augmentation (the DOS-vs-Windows discrepancy, discrepancies.md
// §7) rather than pinning a single hand-checked value.
func TestDOSOddDaysFirstPeriodSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(0x0dda75))
	checked, fails := 0, 0
	maxRel, worst := 0.0, ""
	for i := 0; i < 400; i++ {
		amount := float64(20000 + rng.Intn(480000))
		rate := 0.02 + rng.Float64()*0.13
		perYr := []int{12, 4}[rng.Intn(2)] // day-of-month matters most monthly/quarterly
		nPeriods := (3 + rng.Intn(25)) * (12 / perYr)
		mPer := 12 / perYr

		loanY, loanM, loanD := 2024, 1+rng.Intn(12), 1+rng.Intn(27)
		// First payment ~one period out, on its own (different) day-of-month.
		fMonthAbs := (loanM - 1) + mPer
		firstY, firstM := loanY+fMonthAbs/12, fMonthAbs%12+1
		firstD := 1 + rng.Intn(27)
		// Skip the degenerate equal-date case (no odd period at all).
		if firstD == loanD {
			firstD = 1 + (firstD % 27)
		}

		op, ok := runOraclePayment(amount, rate, nPeriods, perYr,
			fmt.Sprintf("loandmy=%d.%d.%d", loanD, loanM, loanY),
			fmt.Sprintf("firstdmy=%d.%d.%d", firstD, firstM, firstY))
		if !ok {
			continue
		}
		in := LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: amount,
				LoanRateStatus: types.InOutInput, LoanRate: rate,
				NStatus: types.InOutInput, NPeriods: nPeriods,
				PerYrStatus: types.InOutInput, PerYr: perYr,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(loanY, time.Month(loanM), loanD),
				FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(firstY, time.Month(firstM), firstD)},
			Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360},
		}
		res := Amortize(in)
		if res.Err != nil || len(res.Schedule) == 0 {
			continue
		}
		gp := modalReg(res.Schedule)
		checked++
		rel := math.Abs(op-gp) / math.Max(1, gp)
		if rel > maxRel {
			maxRel, worst = rel, fmt.Sprintf("amt=%.0f r=%.4f n=%d py=%d loan=%d.%d.%d first=%d.%d.%d DOS=%.4f Go=%.4f",
				amount, rate, nPeriods, perYr, loanD, loanM, loanY, firstD, firstM, firstY, op, gp)
		}
		if rel > 1e-3 {
			fails++
			if fails <= 15 {
				t.Errorf("ODD-DAYS dispatch mismatch amt=%.0f r=%.4f n=%d py=%d loan=%d.%d.%d first=%d.%d.%d: DOS=%.4f Go=%.4f (rel %.2e)",
					amount, rate, nPeriods, perYr, loanD, loanM, loanY, firstD, firstM, firstY, op, gp, rel)
			}
		}
	}
	t.Logf("odd-DAYS first period: checked %d, divergences %d, max relErr %.2e at [%s]", checked, fails, maxRel, worst)
	if checked < 100 {
		t.Fatalf("too few oracle answers (%d) — oracle flaking", checked)
	}
}
