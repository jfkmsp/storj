// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

// Package piecedeletion implements service for deleting pieces that combines concurrent requests.
package piecedeletion

import (
	"github.com/zeebo/errs"
)

// Error is the default error class for piece deletion.
var Error = errs.Class("piece deletion")
