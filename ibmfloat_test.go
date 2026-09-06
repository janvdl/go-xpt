// Copyright 2026 Jan van der Linde
// SPDX-License-Identifier: Apache-2.0

package goxpt

import (
	"math"
	"testing"
)

func TestIBMToFloat64(t *testing.T) {
	cases := []struct {
		b    []byte
		want float64
	}{
		{[]byte{0x41, 0x10, 0, 0, 0, 0, 0, 0}, 1},
		{[]byte{0xC1, 0x10, 0, 0, 0, 0, 0, 0}, -1},
		{[]byte{0x41, 0x28, 0, 0, 0, 0, 0, 0}, 2.5},
		{[]byte{0x40, 0x80, 0, 0, 0, 0, 0, 0}, 0.5},
		{[]byte{0x42, 0x64, 0, 0, 0, 0, 0, 0}, 100},
		{make([]byte, 8), 0},
	}
	for _, c := range cases {
		got, missing, _ := ibmToFloat64(c.b)
		if missing || got != c.want {
			t.Errorf("ibmToFloat64(% x) = %v (missing=%v), want %v", c.b, got, missing, c.want)
		}
	}

	if _, missing, code := ibmToFloat64([]byte{'.', 0, 0, 0, 0, 0, 0, 0}); !missing || code != 0 {
		t.Errorf("ordinary missing '.' not detected (missing=%v code=%q)", missing, code)
	}
	if v, missing, code := ibmToFloat64([]byte{'B', 0, 0, 0, 0, 0, 0, 0}); !missing || code != 'B' || !math.IsNaN(v) {
		t.Errorf("special missing .B not detected (v=%v missing=%v code=%q)", v, missing, code)
	}
	if _, missing, code := ibmToFloat64([]byte{'_', 0, 0, 0, 0, 0, 0, 0}); !missing || code != '_' {
		t.Errorf("special missing ._ not detected (missing=%v code=%q)", missing, code)
	}
}
