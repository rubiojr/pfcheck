// Command pfcheck detects personally identifiable information (PII) in text
// using the privacy-filter.cpp inference engine.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rubiojr/pfcheck/cmd/pfcheck/check"
	"github.com/rubiojr/pfcheck/cmd/pfcheck/download"
	"github.com/rubiojr/pfcheck/cmd/pfcheck/util"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:      util.AppName,
		Usage:     "Detect PII in text using privacy-filter.cpp",
		Version:   util.Version,
		ArgsUsage: "[text...]",
		// The root command behaves as `check`, so `echo ... | pfcheck`,
		// `pfcheck "some text"` and `pfcheck --quiet` all work. Explicit
		// subcommands (check, download-model) still take precedence.
		Flags:  check.Flags(),
		Action: check.Run,
		Commands: []*cli.Command{
			check.Command,
			download.Command,
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
