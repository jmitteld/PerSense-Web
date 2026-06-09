// Package api provides HTTP handlers for the Per%Sense financial calculation
// REST API. Each screen type (mortgage, amortization, present value) has
// dedicated endpoints for calculation and file import.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/finance/amortization"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/finance/mortgage"
	"github.com/persense/persense-port/internal/finance/presentvalue"
	"github.com/persense/persense-port/internal/types"
)

// apiDateLayouts are the date formats accepted on every date-bearing
// API field, tried in order. ISO (YYYY-MM-DD) is the canonical wire
// format the frontend sends; the US M/D/Y forms are accepted because
// the validation messages advertise MM/DD/YYYY and DOS users expect it.
// Single-digit month/day variants are included so "1/2/2026" works as
// well as "01/02/2026".
var apiDateLayouts = []string{
	"2006-01-02", // ISO, canonical
	"01/02/2006", // US, zero-padded
	"1/2/2006",   // US, unpadded
}

// parseAPIDate parses a date string from an API request, accepting both
// the canonical ISO form and the US MM/DD/YYYY form advertised in the
// validation error messages. It returns the same error shape as
// time.Parse on failure so existing callers keep working unchanged.
func parseAPIDate(s string) (time.Time, error) {
	var firstErr error
	for _, layout := range apiDateLayouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return time.Time{}, firstErr
}

// --- Request/Response types ---

// MortgageRequest is the JSON input for a mortgage calculation.
type MortgageRequest struct {
	Price         *float64 `json:"price,omitempty"`
	Points        *float64 `json:"points,omitempty"`
	PctDown       *float64 `json:"pctDown,omitempty"`
	Cash          *float64 `json:"cash,omitempty"`
	Financed      *float64 `json:"financed,omitempty"`
	Years         *int     `json:"years,omitempty"`
	Rate          *float64 `json:"rate,omitempty"`
	Tax           *float64 `json:"tax,omitempty"`
	Monthly       *float64 `json:"monthly,omitempty"`
	BalloonYears  *int     `json:"balloonYears,omitempty"`
	BalloonAmount *float64 `json:"balloonAmount,omitempty"`
	// Basis selects the day-count used for the APR calculation
	// ("360", "365", "365/360"). DOS computes the APR on the screen's
	// basis; omitting it keeps the historical 365.25-day default.
	Basis string `json:"basis,omitempty"`
}

// mtgAPRYrDays maps a mortgage request basis string to the days-per-
// year used in the APR day-count. An empty basis preserves the
// historical 365.25-day default.
func mtgAPRYrDays(basis string) float64 {
	if basis == "360" {
		return 360.0
	}
	return 365.25
}

// MortgageResponse is the JSON output for a mortgage calculation.
type MortgageResponse struct {
	Price         float64 `json:"price"`
	Points        float64 `json:"points"`
	PctDown       float64 `json:"pctDown"`
	Cash          float64 `json:"cash"`
	Financed      float64 `json:"financed"`
	Years         int     `json:"years"`
	Rate          float64 `json:"rate"`
	Tax           float64 `json:"tax"`
	Monthly       float64 `json:"monthly"`
	BalloonYears  int     `json:"balloonYears,omitempty"`
	BalloonAmount float64 `json:"balloonAmount,omitempty"`
	APR           float64 `json:"apr,omitempty"`
	APRConverged  bool    `json:"aprConverged,omitempty"`
	// Warnings carries non-fatal advisories (e.g. the APR solve did
	// not converge). Present alongside a normal result.
	Warnings []string `json:"warnings,omitempty"`
	Error    string   `json:"error,omitempty"`
	// ErrorDetail carries the structured form of Error (dispatch_gaps §4.3).
	ErrorDetail *FieldError `json:"errorDetail,omitempty"`
}

// mortgageLineInput holds the primary inputs of one mortgage for the
// compare and what-if endpoints. Omitted (nil) fields are treated as
// blank and solved for by mortgage.Calc.
type mortgageLineInput struct {
	Price    *float64 `json:"price,omitempty"`
	Points   *float64 `json:"points,omitempty"`
	PctDown  *float64 `json:"pctDown,omitempty"`
	Cash     *float64 `json:"cash,omitempty"`
	Financed *float64 `json:"financed,omitempty"`
	Years    *int     `json:"years,omitempty"`
	Rate     *float64 `json:"rate,omitempty"`
	Monthly  *float64 `json:"monthly,omitempty"`
}

// MortgageCompareRequest is the JSON input for POST /api/mortgage/compare.
type MortgageCompareRequest struct {
	A mortgageLineInput `json:"a"`
	B mortgageLineInput `json:"b"`
	// Basis selects the APR day-count ("360", "365", "365/360");
	// empty keeps the 365.25-day default.
	Basis string `json:"basis,omitempty"`
}

// MortgageCompareResponse is the JSON output for a mortgage APR comparison.
type MortgageCompareResponse struct {
	APR1           float64 `json:"apr1"`
	APR1Converged  bool    `json:"apr1Converged"`
	APR2           float64 `json:"apr2"`
	APR2Converged  bool    `json:"apr2Converged"`
	CrossoverAPR   float64 `json:"crossoverApr,omitempty"`
	CrossoverYears float64 `json:"crossoverYears,omitempty"`
	Summary        string  `json:"summary,omitempty"`
	Error          string  `json:"error,omitempty"`
}

// MortgageWhatIfRequest is the JSON input for POST /api/mortgage/whatif.
// Vary names the field to step (rate, years, points, pctDown, price,
// monthly); Increment is the per-row step; Count is the number of rows.
type MortgageWhatIfRequest struct {
	Base      mortgageLineInput `json:"base"`
	Vary      string            `json:"vary"`
	Increment float64           `json:"increment"`
	Count     int               `json:"count"`
}

// MortgageWhatIfRow is one generated row in a what-if table.
type MortgageWhatIfRow struct {
	Price    float64 `json:"price"`
	Points   float64 `json:"points"`
	PctDown  float64 `json:"pctDown"`
	Cash     float64 `json:"cash"`
	Financed float64 `json:"financed"`
	Years    int     `json:"years"`
	Rate     float64 `json:"rate"`
	Monthly  float64 `json:"monthly"`
}

// MortgageWhatIfResponse is the JSON output for a what-if table.
type MortgageWhatIfResponse struct {
	Rows  []MortgageWhatIfRow `json:"rows"`
	Error string              `json:"error,omitempty"`
}

