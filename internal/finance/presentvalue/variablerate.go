// Variable-rate present value support. Ports the forward path of the
// DOS "PVL fancy" (variable-rate Present Value) screen documented in
// legacy/src/win_source/Help/PV_VariableRate.html and implemented in
// legacy/src/dos_source/PRESVALU.pas (compile-time guarded by PVLX).
//
// What's supported in this port:
//   - Forward PV of lump sums and periodic payments using a
//     piecewise-constant rate schedule.
//   - Periodic payments with COLA (each payment integrated period-by-
//     period, COLA applied as a multiplicative factor on amount).
//   - Life-contingency weighting in combination with variable rates.
//     Each payment's variable-rate discount factor is multiplied by
//     the survival probability at the payment date. POD (Payment on
//     Death) is integrated against the variable-rate schedule rather
//     than a single rate. Matches DOS PRESVALU.pas's
//     `{$ifdef ACTU} or (fold_in_life) {$endif}` branches inside the
//     PVLX summation loops (lines 290, 295-300, 389, 395-400) — a
//     combination the Windows port had dropped because the Windows
//     build was compiled without -DACTU.
//
// Backward solves in VR mode (a blank Amount, Date, or Payment-on-Death
// with the screen Sum Value supplied) are supported by inverting the
// true variable-rate forward valuation — see solveVariableRateAmount,
// solveVariableRateDate and solveVariableRatePOD. DOS PVLX supports the
// same set (amtn := valn/FancySummation, the lump/periodic date solve,
// and XPODValue). Solving for an unknown *rate* remains out of scope:
// DOS says "rates cannot be the target of a computation" on the VR
// screen — the IRR concept doesn't fit a rate schedule.
//
// What's deliberately out of scope (mirrors DOS limitations and is
// noted in the help text):
//   - Solving for an unknown rate in VR mode (see above).
//   - SIMPLE interest. DOS exposes a SIMPLE/COMPOUND toggle for the
//     niche legal-damages use case; this port assumes COMPOUND.

package presentvalue

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// SortRateSchedule sorts the schedule by Date ascending in place,
// preserving the contract that schedule[0] is the "starting rate" —
// its Date is treated as -infinity by the integrator regardless of
// the stored value.
func SortRateSchedule(s []RateLine) {
	sort.SliceStable(s, func(i, j int) bool {
		return dateutil.DateComp(s[i].Date, s[j].Date) < 0
	})
}

// integrateRateForward returns ∫_from^to r(s) ds where r(s) is the
// piecewise-constant rate defined by `schedule`. Assumes from <= to
// (the caller is responsible for handling the reverse case via sign).
//
// Algorithm: walk through time from `from`, repeatedly identifying
// the rate currently in effect and the next "rate change" boundary,
// accruing rate × time-span until reaching `to`. Schedule must be
// sorted by Date ascending (use SortRateSchedule first if unsure).
//
// The first schedule entry's Date is intentionally ignored — its
// rate is treated as the starting rate, in effect from -infinity.
// This matches the DOS UX where the first row's date cell was
// labeled "XX" / read-only.
func integrateRateForward(from, to types.DateRec, schedule []RateLine,
	basis types.BasisType, yrInv float64) float64 {

	if len(schedule) == 0 || dateutil.DateComp(from, to) >= 0 {
		return 0
	}

	total := 0.0
	pos := from
	for dateutil.DateComp(pos, to) < 0 {
		// Identify the rate in force at `pos`. Walk forward until the
		// next entry's Date strictly exceeds pos.
		rate := schedule[0].Rate
		for i := 1; i < len(schedule); i++ {
			if dateutil.DateComp(schedule[i].Date, pos) <= 0 {
				rate = schedule[i].Rate
			} else {
				break
			}
		}
		// Find the next boundary after pos (or `to`, whichever comes
		// first). Boundaries are the Dates of entries strictly after
		// the one we're currently inside.
		next := to
		for i := 1; i < len(schedule); i++ {
			if dateutil.DateComp(schedule[i].Date, pos) > 0 {
				if dateutil.DateComp(schedule[i].Date, next) < 0 {
					next = schedule[i].Date
				}
				break // schedule is sorted; first match is the soonest
			}
		}
		yrs := dateutil.YearsDif(next, pos, basis, yrInv, false)
		total += rate * yrs
		pos = next
	}
	return total
}

