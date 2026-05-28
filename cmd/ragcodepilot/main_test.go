package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dinhvy/ragcodepilot/internal/eval"
	"github.com/dinhvy/ragcodepilot/internal/search"
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

func TestRunEvalRejectsLimitBelowDefault(t *testing.T) {
	t.Parallel()

	err := runEval(
		"docs/eval/golden.yaml",
		"code_chunks",
		"human",
		eval.DefaultLimit-1,
		5, // answerLimit
		"",
		"", // subtypeFilter
		search.DefaultSearchMode,
		"localhost",
		6334,
		"ollama",
		"nomic-embed-text",
		nil,
		nil,
		"",
	)
	if err == nil {
		t.Fatal("expected error for limit below default")
	}
	if !strings.Contains(err.Error(), "must be >=") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveGenerator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		genType   string
		wantErr   bool
		errSubstr string
	}{
		{name: "ollama", genType: "ollama"},
		{name: "fake", genType: "fake"},
		{name: "unknown", genType: "bogus", wantErr: true, errSubstr: "unknown generator"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gen, err := resolveGenerator(tt.genType, "http://localhost:11434", "qwen2.5-coder:7b")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.genType)
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error = %v, want substring %q", err, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gen == nil {
				t.Error("expected a non-nil generator")
			}
		})
	}
}
