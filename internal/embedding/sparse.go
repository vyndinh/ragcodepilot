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

// --- TF-IDF -------------------------------------------------------------

// tokenHash returns the CRC32 hash of a token, used as the sparse vector
// index. CRC32 with ~1M unique tokens gives ~120 expected collisions
// (birthday paradox). Accepted for the MVP — see hybrid_search.md §1.
func tokenHash(token string) uint32 {
	return crc32.ChecksumIEEE([]byte(token))
}

// ComputeIDF computes the inverse document frequency for each unique token
// across the full corpus. Call this once per indexing run with all chunk texts.
//
//	idf(t) = log(N / df(t))
//
// where N is the total number of documents and df(t) is the number of
// documents containing token t.
func ComputeIDF(texts []string) map[string]float64 {
	n := len(texts)
	if n == 0 {
		return nil
	}

	// Count document frequency for each token.
	df := make(map[string]int)
	for _, text := range texts {
		seen := make(map[string]struct{})
		for _, tok := range Tokenize(text) {
			if _, ok := seen[tok]; ok {
				continue
			}
			seen[tok] = struct{}{}
			df[tok]++
		}
	}

	// Compute IDF.
	idf := make(map[string]float64, len(df))
	nf := float64(n)
	for tok, count := range df {
		idf[tok] = math.Log(nf / float64(count))
	}
	return idf
}

// BuildSparseVectors generates one SparseVector per input text using TF-IDF
// weights. The idfMap should come from ComputeIDF over the full corpus.
//
// For each document:
//
//	tf(t,d) = count(t,d) / len(d)
//	weight(t,d) = tf(t,d) * idf(t)
func BuildSparseVectors(texts []string, idfMap map[string]float64) []SparseVector {
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

		// Build sparse vector: index = hash(token), value = TF * IDF.
		// Sort by index for deterministic output (Go map iteration is random).
		docLen := float64(len(tokens))

		type entry struct {
			idx uint32
			val float32
		}
		entries := make([]entry, 0, len(tf))

		for tok, count := range tf {
			idf, ok := idfMap[tok]
			if !ok {
				// Token not in IDF map (shouldn't happen if idfMap came from
				// the same corpus, but be defensive).
				continue
			}
			weight := (float64(count) / docLen) * idf
			if weight <= 0 {
				continue
			}
			entries = append(entries, entry{idx: tokenHash(tok), val: float32(weight)})
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