// VRDiscountFactor returns the value-at-asof multiplier for a single
// payment at paymentDate under variable rates. Result satisfies:
//
//		value_at_asof(payment) = payment.Amount × VRDiscountFactor(...)
//
//	  - paymentDate > asof:  factor < 1  (future cash, discounted)
//	  - paymentDate < asof:  factor > 1  (past cash, accumulated forward)
//	  - paymentDate == asof: factor = 1
//
// Mirrors the sign convention of LumpSumValue in the fixed-rate case:
// fixed-rate uses exp(rate × YearsDif(asof, paymentDate)) which is
// negative when paymentDate is in the future.
func VRDiscountFactor(asof, paymentDate types.DateRec, schedule []RateLine,
	basis types.BasisType, yrInv float64) (float64, error) {

	cmp := dateutil.DateComp(paymentDate, asof)
	if cmp == 0 {
		return 1.0, nil
	}
	if cmp > 0 {
		// paymentDate > asof: discount
		intg := integrateRateForward(asof, paymentDate, schedule, basis, yrInv)
		return interest.Exxp(-intg)
	}
	// paymentDate < asof: accumulate forward
	intg := integrateRateForward(paymentDate, asof, schedule, basis, yrInv)
	return interest.Exxp(intg)
}

// vrPeriodicValue computes the present value of a periodic payment
// series under variable rates by walking period-by-period. Returns
// (value, averageProb) — averageProb is 1.0 when no actuarial config
// is supplied, matching the fixed-rate non-actuarial convention.
//
// COLA convention matches the fixed-rate `periodicSumAnnualCOLA`
// path in calc.go: the entered COLA is an effective annual yield, so
// each anniversary step multiplies the payment amount by (1+cola).
// `COLAMonth` controls when the step lands (anniversary, a specific
// month 1-12, or continuous). For the continuous setting the
// multiplier is exp(yrsFromStart × ln(1+cola)). Before V6-4 / V9
// this path always applied continuous COLA regardless of the
// COLAMonth setting; now it matches the rest of PV.
//
// When actu != nil and contingency != NotContingent, each period's
// contribution is additionally weighted by LifeProb(t, contingency).
// Ported from PRESVALU.pas:290-300 — the ACTU+PVLX combined branch
// inside the periodic summation.
func vrPeriodicValue(amount, cola float64, asOf, fromDate, toDate types.DateRec,
	peryr int, schedule []RateLine, settings *PVSettings,
	actu *actuarial.ActuarialConfig, contingency byte) (float64, float64, []PeriodicInstallment, error) {

	if peryr <= 0 {
		return 0, 0, nil, fmt.Errorf("a periodic payment row has Pmts-Yr blank or zero. Enter how many payments are made per year (for example 12 for monthly)")
	}
	if dateutil.DateComp(fromDate, toDate) > 0 {
		return 0, 0, nil, fmt.Errorf("a periodic payment row has its From Date after its To Date. Swap the two dates so the payments run forward in time")
	}

	applyLife := actu != nil && contingency != actuarial.NotContingent

	// Stepped vs. continuous COLA.
	//
	// This routine is the port of DOS's FANCY summation (PVLXSCRN.pas:305-352
	// FancySummation), which values every payment through
	// UpdateAmountWithCola (PVLXSCRN.pas:103-113):
	//
	//	if (df.c.COLAmonth=CNT) or (b.cola=0) then
	//	  scaledamt:=exxp(b.cola*YearsDif(t,b.fromdate))
	//	else begin
	//	  if (DateComp(t,coladate)>=0) then begin
	//	     scaledamt:=scaledamt*exxp(b.cola);
	//	     inc(coladate.y);
	//	     end;
	//	  end;
	//
	// The gate there is ONLY `COLAmonth=CNT or cola=0`. There is no
	// payments-per-year condition — the fancy walk steps the multiplier at
	// the anniversary for every frequency, annual included.
	//
	// This code used to carry `peryr > 1`, lifted from the *non-fancy*
	// closed-form Summation (PRESVALU.pas:384):
	//
	//	if (cola<>0) and (df.c.COLAmonth<>CNT) and (b[j]^.peryr>1) then
	//	   Summation:=SummationForSteppedCola(k,j);
	//
	// In THAT routine the clause is a harmless shortcut: at peryr=1 the
	// closed form's own per-period factor exxp((cola-rate)/RealPerYr(peryr))
	// already advances the payment by exactly exxp(cola) once a year, so the
	// stepped and closed-form results coincide and DOS skips the slower
	// branch. Here the fallback is NOT that closed form — it is
	// exp(YearsDif(t,fromDate) × contCola), and YearsDif is BASIS-DEPENDENT.
	// On 30/360 an anniversary is exactly 1.0 years, so the two agreed and
	// the bug stayed invisible; on actual/365.25 a leap year is 366/365.25 =
	// 1.002053 years and on actual/360 a year is 365/360 = 1.013889, so the
	// annual-payment COLA compounded on the wrong clock.
	//
	// Evidence (pv_oracle vrp_gen, amt=1000 peryr=1 n=3 cola=0.05 rate=0.10
	// from 1/1/2024, single rate step):
	//
	//	basis x360      DOS 3710.319637  Go 3710.319637   (matched)
	//	basis x365      DOS 3709.942314  Go 3710.126403   (+0.18)
	//	basis x365_360  DOS 3702.180548  Go 3706.148131   (+3.97)
	//
	// and the same case with cola=0 matched to all printed digits on every
	// basis, isolating the fault to the COLA term rather than the discount.
	// The table walk (table.go:429) already had this right.
	useStepped := cola != 0 && settings.COLAMonth != types.COLAContinuous
	colaPerYear := 1.0 + cola
	var stepMult float64 = 1.0
	var coladate colaAnniversary
	if useStepped {
		coladate = firstColaAnniversary(fromDate, settings)
	}
	// Continuous COLA: convert the entered yield to a continuous rate
	// so exp(yrsFromStart × contCola) equals (1+cola)^yrsFromStart at
	// integer years.
	contCola := cola
	if cola != 0 && !useStepped {
		contCola = math.Log1p(cola)
	}

	total := 0.0
	probSum := 0.0
	count := 0
	var installments []PeriodicInstallment
	t := fromDate
	origDay := fromDate.Time.Day()

	for dateutil.DateComp(t, toDate) <= 0 {
		var colaMult float64
		if useStepped {
			// ONE step per payment, never a catch-up loop. DOS's
			// UpdateAmountWithCola (PVLXSCRN.pas:108-111) is a single `if`:
			//
			//	if (DateComp(t,coladate)>=0) then begin
			//	   scaledamt:=scaledamt*exxp(b.cola);
			//	   inc(coladate.y);
			//	   end;
			//
			// A `for` here is indistinguishable from the `if` for every anchor
			// whose own month/day exists in every year — but not for a Feb-29
			// anchor. There the raw anniversary 2/29/YYYY is unreachable in the
			// three non-leap years (AddPeriod clamps the payment to 2/28, and
			// 28 >= 29 is false), so DOS's coladate falls a year behind the
			// payment cursor and stays there; the next leap-year payment lands
			// exactly ON 2/29 and, under a `for`, satisfies the comparison
			// twice — double-COLA'ing that one payment and re-syncing after it.
			//
			// Evidence (pv_oracle vrp_gen, amt=1000 peryr=1 cola=0.05
			// rate=0.10 basis x360, from 2/29/2024): identical through n=3,
			// then a FLAT +38.178877 for n=4..7 — the signature of exactly one
			// over-COLA'd payment (2/29/2028) rather than a drift — and a
			// second step up at n=8 when 2/29/2032 comes into range.
			if coladate.reached(t) {
				stepMult *= colaPerYear
				// Raw (y,m,d) anniversary (DOS inc(coladate.y) on an
				// unnormalized daterec) -- a normalized Feb-29 -> Mar-01 anchor
				// would step one payment late in leap years, under-COLA'ing the
				// Feb-29 payment (audit D1-followup, discrepancies.md sec 32).
				coladate = coladate.next()
			}
			colaMult = stepMult
		} else {
			yrsFromStart := dateutil.YearsDif(t, fromDate, settings.Basis, settings.YrInv, false)
			m, err := interest.Exxp(yrsFromStart * contCola)
			if err != nil {
				return 0, 0, nil, err
			}
			colaMult = m
		}
		df, err := VRDiscountFactor(asOf, t, schedule, settings.Basis, settings.YrInv)
		if err != nil {
			return 0, 0, nil, err
		}
		prob := 1.0
		if applyLife {
			prob = actu.LifeProb(t, contingency)
		}
		ifpd := amount * colaMult * df // discounted, NOT survival-weighted (DOS ifpd)
		weighted := ifpd * prob
		total += weighted
		probSum += prob
		count++
		// Per-payment-date breakdown for the DOS table's probability column
		// (pvltable.pas:514-533), populated only under a life contingency.
		if applyLife {
			installments = append(installments, PeriodicInstallment{
				Date:   t,
				IfPaid: ifpd,
				Prob:   prob,
				Value:  weighted,
			})
		}

		// DOS's FancySummation stops early once one payment's contribution
		// underflows (PVLXSCRN.pas:332):
		//
		//	until (DateComp(movingdate,todate)>0) or (abs(part)<teeny);
		//
		// `part` there is ValueOfOnePayment(colamt, movingdate) — the summation
		// is scaled to a UNIT payment and multiplied by amtn afterwards, so the
		// threshold is on colaMult*discount (times LifeProb under a
		// contingency), NOT on the amount-scaled figure. Comparing `ifpd` here
		// would move the cutoff by a factor of `amount` and truncate a stream
		// DOS would keep summing.
		//
		// `repeat...until` is post-test, so the payment that trips the guard is
		// itself included and only the tail beyond it is dropped — hence the
		// break sits after the accumulation.
		//
		// Measured effect on a stream long enough to reach it (amt=1000 peryr=1
		// n=90 rate=0.50 x360): DOS 2541.494082 vs an un-truncated Go
		// 2541.494083. Real but ~1e-9 — ported for exactness, not because any
		// screen could show it.
		//
		// SCOPED TO THE NON-LIFE PATH, deliberately. Written as a literal
		// transcription of FancySummation this guard multiplies LifeProb into
		// the tested quantity, and a Dead / Only-1 / Only-2 contingency has
		// LifeProb ~= 0 at the START of the stream — so it trips on the FIRST
		// payment and returns 0 for the entire dead branch. Measured on
		// TestVR_ActuarialComplementarity: plain=358387.4451 living=338873.2507
		// dead=0.0000, i.e. living+dead misses plain by 19,514.1943 and the
		// Living+Dead == non-contingent identity the help advertises collapses.
		//
		// This is not a judgement call against DOS — it is what DOS itself
		// does. Summation, the sibling walk in PRESVALU.pas:395-403, carries
		// the same `abs(part)>teeny` test AND an explicit suppression of it for
		// exactly these three contingencies:
		//
		//	while (DateComp(t,todate)<=0) and (abs(part)>teeny) do begin
		//	   part:=exxp(YearsDif(t,fromdate)*cola-YearsDif(t,asof)*r.rate);
		//	   if (fold_in_life) then part:=part*LifeProb(t,b[j]^.actn);
		//	   theresult:=theresult + part;
		//	   if (b[j]^.actn in [DEAD,ONLY_1,ONLY_2]) and (DateComp(t,actu_now)<=0)
		//	     then part:=1;
		//	       {Under this condition, part will be zero, even though the non-zero
		//	        part of the contingent payements is yet to come.  We don' want
		//	        (part<teeny) to cause the summation above to truncate (JJM 2/28/93).}
		//
		// (DEAD=2, ONLY_1=ONLY_1_LIVING=3, ONLY_2=ONLY_2_LIVING=4 —
		// PETYPES.PAS:169-171 — matching types.Dead / Only1Living / Only2Living.)
		// JJM's patch pins `part` back to 1 for every payment up to actu_now so
		// the underflow test cannot fire while the survival weight is still in
		// its zero window. FancySummation never received that patch, so the PVLX
		// fancy walk retains the defect JJM had already diagnosed and fixed
		// upstream — and the oracle cannot arbitrate which is intended, because
		// it is compiled without ACTU (legacy/oracle/build_linux.sh:108 builds
		// -DV_3;SCROLLS;PVLX only) and PEDATA.pas:146 pins `fold_in_life=false`
		// in that configuration. Under the oracle's own build `prob` is always
		// exactly 1, so the non-life restriction below reproduces every
		// differential result bit for bit while declining to import a
		// truncation DOS's own author documented as wrong.
		//
		// The same reasoning, reached before the DOS citation was found, already
		// governs the sibling forward walk — see the note in
		// calc.go periodicWithActuarial.
		if !applyLife {
			unit := colaMult * df
			if math.Abs(unit) < teeny {
				break
			}
		}

		next, err := dateutil.AddPeriod(t, peryr, origDay, false)
		if err != nil {
			return 0, 0, nil, err
		}
		t = next
	}
	avgProb := 1.0
	if count > 0 {
		avgProb = probSum / float64(count)
	}
	return total, avgProb, installments, nil
}

