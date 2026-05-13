// Package eval provides offline evaluation of retrieval quality against a
// golden query dataset. It computes standard IR metrics (hit@k, MRR@k,
// recall@k) plus per-stage latency percentiles, and produces a JSON or
// human-readable report.
package eval

import (
	"slices"
	"time"

	"github.com/dinhvy/ragcodepilot/internal/model"
)

// Expected describes the ground truth for a single golden query.
type Expected struct {
	// Files lists relative file paths that should appear in the top results.
	Files []string

	// Symbols lists function/symbol names that should appear in the top results.
	Symbols []string
}

// HasAny reports whether at least one expected file or symbol is defined.
func (e Expected) HasAny() bool {
	return len(e.Files) > 0 || len(e.Symbols) > 0
}

// IsRelevant reports whether a single search result matches any expected
// file or symbol. A match on either dimension counts as relevant.
func IsRelevant(r model.SearchResult, expected Expected) bool {
	if slices.Contains(expected.Files, r.Chunk.FilePath) {
		return true
	}
	if slices.Contains(expected.Symbols, r.Chunk.Name) {
		return true
	}
	return false
}

// HitAtK reports whether at least one of the first k results is relevant.
// k=0 always returns false.
func HitAtK(results []model.SearchResult, expected Expected, k int) bool {
	limit := min(k, len(results))
	for i := range limit {
		if IsRelevant(results[i], expected) {
			return true
		}
	}
	return false
}

// MRRAtK returns the reciprocal rank of the first relevant result within the
// top k, or 0 if none are relevant. Ranks are 1-based.
func MRRAtK(results []model.SearchResult, expected Expected, k int) float64 {
	limit := min(k, len(results))
	for i := range limit {
		if IsRelevant(results[i], expected) {
			return 1.0 / float64(i+1)
		}
	}
	return 0.0
}

// RecallAtK returns the fraction of expected files that appear in the top k
// results. Symbols are not counted toward recall — they're typically nested
// within an expected file, and recall is defined over the file set.
//
// Returns 0 when there are no expected files (recall is undefined).
func RecallAtK(results []model.SearchResult, expected Expected, k int) float64 {
	if len(expected.Files) == 0 {
		return 0.0
	}

	limit := min(k, len(results))

	matched := make(map[string]struct{}, len(expected.Files))
	for i := range limit {
		fp := results[i].Chunk.FilePath
		if slices.Contains(expected.Files, fp) {
			matched[fp] = struct{}{}
		}
	}
	return float64(len(matched)) / float64(len(expected.Files))
}

// Percentile returns the value at the given percentile (0-100) of the input
// durations using the nearest-rank method. An empty input returns 0.
func Percentile(durations []time.Duration, p float64) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	slices.Sort(sorted)

	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}

	rank := int((p/100.0)*float64(len(sorted)) + 0.999999)
	rank = max(rank, 1)
	rank = min(rank, len(sorted))
	return sorted[rank-1]
}
