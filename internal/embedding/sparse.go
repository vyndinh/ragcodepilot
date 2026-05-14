package embedding

import (
	"cmp"
	"fmt"
	"hash/crc32"
	"math"
	"slices"
	"strings"
	"unicode"
)

// SparseVector represents a sparse vector as parallel index/value arrays.
// Indices are CRC32 hashes of tokens; values are TF-IDF weights.
type SparseVector struct {
	Indices []uint32
	Values  []float32
}

// NewSparseVector creates a SparseVector after validating that indices and
// values have the same length.
func NewSparseVector(indices []uint32, values []float32) (SparseVector, error) {
	if len(indices) != len(values) {
		return SparseVector{}, fmt.Errorf("sparse vector length mismatch: %d indices vs %d values", len(indices), len(values))
	}
	return SparseVector{Indices: indices, Values: values}, nil
}

// --- Stop words ---------------------------------------------------------

// stopWords contains Go keywords and common English stop words that carry
// little retrieval signal. Kept small and code-oriented.
var stopWords = map[string]struct{}{
	// English stop words
	"a": {}, "an": {}, "the": {}, "and": {}, "or": {}, "not": {},
	"is": {}, "are": {}, "was": {}, "were": {}, "be": {}, "been": {},
	"in": {}, "on": {}, "at": {}, "to": {}, "for": {}, "of": {}, "with": {},
	"it": {}, "this": {}, "that": {}, "from": {}, "by": {}, "as": {},
	"if": {}, "else": {}, "do": {}, "does": {}, "did": {},
	"has": {}, "have": {}, "had": {},
	"no": {}, "so": {}, "but": {}, "up": {},
	// Go keywords
	"func": {}, "package": {}, "import": {}, "return": {}, "var": {},
	"const": {}, "type": {}, "struct": {}, "interface": {}, "map": {},
	"chan": {}, "go": {}, "defer": {}, "select": {}, "case": {},
	"switch": {}, "default": {}, "break": {}, "continue": {},
	"fallthrough": {}, "range": {}, "nil": {},
}

// --- Tokenizer ----------------------------------------------------------

// Tokenize is the canonical tokenizer used by both index-time and query-time
// sparse vector generation. It splits text into normalized tokens using
// code-aware rules:
//
//   - Split on whitespace and punctuation.
//   - Sub-split camelCase: "ChunkFile" → ["chunk", "file"].
//   - Sub-split snake_case: "chunk_file" → ["chunk", "file"].
//   - Keep digit runs attached: "sha256Hash" → ["sha256", "hash"].
//   - Lowercase all tokens.
//   - Remove stop words (Go keywords + common English).
//
// This function is the single source of truth — BuildSparseVectors and
// TokenizeQuery both call it internally.
func Tokenize(text string) []string {
	// Step 1: Split on whitespace and punctuation into raw words.
	rawWords := splitOnBoundaries(text)

	// Step 2: Sub-split each word on camelCase / snake_case boundaries.
	var tokens []string
	for _, word := range rawWords {
		parts := splitCamelSnake(word)
		for _, part := range parts {
			lower := strings.ToLower(part)
			if lower == "" {
				continue
			}
			if _, stop := stopWords[lower]; stop {
				continue
			}
			tokens = append(tokens, lower)
		}
	}
	return tokens
}

// splitOnBoundaries splits text on any character that is not a letter, digit,
// or underscore. Underscores are kept so splitCamelSnake can handle snake_case.
func splitOnBoundaries(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})
}

// splitCamelSnake splits a single word on camelCase transitions and
// underscores. Digit runs stay attached to their left context when they appear
// inside a word (e.g. "sha256Hash" → ["sha256", "hash"]).
func splitCamelSnake(word string) []string {
	// First, split on underscores.
	underscoreParts := strings.Split(word, "_")

	var result []string
	for _, part := range underscoreParts {
		if part == "" {
			continue
		}
		result = append(result, splitCamel(part)...)
	}
	return result
}

