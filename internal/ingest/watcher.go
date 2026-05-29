package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/dinhvy/ragcodepilot/internal/config"
)

// DefaultWatchDebounce is how long the watcher waits after the last filesystem
// event before triggering a re-index. Tuned so editor save sequences (write
// temp file, rename to target, multiple Write events) collapse into a single
// Pipeline.Run, and "git pull"-style burst changes batch into one pass.
const DefaultWatchDebounce = 500 * time.Millisecond

// Watcher monitors a repo directory for source file changes and re-runs
// the ingestion pipeline (debounced) when something relevant changes.
//
// Design choice: instead of incrementally re-indexing only the changed files,
// each debounced fire invokes the full Pipeline.Run. This keeps the sparse
// vector IDF — which is corpus-wide — consistent across the corpus after
// every change. Pipeline.Run is already idempotent and uses file hashes to
// skip unchanged files cheaply, so the cost is "stat + hash unchanged files
// once per change burst." For small-to-medium repos this is well under a
// second; large repos can tune DefaultWatchDebounce or invoke Pipeline.Run
// less aggressively.
type Watcher struct {
	pipeline *Pipeline
	cfg      *config.Config
	repoPath string
	debounce time.Duration

	fsw         *fsnotify.Watcher
	watchedDirs map[string]struct{}
}

// NewWatcher constructs a Watcher rooted at the given repo path. The returned
// Watcher does not begin observing until Watch is called.
func NewWatcher(p *Pipeline, cfg *config.Config, repoPath string) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating fsnotify watcher: %w", err)
	}
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		_ = fsw.Close()
		return nil, fmt.Errorf("resolving repo path: %w", err)
	}
	return &Watcher{
		pipeline:    p,
		cfg:         cfg,
		repoPath:    absPath,
		debounce:    DefaultWatchDebounce,
		fsw:         fsw,
		watchedDirs: make(map[string]struct{}),
	}, nil
}

// SetDebounce overrides the debounce window. Useful in tests to keep them fast.
func (w *Watcher) SetDebounce(d time.Duration) { w.debounce = d }

// Watch blocks until ctx is cancelled. It registers fsnotify watchers on every
// directory in the repo tree that the config would also walk for indexing, then
// drives a debounced event loop:
//   - any relevant event resets a debounce timer
//   - when the timer fires, Pipeline.Run executes against the full repo
//   - after each run, watchers are refreshed in case new dirs were created
//
// Errors from individual re-runs are logged to stderr but do not stop the
// loop — a transient Qdrant or Ollama hiccup should not kill watch mode.
func (w *Watcher) Watch(ctx context.Context) error {
	defer func() { _ = w.fsw.Close() }()

	if err := w.refreshWatchedDirs(); err != nil {
		return fmt.Errorf("registering initial watchers: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Watching %s for changes (debounce %s). Ctrl-C to exit.\n", w.repoPath, w.debounce)

	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	pending := false

	for {
		select {
		case <-ctx.Done():
			return nil

		case evt, ok := <-w.fsw.Events:
			if !ok {
				return nil
			}
			if !w.eventIsRelevant(evt) {
				continue
			}
			// (Re)start debounce timer.
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.debounce)
			pending = true

		case <-timer.C:
			if !pending {
				continue
			}
			pending = false
			fmt.Fprintln(os.Stderr, "Change detected — re-indexing...")
			if err := w.pipeline.Run(ctx, w.repoPath); err != nil {
				fmt.Fprintf(os.Stderr, "re-index error (continuing): %v\n", err)
			}
			// New directories may have been created; refresh watchers.
			if err := w.refreshWatchedDirs(); err != nil {
				fmt.Fprintf(os.Stderr, "refresh watchers error (continuing): %v\n", err)
			}

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintf(os.Stderr, "watcher error (continuing): %v\n", err)
		}
	}
}

// eventIsRelevant returns true when the event is for a source file the indexer
// would actually process. Mirrors the WalkFiles filter so the watcher never
// triggers a re-run for hidden files, test files, or non-source extensions.
func (w *Watcher) eventIsRelevant(evt fsnotify.Event) bool {
	base := filepath.Base(evt.Name)

	// Skip hidden files (mirrors walker.go).
	if strings.HasPrefix(base, ".") {
		return false
	}
	// On Remove/Rename, the file no longer exists — but the pipeline's stale
	// detection will handle deletion of its chunks. Let the event through if
	// the path looks like a source file or test file (rules below).
	if w.cfg.ShouldSkipFile(base) {
		return false
	}
	if !w.cfg.IsSourceFile(base) {
		return false
	}
	return true
}

// refreshWatchedDirs walks the repo tree (respecting skip_dirs and the
// hidden-dir convention) and registers fsnotify watchers on every directory
// that isn't already being watched. Safe to call repeatedly.
func (w *Watcher) refreshWatchedDirs() error {
	return filepath.Walk(w.repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // ignore unreadable paths
		}
		if !info.IsDir() {
			return nil
		}
		name := info.Name()
		// Skip hidden dirs (except the walk root) and configured skip_dirs.
		// Mirrors internal/ingest/walker.go.
		if path != w.repoPath && strings.HasPrefix(name, ".") {
			return filepath.SkipDir
		}
		if w.cfg.ShouldSkipDir(name) {
			return filepath.SkipDir
		}
		if _, already := w.watchedDirs[path]; already {
			return nil
		}
		if err := w.fsw.Add(path); err != nil {
			// Don't fail the whole walk for one unwatchable dir; just skip it.
			fmt.Fprintf(os.Stderr, "warning: cannot watch %s: %v\n", path, err)
			return nil
		}
		w.watchedDirs[path] = struct{}{}
		return nil
	})
}
