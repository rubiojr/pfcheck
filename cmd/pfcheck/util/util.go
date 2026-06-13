// Package util holds shared metadata and helpers for the pfcheck CLI.
package util

// AppName is the CLI binary name.
const AppName = "pfcheck"

// Version is overridden at build time via -ldflags "-X .../util.Version=...".
var Version = "dev"