// splitCamel splits a single word (no underscores) on camelCase boundaries.
//
// Rules:
//   - A transition from lowercase/digit to uppercase starts a new token.
//   - A run of uppercase letters followed by a lowercase letter splits before
//     the last uppercase (e.g. "HTTPClient" → ["HTTP", "Client"]).
//   - Digit runs stay attached to the preceding letters
//     (e.g. "sha256" stays as one token, but "sha256Hash" → ["sha256", "Hash"]).
func splitCamel(s string) []string {
	if s == "" {
		return nil
	}

	runes := []rune(s)
	var parts []string
	start := 0

	for i := 1; i < len(runes); i++ {
		prev := runes[i-1]
		cur := runes[i]

		splitAt := -1 // -1 means no split

		// lowercase/digit → uppercase: "chunkF" splits before F.
		if (unicode.IsLower(prev) || unicode.IsDigit(prev)) && unicode.IsUpper(cur) {
			splitAt = i
		}

		// Uppercase run followed by lowercase: "HTTPClient" splits before 'C'.
		// We need at least two uppercase letters before the current lowercase.
		// Split at i-1 (before the last uppercase that starts the new word).
		if splitAt == -1 && i >= 2 && unicode.IsUpper(prev) && unicode.IsUpper(runes[i-2]) && unicode.IsLower(cur) {
			splitAt = i - 1
		}

		// digit → letter transition: "sha256Hash" splits before 'H'.
		if splitAt == -1 && unicode.IsDigit(prev) && unicode.IsLetter(cur) {
			splitAt = i
		}

		if splitAt >= 0 && splitAt > start {
			parts = append(parts, string(runes[start:splitAt]))
			start = splitAt
		}
	}
	parts = append(parts, string(runes[start:]))

	return parts
}

// --- BM25 ---------------------------------------------------------------

// BM25 parameters. k1 is softened from the Elasticsearch default (1.2)
// because Round 1 eval showed that aggressive saturation pulled
// identifier-overlap chunks above semantically-closer chunks on a concept
// query. k1 = 0.5 keeps the saturation curve flatter, which behaves closer
// to TF-IDF when term frequencies are low — the typical case in short
// code chunks — while still capping pathological repeats.
const (
	bm25K1 = 0.5
	bm25B  = 0.75
)

// tokenHash returns the CRC32 hash of a token, used as the sparse vector
// index. CRC32 with ~1M unique tokens gives ~120 expected collisions
// (birthday paradox). Accepted for the MVP — see hybrid_search.md §1.
func tokenHash(token string) uint32 {
	return crc32.ChecksumIEEE([]byte(token))
}

// CorpusStats holds the per-corpus statistics needed to compute BM25 weights.
// Computed once per indexing run and shared across all batches.
type CorpusStats struct {
	IDF       map[string]float64 // BM25-smoothed inverse document frequency
	AvgDocLen float64            // average document length in tokens
}

// ComputeCorpusStats walks the corpus once, computing:
//   - document frequency df(t) for every token
//   - BM25-smoothed idf(t) = log((N - df(t) + 0.5) / (df(t) + 0.5) + 1)
//   - the average document length avgdl (in tokens)
//
// Compared to classic log(N/df), the BM25 smoothing keeps idf strictly
// positive even for tokens appearing in more than half the corpus, so common
// tokens still contribute (weakly) rather than dropping to zero.
//
// Returns the zero value if texts is empty.
func ComputeCorpusStats(texts []string) CorpusStats {
	n := len(texts)
	if n == 0 {
		return CorpusStats{}
	}

	df := make(map[string]int)
	totalDocLen := 0
	for _, text := range texts {
		tokens := Tokenize(text)
		totalDocLen += len(tokens)
		seen := make(map[string]struct{}, len(tokens))
		for _, tok := range tokens {
			if _, ok := seen[tok]; ok {
				continue
			}
			seen[tok] = struct{}{}
			df[tok]++
		}
	}

	nf := float64(n)
	idf := make(map[string]float64, len(df))
	for tok, count := range df {
		idf[tok] = math.Log((nf-float64(count)+0.5)/(float64(count)+0.5) + 1)
	}

	return CorpusStats{
		IDF:       idf,
		AvgDocLen: float64(totalDocLen) / nf,
	}
}

