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
	want := []string{"new", "vector", "input", "sparse"}
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

// --- ComputeIDF tests ---------------------------------------------------

func TestComputeIDF_Basic(t *testing.T) {
	texts := []string{
		"ChunkFile parser",
		"ChunkFile writer",
		"parser reader",
	}

	idf := ComputeIDF(texts)

	// "chunk" and "file" appear in 2/3 docs → idf = log(3/2)
	// "parser" appears in 2/3 docs → idf = log(3/2)
	// "writer" appears in 1/3 docs → idf = log(3/1)
	// "reader" appears in 1/3 docs → idf = log(3/1)

	expectedIDF := math.Log(3.0 / 2.0)
	if diff := math.Abs(idf["chunk"] - expectedIDF); diff > 0.001 {
		t.Errorf("idf[chunk] = %.4f, want ~%.4f", idf["chunk"], expectedIDF)
	}

	expectedRare := math.Log(3.0 / 1.0)
	if diff := math.Abs(idf["writer"] - expectedRare); diff > 0.001 {
		t.Errorf("idf[writer] = %.4f, want ~%.4f", idf["writer"], expectedRare)
	}
}

func TestComputeIDF_Empty(t *testing.T) {
	idf := ComputeIDF(nil)
	if idf != nil {
		t.Errorf("expected nil, got %v", idf)
	}
}

func TestComputeIDF_TokenInAllDocs(t *testing.T) {
	texts := []string{"hello world", "hello earth", "hello mars"}
	idf := ComputeIDF(texts)

	// "hello" in all 3 docs → idf = log(3/3) = 0
	if idf["hello"] != 0 {
		t.Errorf("idf[hello] = %.4f, want 0", idf["hello"])
	}
}

// --- BuildSparseVectors tests -------------------------------------------

func TestBuildSparseVectors_ProducesNonEmpty(t *testing.T) {
	texts := []string{"ChunkFile parser", "writer handler"}
	idf := ComputeIDF(texts)
	vectors := BuildSparseVectors(texts, idf)

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
	idf := ComputeIDF([]string{"hello world"})
	vectors := BuildSparseVectors([]string{""}, idf)
	if len(vectors) != 1 {
		t.Fatalf("expected 1 vector, got %d", len(vectors))
	}
	if len(vectors[0].Indices) != 0 {
		t.Error("expected empty sparse vector for empty text")
	}
}

func TestBuildSparseVectors_AllStopWords(t *testing.T) {
	texts := []string{"the func for return"}
	idf := ComputeIDF(texts)
	vectors := BuildSparseVectors(texts, idf)
	if len(vectors[0].Indices) != 0 {
		t.Error("expected empty sparse vector for all-stop-word text")
	}
}

func TestBuildSparseVectors_Deterministic(t *testing.T) {
	texts := []string{"ChunkFile parser writer handler reader"}
	idf := ComputeIDF(append(texts, "unrelated filler noise padding"))

	first := BuildSparseVectors(texts, idf)
	for i := 0; i < 20; i++ {
		again := BuildSparseVectors(texts, idf)
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
		idf := ComputeIDF([]string{input})
		built := BuildSparseVectors([]string{input}, idf)
		uniqueCanonical := uniqueStrings(canonical)

		// Filter out tokens with IDF=0 (appears in all docs — here only 1 doc,
		// so log(1/1)=0 → filtered out in BuildSparseVectors).
		// With a single doc, ALL tokens have idf=0, so the sparse vector is empty.
		// Use a 2-doc corpus to get meaningful IDF.
		idf2 := ComputeIDF([]string{input, "unrelated filler text"})
		built2 := BuildSparseVectors([]string{input}, idf2)

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
