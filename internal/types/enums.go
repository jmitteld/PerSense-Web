package types

// BasisType represents the day-count convention used for interest calculations.
// Ported from legacy/source/PETYPES.PAS: basistype=(x365,x360,x365_360)
type BasisType int

const (
	Basis365    BasisType = iota // x365: actual/365 day-count
	Basis360                     // x360: 30/360 day-count
	Basis365360                  // x365_360: actual/360 (hybrid)
)

// String returns a human-readable representation of the BasisType.
func (b BasisType) String() string {
	switch b {
	case Basis365:
		return "365"
	case Basis360:
		return "360"
	case Basis365360:
		return "365/360"
	default:
		return "unknown"
	}
}

// MethodType represents the interest calculation method.
// Ported from legacy/source/PETYPES.PAS:
//
//	methodtype=(mCONTINUOUS,mPERDC_CONT,mDAILY,mPERIODIC,mPMT_TO_PMT,mSKIP_PMT,mUS_RULE,mP_BEFORE_I)
type MethodType int

const (
	MethodContinuous  MethodType = iota // mCONTINUOUS: continuous compounding
	MethodPerdcCont                     // mPERDC_CONT: periodic/continuous hybrid
	MethodDaily                         // mDAILY: daily compounding
	MethodPeriodic                      // mPERIODIC: standard periodic compounding
	MethodPmtToPmt                      // mPMT_TO_PMT: payment-to-payment
	MethodSkipPmt                       // mSKIP_PMT: skip payment
	MethodUSRule                        // mUS_RULE: US Rule calculation
	MethodPBeforeI                      // mP_BEFORE_I: principal before interest
)

// MethodNames maps each MethodType to its display string.
// Ported from legacy/source/PETYPES.PAS: methodstr array
var MethodNames = map[MethodType]string{
	MethodContinuous: "CONTINUOUS",
	MethodPerdcCont:  "PERDC/CONT",
	MethodDaily:      "DAILY",
	MethodPeriodic:   "PERIODIC",
	MethodPmtToPmt:   "PMT-TO-PMT",
	MethodSkipPmt:    "SKIP-PMT",
	MethodUSRule:     "US RULE",
	MethodPBeforeI:   "P BEFORE I",
}

// String returns the display name of the MethodType.
func (m MethodType) String() string {
	if s, ok := MethodNames[m]; ok {
		return s
	}
	return "unknown"
}

// Destination represents where output is directed.
// Ported from legacy/source/PETYPES.PAS: destinations=(none,tolotus,toprinter,totext)
type Destination int

const (
	DestNone    Destination = iota // none
	DestLotus                      // tolotus
	DestPrinter                    // toprinter
	DestText                       // totext
)

// BalloonStatus indicates the state of a balloon payment field.
// Ported from legacy/source/PETYPES.PAS:
//
//	balloonstatus: (balloon_known, balloon_unk, balloon_blank)
type BalloonStatus int

const (
	BalloonKnown BalloonStatus = iota
	BalloonUnk
	BalloonBlank
)

// Upto represents a date comparison mode.
// Ported from legacy/source/PETYPES.PAS: upto=(before, on_or_before, after, on_or_after)
type Upto int

const (
	Before      Upto = iota
	OnOrBefore
	After
	OnOrAfter
)
