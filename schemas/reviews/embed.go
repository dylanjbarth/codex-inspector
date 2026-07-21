// Package reviewschemas embeds the frozen Review contracts in the Inspector binary.
package reviewschemas

import "embed"

// Files contains the canonical manifest, run, and report JSON schemas.
//
//go:embed *.schema.json
var Files embed.FS
