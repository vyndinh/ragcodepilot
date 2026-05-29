package ingest

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/fsnotify/fsnotify"

	"github.com/dinhvy/ragcodepilot/internal/config"
)

// newWatcherForTest builds a Watcher without a Pipeline. eventIsRelevant and
// refreshWatchedDirs don't touch the pipeline; tests for those can use this.
func newWatcherForTest(t *testing.T, repoPath string) *Watcher {
	t.Helper()
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("fsnotify.NewWatcher: %v", err)
	}
	t.Cleanup(func() { _ = fsw.Close() })
	return &Watcher{
		cfg:         config.Default(),
		repoPath:    repoPath,
		fsw:         fsw,
		watchedDirs: make(map[string]struct{}),
	}
}

func TestEventIsRelevant(t *testing.T) {
	t.Parallel()
	w := newWatcherForTest(t, t.TempDir())

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"go source file", "internal/ingest/walker.go", true},
		{"test file skipped by default", "internal/ingest/walker_test.go", false},
		{"hidden file", "internal/ingest/.swp", false},
		{"non-source extension", "README.md", true}, // .md is a configured language
		{"unknown extension", "internal/foo.unknown", false},
		{"hidden dir prefix on file does not apply", "internal/.dotfile.go", false}, // basename starts with .
		{"deep path", "a/b/c/d/handler.go", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := w.eventIsRelevant(fsnotify.Event{Name: tt.path})
			if got != tt.want {
				t.Errorf("eventIsRelevant(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestRefreshWatchedDirsRespectsConfig(t *testing.T) {
	t.Parallel()

	// Build a small repo tree:
	//   root/
	//     pkg/foo.go
	//     .hidden/leak.go         (hidden dir — must be skipped)
	//     .git/internal.go        (in skip_dirs by default — must be skipped)
	//     vendor/dep.go           (in skip_dirs by default — must be skipped)
	//     internal/ingest/wal.go  (regular)
	root := t.TempDir()
	dirs := []string{
		"pkg",
		".hidden",
		".git",
		"vendor",
		"internal/ingest",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Touch a file in each so the dirs are real (not strictly needed but matches reality).
	for _, d := range dirs {
		if err := os.WriteFile(filepath.Join(root, d, "f.go"), []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	w := newWatcherForTest(t, root)
	if err := w.refreshWatchedDirs(); err != nil {
		t.Fatalf("refreshWatchedDirs: %v", err)
	}

	// Collect watched dirs (relative to root for readability) and sort.
	var watched []string
	for d := range w.watchedDirs {
		rel, err := filepath.Rel(root, d)
		if err != nil {
			rel = d
		}
		watched = append(watched, rel)
	}
	sort.Strings(watched)

	// Must include: root, pkg, internal, internal/ingest
	// Must exclude: .hidden, .git, vendor
	mustInclude := []string{".", "pkg", "internal", "internal/ingest"}
	mustExclude := []string{".hidden", ".git", "vendor"}

	for _, want := range mustInclude {
		if !slices.Contains(watched, want) {
			t.Errorf("expected %q to be watched; got %v", want, watched)
		}
	}
	for _, banned := range mustExclude {
		for _, got := range watched {
			if strings.HasPrefix(got, banned) {
				t.Errorf("watcher unexpectedly added %q (matches banned prefix %q)", got, banned)
			}
		}
	}
}

func TestRefreshWatchedDirsIsIdempotent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a/b"), 0o755); err != nil {
		t.Fatal(err)
	}
	w := newWatcherForTest(t, root)

	if err := w.refreshWatchedDirs(); err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	first := len(w.watchedDirs)

	if err := w.refreshWatchedDirs(); err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	if got := len(w.watchedDirs); got != first {
		t.Errorf("second refresh changed watchedDirs count: %d -> %d", first, got)
	}
}

func TestRefreshWatchedDirsPicksUpNewSubdirs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	w := newWatcherForTest(t, root)

	if err := w.refreshWatchedDirs(); err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	initial := len(w.watchedDirs)

	// Add a new subdir post-watch — simulates a dir created during watch mode.
	if err := os.MkdirAll(filepath.Join(root, "newpkg"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := w.refreshWatchedDirs(); err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	if got := len(w.watchedDirs); got != initial+1 {
		t.Errorf("expected one new watched dir; got %d -> %d", initial, got)
	}
}
