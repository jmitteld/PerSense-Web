package fileio

import (
	"os"
	"testing"
)

const mtgFixture = "../../legacy/src/win_source/DOS.MTG"

func TestReadFrom_TruncatedAndBadIdentifier(t *testing.T) {
	full, err := os.ReadFile(mtgFixture)
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	// Full file parses cleanly.
	if _, _, err := ReadBytes(full); err != nil {
		t.Fatalf("ReadBytes(full) error: %v", err)
	}
	// Progressive truncation exercises each header-read error stage.
	for k := 0; k < 30 && k < len(full); k++ {
		_, _, _ = ReadBytes(full[:k]) // coverage of binary.Read error returns
	}
	if _, _, err := ReadBytes(full[:0]); err == nil {
		t.Error("empty input should error on version read")
	}
	// Bad identifier byte (offset 2 must be '%').
	bad := make([]byte, len(full))
	copy(bad, full)
	bad[2] = 0x00
	if _, _, err := ReadBytes(bad); err == nil {
		t.Error("bad identifier should error")
	}
}

func TestLoadBytes_TypeMismatchAndShortInput(t *testing.T) {
	// Wrong file type: feed an AMZ file to the mortgage loader.
	if amz, err := os.ReadFile("../../legacy/src/win_source/DOS.AMZ"); err == nil {
		if _, err := LoadMortgageBytes(amz); err == nil {
			t.Error("LoadMortgageBytes on an AMZ file should error (type mismatch)")
		}
	}
	// Too-short input: ReadBytes fails before any record parsing.
	short := []byte{0x01, 0x03}
	if _, err := LoadAmortizationBytes(short); err == nil {
		t.Error("LoadAmortizationBytes on short input should error")
	}
	if _, err := LoadPresentValueBytes(short); err == nil {
		t.Error("LoadPresentValueBytes on short input should error")
	}
}

func TestLoadBytes_TruncatedRecordData(t *testing.T) {
	// Truncate valid fixtures just past the header so ReadBytes succeeds but the
	// record readers run out of data — exercising past-end guards in readByte,
	// readBool and readShortStr.
	for _, f := range []string{
		"../../legacy/src/win_source/DOS.AMZ",
		"../../legacy/src/win_source/DOS.PVL",
	} {
		full, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for k := 33; k <= 70 && k < len(full); k += 3 {
			if hdr, _, err := ReadBytes(full[:k]); err == nil {
				_ = hdr
				switch hdr.FileType {
				case FileTypeAmortization:
					_, _ = LoadAmortizationBytes(full[:k])
				case FileTypePresentValue:
					_, _ = LoadPresentValueBytes(full[:k])
				}
			}
		}
	}
}

func TestLoadFiles_BadPath(t *testing.T) {
	if _, err := LoadMortgageFile("/no/such/file.mtg"); err == nil {
		t.Error("LoadMortgageFile bad path should error")
	}
	if _, err := LoadAmortizationFile("/no/such/file.amz"); err == nil {
		t.Error("LoadAmortizationFile bad path should error")
	}
	if _, err := LoadPresentValueFile("/no/such/file.pvl"); err == nil {
		t.Error("LoadPresentValueFile bad path should error")
	}
}

func TestInt16Helpers_RoundTrip(t *testing.T) {
	for _, v := range []int16{0, 1, -1, 32767, -32768, 1234} {
		b := make([]byte, 2)
		WriteInt16(b, v)
		if got := ReadInt16(b); got != v {
			t.Errorf("ReadInt16 round-trip: got %d, want %d", got, v)
		}
		if got := ReadUint16(b); got != uint16(v) {
			t.Errorf("ReadUint16: got %d, want %d", got, uint16(v))
		}
	}
}

func TestFloat64ToReal48_OverflowUnderflow(t *testing.T) {
	if b := Float64ToReal48(1e300); b != [6]byte{} {
		t.Errorf("overflow should yield zero bytes, got %v", b)
	}
	if b := Float64ToReal48(1e-300); b != [6]byte{} {
		t.Errorf("underflow should yield zero bytes, got %v", b)
	}
	if b := Float64ToReal48(0); b != [6]byte{} {
		t.Errorf("zero should yield zero bytes, got %v", b)
	}
}

func TestNewDateRecFromBytes_Branches(t *testing.T) {
	// Valid month.
	dr := newDateRecFromBytes([3]byte{15, 6, 121})
	if dr.IsUnknown() || dr.Time.Year() != 2021 {
		t.Errorf("valid date parse failed: %+v", dr)
	}
	// Invalid month -> unknown.
	if !newDateRecFromBytes([3]byte{15, 13, 121}).IsUnknown() {
		t.Error("month 13 should be unknown")
	}
	if !newDateRecFromBytes([3]byte{15, 0, 121}).IsUnknown() {
		t.Error("month 0 should be unknown")
	}
}