// AmortizationRequest is the JSON input for an amortization calculation.
//
// The Advanced Options block (Prepayments, Balloons, Adjustments,
// Moratorium, Target, SkipMonths) is optional. When any of those is
// supplied, the engine runs in fancy mode automatically.
type AmortizationRequest struct {
	// Amount and Rate are pointers so an omitted field (nil) is
	// distinguishable from an explicit zero. A nil Amount dispatches
	// to amortization.SolveLoanAmount; a nil Rate dispatches to
	// amortization.SolveRate (DOS field-presence dispatch). Both nil
	// is the derive-only "how many payments" case.
	Amount    *float64 `json:"amount,omitempty"`
	LoanDate  string   `json:"loanDate"` // YYYY-MM-DD
	Rate      *float64 `json:"rate,omitempty"`
	FirstDate string   `json:"firstDate,omitempty"` // YYYY-MM-DD, optional (defaults to loanDate + 1 period)
	LastDate  string   `json:"lastDate,omitempty"`  // YYYY-MM-DD, optional (alternative to nPeriods)
	NPeriods  int      `json:"nPeriods,omitempty"`  // optional if lastDate is supplied
	PerYr     int      `json:"perYr"`
	Payment   float64  `json:"payment,omitempty"`
	Basis     string   `json:"basis,omitempty"` // "360", "365", "365/360"

	// --- Advanced Options ---
	Prepayments []AmortPrepaymentReq `json:"prepayments,omitempty"`
	Balloons    []AmortBalloonReq    `json:"balloons,omitempty"`
	Adjustments []AmortAdjustmentReq `json:"adjustments,omitempty"`
	Moratorium  *string              `json:"moratorium,omitempty"` // YYYY-MM-DD
	TargetAmt   *float64             `json:"targetAmt,omitempty"`
	SkipMonths  string               `json:"skipMonths,omitempty"` // e.g. "6-8,12"

	// BalloonIncludesRegular mirrors the DOS Computational Setting
	// "Stated balloon includes regular pmt". When false (the DOS
	// default), a balloon at a payment date is ADDED to the regular
	// payment that period. When true, the balloon AMOUNT is treated
	// as the total for that period — replacing the regular payment.
	// Omitting the field defaults to false (the DOS default).
	BalloonIncludesRegular bool `json:"balloonIncludesRegular,omitempty"`

	// Points is the discount-points charge as a fraction of the loan
	// amount (e.g. 0.02 = 2 points). When supplied, the engine
	// computes the loan's APR (DOS EstimateAndRefineAPRwithPoints).
	Points *float64 `json:"points,omitempty"`

	// InAdvance and USARule mirror the DOS Computational Settings of
	// the same name (ComputationalSettingsDlgUnit / AMORTOP.pas).
	//   - InAdvance: interest is charged on payments made at the start
	//     of each period rather than the end (annuity-due). The engine
	//     applies the ff=(f-1)/(2-f) prorate factor.
	//   - USARule: unpaid interest is tracked separately and is not
	//     compounded into principal (US Rule). Default (false) is the
	//     ordinary actuarial method.
	// Both default to false, matching the DOS defaults.
	InAdvance bool `json:"inAdvance,omitempty"`
	USARule   bool `json:"usaRule,omitempty"`

	// Rule78 selects Rule-of-78 ("sum of the digits") interest
	// allocation for a basic (non-fancy) loan — interest is
	// front-loaded across the term. Mirrors the DOS "Rule of 78s"
	// computational setting. Default false.
	Rule78 bool `json:"rule78,omitempty"`

	// FirstIntPrepaid mirrors the DOS "1st interest prepaid at
	// settlement" computational setting. When true (the DOS default),
	// the partial-period interest from the loan date to the first
	// payment is collected at closing as a settlement-stub row
	// (PayNum 0) and every regular payment then covers a full period.
	// When false, that partial-period interest is instead rolled into
	// the first regular payment and there is no stub row. It is a
	// pointer so an omitted field (nil) preserves the DOS default of
	// true rather than the Go bool zero value (false).
	FirstIntPrepaid *bool `json:"firstIntPrepaid,omitempty"`
}

// AmortPrepaymentReq is one extra-payment series in an amortization
// request. Each series adds PerYr extra payments per year between
// StartDate and StopDate (or until NPmts payments have been made).
type AmortPrepaymentReq struct {
	StartDate string `json:"startDate"`          // YYYY-MM-DD
	NPmts     int    `json:"nPmts,omitempty"`    // number of extra payments
	StopDate  string `json:"stopDate,omitempty"` // YYYY-MM-DD; alt to NPmts
	PerYr     int    `json:"perYr"`
	// Amount is a pointer: omitting it (nil) asks the engine to solve
	// the prepayment amount that retires the loan — the DOS "unknown
	// prepayment" (EstimateAndRefinePeriodicPrepayment). When the
	// amount is omitted the series must be bounded by NPmts or
	// StopDate so the number of extra payments is known.
	Amount *float64 `json:"amount,omitempty"`
}

// AmortBalloonReq is one balloon payment in an amortization request.
// Amount is a pointer: omitting it (nil) asks the engine to solve for
// the balloon amount that drives the schedule balance to zero — the
// DOS "target balloon" (EstimateAndRefineBalloon).
type AmortBalloonReq struct {
	Date   string   `json:"date"` // YYYY-MM-DD
	Amount *float64 `json:"amount,omitempty"`
}

// AmortAdjustmentReq is one rate / payment adjustment in an
// amortization request. Either Rate or Amount (or both) can be set.
type AmortAdjustmentReq struct {
	Date   string   `json:"date"` // YYYY-MM-DD
	Rate   *float64 `json:"rate,omitempty"`
	Amount *float64 `json:"amount,omitempty"`
}

// AmortizationResponse is the JSON output for an amortization schedule.
//
// NPeriods, FirstDate, and LastDate echo the post-FirstPass values the
// engine actually used. They're always populated on success, even when
// the caller supplied the same field on input — the frontend can use
// them to fill in a blank cell after deriving from siblings (e.g. the
// user supplies first + last and the engine returns the derived term).
type AmortizationResponse struct {
	Schedule  []PaymentLine `json:"schedule"`
	TotalPaid float64       `json:"totalPaid"`
	TotalInt  float64       `json:"totalInterest"`
	NPeriods  int           `json:"nPeriods,omitempty"`
	// Amount and Rate echo the loan principal and (user-facing) loan
	// rate the engine actually used. They matter when the caller left
	// one of them blank for the backward solver to fill: the response
	// carries the solved value so the UI can display it.
	Amount    float64 `json:"amount,omitempty"`
	Rate      float64 `json:"rate,omitempty"`
	FirstDate string  `json:"firstDate,omitempty"` // YYYY-MM-DD
	LastDate  string  `json:"lastDate,omitempty"`  // YYYY-MM-DD
	// APR is the annual percentage rate, computed only when the
	// caller supplied discount Points. APRConverged reports whether
	// the iterative solve reached a stable value.
	APR          float64 `json:"apr,omitempty"`
	APRConverged bool    `json:"aprConverged,omitempty"`
	// Warnings carries non-fatal advisories (e.g. the loan retired
	// before its scheduled term). Present alongside a normal result.
	Warnings []string `json:"warnings,omitempty"`
	Error    string   `json:"error,omitempty"`
	// ErrorDetail carries the structured form of Error when the
	// failure can be tied to a specific field/row (dispatch_gaps §4.3).
	ErrorDetail *FieldError `json:"errorDetail,omitempty"`
}

// PaymentLine is one row in an amortization schedule.
type PaymentLine struct {
	PayNum    int     `json:"payNum"`
	Date      string  `json:"date"`
	Payment   float64 `json:"payment"`
	Interest  float64 `json:"interest"`
	Principal float64 `json:"principal"`
	IntToDate float64 `json:"intToDate"`
}

// PVRequest is the JSON input for a present value calculation.
//
// Any field on the input that is omitted is treated as "blank" and
// becomes a candidate for backward solving. To support solving for an
// unknown rate, as-of date, or sum value, the corresponding scalar
// fields are pointers — omit them from the request payload to leave
// them blank.
type PVRequest struct {
	AsOfDate  *string         `json:"asOfDate,omitempty"` // YYYY-MM-DD; omit to solve for it
	Rate      *float64        `json:"rate,omitempty"`     // omit to solve for the rate
	SumValue  *float64        `json:"sumValue,omitempty"` // provide for backward calc
	LumpSums  []PVLumpSumReq  `json:"lumpSums,omitempty"`
	Periodics []PVPeriodicReq `json:"periodics,omitempty"`
	Actuarial *PVActuarialReq `json:"actuarial,omitempty"`

	// COLAMonth selects how a periodic-payment COLA escalates:
	// 99 = anniversary of the from-date (default), 98 = continuous,
	// 1-12 = the 1st of that calendar month. Omitting it (0) defaults
	// to anniversary. Mirrors the DOS "COLA escalation month" setting.
	COLAMonth int `json:"colaMonth,omitempty"`

	// RateSchedule, when non-empty, switches the engine into
	// variable-rate mode (DOS PVL fancy). PresVal.Rate is ignored;
	// each cash flow is discounted through the piecewise schedule.
	// Backward solving (unknown rate / payment / date) is not
	// supported in this mode — matches DOS PV_VariableRate.html.
	RateSchedule []PVRateLineReq `json:"rateSchedule,omitempty"`
}

