package types

// ScreenData holds the in-memory data arrays for all financial screens.
// In the original Pascal, these were global variables (a, b, c, cc, d, e, g, h, etc.)
// accessed via blockdata[]. Here they are collected into a single struct.
//
// Ported from legacy/source/PEDATA.pas: global variable declarations
type ScreenData struct {
	// Present Value screen
	LumpSums  [MaxLines]*LumpSum  // a[]: single payment entries
	Periodics [MaxLines]*Periodic // b[]: periodic payment entries
	PresVals  [PresValLines]*PresVal // c[]: present value summary lines
	RateLines [MaxLines]*RateLine // cc[]: rate entries
	XPresVal  *XPresVal           // d: extended present value

	// Mortgage screen
	Mortgages [MaxLines]*MortgageLine // e[]: mortgage comparison rows

	// Chronological screen
	CHRLines [MaxLines]*CHRLine // g[]: chronological entries

	// Amortization screen
	AMZ       *AMZLoan                   // h: loan parameters
	AsOf      *BalloonRec                // w: as-of balance query
	Prepays   [MaxPrepay]*PrepaymentRec  // pre[]: prepayment series
	Balloons  [MaxBalloon]*BalloonRec    // balloon[]: balloon payments
	Adjs      [MaxAdj]*AdjRec           // adj[]: rate/payment adjustments
	Moratoriu *MoratoriumRec             // mor: deferment record
	Target    *TargetRec                 // targ: targeted principal reduction
	Skip      *SkipRec                   // skp: skip month record

	// Per-block metadata
	NLines    [NBlocks + 1]byte // nlines[]: allocated lines per block
	ScrollPos [NBlocks + 1]byte // scrollpos[]: current scroll position per block
}

// NewScreenData creates a new ScreenData with all pointers nil and
// metadata initialized to zero.
func NewScreenData() *ScreenData {
	return &ScreenData{}
}

// BlockInfo holds display metadata for a single data block.
// Ported from various arrays in legacy/source/PEDATA.pas
type BlockInfo struct {
	FirstCol    byte // fcol: first column in this block
	LastCol     byte // lcol: last column in this block
	Scrollable  bool // scrolls[block]
	LineCount   byte // screenlines/lineCount: visible lines
	DataSize    byte // datasize[block]: byte size of one data record
}

// Placeholder stores a saved cursor position.
// Ported from legacy/source/PETYPES.PAS: placeholder record
type Placeholder struct {
	XGo   byte
	YGo   byte
	Col   byte
	Block byte
}
