package pfcheck

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rubiojr/pfcheck/internal/embedbin"
)

// HasEmbeddedBinary reports whether a pf-cli binary was embedded into this
// build (via the `embed_pfcli` build tag).
func HasEmbeddedBinary() bool {
	return embedbin.Embedded && len(embedbin.Binary) > 0
}

// extractEmbeddedBinary materialises the embedded pf-cli into the cache and
// returns its path. The filename includes a content hash so a rebuilt/updated
// binary is written to a fresh path, and extraction is skipped when an
// identical copy already exists.
func extractEmbeddedBinary() (string, error) {
	if !HasEmbeddedBinary() {
		return "", errors.New("no embedded pf-cli binary in this build")
	}

	sum := sha256.Sum256(embedbin.Binary)
	name := BinaryName + "-" + hex.EncodeToString(sum[:])[:12]

	dir, err := CacheDir()
	if err != nil {
		return "", err
	}
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", fmt.Errorf("creating cache bin dir: %w", err)
	}
	path := filepath.Join(binDir, name)

	// Reuse an existing identical extraction.
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() &&
		fi.Size() == int64(len(embedbin.Binary)) && fi.Mode()&0o111 != 0 {
		return path, nil
	}

	// Atomic write: temp file in the same dir, then rename.
	tmp, err := os.CreateTemp(binDir, ".pf-cli-*.tmp")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(embedbin.Binary); err != nil {
		tmp.Close()
		return "", fmt.Errorf("writing embedded binary: %w", err)
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return "", fmt.Errorf("chmod embedded binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("closing embedded binary: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return "", fmt.Errorf("installing embedded binary: %w", err)
	}
	return path, nil
}
