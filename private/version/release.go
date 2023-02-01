// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package version

import _ "unsafe" // needed for go:linkname

//go:linkname buildTimestamp storj.io/private/version.buildTimestamp
var buildTimestamp string = "1675284126"

//go:linkname buildCommitHash storj.io/private/version.buildCommitHash
var buildCommitHash string = "e751ee3c1bf510bd1acf09d5133853cc048bb993"

//go:linkname buildVersion storj.io/private/version.buildVersion
var buildVersion string = "v1.72.0-rc"

//go:linkname buildRelease storj.io/private/version.buildRelease
var buildRelease string = "true"

// ensure that linter understands that the variables are being used.
func init() { use(buildTimestamp, buildCommitHash, buildVersion, buildRelease) }

func use(...interface{}) {}
