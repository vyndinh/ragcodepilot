package eval

import (
	"testing"
	"time"

	"github.com/dinhvy/ragcodepilot/internal/model"
)

func makeResults(rows ...string) []model.SearchResult {
	results := make([]model.SearchResult, 0, len(rows)/2)
	for i := 0; i < len(rows); i += 2 {
		results = append(results, model.SearchResult{
			Chunk: model.CodeChunk{FilePath: rows[i], Name: rows[i+1]},
			Score: 1.0,
		})
	}
	return results
}

func TestIsRelevantMatchesFile(t *testing.T) {
	t.Parallel()
	r := model.SearchResult{Chunk: model.CodeChunk{FilePath: "internal/foo.go", Name: "Bar"}}
	if !IsRelevant(r, Expected{Files: []string{"internal/foo.go"}}) {
		t.Fatal("expected file match to be relevant")
	}
}

func TestIsRelevantMatchesSymbol(t *testing.T) {
	t.Parallel()
	r := model.SearchResult{Chunk: model.CodeChunk{FilePath: "internal/other.go", Name: "Bar"}}
	if !IsRelevant(r, Expected{Symbols: []string{"Bar"}}) {
		t.Fatal("expected symbol match to be relevant")
	}
}

func TestIsRelevantNoMatch(t *testing.T) {
	t.Parallel()
	r := model.SearchResult{Chunk: model.CodeChunk{FilePath: "x.go", Name: "X"}}
	if IsRelevant(r, Expected{Files: []string{"y.go"}, Symbols: []string{"Y"}}) {
		t.Fatal("expected no match to be irrelevant")
	}
}

func TestHitAtK(t *testing.T) {
	t.Parallel()
	results := makeResults(
		"a.go", "A",
		"b.go", "B",
		"c.go", "C",
		"d.go", "D",
		"e.go", "E",
	)
	expected := Expected{Files: []string{"c.go"}}

	tests := []struct {
		k    int
		want bool
	}{
		{k: 0, want: false},
		{k: 1, want: false},
		{k: 2, want: false},
		{k: 3, want: true},
		{k: 5, want: true},
		{k: 100, want: true}, // k larger than result count
	}
	for _, tt := range tests {
		if got := HitAtK(results, expected, tt.k); got != tt.want {
			t.Errorf("HitAtK(k=%d) = %v, want %v", tt.k, got, tt.want)
		}
	}
}

func TestHitAtKNoMatch(t *testing.T) {
	t.Parallel()
	results := makeResults("a.go", "A", "b.go", "B")
	expected := Expected{Files: []string{"z.go"}}
	if HitAtK(results, expected, 5) {
		t.Fatal("expected no hit when no result matches")
	}
}

func TestMRRAtK(t *testing.T) {
	t.Parallel()
	results := makeResults(
		"a.go", "A",
		"b.go", "B",
		"c.go", "C",
	)

	tests := []struct {
		name     string
		expected Expected
		k        int
		want     float64
	}{
		{"first position", Expected{Files: []string{"a.go"}}, 5, 1.0},
		{"second position", Expected{Files: []string{"b.go"}}, 5, 0.5},
		{"third position", Expected{Files: []string{"c.go"}}, 5, 1.0 / 3.0},
		{"not in top k", Expected{Files: []string{"c.go"}}, 2, 0.0},
		{"no match", Expected{Files: []string{"z.go"}}, 5, 0.0},
		{"symbol match", Expected{Symbols: []string{"B"}}, 5, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MRRAtK(results, tt.expected, tt.k)
			if got != tt.want {
				t.Errorf("MRRAtK = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecallAtK(t *testing.T) {
	t.Parallel()
	results := makeResults(
		"a.go", "A",
		"b.go", "B",
		"c.go", "C",
		"d.go", "D",
	)

	tests := []struct {
		name     string
		expected Expected
		k        int
		want     float64
	}{
		{"all expected in top k", Expected{Files: []string{"a.go", "b.go"}}, 5, 1.0},
		{"half in top k", Expected{Files: []string{"a.go", "z.go"}}, 5, 0.5},
		{"none in top k", Expected{Files: []string{"y.go", "z.go"}}, 5, 0.0},
		{"limited k", Expected{Files: []string{"a.go", "c.go"}}, 2, 0.5},
		{"no expected files", Expected{Symbols: []string{"A"}}, 5, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RecallAtK(results, tt.expected, tt.k)
			if got != tt.want {
				t.Errorf("RecallAtK = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecallAtKDuplicateMatchesCountOnce(t *testing.T) {
	t.Parallel()
	// Same file appearing in multiple results should not inflate recall.
	results := makeResults(
		"a.go", "Foo",
		"a.go", "Bar",
		"b.go", "Baz",
	)
	expected := Expected{Files: []string{"a.go", "b.go"}}
	got := RecallAtK(results, expected, 5)
	if got != 1.0 {
		t.Errorf("RecallAtK with duplicate file = %v, want 1.0", got)
	}
}

func TestPercentile(t *testing.T) {
	t.Parallel()
	durations := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
		60 * time.Millisecond,
		70 * time.Millisecond,
		80 * time.Millisecond,
		90 * time.Millisecond,
		100 * time.Millisecond,
	}

	tests := []struct {
		p    float64
		want time.Duration
	}{
		{50, 50 * time.Millisecond},
		{95, 100 * time.Millisecond},
		{100, 100 * time.Millisecond},
		{0, 10 * time.Millisecond},
	}
	for _, tt := range tests {
		got := Percentile(durations, tt.p)
		if got != tt.want {
			t.Errorf("Percentile(p=%v) = %v, want %v", tt.p, got, tt.want)
		}
	}
}

func TestPercentileEmpty(t *testing.T) {
	t.Parallel()
	if got := Percentile(nil, 50); got != 0 {
		t.Errorf("Percentile(nil) = %v, want 0", got)
	}
}
