// Package api provides HTTP handlers for the Per%Sense financial calculation
// REST API. Each screen type (mortgage, amortization, present value) has
// dedicated endpoints for calculation and file import.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/finance/amortization"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/finance/mortgage"
	"github.com/persense/persense-port/internal/finance/presentvalue"
	"github.com/persense/persense-port/internal/types"
)

// --- Request/Response types ---

// MortgageRequest is the JSON input for a mortgage calculation.
type MortgageRequest struct {
	Price    *float64 `json:"price,omitempty"`
	Points   *float64 `json:"points,omitempty"`
	PctDown  *float64 `json:"pctDown,omitempty"`
	Cash     *float64 `json:"cash,omitempty"`
	Financed *float64 `json:"financed,omitempty"`
	Years    *int     `json:"years,omitempty"`
	Rate     *float64 `json:"rate,omitempty"`
	Tax      *float64 `json:"tax,omitempty"`
	Monthly  *float64 `json:"monthly,omitempty"`
	BalloonYears  *int     `json:"balloonYears,omitempty"`
	BalloonAmount *float64 `json:"balloonAmount,omitempty"`
}

// MortgageResponse is the JSON output for a mortgage calculation.
type MortgageResponse struct {
	Price    float64 `json:"price"`
	Points   float64 `json:"points"`
	PctDown  float64 `json:"pctDown"`
	Cash     float64 `json:"cash"`
	Financed float64 `json:"financed"`
	Years    int     `json:"years"`
	Rate     float64 `json:"rate"`
	Tax      float64 `json:"tax"`
	Monthly  float64 `json:"monthly"`
	BalloonYears  int     `json:"balloonYears,omitempty"`
	BalloonAmount float64 `json:"balloonAmount,omitempty"`
	APR           float64 `json:"apr,omitempty"`
	APRConverged  bool    `json:"aprConverged,omitempty"`
	Error    string  `json:"error,omitempty"`
}

// AmortizationRequest is the JSON input for an amortization calculation.
//
// The Advanced Options block (Prepayments, Balloons, Adjustments,
// Moratorium, Target, SkipMonths) is optional. When any of those is
// supplied, the engine runs in fancy mode automatically.
type AmortizationRequest struct {
	Amount    float64 `json:"amount"`
	LoanDate  string  `json:"loanDate"`  // YYYY-MM-DD
	Rate      float64 `json:"rate"`
	FirstDate string  `json:"firstDate"` // YYYY-MM-DD
	NPeriods  int     `json:"nPeriods"`
	PerYr     int     `json:"perYr"`
	Payment   float64 `json:"payment,omitempty"`
	Basis     string  `json:"basis,omitempty"` // "360", "365", "365/360"

	// --- Advanced Options ---
	Prepayments []AmortPrepaymentReq `json:"prepayments,omitempty"`
	Balloons    []AmortBalloonReq    `json:"balloons,omitempty"`
	Adjustments []AmortAdjustmentReq `json:"adjustments,omitempty"`
	Moratorium  *string              `json:"moratorium,omitempty"`  // YYYY-MM-DD
	TargetAmt   *float64             `json:"targetAmt,omitempty"`
	SkipMonths  string               `json:"skipMonths,omitempty"`  // e.g. "6-8,12"
}

// AmortPrepaymentReq is one extra-payment series in an amortization
// request. Each series adds PerYr extra payments per year between
// StartDate and StopDate (or until NPmts payments have been made).
type AmortPrepaymentReq struct {
	StartDate string  `json:"startDate"`         // YYYY-MM-DD
	NPmts     int     `json:"nPmts,omitempty"`   // number of extra payments
	StopDate  string  `json:"stopDate,omitempty"` // YYYY-MM-DD; alt to NPmts
	PerYr     int     `json:"perYr"`
	Amount    float64 `json:"amount"`
}

// AmortBalloonReq is one balloon payment in an amortization request.
type AmortBalloonReq struct {
	Date   string  `json:"date"`   // YYYY-MM-DD
	Amount float64 `json:"amount"`
}

// AmortAdjustmentReq is one rate / payment adjustment in an
// amortization request. Either Rate or Amount (or both) can be set.
type AmortAdjustmentReq struct {
	Date   string   `json:"date"`             // YYYY-MM-DD
	Rate   *float64 `json:"rate,omitempty"`
	Amount *float64 `json:"amount,omitempty"`
}

