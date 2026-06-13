//go:build !embed_pfcli

// Package embedbin optionally embeds the privacy-filter.cpp `pf-cli` binary
// into the pfcheck executable. This file is the default (no embedding); the
// embedded variant is compiled only with the `embed_pfcli` build tag.
package embedbin

// Binary holds the embedded pf-cli executable, empty in non-embedded builds.
var Binary []byte

// Embedded reports whether a binary was embedded at build time.
const Embedded = false
