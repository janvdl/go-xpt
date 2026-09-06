// Copyright 2026 Jan van der Linde
// SPDX-License-Identifier: Apache-2.0

package goxpt

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// recordSize is the fixed length of every physical record in an XPT file.
	recordSize = 80

	// headerPrefix begins every XPORT header record. The record type marker
	// (e.g. "MEMBER  HEADER RECORD") follows immediately after it.
	headerPrefix = "HEADER RECORD*******"

	// xptTimeLayout matches the "ddMMMyy:hh:mm:ss" datetimes in the headers.
	xptTimeLayout = "02Jan06:15:04:05"
)

// ReadXPTFile opens path and parses every member with [ReadXPT].
func ReadXPTFile(path string) ([]*Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadXPT(f)
}

// ReadXPT parses every member (dataset) contained in an XPT stream. Most XPT
// files hold exactly one member, so callers commonly use the result's first
// element.
//
// The observation section of an XPT file records neither a row count nor an
// end marker; it is simply blank-padded to a multiple of 80 bytes. ReadXPT
// stops at the last non-blank row. A genuine trailing observation whose every
// value is blank (an all-character row of missings) is therefore indistinguishable
// from padding and will be dropped.
func ReadXPT(r io.Reader) ([]*Dataset, error) {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	return (&reader{src: br}).run()
}

type parseState int

const (
	stateStart parseState = iota
	stateLibrary
	stateMember
	stateDescriptor
	stateNamestr
	stateObs
)

// reader holds the state for a single ReadXPT call. It is not reused or shared.
type reader struct {
	src   *bufio.Reader
	rec   [recordSize]byte
	state parseState

	lib      LibraryInfo
	libRecs  [][]byte // real header records following LIBRARY HEADER
	descRecs [][]byte // real header records following DSCRPTR HEADER
	leftover []byte   // bytes carried across records for NAMESTR / OBS parsing

	datasets []*Dataset
	cur      *Dataset // member currently being built
}

func (rd *reader) run() ([]*Dataset, error) {
	for {
		_, err := io.ReadFull(rd.src, rd.rec[:])
		switch {
		case err == nil:
			if e := rd.handleRecord(); e != nil {
				return nil, e
			}
		case errors.Is(err, io.EOF):
			if e := rd.finish(); e != nil {
				return nil, e
			}
			return rd.datasets, nil
		case errors.Is(err, io.ErrUnexpectedEOF):
			return nil, fmt.Errorf("goxpt: truncated stream: length is not a multiple of %d bytes", recordSize)
		default:
			return nil, fmt.Errorf("goxpt: reading record: %w", err)
		}
	}
}

func (rd *reader) finish() error {
	rd.finalizeLibrary()
	rd.finalizeMember()
	return rd.finalizeObs()
}

func (rd *reader) handleRecord() error {
	rec := rd.rec[:]
	if bytes.HasPrefix(rec, []byte(headerPrefix)) {
		return rd.handleHeader(rec)
	}

	switch rd.state {
	case stateLibrary:
		rd.libRecs = append(rd.libRecs, cloneRecord(rec))
	case stateDescriptor:
		rd.descRecs = append(rd.descRecs, cloneRecord(rec))
	case stateNamestr:
		return rd.parseNamestr(rec)
	case stateObs:
		rd.parseObs(rec)
	}
	return nil
}