// forwardVariableRate is the entry point for forward PV calculations
// when input.RateSchedule is non-empty. Mirrors forwardOnly's outer
// structure but routes every discount through the schedule rather
// than a single rate.
//
// Returns an error (via result.Err) if any row carries a missing
// input (backward solve in VR mode isn't supported here — matches
// DOS limitation flagged in PV_VariableRate.html).
func forwardVariableRate(input PVInput) PVResult {
	var result PVResult
	result.LumpSums = input.LumpSums
	result.Periodics = input.Periodics

	if len(input.RateSchedule) == 0 {
		result.Err = fmt.Errorf("forwardVariableRate called without a rate schedule")
		return result
	}
	// Defensive sort — the API plumbing is supposed to do this but
	// the engine accepting unsorted input would silently produce
	// wrong numbers, so don't trust the caller.
	schedule := make([]RateLine, len(input.RateSchedule))
	copy(schedule, input.RateSchedule)
	SortRateSchedule(schedule)

	if !dateutil.DateOK(input.PresVal.AsOf) {
		result.Err = fmt.Errorf("the variable-rate present value cannot be computed without an As-of Date. Fill in the As-of Date — it is the date everything is discounted to")
		return result
	}
	asOf := input.PresVal.AsOf

	sumValue := 0.0

	// Lump sums.
	for i := range result.LumpSums {
		ls := &result.LumpSums[i]
		if !dateutil.DateOK(ls.Date) || ls.AmtStatus < types.InOutDefault {
			result.Err = fmt.Errorf("single payment line %d is missing its Date or Amount. With a variable-rate schedule every row must be complete — Per%%Sense cannot solve for a blank field here, so fill in both the Date and the Amount", i+1)
			return result
		}
		df, err := VRDiscountFactor(asOf, ls.Date, schedule,
			input.Settings.Basis, input.Settings.YrInv)
		if err != nil {
			result.Err = err
			return result
		}
		prob := 1.0
		if input.Actuarial != nil && ls.Act != actuarial.NotContingent {
			prob = input.Actuarial.LifeProb(ls.Date, ls.Act)
		}
		ls.Val = ls.Amt * df * prob
		ls.Prob = prob
		ls.ValStatus = types.InOutOutput
		sumValue += ls.Val
	}

	// Periodics.
	for j := range result.Periodics {
		pp := &result.Periodics[j]
		if !dateutil.DateOK(pp.FromDate) || !dateutil.DateOK(pp.ToDate) {
			result.Err = fmt.Errorf("periodic payment line %d is missing its From Date or To Date. With a variable-rate schedule every row must be complete — fill in both the From Date and the To Date", j+1)
			return result
		}
		if pp.AmtStatus < types.InOutDefault {
			result.Err = fmt.Errorf("periodic payment line %d has a blank Amount. A variable-rate schedule can solve for one blank Amount only when the target Present Value is supplied — fill in the Present Value, or enter the Amount on this row", j+1)
			return result
		}
		if pp.PerYr <= 0 {
			result.Err = fmt.Errorf("periodic payment line %d has Pmts-Yr blank or zero (got %d). Enter how many payments are made per year, for example 12 for monthly", j+1, pp.PerYr)
			return result
		}
		cola := pp.COLA
		if pp.COLAStatus < types.InOutDefault {
			cola = 0
		}
		val, avgProb, insts, err := vrPeriodicValue(pp.Amt, cola, asOf, pp.FromDate, pp.ToDate,
			pp.PerYr, schedule, &input.Settings, input.Actuarial, pp.Act)
		if err != nil {
			result.Err = err
			return result
		}
		pp.Val = val
		pp.ValStatus = types.InOutOutput
		pp.Prob = avgProb
		pp.Installments = insts
		sumValue += val
	}

	// Payment on Death under variable rates. Mirrors PRESVALU.pas line
	// 689 (`if (fold_in_life) then sumvalue:=sumvalue+PodValue(...)`)
	// inside the PVLX branch. The discount callback integrates the
	// rate schedule from asOf to the mid-point of each future death
	// year, so POD value is folded into the running sum the same way
	// the fixed-rate forwardOnly path does.
	if input.Actuarial != nil && input.Actuarial.POD != 0 {
		podVal := input.Actuarial.PODValueFunc(asOf,
			func(yearsFromAsOf float64) float64 {
				// Find the date `yearsFromAsOf` years after asOf
				// using the same basis the rest of the engine uses.
				// We can't synthesize a DateRec from a float year
				// count via dateutil, so do the integration directly
				// in years space: pretend each year is a single
				// segment under the rate active at that distance.
				return vrDiscountByYears(yearsFromAsOf, asOf, schedule, &input.Settings)
			})
		result.PODValue = podVal
		sumValue += podVal
	}

	result.SumValue = sumValue
	return result
}

