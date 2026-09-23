// Package ingest handles the ingestion pipeline: walking files, chunking code,
// embedding chunks, and upserting them to the vector database.
package ingest

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dinhvy/ragcodepilot/internal/config"
)

const binaryCheckBytes = 8000

func WalkFiles(root string, cfg *config.Config) ([]string, error) {
	var files []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// Skip hidden directories (.git, .claude, .idea, .venv, ...) — except
			// the walk root itself, which may legitimately be a dot path. Hidden
			// dirs hold VCS data, tooling state, and git worktrees (a full repo
			// copy under .claude/worktrees/) that would pollute the index with
			// duplicate chunks of the whole codebase.
			if path != root && strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			if cfg.ShouldSkipDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		if !cfg.IsSourceFile(info.Name()) {
			return nil
		}

		if cfg.ShouldSkipFile(info.Name()) {
			return nil
		}

		if isBinaryFile(path) {
			return nil
		}

		files = append(files, path)
		return nil
	})

	if err != nil {
		return nil, err
	}
	return applyGitIgnore(root, files)
}

// applyGitIgnore applies the repository's nested .gitignore rules after the
// configured walker exclusions. Git remains the source of truth for negation,
// directory precedence, and pattern anchoring; config and hidden-file skips
// therefore always take precedence over Git negation.
func applyGitIgnore(root string, files []string) ([]string, error) {
	if len(files) == 0 {
		return files, nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return files, nil
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		rel, err := filepath.Rel(root, file)
		if err != nil {
			return nil, fmt.Errorf("resolving gitignore path %s: %w", file, err)
		}
		paths = append(paths, filepath.ToSlash(rel))
	}
	var input bytes.Buffer
	for _, path := range paths {
		input.WriteString(path)
		input.WriteByte(0)
	}
	cmd := exec.Command("git", "-C", root, "check-ignore", "--no-index", "--stdin", "-z")
	cmd.Stdin = &input
	output, err := cmd.Output()
	if err != nil {
		// Exit status 1 means no paths matched. Status 128 means the root is
		// not a Git worktree; in both cases there is no Git policy to apply.
		if exitErr, ok := err.(*exec.ExitError); ok && (exitErr.ExitCode() == 1 || exitErr.ExitCode() == 128) {
			return files, nil
		}
		return nil, fmt.Errorf("checking .gitignore rules: %w", err)
	}
	ignored := make(map[string]struct{})
	for _, raw := range bytes.Split(output, []byte{0}) {
		if len(raw) > 0 {
			ignored[string(raw)] = struct{}{}
		}
	}
	filtered := make([]string, 0, len(files)-len(ignored))
	for i, file := range files {
		if _, skip := ignored[paths[i]]; !skip {
			filtered = append(filtered, file)
		}
	}
	return filtered, nil
}

func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, binaryCheckBytes)
	n, err := f.Read(buf)
	if err != nil {
		return true
	}

	buf = buf[:n]
	for _, b := range buf {
		if b == 0 {
			return true
		}
	}
	return false
}
