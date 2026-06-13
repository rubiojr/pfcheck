// Package pfcheck wraps the privacy-filter.cpp PII/NER inference engine
// (https://github.com/localai-org/privacy-filter.cpp) to detect personally
// identifiable information (PII) in arbitrary text.
//
// It shells out to the upstream `pf-cli` binary in `--classify` mode, feeding
// text on stdin and parsing the JSON entity list it emits. The required GGUF
// model is downloaded on demand (see EnsureModel).
package pfcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// DefaultThreshold is the score threshold below which detected entities are
// discarded by pf-cli. 0.5 mirrors the upstream README example.
const DefaultThreshold = 0.5

// Entity is a single PII/NER span detected in the input text. It mirrors the
// JSON object emitted by `pf-cli --classify`.
type Entity struct {
	// EntityGroup is the entity category, e.g. "PERSON", "EMAIL", "PHONE".
	EntityGroup string `json:"entity_group"`
	// Start is the inclusive UTF-8 byte offset of the span in the input.
	Start int `json:"start"`
	// End is the exclusive UTF-8 byte offset of the span in the input.
	End int `json:"end"`
	// Score is the model confidence for the span, in [0, 1].
	Score float64 `json:"score"`
	// Text is the matched substring.
	Text string `json:"text"`
}

// Options configures a Checker.
type Options struct {
	// BinaryPath is the path to the `pf-cli` executable. When empty, the
	// binary is resolved via ResolveBinary (PATH, PFCHECK_PF_CLI, cache dir).
	BinaryPath string
	// ModelPath is the path to the GGUF model file. When empty, the model is
	// resolved/downloaded via EnsureModelSpec into the user cache directory.
	ModelPath string
	// ModelRepo overrides the HuggingFace repository to download the model
	// from. Empty uses the default (see DefaultModelSpec).
	ModelRepo string
	// ModelFile overrides the GGUF filename within ModelRepo. Empty uses the
	// default (see DefaultModelSpec).
	ModelFile string
	// Threshold is the minimum entity score to report. Defaults to
	// DefaultThreshold when zero.
	Threshold float64
	// Device selects the inference backend: "cpu" (default) or "vulkan".
	Device string
}

// Checker runs PII detection against the privacy-filter model.
type Checker struct {
	binaryPath string
	modelPath  string
	threshold  float64
	device     string
}

// New builds a Checker, resolving the pf-cli binary and the GGUF model.
//
// If opts.ModelPath is empty the model is downloaded on demand via
// EnsureModelSpec, which may take a while on first use. To control that
// behaviour (e.g. progress reporting) call EnsureModelSpec yourself and pass
// the resulting path in opts.ModelPath.
func New(ctx context.Context, opts Options) (*Checker, error) {
	bin := opts.BinaryPath
	if bin == "" {
		var err error
		bin, err = ResolveBinary()
		if err != nil {
			return nil, err
		}
	} else if _, err := exec.LookPath(bin); err != nil {
		return nil, fmt.Errorf("pf-cli binary %q not usable: %w", bin, err)
	}

	model := opts.ModelPath
	if model == "" {
		spec := DefaultModelSpec()
		if opts.ModelRepo != "" {
			spec.Repo = opts.ModelRepo
		}
		if opts.ModelFile != "" {
			spec.File = opts.ModelFile
		}
		var err error
		model, err = EnsureModelSpec(ctx, spec, "", nil)
		if err != nil {
			return nil, err
		}
	} else if err := validateModel(model); err != nil {
		return nil, err
	}

	threshold := opts.Threshold
	if threshold == 0 {
		threshold = DefaultThreshold
	}
	device := opts.Device
	if device == "" {
		device = "cpu"
	}

	return &Checker{
		binaryPath: bin,
		modelPath:  model,
		threshold:  threshold,
		device:     device,
	}, nil
}

// BinaryPath returns the resolved pf-cli path.
func (c *Checker) BinaryPath() string { return c.binaryPath }

// ModelPath returns the resolved GGUF model path.
func (c *Checker) ModelPath() string { return c.modelPath }

// Detect classifies text and returns every PII/NER entity found at or above
// the configured threshold.
func (c *Checker) Detect(ctx context.Context, text string) ([]Entity, error) {
	args := []string{
		"--classify",
		c.modelPath,
		fmt.Sprintf("%g", c.threshold),
	}
	if c.device != "" && c.device != "cpu" {
		args = append(args, c.device)
	}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	cmd.Stdin = strings.NewReader(text)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pf-cli classify failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return parseEntities(stdout.Bytes(), text)
}

// HasPII reports whether text contains any PII/NER entity at or above the
// configured threshold.
func (c *Checker) HasPII(ctx context.Context, text string) (bool, error) {
	ents, err := c.Detect(ctx, text)
	if err != nil {
		return false, err
	}
	return len(ents) > 0, nil
}

// entityRe extracts the structured fields of one entity object emitted by
// pf-cli, ignoring the trailing "text" field. The field order is fixed by
// pf-cli's printf format.
var entityRe = regexp.MustCompile(
	`"entity_group"\s*:\s*"([^"]*)"\s*,\s*"start"\s*:\s*(\d+)\s*,\s*"end"\s*:\s*(\d+)\s*,\s*"score"\s*:\s*([0-9eE.+-]+)`)

// parseEntities decodes the entity array emitted by pf-cli. pf-cli always
// prints a JSON array (possibly empty) on stdout, but it does NOT escape the
// "text" field, so spans containing quotes, backslashes or newlines (common in
// source code) produce invalid JSON. We therefore try strict JSON first and
// fall back to a lenient parse that rebuilds each text span from byte offsets
// in the original input.
func parseEntities(out []byte, input string) ([]Entity, error) {
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil, errors.New("pf-cli produced no output")
	}

	// Fast path: well-formed JSON (no special characters in any span).
	var ents []Entity
	if err := json.Unmarshal(out, &ents); err == nil {
		return ents, nil
	} else {
		jsonErr := err
		if recovered := parseEntitiesLenient(out, input); len(recovered) > 0 {
			return recovered, nil
		}
		return nil, fmt.Errorf("parsing pf-cli output: %w", jsonErr)
	}
}

// parseEntitiesLenient recovers entities from pf-cli output that is not valid
// JSON because the "text" field was emitted unescaped. The text of each span is
// reconstructed from the original input using the reported byte offsets.
func parseEntitiesLenient(out []byte, input string) []Entity {
	matches := entityRe.FindAllSubmatch(out, -1)
	ents := make([]Entity, 0, len(matches))
	for _, m := range matches {
		start, _ := strconv.Atoi(string(m[2]))
		end, _ := strconv.Atoi(string(m[3]))
		score, _ := strconv.ParseFloat(string(m[4]), 64)
		e := Entity{
			EntityGroup: string(m[1]),
			Start:       start,
			End:         end,
			Score:       score,
		}
		if start >= 0 && end <= len(input) && start <= end {
			e.Text = input[start:end]
		}
		ents = append(ents, e)
	}
	return ents
}
