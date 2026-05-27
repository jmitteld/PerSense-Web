package api

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/persense/persense-port/internal/fileio"
	"github.com/persense/persense-port/internal/types"
)

// PSNImportResponse is the JSON returned by /api/import/psn.
//
// Exactly one of Mortgage / Amortization / PresentValue will be set,
// matching the screen the file came from. The Screen field is a short
// label the frontend uses to switch to the right view ("mortgage",
// "amortization", "presentvalue").
type PSNImportResponse struct {
	Screen       string                 `json:"screen,omitempty"`
	Mortgage     *PSNMortgagePayload    `json:"mortgage,omitempty"`
	Amortization *PSNAmortizationPayload `json:"amortization,omitempty"`
	PresentValue *PSNPresentValuePayload `json:"presentValue,omitempty"`
	Warnings     []string               `json:"warnings,omitempty"`
	Error        string                 `json:"error,omitempty"`
}

// PSNMortgagePayload mirrors the input fields of the mortgage screen
// for a single line. The frontend reconstructs multi-row state by
// calling Calculate Row on each line.
type PSNMortgagePayload struct {
	Lines []PSNMortgageLine `json:"lines"`
}

type PSNMortgageLine struct {
	Price    *float64 `json:"price,omitempty"`
	Points   *float64 `json:"points,omitempty"`
	PctDown  *float64 `json:"pctDown,omitempty"`
	Cash     *float64 `json:"cash,omitempty"`
	Financed *float64 `json:"financed,omitempty"`
	Years    *int     `json:"years,omitempty"`
	Rate     *float64 `json:"rate,omitempty"`
	Tax      *float64 `json:"tax,omitempty"`
	Monthly  *float64 `json:"monthly,omitempty"`
	Balloon  *float64 `json:"balloon,omitempty"`
}

// PSNAmortizationPayload mirrors the input fields of the amortization
// screen, including any Advanced Options the file carried.
type PSNAmortizationPayload struct {
	Amount     *float64                `json:"amount,omitempty"`
	LoanDate   string                  `json:"loanDate,omitempty"`
	FirstDate  string                  `json:"firstDate,omitempty"`
	LastDate   string                  `json:"lastDate,omitempty"`
	NPeriods   int                     `json:"nPeriods,omitempty"`
	PerYr      int                     `json:"perYr,omitempty"`
	Rate       *float64                `json:"rate,omitempty"`
	Payment    *float64                `json:"payment,omitempty"`
	Basis      string                  `json:"basis,omitempty"`
	Prepayments []PSNAmortPrepayment   `json:"prepayments,omitempty"`
	Balloons   []PSNAmortBalloon       `json:"balloons,omitempty"`
	Adjustments []PSNAmortAdjustment   `json:"adjustments,omitempty"`
	Moratorium string                  `json:"moratorium,omitempty"`
	TargetAmt  *float64                `json:"targetAmt,omitempty"`
	SkipMonths string                  `json:"skipMonths,omitempty"`
}

type PSNAmortPrepayment struct {
	StartDate string   `json:"startDate,omitempty"`
	StopDate  string   `json:"stopDate,omitempty"`
	PerYr     int      `json:"perYr,omitempty"`
	NPmts     int      `json:"nPmts,omitempty"`
	Amount    *float64 `json:"amount,omitempty"`
}

type PSNAmortBalloon struct {
	Date   string   `json:"date,omitempty"`
	Amount *float64 `json:"amount,omitempty"`
}

type PSNAmortAdjustment struct {
	Date    string   `json:"date,omitempty"`
	Rate    *float64 `json:"rate,omitempty"`
	Payment *float64 `json:"payment,omitempty"`
}

// PSNPresentValuePayload mirrors the input fields of the PV screen.
type PSNPresentValuePayload struct {
	LumpSums  []PSNPVLumpSum  `json:"lumpSums,omitempty"`
	Periodics []PSNPVPeriodic `json:"periodics,omitempty"`
}

type PSNPVLumpSum struct {
	Date   string   `json:"date,omitempty"`
	Amount *float64 `json:"amount,omitempty"`
	Value  *float64 `json:"value,omitempty"`
}

type PSNPVPeriodic struct {
	FromDate string   `json:"fromDate,omitempty"`
	ToDate   string   `json:"toDate,omitempty"`
	PerYr    int      `json:"perYr,omitempty"`
	Amount   *float64 `json:"amount,omitempty"`
	Value    *float64 `json:"value,omitempty"`
	COLA     *float64 `json:"cola,omitempty"`
}

