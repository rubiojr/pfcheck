package pfcheck

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	// ModelRepo is the default HuggingFace repository hosting the GGUF model.
	ModelRepo = "LocalAI-io/privacy-filter-multilingual-GGUF"
	// ModelFile is the default GGUF filename within ModelRepo.
	ModelFile = "privacy-filter-multilingual-f16.gguf"
	// modelMinSize is a sanity lower bound (~1.3 GiB) used to detect a
	// truncated or partial download. The default model is ~2.6 GiB.
	modelMinSize = 1_300_000_000
)

// ModelSpec identifies a GGUF model to download: a HuggingFace repository and
// the GGUF filename within it. The model must use the openai-privacy-filter
// architecture for pfcheck/pf-cli to load it.
type ModelSpec struct {
	// Repo is the HuggingFace repo, e.g. "LocalAI-io/privacy-filter-multilingual-GGUF".
	Repo string
	// File is the GGUF filename within Repo.
	File string
}

// DefaultModelSpec returns the default model, applying the PFCHECK_MODEL_REPO
// and PFCHECK_MODEL_FILE environment overrides when set.
func DefaultModelSpec() ModelSpec {
	s := ModelSpec{Repo: ModelRepo, File: ModelFile}
	if v := os.Getenv("PFCHECK_MODEL_REPO"); v != "" {
		s.Repo = v
	}
	if v := os.Getenv("PFCHECK_MODEL_FILE"); v != "" {
		s.File = v
	}
	return s
}

// URL is the direct download URL for the model.
func (s ModelSpec) URL() string {
	return fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", s.Repo, s.File)
}

// CachePath returns the path where this model is stored in the pfcheck cache.
// Distinct filenames let multiple models coexist.
func (s ModelSpec) CachePath() (string, error) {
	dir, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, s.File), nil
}

// ModelURL is the direct download URL for the default model.
func ModelURL() string {
	return DefaultModelSpec().URL()
}

// CacheDir returns the directory pfcheck uses to store the downloaded model
// and (optionally) the pf-cli binary. It honours PFCHECK_CACHE_DIR, falling
// back to <user-cache-dir>/pfcheck.
func CacheDir() (string, error) {
	if dir := os.Getenv("PFCHECK_CACHE_DIR"); dir != "" {
		return dir, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolving user cache dir: %w", err)
	}
	return filepath.Join(base, "pfcheck"), nil
}

// DefaultModelPath returns the path where the default model is stored.
func DefaultModelPath() (string, error) {
	return DefaultModelSpec().CachePath()
}

// ProgressFunc reports download progress. total is -1 when the content length
// is unknown.
type ProgressFunc func(downloaded, total int64)

// EnsureModel returns the path to a usable copy of the default model,
// downloading it on first use. See EnsureModelSpec.
func EnsureModel(ctx context.Context, dest string, progress ProgressFunc) (string, error) {
	return EnsureModelSpec(ctx, DefaultModelSpec(), dest, progress)
}

// EnsureModelSpec returns the path to a usable copy of spec, downloading it on
// first use. When dest is empty, the destination is taken from PFCHECK_MODEL or
// else spec.CachePath(). If a valid model already exists at the destination it
// is returned without re-downloading. progress may be nil.
func EnsureModelSpec(ctx context.Context, spec ModelSpec, dest string, progress ProgressFunc) (string, error) {
	if dest == "" {
		// Allow overriding via env for both library and CLI callers.
		if env := os.Getenv("PFCHECK_MODEL"); env != "" {
			dest = env
		} else {
			var err error
			dest, err = spec.CachePath()
			if err != nil {
				return "", err
			}
		}
	}

	if err := validateModel(dest); err == nil {
		return dest, nil
	}

	if err := downloadModel(ctx, spec.URL(), dest, progress); err != nil {
		return "", err
	}
	if err := validateModel(dest); err != nil {
		return "", fmt.Errorf("model invalid after download: %w", err)
	}
	return dest, nil
}

// validateModel checks that path looks like a complete GGUF model.
func validateModel(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("model not found at %q: %w", path, err)
	}
	if fi.IsDir() {
		return fmt.Errorf("model path %q is a directory", path)
	}
	if fi.Size() < modelMinSize {
		return fmt.Errorf("model at %q looks truncated (%d bytes)", path, fi.Size())
	}
	return nil
}

func downloadModel(ctx context.Context, url, dest string, progress ProgressFunc) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if tok := os.Getenv("HF_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading model: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading model: unexpected status %s", resp.Status)
	}

	// Download to a temp file alongside dest, then atomically rename.
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".pfcheck-model-*.part")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	var reader io.Reader = resp.Body
	if progress != nil {
		reader = &progressReader{r: resp.Body, total: resp.ContentLength, fn: progress}
	}

	if _, err := io.Copy(tmp, reader); err != nil {
		tmp.Close()
		return fmt.Errorf("writing model: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing model file: %w", err)
	}

	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("finalising model: %w", err)
	}
	return nil
}

type progressReader struct {
	r          io.Reader
	total      int64
	downloaded int64
	fn         ProgressFunc
	lastReport time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.downloaded += int64(n)
	// Throttle reports to ~10/s to avoid spamming callers.
	if now := time.Now(); now.Sub(p.lastReport) > 100*time.Millisecond || err == io.EOF {
		p.lastReport = now
		p.fn(p.downloaded, p.total)
	}
	return n, err
}
