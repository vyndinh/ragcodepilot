// Package ingest handles the ingestion pipeline: walking files, chunking code,
// embedding chunks, and upserting them to the vector database.
package ingest

import (
	"os"
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

		if isBinaryFile(path) {
			return nil
		}

		files = append(files, path)
		return nil
	})

	return files, err
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