// HandleImportPSN accepts a legacy Per%Sense binary file (extension
// .psn historically, though the handler doesn't enforce the
// extension) as the raw HTTP request body and returns the parsed
// contents as a JSON payload shaped like the matching screen's
// inputs.
//
// The handler auto-detects the file's screen via the first grid
// header byte: see fileio.FileType constants.
//
// Request:  POST /api/import/psn  body: raw .psn bytes
// Response: 200 OK + JSON PSNImportResponse on success
//           400 with {error: "..."} on parse failure
func HandleImportPSN(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Limit upload size to 256 KiB. Real .psn files are at most a few
	// kilobytes; this guards against accidental huge uploads.
	const maxBody = 256 << 10
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, PSNImportResponse{Error: "could not read request body: " + err.Error()})
		return
	}
	if len(body) > maxBody {
		writeJSON(w, http.StatusBadRequest, PSNImportResponse{Error: "file too large (max 256 KB)"})
		return
	}
	hdr, _, err := fileio.ReadBytes(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, PSNImportResponse{Error: "could not parse .psn header: " + err.Error()})
		return
	}

	switch hdr.FileType {
	case fileio.FileTypeMortgage:
		f, err := fileio.LoadMortgageBytes(body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, PSNImportResponse{Error: "mortgage file: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, PSNImportResponse{
			Screen:   "mortgage",
			Mortgage: psnMortgagePayload(f),
		})
	case fileio.FileTypeAmortization:
		f, err := fileio.LoadAmortizationBytes(body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, PSNImportResponse{Error: "amortization file: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, PSNImportResponse{
			Screen:       "amortization",
			Amortization: psnAmortizationPayload(f),
		})
	case fileio.FileTypePresentValue:
		f, err := fileio.LoadPresentValueBytes(body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, PSNImportResponse{Error: "present value file: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, PSNImportResponse{
			Screen:       "presentvalue",
			PresentValue: psnPresentValuePayload(f),
		})
	default:
		writeJSON(w, http.StatusBadRequest, PSNImportResponse{
			Error: fmt.Sprintf("unrecognised .psn file type (grid ID = %d)", hdr.Grids[0].GridID),
		})
	}
}

// --- payload builders -----------------------------------------------------

func fptr(s int8, v float64) *float64 {
	if s >= types.InOutDefault {
		x := v
		return &x
	}
	return nil
}

func iptr(s int8, v int) *int {
	if s >= types.InOutDefault {
		x := v
		return &x
	}
	return nil
}

func dateStr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

func psnMortgagePayload(f *fileio.MortgageFile) *PSNMortgagePayload {
	p := &PSNMortgagePayload{}
	for _, m := range f.Mortgages {
		p.Lines = append(p.Lines, PSNMortgageLine{
			Price:    fptr(m.PriceStatus, m.Price),
			Points:   fptr(m.PointsStatus, m.Points),
			PctDown:  fptr(m.PctStatus, m.Pct),
			Cash:     fptr(m.CashStatus, m.Cash),
			Financed: fptr(m.FinancedStatus, m.Financed),
			Years:    iptr(m.YearsStatus, m.Years),
			Rate:     fptr(m.RateStatus, m.Rate),
			Tax:      fptr(m.TaxStatus, m.Tax),
			Monthly:  fptr(m.MonthlyStatus, m.Monthly),
			Balloon:  fptr(m.HowMuchStatus, m.HowMuch),
		})
	}
	return p
}

func psnAmortizationPayload(f *fileio.AmortizationFile) *PSNAmortizationPayload {
	p := &PSNAmortizationPayload{
		Amount:    fptr(f.Loan.AmountStatus, f.Loan.Amount),
		LoanDate:  dateStr(f.Loan.LoanDate.Time),
		FirstDate: dateStr(f.Loan.FirstDate.Time),
		LastDate:  dateStr(f.Loan.LastDate.Time),
		NPeriods:  f.Loan.NPeriods,
		PerYr:     f.Loan.PerYr,
		Rate:      fptr(f.Loan.LoanRateStatus, f.Loan.LoanRate),
		Payment:   fptr(f.Loan.PayAmtStatus, f.Loan.PayAmt),
	}
	for _, pp := range f.Prepayments {
		p.Prepayments = append(p.Prepayments, PSNAmortPrepayment{
			StartDate: dateStr(pp.StartDate.Time),
			StopDate:  dateStr(pp.StopDate.Time),
			PerYr:     pp.PerYr,
			NPmts:     pp.NN,
			Amount:    fptr(pp.PaymentStatus, pp.Payment),
		})
	}
	for _, b := range f.Balloons {
		p.Balloons = append(p.Balloons, PSNAmortBalloon{
			Date:   dateStr(b.Date.Time),
			Amount: fptr(b.AmountStatus, b.Amount),
		})
	}
	for _, a := range f.Adjustments {
		p.Adjustments = append(p.Adjustments, PSNAmortAdjustment{
			Date:    dateStr(a.Date.Time),
			Rate:    fptr(a.LoanRateStatus, a.LoanRate),
			Payment: fptr(a.AmountStatus, a.Amount),
		})
	}
	if f.Moratorium.FirstRepayStatus >= types.InOutDefault {
		p.Moratorium = dateStr(f.Moratorium.FirstRepay.Time)
	}
	if f.Target.TargetStatus >= types.InOutDefault {
		v := f.Target.TargetValue
		p.TargetAmt = &v
	}
	p.SkipMonths = f.SkipMonths.SkipStr
	return p
}

func psnPresentValuePayload(f *fileio.PresentValueFile) *PSNPresentValuePayload {
	p := &PSNPresentValuePayload{}
	for _, l := range f.LumpSums {
		p.LumpSums = append(p.LumpSums, PSNPVLumpSum{
			Date:   dateStr(l.Date.Time),
			Amount: fptr(l.AmtStatus, l.Amt),
			Value:  fptr(l.ValStatus, l.Val),
		})
	}
	for _, q := range f.Periodics {
		p.Periodics = append(p.Periodics, PSNPVPeriodic{
			FromDate: dateStr(q.FromDate.Time),
			ToDate:   dateStr(q.ToDate.Time),
			PerYr:    q.PerYr,
			Amount:   fptr(q.AmtStatus, q.Amt),
			Value:    fptr(q.ValStatus, q.Val),
			COLA:     fptr(q.COLAStatus, q.COLA),
		})
	}
	return p
}

