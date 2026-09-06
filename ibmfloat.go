// Copyright 2026 Jan van der Linde
// SPDX-License-Identifier: Apache-2.0

package goxpt

import (
	"encoding/binary"
	"math"
)

// ibmToFloat64 converts an 8-byte IBM System/360 hexadecimal floating-point
// value (big-endian, as stored in XPT files) to a float64.
//
// SAS missing values are encoded as a marker byte followed by seven zero bytes:
// '.' (0x2E) for an ordinary missing and '_' (0x5F) or 'A'-'Z' (0x41-0x5A) for
// the special missings. For those, missing is true, code is the marker byte
// (0 for an ordinary '.'), and value is NaN.
//
// b must be exactly 8 bytes.
func ibmToFloat64(b []byte) (value float64, missing bool, code byte) {
	_ = b[7] // bounds-check hint; callers always pass 8 bytes

	// A zero mantissa means either 0.0 or a missing-value marker.
	if b[1]|b[2]|b[3]|b[4]|b[5]|b[6]|b[7] == 0 {
		switch c := b[0]; {
		case c == 0x00:
			return 0, false, 0
		case c == '.':
			return math.NaN(), true, 0
		case c == '_' || (c >= 'A' && c <= 'Z'):
			return math.NaN(), true, c
		}
	}

	// value = (-1)^sign * f * 16^(exp-64), where f is the 56-bit fraction over
	// 2^56. That is value = f * 2^(4*exp - 312).
	exp := int(b[0] & 0x7f)
	fraction := binary.BigEndian.Uint64(b) & 0x00ffffffffffffff
	value = math.Ldexp(float64(fraction), 4*exp-312)
	if b[0]&0x80 != 0 {
		value = -value
	}
	return value, false, 0
}
