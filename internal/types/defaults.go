package types

// CompDefaults holds computational default settings.
//
// Ported from legacy/source/PETYPES.PAS: compdefaults record
type CompDefaults struct {
	COLAMonth  byte      // ANN (99) or CNT (98)
	CenturyDiv byte      // century boundary for 2-digit year input (default 50)
	PerYr      byte      // default compounding frequency (default 12)
	USARule    bool      // use US Rule for interest calculations
	Basis      BasisType // day-count convention
	Prepaid    bool      // prepaid interest
	InAdvance  bool      // payments in advance
	PlusRegular bool     // plus regular payment with prepaid
	Exact      bool      // exact interest calculations
	R78        bool      // Rule of 78 amortization
}

// DefaultCompDefaults returns the default computational settings.
// Ported from legacy/source/PEDATA.pas: df.c initialization
func DefaultCompDefaults() CompDefaults {
	return CompDefaults{
		COLAMonth:   COLAAnnual,
		CenturyDiv:  50,
		PerYr:       12,
		USARule:     false,
		Basis:       Basis360,
		Prepaid:     true,
		InAdvance:   false,
		PlusRegular: false,
		Exact:       false,
		R78:         false,
	}
}

// AppDefaults holds the top-level application defaults record.
// This is a simplified version that omits hardware, printer, and directory
// defaults which are not relevant to the web port.
//
// Ported from legacy/source/PETYPES.PAS: defrec record
type AppDefaults struct {
	Version  uint16
	Comp     CompDefaults
	XSimple  bool       // extended simple interest mode
	RMethod  MethodType // default interest calculation method
	Commas   bool       // display commas in numbers
}

// DefaultAppDefaults returns the standard application defaults.
// Ported from legacy/source/PEDATA.pas: df initialization
func DefaultAppDefaults() AppDefaults {
	return AppDefaults{
		Version: 0,
		Comp:    DefaultCompDefaults(),
		XSimple: false,
		RMethod: MethodPerdcCont,
		Commas:  true,
	}
}

// ValidPerYrValues returns the set of valid compounding frequencies.
// Ported from legacy/source/PEDATA.pas: peryrset := [1, 2, 3, 4, 6, 12, 24, 26, 52]
func ValidPerYrValues() []int {
	return []int{1, 2, 3, 4, 6, 12, 24, 26, 52}
}

// IsValidPerYr checks if a compounding frequency is valid.
func IsValidPerYr(peryr int) bool {
	for _, v := range ValidPerYrValues() {
		if v == peryr {
			return true
		}
	}
	return false
}
