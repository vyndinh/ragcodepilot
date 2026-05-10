package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveIndexConfig_MissingUsesDefault(t *testing.T) {
	withTempWorkingDir(t)

	cfg, err := resolveIndexConfig()
	if err != nil {
		t.Fatalf("resolveIndexConfig() unexpected error: %v", err)
	}

	if !cfg.IsSourceFile("main.go") {
		t.Fatalf("expected default config to recognize .go files")
	}
}

func TestResolveIndexConfig_ValidConfigFile(t *testing.T) {
	withTempWorkingDir(t)

	content := `
languages:
  zig: [".zig"]
skip_dirs:
  - .zig-cache
`
	if err := os.WriteFile(defaultConfigPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	cfg, err := resolveIndexConfig()
	if err != nil {
		t.Fatalf("resolveIndexConfig() unexpected error: %v", err)
	}

	if !cfg.IsSourceFile("main.zig") {
		t.Fatalf("expected config.yaml to recognize .zig files")
	}
	if cfg.IsSourceFile("main.go") {
		t.Fatalf("expected config.yaml to override defaults; .go should not be recognized")
	}
}

func TestResolveIndexConfig_InvalidYAMLReturnsError(t *testing.T) {
	withTempWorkingDir(t)

	if err := os.WriteFile(defaultConfigPath, []byte("languages: ["), 0o644); err != nil {
		t.Fatalf("write invalid config.yaml: %v", err)
	}

	_, err := resolveIndexConfig()
	if err == nil {
		t.Fatalf("expected error for invalid config.yaml")
	}
}

func TestResolveIndexConfig_UnreadableReturnsError(t *testing.T) {
	withTempWorkingDir(t)

	if err := os.Mkdir(defaultConfigPath, 0o755); err != nil {
		t.Fatalf("create directory named config.yaml: %v", err)
	}

	_, err := resolveIndexConfig()
	if err == nil {
		t.Fatalf("expected error when config.yaml is not a readable file")
	}
}

func withTempWorkingDir(t *testing.T) {
	t.Helper()

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir to temp dir %s: %v", tempDir, err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(filepath.Clean(originalWD))
	})
}
