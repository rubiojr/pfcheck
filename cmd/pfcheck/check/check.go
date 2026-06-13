// Package check implements the `pfcheck check` command: read text, report PII.
package check

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rubiojr/pfcheck"
	"github.com/rubiojr/pfcheck/cmd/pfcheck/util"
	"github.com/urfave/cli/v3"
)

// textExtensions is the set of file extensions treated as plain-text input by
// --dir. Matched case-insensitively. It covers prose, config/data formats, and
// source code for the most common programming languages.
var textExtensions = newExtSet(
	// Prose / documentation.
	".txt", ".text", ".md", ".markdown", ".mdown", ".mkd", ".mdx",
	".rst", ".adoc", ".asciidoc", ".org", ".tex", ".bib", ".rtf",
	".rmd", ".qmd", ".log", ".me", ".1", ".man", ".pod",

	// Config / data / markup.
	".csv", ".tsv", ".json", ".jsonc", ".json5", ".ndjson", ".geojson",
	".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".config",
	".properties", ".env", ".editorconfig", ".xml", ".xsd", ".xsl",
	".xslt", ".dtd", ".plist", ".svg", ".html", ".htm", ".xhtml",
	".css", ".scss", ".sass", ".less", ".styl", ".proto", ".graphql",
	".gql", ".hcl", ".tf", ".tfvars", ".nix", ".bzl", ".bazel",
	".cmake", ".mk", ".mak", ".make", ".gradle", ".sbt", ".ipynb",

	// Templates.
	".vue", ".svelte", ".astro", ".ejs", ".hbs", ".handlebars",
	".mustache", ".liquid", ".twig", ".njk", ".jinja", ".jinja2",
	".j2", ".pug", ".jade", ".haml", ".slim", ".erb", ".jsp", ".asp",
	".aspx", ".cshtml", ".razor", ".phtml",

	// Shell / scripting.
	".sh", ".bash", ".zsh", ".fish", ".ksh", ".csh", ".tcsh",
	".ps1", ".psm1", ".psd1", ".bat", ".cmd", ".awk", ".sed", ".vim",

	// Source code (top languages).
	".go", ".py", ".pyw", ".pyi", ".pyx", ".rpy",
	".js", ".mjs", ".cjs", ".jsx", ".ts", ".tsx", ".mts", ".cts",
	".coffee", ".ls", ".java", ".kt", ".kts", ".scala", ".sc",
	".groovy", ".clj", ".cljs", ".cljc", ".edn",
	".c", ".h", ".cpp", ".cc", ".cxx", ".c++", ".hpp", ".hh", ".hxx",
	".h++", ".cs", ".fs", ".fsi", ".fsx", ".vb", ".vbs",
	".rb", ".rake", ".gemspec", ".ru", ".php", ".php3", ".php4",
	".php5", ".rs", ".swift", ".m", ".mm", ".pl", ".pm", ".t", ".pod",
	".r", ".jl", ".lua", ".dart", ".ex", ".exs", ".erl", ".hrl",
	".hs", ".lhs", ".ml", ".mli", ".ocaml", ".elm", ".purs", ".res",
	".resi", ".nim", ".nims", ".cr", ".zig", ".v", ".sv", ".svh",
	".vhd", ".vhdl", ".d", ".pas", ".pp", ".adb", ".ads", ".f",
	".for", ".f90", ".f95", ".f03", ".f08", ".cob", ".cbl", ".lisp",
	".lsp", ".cl", ".el", ".scm", ".ss", ".rkt", ".tcl", ".asm",
	".s", ".sql", ".sol", ".glsl", ".vert", ".frag", ".comp", ".hlsl",
	".wgsl", ".metal", ".cu", ".cuh", ".sml", ".idr", ".agda",
	".vala", ".hx", ".haxe", ".gd", ".ahk", ".as", ".abap", ".cls",
	".trigger", ".apex", ".st", ".prolog", ".pro", ".p", ".sas",
	".do", ".q", ".qs", ".wl", ".wls", ".tla", ".thy", ".lean",
	".raku", ".rakumod", ".moon", ".wat", ".jq", ".nu", ".rego",
	".cue", ".smithy", ".thrift", ".capnp", ".fbs", ".ipy",
)

func newExtSet(exts ...string) map[string]bool {
	m := make(map[string]bool, len(exts))
	for _, e := range exts {
		m[e] = true
	}
	return m
}