// vrDiscountByYears returns the variable-rate discount factor for a
// payment that lands `yearsFromAsOf` years after `asOf`. Used by the
// PODValue integration where the death year is expressed as a
// fractional-year offset rather than an explicit DateRec. Internally
// converts the offset back to a synthetic date by adding 365×offset
// days to asOf (the 1.0/365 unit cancels regardless of basis because
// we then re-invoke YearsDif on the schedule walk inside
// integrateRateForward — both directions use the same Basis).
func vrDiscountByYears(yearsFromAsOf float64, asOf types.DateRec,
	schedule []RateLine, settings *PVSettings) float64 {

	if yearsFromAsOf == 0 {
		return 1.0
	}
	// Synthesize the target date by adding days.
	days := int(yearsFromAsOf*365 + 0.5)
	targetTime := asOf.Time.AddDate(0, 0, days)
	targetDate := types.NewDateRec(targetTime.Year(), targetTime.Month(), targetTime.Day())
	df, err := VRDiscountFactor(asOf, targetDate, schedule,
		settings.Basis, settings.YrInv)
	if err != nil {
		return 0
	}
	return df
}

// vrUnknownAmount scans a variable-rate input for exactly one row
// (lump sum or periodic) whose amount is missing. It returns the
// row's kind and index, and ok=true only when precisely one amount
// is unknown — the case the VR backward solve can handle.
func vrUnknownAmount(input *PVInput) (isLump bool, idx int, ok bool) {
	count := 0
	for i := range input.LumpSums {
		ls := &input.LumpSums[i]
		if dateutil.DateOK(ls.Date) && ls.AmtStatus < types.InOutDefault {
			isLump, idx = true, i
			count++
		}
	}
	for j := range input.Periodics {
		pp := &input.Periodics[j]
		if dateutil.DateOK(pp.FromDate) && dateutil.DateOK(pp.ToDate) &&
			pp.AmtStatus < types.InOutDefault {
			isLump, idx = false, j
			count++
		}
	}
	return isLump, idx, count == 1
}

