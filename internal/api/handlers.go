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
type AmortizationRequest struct {
	Amount    float64 `json:"amount"`
	LoanDate  string  `json:"loanDate"`  // YYYY-MM-DD
	Rate      float64 `json:"rate"`
	FirstDate string  `json:"firstDate"` // YYYY-MM-DD
	NPeriods  int     `json:"nPeriods"`
	PerYr     int     `json:"perYr"`
	Payment   float64 `json:"payment,omitempty"`
	Basis     string  `json:"basis,omitempty"` // "360", "365", "365/360"
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
type PVRequest struct {
	AsOfDate  string              `json:"asOfDate"` // YYYY-MM-DD
	Rate      float64             `json:"rate"`
	LumpSums  []PVLumpSumReq      `json:"lumpSums,omitempty"`
	Periodics []PVPeriodicReq     `json:"periodics,omitempty"`
	Actuarial *PVActuarialReq     `json:"actuarial,omitempty"`
}

// PVLumpSumReq represents a lump sum payment in a PV request.
type PVLumpSumReq struct {
	Date   string  `json:"date"`
	Amount float64 `json:"amount"`
	Act    string  `json:"act,omitempty"` // contingency: N,L,D,1,2,E,B
}

// PVPeriodicReq represents a periodic payment series in a PV request.
type PVPeriodicReq struct {
	FromDate string  `json:"fromDate"`
	ToDate   string  `json:"toDate"`
	PerYr    int     `json:"perYr"`
	Amount   float64 `json:"amount"`
	COLA     float64 `json:"cola,omitempty"`
	Act      string  `json:"act,omitempty"` // contingency: N,L,D,1,2,E,B
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
		m.Rate = *req.Rate
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
		Rate:     result.Line.Rate,
		Tax:      result.Line.Tax,
		Monthly:  result.Line.Monthly,
		BalloonYears:  result.Line.When,
		BalloonAmount: result.Line.HowMuch,
	}

	if result.Err != nil {
		resp.Error = result.Err.Error()
	} else if mortgage.EnoughDataForAPR(&result.Line) {
		apr, conv, _ := mortgage.FullTermAPR(result.Line, 365.25)
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
	firstDate, err := time.Parse("2006-01-02", req.FirstDate)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, AmortizationResponse{Error: "invalid firstDate format, use YYYY-MM-DD"})
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
			FirstStatus:    types.InOutInput,
			FirstDate:      types.NewDateRec(firstDate.Year(), firstDate.Month(), firstDate.Day()),
			NStatus:        types.InOutInput,
			NPeriods:       req.NPeriods,
			PerYrStatus:    types.InOutInput,
			PerYr:          req.PerYr,
			PayAmtStatus:   types.InOutInput,
			PayAmt:         req.Payment,
			LastOK:         true,
		},
		Settings: amortization.Settings{
			Basis:   basis,
			PerYr:   byte(req.PerYr),
			Prepaid: true,
			YrDays:  ctx.YrDays,
			YrInv:   ctx.YrInv,
		},
	}
	if req.Payment == 0 {
		input.Loan.PayAmtStatus = types.StatusEmpty
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

	asOf, err := time.Parse("2006-01-02", req.AsOfDate)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, PVResponse{Error: "invalid asOfDate format"})
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

	input := presentvalue.PVInput{
		PresVal: presentvalue.PresValLine{
			AsOfStatus: types.InOutInput,
			AsOf:       types.NewDateRec(asOf.Year(), asOf.Month(), asOf.Day()),
			R: presentvalue.RateEntry{
				Status: types.StatusFromRate,
				Rate:   req.Rate,
			},
		},
		Settings: settings,
	}

	for _, ls := range req.LumpSums {
		d, err := time.Parse("2006-01-02", ls.Date)
		if err != nil {
			continue
		}
		input.LumpSums = append(input.LumpSums, presentvalue.LumpSumPayment{
			DateStatus: types.InOutInput,
			Date:       types.NewDateRec(d.Year(), d.Month(), d.Day()),
			AmtStatus:  types.InOutInput,
			Amt:        ls.Amount,
			Act:        actuarial.ContingencyFromCode(ls.Act),
		})
	}

	for _, pp := range req.Periodics {
		from, err1 := time.Parse("2006-01-02", pp.FromDate)
		to, err2 := time.Parse("2006-01-02", pp.ToDate)
		if err1 != nil || err2 != nil {
			continue
		}
		colaStatus := int8(types.StatusEmpty)
		if pp.COLA != 0 {
			colaStatus = types.InOutInput
		}
		input.Periodics = append(input.Periodics, presentvalue.PeriodicPayment{
			FromDateStatus: types.InOutInput,
			FromDate:       types.NewDateRec(from.Year(), from.Month(), from.Day()),
			ToDateStatus:   types.InOutInput,
			ToDate:         types.NewDateRec(to.Year(), to.Month(), to.Day()),
			PerYrStatus:    types.InOutInput,
			PerYr:          pp.PerYr,
			AmtStatus:      types.InOutInput,
			Amt:            pp.Amount,
			COLAStatus:     colaStatus,
			COLA:           pp.COLA,
			Act:            actuarial.ContingencyFromCode(pp.Act),
		})
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
