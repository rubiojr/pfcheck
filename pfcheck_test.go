package pfcheck

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseEntities(t *testing.T) {
	// Matches the exact JSON shape pf-cli emits.
	out := []byte(`[
 {"entity_group": "PERSON", "start": 8, "end": 16, "score": 0.9876, "text": "John Doe"},
 {"entity_group": "EMAIL", "start": 20, "end": 36, "score": 0.9412, "text": "jdoe@example.com"}
]
`)
	ents, err := parseEntities(out, "")
	if err != nil {
		t.Fatalf("parseEntities: %v", err)
	}
	if len(ents) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(ents))
	}
	if ents[0].EntityGroup != "PERSON" || ents[0].Text != "John Doe" {
		t.Errorf("entity[0] mismatch: %+v", ents[0])
	}
	if ents[0].Start != 8 || ents[0].End != 16 {
		t.Errorf("entity[0] offsets mismatch: %+v", ents[0])
	}
	if ents[1].EntityGroup != "EMAIL" || ents[1].Score != 0.9412 {
		t.Errorf("entity[1] mismatch: %+v", ents[1])
	}
}

func TestParseEntitiesEmpty(t *testing.T) {
	ents, err := parseEntities([]byte("[\n]\n"), "")
	if err != nil {
		t.Fatalf("parseEntities: %v", err)
	}
	if len(ents) != 0 {
		t.Fatalf("expected 0 entities, got %d", len(ents))
	}
}

func TestParseEntitiesNoOutput(t *testing.T) {
	if _, err := parseEntities([]byte("   \n"), ""); err == nil {
		t.Fatal("expected error for empty output")
	}
}

func TestParseEntitiesBadJSON(t *testing.T) {
	if _, err := parseEntities([]byte("not json"), ""); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestParseEntitiesUnescapedText(t *testing.T) {
	// pf-cli does not escape the "text" field. When a span contains a double
	// quote the output is invalid JSON; the lenient path must recover it and
	// rebuild text from byte offsets in the input.
	input := `the user "rubiojr" logged in`
	out := []byte(`[
 {"entity_group": "USERNAME", "start": 9, "end": 18, "score": 0.7500, "text": ""rubiojr""}
]
`)
	ents, err := parseEntities(out, input)
	if err != nil {
		t.Fatalf("parseEntities: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(ents))
	}
	got := ents[0]
	if got.EntityGroup != "USERNAME" || got.Start != 9 || got.End != 18 {
		t.Errorf("unexpected entity fields: %+v", got)
	}
	if got.Score != 0.75 {
		t.Errorf("score = %v, want 0.75", got.Score)
	}
	if want := `"rubiojr"`; got.Text != want {
		t.Errorf("text = %q, want %q", got.Text, want)
	}
}

func TestParseEntitiesUnescapedBackslashAndNewline(t *testing.T) {
	// A span containing a backslash (e.g. a Windows path in source code) also
	// breaks strict JSON; verify recovery and multiple entities.
	input := `path C:\Users\sergio and mail jane@x.io`
	out := []byte(`[
 {"entity_group": "PATH", "start": 5, "end": 20, "score": 0.6000, "text": "C:\Users\sergio"},
 {"entity_group": "EMAIL", "start": 30, "end": 39, "score": 0.9900, "text": "jane@x.io"}
]
`)
	ents, err := parseEntities(out, input)
	if err != nil {
		t.Fatalf("parseEntities: %v", err)
	}
	if len(ents) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(ents))
	}
	if ents[0].Text != `C:\Users\sergio` {
		t.Errorf("entity[0].Text = %q", ents[0].Text)
	}
	if ents[1].EntityGroup != "EMAIL" || ents[1].Text != "jane@x.io" {
		t.Errorf("entity[1] mismatch: %+v", ents[1])
	}
}

func TestModelURL(t *testing.T) {
	want := "https://huggingface.co/LocalAI-io/privacy-filter-multilingual-GGUF/resolve/main/privacy-filter-multilingual-f16.gguf"
	if got := ModelURL(); got != want {
		t.Errorf("ModelURL() = %q, want %q", got, want)
	}
}

func TestDefaultModelSpec(t *testing.T) {
	s := DefaultModelSpec()
	if s.Repo != ModelRepo || s.File != ModelFile {
		t.Errorf("DefaultModelSpec() = %+v, want repo=%q file=%q", s, ModelRepo, ModelFile)
	}
}

