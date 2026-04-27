package fileio

import (
	"fmt"
	"time"

	"github.com/persense/persense-port/internal/finance/amortization"
	"github.com/persense/persense-port/internal/finance/mortgage"
	"github.com/persense/persense-port/internal/finance/presentvalue"
	"github.com/persense/persense-port/internal/types"
)

// MortgageFile holds the parsed contents of a legacy mortgage file.
type MortgageFile struct {
	Header    FileHeader
	Mortgages []mortgage.MtgLine
}

// AmortizationFile holds the parsed contents of a legacy amortization file.
type AmortizationFile struct {
	Header      FileHeader
	Loan        amortization.Loan
	Payoff      amortization.BalloonPayment
	Prepayments []amortization.Prepayment
	Balloons    []amortization.BalloonPayment
	Adjustments []amortization.RateAdjustment
	Moratorium  amortization.Moratorium
	Target      amortization.Target
	SkipMonths  amortization.SkipMonths
}

// PresentValueFile holds the parsed contents of a legacy present value file.
type PresentValueFile struct {
	Header    FileHeader
	LumpSums  []presentvalue.LumpSumPayment
	Periodics []presentvalue.PeriodicPayment
	PresVals  []presentvalue.PresValLine
	Fancy     bool
}

// LoadMortgageFile reads and parses a legacy mortgage screen file.
//
// Ported from legacy/source/FileIOUnit.pas: TFileIO.LoadMortgageData
func LoadMortgageFile(path string) (*MortgageFile, error) {
	hdr, data, err := ReadFile(path)
	if err != nil {
		return nil, err
	}
	if hdr.FileType != FileTypeMortgage {
		return nil, fmt.Errorf("not a mortgage file: grid ID = %d", hdr.Grids[0].GridID)
	}

	lineCount := int(hdr.Grids[0].LineCount)
	result := &MortgageFile{Header: *hdr}

	pos := 0
	for i := 0; i < lineCount; i++ {
		var m mortgage.MtgLine
		m.PriceStatus = readInt8(data, &pos)
		m.Price = readReal48(data, &pos)
		m.PointsStatus = readInt8(data, &pos)
		m.Points = readReal48(data, &pos)
		m.PctStatus = readInt8(data, &pos)
		m.Pct = readReal48(data, &pos)
		m.CashStatus = readInt8(data, &pos)
		m.Cash = readReal48(data, &pos)
		m.FinancedStatus = readInt8(data, &pos)
		m.Financed = readReal48(data, &pos)
		m.YearsStatus = readInt8(data, &pos)
		m.Years = int(readInt16LE(data, &pos))
		m.RateStatus = readInt8(data, &pos)
		m.Rate = readReal48(data, &pos)
		m.TaxStatus = readInt8(data, &pos)
		m.Tax = readReal48(data, &pos)
		m.MonthlyStatus = readInt8(data, &pos)
		m.Monthly = readReal48(data, &pos)
		m.WhenStatus = readInt8(data, &pos)
		m.When = int(readInt16LE(data, &pos))
		m.HowMuchStatus = readInt8(data, &pos)
		m.HowMuch = readReal48(data, &pos)
		skip(&pos, 1) // balloon status byte (72)

		result.Mortgages = append(result.Mortgages, m)
	}

	return result, nil
}

