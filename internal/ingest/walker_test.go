package ingest

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dinhvy/ragcodepilot/internal/config"
)

func TestWalkFilesSkipsTestFiles(t *testing.T) {
	root := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("foo.go")
	write("foo_test.go")
	write("bar.go")
	write("bar_test.go")

	cfg := config.Default()

	files, err := WalkFiles(root, cfg)
	if err != nil {
		t.Fatalf("WalkFiles: %v", err)
	}

	var got []string
	for _, f := range files {
		got = append(got, filepath.Base(f))
	}
	sort.Strings(got)

	want := []string{"bar.go", "foo.go"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
			break
		}
	}
}

func TestWalkFilesSkipsHiddenDirs(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Canonical source plus copies buried in hidden dirs that must NOT be indexed:
	// a git worktree under .claude (the real-world duplication bug) and a .git copy.
	mustWrite("internal/foo.go")
	mustWrite(".claude/worktrees/wt1/internal/foo.go")
	mustWrite(".git/somecopy.go")

	files, err := WalkFiles(root, config.Default())
	if err != nil {
		t.Fatalf("WalkFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected only the canonical file, got %d: %v", len(files), files)
	}
	for _, f := range files {
		if strings.Contains(f, ".claude") || strings.Contains(f, ".git") {
			t.Errorf("hidden-dir file should be skipped, got %v", f)
		}
	}
}

func TestWalkFilesIncludesTestFilesWhenDisabled(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "foo.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "foo_test.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.SkipFilePatterns = []string{}

	files, err := WalkFiles(root, cfg)
	if err != nil {
		t.Fatalf("WalkFiles: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files when skipping disabled, got %d: %v", len(files), files)
	}
}