// PVRateLineReq is one entry in a variable-rate schedule. Each entry
// names a date a new rate takes effect; the rate stays in force until
// the next entry's Date. The first entry's Date is treated as "from
// the beginning" — its rate is the starting rate regardless of the
// stored date (matches the DOS UX where the first row's date cell
// was a non-editable "XX").
//
// The API only accepts the continuously-compounded TrueRate. The UI
// presents Loan Rate / True Rate / Yield as equivalent inputs and
// converts to TrueRate before posting, so the engine never has to
// figure out which form the caller intended.
type PVRateLineReq struct {
	Date     string  `json:"date"`     // YYYY-MM-DD
	TrueRate float64 `json:"trueRate"` // continuously-compounded
}

// PVLumpSumReq represents a lump sum payment in a PV request.
//
// Either Date or Amount may be omitted to ask the backward calc to
// solve for it (given enough other data on the screen). Value (Val)
// can be supplied to pin the row's expected present value.
type PVLumpSumReq struct {
	Date   *string  `json:"date,omitempty"`
	Amount *float64 `json:"amount,omitempty"`
	Value  *float64 `json:"value,omitempty"`
	Act    string   `json:"act,omitempty"` // contingency: N,L,D,1,2,E,B
}

// PVPeriodicReq represents a periodic payment series in a PV request.
//
// Either FromDate or ToDate may be omitted to ask the backward calc to
// solve for it. Amount may be omitted when both dates are present and
// the row's value is known.
type PVPeriodicReq struct {
	FromDate *string  `json:"fromDate,omitempty"`
	ToDate   *string  `json:"toDate,omitempty"`
	PerYr    *int     `json:"perYr,omitempty"`
	Amount   *float64 `json:"amount,omitempty"`
	Value    *float64 `json:"value,omitempty"`
	COLA     *float64 `json:"cola,omitempty"`
	Act      string   `json:"act,omitempty"` // contingency: N,L,D,1,2,E,B
}

// PVActuarialReq holds actuarial (life contingency) configuration.
type PVActuarialReq struct {
	Table1  [][]float64 `json:"table1"`           // [[age, qx], ...]
	DOB1    string      `json:"dob1"`             // YYYY-MM-DD
	Table2  [][]float64 `json:"table2,omitempty"` // optional second life
	DOB2    string      `json:"dob2,omitempty"`
	AsOfNow string      `json:"asOfNow"` // reference "now" date
	// POD is the payment-on-death amount. Omit it (nil) to have the
	// engine solve for it from the target Sum Value (ComputeUnknownPOD).
	POD *float64 `json:"pod,omitempty"`
}

// PVResponse is the JSON output for a present value calculation.
type PVResponse struct {
	SumValue  float64          `json:"sumValue"`
	PODValue  float64          `json:"podValue,omitempty"`
	// POD carries the solved Payment-on-Death amount when the
	// actuarial config left it blank (ComputeUnknownPOD).
	POD float64 `json:"pod,omitempty"`
	LumpSums  []PVLumpSumResp  `json:"lumpSums,omitempty"`
	Periodics []PVPeriodicResp `json:"periodics,omitempty"`
	// Rate and AsOfDate echo the discount rate and as-of date the
	// engine actually used. They carry the solved value back to the
	// caller for the backward solves PV-8 (blank rate) and PV-9
	// (blank as-of date).
	Rate     float64 `json:"rate,omitempty"`
	AsOfDate string  `json:"asOfDate,omitempty"` // YYYY-MM-DD
	// Warnings carries non-fatal advisories (e.g. an over-specified
	// row). Present alongside a normal result, not in place of one.
	Warnings []string `json:"warnings,omitempty"`
	Error    string   `json:"error,omitempty"`
	// ErrorDetail carries the structured form of Error (dispatch_gaps §4.3).
	ErrorDetail *FieldError `json:"errorDetail,omitempty"`
}

// PVLumpSumResp represents a computed lump sum value.
type PVLumpSumResp struct {
	Date   string  `json:"date"`
	Amount float64 `json:"amount"`
	Value  float64 `json:"value"`
	Prob   float64 `json:"prob,omitempty"` // survival probability (actuarial)
}

// PVPeriodicResp represents a computed periodic payment value.
type PVPeriodicResp struct {
	FromDate string  `json:"fromDate"`
	ToDate   string  `json:"toDate"`
	Amount   float64 `json:"amount"`
	Value    float64 `json:"value"`
	Prob     float64 `json:"prob,omitempty"` // avg survival probability (actuarial)
}

// --- Handlers ---

// HandleMortgageCalc handles POST /api/mortgage/calc
func HandleMortgageCalc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MortgageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, MortgageResponse{Error: "invalid JSON: " + err.Error()})
		return
	}

	// Build MtgLine from request
	m := mortgage.MtgLine{}
	if req.Price != nil {
		m.PriceStatus = types.InOutInput
		m.Price = *req.Price
	}
	if req.Points != nil {
		m.PointsStatus = types.InOutInput
		m.Points = *req.Points
	}
	if req.PctDown != nil {
		m.PctStatus = types.InOutInput
		m.Pct = *req.PctDown
	}
	if req.Cash != nil {
		m.CashStatus = types.InOutInput
		m.Cash = *req.Cash
	}
	if req.Financed != nil {
		m.FinancedStatus = types.InOutInput
		m.Financed = *req.Financed
	}
	if req.Years != nil {
		m.YearsStatus = types.InOutInput
		m.Years = *req.Years
	}
	if req.Rate != nil {
		m.RateStatus = types.InOutInput
		// Request carries the user-facing loan rate (e.g. 0.08 for
		// 8%); MtgLine.Rate is the continuously-compounded true
		// rate. Convert at the boundary.
		m.Rate = mortgage.LoanRateToTrueRate(*req.Rate)
	}
	if req.Tax != nil {
		m.TaxStatus = types.InOutInput
		m.Tax = *req.Tax
	}
	if req.Monthly != nil {
		m.MonthlyStatus = types.InOutInput
		m.Monthly = *req.Monthly
	}
	if req.BalloonYears != nil {
		m.WhenStatus = types.InOutInput
		m.When = *req.BalloonYears
	}
	if req.BalloonAmount != nil {
		m.HowMuchStatus = types.InOutInput
		m.HowMuch = *req.BalloonAmount
	}

	result := mortgage.Calc(m)
	resp := MortgageResponse{
		Price:    result.Line.Price,
		Points:   result.Line.Points,
		PctDown:  result.Line.Pct,
		Cash:     result.Line.Cash,
		Financed: result.Line.Financed,
		Years:    result.Line.Years,
		// Convert the internal true rate back to a user-facing
		// loan rate before responding.
		Rate:          mortgage.TrueRateToLoanRate(result.Line.Rate),
		Tax:           result.Line.Tax,
		Monthly:       result.Line.Monthly,
		BalloonYears:  result.Line.When,
		BalloonAmount: result.Line.HowMuch,
		Warnings:      result.Warnings,
	}

	if result.Err != nil {
		resp.Error = result.Err.Error()
	} else if mortgage.EnoughDataForAPR(&result.Line) {
		apr, conv, _ := mortgage.FullTermAPR(result.Line, mtgAPRYrDays(req.Basis))
		// FullTermAPR converts its internal true-rate iterate back
		// to a yield at monthly compounding (via YieldFromRate) so
		// the returned value is already a loan rate — no further
		// conversion needed here.
		resp.APR = apr
		resp.APRConverged = conv
		if !conv {
			resp.Warnings = append(resp.Warnings,
				"APR did not converge — the reported APR is approximate.")
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// mtgLineFromInput builds a mortgage.MtgLine from the compare/what-if
// request shape. Each supplied field is marked as an input; the user
// loan rate is converted to the internal continuously-compounded true
// rate at the boundary, matching HandleMortgageCalc.
func mtgLineFromInput(in mortgageLineInput) mortgage.MtgLine {
	m := mortgage.MtgLine{}
	if in.Price != nil {
		m.PriceStatus = types.InOutInput
		m.Price = *in.Price
	}
	if in.Points != nil {
		m.PointsStatus = types.InOutInput
		m.Points = *in.Points
	}
	if in.PctDown != nil {
		m.PctStatus = types.InOutInput
		m.Pct = *in.PctDown
	}
	if in.Cash != nil {
		m.CashStatus = types.InOutInput
		m.Cash = *in.Cash
	}
	if in.Financed != nil {
		m.FinancedStatus = types.InOutInput
		m.Financed = *in.Financed
	}
	if in.Years != nil {
		m.YearsStatus = types.InOutInput
		m.Years = *in.Years
	}
	if in.Rate != nil {
		m.RateStatus = types.InOutInput
		m.Rate = mortgage.LoanRateToTrueRate(*in.Rate)
	}
	if in.Monthly != nil {
		m.MonthlyStatus = types.InOutInput
		m.Monthly = *in.Monthly
	}
	return m
}

// HandleMortgageCompare handles POST /api/mortgage/compare. It computes
// each mortgage, then runs mortgage.CompareAPRs — which reports each
// full-term APR and, when the APRs cross, the crossover APR and the
// holding period beyond which the other mortgage becomes cheaper.
//
// This exposes the engine's CompareAPRs (mortgage.go) that the
// frontend previously approximated client-side. See dispatch_gaps M14.
func HandleMortgageCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MortgageCompareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, MortgageCompareResponse{Error: "invalid JSON: " + err.Error()})
		return
	}

	// CompareAPRs needs fully-computed mortgages (Financed, Monthly,
	// etc.), so run Calc on each line first.
	a := mortgage.Calc(mtgLineFromInput(req.A))
	if a.Err != nil {
		writeJSON(w, http.StatusBadRequest, MortgageCompareResponse{Error: "mortgage A: " + a.Err.Error()})
		return
	}
	b := mortgage.Calc(mtgLineFromInput(req.B))
	if b.Err != nil {
		writeJSON(w, http.StatusBadRequest, MortgageCompareResponse{Error: "mortgage B: " + b.Err.Error()})
		return
	}

	cmp, err := mortgage.CompareAPRs(a.Line, b.Line, mtgAPRYrDays(req.Basis))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, MortgageCompareResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, MortgageCompareResponse{
		APR1:           cmp.APR1,
		APR1Converged:  cmp.APR1Converged,
		APR2:           cmp.APR2,
		APR2Converged:  cmp.APR2Converged,
		CrossoverAPR:   cmp.CrossoverAPR,
		CrossoverYears: cmp.CrossoverTime,
		Summary:        cmp.Summary,
	})
}