// textFilenames are well-known extensionless text/source files (matched by
// exact base name).
var textFilenames = newExtSet(
	"Makefile", "GNUmakefile", "makefile", "Dockerfile", "Containerfile",
	"Rakefile", "Gemfile", "Guardfile", "Procfile", "Vagrantfile",
	"Brewfile", "Jenkinsfile", "Justfile", "Caddyfile", "Fastfile",
	"Berksfile", "Thorfile", "Capfile", "BUILD", "WORKSPACE", "Earthfile",
)

// Command detects PII in text from stdin, a file, or arguments. The same flags
// and action are reused as the root command in main, so `pfcheck <text>` and
// `pfcheck check <text>` behave identically.
var Command = &cli.Command{
	Name:      "check",
	Usage:     "Detect PII in text from stdin, a file, or arguments",
	ArgsUsage: "[text...]",
	Flags:     Flags(),
	Action:    Run,
}

// Flags returns the flag set shared by the `check` subcommand and the root
// command.
func Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "file",
			Aliases: []string{"f"},
			Usage:   "Read input from `FILE` instead of stdin",
		},
		&cli.StringFlag{
			Name:    "dir",
			Aliases: []string{"d"},
			Usage:   "Scan all common text files under `DIR` recursively",
		},
		&cli.BoolFlag{
			Name:    "json",
			Aliases: []string{"j"},
			Usage:   "Emit detected entities as JSON",
		},
		&cli.BoolFlag{
			Name:    "quiet",
			Aliases: []string{"q"},
			Usage:   "Print nothing; communicate only via exit code",
		},
		&cli.FloatFlag{
			Name:    "threshold",
			Aliases: []string{"t"},
			Value:   pfcheck.DefaultThreshold,
			Usage:   "Minimum entity score to report",
		},
		&cli.StringFlag{
			Name:  "device",
			Value: "cpu",
			Usage: "Inference backend: cpu or vulkan",
		},
		&cli.StringFlag{
			Name:    "model",
			Usage:   "Path to GGUF model (downloaded automatically if unset)",
			Sources: cli.EnvVars("PFCHECK_MODEL"),
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
		&cli.StringFlag{
			Name:    "binary",
			Usage:   "Path to the pf-cli binary",
			Sources: cli.EnvVars("PFCHECK_PF_CLI"),
		},
	}
}

// fileResult holds the entities detected in one input.
type fileResult struct {
	File     string           `json:"file"`
	Entities []pfcheck.Entity `json:"entities"`
}

// input is a single unit of text to classify. name is empty for stdin/argument
// input and set to the path for file/dir input.
type input struct {
	name string
	text string
}

// Run is the check action. Exit codes: 0 = no PII, 1 = PII found, 2 = error.
// cli.Exit is used so the process exit status is meaningful for scripting.
func Run(ctx context.Context, cmd *cli.Command) error {
	quiet := cmd.Bool("quiet")
	multi := cmd.String("dir") != ""

	inputs, err := gatherInputs(cmd)
	if err != nil {
		return cli.Exit(fmt.Sprintf("pfcheck: %v", err), 2)
	}
	if len(inputs) == 0 {
		return cli.Exit("pfcheck: no input text provided", 2)
	}
	// For a single interactive input, reject empty text outright.
	if !multi && strings.TrimSpace(inputs[0].text) == "" {
		return cli.Exit("pfcheck: no input text provided", 2)
	}

	// Resolve (and, if needed, download) the model up front so we can show
	// download progress. New() would otherwise download silently.
	modelPath, err := resolveModel(ctx, cmd, quiet)
	if err != nil {
		return cli.Exit(fmt.Sprintf("pfcheck: %v", err), 2)
	}

	checker, err := pfcheck.New(ctx, pfcheck.Options{
		BinaryPath: cmd.String("binary"),
		ModelPath:  modelPath,
		Threshold:  cmd.Float("threshold"),
		Device:     cmd.String("device"),
	})
	if err != nil {
		return cli.Exit(fmt.Sprintf("pfcheck: %v", err), 2)
	}

	// Loading the model and running inference takes a while with no output
	// from pf-cli, so show a spinner while we wait. The model is loaded once
	// and reused across every file.
	var sp *util.Spinner
	if !quiet {
		sp = util.NewSpinner("Loading model and analyzing...")
		sp.Start()
	}

	results := make([]fileResult, 0, len(inputs))
	for i, in := range inputs {
		if sp != nil && in.name != "" {
			sp.SetMessage(fmt.Sprintf("Analyzing (%d/%d) %s", i+1, len(inputs), in.name))
		}
		ents, derr := checker.Detect(ctx, in.text)
		if derr != nil {
			if sp != nil {
				sp.Stop()
			}
			return cli.Exit(fmt.Sprintf("pfcheck: %s: %v", inputLabel(in), derr), 2)
		}
		results = append(results, fileResult{File: in.name, Entities: ents})
	}
	if sp != nil {
		sp.Stop()
	}

	if !quiet {
		if err := printResults(results, multi, cmd.Bool("json")); err != nil {
			return cli.Exit(fmt.Sprintf("pfcheck: %v", err), 2)
		}
	}

	for _, r := range results {
		if len(r.Entities) > 0 {
			return cli.Exit("", 1)
		}
	}
	return nil
}

