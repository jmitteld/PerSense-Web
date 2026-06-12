package fileio

import (
	"math"
	"os"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// These tests load the THREE REAL legacy Per%Sense worksheet files shipped in
// the repo (copied into testdata/ from legacy/src/win_source/DOS.{MTG,AMZ,PVL})
// and pin the parsed contents. This is the strongest fidelity check for the file
// loaders: it proves the port reads genuine client-format data correctly — not
// just synthetic byte blobs — which is what every saved customer worksheet
// depends on. The legacy floats are 6-byte Turbo Pascal Real48 values, so the
// reconstructed float64s carry ~1e-10 representation noise; comparisons use
// small tolerances accordingly.

func approx(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %.6f, want %.6f (±%g)", name, got, want, tol)
	}
}

func ymd(t *testing.T, name string, d types.DateRec, y int, m int, day int) {
	t.Helper()
	if d.Time.Year() != y || int(d.Time.Month()) != m || d.Time.Day() != day {
		t.Errorf("%s = %04d-%02d-%02d, want %04d-%02d-%02d",
			name, d.Time.Year(), d.Time.Month(), d.Time.Day(), y, m, day)
	}
}

func TestLoadLegacyMortgageFixture(t *testing.T) {
	f, err := LoadMortgageFile("testdata/DOS.MTG")
	if err != nil {
		t.Fatalf("LoadMortgageFile: %v", err)
	}
	if f.Header.FileType != FileTypeMortgage {
		t.Errorf("file type = %d, want mortgage", f.Header.FileType)
	}
	if len(f.Mortgages) != 2 {
		t.Fatalf("got %d mortgage lines, want 2", len(f.Mortgages))
	}
	m := f.Mortgages[0]
	if m.PriceStatus != types.InOutInput || m.YearsStatus != types.InOutInput {
		t.Errorf("price/years should be user inputs, got %d/%d", m.PriceStatus, m.YearsStatus)
	}
	approx(t, "price", m.Price, 10000, 1e-3)
	approx(t, "pct", m.Pct, 0.03, 1e-6)
	approx(t, "points", m.Points, 0.02, 1e-6)
	approx(t, "cash", m.Cash, 494, 1e-3)
	approx(t, "financed", m.Financed, 9700, 1e-3)
	if m.Years != 4 {
		t.Errorf("years = %d, want 4", m.Years)
	}
	approx(t, "rate", m.Rate, 0.049896, 1e-5)
	approx(t, "tax", m.Tax, 6, 1e-3)
	approx(t, "monthly", m.Monthly, 229.384148, 1e-4)
	// Cash, Financed and Monthly are COMPUTED outputs in this saved sheet.
	if m.CashStatus != types.InOutOutput || m.MonthlyStatus != types.InOutOutput {
		t.Errorf("cash/monthly should be outputs, got %d/%d", m.CashStatus, m.MonthlyStatus)
	}
}

func TestLoadLegacyAmortizationFixture(t *testing.T) {
	f, err := LoadAmortizationFile("testdata/DOS.AMZ")
	if err != nil {
		t.Fatalf("LoadAmortizationFile: %v", err)
	}
	if f.Header.FileType != FileTypeAmortization {
		t.Errorf("file type = %d, want amortization", f.Header.FileType)
	}
	l := f.Loan
	approx(t, "amount", l.Amount, 10000, 1e-3)
	approx(t, "loanRate", l.LoanRate, 0.09, 1e-6)
	if l.NPeriods != 120 {
		t.Errorf("nPeriods = %d, want 120", l.NPeriods)
	}
	if l.PerYr != 2 {
		t.Errorf("perYr = %d, want 2", l.PerYr)
	}
	approx(t, "payAmt", l.PayAmt, 373.291196, 1e-4)
	approx(t, "points", l.Points, 0.02, 1e-6)
	approx(t, "apr", l.APR, 0.065169, 1e-5)
	ymd(t, "loanDate", l.LoanDate, 1975, 12, 16)
	ymd(t, "firstDate", l.FirstDate, 1976, 12, 1)
	ymd(t, "lastDate", l.LastDate, 2036, 6, 1)
	if !l.LastOK {
		t.Error("LastOK should be true")
	}
	if l.AmountStatus != types.InOutInput || l.PayAmtStatus != types.InOutOutput {
		t.Errorf("amount input / payment output expected, got %d/%d", l.AmountStatus, l.PayAmtStatus)
	}
}