// varyFieldFromString maps the JSON "vary" name to a mortgage.VaryField.
func varyFieldFromString(s string) (mortgage.VaryField, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "rate":
		return mortgage.VaryRate, nil
	case "years":
		return mortgage.VaryYears, nil
	case "points":
		return mortgage.VaryPoints, nil
	case "pctdown", "pct", "percentdown":
		return mortgage.VaryPctDown, nil
	case "price":
		return mortgage.VaryPrice, nil
	case "monthly":
		return mortgage.VaryMonthly, nil
	default:
		return mortgage.VaryNone, fmt.Errorf(
			"unknown vary field %q (use rate, years, points, pctDown, price, or monthly)", s)
	}
}

// HandleMortgageWhatIf handles POST /api/mortgage/whatif. It computes
// the base mortgage, then calls mortgage.GenerateRows to produce Count
// rows, stepping the chosen field by Increment and re-solving the
// dependent field on each row.
//
// This exposes the engine's GenerateRows (rowgen.go) that the frontend
// previously approximated by looping /api/mortgage/calc. See
// dispatch_gaps M15.
func HandleMortgageWhatIf(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MortgageWhatIfRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, MortgageWhatIfResponse{Error: "invalid JSON: " + err.Error()})
		return
	}

	vary, err := varyFieldFromString(req.Vary)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, MortgageWhatIfResponse{Error: err.Error()})
		return
	}
	if req.Count <= 0 {
		writeJSON(w, http.StatusBadRequest, MortgageWhatIfResponse{
			Error: "What-If: the row count must be a positive whole number — " +
				"set how many rows the table should generate."})
		return
	}

	// Compute the base row first so one of {Price, Monthly, Balloon}
	// is a computed OUTPUT — GenerateRows re-solves that field on
	// every generated row.
	base := mortgage.Calc(mtgLineFromInput(req.Base))
	if base.Err != nil {
		writeJSON(w, http.StatusBadRequest, MortgageWhatIfResponse{Error: "base mortgage: " + base.Err.Error()})
		return
	}

	rows, err := mortgage.GenerateRows(base.Line, vary, req.Increment, req.Count)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, MortgageWhatIfResponse{Error: err.Error()})
		return
	}

	resp := MortgageWhatIfResponse{}
	for _, row := range rows {
		resp.Rows = append(resp.Rows, MortgageWhatIfRow{
			Price:    row.Price,
			Points:   row.Points,
			PctDown:  row.Pct,
			Cash:     row.Cash,
			Financed: row.Financed,
			Years:    row.Years,
			Rate:     mortgage.TrueRateToLoanRate(row.Rate),
			Monthly:  row.Monthly,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// HandleAmortizationCalc handles POST /api/amortization/calc
func HandleAmortizationCalc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AmortizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: "invalid JSON: " + err.Error()})
		return
	}

	// Derive-only mode: when both Amount and Rate are blank, the
	// caller just wants the term (and any of {firstDate, lastDate,
	// nPeriods} derivable from the others) — no schedule, no payment.
	// Matches Help/Amortization Example 1c's spirit ("how many
	// payments is that?"). We skip the full Amortize pipeline and run
	// FirstPass alone, which is the only step that performs the
	// term-derivation. See amortization.FirstPass for the arm
	// semantics (A-FP-defFirst, A-FP-last, A-FP-n).
	if req.Amount == nil && req.Rate == nil {
		handleAmortizationDeriveOnly(w, req)
		return
	}

	loanDate, err := parseAPIDate(req.LoanDate)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: "Loan Date is unparseable — use MM/DD/YYYY (or ISO YYYY-MM-DD)"})
		return
	}

	basis := types.Basis360
	switch req.Basis {
	case "365":
		basis = types.Basis365
	case "365/360":
		basis = types.Basis365360
	}

	// Weekly/biweekly loans on a 360-day basis are coerced to a
	// 365-day basis — a 30/360 day count is meaningless for sub-
	// monthly payment frequencies. Mirrors DOS Amortize.pas:297-303,
	// which makes the same switch (and shows a notice).
	var basisCoerced bool
	if (req.PerYr == 26 || req.PerYr == 52) && basis == types.Basis360 {
		basis = types.Basis365
		basisCoerced = true
	}

	ctx := interest.NewCalcContext(basis, byte(req.PerYr))
	input := amortization.LoanInput{
		Loan: amortization.Loan{
			LoanDateStatus: types.InOutInput,
			LoanDate:       types.NewDateRec(loanDate.Year(), loanDate.Month(), loanDate.Day()),
			PerYrStatus:    types.InOutInput,
			PerYr:          req.PerYr,
			PayAmtStatus:   types.InOutInput,
			PayAmt:         req.Payment,
			// LastOK is set by amortization.FirstPass once any of
			// {firstDate, lastDate, nPeriods} is derived from the
			// others. Previously hard-coded to true here, which
			// caused the engine to terminate on a zero-time
			// comparison when no lastDate was supplied.
			LastOK: false,
		},
		Settings: amortization.Settings{
			Basis: basis,
			PerYr: byte(req.PerYr),
			// "1st interest prepaid at settlement" (DOS default YES).
			// A nil request field keeps that default; an explicit
			// false rolls the partial-period interest into the first
			// payment instead of emitting a settlement-stub row.
			Prepaid: req.FirstIntPrepaid == nil || *req.FirstIntPrepaid,
			// PlusRegular=true means a balloon ADDS to the regular
			// payment at the balloon date (DOS-faithful default,
			// "stated balloon includes regular pmt = No"). With
			// PlusRegular=false the balloon REPLACES the regular
			// payment instead. The Go zero-value was false, which
			// silently flipped the default away from DOS — Help
			// Example 5 (interest-only + balloon) depends on the
			// ADD semantics to clear the principal at term-end.
			PlusRegular: !req.BalloonIncludesRegular,
			// DOS Computational Settings, threaded from the request.
			InAdvance: req.InAdvance,
			USARule:   req.USARule,
			R78:       req.Rule78,
			YrDays:    ctx.YrDays,
			YrInv:     ctx.YrInv,
		},
	}
	// Amount / Rate carry a status of "input" only when the caller
	// supplied the field. A nil pointer leaves the status at its zero
	// value (StatusEmpty) so the solver dispatch below treats it as
	// the field to solve for.
	if req.Amount != nil {
		input.Loan.AmountStatus = types.InOutInput
		input.Loan.Amount = *req.Amount
	}
	if req.Rate != nil {
		input.Loan.LoanRateStatus = types.InOutInput
		input.Loan.LoanRate = *req.Rate
	}
	// Points, when supplied, triggers the APR computation.
	if req.Points != nil {
		input.Loan.PointsStatus = types.InOutInput
		input.Loan.Points = *req.Points
	}
	// NPeriods is optional when LastDate is supplied — FirstPass will
	// derive the term from firstDate + lastDate (DOS A-FP-n).
	if req.NPeriods > 0 {
		input.Loan.NStatus = types.InOutInput
		input.Loan.NPeriods = req.NPeriods
	}
	// FirstDate is optional. If omitted, amortization.FirstPass will
	// derive it as loanDate + 1 period (DOS A-FP-defFirst).
	if req.FirstDate != "" {
		firstDate, err := parseAPIDate(req.FirstDate)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: "1st Pmt Date is unparseable — use MM/DD/YYYY (or ISO YYYY-MM-DD)"})
			return
		}
		input.Loan.FirstStatus = types.InOutInput
		input.Loan.FirstDate = types.NewDateRec(firstDate.Year(), firstDate.Month(), firstDate.Day())
	} else {
		input.Loan.FirstDate = types.UnknownDate()
	}
	// LastDate is optional. When supplied (and NPeriods is blank),
	// FirstPass derives NPeriods from firstDate + lastDate (DOS A-FP-n).
	if req.LastDate != "" {
		lastDate, err := parseAPIDate(req.LastDate)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: "Last Pmt Date is unparseable — use MM/DD/YYYY (or ISO YYYY-MM-DD)"})
			return
		}
		input.Loan.LastStatus = types.InOutInput
		input.Loan.LastDate = types.NewDateRec(lastDate.Year(), lastDate.Month(), lastDate.Day())
	} else {
		input.Loan.LastDate = types.UnknownDate()
	}
	if req.Payment == 0 {
		input.Loan.PayAmtStatus = types.StatusEmpty
	}

	// --- Advanced Options ---
	advanced := false

	for i, p := range req.Prepayments {
		rowNum := i + 1
		// Reject half-filled rows with field-named, row-indexed errors
		// so the UI can highlight the offending cell instead of
		// silently dropping the row (dispatch_gaps S-3).
		if p.StartDate == "" {
			writeAmortFieldErr(w, newFieldError("AMORT_PREPAY_INCOMPLETE", "prepayment",
				rowNum, []string{"Start Date"},
				fmt.Sprintf("Prepayment row %d: Start Date is required.", rowNum)))
			return
		}
		// A nil Amount asks the engine to solve the prepayment amount.
		// That only works if the series is bounded (NPmts or StopDate),
		// so the count of extra payments is known.
		if p.Amount == nil && p.NPmts <= 0 && p.StopDate == "" {
			writeAmortFieldErr(w, newFieldError("AMORT_PREPAY_INCOMPLETE", "prepayment",
				rowNum, []string{"Amount", "Number of Payments", "Stop Date"},
				fmt.Sprintf("Prepayment row %d: supply Amount, or give "+
					"Number of Payments / Stop Date so the amount can be solved.", rowNum)))
			return
		}
		if p.PerYr <= 0 {
			writeAmortFieldErr(w, newFieldError("AMORT_PREPAY_INCOMPLETE", "prepayment",
				rowNum, []string{"Pmts/Yr"},
				fmt.Sprintf("Prepayment row %d: Pmts/Yr is required.", rowNum)))
			return
		}
		start, err := parseAPIDate(p.StartDate)
		if err != nil {
			writeAmortFieldErr(w, newFieldError("AMORT_PREPAY_BADDATE", "prepayment",
				rowNum, []string{"Start Date"},
				fmt.Sprintf("Prepayment row %d: invalid Start Date (use YYYY-MM-DD).", rowNum)))
			return
		}
		row := amortization.Prepayment{
			StartDateStatus: types.InOutInput,
			StartDate:       types.NewDateRec(start.Year(), start.Month(), start.Day()),
			PerYrStatus:     types.InOutInput,
			PerYr:           p.PerYr,
			NextDate:        types.NewDateRec(start.Year(), start.Month(), start.Day()),
		}
		// A nil Amount leaves PaymentStatus empty so the engine solves
		// for the prepayment amount (AO9).
		if p.Amount != nil {
			row.PaymentStatus = types.InOutInput
			row.Payment = *p.Amount
		}
		if p.NPmts > 0 {
			row.NNStatus = types.InOutInput
			row.NN = p.NPmts
		}
		if p.StopDate != "" {
			stop, err := parseAPIDate(p.StopDate)
			if err != nil {
				writeAmortFieldErr(w, newFieldError("AMORT_PREPAY_BADDATE", "prepayment",
					rowNum, []string{"Stop Date"},
					fmt.Sprintf("Prepayment row %d: invalid Stop Date (use YYYY-MM-DD).", rowNum)))
				return
			}
			row.StopDateStatus = types.InOutInput
			row.StopDate = types.NewDateRec(stop.Year(), stop.Month(), stop.Day())
		}
		input.Prepayments = append(input.Prepayments, row)
		advanced = true
	}

	for i, b := range req.Balloons {
		rowNum := i + 1
		if b.Date == "" {
			writeAmortFieldErr(w, newFieldError("AMORT_BALLOON_INCOMPLETE", "balloon",
				rowNum, []string{"Date"},
				fmt.Sprintf("Balloon row %d: Date is required.", rowNum)))
			return
		}
		d, err := parseAPIDate(b.Date)
		if err != nil {
			writeAmortFieldErr(w, newFieldError("AMORT_BALLOON_BADDATE", "balloon",
				rowNum, []string{"Date"},
				fmt.Sprintf("Balloon row %d: invalid Date (use YYYY-MM-DD).", rowNum)))
			return
		}
		bp := amortization.BalloonPayment{
			DateStatus: types.InOutInput,
			Date:       types.NewDateRec(d.Year(), d.Month(), d.Day()),
		}
		// A nil Amount marks an unknown ("target") balloon: the engine
		// solves the amount that clears the loan at the schedule end.
		if b.Amount != nil {
			bp.AmountStatus = types.InOutInput
			bp.Amount = *b.Amount
		}
		input.Balloons = append(input.Balloons, bp)
		advanced = true
	}

	for i, a := range req.Adjustments {
		rowNum := i + 1
		if a.Date == "" {
			writeAmortFieldErr(w, newFieldError("AMORT_ADJ_INCOMPLETE", "adjustment",
				rowNum, []string{"Date"},
				fmt.Sprintf("Adjustment row %d: Date is required.", rowNum)))
			return
		}
		// A date-only adjustment row (no new Rate, no new Pmt Amount)
		// is DOS AO7 "re-amortize at current rate" — the engine
		// re-solves the regular payment over the remaining term at
		// the unchanged rate. This is now handled by the same
		// engine branch as AO5; the API just forwards the row.
		d, err := parseAPIDate(a.Date)
		if err != nil {
			writeAmortFieldErr(w, newFieldError("AMORT_ADJ_BADDATE", "adjustment",
				rowNum, []string{"Date"},
				fmt.Sprintf("Adjustment row %d: invalid Date (use YYYY-MM-DD).", rowNum)))
			return
		}
		row := amortization.RateAdjustment{
			DateStatus: types.InOutInput,
			Date:       types.NewDateRec(d.Year(), d.Month(), d.Day()),
		}
		if a.Rate != nil {
			row.LoanRateStatus = types.InOutInput
			row.LoanRate = *a.Rate
		}
		if a.Amount != nil {
			row.AmountStatus = types.InOutInput
			row.Amount = *a.Amount
			row.AmtOK = true
		}
		input.Adjustments = append(input.Adjustments, row)
		advanced = true
	}

	if req.Moratorium != nil && *req.Moratorium != "" {
		m, err := parseAPIDate(*req.Moratorium)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, AmortizationResponse{
				Error: "Moratorium Date is unparseable — use MM/DD/YYYY (or ISO YYYY-MM-DD)."})
			return
		}
		input.Moratorium = amortization.Moratorium{
			FirstRepayStatus: types.InOutInput,
			FirstRepay:       types.NewDateRec(m.Year(), m.Month(), m.Day()),
		}
		advanced = true
	}

	if req.TargetAmt != nil {
		input.Target = amortization.Target{
			TargetStatus: types.InOutInput,
			TargetValue:  *req.TargetAmt,
		}
		advanced = true
	}

	if req.SkipMonths != "" {
		monthSet, err := amortization.MonthSetFromString(req.SkipMonths)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, AmortizationResponse{
				Error: "Skip Months is not valid (" + err.Error() + "). Use a " +
					"comma-separated list of months or ranges, e.g. \"6-8,12\" to " +
					"skip June through August and December."})
			return
		}
		input.SkipMonths = amortization.SkipMonths{
			SkipStatus: types.InOutInput,
			SkipStr:    req.SkipMonths,
			MonthSet:   monthSet,
		}
		advanced = true
	}

	if advanced {
		input.Fancy = true
	}

	// Track convergence flags from the backward solvers across the
	// whole handler so the response can surface a "did not converge"
	// warning. Default true: a non-solve request is trivially "converged".
	amountConverged, rateConverged := true, true

	// Field-presence dispatch: solve for whichever of {Amount, Rate}
	// the caller left blank, restoring the DOS backward-solve. Both
	// blank is the derive-only case handled earlier, so here exactly
	// one is blank. The solved value is written back into input as an
	// "input" field so the schedule engine runs normally afterward.
	if req.Amount == nil || req.Rate == nil {
		// The backward solvers need a fully-derived term and
		// first-payment date — the derivation FirstPass normally does
		// inside Amortize. Run FirstPass here on a COPY of the loan so
		// the solvers see the derived {firstDate, lastDate, nPeriods};
		// the original input is left untouched for the Amortize call
		// below, which runs its own FirstPass.
		solverInput := input
		solverLoan := input.Loan
		if err := amortization.FirstPass(&solverLoan); err != nil {
			writeJSON(w, http.StatusOK, AmortizationResponse{Error: err.Error()})
			return
		}
		// FirstPass marks fields it DERIVED with InOutOutput status.
		// The solver guards (CanComputeRate / CanComputeLoanAmount)
		// require InOutDefault or higher to count a field as "known",
		// so promote the status of any derived-and-now-known field on
		// this throwaway copy. A field FirstPass could not derive
		// keeps StatusEmpty and still (correctly) fails the guard.
		if solverLoan.FirstStatus > types.StatusEmpty &&
			solverLoan.FirstStatus < types.InOutDefault {
			solverLoan.FirstStatus = types.InOutDefault
		}
		if solverLoan.NPeriods > 0 && solverLoan.NStatus > types.StatusEmpty &&
			solverLoan.NStatus < types.InOutDefault {
			solverLoan.NStatus = types.InOutDefault
		}
		solverInput.Loan = solverLoan

		if req.Amount == nil {
			solved, conv, err := amortization.SolveLoanAmount(solverInput)
			if err != nil {
				writeJSON(w, http.StatusOK, AmortizationResponse{
					Error: "Amount Borrowed is blank and could not be solved (" + err.Error() + ")"})
				return
			}
			input.Loan.AmountStatus = types.InOutInput
			input.Loan.Amount = solved
			amountConverged = conv
		}
		if req.Rate == nil {
			solved, conv, err := amortization.SolveRate(solverInput)
			if err != nil {
				writeJSON(w, http.StatusOK, AmortizationResponse{
					Error: "Rate is blank and could not be solved (" + err.Error() + ")"})
				return
			}
			input.Loan.LoanRateStatus = types.InOutInput
			input.Loan.LoanRate = solved
			rateConverged = conv
		}
	}

	result := amortization.Amortize(input)
	resp := AmortizationResponse{
		TotalPaid: result.TotalPaid,
		TotalInt:  result.TotalInt,
		NPeriods:  result.NPeriods,
		// Echo the principal and loan rate the engine used — these
		// carry the solved value back to the caller when Amount or
		// Rate was left blank for the backward solver.
		Amount: input.Loan.Amount,
		Rate:   input.Loan.LoanRate,
		// APR is populated only when the request supplied Points.
		APR:          result.APR,
		APRConverged: result.APRConverged,
		Warnings:     result.Warnings,
	}
	if basisCoerced {
		resp.Warnings = append(resp.Warnings,
			"Switched to a 365-day basis for weekly/biweekly payments.")
	}
	// Surface non-convergence from the backward solvers. Matches the
	// DOS MessageBox at AMORTOP.pas:1488 ("Computation of payment
	// amount or interest rate did not converge."). The echoed Amount /
	// Rate is the best-seen estimate so the user can still inspect a
	// schedule; the warning lets them know the value may be off.
	if !amountConverged {
		resp.Warnings = append(resp.Warnings,
			"Amount Borrowed solve did not converge — the value shown is the closest "+
				"the iterative refinement reached. Try adjusting the Prepayments or "+
				"Adjustments rows, or enter Amount Borrowed directly.")
	}
	if !rateConverged {
		resp.Warnings = append(resp.Warnings,
			"Loan Rate solve did not converge — the value shown is the closest the "+
				"iterative refinement reached. Try adjusting the Prepayments or "+
				"Adjustments rows, or enter Loan Rate directly.")
	}
	// Result-sanity advisories on the backward solves (A-W1 / A-W3,
	// docs/result_warning_layer_spec.md). The non-convergence warnings
	// above cover A-W2.
	if req.Amount == nil && input.Loan.Amount <= 0 {
		resp.Warnings = append(resp.Warnings, types.FormatAdvisory(types.AdvisoryTier, "A-W3",
			[]string{"amount"},
			"Amount Borrowed solved to a non-positive value — the payment can't support a "+
				"positive loan at this rate and term."))
	}
	if req.Rate == nil && input.Loan.LoanRate <= 0 {
		resp.Warnings = append(resp.Warnings, types.FormatAdvisory(types.AdvisoryTier, "A-W1",
			[]string{"rate"},
			"The payment is below principal ÷ number of payments, so the implied Rate is "+
				"zero or negative. Check the Pmt Amount or the term."))
	}
	// Echo the engine's actual {firstDate, lastDate} only when they
	// were derivable. On error paths FirstPass may not have run, in
	// which case DateOK rejects the zero-time sentinel and we leave
	// the response fields empty so the UI doesn't render junk.
	if dateutil.DateOK(result.FirstDate) {
		resp.FirstDate = result.FirstDate.Time.Format("2006-01-02")
	}
	if dateutil.DateOK(result.LastDate) {
		resp.LastDate = result.LastDate.Time.Format("2006-01-02")
	}
	if result.Err != nil {
		resp.Error = result.Err.Error()
	}
	for _, rec := range result.Schedule {
		resp.Schedule = append(resp.Schedule, PaymentLine{
			PayNum:    rec.PayNum,
			Date:      rec.Date.Time.Format("2006-01-02"),
			Payment:   interest.Round2(rec.PayAmt),
			Interest:  interest.Round2(rec.Interest),
			Principal: interest.Round2(rec.Principal),
			IntToDate: interest.Round2(rec.IntToDate),
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleAmortizationDeriveOnly answers the "how many payments is that?"
// question without producing a full schedule. Used when the caller omits
// both Amount and Rate — the natural derive-only signal, since neither
// is needed to count periods between two dates. The body of the response
// carries only {nPeriods, firstDate, lastDate}; schedule, totals, and
// payment fields are intentionally left zero/empty.
//
// Matches Help/Amortization Example 1c. The engine work is just
// amortization.FirstPass; we skip Amortize entirely.
func handleAmortizationDeriveOnly(w http.ResponseWriter, req AmortizationRequest) {
	if req.PerYr <= 0 {
		writeJSON(w, http.StatusOK, AmortizationResponse{Error: "Pmts/Yr is required — enter how many payments are made per year " +
			"(12 for monthly, 4 for quarterly, 1 for annual) so the term can be counted."})
		return
	}

	loan := amortization.Loan{
		PerYrStatus: types.InOutInput,
		PerYr:       req.PerYr,
		// Date fields default to UnknownDate so dateutil.DateOK
		// rejects them until explicitly set below.
		LoanDate:  types.UnknownDate(),
		FirstDate: types.UnknownDate(),
		LastDate:  types.UnknownDate(),
	}

	if req.LoanDate != "" {
		ld, err := parseAPIDate(req.LoanDate)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: "Loan Date is unparseable — use MM/DD/YYYY (or ISO YYYY-MM-DD)"})
			return
		}
		loan.LoanDateStatus = types.InOutInput
		loan.LoanDate = types.NewDateRec(ld.Year(), ld.Month(), ld.Day())
	}
	if req.FirstDate != "" {
		fd, err := parseAPIDate(req.FirstDate)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: "1st Pmt Date is unparseable — use MM/DD/YYYY (or ISO YYYY-MM-DD)"})
			return
		}
		loan.FirstStatus = types.InOutInput
		loan.FirstDate = types.NewDateRec(fd.Year(), fd.Month(), fd.Day())
	}
	if req.LastDate != "" {
		ld, err := parseAPIDate(req.LastDate)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: "Last Pmt Date is unparseable — use MM/DD/YYYY (or ISO YYYY-MM-DD)"})
			return
		}
		loan.LastStatus = types.InOutInput
		loan.LastDate = types.NewDateRec(ld.Year(), ld.Month(), ld.Day())
	}
	if req.NPeriods > 0 {
		loan.NStatus = types.InOutInput
		loan.NPeriods = req.NPeriods
	}

	if err := amortization.FirstPass(&loan); err != nil {
		writeJSON(w, http.StatusOK, AmortizationResponse{Error: err.Error()})
		return
	}

	// FirstPass succeeded but couldn't derive a positive term — the
	// caller didn't supply enough siblings (e.g. just a loanDate with
	// no nPeriods and no lastDate). Make the error specific so the UI
	// can tell the user what to add.
	if loan.NPeriods <= 0 {
		writeJSON(w, http.StatusOK, AmortizationResponse{Error: "Not enough inputs to count the term: the inputs given are " +
			"insufficient. Supply either # Periods, or both the 1st Pmt Date and " +
			"the Last Pmt Date, and Per%Sense will derive the rest."})
		return
	}

	resp := AmortizationResponse{NPeriods: loan.NPeriods}
	if dateutil.DateOK(loan.FirstDate) {
		resp.FirstDate = loan.FirstDate.Time.Format("2006-01-02")
	}
	if dateutil.DateOK(loan.LastDate) {
		resp.LastDate = loan.LastDate.Time.Format("2006-01-02")
	}
	writeJSON(w, http.StatusOK, resp)
}

