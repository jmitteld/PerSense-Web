package types

import (
	"time"

	"github.com/shopspring/decimal"
)

// DateRec represents a date in the legacy format.
// The original Pascal daterec stores year as a byte (0-249 representing 1900-2149),
// but in Go we use time.Time for all date operations.
//
// Ported from legacy/source/Globals.pas:
//
//	daterec=record d,m:shortint; y:byte; end;
type DateRec struct {
	Time time.Time
}

// NewDateRec creates a DateRec from year, month, day.
func NewDateRec(year int, month time.Month, day int) DateRec {
	return DateRec{Time: time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// IsUnknown returns true if this date represents an unknown/sentinel value.
func (d DateRec) IsUnknown() bool {
	return d.Time.IsZero()
}

// UnknownDate returns a sentinel DateRec representing an unknown date.
// Ported from legacy/source/PETYPES.PAS: unkdate:daterec=(d:0;m:unkbyte;y:0)
func UnknownDate() DateRec {
	return DateRec{}
}

// EarliestDate returns the earliest admissible date (Jan 1, 1900).
// Ported from legacy/source/Globals.pas: earliest:daterec=(d:1;m:1;y:0)
func EarliestDate() DateRec {
	return NewDateRec(1900, time.January, 1)
}

// LatestDate returns the latest admissible date (Dec 1, 2149).
// Ported from legacy/source/Globals.pas: latest:daterec=(d:1;m:12;y:249)
func LatestDate() DateRec {
	return NewDateRec(2149, time.December, 1)
}

// InOut represents the provenance/status of a data cell value.
// Values range from InOutBad (-1) to InOutInput (3).
// Ported from legacy/source/PETYPES.PAS: inout=shortint
type InOut = int8

// RateRec stores an interest rate along with its associated compounding frequency.
// The peryr field tracks the compounding frequency that was in effect when the
// rate was entered, so the rate can be adjusted if peryr later changes.
//
// Ported from legacy/source/PETYPES.PAS:
//
//	raterec = record
//	    status: inout;
//	    rate: real;
//	    peryr: byte;
//	end;
type RateRec struct {
	Status InOut
	Rate   decimal.Decimal
	PerYr  byte // compounding frequency when rate was entered
}

// MortgageLine represents one row of the mortgage comparison screen.
// Each field has an accompanying status (InOut) indicating its provenance.
//
// Ported from legacy/source/PETYPES.PAS: mortgageline record
type MortgageLine struct {
	PriceStatus    InOut
	Price          decimal.Decimal
	PointsStatus   InOut
	Points         decimal.Decimal
	PctStatus      InOut
	Pct            decimal.Decimal // downpayment percentage
	CashStatus     InOut
	Cash           decimal.Decimal // cash required at settlement
	FinancedStatus InOut
	Financed       decimal.Decimal // amount financed
	YearsStatus    InOut
	Years          int // life of mortgage in years (SHORT_FMT)
	RateStatus     InOut
	Rate           decimal.Decimal
	TaxStatus      InOut
	Tax            decimal.Decimal // monthly tax + insurance
	MonthlyStatus  InOut
	Monthly        decimal.Decimal // total monthly payment
	WhenStatus     InOut
	When           int // years to balloon (SHORT_FMT)
	HowMuchStatus  InOut
	HowMuch        decimal.Decimal // balloon amount
	BalloonStat    BalloonStatus
}

// LumpSum represents a single one-time payment on the present value screen.
//
// Ported from legacy/source/PETYPES.PAS: lumpsum record
type LumpSum struct {
	DateStatus InOut
	Date       DateRec
	Amt0Status InOut
	Amt0       decimal.Decimal // payment amount
	Val0Status InOut
	Val0       decimal.Decimal // present value
	Status     int             // composite status code (see PETYPES.PAS comments)
	Act0       byte            // actuarial contingency flag
}

// Periodic represents a series of periodic payments on the present value screen.
//
// Ported from legacy/source/PETYPES.PAS: periodic record
type Periodic struct {
	FromDateStatus InOut
	FromDate       DateRec
	ToDateStatus   InOut
	ToDate         DateRec
	PerYrStatus    InOut
	PerYr          int // payments per year
	AmtNStatus     InOut
	AmtN           decimal.Decimal // payment amount
	COLAStatus     InOut
	COLA           decimal.Decimal // cost of living adjustment %
	ValNStatus     InOut
	ValN           decimal.Decimal // present value
	Status         int             // composite status code
	NInstallments  int             // computed number of installments
	ActN           byte            // actuarial contingency flag
}

// PresVal represents a present value summary line (bottom of PVL screen).
//
// Ported from legacy/source/PETYPES.PAS: presval record
type PresVal struct {
	AsOfStatus     InOut
	AsOf           DateRec
	R              RateRec
	SumValueStatus InOut
	SumValue       decimal.Decimal // total present value
	Status         int
	DurationStatus InOut
	Duration       decimal.Decimal
}

// RateLine represents a rate entry with date (used in rate arrays).
//
// Ported from legacy/source/PETYPES.PAS: rateline record
type RateLine struct {
	DateStatus InOut
	Date       DateRec
	R          RateRec
	Status     int
}

// XPresVal represents extended present value data (PVLX screen).
//
// Ported from legacy/source/PETYPES.PAS: xpresval record
type XPresVal struct {
	XAsOfStatus  InOut
	XAsOf        DateRec
	SimpleStatus InOut // always inp or defp
	Simple       bool  // true = simple interest, false = compound
	XValueStatus InOut
	XValue       decimal.Decimal
	Status       byte
}

// CHRLine represents one row of the chronological (compound) screen.
//
// Ported from legacy/source/PETYPES.PAS: CHRline record
type CHRLine struct {
	DateStatus       InOut
	Date             DateRec
	PrincipalStatus  InOut
	Principal        decimal.Decimal
	R                RateRec
	DepositSumStatus InOut
	DepositSum       decimal.Decimal
	InterestStatus   InOut // always outp or empty
	Interest         decimal.Decimal
	DepositStatus    InOut
	Deposit          decimal.Decimal
	PerYrStatus      InOut
	PerYr            int
	// Computed fields (obiter dicta):
	PerStatus  int             // needs_deposit, needs_sum, etc.
	PDInterest decimal.Decimal // per-period interest
	Exprt      decimal.Decimal
	F          decimal.Decimal
	NPayments  int
	Status     int
}

// AMZLoan represents the top-level amortization loan parameters.
//
// Ported from legacy/source/PETYPES.PAS: AMZLoan record
type AMZLoan struct {
	AmountStatus   InOut
	Amount         decimal.Decimal // loan amount
	LoanDateStatus InOut
	LoanDate       DateRec // date of closing
	LoanRateStatus InOut
	LoanRate       decimal.Decimal // loan interest rate
	FirstStatus    InOut
	FirstDate      DateRec // date of first regular payment
	NStatus        InOut
	NPeriods       int // number of regular periods
	LastStatus     InOut
	LastDate       DateRec // date of last regular payment
	PerYrStatus    InOut
	PerYr          int // payments per year
	PayAmtStatus   InOut
	PayAmt         decimal.Decimal // regular payment amount
	PointsStatus   InOut
	Points         decimal.Decimal // points charge for APR
	APRStatus      InOut
	APR            decimal.Decimal // computed APR
	// Computed field:
	LastOK bool // whether last date is valid
}

// BalloonRec represents a balloon (lump sum) payment in the amortization schedule.
//
// Ported from legacy/source/PETYPES.PAS: balloonrec record
type BalloonRec struct {
	DateStatus   InOut
	Date         DateRec
	AmountStatus InOut
	Amount       decimal.Decimal
}

// AdjRec represents a rate/payment adjustment entry in the amortization schedule.
//
// Ported from legacy/source/PETYPES.PAS: adjrec record
type AdjRec struct {
	DateStatus     InOut
	Date           DateRec
	LoanRateStatus InOut
	LoanRate       decimal.Decimal
	AmountStatus   InOut
	Amount         decimal.Decimal
	// Computed field:
	AmtOK bool
}

// PrepaymentRec represents a series of extra (or skipped) payments.
//
// Ported from legacy/source/PETYPES.PAS: prepaymentrec record
type PrepaymentRec struct {
	StartDateStatus InOut
	StartDate       DateRec
	NNStatus        InOut
	NN              int // number of extra payments in this series
	StopDateStatus  InOut
	StopDate        DateRec
	PerYrStatus     InOut
	PerYr           int // times per year
	PaymentStatus   InOut
	Payment         decimal.Decimal // amount of extra payment (0 = skipped)
	// Computed field:
	NextDate DateRec
}

// MoratoriumRec represents an interest-only (deferment) period.
//
// Ported from legacy/source/PETYPES.PAS: moratoriumrec record
type MoratoriumRec struct {
	FirstRepayStatus InOut
	FirstRepay       DateRec // date when regular payments resume
}

// TargetRec represents a targeted minimum principal reduction per payment.
//
// Ported from legacy/source/PETYPES.PAS: targetrec record
type TargetRec struct {
	TargetStatus InOut
	Target       decimal.Decimal
}

// SkipRec represents months to skip payments.
//
// Ported from legacy/source/PETYPES.PAS: skiprec record
type SkipRec struct {
	SkipStatus InOut
	SkipMonths string // e.g. "1,6,12" — months to skip
}
