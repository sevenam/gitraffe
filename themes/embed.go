// Package themes holds the colour themes that ship with gitraffe.
package themes

import "embed"

// FS is the bundled themes, compiled in: "go install" builds from the module
// cache and copies only the binary, so a themes/ folder on disk would exist
// for people who cloned the repository and nobody else.
//
//go:embed *.yml
var FS embed.FS