// LoadAmortizationFile reads and parses a legacy amortization screen file.
//
// Ported from legacy/source/FileIOUnit.pas: TFileIO.LoadAmortizationData
func LoadAmortizationFile(path string) (*AmortizationFile, error) {
	hdr, data, err := ReadFile(path)
	if err != nil {
		return nil, err
	}
	if hdr.FileType != FileTypeAmortization {
		return nil, fmt.Errorf("not an amortization file: grid ID = %d", hdr.Grids[0].GridID)
	}

	result := &AmortizationFile{Header: *hdr}
	pos := 0

	for i := 0; i < 8; i++ {
		grid := hdr.Grids[i]
		switch grid.GridID {
		case types.BlockAMZTop:
			result.Loan = readAMZLoan(data, &pos)
		case types.BlockAMZBalance:
			result.Payoff = readBalloonPayment(data, &pos)
		case types.BlockAMZPre:
			for j := 0; j < int(grid.LineCount); j++ {
				result.Prepayments = append(result.Prepayments, readPrepayment(data, &pos))
			}
		case types.BlockAMZBalloon:
			for j := 0; j < int(grid.LineCount); j++ {
				result.Balloons = append(result.Balloons, readBalloonPayment(data, &pos))
			}
		case types.BlockAMZChanges:
			for j := 0; j < int(grid.LineCount); j++ {
				result.Adjustments = append(result.Adjustments, readAdjustment(data, &pos))
			}
		case types.BlockAMZMorator:
			result.Moratorium.FirstRepayStatus = readInt8(data, &pos)
			result.Moratorium.FirstRepay = readDateRec(data, &pos)
		case types.BlockAMZTarget:
			result.Target.TargetStatus = readInt8(data, &pos)
			result.Target.TargetValue = readReal48(data, &pos)
		case types.BlockAMZSkip:
			result.SkipMonths.SkipStatus = readInt8(data, &pos)
			result.SkipMonths.SkipStr = readShortStr(data, &pos, 15)
		}
	}

	return result, nil
}

// LoadPresentValueFile reads and parses a legacy present value screen file.
//
// Ported from legacy/source/FileIOUnit.pas: TFileIO.LoadPresentValueData
func LoadPresentValueFile(path string) (*PresentValueFile, error) {
	hdr, data, err := ReadFile(path)
	if err != nil {
		return nil, err
	}
	if hdr.FileType != FileTypePresentValue {
		return nil, fmt.Errorf("not a present value file: grid ID = %d", hdr.Grids[0].GridID)
	}

	result := &PresentValueFile{Header: *hdr, Fancy: hdr.FancyByte > 0}
	pos := 0

	for i := 0; i < 8; i++ {
		grid := hdr.Grids[i]
		switch grid.GridID {
		case types.BlockPVLLumpSum:
			for j := 0; j < int(grid.LineCount); j++ {
				result.LumpSums = append(result.LumpSums, readLumpSum(data, &pos))
			}
		case types.BlockPVLPeriodic:
			for j := 0; j < int(grid.LineCount); j++ {
				result.Periodics = append(result.Periodics, readPeriodic(data, &pos))
			}
		case types.BlockPVLPresVal:
			for j := 0; j < int(grid.LineCount); j++ {
				if result.Fancy {
					// Rate line format (fancy mode)
					skip(&pos, int(grid.LineCount)*13) // skip for now
					break
				}
				result.PresVals = append(result.PresVals, readPresValLine(data, &pos))
			}
		case types.BlockPVLExtra:
			if result.Fancy && grid.LineCount > 0 {
				// XPresVal — skip for now
				skip(&pos, 12)
			}
		}
	}

	return result, nil
}

// --- Internal record readers ---

func readAMZLoan(data []byte, pos *int) amortization.Loan {
	var l amortization.Loan
	l.AmountStatus = readInt8(data, pos)
	l.Amount = readReal48(data, pos)
	l.LoanDateStatus = readInt8(data, pos)
	l.LoanDate = readDateRec(data, pos)
	l.LoanRateStatus = readInt8(data, pos)
	l.LoanRate = readReal48(data, pos)
	l.FirstStatus = readInt8(data, pos)
	l.FirstDate = readDateRec(data, pos)
	l.NStatus = readInt8(data, pos)
	l.NPeriods = int(readInt16LE(data, pos))
	l.LastStatus = readInt8(data, pos)
	l.LastDate = readDateRec(data, pos)
	l.PerYrStatus = readInt8(data, pos)
	l.PerYr = int(readInt16LE(data, pos))
	l.PayAmtStatus = readInt8(data, pos)
	l.PayAmt = readReal48(data, pos)
	l.PointsStatus = readInt8(data, pos)
	l.Points = readReal48(data, pos)
	l.APRStatus = readInt8(data, pos)
	l.APR = readReal48(data, pos)
	l.LastOK = readBool(data, pos)
	return l
}

func readBalloonPayment(data []byte, pos *int) amortization.BalloonPayment {
	var b amortization.BalloonPayment
	b.DateStatus = readInt8(data, pos)
	b.Date = readDateRec(data, pos)
	b.AmountStatus = readInt8(data, pos)
	b.Amount = readReal48(data, pos)
	return b
}

