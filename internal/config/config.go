// Package config loads and provides application configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// defaultSkipFilePatterns is applied when a config does not specify
// skip_file_patterns. It excludes Go test files from indexing.
var defaultSkipFilePatterns = []string{"*_test.go"}

// Config holds the application configuration.
type Config struct {
	// Languages maps a language name to its file extensions.
	// Example: "go" -> [".go"], "c" -> [".c", ".h"]
	Languages map[string][]string `yaml:"languages"`

	// SkipDirs lists directory names to skip during file walking.
	SkipDirs []string `yaml:"skip_dirs"`

	// SkipFilePatterns lists glob patterns (matched against the file's base
	// name) for files to exclude from indexing, e.g. "*_test.go". A nil value
	// means "not configured" and falls back to defaultSkipFilePatterns; an
	// explicitly empty list disables file-pattern skipping entirely.
	SkipFilePatterns []string `yaml:"skip_file_patterns"`

	// skipDirSet is a set built from SkipDirs for O(1) lookup.
	skipDirSet map[string]struct{}

	// extToLang is a reverse lookup built from Languages: extension -> language name.
	extToLang map[string]string
}

// Load reads and parses a config file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("loading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	// A nil slice means the key was omitted; fall back to the default.
	// An explicitly empty list (skip_file_patterns: []) disables skipping.
	if cfg.SkipFilePatterns == nil {
		cfg.SkipFilePatterns = defaultSkipFilePatterns
	}

	cfg.buildIndexes()
	return &cfg, nil
}

// Default returns the default configuration with built-in language mappings.
// Used as a fallback when no config file is provided.
func Default() *Config {
	cfg := &Config{
		Languages: map[string][]string{
			"go":         {".go"},
			"rust":       {".rs"},
			"python":     {".py"},
			"javascript": {".js"},
			"typescript": {".ts"},
			"java":       {".java"},
			"c":          {".c", ".h"},
			"cpp":        {".cpp", ".cc", ".cxx", ".hpp"},
			"ruby":       {".rb"},
			"php":        {".php"},
			"swift":      {".swift"},
			"kotlin":     {".kt"},
			"scala":      {".scala"},
			"shell":      {".sh", ".bash"},
			"sql":        {".sql"},
			"markdown":   {".md"},
			"yaml":       {".yaml", ".yml"},
			"toml":       {".toml"},
			"json":       {".json"},
		},
		SkipDirs: []string{
			".git", "vendor", "node_modules", ".venv",
			"__pycache__", "target", "bin", "build",
		},
		SkipFilePatterns: defaultSkipFilePatterns,
	}
	cfg.buildIndexes()
	return cfg
}

// DetectLanguage returns the language name for a given file path based on its extension.
// Returns "unknown" if the extension is not mapped.
func (c *Config) DetectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	if lang, ok := c.extToLang[ext]; ok {
		return lang
	}
	return "unknown"
}

// IsSourceFile returns true if the file has a recognized source code extension.
func (c *Config) IsSourceFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	_, ok := c.extToLang[ext]
	return ok
}

// ShouldSkipDir returns true if the directory name should be skipped.
func (c *Config) ShouldSkipDir(name string) bool {
	_, ok := c.skipDirSet[name]
	return ok
}

// ShouldSkipFile returns true if the file's base name matches any configured
// skip_file_patterns glob (e.g. "*_test.go"). Invalid patterns are ignored.
func (c *Config) ShouldSkipFile(name string) bool {
	base := filepath.Base(name)
	for _, pattern := range c.SkipFilePatterns {
		if ok, err := filepath.Match(pattern, base); err == nil && ok {
			return true
		}
	}
	return false
}

// buildIndexes creates the reverse lookup maps for fast access.
func (c *Config) buildIndexes() {
	// Build extension -> language index.
	c.extToLang = make(map[string]string, len(c.Languages)*2)
	for lang, exts := range c.Languages {
		for _, ext := range exts {
			c.extToLang[strings.ToLower(ext)] = lang
		}
	}

	// Build skip directory set for O(1) lookup.
	c.skipDirSet = make(map[string]struct{}, len(c.SkipDirs))
	for _, d := range c.SkipDirs {
		c.skipDirSet[d] = struct{}{}
	}
}
