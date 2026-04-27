package fileio

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// File format version constants.
// Ported from legacy/source/FileIOUnit.pas
const (
	Version11 uint16 = 1<<8 + 5  // v1.05
	Version20 uint16 = 2 << 8    // v2.00
	Version22 uint16 = 2<<8 + 2  // v2.02
	Version30 uint16 = 3 << 8    // v3.00
	Version31 uint16 = 3<<8 + 1  // v3.01
)

// FileType identifies the screen type stored in a legacy file.
type FileType byte

const (
	FileTypeMortgage     FileType = FileType(types.BlockMTG)
	FileTypeAmortization FileType = FileType(types.BlockAMZTop)
	FileTypePresentValue FileType = FileType(types.BlockPVLLumpSum)
)

// GridHeader represents one block's metadata in the file header.
// Ported from legacy/source/FileIOUnit.pas: TGridHeader
type GridHeader struct {
	GridID         byte
	ScrollPosition byte
	LineCount      byte
}

// CompDefaultsOnDisk mirrors the Pascal compdefaults record as stored on disk.
// Total size: 10 bytes.
type CompDefaultsOnDisk struct {
	COLAMonth  byte // 1
	CenturyDiv byte // 1
	PerYr      byte // 1
	USARule    byte // 1 (boolean)
	Basis      byte // 1 (enum: 0=x365, 1=x360, 2=x365_360)
	Prepaid    byte // 1
	InAdvance  byte // 1
	PlusRegular byte // 1
	Exact      byte // 1
	R78        byte // 1
}

// FileHeader holds the parsed header of a legacy Per%Sense file.
type FileHeader struct {
	Version   uint16
	FancyByte byte
	CompDefs  CompDefaultsOnDisk
	Grids     [8]GridHeader
	FileType  FileType
}

// ReadFile reads and parses a legacy Per%Sense binary file, returning
// the header and the raw data section as bytes for further parsing.
//
// File format:
//
//	2 bytes: version (uint16 LE)
//	1 byte: identifier (must be '%')
//	1 byte: fancy byte
//	10 bytes: CompDefaults
//	24 bytes: 8 × GridHeader (3 bytes each)
//	variable: screen-specific data
//
// Ported from legacy/source/FileIOUnit.pas: TFileIO.LoadFile
func ReadFile(path string) (*FileHeader, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	var hdr FileHeader

	// Version
	if err := binary.Read(f, binary.LittleEndian, &hdr.Version); err != nil {
		return nil, nil, fmt.Errorf("reading version: %w", err)
	}

	// Identifier
	var identifier byte
	if err := binary.Read(f, binary.LittleEndian, &identifier); err != nil {
		return nil, nil, fmt.Errorf("reading identifier: %w", err)
	}
	if identifier != '%' {
		return nil, nil, fmt.Errorf("invalid file: identifier byte is 0x%02X, want '%%'", identifier)
	}

	// Fancy byte
	if err := binary.Read(f, binary.LittleEndian, &hdr.FancyByte); err != nil {
		return nil, nil, fmt.Errorf("reading fancy byte: %w", err)
	}

	// CompDefaults
	if err := binary.Read(f, binary.LittleEndian, &hdr.CompDefs); err != nil {
		return nil, nil, fmt.Errorf("reading comp defaults: %w", err)
	}

	// Grid headers
	for i := 0; i < 8; i++ {
		if err := binary.Read(f, binary.LittleEndian, &hdr.Grids[i].GridID); err != nil {
			return nil, nil, fmt.Errorf("reading grid header %d: %w", i, err)
		}
		if err := binary.Read(f, binary.LittleEndian, &hdr.Grids[i].ScrollPosition); err != nil {
			return nil, nil, err
		}
		if err := binary.Read(f, binary.LittleEndian, &hdr.Grids[i].LineCount); err != nil {
			return nil, nil, err
		}
	}

	hdr.FileType = FileType(hdr.Grids[0].GridID)

	// Read remaining data
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, nil, fmt.Errorf("reading data: %w", err)
	}

	return &hdr, data, nil
}

// readReal48 reads a 6-byte Real48 from the current position in data,
// advancing pos by 6.
func readReal48(data []byte, pos *int) float64 {
	if *pos+6 > len(data) {
		return 0
	}
	var b [6]byte
	copy(b[:], data[*pos:*pos+6])
	*pos += 6
	return Real48ToFloat64(b)
}

// readInt8 reads a signed byte (inout status), advancing pos by 1.
func readInt8(data []byte, pos *int) int8 {
	if *pos >= len(data) {
		return 0
	}
	v := int8(data[*pos])
	*pos++
	return v
}

// readByte reads an unsigned byte, advancing pos by 1.
func readByte(data []byte, pos *int) byte {
	if *pos >= len(data) {
		return 0
	}
	v := data[*pos]
	*pos++
	return v
}

// readBool reads a Pascal boolean (1 byte), advancing pos by 1.
func readBool(data []byte, pos *int) bool {
	return readByte(data, pos) != 0
}

// readInt16LE reads a little-endian int16, advancing pos by 2.
func readInt16LE(data []byte, pos *int) int16 {
	if *pos+2 > len(data) {
		return 0
	}
	v := int16(binary.LittleEndian.Uint16(data[*pos : *pos+2]))
	*pos += 2
	return v
}

// readDateRec reads a 3-byte Pascal daterec (d, m, y) and converts to DateRec.
// Pascal daterec: d (shortint=1 byte), m (shortint=1 byte), y (byte=1 byte)
// Calendar year = y + 1900
func readDateRec(data []byte, pos *int) types.DateRec {
	if *pos+3 > len(data) {
		return types.UnknownDate()
	}
	d := int(int8(data[*pos]))
	m := int(int8(data[*pos+1]))
	y := int(data[*pos+2])
	*pos += 3

	if m < 1 || m > 12 || d < 1 || d > 31 {
		return types.UnknownDate()
	}
	return types.NewDateRec(y+1900, time.Month(m), d)
}

// readShortStr reads a Pascal short string of fixed max length.
func readShortStr(data []byte, pos *int, maxLen int) string {
	if *pos+maxLen > len(data) {
		*pos += maxLen
		return ""
	}
	// Pascal short strings: first byte is length, then characters
	// But in this file format, it appears to be stored as fixed-length
	raw := data[*pos : *pos+maxLen]
	*pos += maxLen

	// Find null or end
	end := maxLen
	for i := 0; i < maxLen; i++ {
		if raw[i] == 0 {
			end = i
			break
		}
	}
	return string(raw[:end])
}

// skip advances pos by n bytes.
func skip(pos *int, n int) {
	*pos += n
}
