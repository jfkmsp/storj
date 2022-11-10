// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package dbx

import (
	"github.com/zeebo/errs"
)

//go:generate sh gen.sh

func init() {
	// catch dbx errors
	class := errs.Class("multinodedb dbx")
	WrapErr = func(e *Error) error {
		if e.Code == ErrorCode_NoRows {
			return e.Err
		}
		return class.Wrap(e)
	}
}
