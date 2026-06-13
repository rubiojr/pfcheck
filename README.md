# pfcheck

`pfcheck` is a Go CLI **and** library that tells you whether a piece of text
contains personally identifiable information (PII). It wraps
[privacy-filter.cpp](https://github.com/localai-org/privacy-filter.cpp), a
minimal C++/GGML runtime for OpenAI's privacy-filter NER model, and parses its
entity output.

- **One-shot PII check** from stdin, a file, or arguments.
- **Auto-downloads** the required GGUF model on first use.
- **Single self-contained binary**: the `pf-cli` inference engine is embedded
  into the Go binary, so the only runtime requirement is the model download.
- **Docker-based build** of the `pf-cli` inference binary — no C++ toolchain
  needed on the host.
- Usable as a **library** (`github.com/rubiojr/pfcheck`).

## Quick start

```sh
# 1. Build pf-cli AND embed it into the pfcheck binary in one step.
#    Linux: builds pf-cli in Docker. macOS: builds it natively (needs cmake +
#    Xcode CLT). See "Building pf-cli" for details.
make build

# 2. Check some text (the model downloads automatically on first run).
echo "Contact John Doe at jdoe@example.com" | ./build/pfcheck
```

`make build` produces a single self-contained executable for the host OS —
`pf-cli` is embedded via `go:embed`, so the only thing it fetches at runtime is
the model.

Example output:

```
PII detected: 2 entities
  PERSON         "John Doe"                                 score=0.99 [8:16]
  EMAIL          "jdoe@example.com"                         score=0.94 [20:36]
```

The process exit code is scripting-friendly:

| Exit code | Meaning            |
|-----------|--------------------|
| `0`       | No PII detected    |
| `1`       | PII detected       |
| `2`       | Error              |

```sh
if echo "$TEXT" | pfcheck --quiet; then
  echo "clean"
else
  echo "contains PII"
fi
```

## CLI

```
pfcheck [options] [text...]            # root command == check
pfcheck check [options] [text...]
pfcheck download-model [-o PATH] [--force]
```

The root command is the `check` command, so `echo ... | pfcheck`,
`pfcheck "some text"` and `pfcheck --quiet` all work without naming `check`.

### `check` (default command)

| Flag | Description |
|------|-------------|
| `-f, --file FILE`    | Read input from a file instead of stdin |
| `-d, --dir DIR`      | Scan all common text files under `DIR` recursively |
| `-j, --json`         | Emit detected entities as JSON |
| `-q, --quiet`        | No output; communicate via exit code only |
| `-t, --threshold F`  | Minimum entity score to report (default `0.5`) |
| `--device cpu\|vulkan` | Inference backend (default `cpu`) |
| `--model PATH`       | GGUF model path (`$PFCHECK_MODEL`) |
| `--model-repo REPO`  | HuggingFace repo to download from (`$PFCHECK_MODEL_REPO`) |
| `--model-file FILE`  | GGUF filename within the repo (`$PFCHECK_MODEL_FILE`) |
| `--binary PATH`      | `pf-cli` path (`$PFCHECK_PF_CLI`) |

Examples:

```sh
pfcheck "My SSN is 123-45-6789"
pfcheck --json -f notes.txt
cat email.txt | pfcheck --threshold 0.8
```

### Scanning a directory

`--dir` recursively scans every common text file (by extension) under a
directory, loading the model once and reusing it for all files. Hidden
directories (e.g. `.git`) and empty files are skipped.

```sh
pfcheck --dir ./docs
pfcheck --dir ./docs --json     # [{ "file": ..., "entities": [...] }, ...]
pfcheck --dir ./docs --quiet    # exit 1 if any file contains PII
```

Recognised files cover prose (`.txt .md .rst .adoc .org .tex` …), config/data
(`.json .yaml .toml .ini .xml .csv` …), and **source code for ~100 languages**
(`.go .py .js .ts .java .c .cpp .cs .rb .php .rs .swift .kt .sql` …), plus
well-known extensionless build files (`Makefile`, `Dockerfile`, …).

### Choosing a model

By default pfcheck uses the multilingual GGUF
[`LocalAI-io/privacy-filter-multilingual-GGUF`](https://huggingface.co/LocalAI-io/privacy-filter-multilingual-GGUF).
Any GGUF using the `openai-privacy-filter` architecture works — point pfcheck at
a different one with `--model-repo`/`--model-file` (or the matching env vars).
Each model is cached under its own filename, so several can coexist.

```sh
# Use an alternative compatible GGUF.
pfcheck --model-repo someorg/privacy-filter-en-GGUF \
        --model-file privacy-filter-en-f16.gguf "some text"

# Or via environment.
export PFCHECK_MODEL_REPO=someorg/privacy-filter-en-GGUF
export PFCHECK_MODEL_FILE=privacy-filter-en-f16.gguf
```

> Note: detection quality is largely a property of the model architecture, which
> keys heavily on capitalization (e.g. `Sergio` is detected, `sergio` is not). A
> different checkpoint won't change that behavior.

### `download-model`

Pre-fetches the model into the cache (or `-o PATH`). Honors `--model-repo` /
`--model-file` and `HF_TOKEN` for authenticated downloads.

```sh
pfcheck download-model
pfcheck download-model --model-repo someorg/privacy-filter-en-GGUF \
                       --model-file privacy-filter-en-f16.gguf
```

## Library usage

```go
package main

import (
	"context"
	"fmt"

	"github.com/rubiojr/pfcheck"
)

func main() {
	ctx := context.Background()

	// Resolves pf-cli and downloads the model if needed.
	checker, err := pfcheck.New(ctx, pfcheck.Options{})
	if err != nil {
		panic(err)
	}

	has, err := checker.HasPII(ctx, "Email me at jane@example.com")
	if err != nil {
		panic(err)
	}
	fmt.Println("contains PII:", has)

	ents, _ := checker.Detect(ctx, "Call John at +1 555 123 4567")
	for _, e := range ents {
		fmt.Printf("%s: %q (%.2f)\n", e.EntityGroup, e.Text, e.Score)
	}
}
```

Select a different model via `Options`:

```go
checker, err := pfcheck.New(ctx, pfcheck.Options{
	ModelRepo: "someorg/privacy-filter-en-GGUF",
	ModelFile: "privacy-filter-en-f16.gguf",
})
```

For full control over downloading (e.g. progress), call `EnsureModelSpec`
yourself and pass the path as `Options.ModelPath`.

## How it works

`pfcheck` shells out to `pf-cli --classify <model.gguf> <threshold>`, feeding
text on stdin. `pf-cli` prints a JSON array of entities with exact UTF-8 byte
offsets, which `pfcheck` decodes into `[]pfcheck.Entity`. Any entity at or above
the threshold means PII is present.

### Resolution order

`pf-cli` binary:
1. `$PFCHECK_PF_CLI`
2. Embedded binary (when built with `make build`), extracted to `<cache>/bin/pf-cli-<hash>`
3. `<cache>/bin/pf-cli` (where `scripts/build-pf.sh` installs it)
4. `$PATH`

GGUF model:
1. `--model` / `$PFCHECK_MODEL` (explicit path)
2. `<cache>/<model-file>` where the file is set by `--model-repo`/`--model-file`
   (or `$PFCHECK_MODEL_REPO`/`$PFCHECK_MODEL_FILE`), auto-downloaded if missing.
   Defaults to `privacy-filter-multilingual-f16.gguf`.

The cache directory is `$PFCHECK_CACHE_DIR`, or `<user-cache-dir>/pfcheck`.

## Building pf-cli

`make build` builds the upstream `pf-cli`, stages it at
`internal/embedbin/pf-cli`, and embeds it into the Go binary
(`-tags embed_pfcli`). In both build paths `ggml` is statically linked and
`-march=native` is disabled so the binary is portable.

There are two build paths, selected automatically by OS (override with
`PF_BUILD=native|docker`):

| `PF_BUILD` | Script | Requirements | Produces |
|------------|--------|--------------|----------|
| `docker` (default on Linux) | `scripts/build-pf.sh` | Docker | a **Linux** binary |
| `native` (default on macOS) | `scripts/build-pf-native.sh` | `git`, `cmake`, a C++17 compiler | a **host-native** binary (macOS Mach-O, Linux ELF) |

```sh
make build                 # auto: native on macOS, docker on Linux
make build PF_BUILD=native # force the host CMake build (no Docker)
```

On **macOS**, install the toolchain first:

```sh
xcode-select --install     # C++ compiler
brew install cmake
make build                 # uses scripts/build-pf-native.sh automatically
```

To build pfcheck *without* embedding (resolve `pf-cli` from `$PATH`/cache at
runtime):

```sh
make build-noembed
```

Override the upstream source ref with `PF_REF`, or install pf-cli standalone:

```sh
PF_REF=master make pf-cli            # force-rebuild the embedded pf-cli
./scripts/build-pf-native.sh /usr/local/bin   # native install elsewhere
./scripts/build-pf.sh /usr/local/bin          # Linux/Docker install elsewhere
```

Both standalone installs default to `<user-cache-dir>/pfcheck/bin` (matching
`os.UserCacheDir()`), where pfcheck finds the binary automatically. The Docker
path also leaves a runnable container image (`pfcheck/pf-cli:<ref>`).

## Development

```sh
make test    # go test ./...
make fmt     # go fmt ./...
make vet     # go vet ./...
make lint    # staticcheck ./...
```

## License

MIT.