func (rd *reader) handleHeader(rec []byte) error {
	switch {
	case marker(rec, "LIBRARY HEADER RECORD"):
		rd.state = stateLibrary
		rd.libRecs = rd.libRecs[:0]

	case marker(rec, "MEMBER  HEADER RECORD"):
		rd.finalizeLibrary()
		if err := rd.finalizeObs(); err != nil { // close the previous member, if any
			return err
		}
		ds := &Dataset{Library: rd.lib, descriptorSize: descriptorSize(rec)}
		rd.datasets = append(rd.datasets, ds)
		rd.cur = ds
		rd.state = stateMember

	case marker(rec, "DSCRPTR HEADER RECORD"):
		rd.state = stateDescriptor
		rd.descRecs = rd.descRecs[:0]

	case marker(rec, "NAMESTR HEADER RECORD"):
		if rd.cur == nil {
			return errors.New("goxpt: NAMESTR header before MEMBER header")
		}
		rd.finalizeMember()
		n, err := namestrCount(rec)
		if err != nil {
			return err
		}
		rd.cur.numVars = n
		rd.leftover = rd.leftover[:0]
		rd.state = stateNamestr

	case marker(rec, "OBS     HEADER RECORD"):
		if rd.cur == nil {
			return errors.New("goxpt: OBS header before MEMBER header")
		}
		rd.cur.rowSize = observationSize(rd.cur.Variables)
		rd.leftover = rd.leftover[:0]
		rd.state = stateObs
	}
	return nil
}

// finalizeLibrary decodes the two real header records that follow LIBRARY HEADER.
func (rd *reader) finalizeLibrary() {
	if len(rd.libRecs) == 0 {
		return
	}
	r1 := rd.libRecs[0]
	rd.lib.SASVersion = trimField(r1[24:32])
	rd.lib.OS = trimField(r1[32:40])
	rd.lib.Created = parseXPTTime(r1[64:80])
	if len(rd.libRecs) > 1 {
		rd.lib.Modified = parseXPTTime(rd.libRecs[1][0:16])
	}
	rd.libRecs = rd.libRecs[:0]
}

// finalizeMember decodes the two real header records that follow DSCRPTR HEADER.
func (rd *reader) finalizeMember() {
	ds := rd.cur
	if ds == nil || len(rd.descRecs) == 0 {
		return
	}
	r1 := rd.descRecs[0]
	ds.Member.Name = trimField(r1[8:16])
	ds.Member.SASVersion = trimField(r1[24:32])
	ds.Member.OS = trimField(r1[32:40])
	ds.Member.Created = parseXPTTime(r1[64:80])
	if len(rd.descRecs) > 1 {
		r2 := rd.descRecs[1]
		ds.Member.Modified = parseXPTTime(r2[0:16])
		ds.Member.Label = trimField(r2[32:72])
		ds.Member.Type = trimField(r2[72:80])
	}
	rd.descRecs = rd.descRecs[:0]
}

func (rd *reader) parseNamestr(rec []byte) error {
	ds := rd.cur
	if ds == nil {
		return errors.New("goxpt: NAMESTR record before MEMBER header")
	}
	if ds.descriptorSize == 0 {
		ds.descriptorSize = 140
	}

	rd.leftover = append(rd.leftover, rec...)
	for len(rd.leftover) >= ds.descriptorSize && len(ds.Variables) < ds.numVars {
		v, err := parseNamestrRecord(rd.leftover[:ds.descriptorSize])
		if err != nil {
			return err
		}
		ds.Variables = append(ds.Variables, v)
		rd.leftover = rd.leftover[ds.descriptorSize:]
	}
	return nil
}

func parseNamestrRecord(b []byte) (Variable, error) {
	if len(b) < 88 {
		return Variable{}, fmt.Errorf("goxpt: NAMESTR record too short: %d bytes", len(b))
	}
	u16 := func(off int) int { return int(binary.BigEndian.Uint16(b[off : off+2])) }

	v := Variable{
		Num:            u16(6),
		Length:         u16(4),
		Name:           trimField(b[8:16]),
		Label:          trimField(b[16:56]),
		Format:         trimField(b[56:64]),
		FormatLength:   u16(64),
		FormatDecimals: u16(66),
		Informat:       trimField(b[72:80]),
	}

	switch code := u16(0); code {
	case 1:
		v.Type = TypeNumeric
	case 2:
		v.Type = TypeCharacter
	default:
		return Variable{}, fmt.Errorf("goxpt: variable %q: unknown type code %d", v.Name, code)
	}
	if v.Length < 1 || v.Length > 32767 {
		return Variable{}, fmt.Errorf("goxpt: variable %q: invalid length %d", v.Name, v.Length)
	}
	if v.Type == TypeNumeric && v.Length > 8 {
		return Variable{}, fmt.Errorf("goxpt: numeric variable %q: invalid length %d", v.Name, v.Length)
	}
	return v, nil
}

