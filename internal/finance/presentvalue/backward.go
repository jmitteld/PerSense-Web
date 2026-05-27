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
	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// lumpRowPV returns a lump-sum row's present value at (rate, asof),
// weighted by the survival probability when the row is
// life-contingent — matching the forward path (calc.go:331).
func lumpRowPV(ls *LumpSumPayment, asof types.DateRec, rate float64,
	settings *PVSettings, actu *actuarial.ActuarialConfig) (float64, error) {
	v, err := LumpSumValue(ls.Amt, ls.Date, asof, rate, settings.Basis, settings.YrInv)
	if err != nil {
		return 0, err
	}
	if actu != nil && ls.Act != actuarial.NotContingent {
		v *= actu.LifeProb(ls.Date, ls.Act)
	}
	return v, nil
}

// periodicRowPV returns a periodic row's present value at (rate,
// asof), weighted by survival probability when life-contingent.
func periodicRowPV(pp *PeriodicPayment, asof types.DateRec, rate float64,
	settings *PVSettings, actu *actuarial.ActuarialConfig) (float64, error) {
	cola := pp.COLA
	if pp.COLAStatus < types.InOutDefault {
		cola = 0
	}
	nInst := pp.NInstallments
	if nInst <= 0 {
		nInst = estimateInstallments(pp.FromDate, pp.ToDate, pp.PerYr)
	}
	if actu != nil && pp.Act != actuarial.NotContingent {
		val, _ := periodicWithActuarial(rate, cola, asof, pp.FromDate, pp.ToDate,
			pp.PerYr, nInst, settings, actu, pp.Act)
		return val, nil
	}
	factor, err := PeriodicSummation(rate, cola, asof, pp.FromDate, pp.ToDate,
		pp.PerYr, nInst, settings)
	if err != nil {
		return 0, err
	}
	return pp.Amt * factor, nil
}

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
	// Warnings carries non-fatal advisories — e.g. an over-specified
	// row whose redundant Value will be recomputed. DOS surfaces these
	// as cancelable warnings (PRESVALU.pas:1166-1189); the port
	// continues the calculation and reports them alongside the result.
	Warnings []string
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
					"the dates are out of order on periodic payment line %d: the From Date must come before the To Date. Swap the two dates, or check the \"Yr to divide century\" setting if a two-digit year landed in the wrong century",
					j+1)
				return res
			}
		}

		// Periodic classification (PRESVALU.pas:594-629). The row is
		// fully_specified when From + To + Amount are all present
		// (forward computes Value). When Value is present and exactly
		// one of {From, To, Amount} is missing, the row is
		// contains_unknown — the row-level backward path solves the
		// missing field (PV-4 / PV-5 / PV-6). Otherwise the row is
		// underspecified.
		hasFrom := b.FromDateStatus >= types.InOutDefault
		hasTo := b.ToDateStatus >= types.InOutDefault
		hasAmt := b.AmtStatus >= types.InOutDefault
		hasVal := b.ValStatus >= types.InOutDefault
		coreCount := 0
		if hasFrom {
			coreCount++
		}
		if hasTo {
			coreCount++
		}
		if hasAmt {
			coreCount++
		}
		var status byte
		switch coreCount {
		case 3:
			// Full forward inputs. If Value is also given, the row is
			// over-specified — surface a soft warning (DOS
			// PRESVALU.pas:1166-1189) and proceed.
			status = types.LineFullySpecified
			if hasVal {
				res.Warnings = append(res.Warnings, fmt.Sprintf(
					"periodic payment row %d is over-specified - the supplied "+
						"Value is redundant and will be recomputed from the "+
						"dates and amount", j+1))
			}
		case 2:
			// Exactly one of {From, To, Amount} missing. The row-
			// level backward path solves it when Value is supplied;
			// without Value the dispatcher records an error below.
			status = types.LineContainsUnknown
		case 1, 0:
			// Two or more of the core inputs missing — not enough
			// data even with Value present. Fall through with the
			// decremented status so the existing missing-2 / missing-3
			// flags drive the same error messages.
			status = types.LineFullySpecified
			if !hasFrom {
				status--
			}
			if !hasTo {
				status--
			}
			if !hasAmt && !hasVal {
				status--
			}
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
				res.Err = fmt.Errorf("the amount cannot be zero on periodic payment "+
					"line %d when it is the only field given — Per%%Sense would have "+
					"nothing to solve from. Enter a non-zero Amount, or fill in "+
					"the dates and Value and leave the Amount blank to solve for it",
					j+1)
				return res
			}
			if b.ValStatus >= types.InOutDefault && b.Val == 0 {
				res.Err = fmt.Errorf("the value cannot be zero on periodic payment "+
					"line %d when it is the only field given — Per%%Sense would have "+
					"nothing to solve from. Enter a non-zero Value, or fill in "+
					"the dates and Amount and leave the Value blank to solve for it",
					j+1)
				return res
			}
		}
		res.PeriodicStatus[j] = status
		b.Status = int(status)
	}

	// BLOCK 1 - LumpSum block. Each row is fully_specified when it has
	// Date AND Amount (forward computes Value); contains_unknown when
	// exactly one of {Date, Amount} is missing and a target Value is
	// supplied (PV-1 / PV-2); else blank or contains_unknown.
	// PRESVALU.pas inlines this into ComputeLumpsumLineValues.
	for i := range input.LumpSums {
		a := &input.LumpSums[i]
		hasDate := a.DateStatus >= types.InOutDefault
		hasAmt := a.AmtStatus >= types.InOutDefault
		hasVal := a.ValStatus >= types.InOutDefault
		nFields := 0
		if hasDate {
			nFields++
		}
		if hasAmt {
			nFields++
		}
		if hasVal {
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
				res.Err = fmt.Errorf("the amount cannot be zero on single payment "+
					"line %d when it is the only field given — Per%%Sense would "+
					"have nothing to solve from. Enter a non-zero Amount, or fill "+
					"in the Date and Value and leave the Amount blank to solve for it",
					i+1)
				return res
			}
			if a.ValStatus >= types.InOutDefault && a.Val == 0 {
				res.Err = fmt.Errorf("the value cannot be zero on single payment "+
					"line %d when it is the only field given — Per%%Sense would "+
					"have nothing to solve from. Enter a non-zero Value, or fill "+
					"in the Date and Amount and leave the Value blank to solve for it",
					i+1)
				return res
			}
			status = types.LineContainsUnknown
		case 2:
			// Date + Amount is the forward path (engine computes
			// Value). Date + Value (PV-1) or Amount + Value (PV-2)
			// is the row-level backward path — the engine treats
			// Value as the target and solves the missing field.
			// DOS PRESVALU.pas:866-931 — BackwardCalc lumpsum arms.
			if hasDate && hasAmt {
				status = types.LineFullySpecified
			} else {
				status = types.LineContainsUnknown
			}
		default:
			// Lump sum row over-specified — date+amt+val all present.
			// Soft warning, not a hard error: DOS PRESVALU.pas:1166-1189
			// records a cancelable warning ("value already determined
			// by data above") and proceeds, treating the row as fully
			// specified from the date + amount and recomputing the
			// redundant Value. See dispatch_gaps §4.7 PV-warning.
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"single payment row %d is over-specified - the supplied "+
					"Value is redundant and will be recomputed from the "+
					"date and amount", i+1))
			status = types.LineFullySpecified
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

	// Backward dispatch. There are two ways a row-level backward can
	// fire — either a screen-level Sum Value is given (DOS PV-8 / PV-9
	// where the missing row is whatever residual makes the screen
	// total balance), or an individual row supplies its own target
	// Value and is missing one of {Date, Amount} for lumps or one of
	// {From, To, Amount} for periodics (DOS PV-1 / PV-2 / PV-4 / PV-5
	// / PV-6, where the row's own Value is the target).
	hasSumValue := pv.SumValueStatus >= types.InOutDefault
	rowLevelTarget := func() bool {
		if hasSumValue {
			return true
		}
		for i := range input.LumpSums {
			if res.LumpSumStatus[i] == types.LineContainsUnknown &&
				input.LumpSums[i].ValStatus >= types.InOutDefault {
				return true
			}
		}
		for j := range input.Periodics {
			if res.PeriodicStatus[j] == types.LineContainsUnknown &&
				input.Periodics[j].ValStatus >= types.InOutDefault {
				return true
			}
		}
		return false
	}()
	if rowLevelTarget {
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
					"single payment line %d has only a Value filled in, which is not "+
						"enough to solve. Add either the Date or the Amount: with the "+
						"Amount, Per%%Sense solves for the Date; with the Date, it "+
						"solves for the Amount",
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
					"periodic payment line %d does not have enough filled in to solve. "+
						"Per%%Sense can solve for one missing field only: fill in the "+
						"Amount plus one date (From Date or To Date) and it solves for "+
						"the other date, or fill in both dates and leave the Amount "+
						"blank to solve for the Amount",
					j+1)
				return res
			}
		}
		// Cases B / C only apply when the screen Sum Value is given —
		// PV-8 and PV-9 solve the rate / as-of date that makes the
		// screen total match SumValue. Per-row Value targets don't
		// drive a screen-level rate/asof solve.
		if hasSumValue {
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
		result.Err = fmt.Errorf("there is not enough information on the screen to solve for the missing field. Supply the target Present Value, and leave exactly one field blank — the one you want Per%%Sense to compute")
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
	// Echo the rate and as-of date actually used. The rate / as-of
	// solvers mutate input.PresVal in place, so for PV-8 and PV-9 this
	// carries the solved value back to the caller.
	result.Rate = input.PresVal.R.Rate
	result.AsOf = input.PresVal.AsOf
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
		v, err := lumpRowPV(ls, asof, rate, settings, input.Actuarial)
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
		v, err := periodicRowPV(pp, asof, rate, settings, input.Actuarial)
		if err != nil {
			return 0, err
		}
		sum += v
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
	// Life-contingent row: the forward path scales the value by the
	// survival probability, so solving the amount divides it back out
	// (DOS PRESVALU.pas:873-883).
	if input.Actuarial != nil && ls.Act != actuarial.NotContingent {
		prob := input.Actuarial.LifeProb(ls.Date, ls.Act)
		if prob > types.Teeny {
			ls.Amt /= prob
		}
	}
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
			"cannot solve for the Date on single payment line %d because the Rate "+
				"is too small (at or near zero): with no interest, the Amount and "+
				"Value are equal at every date, so the Date is undetermined. Enter a "+
				"non-zero Rate, or fill in the Date and leave the Amount blank instead",
			idx+1)
		return
	}

	// A life-contingent row cannot have its date solved: the survival
	// probability is itself a function of the unknown date, so the
	// equation is not invertible. DOS rejects this outright with
	// "no_time_with_life" (PRESVALU.pas:894-897).
	if input.Actuarial != nil && idx < len(input.LumpSums) &&
		input.LumpSums[idx].Act != actuarial.NotContingent {
		result.Err = fmt.Errorf(
			"cannot solve for the Date on single payment line %d because it is a "+
				"life-contingency payment: the survival probability itself depends "+
				"on the date, so the date cannot be worked out from the Value. Fill "+
				"in the Date and leave the Amount blank to solve for the Amount instead",
			idx+1)
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
			"the Value and the Amount on single payment line %d have opposite "+
				"signs, so no date can make them consistent — discounting can "+
				"shrink or grow a payment but cannot flip it from positive to "+
				"negative. Make the Value and Amount both positive (or both "+
				"negative) so they agree in sign",
			idx+1)
		return
	}

	// First guess: payment occurs at asof.
	wdate := asof
	count := 0
	for {
		count++
		if count > 30 {
			result.Err = fmt.Errorf(
				`the "date" computation for single payment line %d did not converge `+
					`on an answer. The Value may be unreachable for this Amount and `+
					`Rate — check that the Value and Amount are sensible, or fill in `+
					`the Date and leave the Amount blank to solve for the Amount instead`,
				idx+1)
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
			result.Err = fmt.Errorf(
				"the Date computation for single payment line %d could not proceed "+
					"because the calculation stalled (the Amount or Rate gives no "+
					"sensitivity to move the date). Check that the Amount and Rate "+
					"are non-zero and sensible, or fill in the Date and solve for "+
					"the Amount instead",
				idx+1)
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
		result.Err = fmt.Errorf(
			"cannot solve for the Amount on periodic payment line %d: the present "+
				"value of the payment stream works out to essentially zero, so "+
				"dividing the target Value by it gives no answer. Check the From "+
				"Date, To Date, Pmts-Yr and Rate on that row",
			idx+1)
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
// The DOS {$ifdef V_3} const_signal block is intentionally not
// reproduced — V_3 is never defined in the authoritative DOS build,
// so that block is dead code (see docs/dispatch_gaps.md §0.5.5).
//
// The cola != 0 second approximation (PRESVALU.pas:1029-1035) IS
// implemented for the from-date solve, followed by the ±1 period
// refinement loop.
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
			"the Value and the Amount on periodic payment line %d have opposite "+
				"signs, so no From/To Date can make them agree — a stream of "+
				"positive payments cannot have a negative present value. Make the "+
				"Value and Amount both positive (or both negative) so they match "+
				"in sign",
			idx+1)
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
		// Second approximation (PRESVALU.pas:1029-1035): with a
		// non-zero COLA the discount factor changes at fromDate — the
		// rate applies asof->fromDate and (rate-cola) applies
		// fromDate->toDate. Refine the first estimate accordingly.
		if cola != 0 && math.Abs(rate-cola) >= types.Teeny {
			lA, e1 := interest.Exxp(rate * dateutil.YearsDif(asof, fromDate,
				settings.Basis, settings.YrInv, false))
			lB, e2 := interest.Exxp((rate - cola) * dateutil.YearsDif(fromDate, pp.ToDate,
				settings.Basis, settings.YrInv, false))
			if e1 == nil && e2 == nil {
				last2 := lA * lB
				first2 := f*last2 + (1-f)*target/pp.Amt
				if first2 > 0 {
					if lnRatio2, e3 := interest.Lnn(last2 * f / first2); e3 == nil {
						if nfd, e4 := dateutil.AddYears(pp.ToDate,
							lnRatio2/(rate-cola)+1.0/rpy, settings.Basis,
							settings.YrDays); e4 == nil {
							fromDate = nfd
						}
					}
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
					result.Err = fmt.Errorf(`the "rate" computation did not converge ` +
						`on an answer — no single interest rate makes the payments add ` +
						`up to the Present Value you entered. Check that the target ` +
						`Present Value is reachable from the payments, or fill in the ` +
						`Rate and leave the As-of Date or a payment amount blank instead`)
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
					"the Rate is not determined by what is on the screen: the payment " +
						"rows give Per%%Sense no way to pin down a single interest rate. " +
						"Enter the Amount on each payment row (rather than only its " +
						"Value), or fill in the Rate directly")
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
		result.Err = fmt.Errorf(
			"cannot solve for the As-of Date because the Rate is too small (at or " +
				"near zero): with no interest, the present value is the same at " +
				"every date, so the As-of Date is undetermined. Enter a non-zero " +
				"Rate, or fill in the As-of Date and leave the Rate blank instead")
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
			result.Err = fmt.Errorf(
				"cannot solve for the As-of Date because the payments add up to a " +
					"present value of zero, leaving no date to discount toward. " +
					"Check the payment rows, or fill in the As-of Date and leave " +
					"the Rate blank instead")
			return
		}
		ratio := target / sum
		if ratio <= 0 {
			result.Err = fmt.Errorf(
				"cannot solve for the As-of Date because the target Present Value " +
					"has the opposite sign of the payments — no As-of Date can turn " +
					"a positive set of payments into a negative Present Value. Make " +
					"the Present Value match the sign of the payments")
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
			result.Err = fmt.Errorf(`the "as of" computation did not converge ` +
				`on an answer — no As-of Date gives the Present Value you entered ` +
				`(the date ran past the supported range). Check that the target ` +
				`Present Value is reachable from the payments at this Rate, or fill ` +
				`in the As-of Date and leave the Rate blank instead`)
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
		v, err := lumpRowPV(ls, asof, rate, settings, input.Actuarial)
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
		v, err := periodicRowPV(pp, asof, rate, settings, input.Actuarial)
		if err != nil {
			return 0, err
		}
		sum += v
	}
	// POD (payment-on-death) value folds into the total, matching the
	// forward path (calc.go:392) — DOS PRESVALU.pas:689.
	if input.Actuarial != nil && input.Actuarial.POD != 0 {
		sum += input.Actuarial.PODValue(asof, rate)
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
