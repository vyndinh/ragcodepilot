package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type denseCache struct {
	root   string
	hits   int
	misses int
}

type denseCacheEntry struct {
	Key    string    `json:"key"`
	Vector []float32 `json:"vector"`
}

func openDenseCache(stateDir, collection string) (*denseCache, error) {
	if stateDir == "" {
		return nil, nil
	}
	root := filepath.Join(stateDir, "dense-cache", collectionKey(collection))
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("creating dense cache: %w", err)
	}
	return &denseCache{root: root}, nil
}

func denseCacheKey(identity, representation, enriched string) string {
	digest := sha256.Sum256([]byte("identity=" + identity + "\nrepresentation=" + representation + "\ninput=" + enriched))
	return hex.EncodeToString(digest[:])
}

func (c *denseCache) get(key string, expectedDim int) ([]float32, bool) {
	if c == nil {
		return nil, false
	}
	data, err := os.ReadFile(filepath.Join(c.root, key+".json"))
	if err != nil {
		c.misses++
		return nil, false
	}
	var entry denseCacheEntry
	if json.Unmarshal(data, &entry) != nil || entry.Key != key || len(entry.Vector) == 0 || (expectedDim > 0 && len(entry.Vector) != expectedDim) {
		c.misses++
		return nil, false
	}
	c.hits++
	return entry.Vector, true
}

func (c *denseCache) put(key string, vector []float32) error {
	if c == nil {
		return nil
	}
	data, err := json.Marshal(denseCacheEntry{Key: key, Vector: vector})
	if err != nil {
		return fmt.Errorf("encoding dense cache entry: %w", err)
	}
	tmp, err := os.CreateTemp(c.root, ".dense-cache-*.tmp")
	if err != nil {
		return fmt.Errorf("creating dense cache entry: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("setting dense cache permissions: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing dense cache entry: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing dense cache entry: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing dense cache entry: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(c.root, key+".json")); err != nil {
		return fmt.Errorf("publishing dense cache entry: %w", err)
	}
	return nil
}

func (c *denseCache) summary() string {
	if c == nil {
		return "Dense cache: disabled"
	}
	return fmt.Sprintf("Dense cache: %d hits, %d misses", c.hits, c.misses)
}

func fallbackEmbedderIdentity(embedder interface{ Dimension() int }) string {
	return fmt.Sprintf("%T:dimension=%d", embedder, embedder.Dimension())
}

func normalizeCacheIdentity(value string) string {
	return strings.TrimSpace(value)
}