// solveVariableRateAmount back-solves a single unknown payment amount
// in variable-rate mode, given the target Sum Value. The present
// value is linear in the amount, so two forward runs (amount 0 and
// amount 1) bracket it: amount = (target - pvAt0) / unitValue.
//
// Ported from legacy/src/dos_source/PRESVALU.pas: the PVLX branch
// `amtn := valn / FancySummation(j)` (line 949).
func solveVariableRateAmount(input PVInput, isLump bool, idx int) PVResult {
	target := input.PresVal.SumValue

	run := func(amt float64) PVResult {
		clone := input
		if isLump {
			ls := make([]LumpSumPayment, len(input.LumpSums))
			copy(ls, input.LumpSums)
			ls[idx].Amt = amt
			ls[idx].AmtStatus = types.InOutInput
			clone.LumpSums = ls
		} else {
			ps := make([]PeriodicPayment, len(input.Periodics))
			copy(ps, input.Periodics)
			ps[idx].Amt = amt
			ps[idx].AmtStatus = types.InOutInput
			clone.Periodics = ps
		}
		return forwardVariableRate(clone)
	}

	r0 := run(0)
	if r0.Err != nil {
		return r0
	}
	r1 := run(1)
	if r1.Err != nil {
		return r1
	}
	unit := r1.SumValue - r0.SumValue
	if math.Abs(unit) < teeny {
		return PVResult{Err: fmt.Errorf(
			"cannot solve for the blank payment amount: that row contributes a " +
				"present value of zero under the variable-rate schedule, so its " +
				"Amount cannot be worked out from the target Present Value. Check " +
				"the row's dates and the rate schedule, or enter the Amount yourself")}
	}
	solved := (target - r0.SumValue) / unit

	result := run(solved)
	if result.Err == nil {
		if isLump {
			result.LumpSums[idx].AmtStatus = types.InOutOutput
		} else {
			result.Periodics[idx].AmtStatus = types.InOutOutput
		}
	}
	return result
}

