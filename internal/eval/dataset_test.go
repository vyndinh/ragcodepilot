package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodDataset = `queries:
  - id: chunking_overview
    query: "how does chunking work?"
    type: concept
    filters:
      languages: ["go"]
    expected:
      files:
        - internal/ingest/chunker.go
      symbols:
        - ChunkFile

  - id: nonexistent_oauth
    query: "where is the OAuth middleware?"
    type: negative
    negative:
      top1_score_below: 0.5
`

func writeTempDataset(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "golden.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDataset(t *testing.T) {
	t.Parallel()
	path := writeTempDataset(t, goodDataset)
	ds, err := LoadDataset(path)
	if err != nil {
		t.Fatalf("LoadDataset() unexpected error: %v", err)
	}
	if len(ds.Queries) != 2 {
		t.Fatalf("loaded %d queries, want 2", len(ds.Queries))
	}

	q0 := ds.Queries[0]
	if q0.ID != "chunking_overview" {
		t.Errorf("Q0 id = %q, want chunking_overview", q0.ID)
	}
	if q0.Type != TypeConcept {
		t.Errorf("Q0 type = %q, want concept", q0.Type)
	}
	if len(q0.Filters.Languages) != 1 || q0.Filters.Languages[0] != "go" {
		t.Errorf("Q0 languages = %v, want [go]", q0.Filters.Languages)
	}
	if len(q0.Expected.Files) != 1 || q0.Expected.Files[0] != "internal/ingest/chunker.go" {
		t.Errorf("Q0 expected files = %v", q0.Expected.Files)
	}
	if len(q0.Expected.Symbols) != 1 || q0.Expected.Symbols[0] != "ChunkFile" {
		t.Errorf("Q0 expected symbols = %v", q0.Expected.Symbols)
	}

	q1 := ds.Queries[1]
	if q1.Type != TypeNegative {
		t.Errorf("Q1 type = %q, want negative", q1.Type)
	}
	if q1.Negative.Top1ScoreBelow != 0.5 {
		t.Errorf("Q1 top1_score_below = %v, want 0.5", q1.Negative.Top1ScoreBelow)
	}
}

func TestLoadDatasetSubtype(t *testing.T) {
	t.Parallel()
	const subtypeDataset = `queries:
  - id: trace_q
    query: "trace from A to B"
    type: navigation
    subtype: structural
    expected:
      files:
        - internal/foo.go
  - id: plain_q
    query: "regular query"
    type: navigation
    expected:
      files:
        - internal/bar.go
`
	path := writeTempDataset(t, subtypeDataset)
	ds, err := LoadDataset(path)
	if err != nil {
		t.Fatalf("LoadDataset() unexpected error: %v", err)
	}
	if ds.Queries[0].Subtype != "structural" {
		t.Errorf("Q0 Subtype = %q, want structural", ds.Queries[0].Subtype)
	}
	if ds.Queries[1].Subtype != "" {
		t.Errorf("Q1 Subtype should be empty when omitted, got %q", ds.Queries[1].Subtype)
	}
}

func TestLoadDatasetEmpty(t *testing.T) {
	t.Parallel()
	path := writeTempDataset(t, "queries: []\n")
	_, err := LoadDataset(path)
	if err == nil || !strings.Contains(err.Error(), "no queries") {
		t.Fatalf("expected 'no queries' error, got %v", err)
	}
}

func TestLoadDatasetMissingID(t *testing.T) {
	t.Parallel()
	path := writeTempDataset(t, `queries:
  - query: "x"
    expected:
      files: ["a.go"]
`)
	_, err := LoadDataset(path)
	if err == nil || !strings.Contains(err.Error(), "no id") {
		t.Fatalf("expected 'no id' error, got %v", err)
	}
}

func TestLoadDatasetDuplicateID(t *testing.T) {
	t.Parallel()
	path := writeTempDataset(t, `queries:
  - id: x
    query: "a"
    expected:
      files: ["a.go"]
  - id: x
    query: "b"
    expected:
      files: ["b.go"]
`)
	_, err := LoadDataset(path)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected 'duplicate' error, got %v", err)
	}
}

func TestLoadDatasetMissingExpected(t *testing.T) {
	t.Parallel()
	path := writeTempDataset(t, `queries:
  - id: x
    query: "a"
    type: concept
`)
	_, err := LoadDataset(path)
	if err == nil || !strings.Contains(err.Error(), "no expected") {
		t.Fatalf("expected 'no expected' error, got %v", err)
	}
}

func TestLoadDatasetNegativeAllowsNoExpected(t *testing.T) {
	t.Parallel()
	path := writeTempDataset(t, `queries:
  - id: neg
    query: "nothing"
    type: negative
    negative:
      top1_score_below: 0.4
`)
	if _, err := LoadDataset(path); err != nil {
		t.Fatalf("LoadDataset() unexpected error: %v", err)
	}
}
