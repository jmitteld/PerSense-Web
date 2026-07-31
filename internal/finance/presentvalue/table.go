package presentvalue

// The Present Value payment table — a direct port of the DOS Ctrl-T table,
// legacy/src/dos_source/pvltable.pas (MakePVLTable and its nested helpers),
// invoked from PRESVALU.pas MakeTable. "The table contains a line for each
// future payment, the amount of that payment, and its contribution to the
// present value" (win help PV_Tables.html).
//
// Structure mirrors the Pascal:
//
//	SortPayments      → sortTableStreams (sort lumps, compact periodics,
//	                    init COLA walkers, 50-year forever cutoff)
//	SelectNextPayment → nextTablePayment (min-date merge; same-date combine —
//	                    EXCEPT in life mode; per-payment COLA update)
//	TimeIsRipe        → subtotalDue (month-set crossing since last boundary)
//	PrintNextPayment / PrintSummary / PrintPOD / GrandTotals → row emitters
//
// Fidelity notes carried over from the Pascal:
//
//   - The table values each payment on its ACTUAL date — even where the screen
//     used the equal-spacing closed form — so on the 365 basis with monthly
//     payments the table's Cum Value can differ slightly from the screen's
//     Present Value. This is documented behavior ("the answer at the bottom of
//     the table is slightly more accurate than the answer on the screen",
//     PV_Tables.html), NOT a bug to reconcile.
//   - Same-date payments combine into one line (amounts summed) — except under
//     a life contingency, where each source payment keeps its own line because
//     the contingencies may differ (pvltable.pas SelectNextPayment
//     fold_in_life early-exits).
//   - A "forever" periodic row (ToDate in the 2149 sentinel year) is cut at 50
//     years for the table only: the DOS code replaces todate.y with
//     fromdate.y+50, KEEPING todate's month/day (pvltable.pas SortPayments).
//   - Summary-only lines are dated with the LAST payment date of the closed
//     period (DOS `oldt`), not the period boundary.
//   - Life columns: "Value if Paid" is the discounted value ignoring survival
//     (DOS ifpd); Probability is displayed as v/ifpd (recovered, 4dp); the
//     grand-total probability is Σv/Σifpd.
//   - The v3 product build (-dV_3;SCROLLS;PVLX — no ACTU) compiles the table
//     with fold_in_life constant FALSE, so only the standard columns are
//     oracle-checkable. The life columns below port the DOS source's ACTU
//     branches and are validated by internal consistency with the screen's
//     actuarial engine (which is itself oracle-less — see
//     claude/pv_logic_audit docs). One deliberate divergence from the ACTU
//     source: DOS PrintPOD sets `total.ifpd := total.v+podval` (overwriting
//     the accumulated if-paid total with an obviously-corrupt value, a
//     latent bug in the never-shipped branch); we accumulate
//     `total.ifpd += podval` so the grand-total probability stays meaningful.

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// Table detail levels — DOS `cum`: ' ' detail only, 'A'..'Z' detail+summary,
// lowercase summary only (TableOptionsDlgUnit.pas OKBtnClick).
const (
	TableDetailOnly = "detail"
	TableDetailBoth = "both"
	TableSummary    = "summary"
)

// TableRequest selects the view. SummaryMonths is DOS `cumset` — the calendar
// months whose boundary triggers a subtotal (index 1..12); empty = never
// (detail-only behavior regardless of Detail).
type TableRequest struct {
	Detail        string
	SummaryMonths [13]bool
}

// Row kinds.
const (
	TableRowPayment  = "payment"
	TableRowSubtotal = "subtotal"
	TableRowPOD      = "pod"
)

// TableRow is one output line. For life mode, IfPaid/Prob/Contingency are
// populated and CumValue is unused (the DOS life table has no Cum Value
// column). For subtotal rows Payment/Value are the period subtotals and
// CumValue the running total.
type TableRow struct {
	Kind        string
	Date        types.DateRec
	HasDate     bool
	Payment     float64
	Value       float64
	CumValue    float64
	IfPaid      float64
	Prob        float64
	Contingency byte
}