// solveVariableRatePOD back-solves an unknown Payment-on-Death amount in
// variable-rate mode from the target Sum Value. As in fixed-rate mode
// the POD present value is linear in the POD face amount, so two forward
// runs (POD 0 and a large probe) bracket it. Routing through
// forwardVariableRate means the death benefit is discounted through the
// rate schedule (PODValueFunc with VRDiscountFactor), not a single rate
// — the fixed-rate solveUnknownPOD's constant-rate unit value would be
// wrong here.
//
// Ported from legacy/src/dos_source/PRESVALU.pas: the PVLX backward
// `val := val - XPODValue` branch (line 842) feeding ComputeUnknownPOD.
func solveVariableRatePOD(input PVInput) PVResult {
	target := input.PresVal.SumValue

	runPOD := func(pod float64) PVResult {
		a := *input.Actuarial
		a.POD = pod
		a.PODUnknown = false
		clone := input
		clone.Actuarial = &a
		return forwardVariableRate(clone)
	}

	r0 := runPOD(0)
	if r0.Err != nil {
		return r0
	}
	// Probe with a large amount and divide so PODValue's cent-rounding
	// stays negligible (probing with POD=1 would let it swamp the result).
	const probe = 1e6
	rp := runPOD(probe)
	if rp.Err != nil {
		return rp
	}
	unit := (rp.SumValue - r0.SumValue) / probe
	if math.Abs(unit) < teeny {
		return PVResult{Err: fmt.Errorf(
			"cannot solve for the Payment on Death amount because the life-contingency settings give it no chance of being paid (zero death probability) under the rate schedule. Check the age and life-table settings, or enter the Payment on Death amount yourself instead of leaving it blank")}
	}
	solved := (target - r0.SumValue) / unit

	result := runPOD(solved)
	result.POD = solved
	return result
}

