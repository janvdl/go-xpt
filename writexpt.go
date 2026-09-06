// Copyright 2026 Jan van der Linde
// SPDX-License-Identifier: Apache-2.0

package goxpt

import (
	"errors"
	"io"
)

// errNotImplemented is returned by the write API until it is finished.
var errNotImplemented = errors.New("goxpt: writing XPT files is not implemented yet")

// WriteXPT serialises ds to w in XPORT version 5 format.
//
// Not implemented yet: it currently always returns an error. The signature is
// stable so callers can wire it up ahead of time.
func WriteXPT(w io.Writer, ds *Dataset) error {
	_ = w
	_ = ds
	return errNotImplemented
}
