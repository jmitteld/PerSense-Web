// Package fileio provides functions for reading and writing legacy Per%Sense
// binary screen files. The legacy format uses Turbo Pascal's 6-byte real48
// floating-point representation for all monetary values.
//
// Ported from legacy/source/FileIOUnit.pas
package fileio

import (
	"encoding/binary"
	"math"
)

// Real48ToFloat64 converts a 6-byte Turbo Pascal Real48 (also known as
// real48 or Real(6)) to a Go float64.
//
// Real48 format (little-endian):
//
//	Byte 0: biased exponent (0 = zero, 1..255 = exponent + 129)
//	Bytes 1-4: 32 bits of mantissa (implicit leading 1)
//	Byte 5 bit 7: sign (0 = positive, 1 = negative)
//	Byte 5 bits 0-6: high 7 bits of mantissa
//
// The full mantissa is 39 bits: 7 bits from byte 5 + 32 bits from bytes 1-4.
// There is an implicit leading 1 bit, giving 40 bits of significand total.
func Real48ToFloat64(b [6]byte) float64 {
	exp := int(b[0])
	if exp == 0 {
		return 0 // special case: zero
	}

	// Sign from high bit of byte 5
	sign := 1.0
	if b[5]&0x80 != 0 {
		sign = -1.0
	}

	// Build the 39-bit mantissa from bytes 1-5
	// Byte 5 low 7 bits are the highest mantissa bits
	// Bytes 4,3,2,1 are the lower 32 bits
	mantissa := uint64(b[5]&0x7F) << 32
	mantissa |= uint64(b[4]) << 24
	mantissa |= uint64(b[3]) << 16
	mantissa |= uint64(b[2]) << 8
	mantissa |= uint64(b[1])

	// The mantissa represents 0.1mmm...m in binary (implicit leading 1)
	// So the value is (1 + mantissa / 2^39) * 2^(exp - 129)
	frac := float64(mantissa) / float64(uint64(1)<<39)
	value := (1 + frac) * math.Pow(2, float64(exp-129))

	return sign * value
}

// Float64ToReal48 converts a Go float64 to a 6-byte Turbo Pascal Real48.
func Float64ToReal48(f float64) [6]byte {
	var b [6]byte

	if f == 0 {
		return b // all zeros
	}

	sign := byte(0)
	if f < 0 {
		sign = 0x80
		f = -f
	}

	// Decompose: f = frac * 2^exp where 1 <= frac < 2
	frac, exp := math.Frexp(f)
	// math.Frexp returns 0.5 <= frac < 1, exp such that f = frac * 2^exp
	// We need 1 <= frac < 2, so multiply frac by 2 and decrement exp
	frac *= 2
	exp--

	// Biased exponent
	biasedExp := exp + 129
	if biasedExp < 1 || biasedExp > 255 {
		return b // underflow/overflow → zero
	}
	b[0] = byte(biasedExp)

	// Remove the implicit leading 1
	frac -= 1.0

	// Convert to 39-bit integer mantissa
	mantissa := uint64(frac * float64(uint64(1)<<39))

	b[5] = sign | byte((mantissa>>32)&0x7F)
	b[4] = byte(mantissa >> 24)
	b[3] = byte(mantissa >> 16)
	b[2] = byte(mantissa >> 8)
	b[1] = byte(mantissa)

	return b
}

// DateRecBytes represents the on-disk format of a Pascal daterec: 3 bytes (d, m, y).
type DateRecBytes [3]byte

// ReadInt16 reads a little-endian int16 from a byte slice.
func ReadInt16(b []byte) int16 {
	return int16(binary.LittleEndian.Uint16(b))
}

// WriteInt16 writes a little-endian int16 to a byte slice.
func WriteInt16(b []byte, v int16) {
	binary.LittleEndian.PutUint16(b, uint16(v))
}

// ReadUint16 reads a little-endian uint16 from a byte slice.
func ReadUint16(b []byte) uint16 {
	return binary.LittleEndian.Uint16(b)
}
