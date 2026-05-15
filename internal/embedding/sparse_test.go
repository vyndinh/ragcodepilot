package embedding

import (
	"math"
	"slices"
	"testing"
)

// --- Tokenize tests -----------------------------------------------------

func TestTokenize_CamelCase(t *testing.T) {
	got := Tokenize("ChunkFile")
	want := []string{"chunk", "file"}
	assertTokens(t, got, want)
}

func TestTokenize_SnakeCase(t *testing.T) {
	got := Tokenize("chunk_file")
	want := []string{"chunk", "file"}
	assertTokens(t, got, want)
}

func TestTokenize_MixedCamelCase(t *testing.T) {
	got := Tokenize("NewVectorInputSparse")
	want := []string{"new", "vector", "input", "sparse", "spars"}
	assertTokens(t, got, want)
}

func TestTokenize_NumbersAttached(t *testing.T) {
	got := Tokenize("sha256Hash")
	want := []string{"sha256", "hash"}
	assertTokens(t, got, want)
}

func TestTokenize_UppercaseAcronym(t *testing.T) {
	got := Tokenize("HTTPClient")
	want := []string{"http", "client"}
	assertTokens(t, got, want)
}

func TestTokenize_StopWordRemoval(t *testing.T) {
	got := Tokenize("the func for return")
	// All are stop words — should produce nothing.
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestTokenize_MixedContent(t *testing.T) {
	got := Tokenize("// EnsureCollection creates a collection if it doesn't exist.")
	// Should contain meaningful tokens, not stop words.
	assertContains(t, got, "ensure")
	assertContains(t, got, "collection")
	assertContains(t, got, "creates")
	assertContains(t, got, "creat") // additive stemming (Snowball: creates → creat)
	assertContains(t, got, "exist")
	assertNotContains(t, got, "a")
	assertNotContains(t, got, "if")
	assertNotContains(t, got, "it")
}

func TestTokenize_Punctuation(t *testing.T) {
	got := Tokenize("foo.bar(baz, qux)")
	want := []string{"foo", "bar", "baz", "qux"}
	assertTokens(t, got, want)
}

func TestTokenize_Empty(t *testing.T) {
	got := Tokenize("")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestTokenize_WhitespaceOnly(t *testing.T) {
	got := Tokenize("   \t\n  ")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// --- Additive stemming tests --------------------------------------------

func TestTokenize_AdditiveStem_Hashes(t *testing.T) {
	// This is the specific bug: query "hashes" must also emit "hash"
	// so it matches index-time token "hash" from "HashFiles".
	got := Tokenize("hashes")
	assertContains(t, got, "hashes") // original kept
	assertContains(t, got, "hash")   // stemmed variant added
}

func TestTokenize_AdditiveStem_Files(t *testing.T) {
	got := Tokenize("files")
	assertContains(t, got, "files")
	assertContains(t, got, "file")
}

func TestTokenize_AdditiveStem_NoChange(t *testing.T) {
	// "chunk" is already a root — should emit only "chunk".
	got := Tokenize("chunk")
	assertTokens(t, got, []string{"chunk"})
}

func TestTokenize_AdditiveStem_Computing(t *testing.T) {
	// Snowball handles -ing: "computing" → "comput".
	// This goes beyond plurals — it's the main advantage over custom rules.
	got := Tokenize("computing")
	assertContains(t, got, "computing")
	assertContains(t, got, "comput")
}

func TestTokenize_AdditiveStem_Entries(t *testing.T) {
	got := Tokenize("entries")
	assertContains(t, got, "entries")
	// Snowball stems "entries" to "entri", not "entry".
	// Ugly, but consistent at index-time and query-time.
	assertContains(t, got, "entri")
}

// --- stemToken unit tests -----------------------------------------------

func TestStemToken(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		// Plurals
		{"hashes", "hash"},
		{"files", "file"},
		{"chunks", "chunk"},
		{"vectors", "vector"},
		{"results", "result"},
		{"classes", "class"},
		{"indexes", "index"},
		// -ing forms
		{"computing", "comput"},
		{"running", "run"},
		{"indexing", "index"},
		// -ed forms
		{"computed", "comput"},
		{"hashed", "hash"},
		// -tion forms
		{"collection", "collect"},
		// No change expected
		{"hash", "hash"},
		{"chunk", "chunk"},
		{"class", "class"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := stemToken(tt.in)
			if got != tt.want {
				t.Errorf("stemToken(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// --- splitCamel tests ---------------------------------------------------

func TestSplitCamel_Simple(t *testing.T) {
	assertTokens(t, splitCamel("ChunkFile"), []string{"Chunk", "File"})
}

func TestSplitCamel_Acronym(t *testing.T) {
	assertTokens(t, splitCamel("HTTPClient"), []string{"HTTP", "Client"})
}

func TestSplitCamel_DigitsMiddle(t *testing.T) {
	assertTokens(t, splitCamel("sha256Hash"), []string{"sha256", "Hash"})
}

func TestSplitCamel_AllLower(t *testing.T) {
	assertTokens(t, splitCamel("chunker"), []string{"chunker"})
}

func TestSplitCamel_AllUpper(t *testing.T) {
	assertTokens(t, splitCamel("HTTP"), []string{"HTTP"})
}

func TestSplitCamel_Empty(t *testing.T) {
	got := splitCamel("")
	if len(got) != 0 {
		t.Errorf("expected nil, got %v", got)
	}
}

// --- SparseVector constructor tests -------------------------------------

func TestNewSparseVector_Valid(t *testing.T) {
	sv, err := NewSparseVector([]uint32{1, 2, 3}, []float32{0.5, 0.3, 0.2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sv.Indices) != 3 || len(sv.Values) != 3 {
		t.Errorf("expected 3 entries, got indices=%d values=%d", len(sv.Indices), len(sv.Values))
	}
}

func TestNewSparseVector_Mismatch(t *testing.T) {
	_, err := NewSparseVector([]uint32{1, 2}, []float32{0.5})
	if err == nil {
		t.Fatal("expected error for mismatched lengths")
	}
}

func TestNewSparseVector_Empty(t *testing.T) {
	sv, err := NewSparseVector(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sv.Indices) != 0 || len(sv.Values) != 0 {
		t.Error("expected empty sparse vector")
	}
}

// --- ComputeCorpusStats tests -------------------------------------------

func TestComputeCorpusStats_Basic(t *testing.T) {
	// Use synthetic non-stop-word tokens so the math is hand-checkable.
	texts := []string{
		"alpha beta",
		"alpha gamma",
		"beta delta",
	}

	stats := ComputeCorpusStats(texts)

	// BM25-smoothed IDF: log((N - df + 0.5) / (df + 0.5) + 1).
	// alpha and beta each appear in 2/3 docs → log(1.5/2.5 + 1) = log(1.6).
	// delta appears in 1/3 docs → log(2.5/1.5 + 1) = log(8/3).
	expectedCommon := math.Log(1.6)
	if diff := math.Abs(stats.IDF["alpha"] - expectedCommon); diff > 0.001 {
		t.Errorf("idf[alpha] = %.4f, want ~%.4f", stats.IDF["alpha"], expectedCommon)
	}

	expectedRare := math.Log(2.5/1.5 + 1)
	if diff := math.Abs(stats.IDF["delta"] - expectedRare); diff > 0.001 {
		t.Errorf("idf[delta] = %.4f, want ~%.4f", stats.IDF["delta"], expectedRare)
	}

	// Each doc is 2 tokens; avgdl = 2.0.
	if math.Abs(stats.AvgDocLen-2.0) > 0.001 {
		t.Errorf("avgdl = %.4f, want 2.0", stats.AvgDocLen)
	}
}

func TestComputeCorpusStats_Empty(t *testing.T) {
	stats := ComputeCorpusStats(nil)
	if stats.IDF != nil {
		t.Errorf("expected nil IDF, got %v", stats.IDF)
	}
	if stats.AvgDocLen != 0 {
		t.Errorf("expected avgdl=0, got %.4f", stats.AvgDocLen)
	}
}

func TestComputeCorpusStats_TokenInAllDocs(t *testing.T) {
	texts := []string{"hello world", "hello earth", "hello mars"}
	stats := ComputeCorpusStats(texts)

	// BM25-smoothed IDF is strictly positive even for tokens in every doc:
	// log((N - df + 0.5) / (df + 0.5) + 1) > 0 because the argument is > 1.
	// This is the key reason to prefer BM25's IDF over classic log(N/df),
	// which would give 0 here and silently drop the term.
	if stats.IDF["hello"] <= 0 {
		t.Errorf("BM25 IDF should be strictly positive for an all-docs token; got %.4f", stats.IDF["hello"])
	}

	// And it should still be the lowest IDF in the corpus — rarer tokens
	// (world, earth, mars) each appear in only 1/3 docs and should weigh more.
	if stats.IDF["hello"] >= stats.IDF["world"] {
		t.Errorf("IDF should be monotonic in 1/df: hello (%.4f) should be less than world (%.4f)",
			stats.IDF["hello"], stats.IDF["world"])
	}
}

func TestComputeCorpusStats_StemExpansionDoesNotInflateDocLength(t *testing.T) {
	texts := []string{"hashes", "hash"}

	stats := ComputeCorpusStats(texts)

	if math.Abs(stats.AvgDocLen-1.0) > 0.001 {
		t.Fatalf("avgdl = %.4f, want 1.0 from original tokens only", stats.AvgDocLen)
	}
	if _, ok := stats.IDF["hashes"]; !ok {
		t.Fatal("expected original token IDF for hashes")
	}
	if _, ok := stats.IDF["hash"]; !ok {
		t.Fatal("expected stemmed token IDF for hash")
	}
}

// --- BuildSparseVectors tests -------------------------------------------

func TestBuildSparseVectors_ProducesNonEmpty(t *testing.T) {
	texts := []string{"ChunkFile parser", "writer handler"}
	stats := ComputeCorpusStats(texts)
	vectors := BuildSparseVectors(texts, stats)

	if len(vectors) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vectors))
	}

	for i, sv := range vectors {
		if len(sv.Indices) == 0 {
			t.Errorf("vector %d has no indices", i)
		}
		if len(sv.Indices) != len(sv.Values) {
			t.Errorf("vector %d: indices/values length mismatch: %d vs %d",
				i, len(sv.Indices), len(sv.Values))
		}
		for j, v := range sv.Values {
			if v <= 0 {
				t.Errorf("vector %d, entry %d: non-positive weight %.4f", i, j, v)
			}
		}
	}
}

func TestBuildSparseVectors_EmptyText(t *testing.T) {
	stats := ComputeCorpusStats([]string{"hello world"})
	vectors := BuildSparseVectors([]string{""}, stats)
	if len(vectors) != 1 {
		t.Fatalf("expected 1 vector, got %d", len(vectors))
	}
	if len(vectors[0].Indices) != 0 {
		t.Error("expected empty sparse vector for empty text")
	}
}

func TestBuildSparseVectors_UniqueIndices(t *testing.T) {
	// Structural invariant: the output sparse vector must never contain
	// duplicate indices. Any CRC32 collision between two distinct tokens
	// should be merged into a single dimension via weight summing.
	//
	// We don't manufacture a real CRC32 collision (rare; brute-forcing one
	// is wasteful) — instead we verify the invariant across varied input.
	// The accumulator's map[uint32]float32 keying makes the property a
	// structural guarantee; this test catches future regressions that would
	// re-introduce duplicate emission.
	texts := []string{
		"ChunkFile parser writer reader",
		"EnsureCollection deleteByFilePaths search",
		"sha256Hash crc32Checksum xxhash",
	}
	stats := ComputeCorpusStats(texts)
	vectors := BuildSparseVectors(texts, stats)

	for i, sv := range vectors {
		seen := make(map[uint32]bool, len(sv.Indices))
		for _, idx := range sv.Indices {
			if seen[idx] {
				t.Errorf("vector %d contains duplicate index %d", i, idx)
			}
			seen[idx] = true
		}
	}
}

func TestBuildSparseVectors_AllStopWords(t *testing.T) {
	texts := []string{"the func for return"}
	stats := ComputeCorpusStats(texts)
	vectors := BuildSparseVectors(texts, stats)
	if len(vectors[0].Indices) != 0 {
		t.Error("expected empty sparse vector for all-stop-word text")
	}
}

func TestBuildSparseVectors_Deterministic(t *testing.T) {
	texts := []string{"ChunkFile parser writer handler reader"}
	stats := ComputeCorpusStats(append(texts, "unrelated filler noise padding"))

	first := BuildSparseVectors(texts, stats)
	for i := 0; i < 20; i++ {
		again := BuildSparseVectors(texts, stats)
		if len(again[0].Indices) != len(first[0].Indices) {
			t.Fatalf("run %d: indices length %d != %d", i, len(again[0].Indices), len(first[0].Indices))
		}
		for j := range first[0].Indices {
			if again[0].Indices[j] != first[0].Indices[j] {
				t.Fatalf("run %d, index %d: got %d, want %d", i, j, again[0].Indices[j], first[0].Indices[j])
			}
			if again[0].Values[j] != first[0].Values[j] {
				t.Fatalf("run %d, value %d: got %f, want %f", i, j, again[0].Values[j], first[0].Values[j])
			}
		}
	}
}

// --- BM25-specific behavior tests --------------------------------------

// TestBM25_TermFrequencySaturation verifies that BM25's k1 parameter
// damps the effect of repeated tokens. The "high" doc contains "foo"
// five times in a five-token doc; the "low" doc contains it once in a
// five-token doc. Same length, so length normalization is identical and
// the only variable is term frequency.
//
// Under TF-IDF (the previous formula tf=count/docLen × idf), the ratio
// of the two weights would be exactly 5.0. BM25 with k1=1.2 saturates
// the curve, so the ratio drops well below 3.0 while staying above 1.0
// (the higher tf must still win).
func TestBM25_TermFrequencySaturation(t *testing.T) {
	high := "foo foo foo foo foo"
	low := "foo bar baz qux quux"
	other := "unrelated filler text padding"

	texts := []string{high, low, other}
	stats := ComputeCorpusStats(texts)
	vectors := BuildSparseVectors(texts, stats)

	fooHash := tokenHash("foo")
	weightOf := func(v SparseVector, h uint32) float32 {
		for i, idx := range v.Indices {
			if idx == h {
				return v.Values[i]
			}
		}
		return 0
	}

	highW := weightOf(vectors[0], fooHash)
	lowW := weightOf(vectors[1], fooHash)
	if highW == 0 || lowW == 0 {
		t.Fatalf("expected non-zero foo weights, got high=%.4f low=%.4f", highW, lowW)
	}

	ratio := highW / lowW
	if ratio >= 3.0 {
		t.Errorf("BM25 saturation insufficient: ratio = %.2f, want < 3.0 (pure TF-IDF would give ~5.0)", ratio)
	}
	if ratio <= 1.0 {
		t.Errorf("BM25 lost the tf signal entirely: ratio = %.2f, want > 1.0 (higher tf should still win)", ratio)
	}
}

// TestBM25_LengthNormalization verifies that BM25's b parameter penalizes
// documents longer than the corpus average. The same token appearing once
// in a short doc must outweigh the same token appearing once in a long doc.
//
// Under the previous TF-IDF formula (which divided by per-doc length), the
// two weights would be unrelated to corpus context. Under BM25 the longer
// doc gets a bigger lenNorm denominator and a smaller final weight.
func TestBM25_LengthNormalization(t *testing.T) {
	short := "foo bar"
	long := "foo alpha beta gamma delta epsilon zeta eta theta iota " +
		"kappa lambda mu nu xi omicron pi rho sigma tau upsilon"
	other := "completely separate filler content"

	texts := []string{short, long, other}
	stats := ComputeCorpusStats(texts)
	vectors := BuildSparseVectors(texts, stats)

	fooHash := tokenHash("foo")
	weightOf := func(v SparseVector, h uint32) float32 {
		for i, idx := range v.Indices {
			if idx == h {
				return v.Values[i]
			}
		}
		return 0
	}

	shortW := weightOf(vectors[0], fooHash)
	longW := weightOf(vectors[1], fooHash)
	if shortW == 0 || longW == 0 {
		t.Fatalf("expected non-zero foo weights, got short=%.4f long=%.4f", shortW, longW)
	}

	if shortW <= longW {
		t.Errorf("length penalty failed: short=%.4f, long=%.4f (short doc must outweigh long doc for the same token)", shortW, longW)
	}
}

// --- TokenizeQuery tests ------------------------------------------------

func TestTokenizeQuery_ProducesCorrectTokens(t *testing.T) {
	sv := TokenizeQuery("ChunkFile parser")

	if len(sv.Indices) == 0 {
		t.Fatal("expected non-empty sparse vector")
	}
	if len(sv.Indices) != len(sv.Values) {
		t.Fatalf("length mismatch: %d indices vs %d values", len(sv.Indices), len(sv.Values))
	}
	for _, v := range sv.Values {
		if v != 1.0 {
			t.Errorf("expected uniform weight 1.0, got %.4f", v)
		}
	}
}

func TestTokenizeQuery_DedupesTokens(t *testing.T) {
	sv := TokenizeQuery("chunk chunk chunk")

	// "chunk" appears 3 times but should produce only one index.
	if len(sv.Indices) != 1 {
		t.Errorf("expected 1 unique index, got %d", len(sv.Indices))
	}
}

func TestTokenizeQuery_Empty(t *testing.T) {
	sv := TokenizeQuery("")
	if len(sv.Indices) != 0 {
		t.Error("expected empty sparse vector for empty query")
	}
}

// --- Index/query parity test --------------------------------------------

func TestTokenize_IndexQueryParity(t *testing.T) {
	// The same input must produce the same tokens whether called from
	// Tokenize directly, BuildSparseVectors, or TokenizeQuery.
	inputs := []string{
		"ChunkFile",
		"EnsureCollection",
		"sha256Hash",
		"chunk_file_parser",
		"HTTPClient",
	}

	for _, input := range inputs {
		canonical := Tokenize(input)

		// BuildSparseVectors uses Tokenize internally — verify by checking
		// that the number of unique tokens matches the sparse vector length.
		stats := ComputeCorpusStats([]string{input})
		built := BuildSparseVectors([]string{input}, stats)
		uniqueCanonical := uniqueStrings(canonical)

		// Use a 2-doc corpus to make the surrounding assertions easier to
		// read. With BM25's smoothed IDF, single-doc corpora are also fine
		// (IDF is positive), but a wider corpus keeps the test mirroring
		// the production code path.
		stats2 := ComputeCorpusStats([]string{input, "unrelated filler text"})
		built2 := BuildSparseVectors([]string{input}, stats2)

		// Now tokens that appear only in the first doc should have positive IDF.
		if len(built2[0].Indices) == 0 && len(uniqueCanonical) > 0 {
			t.Errorf("input %q: BuildSparseVectors produced empty vector but Tokenize produced %d tokens",
				input, len(uniqueCanonical))
		}

		// TokenizeQuery: verify same number of unique token hashes.
		querySV := TokenizeQuery(input)
		if len(querySV.Indices) != len(uniqueCanonical) {
			t.Errorf("input %q: TokenizeQuery produced %d indices, Tokenize produced %d unique tokens",
				input, len(querySV.Indices), len(uniqueCanonical))
		}

		// Verify the same hashes appear in both.
		queryHashes := make(map[uint32]struct{})
		for _, idx := range querySV.Indices {
			queryHashes[idx] = struct{}{}
		}
		builtHashes := make(map[uint32]struct{})
		for _, idx := range built2[0].Indices {
			builtHashes[idx] = struct{}{}
		}
		for h := range builtHashes {
			if _, ok := queryHashes[h]; !ok {
				t.Errorf("input %q: hash %d in BuildSparseVectors but not in TokenizeQuery", input, h)
			}
		}

		// Ignore the single-doc IDF=0 case.
		_ = built
	}
}

// --- Helpers ------------------------------------------------------------

func assertTokens(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("token count: got %d %v, want %d %v", len(got), got, len(want), want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token[%d]: got %q, want %q (full: %v)", i, got[i], want[i], got)
			return
		}
	}
}

func assertContains(t *testing.T, tokens []string, want string) {
	t.Helper()
	for _, tok := range tokens {
		if tok == want {
			return
		}
	}
	t.Errorf("tokens %v does not contain %q", tokens, want)
}

func assertNotContains(t *testing.T, tokens []string, unwanted string) {
	t.Helper()
	for _, tok := range tokens {
		if tok == unwanted {
			t.Errorf("tokens %v should not contain stop word %q", tokens, unwanted)
			return
		}
	}
}

func uniqueStrings(ss []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, s := range ss {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			result = append(result, s)
		}
	}
	slices.Sort(result)
	return result
}
