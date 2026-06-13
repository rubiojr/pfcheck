//go:build embed_pfcli

// Package embedbin optionally embeds the privacy-filter.cpp `pf-cli` binary
// into the pfcheck executable. The binary is only embedded when building with
// the `embed_pfcli` build tag (see the Makefile), so plain `go build` and
// `go test` do not require the binary to be present.
package embedbin

import _ "embed"

// Binary holds the embedded pf-cli executable.
//
//go:embed pf-cli
var Binary []byte

// Embedded reports whether a binary was embedded at build time.
const Embedded = true
