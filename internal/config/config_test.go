package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestShouldSkipFile(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		file     string
		want     bool
	}{
		{"go test file skipped by default", []string{"*_test.go"}, "foo_test.go", true},
		{"go source file kept", []string{"*_test.go"}, "foo.go", false},
		{"full path go test file skipped", []string{"*_test.go"}, "internal/ingest/walker_test.go", true},
		{"full path go source kept", []string{"*_test.go"}, "internal/ingest/walker.go", false},
		{"ts spec pattern matches", []string{"*.test.ts"}, "button.test.ts", true},
		{"ts source kept", []string{"*.test.ts"}, "button.ts", false},
		{"multiple patterns, second matches", []string{"*_test.go", "*.test.ts"}, "button.test.ts", true},
		{"no patterns skips nothing", []string{}, "foo_test.go", false},
		{"nil patterns skips nothing", nil, "foo_test.go", false},
		{"invalid pattern ignored", []string{"[", "*_test.go"}, "foo_test.go", true},
		{"file named just _test.go kept", []string{"*_test.go"}, "_test.go", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{SkipFilePatterns: tt.patterns}
			if got := c.ShouldSkipFile(tt.file); got != tt.want {
				t.Errorf("ShouldSkipFile(%q) with patterns %v = %v, want %v", tt.file, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestDefaultSkipsGoTestFiles(t *testing.T) {
	cfg := Default()
	if !cfg.ShouldSkipFile("foo_test.go") {
		t.Errorf("Default() config should skip foo_test.go")
	}
	if cfg.ShouldSkipFile("foo.go") {
		t.Errorf("Default() config should not skip foo.go")
	}
}

func TestLoadAppliesDefaultWhenPatternsOmitted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := "languages:\n  go: [\".go\"]\nskip_dirs:\n  - .git\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(cfg.SkipFilePatterns, defaultSkipFilePatterns) {
		t.Errorf("SkipFilePatterns = %v, want default %v", cfg.SkipFilePatterns, defaultSkipFilePatterns)
	}
	if !cfg.ShouldSkipFile("foo_test.go") {
		t.Errorf("config with omitted skip_file_patterns should skip foo_test.go")
	}
}

func TestLoadEmptyPatternsDisablesSkipping(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := "languages:\n  go: [\".go\"]\nskip_file_patterns: []\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ShouldSkipFile("foo_test.go") {
		t.Errorf("explicit empty skip_file_patterns should index test files")
	}
}

func TestLoadHonorsCustomPatterns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := "languages:\n  ts: [\".ts\"]\nskip_file_patterns:\n  - \"*.test.ts\"\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.ShouldSkipFile("button.test.ts") {
		t.Errorf("custom pattern should skip button.test.ts")
	}
	if cfg.ShouldSkipFile("foo_test.go") {
		t.Errorf("custom pattern should not skip foo_test.go")
	}
}