// TableResult is the computed table.
type TableResult struct {
	LifeMode bool
	Rows     []TableRow
	// Grand totals (DOS GrandTotals): payment sum, value sum; in life mode
	// also the if-paid sum and the aggregate probability Σv/Σifpd.
	GrandPayment float64
	GrandValue   float64
	GrandIfPaid  float64
	GrandProb    float64
	// ScreenValue echoes the screen's Present Value from the same worksheet,
	// so callers can surface the documented table-vs-screen difference.
	ScreenValue float64
	PaymentN    int // number of payment lines (detail rows before filtering)
	Err         error
}

// tablePeriodic is one compacted periodic stream with its walkers.
type tablePeriodic struct {
	from, to types.DateRec
	perYr    int
	amt      float64
	cola     float64 // entered yield (engine convention); 0 when blank
	act      byte
	next     types.DateRec // next undischarged payment date; done=true when past to
	done     bool
	colaMult float64
	anniv    colaAnniversary
}

type tableLump struct {
	date types.DateRec
	amt  float64
	act  byte
}

// MakeTable computes the payment table for a worksheet. It first runs the
// ordinary screen calculation (DOS requires the screen computed before Ctrl-T
// and reads the DERIVED amounts — "Must display all before making table",
// PRESVALU.pas:1264); an uncomputable screen refuses with DOS's wording.
func MakeTable(input PVInput, req TableRequest) TableResult {
	var out TableResult

	res := Calculate(input)
	if res.Err != nil {
		out.Err = fmt.Errorf("Insufficient data to make a table. (%v)", res.Err)
		return out
	}
	out.ScreenValue = res.SumValue
	settings := input.Settings
	rate := res.Rate
	asOf := res.AsOf
	actu := input.Actuarial
	vr := input.RateSchedule

	// fold_in_life: any included row contingent, or a POD in force.
	lumps, pers := sortTableStreams(res, &settings)
	life := false
	if actu != nil {
		for _, l := range lumps {
			if l.act != actuarial.NotContingent {
				life = true
			}
		}
		for _, p := range pers {
			if p.act != actuarial.NotContingent {
				life = true
			}
		}
		if actu.POD != 0 || res.POD != 0 {
			life = true
		}
	}
	out.LifeMode = life

	discount := func(t types.DateRec) (float64, error) {
		if len(vr) > 0 {
			return VRDiscountFactor(asOf, t, vr, settings.Basis, settings.YrInv)
		}
		return interest.Exxp(-rate * dateutil.YearsDif(t, asOf, settings.Basis, settings.YrInv, false))
	}

	detail := req.Detail
	if detail == "" {
		detail = TableDetailOnly
	}
	emitDetail := detail == TableDetailOnly || detail == TableDetailBoth
	emitSummary := detail == TableDetailBoth || detail == TableSummary
	cumset := req.SummaryMonths
	if detail == TableDetailOnly {
		cumset = [13]bool{} // DOS detail-only leaves cumset empty
	}

	// Walk state (MakePVLTable body).
	var (
		nexta               int
		subQ, subV, subIfpd float64
		totQ, totV, totIfpd float64
		prevdate            types.DateRec
		havePrev            bool // DOS prevdate.m := -88 sentinel
		oldt                types.DateRec
		haveOld             bool
	)

	// TimeIsRipe (pvltable.pas): months crossed since the previous boundary
	// payment intersect cumset ⇒ a subtotal is due; the boundary advances.
	subtotalDue := func(t types.DateRec) bool {
		if !havePrev {
			havePrev = true
			prevdate = t
			return false
		}
		tt := prevdate
		hit := false
		for tt.Time.Year() < t.Time.Year() ||
			(tt.Time.Year() == t.Time.Year() && int(tt.Time.Month()) < int(t.Time.Month())) {
			if cumset[int(tt.Time.Month())] {
				hit = true
			}
			nd, err := dateutil.AddPeriod(tt, 12, tt.Time.Day(), false)
			if err != nil {
				break
			}
			tt = nd
		}
		if hit {
			prevdate = t
		}
		return hit
	}

	emitSubtotal := func() {
		if !emitSummary {
			subQ, subV, subIfpd = 0, 0, 0
			return
		}
		row := TableRow{Kind: TableRowSubtotal, Payment: subQ, Value: subV, CumValue: totV}
		if haveOld {
			row.Date, row.HasDate = oldt, true
		}
		if life {
			row.IfPaid = subIfpd
			if math.Abs(subIfpd) > teeny {
				row.Prob = subV / subIfpd
			}
		}
		out.Rows = append(out.Rows, row)
		subQ, subV, subIfpd = 0, 0, 0
	}

	// Main walk: SelectNextPayment / PrintNextPayment.
	for {
		t, q, cntg, more, err := nextTablePayment(lumps, pers, &nexta, life, &settings)
		if err != nil {
			out.Err = err
			return out
		}
		if !more {
			break
		}
		if subtotalDue(t) {
			emitSubtotal()
		}
		d, err := discount(t)
		if err != nil {
			out.Err = err
			return out
		}
		v := q * d
		var ifpd, prob float64
		if life {
			ifpd = v
			prob = 1.0
			if actu != nil && cntg != actuarial.NotContingent {
				prob = actu.LifeProb(t, cntg)
			}
			v = ifpd * prob
		}
		subQ += q
		totQ += q
		subV += v
		totV += v
		if life {
			subIfpd += ifpd
			totIfpd += ifpd
		}
		out.PaymentN++
		if emitDetail {
			row := TableRow{Kind: TableRowPayment, Date: t, HasDate: true,
				Payment: q, Value: v, CumValue: totV}
			if life {
				row.IfPaid = ifpd
				row.Contingency = cntg
				if math.Abs(ifpd) > teeny {
					row.Prob = v / ifpd
				}
			}
			out.Rows = append(out.Rows, row)
		}
		oldt, haveOld = t, true
	}

	// Trailing partial period (pvltable.pas: `if (subtotal.q<>0) and (cum>' ')`).
	if subQ != 0 && detail != TableDetailOnly {
		emitSubtotal()
	}

	// POD line (PrintPOD): life mode with a Payment on Death in force. The
	// value is the screen's PODValue (fixed-rate PODValue / VR XPODValue —
	// res.PODValue covers both; a solved-POD worksheet carries res.POD).
	if life {
		podAmt := 0.0
		if actu != nil {
			podAmt = actu.POD
		}
		if res.POD != 0 {
			podAmt = res.POD
		}
		if podAmt != 0 {
			podVal := res.PODValue
			totQ += podAmt
			totV += podVal
			totIfpd += podVal // deliberate divergence from DOS's corrupt `total.ifpd := total.v+podval` — see file header
			out.Rows = append(out.Rows, TableRow{Kind: TableRowPOD,
				Payment: podAmt, IfPaid: podVal, Prob: 1.0, Value: podVal, CumValue: totV})
		}
	}

	out.GrandPayment = totQ
	out.GrandValue = totV
	if life {
		out.GrandIfPaid = totIfpd
		if math.Abs(totIfpd) > teeny {
			out.GrandProb = totV / totIfpd
		}
	}
	return out
}

