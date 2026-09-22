package eval

import (
	"testing"

	"github.com/dinhvy/ragcodepilot/internal/search"
)

func TestScoreKindForMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		mode search.SearchMode
		want ScoreKind
	}{
		{search.SearchModeDense, ScoreKindCosine},
		{search.SearchModeSparse, ScoreKindBM25},
		{search.SearchModeHybrid, ScoreKindRRF},
		{"", ScoreKindRRF},
	}
	for _, tt := range tests {
		if got := ScoreKindForMode(tt.mode); got != tt.want {
			t.Errorf("ScoreKindForMode(%q) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestNegativeCeiling(t *testing.T) {
	t.Parallel()
	const yamlCosine float32 = 0.55

	tests := []struct {
		name          string
		kind          ScoreKind
		yamlThreshold float32
		want          float32
	}{
		{"dense uses yaml", ScoreKindCosine, yamlCosine, yamlCosine},
		{"dense default when unset", ScoreKindCosine, 0, DefaultDenseNegativeCeiling},
		{"hybrid ignores cosine yaml", ScoreKindRRF, yamlCosine, DefaultHybridNegativeCeiling},
		{"hybrid default when unset", ScoreKindRRF, 0, DefaultHybridNegativeCeiling},
		{"hybrid honors explicit rrf yaml", ScoreKindRRF, 0.025, 0.025},
		{"sparse ignores cosine yaml", ScoreKindBM25, yamlCosine, DefaultSparseNegativeCeiling},
		{"sparse default when unset", ScoreKindBM25, 0, DefaultSparseNegativeCeiling},
		{"sparse honors explicit bm25 yaml", ScoreKindBM25, 12, 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NegativeCeiling(tt.kind, tt.yamlThreshold)
			if got != tt.want {
				t.Errorf("NegativeCeiling(%q, %v) = %v, want %v", tt.kind, tt.yamlThreshold, got, tt.want)
			}
		})
	}
}

func TestNegativePasses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		nResults int
		top1     float32
		ceiling  float32
		want     bool
	}{
		{"no results always pass", 0, 0.9, 0.55, true},
		{"dense below ceiling", 5, 0.54, 0.55, true},
		{"dense at ceiling fails", 5, 0.55, 0.55, false},
		{"dense above ceiling", 5, 0.62, 0.55, false},
		// Hybrid RRF from baseline_v6: single-prefetch rank-1 ≈ 0.0167,
		// dual-prefetch agreement ≈ 0.033. Ceiling 0.02 splits them.
		{"hybrid single-list rank-1", 10, 0.016666668, DefaultHybridNegativeCeiling, true},
		{"hybrid dual-list agreement", 10, 0.033333335, DefaultHybridNegativeCeiling, false},
		{"hybrid oauth-like dual hit", 10, 0.030679155, DefaultHybridNegativeCeiling, false},
		{"sparse weak keyword hit", 10, 7.04, DefaultSparseNegativeCeiling, true},
		{"sparse strong false positive", 10, 27.52, DefaultSparseNegativeCeiling, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NegativePasses(tt.nResults, tt.top1, tt.ceiling)
			if got != tt.want {
				t.Errorf("NegativePasses(%d, %v, %v) = %v, want %v",
					tt.nResults, tt.top1, tt.ceiling, got, tt.want)
			}
		})
	}
}

func TestNegativeCeilingMakesHybridYAMLFailAble(t *testing.T) {
	t.Parallel()
	// The I1 bug: comparing RRF ~0.03 to YAML 0.55 always passed.
	yaml := float32(0.55)
	rrfTop1 := float32(0.030679155)
	if !NegativePasses(10, rrfTop1, yaml) {
		t.Fatal("precondition: raw YAML 0.55 still cannot fail an RRF top-1")
	}
	ceiling := NegativeCeiling(ScoreKindRRF, yaml)
	if NegativePasses(10, rrfTop1, ceiling) {
		t.Fatalf("mode-calibrated RRF ceiling %v should fail dual-list top-1 %v", ceiling, rrfTop1)
	}
}