// HandlePVCalc handles POST /api/presentvalue/calc
func HandlePVCalc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PVRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, PVResponse{Error: "invalid JSON: " + err.Error()})
		return
	}

	// COLA escalation mode: 99 = anniversary (default), 98 =
	// continuous, 1-12 = a specific calendar month.
	colaMonth := types.COLAAnnual
	if req.COLAMonth == int(types.COLAContinuous) ||
		(req.COLAMonth >= 1 && req.COLAMonth <= 12) {
		colaMonth = byte(req.COLAMonth)
	}
	settings := presentvalue.PVSettings{
		Basis:     types.Basis360,
		PerYr:     12,
		COLAMonth: colaMonth,
		Exact:     false,
		YrDays:    360,
		YrInv:     1.0 / 360,
	}

	input := presentvalue.PVInput{Settings: settings}

	// As-of date is optional (omit to solve for it).
	if req.AsOfDate != nil && *req.AsOfDate != "" {
		asOf, err := parseAPIDate(*req.AsOfDate)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, PVResponse{
				Error: "As-of Date is unparseable — use MM/DD/YYYY (or ISO YYYY-MM-DD). " +
					"The As-of Date is the date all values are discounted to."})
			return
		}
		input.PresVal.AsOfStatus = types.InOutInput
		input.PresVal.AsOf = types.NewDateRec(asOf.Year(), asOf.Month(), asOf.Day())
	}

	// Rate is optional (omit to solve for it).
	if req.Rate != nil {
		input.PresVal.R = presentvalue.RateEntry{
			Status: types.StatusFromRate,
			Rate:   *req.Rate,
		}
	}

	// SumValue presence flips the screen into backward mode.
	if req.SumValue != nil {
		input.PresVal.SumValueStatus = types.InOutInput
		input.PresVal.SumValue = *req.SumValue
	}

	for i, ls := range req.LumpSums {
		row := presentvalue.LumpSumPayment{
			Act: actuarial.ContingencyFromCode(ls.Act),
		}
		if ls.Date != nil && *ls.Date != "" {
			d, err := parseAPIDate(*ls.Date)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, PVResponse{Error: fmt.Sprintf(
					"Lump Sum row %d: the Date is unparseable — use MM/DD/YYYY "+
						"(or ISO YYYY-MM-DD).", i+1)})
				return
			}
			row.DateStatus = types.InOutInput
			row.Date = types.NewDateRec(d.Year(), d.Month(), d.Day())
		}
		if ls.Amount != nil {
			row.AmtStatus = types.InOutInput
			row.Amt = *ls.Amount
		}
		if ls.Value != nil {
			row.ValStatus = types.InOutInput
			row.Val = *ls.Value
		}
		input.LumpSums = append(input.LumpSums, row)
	}

	for i, pp := range req.Periodics {
		row := presentvalue.PeriodicPayment{
			Act: actuarial.ContingencyFromCode(pp.Act),
		}
		if pp.FromDate != nil && *pp.FromDate != "" {
			from, err := parseAPIDate(*pp.FromDate)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, PVResponse{Error: fmt.Sprintf(
					"Periodic row %d: the From Date is unparseable — use "+
						"MM/DD/YYYY (or ISO YYYY-MM-DD).", i+1)})
				return
			}
			row.FromDateStatus = types.InOutInput
			row.FromDate = types.NewDateRec(from.Year(), from.Month(), from.Day())
		}
		if pp.ToDate != nil && *pp.ToDate != "" {
			to, err := parseAPIDate(*pp.ToDate)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, PVResponse{Error: fmt.Sprintf(
					"Periodic row %d: the To Date is unparseable — use "+
						"MM/DD/YYYY (or ISO YYYY-MM-DD).", i+1)})
				return
			}
			row.ToDateStatus = types.InOutInput
			row.ToDate = types.NewDateRec(to.Year(), to.Month(), to.Day())
		}
		if pp.PerYr != nil {
			row.PerYrStatus = types.InOutInput
			row.PerYr = *pp.PerYr
		}
		if pp.Amount != nil {
			row.AmtStatus = types.InOutInput
			row.Amt = *pp.Amount
		}
		if pp.Value != nil {
			row.ValStatus = types.InOutInput
			row.Val = *pp.Value
		}
		if pp.COLA != nil {
			row.COLAStatus = types.InOutInput
			row.COLA = *pp.COLA
		}
		input.Periodics = append(input.Periodics, row)
	}

	// Build actuarial config if provided
	if req.Actuarial != nil {
		acfg, acErr := buildActuarialConfig(req.Actuarial)
		if acErr != nil {
			writeJSON(w, http.StatusBadRequest, PVResponse{Error: "actuarial: " + acErr.Error()})
			return
		}
		input.Actuarial = acfg
	}

	// A life contingency on any row requires the matching actuarial
	// configuration. The PV engine applies the survival weighting only when
	// Actuarial is non-nil (presentvalue/calc.go), so a contingent row without
	// a config would otherwise be valued as if non-contingent — a silent wrong
	// answer. Reject the request instead.
	if cErr := validateContingencyConfig(input); cErr != "" {
		writeJSON(w, http.StatusBadRequest, PVResponse{Error: cErr})
		return
	}

	// Variable-rate schedule (DOS PVL fancy). When present, the
	// engine ignores PresVal.Rate and discounts each cash flow
	// through this piecewise schedule. See PVRateLineReq for the
	// "first entry's Date is conceptually -infinity" convention.
	if len(req.RateSchedule) > 0 {
		schedule := make([]presentvalue.RateLine, 0, len(req.RateSchedule))
		for i, rl := range req.RateSchedule {
			if rl.Date == "" {
				writeJSON(w, http.StatusBadRequest, PVResponse{Error: fmt.Sprintf(
					"Variable-rate schedule row %d: the effective Date is required — "+
						"it is the date that rate takes over.", i+1)})
				return
			}
			d, err := parseAPIDate(rl.Date)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, PVResponse{Error: fmt.Sprintf(
					"Variable-rate schedule row %d: the Date is unparseable — use "+
						"MM/DD/YYYY (or ISO YYYY-MM-DD).", i+1)})
				return
			}
			schedule = append(schedule, presentvalue.RateLine{
				Date: types.NewDateRec(d.Year(), d.Month(), d.Day()),
				Rate: rl.TrueRate,
			})
		}
		input.RateSchedule = schedule
	}

	result := presentvalue.Calculate(input)
	resp := PVResponse{
		SumValue: result.SumValue,
		PODValue: result.PODValue,
		POD:      result.POD,
		Rate:     result.Rate,
		Warnings: result.Warnings,
	}
	// Echo the as-of date the engine used (carries the PV-9 solved
	// date back). Guard against the zero DateRec on forward / VR runs.
	if !result.AsOf.Time.IsZero() {
		resp.AsOfDate = result.AsOf.Time.Format("2006-01-02")
	}
	if result.Err != nil {
		resp.Error = result.Err.Error()
	}
	for _, ls := range result.LumpSums {
		resp.LumpSums = append(resp.LumpSums, PVLumpSumResp{
			Date:   ls.Date.Time.Format("2006-01-02"),
			Amount: ls.Amt,
			Value:  ls.Val,
			Prob:   ls.Prob,
		})
	}
	for _, pp := range result.Periodics {
		resp.Periodics = append(resp.Periodics, PVPeriodicResp{
			FromDate: pp.FromDate.Time.Format("2006-01-02"),
			ToDate:   pp.ToDate.Time.Format("2006-01-02"),
			Amount:   pp.Amt,
			Value:    pp.Val,
			Prob:     pp.Prob,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// buildActuarialConfig constructs an ActuarialConfig from the API request.
func buildActuarialConfig(req *PVActuarialReq) (*actuarial.ActuarialConfig, error) {
	if len(req.Table1) == 0 || req.DOB1 == "" || req.AsOfNow == "" {
		return nil, fmt.Errorf("the life-contingency calculation needs Person 1's " +
			"life table, date of birth, and the as-of date. Fill in all three, or " +
			"remove the actuarial settings to run an ordinary present value")
	}

	table1Data, err := json.Marshal(req.Table1)
	if err != nil {
		return nil, err
	}
	table1, err := actuarial.ParseJSON("Person 1", table1Data)
	if err != nil {
		return nil, fmt.Errorf("Person 1's life table could not be loaded: %w", err)
	}

	dob1, err := parseAPIDate(req.DOB1)
	if err != nil {
		return nil, fmt.Errorf("Person 1's Date of Birth is unparseable — use " +
			"MM/DD/YYYY (or ISO YYYY-MM-DD)")
	}

	now, err := parseAPIDate(req.AsOfNow)
	if err != nil {
		return nil, fmt.Errorf("the actuarial As-of (today's) date is unparseable " +
			"— use MM/DD/YYYY (or ISO YYYY-MM-DD); it is the date the ages are " +
			"measured from")
	}

	cfg := &actuarial.ActuarialConfig{
		Table1: table1,
		DOB1:   types.NewDateRec(dob1.Year(), dob1.Month(), dob1.Day()),
		Now:    types.NewDateRec(now.Year(), now.Month(), now.Day()),
	}
	// A nil POD asks the engine to solve for the death benefit.
	if req.POD != nil {
		cfg.POD = *req.POD
	} else {
		cfg.PODUnknown = true
	}

	if len(req.Table2) > 0 && req.DOB2 != "" {
		table2Data, err := json.Marshal(req.Table2)
		if err != nil {
			return nil, err
		}
		table2, err := actuarial.ParseJSON("Person 2", table2Data)
		if err != nil {
			return nil, fmt.Errorf("Person 2's life table could not be loaded: %w", err)
		}
		dob2, err := parseAPIDate(req.DOB2)
		if err != nil {
			return nil, fmt.Errorf("Person 2's Date of Birth is unparseable — use " +
				"MM/DD/YYYY (or ISO YYYY-MM-DD)")
		}
		cfg.Table2 = table2
		cfg.DOB2 = types.NewDateRec(dob2.Year(), dob2.Month(), dob2.Day())
	}

	return cfg, nil
}

// validateContingencyConfig enforces that any life contingency used on a
// payment row has the actuarial inputs it needs. Any contingency requires
// Person 1's table, date of birth, and the reference date (carried in
// input.Actuarial); the two-life contingencies additionally require Person 2.
// Without a config the engine values the row as non-contingent rather than
// erroring (presentvalue/calc.go applies LifeProb only when Actuarial != nil),
// so this guard turns that silent mis-valuation into a clear rejection.
//
// The two-life / second-life check delegates to
// presentvalue.CheckSecondLifeProvided so the API and the engine return the
// identical, row-named message whichever layer fires first. Returns "" when
// the request is consistent (including the all-None case).
func validateContingencyConfig(input presentvalue.PVInput) string {
	usesContingency := false
	for _, ls := range input.LumpSums {
		if ls.Act != actuarial.NotContingent {
			usesContingency = true
		}
	}
	for _, pp := range input.Periodics {
		if pp.Act != actuarial.NotContingent {
			usesContingency = true
		}
	}

	if !usesContingency {
		return ""
	}
	if input.Actuarial == nil {
		return "a payment row uses a life contingency, but no actuarial " +
			"configuration was supplied — set Person 1's life table, date of " +
			"birth, and the reference date, or set those rows' Life column to None"
	}
	if err := presentvalue.CheckSecondLifeProvided(input); err != nil {
		return err.Error()
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeAmortFieldErr returns a 400 with both the human-readable
// message and its structured FieldError form, so the frontend can
// highlight the offending cell directly (dispatch_gaps §4.3).
func writeAmortFieldErr(w http.ResponseWriter, fe *FieldError) {
	writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: fe.Message, ErrorDetail: fe})
}