// sortTableStreams ports SortPayments: date-sort the usable lump rows, compact
// the usable periodic rows in order, initialize each periodic's COLA walker
// (InitializeColaData semantics — firstColaAnniversary matches it exactly),
// and cut "forever" streams at 50 years.
func sortTableStreams(res PVResult, settings *PVSettings) ([]tableLump, []*tablePeriodic) {
	var lumps []tableLump
	for i := range res.LumpSums {
		ls := &res.LumpSums[i]
		if ls.DateStatus > types.StatusEmpty && ls.AmtStatus > types.StatusEmpty {
			lumps = append(lumps, tableLump{date: ls.Date, amt: ls.Amt, act: ls.Act})
		}
	}
	sort.SliceStable(lumps, func(i, j int) bool {
		return dateutil.DateComp(lumps[i].date, lumps[j].date) < 0
	})

	var pers []*tablePeriodic
	for i := range res.Periodics {
		pp := &res.Periodics[i]
		if pp.FromDateStatus > types.StatusEmpty && pp.ToDateStatus > types.StatusEmpty &&
			pp.AmtStatus > types.StatusEmpty {
			tp := &tablePeriodic{
				from: pp.FromDate, to: pp.ToDate, perYr: pp.PerYr, amt: pp.Amt,
				act: pp.Act, colaMult: 1,
			}
			if pp.COLAStatus > types.StatusEmpty {
				tp.cola = pp.COLA
			}
			if tp.perYr <= 0 {
				tp.perYr = 12
			}
			// 50-year cutoff for forever streams: DOS replaces ONLY the year
			// (`todate.y := fromdate.y+50`), keeping todate's month/day.
			if tp.to.Time.Year() == types.LatestDate().Time.Year() {
				tp.to = types.NewDateRec(tp.from.Time.Year()+50,
					tp.to.Time.Month(), tp.to.Time.Day())
			}
			tp.next = tp.from
			tp.anniv = firstColaAnniversary(tp.from, settings)
			pers = append(pers, tp)
		}
	}
	return lumps, pers
}