func readPrepayment(data []byte, pos *int) amortization.Prepayment {
	var p amortization.Prepayment
	p.StartDateStatus = readInt8(data, pos)
	p.StartDate = readDateRec(data, pos)
	p.NNStatus = readInt8(data, pos)
	p.NN = int(readInt16LE(data, pos))
	p.StopDateStatus = readInt8(data, pos)
	p.StopDate = readDateRec(data, pos)
	p.PerYrStatus = readInt8(data, pos)
	p.PerYr = int(readInt16LE(data, pos))
	p.PaymentStatus = readInt8(data, pos)
	p.Payment = readReal48(data, pos)
	p.NextDate = readDateRec(data, pos)
	return p
}

func readAdjustment(data []byte, pos *int) amortization.RateAdjustment {
	var a amortization.RateAdjustment
	a.DateStatus = readInt8(data, pos)
	a.Date = readDateRec(data, pos)
	a.LoanRateStatus = readInt8(data, pos)
	a.LoanRate = readReal48(data, pos)
	a.AmountStatus = readInt8(data, pos)
	a.Amount = readReal48(data, pos)
	a.AmtOK = readBool(data, pos)
	return a
}

func readLumpSum(data []byte, pos *int) presentvalue.LumpSumPayment {
	var ls presentvalue.LumpSumPayment
	ls.DateStatus = readInt8(data, pos)
	ls.Date = readDateRec(data, pos)
	ls.AmtStatus = readInt8(data, pos)
	ls.Amt = readReal48(data, pos)
	ls.ValStatus = readInt8(data, pos)
	ls.Val = readReal48(data, pos)
	ls.Status = int(readInt16LE(data, pos))
	skip(pos, 1) // act0
	return ls
}

func readPeriodic(data []byte, pos *int) presentvalue.PeriodicPayment {
	var pp presentvalue.PeriodicPayment
	pp.FromDateStatus = readInt8(data, pos)
	pp.FromDate = readDateRec(data, pos)
	pp.ToDateStatus = readInt8(data, pos)
	pp.ToDate = readDateRec(data, pos)
	pp.PerYrStatus = readInt8(data, pos)
	pp.PerYr = int(readInt16LE(data, pos))
	pp.AmtStatus = readInt8(data, pos)
	pp.Amt = readReal48(data, pos)
	pp.COLAStatus = readInt8(data, pos)
	pp.COLA = readReal48(data, pos)
	pp.ValStatus = readInt8(data, pos)
	pp.Val = readReal48(data, pos)
	pp.Status = int(readInt16LE(data, pos))
	pp.NInstallments = int(readInt16LE(data, pos))
	skip(pos, 1) // actn
	return pp
}

func readPresValLine(data []byte, pos *int) presentvalue.PresValLine {
	var pv presentvalue.PresValLine
	pv.AsOfStatus = readInt8(data, pos)
	pv.AsOf = readDateRec(data, pos)
	pv.R.Status = readInt8(data, pos)
	pv.R.Rate = readReal48(data, pos)
	pv.R.PerYr = readByte(data, pos)
	pv.SumValueStatus = readInt8(data, pos)
	pv.SumValue = readReal48(data, pos)
	pv.Status = int(readInt16LE(data, pos))
	pv.DurationStatus = readInt8(data, pos)
	pv.Duration = readReal48(data, pos)
	return pv
}

// --- DateRec → legacy conversion helper for writing ---

// dateRecToBytes converts a DateRec to the 3-byte Pascal daterec format.
func dateRecToBytes(d types.DateRec) [3]byte {
	if d.IsUnknown() {
		return [3]byte{0, 0xA8, 0} // unkdate: m = -88 = 0xA8 as unsigned
	}
	return [3]byte{
		byte(d.Time.Day()),
		byte(d.Time.Month()),
		byte(d.Time.Year() - 1900),
	}
}

// newDateRecFromBytes is the inverse of dateRecToBytes (for testing).
func newDateRecFromBytes(b [3]byte) types.DateRec {
	d := int(b[0])
	m := int(int8(b[1]))
	y := int(b[2])
	if m < 1 || m > 12 {
		return types.UnknownDate()
	}
	return types.NewDateRec(y+1900, time.Month(m), d)
}
