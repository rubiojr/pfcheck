package pfcheck

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// BinaryName is the upstream privacy-filter.cpp CLI executable name.
const BinaryName = "pf-cli"

// ResolveBinary locates the pf-cli executable, checking, in order:
//
//  1. The PFCHECK_PF_CLI environment variable (explicit override).
//  2. The binary embedded at build time (extracted to the cache).
//  3. The pfcheck cache directory (where scripts/build-pf.sh installs it).
//  4. The system PATH.
func ResolveBinary() (string, error) {
	if env := os.Getenv("PFCHECK_PF_CLI"); env != "" {
		if isExecutable(env) {
			return env, nil
		}
		return "", fmt.Errorf("PFCHECK_PF_CLI=%q is not an executable file", env)
	}

	if HasEmbeddedBinary() {
		path, err := extractEmbeddedBinary()
		if err != nil {
			return "", fmt.Errorf("extracting embedded pf-cli: %w", err)
		}
		return path, nil
	}

	if dir, err := CacheDir(); err == nil {
		candidate := filepath.Join(dir, "bin", BinaryName)
		if isExecutable(candidate) {
			return candidate, nil
		}
	}

	if path, err := exec.LookPath(BinaryName); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("%s not found: set PFCHECK_PF_CLI, add it to PATH, or run scripts/build-pf.sh", BinaryName)
}

func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	return fi.Mode()&0o111 != 0
}