func inputLabel(in input) string {
	if in.name != "" {
		return in.name
	}
	return "input"
}

// printResults renders detection results. Single inputs keep the original
// compact output; multi-file (--dir) scans print per-file results.
func printResults(results []fileResult, multi, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if multi {
			return enc.Encode(results)
		}
		return enc.Encode(results[0].Entities)
	}
	if multi {
		printHumanMulti(results)
		return nil
	}
	printHuman(results[0].Entities)
	return nil
}

func printHuman(ents []pfcheck.Entity) {
	if len(ents) == 0 {
		fmt.Println("No PII detected.")
		return
	}
	fmt.Printf("PII detected: %d entit%s\n", len(ents), plural(len(ents)))
	for _, e := range ents {
		fmt.Printf("  %-14s %-40q score=%.2f [%d:%d]\n",
			e.EntityGroup, e.Text, e.Score, e.Start, e.End)
	}
}

func printHumanMulti(results []fileResult) {
	withPII := 0
	for _, r := range results {
		if len(r.Entities) == 0 {
			fmt.Printf("%s: clean\n", r.File)
			continue
		}
		withPII++
		fmt.Printf("%s: PII detected (%d entit%s)\n", r.File, len(r.Entities), plural(len(r.Entities)))
		for _, e := range r.Entities {
			fmt.Printf("    %-14s %-40q score=%.2f [%d:%d]\n",
				e.EntityGroup, e.Text, e.Score, e.Start, e.End)
		}
	}
	fmt.Printf("\nScanned %d file%s, %d with PII.\n",
		len(results), plural2(len(results)), withPII)
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func plural2(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// resolveModel returns the model path, downloading it with a progress bar when
// it is missing. When --model is set, that path is used verbatim (validated by
// pfcheck.New later); otherwise the model identified by --model-repo/--model-file
// is fetched into the cache.
func resolveModel(ctx context.Context, cmd *cli.Command, quiet bool) (string, error) {
	// An explicit --model / $PFCHECK_MODEL path is handled by pfcheck.New.
	if cmd.String("model") != "" {
		return cmd.String("model"), nil
	}

	spec := pfcheck.ModelSpec{
		Repo: cmd.String("model-repo"),
		File: cmd.String("model-file"),
	}

	// EnsureModelSpec only invokes the progress callback while actually
	// downloading, so the header/bar stay hidden when the model is cached.
	var progress pfcheck.ProgressFunc
	var bar *util.DownloadBar
	if !quiet {
		bar = util.NewDownloadBar(
			fmt.Sprintf("Downloading model %s (first run)...", spec.File))
		progress = bar.Update
	}

	path, err := pfcheck.EnsureModelSpec(ctx, spec, "", progress)
	if bar != nil {
		bar.Finish()
	}
	return path, err
}

// gatherInputs collects the text(s) to classify from --dir, --file, arguments,
// or stdin, in that order of precedence.
func gatherInputs(cmd *cli.Command) ([]input, error) {
	if dir := cmd.String("dir"); dir != "" {
		return gatherDir(dir)
	}

	if f := cmd.String("file"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f, err)
		}
		return []input{{name: f, text: string(b)}}, nil
	}

	if cmd.Args().Len() > 0 {
		return []input{{text: strings.Join(cmd.Args().Slice(), " ")}}, nil
	}

	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	return []input{{text: string(b)}}, nil
}

// gatherDir walks dir recursively, collecting non-empty plain-text files whose
// extension is in textExtensions. Hidden directories (e.g. .git) are skipped.
func gatherDir(dir string) ([]input, error) {
	var inputs []input
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !isTextFile(d.Name()) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		if strings.TrimSpace(string(b)) == "" {
			return nil
		}
		inputs = append(inputs, input{name: path, text: string(b)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("no text files found in %s", dir)
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].name < inputs[j].name })
	return inputs, nil
}

func isTextFile(name string) bool {
	if textFilenames[name] {
		return true
	}
	return textExtensions[strings.ToLower(filepath.Ext(name))]
}