// nextTablePayment ports SelectNextPayment: pick the earliest undischarged
// date across the lump cursor and every periodic walker, then collect the
// payment amount(s) on that date. In life mode only the FIRST matching source
// is consumed (payments keep separate lines — their contingencies may
// differ); otherwise all same-date sources combine.
func nextTablePayment(lumps []tableLump, pers []*tablePeriodic, nexta *int,
	life bool, settings *PVSettings) (t types.DateRec, q float64, cntg byte, more bool, err error) {

	have := false
	if *nexta < len(lumps) {
		t = lumps[*nexta].date
		have = true
	}
	for _, p := range pers {
		if p.done {
			continue
		}
		if !have || dateutil.DateComp(p.next, t) < 0 {
			t = p.next
			have = true
		}
	}
	if !have {
		return t, 0, 0, false, nil
	}

	// Lumps on this date.
	for *nexta < len(lumps) && dateutil.DateComp(lumps[*nexta].date, t) == 0 {
		q += lumps[*nexta].amt
		cntg = lumps[*nexta].act
		*nexta++
		if life {
			return t, q, cntg, true, nil
		}
	}
	// Periodic payments on this date.
	for _, p := range pers {
		if p.done || dateutil.DateComp(p.next, t) != 0 {
			continue
		}
		// UpdateAmountWithCola (PVLXSCRN.pas:103): continuous mode sets the
		// multiplier absolutely from elapsed years; stepped mode multiplies
		// once when the payment reaches the anniversary. The engine stores
		// COLA as an entered YIELD; DOS stores the continuous form, so the
		// stepped multiplier exp(cola_cont) is exactly (1+yield) and the
		// continuous exponent is Log1p(yield) (matches periodicSumAnnualCOLA).
		if settings.COLAMonth == types.COLAContinuous || p.cola == 0 {
			cc, e := colaContinuous(p.cola, settings.YrDays)
			if e != nil {
				return t, 0, 0, false, e
			}
			ex, e := interest.Exxp(cc *
				dateutil.YearsDif(t, p.from, settings.Basis, settings.YrInv, false))
			if e != nil {
				return t, 0, 0, false, e
			}
			p.colaMult = ex
		} else if p.anniv.reached(t) {
			p.colaMult *= 1 + p.cola
			p.anniv = p.anniv.next()
		}
		q += p.amt * p.colaMult
		cntg = p.act
		nd, e := dateutil.AddPeriod(p.next, p.perYr, p.from.Time.Day(), false)
		if e != nil {
			// STOP AND KEEP this stream, keep the payment just emitted, and
			// leave the other streams walking. DOS's MDY writes errorbyte into
			// p.next's month field in place; the very next statement is
			// `if DateComp(p.next, p.to) > 0 then done`, and DateComp orders a
			// poisoned record after every real date, so the stream retires
			// itself (see dateutil.ErrJulianCeiling). Aborting the whole table
			// instead would drop a perpetual biweekly row's payments entirely.
			if errors.Is(e, dateutil.ErrJulianCeiling) {
				p.done = true
				if life {
					return t, q, cntg, true, nil
				}
				continue
			}
			return t, 0, 0, false, e
		}
		p.next = nd
		if dateutil.DateComp(p.next, p.to) > 0 {
			p.done = true
		}
		if life {
			return t, q, cntg, true, nil
		}
	}
	return t, q, cntg, true, nil
}

// ContingencyChar returns the DOS actchar letter for a contingency code
// (PEDATA.pas:144) for display in the table's Cntg column.
func ContingencyChar(c byte) string {
	switch c {
	case actuarial.Living:
		return "L"
	case actuarial.Dead:
		return "D"
	case actuarial.Only1Living:
		return "1"
	case actuarial.Only2Living:
		return "2"
	case actuarial.EitherLiving:
		return "E"
	case actuarial.BothLiving:
		return "B"
	default:
		return "N"
	}
}
