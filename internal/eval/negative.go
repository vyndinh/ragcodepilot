package eval

import "github.com/dinhvy/ragcodepilot/internal/search"

// ScoreKind names the family of the top-1 retrieval score. Negative-pass
// ceilings are not interchangeable across kinds: cosine is ~0–1, RRF is
// ~0.016–0.033, and BM25 is unbounded.
type ScoreKind string

const (
	// ScoreKindCosine is dense-mode cosine similarity.
	ScoreKindCosine ScoreKind = "cosine"
	// ScoreKindRRF is hybrid-mode Reciprocal Rank Fusion (Qdrant k=60).
	ScoreKindRRF ScoreKind = "rrf"
	// ScoreKindBM25 is sparse-mode BM25.
	ScoreKindBM25 ScoreKind = "bm25"
)

// Default ceilings per score kind. Dataset top1_score_below is the dense
// cosine ceiling (historically 0.55). Applying that number to RRF or BM25
// makes negative_pass_rate vacuous — RRF never reaches 0.55, BM25 almost
// always does — so hybrid and sparse use these instead when the YAML value
// is cosine-calibrated.
const (
	// DefaultDenseNegativeCeiling is the cosine top-1 a negative query must
	// stay below. Calibrated on nomic-embed-text: out-of-scope queries land
	// ~0.45–0.54, in-scope positives ~0.60–0.75.
	DefaultDenseNegativeCeiling float32 = 0.55

	// DefaultHybridNegativeCeiling sits between a single-prefetch RRF rank-1
	// (~1/60 ≈ 0.0167) and dual-prefetch agreement (~0.033). A negative fails
	// only when both dense and sparse ranked the same point near the top.
	DefaultHybridNegativeCeiling float32 = 0.02

	// DefaultSparseNegativeCeiling is a BM25 top-1 a negative must stay below.
	// BM25 is unbounded; 0.55 is not a BM25 score. 10 sits between weak true
	// hits on this corpus (~7–9) and strong keyword false positives (~27 on
	// oauth_middleware_negative in baseline_v4_sparse).
	DefaultSparseNegativeCeiling float32 = 10
)

// cosineCalibratedFloor is the smallest YAML top1_score_below that is treated
// as a cosine ceiling rather than an RRF/BM25 override. 0.55 and similar
// values from golden.yaml fall here; an explicit 0.02 (RRF) or 12 (BM25)
// does not.
const cosineCalibratedFloor float32 = 0.1

// ScoreKindForMode maps a search mode to the score family its top-1 uses.
func ScoreKindForMode(mode search.SearchMode) ScoreKind {
	switch mode {
	case search.SearchModeDense:
		return ScoreKindCosine
	case search.SearchModeSparse:
		return ScoreKindBM25
	default:
		return ScoreKindRRF
	}
}

// NegativeCeiling is the top-1 score a negative query must stay strictly
// below to pass. yamlThreshold is the dataset's top1_score_below (0 = unset).
//
// Dense uses the YAML value when set, otherwise DefaultDenseNegativeCeiling.
// Hybrid and sparse ignore a cosine-calibrated YAML value (>= 0.1) so a
// leftover 0.55 cannot silence the check; a smaller (RRF) or larger (BM25)
// YAML value is taken as an explicit override.
func NegativeCeiling(kind ScoreKind, yamlThreshold float32) float32 {
	switch kind {
	case ScoreKindCosine:
		if yamlThreshold > 0 {
			return yamlThreshold
		}
		return DefaultDenseNegativeCeiling
	case ScoreKindRRF:
		if yamlThreshold > 0 && yamlThreshold < cosineCalibratedFloor {
			return yamlThreshold
		}
		return DefaultHybridNegativeCeiling
	case ScoreKindBM25:
		if yamlThreshold >= 1 {
			return yamlThreshold
		}
		return DefaultSparseNegativeCeiling
	default:
		return DefaultDenseNegativeCeiling
	}
}

// NegativePasses reports whether a negative query passed: no results, or
// top-1 score strictly below the mode-calibrated ceiling.
func NegativePasses(nResults int, top1, ceiling float32) bool {
	return nResults == 0 || top1 < ceiling
}