func TestDefaultModelSpecEnvOverride(t *testing.T) {
	t.Setenv("PFCHECK_MODEL_REPO", "acme/custom-GGUF")
	t.Setenv("PFCHECK_MODEL_FILE", "custom-q4.gguf")
	s := DefaultModelSpec()
	if s.Repo != "acme/custom-GGUF" || s.File != "custom-q4.gguf" {
		t.Errorf("DefaultModelSpec() = %+v, want overridden values", s)
	}
	wantURL := "https://huggingface.co/acme/custom-GGUF/resolve/main/custom-q4.gguf"
	if got := s.URL(); got != wantURL {
		t.Errorf("URL() = %q, want %q", got, wantURL)
	}
}

func TestModelSpecCachePath(t *testing.T) {
	t.Setenv("PFCHECK_CACHE_DIR", "/tmp/custom-pfcheck")
	s := ModelSpec{Repo: "acme/custom-GGUF", File: "custom-q4.gguf"}
	got, err := s.CachePath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/tmp/custom-pfcheck", "custom-q4.gguf")
	if got != want {
		t.Errorf("CachePath() = %q, want %q", got, want)
	}
}

func TestCacheDirEnvOverride(t *testing.T) {
	t.Setenv("PFCHECK_CACHE_DIR", "/tmp/custom-pfcheck")
	got, err := CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/custom-pfcheck" {
		t.Errorf("CacheDir() = %q, want /tmp/custom-pfcheck", got)
	}
}

func TestDefaultModelPath(t *testing.T) {
	t.Setenv("PFCHECK_CACHE_DIR", "/tmp/custom-pfcheck")
	got, err := DefaultModelPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/tmp/custom-pfcheck", ModelFile)
	if got != want {
		t.Errorf("DefaultModelPath() = %q, want %q", got, want)
	}
}

func TestValidateModel(t *testing.T) {
	dir := t.TempDir()

	if err := validateModel(filepath.Join(dir, "missing.gguf")); err == nil {
		t.Error("expected error for missing model")
	}

	small := filepath.Join(dir, "small.gguf")
	if err := os.WriteFile(small, []byte("tiny"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateModel(small); err == nil {
		t.Error("expected error for truncated model")
	}

	if err := validateModel(dir); err == nil {
		t.Error("expected error for directory")
	}
}

func TestResolveBinaryEnv(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "pf-cli")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PFCHECK_PF_CLI", bin)
	got, err := ResolveBinary()
	if err != nil {
		t.Fatalf("ResolveBinary: %v", err)
	}
	if got != bin {
		t.Errorf("ResolveBinary() = %q, want %q", got, bin)
	}
}

func TestResolveBinaryEnvNotExecutable(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "pf-cli")
	if err := os.WriteFile(bin, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PFCHECK_PF_CLI", bin)
	if _, err := ResolveBinary(); err == nil {
		t.Error("expected error for non-executable PFCHECK_PF_CLI")
	}
}

func TestResolveBinaryCacheDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PFCHECK_PF_CLI", "")
	t.Setenv("PFCHECK_CACHE_DIR", dir)
	t.Setenv("PATH", "") // ensure PATH lookup fails

	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(binDir, BinaryName)
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveBinary()
	if err != nil {
		t.Fatalf("ResolveBinary: %v", err)
	}
	if got != bin {
		t.Errorf("ResolveBinary() = %q, want %q", got, bin)
	}
}

func TestResolveBinaryNotFound(t *testing.T) {
	t.Setenv("PFCHECK_PF_CLI", "")
	t.Setenv("PFCHECK_CACHE_DIR", t.TempDir())
	t.Setenv("PATH", "")
	if _, err := ResolveBinary(); err == nil {
		t.Error("expected error when pf-cli is unavailable")
	}
}

func TestEnsureModelExisting(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.gguf")
	// Create a file just over the minimum size threshold.
	if err := os.Truncate(createFile(t, model), modelMinSize+1); err != nil {
		t.Fatal(err)
	}

	got, err := EnsureModel(context.Background(), model, nil)
	if err != nil {
		t.Fatalf("EnsureModel: %v", err)
	}
	if got != model {
		t.Errorf("EnsureModel() = %q, want %q", got, model)
	}
}

func createFile(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	return path
}
