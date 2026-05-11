// Backward calculation paths for the present value screen.
//
// The DOS PRESVALU.pas program supports field-presence-driven dispatch:
// the user fills in some fields and leaves others blank, and the program
// chooses a formula to solve for whatever is missing. This file ports the
// classification (FirstPass) and the solver paths (BackwardCalc plus the
// rate / as-of solvers that DOS embedded in FrontwardCalc).
//
// Mapping of solve paths to DOS source:
//
//   PV-1  lump-sum amount given date+value  -> PRESVALU.pas:866-891
//   PV-2  lump-sum date given amount+value  -> PRESVALU.pas:892-931 (Newton)
//   PV-4  periodic amount given dates+value -> PRESVALU.pas:943-956
//   PV-5  periodic toDate given fromDate+amount+value -> PRESVALU.pas:965-999
//   PV-6  periodic fromDate given toDate+amount+value -> PRESVALU.pas:1000-1070
//   PV-8  rate given full payment list + sumvalue -> PRESVALU.pas:693-754
//   PV-9  asof given rate + payments + sumvalue   -> PRESVALU.pas:755-818
//
// Ported from legacy/src/dos_source/PRESVALU.pas.

package presentvalue

import (
	"fmt"
	"math"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// rowStatus is the per-row classification produced by FirstPass.
// Mirrors DOS LineBlank..LineOverDetermined defined in types.constants.go.
type rowStatus byte

// FirstPassResult holds the status classification of the screen.
//
// frontward = true means the screen has enough data to compute the
// SumValue from the input fields (any number of fully_specified rows
// and no rows with missing data).
//
// backward = true means exactly one input field is missing somewhere
// and SumValue is known, so we can solve for the missing field.
//
// Both flags being true means the screen is over-determined ("Too many
// unknowns") and the caller should report an error to the user.
type FirstPassResult struct {
	Frontward bool
	Backward  bool
	// BackwardKind identifies which kind of unknown the caller can
	// solve for. Only populated when Backward is true.
	BackwardKind BackwardKind
	// BackwardIndex is the row index (0-based) when BackwardKind
	// targets a lump-sum or periodic row.
	BackwardIndex int
	// LumpSumStatus and PeriodicStatus carry the per-row status flag
	// set by FirstPass so callers can render hints / errors.
	LumpSumStatus  []byte
	PeriodicStatus []byte
	// Err is set when FirstPass detects an unrecoverable problem
	// (e.g. dates out of order, value but no date or amount).
	Err error
}

// BackwardKind identifies which field the backward calc will solve for.
type BackwardKind int

const (
	BackwardNone           BackwardKind = iota
	BackwardLumpAmount                  // PV-1
	BackwardLumpDate                    // PV-2
	BackwardPeriodicAmount              // PV-4
	BackwardPeriodicToDate              // PV-5
	BackwardPeriodicFrom                // PV-6
	BackwardRate                        // PV-8
	BackwardAsOf                        // PV-9
)

// FirstPass walks the screen state and assigns each row a status code.
// It then sets Frontward / Backward dispatch flags following the rules
// in PRESVALU.pas: procedure FirstPass (lines 544-654).
//
// Frontward is true if every populated row has fully_specified status.
// Backward is true if exactly one row contains_unknown AND the
// SumValue is provided in the present-value line.
//
// Ported from legacy/src/dos_source/PRESVALU.pas: procedure FirstPass.
func FirstPass(input *PVInput) FirstPassResult {
	res := FirstPassResult{
		LumpSumStatus:  make([]byte, len(input.LumpSums)),
		PeriodicStatus: make([]byte, len(input.Periodics)),
	}

	// Block 3: PresVal line classification.
	// In DOS this is YieldRateTranslation -> sets c[k]^.status to
	// missing_3 (rate, asof, sumvalue all missing) up to fully_specified.
	pv := &input.PresVal

	// BLOCK 2 - Periodic block. PRESVALU.pas:594-629.
	for j := range input.Periodics {
		b := &input.Periodics[j]

		// Date order check (PRESVALU.pas:599-604).
		if b.FromDateStatus >= types.InOutDefault &&
			b.ToDateStatus >= types.InOutDefault &&
			b.PerYrStatus >= types.InOutDefault &&
			b.PerYr > 0 {
			if dateutil.DateComp(b.FromDate, b.ToDate) >= 0 {
				res.Err = fmt.Errorf(
					"dates are out of order, line %d (check setting for Yr to divide century)",
					j+1)
				return res
			}
		}

		// status starts at fully_specified, decrements for each missing field.
		status := types.LineFullySpecified
		if b.FromDateStatus < types.InOutDefault {
			status--
		}
		if b.ToDateStatus < types.InOutDefault {
			status--
		}
		// "amount or value" - if both are missing the row is missing-3
		if b.AmtStatus < types.InOutDefault && b.ValStatus < types.InOutDefault {
			status--
		}
		// Over-determined check: all four (fromdate, todate, amount,
		// value) supplied is too much. DOS
		// ComputePeriodicLineValues at PRESVALU.pas:466-533 records
		// this as an over_determined row error.
		if b.FromDateStatus >= types.InOutDefault &&
			b.ToDateStatus >= types.InOutDefault &&
			b.AmtStatus >= types.InOutDefault &&
			b.ValStatus >= types.InOutDefault {
			res.Err = fmt.Errorf("periodic payment row is over-"+
				"determined - leave one of dates, amount, or value "+
				"blank (line %d)", j+1)
			return res
		}
		if b.PerYrStatus < types.InOutDefault {
			// peryr missing -> step down by 4 (DOS: dec(b[j]^.status,4))
			if status >= 4 {
				status -= 4
			} else {
				status = types.LineMissing4
			}
		}
		// C-P-2/3 (periodic): a contains_unknown row with amt=0 or
		// val=0 can't be solved (divides by the supplied field).
		// Status here is between LineMissing4 and LineFullySpecified;
		// LineContainsUnknown is the case we care about.
		if status == types.LineContainsUnknown {
			if b.AmtStatus >= types.InOutDefault && b.Amt == 0 {
				res.Err = fmt.Errorf("amount cannot be zero on a "+
					"periodic payment row, line %d", j+1)
				return res
			}
			if b.ValStatus >= types.InOutDefault && b.Val == 0 {
				res.Err = fmt.Errorf("value cannot be zero on a "+
					"periodic payment row, line %d", j+1)
				return res
			}
		}
		res.PeriodicStatus[j] = status
		b.Status = int(status)
	}

	// BLOCK 1 - LumpSum block. Each row is fully_specified when it has
	// date AND (amount OR value), contains_unknown otherwise.
	// PRESVALU.pas inlines this into ComputeLumpsumLineValues.
	for i := range input.LumpSums {
		a := &input.LumpSums[i]
		nFields := 0
		if a.DateStatus >= types.InOutDefault {
			nFields++
		}
		if a.AmtStatus >= types.InOutDefault {
			nFields++
		}
		if a.ValStatus >= types.InOutDefault {
			nFields++
		}
		var status byte
		switch nFields {
		case 0:
			status = types.LineBlank
		case 1:
			// C-P-2 / C-P-3: a contains_unknown row with only
			// amt=0 or only val=0 cannot be solved (the supplied
			// field is the divisor in the back-solve). DOS
			// PRESVALU.pas: ComputeLumpsumLineValues records this
			// as RecordError(amountcol/valuecol).
			if a.AmtStatus >= types.InOutDefault && a.Amt == 0 {
				res.Err = fmt.Errorf("amount cannot be zero on a "+
					"single payment row, line %d", i+1)
				return res
			}
			if a.ValStatus >= types.InOutDefault && a.Val == 0 {
				res.Err = fmt.Errorf("value cannot be zero on a "+
					"single payment row, line %d", i+1)
				return res
			}
			status = types.LineContainsUnknown
		case 2:
			status = types.LineFullySpecified
		default:
			// C-P-4: lump sum row over-determined — date+amt+val all
			// present. DOS PRESVALU.pas:
			// ComputeLumpsumLineValues records DP_DateAmountNoValue.
			res.Err = fmt.Errorf("single payment row is over-"+
				"determined - leave one of date, amount, or value "+
				"blank (line %d)", i+1)
			return res
		}
		res.LumpSumStatus[i] = status
		a.Status = int(status)
	}

	// BLOCK 3 - PresVal line classification.
	// fully_specified = rate + asof + sumvalue.
	// missing one of those is contains_unknown.
	pvFields := 0
	if pv.R.Status > types.StatusEmpty {
		pvFields++
	}
	if pv.AsOfStatus >= types.InOutDefault {
		pvFields++
	}
	if pv.SumValueStatus >= types.InOutDefault {
		pvFields++
	}
	switch pvFields {
	case 0, 1:
		pv.Status = int(types.LineMissing2) // not enough on its own
	case 2:
		pv.Status = int(types.LineContainsUnknown)
	case 3:
		pv.Status = int(types.LineFullySpecified)
	}

	// Determine frontward / backward dispatch.
	// Frontward: rate + asof known AND every populated row is fully_specified.
	if pv.R.Status > types.StatusEmpty && pv.AsOfStatus >= types.InOutDefault {
		res.Frontward = true
		for i := range input.LumpSums {
			s := res.LumpSumStatus[i]
			if s == types.LineContainsUnknown ||
				(s != types.LineBlank && s < types.LineFullySpecified) {
				res.Frontward = false
				break
			}
		}
		if res.Frontward {
			for j := range input.Periodics {
				s := res.PeriodicStatus[j]
				if s == types.LineContainsUnknown ||
					(s != types.LineBlank && s < types.LineFullySpecified) {
					res.Frontward = false
					break
				}
			}
		}
	}

	// Backward: SumValue is given AND we can identify exactly one
	// missing piece. Tested in priority order matching DOS BackwardCalc.
	if pv.SumValueStatus >= types.InOutDefault {
		// Case A: a lump-sum or periodic row contains exactly one unknown.
		//   PRESVALU.pas:865 (lumpsum) and :939 (periodic).
		for i := range input.LumpSums {
			if res.LumpSumStatus[i] == types.LineContainsUnknown {
				a := &input.LumpSums[i]
				if a.DateStatus >= types.InOutDefault {
					res.Backward = true
					res.BackwardKind = BackwardLumpAmount
					res.BackwardIndex = i
					return res
				}
				if a.AmtStatus >= types.InOutDefault {
					res.Backward = true
					res.BackwardKind = BackwardLumpDate
					res.BackwardIndex = i
					return res
				}
				// only ValStatus set: insufficient data.
				res.Err = fmt.Errorf(
					"specify either date or amount in single payments, line %d",
					i+1)
				return res
			}
		}
		for j := range input.Periodics {
			if res.PeriodicStatus[j] == types.LineContainsUnknown {
				b := &input.Periodics[j]
				bothDates := b.FromDateStatus >= types.InOutDefault &&
					b.ToDateStatus >= types.InOutDefault
				oneDate := b.FromDateStatus >= types.InOutDefault ||
					b.ToDateStatus >= types.InOutDefault
				if bothDates {
					res.Backward = true
					res.BackwardKind = BackwardPeriodicAmount
					res.BackwardIndex = j
					return res
				}
				if oneDate && b.AmtStatus >= types.InOutDefault {
					res.Backward = true
					res.BackwardIndex = j
					if b.ToDateStatus < types.InOutDefault {
						res.BackwardKind = BackwardPeriodicToDate
					} else {
						res.BackwardKind = BackwardPeriodicFrom
					}
					return res
				}
				res.Err = fmt.Errorf(
					"specify either other date or amount in periodic payments, line %d",
					j+1)
				return res
			}
		}
		// Case B: rate is missing but rows are fully specified -> PV-8.
		if pv.R.Status <= types.StatusEmpty &&
			pv.AsOfStatus >= types.InOutDefault {
			res.Backward = true
			res.BackwardKind = BackwardRate
			return res
		}
		// Case C: asof is missing but rate and rows are fully specified -> PV-9.
		if pv.AsOfStatus < types.InOutDefault &&
			pv.R.Status > types.StatusEmpty {
			res.Backward = true
			res.BackwardKind = BackwardAsOf
			return res
		}
	}

	return res
}

// BackwardCalc dispatches to the appropriate solver based on FirstPass
// classification. Caller supplies the FirstPass result (or it'll be
// computed if nil).
//
// Ported from legacy/src/dos_source/PRESVALU.pas: procedure BackwardCalc
// (and the rate/asof solver branches inside FrontwardCalc).
func BackwardCalc(input PVInput, fp *FirstPassResult) PVResult {
	var result PVResult
	if fp == nil {
		fpVal := FirstPass(&input)
		fp = &fpVal
	}
	if fp.Err != nil {
		result.Err = fp.Err
		return result
	}
	if !fp.Backward {
		result.Err = fmt.Errorf("insufficient data on screen")
		return result
	}

	// Copy input rows into the result so solvers can mutate in place.
	result.LumpSums = append(result.LumpSums, input.LumpSums...)
	result.Periodics = append(result.Periodics, input.Periodics...)

	switch fp.BackwardKind {
	case BackwardLumpAmount:
		solveLumpAmount(&input, &result, fp.BackwardIndex)
	case BackwardLumpDate:
		solveLumpDate(&input, &result, fp.BackwardIndex)
	case BackwardPeriodicAmount:
		solvePeriodicAmount(&input, &result, fp.BackwardIndex)
	case BackwardPeriodicToDate:
		solvePeriodicDate(&input, &result, fp.BackwardIndex, true)
	case BackwardPeriodicFrom:
		solvePeriodicDate(&input, &result, fp.BackwardIndex, false)
	case BackwardRate:
		solveRate(&input, &result)
	case BackwardAsOf:
		solveAsOf(&input, &result)
	default:
		result.Err = fmt.Errorf("unknown backward solve kind")
	}
	return result
}

// computeKnownRowSum walks lump-sum and periodic rows and sums the
// values for those that are fully_specified, computing each value
// where needed. The unknown row at unknownIdx (lump or periodic per
// isLumpSum) is excluded.
//
// Returns the partial sum subtracted from the target SumValue.
//
// Ported from legacy/src/dos_source/PRESVALU.pas:852-862 (the loops
// at the top of BackwardCalc).
func computeKnownRowSum(input *PVInput, isLumpSum bool, unknownIdx int) (float64, error) {
	rate := input.PresVal.R.Rate
	asof := input.PresVal.AsOf
	settings := &input.Settings

	var sum float64
	for i := range input.LumpSums {
		ls := &input.LumpSums[i]
		if isLumpSum && i == unknownIdx {
			continue
		}
		if ls.DateStatus < types.InOutDefault || ls.AmtStatus < types.InOutDefault {
			continue
		}
		v, err := LumpSumValue(ls.Amt, ls.Date, asof, rate, settings.Basis, settings.YrInv)
		if err != nil {
			return 0, err
		}
		sum += v
	}
	for j := range input.Periodics {
		pp := &input.Periodics[j]
		if !isLumpSum && j == unknownIdx {
			continue
		}
		if pp.FromDateStatus < types.InOutDefault ||
			pp.ToDateStatus < types.InOutDefault ||
			pp.AmtStatus < types.InOutDefault {
			continue
		}
		cola := pp.COLA
		if pp.COLAStatus < types.InOutDefault {
			cola = 0
		}
		nInst := pp.NInstallments
		if nInst <= 0 {
			nInst = estimateInstallments(pp.FromDate, pp.ToDate, pp.PerYr)
		}
		factor, err := PeriodicSummation(rate, cola, asof, pp.FromDate, pp.ToDate,
			pp.PerYr, nInst, settings)
		if err != nil {
			return 0, err
		}
		sum += pp.Amt * factor
	}
	return sum, nil
}

// solveLumpAmount handles PV-1: lump-sum row has date and value, solve
// for amount = value * exp(rate * yearsDif(date, asof)).
//
// Ported from legacy/src/dos_source/PRESVALU.pas:866-891.
func solveLumpAmount(input *PVInput, result *PVResult, idx int) {
	rate := input.PresVal.R.Rate
	asof := input.PresVal.AsOf
	settings := &input.Settings

	known, err := computeKnownRowSum(input, true, idx)
	if err != nil {
		result.Err = err
		return
	}
	target := input.PresVal.SumValue - known

	ls := &result.LumpSums[idx]
	// In DOS, "value" of an unknown row is the residual sumvalue; here we
	// treat user-supplied ls.Val as the row's value if present, else the
	// implied residual.
	rowValue := target
	if ls.ValStatus >= types.InOutDefault {
		rowValue = ls.Val
	}
	years := dateutil.YearsDif(ls.Date, asof, settings.Basis, settings.YrInv, false)
	exprt, err := interest.Exxp(rate * years)
	if err != nil {
		result.Err = err
		return
	}
	ls.Amt = rowValue * exprt
	ls.AmtStatus = types.InOutOutput
	ls.Val = rowValue
	ls.ValStatus = types.InOutOutput
	result.SumValue = input.PresVal.SumValue
}

// solveLumpDate handles PV-2: lump-sum row has amount and value, solve
// for the date via Newton-Raphson on yrs = ln(value/amount)/rate, then
// AddYears.
//
// Iteration mirrors PRESVALU.pas:892-931:
//   - guard against opposite signs (value/amount sign mismatch)
//   - guess starts at asof, count <= 30
//   - convergence: |diff| < 0.003 (one day)
//   - cap diff in (-20, 20) years per step
//
// Ported from legacy/src/dos_source/PRESVALU.pas:892-931.
func solveLumpDate(input *PVInput, result *PVResult, idx int) {
	rate := input.PresVal.R.Rate
	asof := input.PresVal.AsOf
	settings := &input.Settings

	if math.Abs(rate) < types.Teeny {
		result.Err = fmt.Errorf(
			"cannot compute date - interest rate too small (line %d)", idx+1)
		return
	}

	known, err := computeKnownRowSum(input, true, idx)
	if err != nil {
		result.Err = err
		return
	}
	rowValue := input.PresVal.SumValue - known
	ls := &result.LumpSums[idx]
	if ls.ValStatus >= types.InOutDefault {
		rowValue = ls.Val
	}

	if (rowValue > 0) != (ls.Amt > 0) {
		result.Err = fmt.Errorf(
			"value and amount must have the same sign (line %d)", idx+1)
		return
	}

	// First guess: payment occurs at asof.
	wdate := asof
	count := 0
	for {
		count++
		if count > 30 {
			result.Err = fmt.Errorf(
				`"date" computation did not converge, line %d`, idx+1)
			return
		}
		yrs := dateutil.YearsDif(asof, wdate, settings.Basis, settings.YrInv, false)
		exprt, err := interest.Exxp(rate * yrs)
		if err != nil {
			result.Err = err
			return
		}
		val := ls.Amt * exprt
		dval := ls.Amt * rate * exprt
		if math.Abs(dval) < types.Teeny {
			result.Err = fmt.Errorf("date computation diverged, line %d", idx+1)
			return
		}
		diff := (rowValue - val) / dval
		if diff > 20 {
			diff = 20
		} else if diff < -20 {
			diff = -20
		}
		wdate, err = dateutil.AddYears(wdate, -diff, settings.Basis, settings.YrDays)
		if err != nil {
			result.Err = err
			return
		}
		if math.Abs(diff) < 0.003 {
			break
		}
	}

	ls.Date = wdate
	ls.DateStatus = types.InOutOutput
	ls.Val = rowValue
	ls.ValStatus = types.InOutOutput
	result.SumValue = input.PresVal.SumValue
}

// solvePeriodicAmount handles PV-4: periodic row has both dates and
// value, solve for amount = value / Summation.
//
// Ported from legacy/src/dos_source/PRESVALU.pas:943-956.
func solvePeriodicAmount(input *PVInput, result *PVResult, idx int) {
	rate := input.PresVal.R.Rate
	asof := input.PresVal.AsOf
	settings := &input.Settings

	known, err := computeKnownRowSum(input, false, idx)
	if err != nil {
		result.Err = err
		return
	}
	rowValue := input.PresVal.SumValue - known

	pp := &result.Periodics[idx]
	if pp.ValStatus >= types.InOutDefault {
		rowValue = pp.Val
	}

	cola := pp.COLA
	if pp.COLAStatus < types.InOutDefault {
		cola = 0
	}
	nInst := pp.NInstallments
	if nInst <= 0 {
		nInst = estimateInstallments(pp.FromDate, pp.ToDate, pp.PerYr)
		pp.NInstallments = nInst
	}
	factor, err := PeriodicSummation(rate, cola, asof, pp.FromDate, pp.ToDate,
		pp.PerYr, nInst, settings)
	if err != nil {
		result.Err = err
		return
	}
	if math.Abs(factor) < types.Teeny {
		result.Err = fmt.Errorf("summation factor too small (line %d)", idx+1)
		return
	}
	pp.Amt = rowValue / factor
	pp.AmtStatus = types.InOutOutput
	pp.Val = rowValue
	pp.ValStatus = types.InOutOutput
	result.SumValue = input.PresVal.SumValue
}

// solvePeriodicDate handles PV-5 (toDate unknown) and PV-6 (fromDate
// unknown). Uses the closed-form approximation
//
//	yrs = (-1/(r-cola)) * ln( first ) ;  fromDate via similar derivation
//
// then refines by stepping ±1 period until error stops decreasing.
//
// PV-5: PRESVALU.pas:965-999.
// PV-6: PRESVALU.pas:1000-1070.
//
// TODO: verify logic — DOS has a {$ifdef V_3} block that subtracts cola
// from rate and zeros cola when colastatus = const_signal; we don't
// reproduce that here because const_signal is not yet wired through.
// TODO: verify logic — second-pass refinement when cola != 0
// re-uses the first answer; current implementation only does the first
// pass plus the ±1 period refinement loop. Confirm behavioral parity
// against legacy/reference-output/ once available.
func solvePeriodicDate(input *PVInput, result *PVResult, idx int, solveTo bool) {
	rate := input.PresVal.R.Rate
	asof := input.PresVal.AsOf
	settings := &input.Settings

	known, err := computeKnownRowSum(input, false, idx)
	if err != nil {
		result.Err = err
		return
	}
	target := input.PresVal.SumValue - known
	pp := &result.Periodics[idx]
	if pp.ValStatus >= types.InOutDefault {
		target = pp.Val
	}

	if (target > 0) != (pp.Amt > 0) {
		result.Err = fmt.Errorf(
			"value and amount must have the same sign (line %d)", idx+1)
		return
	}

	cola := pp.COLA
	if pp.COLAStatus < types.InOutDefault {
		cola = 0
	}
	rpy := interest.RealPerYr(byte(pp.PerYr), settings.YrDays)
	f, err := interest.Exxp((cola - rate) / rpy)
	if err != nil {
		result.Err = err
		return
	}

	if solveTo {
		// PV-5: solve for toDate.
		first, err := interest.Exxp(rate * dateutil.YearsDif(asof, pp.FromDate,
			settings.Basis, settings.YrInv, false))
		if err != nil {
			result.Err = err
			return
		}
		last := (first - (1-f)*target/pp.Amt) / f
		toDate := pp.FromDate
		if math.Abs(rate-cola) < types.Teeny {
			n := int(math.Round(target / pp.Amt))
			toDate, err = dateutil.AddNPeriods(pp.FromDate, pp.PerYr, n)
		} else {
			lnLast, lerr := interest.Lnn(last * f / first)
			if lerr != nil {
				err = lerr
			} else {
				// PRESVALU.pas:970 — note the leading minus sign:
				//   AddYears(todate, -(lnn(last*f/first)/(r.rate-cola) + 1/RealPerYr(peryr)))
				yrs := -(lnLast/(rate-cola) + 1.0/rpy)
				toDate, err = dateutil.AddYears(toDate, yrs, settings.Basis, settings.YrDays)
			}
		}
		if err != nil {
			result.Err = err
			return
		}
		pp.ToDate = refinePeriodicDate(input, pp, idx, toDate, target, true, settings)
		pp.ToDateStatus = types.InOutOutput
	} else {
		// PV-6: solve for fromDate.
		// First approximation - if toDate is "latest", use simpler form.
		latest := types.LatestDate()
		var fromDate types.DateRec
		if pp.ToDate.Time.Equal(latest.Time) {
			firstTerm := (1 - f) * target / pp.Amt
			lnFirst, lerr := interest.Lnn(firstTerm)
			if lerr != nil {
				result.Err = lerr
				return
			}
			yrs := (-1.0 / (rate - cola)) * lnFirst
			fromDate, err = dateutil.AddYears(asof, yrs, settings.Basis, settings.YrDays)
			if err != nil {
				result.Err = err
				return
			}
		} else {
			last, err := interest.Exxp((rate - cola) * dateutil.YearsDif(asof, pp.ToDate,
				settings.Basis, settings.YrInv, false))
			if err != nil {
				result.Err = err
				return
			}
			firstTerm := f*last + (1-f)*target/pp.Amt
			if firstTerm <= 0 || math.Abs(rate-cola) < types.Teeny {
				// PUNT path - PRESVALU.pas:1020 - rare case where cola>rate.
				// Fall back to toDate as the starting point.
				fromDate = pp.ToDate
			} else {
				lnRatio, err := interest.Lnn(last * f / firstTerm)
				if err != nil {
					result.Err = err
					return
				}
				fromDate = pp.ToDate
				fromDate, err = dateutil.AddYears(fromDate,
					lnRatio/(rate-cola)+1.0/rpy, settings.Basis, settings.YrDays)
				if err != nil {
					result.Err = err
					return
				}
			}
		}
		pp.FromDate = refinePeriodicDate(input, pp, idx, fromDate, target, false, settings)
		pp.FromDateStatus = types.InOutOutput
	}

	// Final amount correction within 1 cent (PRESVALU.pas:1072-1078).
	cola = pp.COLA
	if pp.COLAStatus < types.InOutDefault {
		cola = 0
	}
	pp.NInstallments = estimateInstallments(pp.FromDate, pp.ToDate, pp.PerYr)
	factor, err := PeriodicSummation(rate, cola, asof, pp.FromDate, pp.ToDate,
		pp.PerYr, pp.NInstallments, settings)
	if err != nil {
		result.Err = err
		return
	}
	valn := pp.Amt * factor
	if math.Abs(valn) > types.Small {
		corAmt := pp.Amt * (target / valn)
		if math.Abs(corAmt-pp.Amt) > 0.01 {
			pp.Amt = corAmt
			pp.AmtStatus = types.InOutDefault
			valn = target
		}
	}
	pp.Val = valn
	pp.ValStatus = types.InOutOutput
	result.SumValue = input.PresVal.SumValue
}

// refinePeriodicDate steps the candidate date ±1 period at a time until
// the resulting periodic value moves *away* from the target, then backs
// off by one. Mirrors the "alternate try" loops in PRESVALU.pas:976-987
// and 1047-1062.
func refinePeriodicDate(input *PVInput, pp *PeriodicPayment, idx int,
	candidate types.DateRec, target float64, solveTo bool, settings *PVSettings) types.DateRec {

	rate := input.PresVal.R.Rate
	asof := input.PresVal.AsOf
	cola := pp.COLA
	if pp.COLAStatus < types.InOutDefault {
		cola = 0
	}

	originalTo, originalFrom := pp.ToDate, pp.FromDate

	calc := func(from, to types.DateRec) float64 {
		n := estimateInstallments(from, to, pp.PerYr)
		factor, err := PeriodicSummation(rate, cola, asof, from, to, pp.PerYr, n, settings)
		if err != nil {
			return math.NaN()
		}
		return pp.Amt * factor
	}

	var altDate types.DateRec
	var subtract bool
	if solveTo {
		altDate = candidate
		val := calc(originalFrom, candidate)
		altVal := val
		// decide direction
		subtract = (target < val) != (pp.Amt < 0)
		for i := 0; i < 60; i++ {
			next, err := dateutil.AddPeriod(altDate, pp.PerYr,
				originalFrom.Time.Day(), subtract)
			if err != nil {
				break
			}
			newVal := calc(originalFrom, next)
			if math.IsNaN(newVal) {
				break
			}
			if math.Abs(target-altVal) < math.Abs(target-newVal) {
				return altDate
			}
			altDate = next
			altVal = newVal
		}
		return altDate
	}

	// solve fromDate
	altDate = candidate
	val := calc(candidate, originalTo)
	altVal := val
	subtract = (target > val) != (pp.Amt < 0)
	for i := 0; i < 60; i++ {
		next, err := dateutil.AddPeriod(altDate, pp.PerYr,
			candidate.Time.Day(), subtract)
		if err != nil {
			break
		}
		newVal := calc(next, originalTo)
		if math.IsNaN(newVal) {
			break
		}
		if math.Abs(target-altVal) < math.Abs(target-newVal) {
			return altDate
		}
		altDate = next
		altVal = newVal
	}
	return altDate
}

// solveRate handles PV-8: rate is unknown. Newton iteration on rate,
// damped at ±0.04 per step, with second-pass restart from rate=0 if
// the first pass fails to converge in 30 iterations.
//
// Ported from legacy/src/dos_source/PRESVALU.pas:693-754.
func solveRate(input *PVInput, result *PVResult) {
	asof := input.PresVal.AsOf
	settings := &input.Settings
	target := input.PresVal.SumValue

	guess := 0.1
	secondTime := false
	for attempt := 0; attempt < 2; attempt++ {
		rate := guess
		var oldSum, diff float64
		count := 0
		for {
			count++
			if count > 30 {
				if secondTime {
					result.Err = fmt.Errorf(`"rate" computation did not converge`)
					return
				}
				secondTime = true
				guess = 0
				break // will restart outer loop with rate=0
			}
			sum, err := evaluatePVAt(input, rate, asof, settings)
			if err != nil {
				result.Err = err
				return
			}
			denom := sum - oldSum
			if math.Abs(denom) < types.Teeny {
				denom = types.Teeny
			}
			if count == 1 {
				diff = 0.001
			} else {
				diff = (target - sum) * diff / denom
			}
			if count == 2 && diff == 0 {
				result.Err = fmt.Errorf(
					"rate is not determined - specify amounts instead of values")
				return
			}
			oldSum = sum
			if diff < -0.04 {
				diff = -0.04
			} else if diff > 0.04 {
				diff = 0.04
			}
			rate = rate - diff
			if math.Abs(diff) < types.Teeny {
				// converged.
				input.PresVal.R.Rate = rate
				input.PresVal.R.Status = types.StatusFromCalc
				// regenerate row values with the solved rate.
				fwResult := frontwardCompute(input)
				*result = fwResult
				return
			}
		}
		if !secondTime {
			break
		}
	}
}

// solveAsOf handles PV-9: as-of date is unknown. PRESVALU.pas:755-818.
//
// Iteration uses asof = asof + ln(target/sum)/rate; converges in ~2
// passes per the DOS comment "doesn't really require iteration".
func solveAsOf(input *PVInput, result *PVResult) {
	rate := input.PresVal.R.Rate
	settings := &input.Settings
	target := input.PresVal.SumValue

	if math.Abs(rate) < types.Teeny {
		result.Err = fmt.Errorf("cannot compute date - interest rate too small")
		return
	}

	// First guess: 1900-01-01 (DOS uses pascal year 100 = year 0 = 1900-01-01).
	asof := types.NewDateRec(1900, 1, 1)
	maxDate := types.LatestDate()
	for count := 0; count < 10; count++ {
		sum, err := evaluatePVAt(input, rate, asof, settings)
		if err != nil {
			result.Err = err
			return
		}
		if math.Abs(sum) < types.Teeny {
			result.Err = fmt.Errorf("cannot solve as-of date - sum is zero")
			return
		}
		ratio := target / sum
		if ratio <= 0 {
			result.Err = fmt.Errorf("cannot solve as-of date - sign mismatch")
			return
		}
		ln, err := interest.Lnn(ratio)
		if err != nil {
			result.Err = err
			return
		}
		diff := ln / rate
		newAsof, err := dateutil.AddYears(asof, diff, settings.Basis, settings.YrDays)
		if err != nil {
			result.Err = err
			return
		}
		asof = newAsof
		if math.Abs(diff) < 0.002 {
			break
		}
		if dateutil.DateComp(asof, maxDate) > 0 {
			result.Err = fmt.Errorf(`"as of" computation did not converge`)
			return
		}
	}

	input.PresVal.AsOf = asof
	input.PresVal.AsOfStatus = types.InOutOutput
	*result = frontwardCompute(input)
	result.SumValue = target
}

// evaluatePVAt computes the total present value of all populated rows
// at a candidate (rate, asof) without mutating input. Used by the
// rate and as-of solvers.
func evaluatePVAt(input *PVInput, rate float64, asof types.DateRec,
	settings *PVSettings) (float64, error) {

	var sum float64
	for i := range input.LumpSums {
		ls := &input.LumpSums[i]
		if ls.DateStatus < types.InOutDefault || ls.AmtStatus < types.InOutDefault {
			continue
		}
		v, err := LumpSumValue(ls.Amt, ls.Date, asof, rate, settings.Basis, settings.YrInv)
		if err != nil {
			return 0, err
		}
		sum += v
	}
	for j := range input.Periodics {
		pp := &input.Periodics[j]
		if pp.FromDateStatus < types.InOutDefault ||
			pp.ToDateStatus < types.InOutDefault ||
			pp.AmtStatus < types.InOutDefault {
			continue
		}
		cola := pp.COLA
		if pp.COLAStatus < types.InOutDefault {
			cola = 0
		}
		nInst := pp.NInstallments
		if nInst <= 0 {
			nInst = estimateInstallments(pp.FromDate, pp.ToDate, pp.PerYr)
		}
		factor, err := PeriodicSummation(rate, cola, asof, pp.FromDate, pp.ToDate,
			pp.PerYr, nInst, settings)
		if err != nil {
			return 0, err
		}
		sum += pp.Amt * factor
	}
	return sum, nil
}

// frontwardCompute is the existing forward path extracted to a helper
// so backward solvers can re-run it once they've nailed down the
// missing input. Mirrors PRESVALU.pas: FrontwardCalc post-solve refresh.
func frontwardCompute(input *PVInput) PVResult {
	// The existing public Calculate runs the forward path when rate +
	// asof are present; reuse it to avoid duplicate logic.
	cp := *input
	cp.PresVal.SumValueStatus = types.StatusEmpty // force recompute
	return forwardOnly(cp)
}
