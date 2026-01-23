/*
	go-xpt: an open-source, Go solution to reading/writing XPT (SAS Transport) files.
    Copyright (C) 2026  Jan van der Linde

    This program is free software: you can redistribute it and/or modify
    it under the terms of the GNU General Public License as published by
    the Free Software Foundation, either version 3 of the License, or
    (at your option) any later version.

    This program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU General Public License for more details.

    You should have received a copy of the GNU General Public License
    along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
)

// all SAS records are 80 bytes in length and padded with
// ASCII blanks where necessary to reach this length
const recordSize = 80

// buffer for building 136 or 140 byte records
// also reused for building data rows with length Dataset.dataRecordSize
var buffer []byte = []byte{}

// states to keep track of XPORT headers
type HeaderState int
type VariableType int

const (
	NON_HEADER HeaderState = iota
	LIB_HEADER
	MEM_HEADER
	DES_HEADER
	NAM_HEADER
	OBS_HEADER
)

const (
	NUMERIC VariableType = iota
	CHARACTER
)

// header structs
type LibraryRecord struct {
	sas_symbol1 [8]byte
	sas_symbol2 [8]byte
	sas_lib     [8]byte
	sas_ver     [8]byte
	sas_os      [8]byte
	blanks      [24]byte
	sas_create  [16]byte
}

type MemberRecord struct {
	sas_symbol [8]byte
	sas_dsname [8]byte
	sas_data   [8]byte
	sas_ver    [8]byte
	sas_os     [8]byte
	blanks     [24]byte
	sas_create [16]byte
}

type MemberRecord2 struct {
	dtmod_day    [2]byte
	dtmod_month  [3]byte
	dtmod_year   [2]byte
	dtmod_colon1 [1]byte
	dtmod_hour   [2]byte
	dtmod_colon2 [1]byte
	dtmod_minute [2]byte
	dtmod_colon3 [1]byte
	dtmod_second [2]byte
	padding      [16]byte
	ds_label     [40]byte
	ds_type      [8]byte
}

type NameStrRecord struct {
	ntype  [2]byte
	nhfun  [2]byte
	nlng   [2]byte
	nvar0  [2]byte
	nname  [8]byte
	nlabel [40]byte
	nform  [8]byte
	nfl    [2]byte
	nfd    [2]byte
	nfj    [2]byte
	nfill  [2]byte
	niform [8]byte
	nifl   [2]byte
	nifd   [2]byte
	npos   [4]byte
	rest   [52]byte
}

// Variable struct for observation records
type Variable struct {
	varnum  int
	name    string
	label   string
	length  int
	vartype VariableType
	data    []DataCell
}

// Data cell struct to be packed into Variables
type DataCell struct {
	value_numeric float64
	value_char    string
}

type Dataset struct {
	descriptorSize int // either 136 (VAX systems) or 140 bytes per NAMESTR record
	numOfVars      int // how many variables are expected in the dataset
	dataRecordSize int // how many bytes are occupied by one row of the dataset
	LibRec         LibraryRecord
	MemRec1        MemberRecord
	MemRec2        MemberRecord2
	NamRecs        []NameStrRecord
	Vars           []Variable
}

func readXPT(path string) (*Dataset, error) {
	eof := false
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	ds := &Dataset{}

	// set initial state
	currentState := NON_HEADER

	for !eof {
		rec, err := readRecord(r)

		if err != nil {
			log.Println(err.Error())

			if err.Error() == "EOF" {
				eof = true
				break
			} else {
				return ds, err
			}
		}

		// parse record by record and switch from one header state to the next as needed
		rec_str := string(rec)
		fmt.Println(rec_str)

		// check if rec is a header record
		if strings.Contains(rec_str, "HEADER RECORD*******") {
			if strings.Contains(rec_str, "HEADER RECORD*******LIBRARY HEADER RECORD!!!!!!!") {
				currentState = LIB_HEADER
				continue
			} else if strings.Contains(rec_str, "HEADER RECORD*******MEMBER  HEADER RECORD!!!!!!!") {
				currentState = MEM_HEADER

				// parse the MEMBER header record to understand whether NAMESTR and OBS are 140 or 136 bytes
				parseMemHeader(rec, ds)
				continue
			} else if strings.Contains(rec_str, "HEADER RECORD*******DSCRPTR HEADER RECORD!!!!!!!") {
				currentState = DES_HEADER
				continue
			} else if strings.Contains(rec_str, "HEADER RECORD*******NAMESTR HEADER RECORD!!!!!!!") {
				currentState = NAM_HEADER

				// parse the NAMESTR header record to get the number of vars expected
				parseNamHeader(rec, ds)
				continue
			} else if strings.Contains(rec_str, "HEADER RECORD*******OBS     HEADER RECORD!!!!!!!") {
				currentState = OBS_HEADER

				// calculate how long each data record is expected to be based on the sizes defined in the NAMESTR records
				calculateDataRecordSize(ds)

				// clear the buffer of any namestr remnants
				buffer = []byte{}
				continue
			}
		} else {
			// not a header record - depending on current state, route accordingly
			switch currentState {
			case LIB_HEADER:
				parseLibRecord(rec, ds)
			case MEM_HEADER:
				parseMemRecord(rec, ds)
			case DES_HEADER:
				parseDesRecord(rec, ds)
			case NAM_HEADER:
				parseNamRecord(rec, ds)
			case OBS_HEADER:
				parseObsRecord(rec, ds)
			}
		}
	}

	return ds, err
}

func readRecord(r *bufio.Reader) ([]byte, error) {
	buf := make([]byte, recordSize)
	_, err := io.ReadFull(r, buf)

	if err != nil {
		return buf, err
	}

	return buf, nil
}

func calculateDataRecordSize(ds *Dataset) {
	for i := range ds.Vars {
		v := &ds.Vars[i]
		ds.dataRecordSize += v.length
	}
}

func parseLibRecord(rec []byte, ds *Dataset) {

}

func parseMemHeader(rec []byte, ds *Dataset) {
	// get the size of the variable descriptor record
	// usually 140 bytes but 136 on VAX/VMS systems
	desSize := string(rec[75:78])
	if desSize == "140" {
		ds.descriptorSize = 140
	} else {
		ds.descriptorSize = 136
	}
}

func parseMemRecord(rec []byte, ds *Dataset) {

}

func parseDesRecord(rec []byte, ds *Dataset) {

}

func parseNamHeader(rec []byte, ds *Dataset) {
	numOfVars, err := strconv.Atoi(string(rec[54:58]))

	if err != nil {
		panic(err)
	}

	ds.numOfVars = numOfVars
}

func parseNamRecord(rec []byte, ds *Dataset) {
	buffer = append(buffer, rec...)
	for len(buffer) >= ds.descriptorSize {
		// select 136/140 bytes, this is a full namestr record
		// retain the remainder in the buffer until another full record is reached
		tmp := buffer[0:ds.descriptorSize]
		buffer = buffer[ds.descriptorSize:]

		nam := NameStrRecord{}
		copy(nam.ntype[:], tmp[0:2])
		copy(nam.nhfun[:], tmp[2:4])
		copy(nam.nlng[:], tmp[4:6])
		copy(nam.nvar0[:], tmp[6:8])
		copy(nam.nname[:], tmp[8:16])
		copy(nam.nlabel[:], tmp[16:56])
		copy(nam.nform[:], tmp[56:64])
		copy(nam.nfl[:], tmp[64:66])
		copy(nam.nfd[:], tmp[66:68])
		copy(nam.nfj[:], tmp[68:70])
		copy(nam.nfill[:], tmp[70:72])
		copy(nam.niform[:], tmp[72:80])
		copy(nam.nifl[:], tmp[80:82])
		copy(nam.nifd[:], tmp[82:84])
		copy(nam.npos[:], tmp[84:86])
		copy(nam.rest[:], tmp[86:])

		ds.NamRecs = append(ds.NamRecs, nam)

		// human friendly var, i.e., not just a bunch of bytes
		v := Variable{}
		v.varnum = int(binary.BigEndian.Uint16(nam.nvar0[:]))
		v.length = int(binary.BigEndian.Uint16(nam.nlng[:]))
		v.name = strings.TrimSpace(string(nam.nname[:]))
		v.label = strings.TrimSpace(string(nam.nlabel[:]))
		v.data = []DataCell{}

		if vartype := int(binary.BigEndian.Uint16(nam.ntype[:])); vartype == 1 {
			v.vartype = NUMERIC
		} else {
			v.vartype = CHARACTER
		}

		ds.Vars = append(ds.Vars, v)
	}
}

func parseObsRecord(rec []byte, ds *Dataset) {
	// TODO: parse missings according to XPT standards doc

	buffer = append(buffer, rec...)
	for len(buffer) >= ds.dataRecordSize {
		for i := range ds.Vars {
			v := &ds.Vars[i]

			l := v.length
			tmp := buffer[0:l]
			buffer = buffer[l:]

			d := DataCell{}

			if v.vartype == NUMERIC {
				d.value_numeric = ibmFloat64(tmp)
				d.value_char = fmt.Sprintf("%f", d.value_numeric)
			} else {
				d.value_char = strings.TrimSpace(string(tmp))
			}

			v.data = append(v.data, d)
		}
	}
}

// XPT files are always stored as Big-endian
// this function converts the IBM floating-point format to a float64
func ibmFloat64(b []byte) float64 {
	if len(b) != 8 {
		panic("IBM float must be 8 bytes")
	}

	// all zero = 0.0
	if b[0]|b[1]|b[2]|b[3]|b[4]|b[5]|b[6]|b[7] == 0 {
		return 0
	}

	sign := (b[0] & 0x80) != 0
	exponent := int(b[0]&0x7F) - 64

	// fraction is base-16
	var frac float64
	for i := 1; i < 8; i++ {
		frac += float64(b[i]) / math.Pow(256, float64(i))
	}

	val := frac * math.Pow(16, float64(exponent))
	if sign {
		val = -val
	}
	return val
}