// VR date-solve target kinds.
const (
	vrDateLump = iota
	vrDatePeriodicFrom
	vrDatePeriodicTo
)

// vrUnknownDate reports whether exactly one row in a variable-rate
// screen has a single blank date (a lump Date, or a periodic From/To
// date) while that row's Amount is supplied. Rows missing their Amount
// are handled by the amount solver, which runs first; a periodic row
// missing both dates counts as two unknowns and disqualifies the solve.
func vrUnknownDate(input *PVInput) (kind, idx int, ok bool) {
	count := 0
	for i := range input.LumpSums {
		ls := &input.LumpSums[i]
		if ls.AmtStatus >= types.InOutDefault && !dateutil.DateOK(ls.Date) {
			kind, idx = vrDateLump, i
			count++
		}
	}
	for j := range input.Periodics {
		pp := &input.Periodics[j]
		if pp.AmtStatus < types.InOutDefault {
			continue
		}
		fromOK := dateutil.DateOK(pp.FromDate)
		toOK := dateutil.DateOK(pp.ToDate)
		switch {
		case fromOK && !toOK:
			kind, idx = vrDatePeriodicTo, j
			count++
		case !fromOK && toOK:
			kind, idx = vrDatePeriodicFrom, j
			count++
		case !fromOK && !toOK:
			count += 2 // both blank: not a single-unknown solve
		}
	}
	return kind, idx, count == 1
}

