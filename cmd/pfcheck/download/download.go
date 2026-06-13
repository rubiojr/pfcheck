// Package download implements the `pfcheck download-model` command.
package download

import (
	"context"
	"fmt"
	"os"

	"github.com/rubiojr/pfcheck"
	"github.com/rubiojr/pfcheck/cmd/pfcheck/util"
	"github.com/urfave/cli/v3"
)

// Command downloads the GGUF model into the pfcheck cache.
var Command = &cli.Command{
	Name:  "download-model",
	Usage: "Download the privacy-filter GGUF model into the cache",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Usage:   "Destination path (defaults to the pfcheck cache)",
		},
		&cli.BoolFlag{
			Name:  "force",
			Usage: "Re-download even if a valid model already exists",
		},
		&cli.StringFlag{
			Name:    "model-repo",
			Value:   pfcheck.ModelRepo,
			Usage:   "HuggingFace repo to download the model from",
			Sources: cli.EnvVars("PFCHECK_MODEL_REPO"),
		},
		&cli.StringFlag{
			Name:    "model-file",
			Value:   pfcheck.ModelFile,
			Usage:   "GGUF filename within the model repo",
			Sources: cli.EnvVars("PFCHECK_MODEL_FILE"),
		},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		spec := pfcheck.ModelSpec{
			Repo: cmd.String("model-repo"),
			File: cmd.String("model-file"),
		}

		dest := cmd.String("output")
		if dest == "" {
			var err error
			dest, err = spec.CachePath()
			if err != nil {
				return cli.Exit(fmt.Sprintf("pfcheck: %v", err), 2)
			}
		}

		if cmd.Bool("force") {
			_ = os.Remove(dest)
		}

		fmt.Fprintf(os.Stderr, "Downloading %s\n  from %s\n  to   %s\n",
			spec.File, spec.URL(), dest)

		bar := util.NewDownloadBar("")
		path, err := pfcheck.EnsureModelSpec(ctx, spec, dest, bar.Update)
		bar.Finish()
		if err != nil {
			return cli.Exit(fmt.Sprintf("pfcheck: %v", err), 2)
		}
		fmt.Fprintf(os.Stderr, "Model ready: %s\n", path)
		return nil
	},
}
