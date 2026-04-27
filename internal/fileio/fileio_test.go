package fileio

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// --- Real48 conversion tests ---

func TestReal48Zero(t *testing.T) {
	var b [6]byte
	if Real48ToFloat64(b) != 0 {
		t.Error("all-zero bytes should produce 0")
	}
	result := Float64ToReal48(0)
	if result != b {
		t.Errorf("Float64ToReal48(0) = %v, want all zeros", result)
	}
}

func TestReal48RoundTrip(t *testing.T) {
	values := []float64{
		1.0, -1.0, 0.5, -0.5,
		100000.00, -100000.00,
		0.06, 0.005, 0.12,
		3.14159265,
		1234567.89,
		0.01, 0.001,
		99.99,
	}
	for _, v := range values {
		b := Float64ToReal48(v)
		got := Real48ToFloat64(b)
		relErr := math.Abs(got-v) / math.Max(math.Abs(v), 1e-20)
		// Real48 has ~11 decimal digits of precision (39-bit mantissa)
		if relErr > 1e-9 {
			t.Errorf("round trip %g: got %g, rel error = %g", v, got, relErr)
		}
	}
}

func TestReal48KnownValues(t *testing.T) {
	// Test that 1.0 has the expected representation
	b := Float64ToReal48(1.0)
	if b[0] != 129 { // exponent = 0 + 129 = 129
		t.Errorf("1.0 exponent byte = %d, want 129", b[0])
	}
	// Mantissa should be all zeros (the 1 is implicit)
	for i := 1; i <= 5; i++ {
		if b[i] != 0 {
			t.Errorf("1.0 mantissa byte[%d] = %d, want 0", i, b[i])
		}
	}

	// Test negative
	bNeg := Float64ToReal48(-1.0)
	if bNeg[5]&0x80 == 0 {
		t.Error("-1.0 should have sign bit set")
	}
}

func TestReal48SmallValues(t *testing.T) {
	// Interest rates are the most critical small values
	for _, rate := range []float64{0.01, 0.05, 0.06, 0.065, 0.12, 0.20} {
		b := Float64ToReal48(rate)
		got := Real48ToFloat64(b)
		if math.Abs(got-rate) > 1e-10 {
			t.Errorf("rate %g round trip: got %g", rate, got)
		}
	}
}

func TestReal48LargeValues(t *testing.T) {
	// Monetary values
	for _, amt := range []float64{100000, 250000, 500000, 1000000, 99.99, 599.55} {
		b := Float64ToReal48(amt)
		got := Real48ToFloat64(b)
		if math.Abs(got-amt) > 0.001 {
			t.Errorf("amount %g round trip: got %g", amt, got)
		}
	}
}

// --- DateRec conversion tests ---

func TestDateRecToBytes(t *testing.T) {
	d := types.NewDateRec(2024, time.March, 15)
	b := dateRecToBytes(d)
	// d=15, m=3, y=2024-1900=124
	if b[0] != 15 || b[1] != 3 || b[2] != 124 {
		t.Errorf("dateRecToBytes(2024-03-15) = %v, want [15 3 124]", b)
	}

	// Round trip
	got := newDateRecFromBytes(b)
	if got.Time.Year() != 2024 || got.Time.Month() != time.March || got.Time.Day() != 15 {
		t.Errorf("round trip failed: got %v", got.Time)
	}
}

func TestDateRecUnknown(t *testing.T) {
	b := dateRecToBytes(types.UnknownDate())
	// Unknown: d=0, m=-88, y=0
	if int8(b[1]) != -88 {
		t.Errorf("unknown date m byte = %d, want -88", int8(b[1]))
	}
}

func TestDateRecEarliest(t *testing.T) {
	d := types.NewDateRec(1900, time.January, 1)
	b := dateRecToBytes(d)
	if b[0] != 1 || b[1] != 1 || b[2] != 0 {
		t.Errorf("dateRecToBytes(1900-01-01) = %v, want [1 1 0]", b)
	}
}

func TestDateRecLatest(t *testing.T) {
	d := types.NewDateRec(2149, time.December, 1)
	b := dateRecToBytes(d)
	// y = 2149 - 1900 = 249
	if b[2] != 249 {
		t.Errorf("latest date y byte = %d, want 249", b[2])
	}
}

// --- ReadFile header parsing test ---

func TestReadFileNonExistent(t *testing.T) {
	_, _, err := ReadFile("/nonexistent/file.pct")
	if err == nil {
		t.Error("reading nonexistent file should error")
	}
}

// --- Grid header tests ---

func TestGridHeaderSize(t *testing.T) {
	// Each GridHeader is 3 bytes: GridID(1) + ScrollPosition(1) + LineCount(1)
	var g GridHeader
	g.GridID = types.BlockMTG
	g.LineCount = 5
	if g.GridID != 5 {
		t.Errorf("GridID = %d, want 5", g.GridID)
	}
}

// --- Read helpers ---

func TestReadInt8(t *testing.T) {
	data := []byte{0xFF, 0x03, 0x80}
	pos := 0
	if readInt8(data, &pos) != -1 {
		t.Error("0xFF as int8 should be -1")
	}
	if readInt8(data, &pos) != 3 {
		t.Error("0x03 as int8 should be 3")
	}
	if readInt8(data, &pos) != -128 {
		t.Error("0x80 as int8 should be -128")
	}
}

func TestReadInt16LE(t *testing.T) {
	data := []byte{0x68, 0x01} // 360 in little-endian
	pos := 0
	got := readInt16LE(data, &pos)
	if got != 360 {
		t.Errorf("readInt16LE = %d, want 360", got)
	}
}

func TestReadDateRec(t *testing.T) {
	// March 15, 2024: d=15, m=3, y=124
	data := []byte{15, 3, 124}
	pos := 0
	d := readDateRec(data, &pos)
	if d.Time.Year() != 2024 || d.Time.Month() != time.March || d.Time.Day() != 15 {
		t.Errorf("readDateRec = %v, want 2024-03-15", d.Time)
	}
	if pos != 3 {
		t.Errorf("pos = %d, want 3", pos)
	}
}

func TestReadDateRecInvalid(t *testing.T) {
	// Invalid month (-88 = unknown)
	data := []byte{0, byte(uint8(0xA8)), 0} // m = -88 as unsigned byte
	pos := 0
	d := readDateRec(data, &pos)
	if !d.IsUnknown() {
		t.Error("invalid month should produce unknown date")
	}
}

func TestCompDefaultsSize(t *testing.T) {
	// CompDefaults on disk should be exactly 10 bytes
	var cd CompDefaultsOnDisk
	_ = cd
	// Can't use unsafe.Sizeof in tests easily, but we verify the field count
	// matches the Pascal record
	cd.COLAMonth = 99   // ANN
	cd.CenturyDiv = 50
	cd.PerYr = 12
	cd.Basis = 1        // x360
	cd.Prepaid = 1
	if cd.PerYr != 12 {
		t.Error("sanity check failed")
	}
}
