package check

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsTextFile(t *testing.T) {
	cases := map[string]bool{
		"notes.txt":      true,
		"README.md":      true,
		"DOC.MD":         true, // case-insensitive
		"data.json":      true,
		"page.html":      true,
		"config.yaml":    true,
		"thing.markdown": true,
		// Source code (top languages).
		"main.go":     true,
		"app.py":      true,
		"index.ts":    true,
		"lib.rs":      true,
		"Server.java": true,
		"query.sql":   true,
		"core.cpp":    true,
		"script.rb":   true,
		"view.vue":    true,
		// Extensionless build files.
		"Makefile":   true,
		"Dockerfile": true,
		// Non-text / binary.
		"archive.bin": false,
		"image.png":   false,
		"binary":      false,
		"photo.jpeg":  false,
		"sound.mp3":   false,
	}
	for name, want := range cases {
		if got := isTextFile(name); got != want {
			t.Errorf("isTextFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestGatherDir(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.md"), "My name is Sergio")
	mustWrite(t, filepath.Join(dir, "clean.txt"), "nothing here")
	mustWrite(t, filepath.Join(dir, "empty.txt"), "   \n")
	mustWrite(t, filepath.Join(dir, "ignore.bin"), "binary-ish")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "sub", "c.rst"), "nested text")
	// Hidden directory should be skipped.
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".git", "config.txt"), "should be skipped")

	inputs, err := gatherDir(dir)
	if err != nil {
		t.Fatalf("gatherDir: %v", err)
	}

	got := make([]string, len(inputs))
	for i, in := range inputs {
		got[i] = in.name
	}
	want := []string{
		filepath.Join(dir, "a.md"),
		filepath.Join(dir, "clean.txt"),
		filepath.Join(dir, "sub", "c.rst"),
	}
	if len(got) != len(want) {
		t.Fatalf("gatherDir returned %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("input[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestGatherDirEmpty(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "ignore.bin"), "data")
	if _, err := gatherDir(dir); err == nil {
		t.Error("expected error when no text files are found")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