// solveVariableRateDate back-solves a single blank date (lump Date, or
// periodic From/To) in variable-rate mode from the target Sum Value.
// The present value is monotonic in the unknown date, so the solver
// brackets a sign change and bisects on the calendar day against the
// true variable-rate forward valuation — which already folds in the
// rate schedule, COLA and any life-contingency weighting. This is more
// accurate than DOS's PVLX date solve, which approximates with the
// single starting rate (PRESVALU.pas:957-999 uses c[1]^.r.rate); the
// trade-off is intentional and documented, like the day-level
// refinement on the fixed-rate date solve.
func solveVariableRateDate(input PVInput, kind, idx int) PVResult {
	target := input.PresVal.SumValue

	// build returns a clone with the unknown date set to cand and the
	// solved-field status stamped (output when final).
	build := func(cand types.DateRec, status int8) PVInput {
		clone := input
		switch kind {
		case vrDateLump:
			ls := make([]LumpSumPayment, len(input.LumpSums))
			copy(ls, input.LumpSums)
			ls[idx].Date = cand
			ls[idx].DateStatus = status
			clone.LumpSums = ls
		case vrDatePeriodicTo:
			ps := make([]PeriodicPayment, len(input.Periodics))
			copy(ps, input.Periodics)
			ps[idx].ToDate = cand
			ps[idx].ToDateStatus = status
			clone.Periodics = ps
		case vrDatePeriodicFrom:
			ps := make([]PeriodicPayment, len(input.Periodics))
			copy(ps, input.Periodics)
			ps[idx].FromDate = cand
			ps[idx].FromDateStatus = status
			clone.Periodics = ps
		}
		return clone
	}

	sumAt := func(cand types.DateRec) (float64, error) {
		r := forwardVariableRate(build(cand, types.InOutInput))
		if r.Err != nil {
			return 0, r.Err
		}
		return r.SumValue, nil
	}

	// Bracket the unknown date. The value is monotonic in it, so a
	// sign change of (sumAt - target) is bracketed by the row's
	// natural date limits widened by up to ~80 years.
	const span = 80
	asof := input.PresVal.AsOf
	var lo, hi types.DateRec
	switch kind {
	case vrDateLump:
		lo = asof
		hi = types.DateRec{Time: asof.Time.AddDate(span, 0, 0)}
	case vrDatePeriodicTo:
		from := input.Periodics[idx].FromDate
		lo = types.DateRec{Time: from.Time.AddDate(0, 0, 1)}
		hi = types.DateRec{Time: from.Time.AddDate(span, 0, 0)}
	case vrDatePeriodicFrom:
		to := input.Periodics[idx].ToDate
		lo = types.DateRec{Time: to.Time.AddDate(-span, 0, 0)}
		hi = types.DateRec{Time: to.Time.AddDate(0, 0, -1)}
	}

	fLoV, err := sumAt(lo)
	if err != nil {
		return PVResult{Err: err}
	}
	fHiV, err := sumAt(hi)
	if err != nil {
		return PVResult{Err: err}
	}
	fLo := fLoV - target
	fHi := fHiV - target
	if (fLo > 0) == (fHi > 0) {
		return PVResult{Err: fmt.Errorf(
			"cannot solve for the blank date on that row: the target Present Value of %.2f is not reachable from the row's Amount and the rate schedule at any date in the supported range. Check the Amount, the Present Value, and the rate schedule, or enter the date yourself", target)}
	}

	for i := 0; i < 60 && hi.Time.Sub(lo.Time) > 24*time.Hour; i++ {
		mid := types.DateRec{Time: lo.Time.Add(hi.Time.Sub(lo.Time) / 2)}
		fMidV, e := sumAt(mid)
		if e != nil {
			break
		}
		fMid := fMidV - target
		if (fMid > 0) == (fLo > 0) {
			lo, fLo = mid, fMid
		} else {
			hi, fHi = mid, fMid
		}
	}

	// Pick the bracket end closest to the target.
	solved := lo
	if math.Abs(fHi) < math.Abs(fLo) {
		solved = hi
	}

	// The To Date value is a step function — it is flat between
	// installment dates (no payment is added until the date crosses the
	// next one), so the bisection can land anywhere inside the last flat
	// interval. Snap down to the actual last installment date so the
	// reported To Date is the real final payment date (matching the
	// forward input and DOS's installment-grid answer), not an arbitrary
	// day later that happens to give the same present value.
	if kind == vrDatePeriodicTo {
		from := input.Periodics[idx].FromDate
		peryr := input.Periodics[idx].PerYr
		cur := from
		for {
			next, e := dateutil.AddPeriod(cur, peryr, from.Time.Day(), false)
			if e != nil || next.Time.After(solved.Time) {
				break
			}
			cur = next
		}
		solved = cur
	}

	// Run once more to produce the final result with the solved date
	// stamped as output.
	result := forwardVariableRate(build(solved, types.InOutOutput))
	return result
}