func (rd *reader) parseObs(rec []byte) {
	ds := rd.cur
	rd.leftover = append(rd.leftover, rec...)
	if ds == nil || ds.rowSize <= 0 {
		return
	}
	// A full row followed by at least one more physical record cannot be
	// trailing padding: padding is always shorter than a record (80 bytes) and
	// only appears at the very end of the observation section. Decode those
	// rows now so the whole section need not be held in memory at once.
	for len(rd.leftover) >= ds.rowSize+recordSize {
		decodeRow(ds, rd.leftover[:ds.rowSize])
		rd.leftover = rd.leftover[ds.rowSize:]
	}
}

func (rd *reader) finalizeObs() error {
	ds := rd.cur
	if ds == nil || rd.state != stateObs {
		return nil
	}
	rd.state = stateStart

	data := rd.leftover
	rd.leftover = nil
	if ds.rowSize <= 0 {
		return nil
	}

	pad := len(data) % ds.rowSize
	if !allBlank(data[len(data)-pad:]) {
		return fmt.Errorf("goxpt: member %q: corrupt OBS section, %d trailing bytes are not padding",
			ds.Member.Name, pad)
	}
	realLen := len(data) - pad
	// Drop trailing all-blank rows that lie inside the final padded record.
	for realLen >= ds.rowSize &&
		realLen-ds.rowSize >= len(data)-recordSize &&
		allBlank(data[realLen-ds.rowSize:realLen]) {
		realLen -= ds.rowSize
	}
	for off := 0; off+ds.rowSize <= realLen; off += ds.rowSize {
		decodeRow(ds, data[off:off+ds.rowSize])
	}
	return nil
}

func decodeRow(ds *Dataset, row []byte) {
	pos := 0
	for i := range ds.Variables {
		v := &ds.Variables[i]
		if pos+v.Length > len(row) {
			break
		}
		raw := row[pos : pos+v.Length]
		pos += v.Length

		var cell DataCell
		if v.Type == TypeNumeric {
			var buf [8]byte
			copy(buf[:], raw) // short numerics are left-aligned; pad with zero bytes
			cell.Numeric, cell.Missing, cell.MissingCode = ibmToFloat64(buf[:])
		} else {
			cell.Char = strings.TrimRight(string(raw), " \x00")
		}
		v.Data = append(v.Data, cell)
	}
}

// marker reports whether rec is the header record for the given type.
func marker(rec []byte, name string) bool {
	const off = len(headerPrefix)
	return len(rec) >= off && bytes.HasPrefix(rec[off:], []byte(name))
}

// descriptorSize returns the NAMESTR/observation descriptor width declared in a
// MEMBER header record: 136 on VAX/VMS, 140 everywhere else.
func descriptorSize(rec []byte) int {
	trailer := strings.TrimRight(string(rec), " \x00")
	if strings.HasSuffix(trailer, "136") {
		return 136
	}
	return 140
}

// namestrCount returns the variable count declared in a NAMESTR header record.
func namestrCount(rec []byte) (int, error) {
	if len(rec) < 58 {
		return 0, errors.New("goxpt: NAMESTR header record too short")
	}
	field := strings.TrimSpace(string(rec[54:58]))
	n, err := strconv.Atoi(field)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("goxpt: invalid variable count %q in NAMESTR header", field)
	}
	return n, nil
}

func observationSize(vars []Variable) int {
	n := 0
	for i := range vars {
		n += vars[i].Length
	}
	return n
}

func cloneRecord(rec []byte) []byte {
	return append([]byte(nil), rec...)
}

func trimField(b []byte) string {
	return strings.TrimRight(string(b), " \x00")
}

// allBlank reports whether b is empty or entirely ASCII spaces (the pad byte
// mandated by the XPORT spec).
func allBlank(b []byte) bool {
	for _, c := range b {
		if c != ' ' {
			return false
		}
	}
	return true
}

func parseXPTTime(b []byte) time.Time {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(xptTimeLayout, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
