// Copyright 2026 Jan van der Linde
// SPDX-License-Identifier: Apache-2.0

// Package goxpt reads SAS Transport (XPORT / .xpt) version 5 and 6 files.
//
// The on-disk layout follows the SAS technical paper "Record Layout of a SAS
// Version 5 or 6 Data Set in SAS Transport (XPORT) Format":
// https://support.sas.com/content/dam/SAS/support/en/technical-papers/record-layout-of-a-sas-version-5-or-6-data-set-in-sas-transport-xport-format.pdf
package goxpt

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// VariableType distinguishes SAS numeric and character variables.
type VariableType int

const (
	// TypeNumeric is a SAS numeric variable, stored as an IBM hexadecimal float.
	TypeNumeric VariableType = iota
	// TypeCharacter is a SAS character variable, stored as blank-padded text.
	TypeCharacter
)

func (t VariableType) String() string {
	switch t {
	case TypeNumeric:
		return "numeric"
	case TypeCharacter:
		return "character"
	default:
		return fmt.Sprintf("VariableType(%d)", int(t))
	}
}

// LibraryInfo holds the metadata from the XPT library header.
type LibraryInfo struct {
	SASVersion string
	OS         string
	Created    time.Time
	Modified   time.Time
}

// MemberInfo holds the metadata describing a single dataset (member).
type MemberInfo struct {
	Name       string
	Label      string
	Type       string
	SASVersion string
	OS         string
	Created    time.Time
	Modified   time.Time
}

// Variable describes one column of a dataset together with its decoded values.
type Variable struct {
	Num            int
	Name           string
	Label          string
	Length         int // byte width in an observation (1-8 for numeric)
	Type           VariableType
	Format         string
	FormatLength   int
	FormatDecimals int
	Informat       string
	Data           []DataCell
}

// StringAt renders the value in row as text, or "" if row is out of range.
func (v Variable) StringAt(row int) string {
	if row < 0 || row >= len(v.Data) {
		return ""
	}
	return v.Data[row].Value(v.Type)
}

// DataCell is a single value. For a numeric variable read Numeric (after
// checking Missing); for a character variable read Char.
type DataCell struct {
	Numeric     float64
	Char        string
	Missing     bool // numeric SAS missing value
	MissingCode byte // 0 for an ordinary '.', otherwise '_' or 'A'-'Z'
}

// Value renders the cell as text, given the type of its variable.
func (c DataCell) Value(t VariableType) string {
	if t == TypeCharacter {
		return c.Char
	}
	if c.Missing {
		if c.MissingCode == 0 || c.MissingCode == '.' {
			return "."
		}
		return "." + strings.ToUpper(string(c.MissingCode))
	}
	return strconv.FormatFloat(c.Numeric, 'g', -1, 64)
}

// Dataset is one member (dataset) parsed from an XPT file.
type Dataset struct {
	Library   LibraryInfo
	Member    MemberInfo
	Variables []Variable

	descriptorSize int // 136 (VAX/VMS) or 140 bytes per NAMESTR record
	numVars        int // variable count declared in the NAMESTR header
	rowSize        int // bytes per observation
}

// NumRows returns the number of observations in the dataset.
func (ds *Dataset) NumRows() int {
	if len(ds.Variables) == 0 {
		return 0
	}
	n := len(ds.Variables[0].Data)
	for _, v := range ds.Variables[1:] {
		if len(v.Data) < n {
			n = len(v.Data)
		}
	}
	return n
}

// AsSimpleGrid returns the dataset as a 2D string grid: row 0 holds the
// variable names and each following row holds one observation.
func (ds *Dataset) AsSimpleGrid() [][]string {
	rows := ds.NumRows()
	grid := make([][]string, 0, rows+1)
	if len(ds.Variables) == 0 {
		return grid
	}

	header := make([]string, len(ds.Variables))
	for i, v := range ds.Variables {
		header[i] = v.Name
	}
	grid = append(grid, header)

	for r := 0; r < rows; r++ {
		row := make([]string, len(ds.Variables))
		for c := range ds.Variables {
			row[c] = ds.Variables[c].StringAt(r)
		}
		grid = append(grid, row)
	}
	return grid
}