func TestLoadLegacyPresentValueFixture(t *testing.T) {
	f, err := LoadPresentValueFile("testdata/DOS.PVL")
	if err != nil {
		t.Fatalf("LoadPresentValueFile: %v", err)
	}
	if f.Header.FileType != FileTypePresentValue {
		t.Errorf("file type = %d, want present value", f.Header.FileType)
	}
	if f.Fancy {
		t.Error("fixture is not a fancy (variable-rate) sheet")
	}
	if len(f.LumpSums) != 1 {
		t.Fatalf("got %d lump sums, want 1", len(f.LumpSums))
	}
	ls := f.LumpSums[0]
	approx(t, "lump amt", ls.Amt, 5000, 1e-3)
	approx(t, "lump val", ls.Val, 4628.996168, 1e-4)
	ymd(t, "lump date", ls.Date, 1981, 12, 3)
	if len(f.Periodics) != 2 {
		t.Errorf("got %d periodics, want 2", len(f.Periodics))
	}
	if len(f.PresVals) != 2 {
		t.Fatalf("got %d present-value lines, want 2", len(f.PresVals))
	}
	approx(t, "presval0 sum", f.PresVals[0].SumValue, 16167.257126, 1e-3)
	ymd(t, "presval0 asof", f.PresVals[0].AsOf, 1980, 12, 16)
	approx(t, "presval1 rate", f.PresVals[1].R.Rate, 0.068803, 1e-5)
	approx(t, "presval1 sum", f.PresVals[1].SumValue, 31462.492546, 1e-3)
	ymd(t, "presval1 asof", f.PresVals[1].AsOf, 1990, 1, 1)
}

// --- corrupt / mismatched input handling ----------------------------------

func TestLoadWrongFileTypeRejected(t *testing.T) {
	b, err := os.ReadFile("testdata/DOS.MTG")
	if err != nil {
		t.Fatal(err)
	}
	// A mortgage file fed to the amortization / PV loaders must be refused,
	// not silently mis-parsed.
	if _, err := LoadAmortizationBytes(b); err == nil {
		t.Error("LoadAmortizationBytes accepted a mortgage file")
	}
	if _, err := LoadPresentValueBytes(b); err == nil {
		t.Error("LoadPresentValueBytes accepted a mortgage file")
	}
	// The matching loader must accept it.
	if _, err := LoadMortgageBytes(b); err != nil {
		t.Errorf("LoadMortgageBytes rejected a valid mortgage file: %v", err)
	}
}

func TestLoadEmptyAndTruncatedInput(t *testing.T) {
	// Empty input has no header — must error, not panic.
	if _, err := LoadMortgageBytes(nil); err == nil {
		t.Error("empty input should error")
	}
	if _, err := LoadMortgageBytes([]byte{}); err == nil {
		t.Error("zero-length input should error")
	}
	// A header truncated mid-way must error rather than panic.
	b, _ := os.ReadFile("testdata/DOS.MTG")
	if len(b) > 8 {
		if _, err := LoadMortgageBytes(b[:8]); err == nil {
			t.Error("truncated header should error")
		}
	}
	// A valid header followed by a truncated body must not panic — the record
	// readers clamp past end-of-buffer. Loading must return without crashing.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("loading a body-truncated file panicked: %v", r)
			}
		}()
		if len(b) > 60 {
			_, _ = LoadMortgageBytes(b[:60]) // header intact, body cut short
		}
	}()
}
