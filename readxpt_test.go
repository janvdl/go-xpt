// Copyright 2026 Jan van der Linde
// SPDX-License-Identifier: Apache-2.0

package goxpt

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// --- helpers for assembling a minimal, spec-shaped XPT stream ---------------

func blankRecord() []byte { return bytes.Repeat([]byte{' '}, recordSize) }

func fill(dst []byte, s string) {
	for i := range dst {
		dst[i] = ' '
	}
	copy(dst, s)
}

func headerRecord(name string) []byte {
	rec := blankRecord()
	copy(rec, headerPrefix+name+"!!!!!!!")
	return rec
}

func namestrHeaderRecord(n int) []byte {
	rec := blankRecord()
	copy(rec, fmt.Sprintf("%sNAMESTR HEADER RECORD!!!!!!!%06d%04d", headerPrefix, 0, n))
	return rec
}

func namestrRecord(num, typ, length int, name, label string) []byte {
	b := make([]byte, 140)
	binary.BigEndian.PutUint16(b[0:2], uint16(typ))
	binary.BigEndian.PutUint16(b[4:6], uint16(length))
	binary.BigEndian.PutUint16(b[6:8], uint16(num))
	fill(b[8:16], name)
	fill(b[16:56], label)
	fill(b[56:64], "")
	fill(b[72:80], "")
	return b
}

// ibmBytes encodes f as an 8-byte IBM hexadecimal float (test-only encoder).
func ibmBytes(f float64) []byte {
	b := make([]byte, 8)
	if f == 0 {
		return b
	}
	neg := math.Signbit(f)
	f = math.Abs(f)
	exp := 0
	for f >= 1 {
		f /= 16
		exp++
	}
	for f < 1.0/16 {
		f *= 16
		exp--
	}
	frac := uint64(math.Round(f * math.Exp2(56)))
	b[0] = byte(exp + 64)
	if neg {
		b[0] |= 0x80
	}
	for i := 1; i < 8; i++ {
		b[i] = byte(frac >> (8 * (7 - i)))
	}
	return b
}

func pad80(b *bytes.Buffer) {
	for b.Len()%recordSize != 0 {
		b.WriteByte(' ')
	}
}

func buildXPT() []byte {
	var buf bytes.Buffer

	buf.Write(headerRecord("LIBRARY HEADER RECORD"))
	lib1 := blankRecord()
	fill(lib1[0:8], "SAS")
	fill(lib1[8:16], "SAS")
	fill(lib1[16:24], "SASLIB")
	fill(lib1[24:32], "9.4")
	fill(lib1[32:40], "LINUX")
	fill(lib1[64:80], "06SEP26:12:00:00")
	buf.Write(lib1)
	lib2 := blankRecord()
	fill(lib2[0:16], "06SEP26:13:00:00")
	buf.Write(lib2)

	buf.Write(headerRecord("MEMBER  HEADER RECORD"))
	buf.Write(headerRecord("DSCRPTR HEADER RECORD"))
	m1 := blankRecord()
	fill(m1[0:8], "SAS")
	fill(m1[8:16], "TESTDS")
	fill(m1[16:24], "SASDATA")
	fill(m1[24:32], "9.4")
	fill(m1[32:40], "LINUX")
	fill(m1[64:80], "06SEP26:12:00:00")
	buf.Write(m1)
	m2 := blankRecord()
	fill(m2[0:16], "06SEP26:12:30:00")
	fill(m2[32:72], "Test dataset")
	fill(m2[72:80], "")
	buf.Write(m2)

	buf.Write(namestrHeaderRecord(3))
	var ns bytes.Buffer
	ns.Write(namestrRecord(1, 1, 8, "NUM1", "First number"))
	ns.Write(namestrRecord(2, 1, 4, "NUM2", "Second number"))
	ns.Write(namestrRecord(3, 2, 3, "CHARV", "A string"))
	pad80(&ns)
	buf.Write(ns.Bytes())

	buf.Write(headerRecord("OBS     HEADER RECORD"))
	var obs bytes.Buffer
	obs.Write(ibmBytes(1.0))
	obs.Write(ibmBytes(2.5)[:4])
	obs.WriteString("foo")
	obs.Write([]byte{'.', 0, 0, 0, 0, 0, 0, 0})
	obs.Write(ibmBytes(3.0)[:4])
	obs.WriteString("ba ")
	pad80(&obs)
	buf.Write(obs.Bytes())

	return buf.Bytes()
}

// --- tests ------------------------------------------------------------------

func TestReadXPT(t *testing.T) {
	dss, err := ReadXPT(bytes.NewReader(buildXPT()))
	if err != nil {
		t.Fatalf("ReadXPT: %v", err)
	}
	if len(dss) != 1 {
		t.Fatalf("got %d datasets, want 1", len(dss))
	}
	ds := dss[0]

	if ds.Library.SASVersion != "9.4" {
		t.Errorf("library version = %q, want 9.4", ds.Library.SASVersion)
	}
	if want := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC); !ds.Library.Created.Equal(want) {
		t.Errorf("library created = %v, want %v", ds.Library.Created, want)
	}
	if ds.Member.Name != "TESTDS" {
		t.Errorf("member name = %q, want TESTDS", ds.Member.Name)
	}
	if ds.Member.Label != "Test dataset" {
		t.Errorf("member label = %q, want %q", ds.Member.Label, "Test dataset")
	}
	if len(ds.Variables) != 3 || ds.Variables[1].Length != 4 {
		t.Fatalf("variables = %+v", ds.Variables)
	}
	if got := ds.NumRows(); got != 2 {
		t.Fatalf("NumRows = %d, want 2", got)
	}
	if !ds.Variables[0].Data[1].Missing {
		t.Errorf("NUM1 row 2 should be a missing value")
	}

	want := [][]string{
		{"NUM1", "NUM2", "CHARV"},
		{"1", "2.5", "foo"},
		{".", "3", "ba"},
	}
	if got := ds.AsSimpleGrid(); !reflect.DeepEqual(got, want) {
		t.Errorf("AsSimpleGrid mismatch:\n got %v\nwant %v", got, want)
	}
}

func TestReadXPTTruncated(t *testing.T) {
	data := buildXPT()
	if _, err := ReadXPT(bytes.NewReader(data[:len(data)-3])); err == nil {
		t.Fatal("expected an error for a truncated stream")
	}
}

func TestReadXPTFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xpt")
	if err := os.WriteFile(path, buildXPT(), 0o644); err != nil {
		t.Fatal(err)
	}
	dss, err := ReadXPTFile(path)
	if err != nil {
		t.Fatalf("ReadXPTFile: %v", err)
	}
	if len(dss) != 1 || dss[0].NumRows() != 2 {
		t.Fatalf("unexpected result: %+v", dss)
	}
}