// AmortizationResponse is the JSON output for an amortization schedule.
type AmortizationResponse struct {
	Schedule  []PaymentLine `json:"schedule"`
	TotalPaid float64       `json:"totalPaid"`
	TotalInt  float64       `json:"totalInterest"`
	Error     string        `json:"error,omitempty"`
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
	Table1 [][]float64 `json:"table1"`           // [[age, qx], ...]
	DOB1   string      `json:"dob1"`             // YYYY-MM-DD
	Table2 [][]float64 `json:"table2,omitempty"`  // optional second life
	DOB2   string      `json:"dob2,omitempty"`
	AsOfNow string     `json:"asOfNow"`          // reference "now" date
	POD    float64     `json:"pod,omitempty"`     // payment on death amount
}

// PVResponse is the JSON output for a present value calculation.
type PVResponse struct {
	SumValue  float64            `json:"sumValue"`
	PODValue  float64            `json:"podValue,omitempty"`
	LumpSums  []PVLumpSumResp    `json:"lumpSums,omitempty"`
	Periodics []PVPeriodicResp   `json:"periodics,omitempty"`
	Error     string             `json:"error,omitempty"`
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
		Rate:     mortgage.TrueRateToLoanRate(result.Line.Rate),
		Tax:      result.Line.Tax,
		Monthly:  result.Line.Monthly,
		BalloonYears:  result.Line.When,
		BalloonAmount: result.Line.HowMuch,
	}

	if result.Err != nil {
		resp.Error = result.Err.Error()
	} else if mortgage.EnoughDataForAPR(&result.Line) {
		apr, conv, _ := mortgage.FullTermAPR(result.Line, 365.25)
		// FullTermAPR converts its internal true-rate iterate back
		// to a yield at monthly compounding (via YieldFromRate) so
		// the returned value is already a loan rate — no further
		// conversion needed here.
		resp.APR = apr
		resp.APRConverged = conv
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

	loanDate, err := time.Parse("2006-01-02", req.LoanDate)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: "invalid loanDate format, use YYYY-MM-DD"})
		return
	}

	basis := types.Basis360
	switch req.Basis {
	case "365":
		basis = types.Basis365
	case "365/360":
		basis = types.Basis365360
	}

	ctx := interest.NewCalcContext(basis, byte(req.PerYr))
	input := amortization.LoanInput{
		Loan: amortization.Loan{
			AmountStatus:   types.InOutInput,
			Amount:         req.Amount,
			LoanDateStatus: types.InOutInput,
			LoanDate:       types.NewDateRec(loanDate.Year(), loanDate.Month(), loanDate.Day()),
			LoanRateStatus: types.InOutInput,
			LoanRate:       req.Rate,
			NStatus:        types.InOutInput,
			NPeriods:       req.NPeriods,
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
			Basis:   basis,
			PerYr:   byte(req.PerYr),
			Prepaid: true,
			YrDays:  ctx.YrDays,
			YrInv:   ctx.YrInv,
		},
	}
	// FirstDate is optional. If omitted, amortization.FirstPass will
	// derive it as loanDate + 1 period (DOS A-FP-defFirst).
	if req.FirstDate != "" {
		firstDate, err := time.Parse("2006-01-02", req.FirstDate)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: "invalid firstDate format, use YYYY-MM-DD"})
			return
		}
		input.Loan.FirstStatus = types.InOutInput
		input.Loan.FirstDate = types.NewDateRec(firstDate.Year(), firstDate.Month(), firstDate.Day())
	} else {
		input.Loan.FirstDate = types.UnknownDate()
	}
	if req.Payment == 0 {
		input.Loan.PayAmtStatus = types.StatusEmpty
	}

	// --- Advanced Options ---
	advanced := false

	for _, p := range req.Prepayments {
		start, err := time.Parse("2006-01-02", p.StartDate)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: "invalid prepayment startDate"})
			return
		}
		row := amortization.Prepayment{
			StartDateStatus: types.InOutInput,
			StartDate:       types.NewDateRec(start.Year(), start.Month(), start.Day()),
			PerYrStatus:     types.InOutInput,
			PerYr:           p.PerYr,
			PaymentStatus:   types.InOutInput,
			Payment:         p.Amount,
			NextDate:        types.NewDateRec(start.Year(), start.Month(), start.Day()),
		}
		if p.NPmts > 0 {
			row.NNStatus = types.InOutInput
			row.NN = p.NPmts
		}
		if p.StopDate != "" {
			stop, err := time.Parse("2006-01-02", p.StopDate)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: "invalid prepayment stopDate"})
				return
			}
			row.StopDateStatus = types.InOutInput
			row.StopDate = types.NewDateRec(stop.Year(), stop.Month(), stop.Day())
		}
		input.Prepayments = append(input.Prepayments, row)
		advanced = true
	}

	for _, b := range req.Balloons {
		d, err := time.Parse("2006-01-02", b.Date)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: "invalid balloon date"})
			return
		}
		input.Balloons = append(input.Balloons, amortization.BalloonPayment{
			DateStatus:   types.InOutInput,
			Date:         types.NewDateRec(d.Year(), d.Month(), d.Day()),
			AmountStatus: types.InOutInput,
			Amount:       b.Amount,
		})
		advanced = true
	}

	for _, a := range req.Adjustments {
		d, err := time.Parse("2006-01-02", a.Date)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: "invalid adjustment date"})
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
		m, err := time.Parse("2006-01-02", *req.Moratorium)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: "invalid moratorium date"})
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
			writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: "invalid skipMonths: " + err.Error()})
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

	result := amortization.Amortize(input)
	resp := AmortizationResponse{
		TotalPaid: result.TotalPaid,
		TotalInt:  result.TotalInt,
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

	settings := presentvalue.PVSettings{
		Basis:     types.Basis360,
		PerYr:     12,
		COLAMonth: types.COLAAnnual,
		Exact:     false,
		YrDays:    360,
		YrInv:     1.0 / 360,
	}

	input := presentvalue.PVInput{Settings: settings}

	// As-of date is optional (omit to solve for it).
	if req.AsOfDate != nil && *req.AsOfDate != "" {
		asOf, err := time.Parse("2006-01-02", *req.AsOfDate)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, PVResponse{Error: "invalid asOfDate format"})
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

	for _, ls := range req.LumpSums {
		row := presentvalue.LumpSumPayment{
			Act: actuarial.ContingencyFromCode(ls.Act),
		}
		if ls.Date != nil && *ls.Date != "" {
			d, err := time.Parse("2006-01-02", *ls.Date)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, PVResponse{Error: "invalid lump sum date"})
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

	for _, pp := range req.Periodics {
		row := presentvalue.PeriodicPayment{
			Act: actuarial.ContingencyFromCode(pp.Act),
		}
		if pp.FromDate != nil && *pp.FromDate != "" {
			from, err := time.Parse("2006-01-02", *pp.FromDate)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, PVResponse{Error: "invalid fromDate"})
				return
			}
			row.FromDateStatus = types.InOutInput
			row.FromDate = types.NewDateRec(from.Year(), from.Month(), from.Day())
		}
		if pp.ToDate != nil && *pp.ToDate != "" {
			to, err := time.Parse("2006-01-02", *pp.ToDate)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, PVResponse{Error: "invalid toDate"})
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

	result := presentvalue.Calculate(input)
	resp := PVResponse{SumValue: result.SumValue, PODValue: result.PODValue}
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
		return nil, fmt.Errorf("table1, dob1, and asOfNow are required")
	}

	table1Data, err := json.Marshal(req.Table1)
	if err != nil {
		return nil, err
	}
	table1, err := actuarial.ParseJSON("Person 1", table1Data)
	if err != nil {
		return nil, fmt.Errorf("table1: %w", err)
	}

	dob1, err := time.Parse("2006-01-02", req.DOB1)
	if err != nil {
		return nil, fmt.Errorf("dob1: invalid date")
	}

	now, err := time.Parse("2006-01-02", req.AsOfNow)
	if err != nil {
		return nil, fmt.Errorf("asOfNow: invalid date")
	}

	cfg := &actuarial.ActuarialConfig{
		Table1: table1,
		DOB1:   types.NewDateRec(dob1.Year(), dob1.Month(), dob1.Day()),
		Now:    types.NewDateRec(now.Year(), now.Month(), now.Day()),
		POD:    req.POD,
	}

	if len(req.Table2) > 0 && req.DOB2 != "" {
		table2Data, err := json.Marshal(req.Table2)
		if err != nil {
			return nil, err
		}
		table2, err := actuarial.ParseJSON("Person 2", table2Data)
		if err != nil {
			return nil, fmt.Errorf("table2: %w", err)
		}
		dob2, err := time.Parse("2006-01-02", req.DOB2)
		if err != nil {
			return nil, fmt.Errorf("dob2: invalid date")
		}
		cfg.Table2 = table2
		cfg.DOB2 = types.NewDateRec(dob2.Year(), dob2.Month(), dob2.Day())
	}

	return cfg, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
