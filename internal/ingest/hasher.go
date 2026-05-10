package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// HashFile returns the hex-encoded SHA-256 hash of the file at the given path.
func HashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading file for hashing: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// HashFiles computes SHA-256 hashes for all given file paths.
// Returns a map of path → hex hash. Errors on individual files are returned immediately.
func HashFiles(paths []string) (map[string]string, error) {
	hashes := make(map[string]string, len(paths))
	for _, p := range paths {
		h, err := HashFile(p)
		if err != nil {
			return nil, fmt.Errorf("hashing %s: %w", p, err)
		}
		hashes[p] = h
	}
	return hashes, nil
}
