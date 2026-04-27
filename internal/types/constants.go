// Package types defines core type definitions and constants ported from the
// legacy Delphi/Pascal application. All financial record types, enumerations,
// column identifiers, and status codes are defined here.
//
// Ported from legacy/source/PETYPES.PAS and legacy/source/PEDATA.pas
package types

// MaxLines is the maximum number of data rows per scrollable block.
// Ported from legacy/source/PETYPES.PAS: maxlines=127
const MaxLines = 127

// NBlocks is the total number of data blocks in the application.
// Ported from legacy/source/PETYPES.PAS: nblocks=15
const NBlocks = 15

// NCols is the total number of column identifiers.
// Ported from legacy/source/PETYPES.PAS: ncols=79
const NCols = 79

// PresValLines is the number of lines in the present value block.
// Ported from legacy/source/PETYPES.PAS: presvallines=3
const PresValLines = 3

// MaxPrepay is the maximum number of prepayment series.
// Ported from legacy/source/PETYPES.PAS: maxprepay=2
const MaxPrepay = 2

// MaxAdj is the maximum number of rate adjustment entries.
// Ported from legacy/source/PETYPES.PAS: maxadj=maxlines
const MaxAdj = MaxLines

// MaxBalloon is the maximum number of balloon payment entries.
// Ported from legacy/source/PETYPES.PAS: maxballoon=maxlines
const MaxBalloon = MaxLines

// --- Cell/field status codes ---
// These indicate the origin of a data value in a cell.
// Ported from legacy/source/PETYPES.PAS

const (
	StatusEmpty      int8 = 0 // Cell is empty / no data
	StatusCalculated int8 = 1 // Value was calculated (output)
	StatusFromCalc   int8 = 1 // Synonym for StatusCalculated
	StatusFromRate   int8 = 2 // Value derived from rate
	StatusFromAPR    int8 = 3 // Value derived from APR
	StatusFromYield  int8 = 4 // Value derived from yield
)

// --- InOut status codes ---
// These indicate the "hardness" or provenance of data in a cell.
// Ported from legacy/source/PETYPES.PAS

const (
	InOutBad     int8 = -1 // Data improperly formatted, unreadable, or out of bounds
	InOutEmpty   int8 = 0  // No data present
	InOutOutput  int8 = 1  // Computed output
	InOutDefault int8 = 2  // Default value
	InOutInput   int8 = 3  // User-entered input
)

// --- Sentinel values ---
// Ported from legacy/source/PETYPES.PAS

const (
	ErrorVal = -8888 // Sentinel for error / unknown values
	Unk      = -8888 // Synonym for ErrorVal
	Blank    = -7777 // Sentinel for blank fields
	UnkByte  = -88   // Unknown byte sentinel for dates
)

// --- Numeric format qualifiers ---
// These identify the display format for a column.
// Ported from legacy/source/PETYPES.PAS

const (
	ShortFmt      byte = 0
	CurrencyFmt   byte = 1
	PercentFmt    byte = 2
	PctFmt             = PercentFmt
	DateFmt       byte = 3
	StringFmt     byte = 4
	ThreeDigitFmt byte = 5
	Str15Fmt      byte = 6
	UnusedFmt     byte = 99
)

// --- COLA month options ---
// Ported from legacy/source/PETYPES.PAS

const (
	COLAContinuous byte = 98 // CNT
	COLAAnnual     byte = 99 // ANN
)

// --- Screen identifiers ---
// Ported from legacy/source/PETYPES.PAS and legacy/source/PEDATA.pas

const (
	ScreenOpen         int8 = 0 // iopen
	ScreenPresentValue int8 = 1 // ipvl
	ScreenChronolog    int8 = 2 // ichr / ivar
	ScreenMortgage     int8 = 3 // imtg
	ScreenAmortization int8 = 4 // iamz
	NumScreens              = 4 // nscr
)

// --- Compounding frequency special values ---
// Ported from legacy/source/PETYPES.PAS

const (
	CompoundingDaily    byte = 64  // daily compounding mode
	CompoundingCanadian byte = 128 // Canadian-style compounding
)

// --- Output mode flags ---
// Ported from legacy/source/PETYPES.PAS

const (
	OutputNoTab       = 0
	OutputWithTab     = 1
	OutputMakeTable   = 2
	OutputWithLife    = 4
	OutputLotusExport = 8
	OutputAPRReport   = 16
)

// --- Contingency types (actuarial) ---
// Ported from legacy/source/PETYPES.PAS

const (
	NotContingent byte = 0
	Living        byte = 1
	Dead          byte = 2
	Only1Living   byte = 3
	Only2Living   byte = 4
	EitherLiving  byte = 5
	BothLiving    byte = 6
)

// --- Screen status bit flags ---
// Ported from legacy/source/PETYPES.PAS

const (
	FlagNotSaved    byte = 1
	FlagNeedsCalc   byte = 2
	FlagDidSometing byte = 4
	FlagCellChanged byte = 8
	FlagNeedsAll    byte = 15
	FlagNeedsNothin byte = 0
)

// --- Mathematical constants ---
// Ported from legacy/source/PETYPES.PAS

const (
	Small    = 1e-4
	Teeny    = 1e-10
	Tiny     = 1e-5 // from Globals.pas
	Half     = 0.5
	Twelfth  = 1.0 / 12.0
	Hundredt = 0.01
	FourYear = 1461  // days in 4 years (incl. leap)
	TwentyYr = 7305  // days in 20 years
	Inv365   = 1.0 / 365.0
	MaxReal  = 1.7e38
	Kicker   = 365.0 / 360.0
	Infinity = 1.6986727435e38
)

// --- Line determination status ---
// Used to track what a data line needs for computation.
// Ported from legacy/source/PETYPES.PAS

const (
	LineBlank           byte = 0   // blank_line / empty
	LineNeedsDeposit    byte = 1   // needs_deposit
	LineNeedsTotal      byte = 2   // needs_total
	LineNeedsBoth       byte = 3   // needs_both
	LineMissing4        byte = 250 // missing_4
	LineMissing3        byte = 251 // missing_3
	LineMissing2        byte = 252 // missing_2 / under_determined
	LineContainsUnknown byte = 253 // contains_unknown
	LineFullySpecified  byte = 254 // fully_specified
	LineOverDetermined  byte = 255 // over_determined
)