// BuildSparseVectors generates one SparseVector per input text using BM25
// weights. The stats should come from ComputeCorpusStats over the full
// corpus indexed in this run.
//
// For each document d and term t with raw count f = tf(t,d):
//
//	lenNorm = 1 - b + b * (|d| / avgdl)
//	weight  = idf(t) * (f * (k1 + 1)) / (f + k1 * lenNorm)
//
// The k1 term saturates the effect of repeated tokens (so client × 30 no
// longer dominates client × 5). The b term penalizes docs longer than the
// corpus average.
func BuildSparseVectors(texts []string, stats CorpusStats) []SparseVector {
	result := make([]SparseVector, len(texts))

	for i, text := range texts {
		tokens := Tokenize(text)
		if len(tokens) == 0 {
			result[i] = SparseVector{}
			continue
		}

		// Count term frequencies.
		tf := make(map[string]int)
		for _, tok := range tokens {
			tf[tok]++
		}

		// Length normalization. Falls back to 1.0 if the corpus is empty
		// (avgdl == 0) so we never divide by zero.
		docLen := float64(len(tokens))
		lenNorm := 1.0
		if stats.AvgDocLen > 0 {
			lenNorm = 1 - bm25B + bm25B*(docLen/stats.AvgDocLen)
		}

		// Build sparse vector keyed by hashed index. Different tokens that
		// hash to the same uint32 (CRC32 collision) naturally merge into one
		// dimension because the map key is the hash — duplicate writes use
		// += to sum weights. This makes the output's "one dimension per
		// unique hash" property a structural guarantee.
		hashWeights := make(map[uint32]float32, len(tf))

		for tok, count := range tf {
			idf, ok := stats.IDF[tok]
			if !ok || idf <= 0 {
				// Token not in IDF map (shouldn't happen if stats came from
				// the same corpus), or zero/negative IDF — skip.
				continue
			}
			f := float64(count)
			weight := idf * f * (bm25K1 + 1) / (f + bm25K1*lenNorm)
			if weight <= 0 {
				continue
			}
			hashWeights[tokenHash(tok)] += float32(weight)
		}

		// Sort by index for deterministic output (Go map iteration is random).
		type entry struct {
			idx uint32
			val float32
		}
		entries := make([]entry, 0, len(hashWeights))
		for idx, val := range hashWeights {
			entries = append(entries, entry{idx: idx, val: val})
		}
		slices.SortFunc(entries, func(a, b entry) int { return cmp.Compare(a.idx, b.idx) })

		indices := make([]uint32, len(entries))
		values := make([]float32, len(entries))
		for j, e := range entries {
			indices[j] = e.idx
			values[j] = e.val
		}

		result[i] = SparseVector{Indices: indices, Values: values}
	}
	return result
}

// TokenizeQuery generates a SparseVector for a search query using uniform
// weights (1.0 per unique token). Query-side does not need corpus IDF —
// the sparse index in Qdrant handles the matching.
//
// Uses the same Tokenize() function as index-time to guarantee token parity.
func TokenizeQuery(query string) SparseVector {
	tokens := Tokenize(query)
	if len(tokens) == 0 {
		return SparseVector{}
	}

	// Deduplicate: each unique token gets weight 1.0.
	// Output is in first-appearance order (deterministic via Tokenize's slice order).
	// Qdrant doesn't require sorted sparse indices, so no sort needed here.
	seen := make(map[uint32]struct{})
	var indices []uint32
	var values []float32

	for _, tok := range tokens {
		h := tokenHash(tok)
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		indices = append(indices, h)
		values = append(values, 1.0)
	}

	return SparseVector{Indices: indices, Values: values}
}
