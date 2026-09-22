/* ==========================================================================
   ragcodepilot — Modern Neobrutalism Interactive Application
   ========================================================================== */

document.addEventListener('DOMContentLoaded', () => {
  initEvidence();
  initTheme();
  initNav();
  initPipeline();
  initSimulator();
  initDeepDives();
  initWorkedExamples();
  initSourceLinks();
  initEvalTable();
  initTerminal();
  initQuickActions();
  document.querySelectorAll('[data-copy-command]').forEach(button => {
    button.addEventListener('click', () => copyCommand(document.getElementById(button.dataset.copyCommand).textContent, button));
  });
  initScrollSpy();
});

// Pin source links so the illustrated implementation remains reviewable as main changes.
const sourceBase = `${SITE_EVIDENCE.repository}/blob/${SITE_EVIDENCE.implementation_revision}/`;

function initEvidence() {
  const report = SITE_EVIDENCE.retrieval_report;
  const a = report.aggregate;
  const generation = SITE_EVIDENCE.generation_report;
  const percent = value => `${(value * 100).toFixed(1)}%`;
  const values = {
    hit1: percent(a.hit_at_1), hit5: percent(a.hit_at_5),
    recall10: percent(a.recall_at_10), mrr5: a.mrr_at_5.toFixed(3),
    negativePass: a.negative_pass_rate.toFixed(2), p95: `${a.latency_total_p95_ms}ms`,
    positiveScope: `${a.positive_queries} positive queries, ${report.mode} mode`,
    corpusScope: `${a.queries} golden queries on this Go repository`,
    queryCount: `GOLDEN BENCHMARK (${a.queries} QUERIES)`,
    retrievalDate: report.run_id.slice(0, 10),
    allRows: `All ${a.queries} queries from the saved report; ${report.queries.filter(q => q.type !== 'negative' && q.hit_at_5).length} of ${a.positive_queries} positives hit@5. Returned files may be irrelevant, especially for negative queries.`,
    negativeScope: `${report.queries.filter(q => q.type === 'negative' && !q.negative.pass).length} of ${a.negative_queries} out-of-scope queries fail the calibrated check`,
    generationScope: `${generation.answer.generated}-query structural answer run`,
    generationDate: generation.run_id.slice(0, 10),
    generationP50: `${(generation.answer.generate_p50_ms / 1000).toFixed(1)}s`,
    generationP95: `${(generation.answer.generate_p95_ms / 1000).toFixed(1)}s`,
    revision: SITE_EVIDENCE.implementation_revision.slice(0, 12),
    reviewed: SITE_EVIDENCE.implementation_reviewed_on,
    goVersion: SITE_EVIDENCE.go_version
  };
  document.querySelectorAll('[data-evidence]').forEach(node => {
    node.textContent = values[node.dataset.evidence];
  });
  document.getElementById('implementationRevision').href = `${SITE_EVIDENCE.repository}/tree/${SITE_EVIDENCE.implementation_revision}`;
}

function sourceUrl(path, lines = '') {
  const range = /^(\d+)(?:-(\d+))?$/.exec(lines);
  return sourceBase + path + (range ? `#L${range[1]}${range[2] ? `-L${range[2]}` : ''}` : '');
}

function initSourceLinks() {
  document.querySelectorAll('[data-source]').forEach(link => {
    link.href = sourceUrl(link.dataset.source, link.dataset.lines);
    link.target = '_blank';
    link.rel = 'noopener';
  });
}

function prefersReducedMotion() {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/* ==========================================================================
   1. Theme Management (Light Parchment <-> Sleek Dark Neobrutalism)
   ========================================================================== */
function initTheme() {
  const themeToggleBtn = document.getElementById('themeToggleBtn');
  const themeIcon = document.getElementById('themeIcon');
  const themeText = document.getElementById('themeText');
  const htmlEl = document.documentElement;

  const savedTheme = localStorage.getItem('ragcodepilot_theme') || 'light';
  applyTheme(savedTheme);

  themeToggleBtn.addEventListener('click', () => {
    const currentTheme = htmlEl.getAttribute('data-theme') || 'light';
    const newTheme = currentTheme === 'light' ? 'dark' : 'light';
    applyTheme(newTheme);
    localStorage.setItem('ragcodepilot_theme', newTheme);
  });

  function applyTheme(theme) {
    htmlEl.setAttribute('data-theme', theme);
    if (theme === 'dark') {
      themeIcon.textContent = '☀️';
      themeText.textContent = 'Light Mode';
    } else {
      themeIcon.textContent = '🌙';
      themeText.textContent = 'Dark Mode';
    }
  }
}

/* ==========================================================================
   2. Interactive Dual-Pipeline Architecture
   ========================================================================== */
const pipelineData = {
  ingestion: [
    {
      id: 'ingest-source',
      icon: '📂',
      title: 'Repository Source',
      subtitle: 'Local Git checkout',
      symbol: 'ragcodepilot index <repoPath>',
      pkg: 'root filesystem',
      input: 'Local repository path (e.g., ".")',
      output: 'File tree for the walker to traverse',
      desc: 'The indexing entry point: a local checkout of a Git repository. With the default local service addresses, source content stays on this machine. The walker applies the language and skip rules defined in config.yaml.',
      code: `// Everything runs locally — no file content is sent to any cloud API.
//   ragcodepilot index --language go <repoPath>
//
// The walker only sees paths that survive the config.yaml rules:
//   - skip dirs:  .git, vendor, node_modules, hidden directories
//   - skip files: *_test.go (test assertions dilute code search)`
    },
    {
      id: 'ingest-walker',
      icon: '📂',
      title: '1. File Walker',
      subtitle: 'Recursive traversal',
      symbol: 'WalkFiles(root string, config *Config)',
      pkg: 'internal/ingest/walker.go',
      input: 'Repository root path (e.g., ".")',
      output: '[]string of valid file paths',
      desc: 'Traverses the repository directory tree, enforcing ignore lists from config.yaml. Skips .git, vendor, hidden files, and *_test.go files to ensure test assertions do not contaminate code retrieval.',
      code: `// WalkFiles traverses root and filters candidate source files
func WalkFiles(root string, cfg *config.Config) ([]string, error) {
    var files []string
    err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
        if d.IsDir() && cfg.ShouldSkipDir(d.Name()) {
            return filepath.SkipDir
        }
        if cfg.ShouldSkipFile(d.Name()) { // skips *_test.go
            return nil
        }
        files = append(files, path)
        return nil
    })
    return files, err
}`
    },
    {
      id: 'ingest-hasher',
      icon: '🔐',
      title: '2. SHA-256 Hasher',
      subtitle: 'Change detection',
      symbol: 'HashFile(path string) (string, error)',
      pkg: 'internal/ingest/hasher.go',
      input: 'File content byte stream',
      output: 'Hex-encoded SHA-256 digest per file',
      desc: 'Calculates the SHA-256 checksum of each file. If every hash and index version match, the run is a no-op. If any file changed, BM25 IDF is rebuilt and all current files are re-chunked and re-embedded — not only the files that changed.',
      code: `// HashFile returns the hex-encoded SHA-256 hash of the file at the given path.
func HashFile(path string) (string, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return "", fmt.Errorf("reading file for hashing: %w", err)
    }
    sum := sha256.Sum256(data)
    return hex.EncodeToString(sum[:]), nil
}`
    },
    {
      id: 'ingest-chunker',
      icon: '✂️',
      title: '3. AST Chunker',
      subtitle: 'Function-level units',
      symbol: 'chunkGoFile(path, repoRoot, repo, chunkSize, overlap, cfg)',
      pkg: 'internal/ingest/chunker_go.go',
      input: 'Raw Go source code',
      output: '[]model.CodeChunk (functions, types, interfaces, blocks)',
      desc: 'Uses go/parser to extract function/method declarations and named type/interface specs. Declarations longer than 80 lines use sliding-window splitting. Remaining imports and vars become block chunks. Syntax errors fall back to the generic sliding window.',
      code: `func chunkGoFile(...) ([]model.CodeChunk, error) {
    fset := gotoken.NewFileSet()
    file, parseErr := parser.ParseFile(fset, filePath, src, parser.ParseComments)
    if parseErr != nil {
        return chunkGeneric(...) // syntax-error fallback
    }
    for _, decl := range file.Decls {
        switch d := decl.(type) {
        case *ast.FuncDecl:  // one chunk per function/method
        case *ast.GenDecl:   // named type / interface specs
        }
    }
    // leftover lines → block chunks
}`
    },
    {
      id: 'ingest-enrich',
      icon: '🏷️',
      title: '4. Enrichment',
      subtitle: 'Metadata prepending',
      symbol: 'enrichForEmbedding(chunk model.CodeChunk)',
      pkg: 'internal/ingest/enrichment.go',
      input: 'CodeChunk struct',
      output: 'Enriched string for embedding input',
      desc: 'Prepends File, Language, and Function/Type/Interface (or Type: Block). Both dense embedding and sparse document vector generation use this enriched text. Qdrant stores the original raw code payload.',
      code: `func enrichForEmbedding(chunk model.CodeChunk) string {
    var b strings.Builder
    fmt.Fprintf(&b, "File: %s\\n", chunk.FilePath)
    fmt.Fprintf(&b, "Language: %s\\n", chunk.Language)
    label := chunkTypeLabel(chunk.ChunkType)
    if chunk.Name != "" {
        fmt.Fprintf(&b, "%s: %s\\n", label, chunk.Name)
    } else {
        fmt.Fprintf(&b, "Type: %s\\n", label)
    }
    b.WriteString("\\n")
    b.WriteString(chunk.Content)
    return b.String()
}`
    },
    {
      id: 'ingest-vectorize',
      icon: '🧠',
      title: '5. Dual Vectorizer',
      subtitle: 'Dense 768d + BM25',
      symbol: 'Embed(texts) + BuildSparseVectors(texts, stats)',
      pkg: 'internal/embedding/ollama.go',
      input: 'Enriched code texts',
      output: '768d float vector + Sparse BM25 term weights',
      desc: 'Calls Ollama HTTP /api/embed (nomic-embed-text) for dense vectors, then builds sparse BM25 document vectors using corpus-wide statistics. Tokenization preserves full identifiers and adds Snowball stems; terms are CRC32-hashed. These are sequential steps in each ingestion batch, not parallel workers.',
      code: `// Embedder.Embed takes a batch of texts (nomic-embed-text, 768d)
func (e *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
    // POST /api/embed  { model, input: texts }
}`
    },
    {
      id: 'ingest-upsert',
      icon: '🗄️',
      title: '6. Qdrant Upsert',
      subtitle: 'gRPC multi-vector batch',
      symbol: '(*Client).Upsert(ctx, collection, chunks, vectors, sparseVectors)',
      pkg: 'internal/qdrant/client.go',
      input: 'Multi-vector points with payloads',
      output: 'Qdrant points stored & indexes synced',
      desc: 'Upserts dense and sparse vectors plus raw code and metadata over gRPC. Keyword payload indexes cover repo, language, and file_path. file_hash and index_version are stored metadata. The ingestion pipeline separately deletes stale points.',
      code: `// Excerpt of Client.Upsert: one named dense and sparse vector per point.
// Validation, payload construction, and the batch loop are omitted here.
vectorsMap := map[string]*pb.Vector{
    "dense": pb.NewVectorDense(vectors[i]),
}
vectorsMap["sparse"] = pb.NewVectorSparse(
    sparseVectors[i].Indices, sparseVectors[i].Values,
)
// Point ID + vectorsMap + raw code payload → batches of up to 64 points.`
    }
  ],

  search: [
    {
      id: 'search-query',
      icon: '💬',
      title: '1. User Query',
      subtitle: 'Plain English input',
      symbol: 'runSearch(query, flags)',
      pkg: 'cmd/ragcodepilot/main.go',
      input: 'Natural language string from terminal',
      output: 'SearchRequest parameters',
      desc: 'Parses CLI command arguments including search query string, retrieval mode (--mode hybrid|dense|sparse), repo/language filters, and opt-in --answer flag.',
      code: `// Search CLI command flags (cmd/ragcodepilot/main.go)
mode := fs.String("mode", string(search.DefaultSearchMode), "Search mode: dense, sparse, hybrid")
language := fs.String("language", "", "Comma-separated language filter (e.g., go,rust)")
repo := fs.String("repo", "", "Comma-separated repo name filter")
answerMode := fs.Bool("answer", false, "Generate an answer from the retrieved chunks (RAG mode)")
...
searchMode, err := search.ParseSearchMode(*mode)
return runSearch(query, *collection, languages, repos, searchMode, *limit, *answerLimit, ...)`
    },
    {
      id: 'search-encode',
      icon: '🔤',
      title: '2. Query Encoding',
      subtitle: 'Dense & sparse terms',
      symbol: 'SearchWithTimings(ctx, collection, query, mode, ...)',
      pkg: 'internal/search/searcher.go',
      input: 'Query text',
      output: 'Query 768d vector + BM25 query terms',
      desc: 'Embeds the query text with Ollama nomic-embed-text to construct the dense search vector, and stems the query tokens with Snowball to generate the sparse BM25 query vector.',
      code: `// SearchWithTimings builds dual vectors for the incoming search query
if mode == SearchModeDense || mode == SearchModeHybrid {
    vectors, err := s.embedder.Embed(ctx, []string{query})
    if err != nil { return nil, t, fmt.Errorf("embedding query: %w", err) }
    queryDim, err := embedding.ValidateVectorBatch(vectors, 0)
    if err != nil { return nil, t, err }
    denseVector = vectors[0]
}
if mode == SearchModeSparse || mode == SearchModeHybrid {
    sv := embedding.TokenizeQuery(query)
    sparseVector = &sv
}`
    },
    {
      id: 'search-bm25',
      icon: '🔡',
      title: '2b. BM25 Query Encoding',
      subtitle: 'Stemmed sparse terms',
      symbol: 'TokenizeQuery(query string) SparseVector',
      pkg: 'internal/embedding/sparse.go',
      input: 'Query text',
      output: 'Sparse vector (CRC32 term hashes + uniform weights of 1.0)',
      desc: 'Uses the same identifier splitting, preserved full identifiers, and additive Snowball stems as indexing. Each unique term hash gets query weight 1.0. Corpus IDF and length normalization are already included in stored document vectors, not recomputed for a query.',
      code: `// Query weights are uniform; BM25 weights live in document vectors.
func TokenizeQuery(query string) SparseVector {
    tokens := Tokenize(query)
    // Deduplicate CRC32 hashes, then append 1.0 per unique hash.
    // Return SparseVector{Indices: indices, Values: values}.
}

// tokenHash maps a term to its 32-bit sparse index (CRC32).
func tokenHash(token string) uint32 { ... }`
    },
    {
      id: 'search-filter',
      icon: '🎯',
      title: '3. Payload Filters',
      subtitle: 'Indexed constraints',
      symbol: 'buildFilter(languages, repos []string)',
      pkg: 'internal/qdrant/client.go',
      input: 'CLI filter flags (--language, --repo)',
      output: 'Qdrant Filter protobuf struct',
      desc: 'Constructs fast boolean filters against indexed payload fields. Filters are evaluated on each prefetch stage before vector comparison, ensuring high search speed across multi-repository workspaces.',
      code: `// buildFilter constructs a Qdrant filter from language and repo slices.
func buildFilter(languages, repos []string) *pb.Filter {
    // language → pb.NewMatch("language", lang)
    // repo     → pb.NewMatch("repo", repo)
    // combined into a Must condition list
}`
    },
    {
      id: 'search-lookup',
      icon: '🔍',
      title: '4. Vector Lookup',
      subtitle: 'Parallel candidate fetch',
      symbol: 'Search(ctx, collection, dense, sparse, mode, limit, ...)',
      pkg: 'internal/qdrant/client.go',
      input: 'Query vectors + Filters',
      output: 'Dense top-K list & Sparse top-K list',
      desc: 'Executes parallel candidate retrieval inside Qdrant: cosine similarity across 768-dim dense vectors and BM25 dot products across the sparse index, each fetching limit×2 prefetch candidates with the payload filter applied.',
      code: `// Hybrid mode issues two prefetch stages (dense + sparse) in one gRPC call
prefetchLimit := limit * 2
Prefetch: []*pb.PrefetchQuery{
    {
        Query:  pb.NewQueryDense(denseVector),
        Using:  pb.PtrOf("dense"),
        Limit:  pb.PtrOf(prefetchLimit),
        Filter: filter,
    },
    {
        Query:  pb.NewQuerySparse(sparseVector.Indices, sparseVector.Values),
        Using:  pb.PtrOf("sparse"),
        Limit:  pb.PtrOf(prefetchLimit),
        Filter: filter,
    },
}`
    },
    {
      id: 'search-rrf',
      icon: '⚖️',
      title: '5. RRF Fusion',
      subtitle: 'Server-side rerank (k=60)',
      symbol: 'pb.NewQueryRRF(&pb.Rrf{K: 60})',
      pkg: 'internal/qdrant/client.go',
      input: 'Ranked dense & sparse candidate lists',
      output: 'Fused & sorted []SearchResult',
      desc: 'Fusion runs inside Qdrant in the same query RPC. With k=60 and zero-based positions (first = 0), each list contributes 1/(60 + position), or 0 if the chunk is absent. The sum determines the fused order; raw cosine and BM25 scores are not added.',
      code: `// rrfK is the standard Reciprocal Rank Fusion constant (client.go:269)
const rrfK = 60

queryPoints = &pb.QueryPoints{
    CollectionName: collection,
    Prefetch:       [ /* dense + sparse prefetch stages */ ],
    Query:          pb.NewQueryRRF(&pb.Rrf{K: pb.PtrOf(uint32(rrfK))}),
    Limit:          pb.PtrOf(limit),
    WithPayload:    pb.NewWithPayload(true),
}`
    },
    {
      id: 'search-context',
      icon: '📄',
      title: '6. Context Assembly',
      subtitle: 'Top-K result payloads',
      symbol: 'FormatResults(results []model.SearchResult)',
      pkg: 'internal/search/searcher.go',
      input: 'Fused top-K ScoredPoints from Qdrant',
      output: 'Ranked []model.SearchResult with payloads',
      desc: 'Qdrant returns the fused top-K points with their payloads (repo, file path, line range, symbol name, language). These become the ranked search results shown in the terminal — or the context fed to the answer generator.',
      code: `// FormatResults renders ranked chunks for terminal display
for i, r := range results {
    fmt.Fprintf(&b, "\\n--- Result %d (score: %.4f) ---\\n", i+1, r.Score)
    fmt.Fprintf(&b, "Repo: %s  File: %s  Lang: %s\\n", r.Chunk.Repo, r.Chunk.FilePath, r.Chunk.Language)
    fmt.Fprintf(&b, "Lines %d-%d", r.Chunk.StartLine, r.Chunk.EndLine)
}`
    },
    {
      id: 'search-answer',
      icon: '🤖',
      title: '7. Answer & Citations',
      subtitle: 'Grounded LLM synthesis',
      symbol: '(*OllamaGenerator).Generate(ctx, prompt)',
      pkg: 'internal/answer/ollama.go',
      input: 'Top-K retrieved chunks + frozen prompt',
      output: 'Grounded explanation with citations [1], [2]',
      desc: 'When --answer is enabled, retrieved code chunks are injected into a prompt built by answer.BuildPrompt. Local Ollama (qwen2.5-coder:7b) generates a concise, grounded explanation with exact file citations.',
      code: `// Generate calls local qwen2.5-coder with the grounded prompt
func (o *OllamaGenerator) Generate(ctx context.Context, prompt Prompt) (string, error) {
    // POST /api/chat { model: "qwen2.5-coder:7b", messages: [...], ... }
}`
    },
    {
      id: 'search-results', icon: '💻', title: '7. Ranked Code Results',
      subtitle: 'Default CLI output', symbol: 'FormatResults(results)',
      pkg: 'internal/search/searcher.go', input: 'Ranked results with raw source payloads',
      output: 'Code, scores, file paths, symbols, and line ranges',
      desc: 'The default search ends here. FormatResults prints ranked source code directly; no generative model is called. Only --answer opts into generation from the top answer-limit results.',
      code: `// cmd/ragcodepilot/main.go: retrieval-only branch
if gen == nil {
    fmt.Print(search.FormatResults(results))
    return nil
}`
    }
  ]
};

/* ==========================================================================
   2. Interactive Animated Visual Pipeline Canvas
   ========================================================================== */

const canvasFlowDefinitions = {
  ingestion: {
    nodes: [
      { id: 'ingest-source', detail: 'ingest-source', x: 25, y: 145, w: 125, h: 76, icon: '📂', name: 'Git Files', sub: 'Repo source', pkg: 'root filesystem', color: '#ffde59', step: 1 },
      { id: 'ingest-walker', detail: 'ingest-walker', x: 185, y: 145, w: 130, h: 76, icon: '🚶', name: 'File Walker', sub: 'config.yaml', pkg: 'ingest/pipeline.go', color: '#ffde59', step: 2 },
      { id: 'ingest-hasher', detail: 'ingest-hasher', x: 350, y: 145, w: 135, h: 76, icon: '🔐', name: 'SHA-256 Check', sub: 'Diff & cache', pkg: 'ingest/hasher.go', color: '#ffadc6', step: 3 },
      { id: 'ingest-chunker', detail: 'ingest-chunker', x: 520, y: 145, w: 130, h: 76, icon: '✂️', name: 'AST Chunker', sub: 'Function units', pkg: 'chunker_go.go', color: '#a7f3d0', step: 4 },
      { id: 'ingest-enrich', detail: 'ingest-enrich', x: 685, y: 145, w: 125, h: 76, icon: '🏷️', name: 'Enrichment', sub: 'Metadata inject', pkg: 'enrichment.go', color: '#ffde59', step: 5 },
      { id: 'ingest-ollama', detail: 'ingest-vectorize', x: 845, y: 45, w: 145, h: 76, icon: '🧠', name: 'Ollama Embed', sub: 'nomic-embed (768d)', pkg: 'embedding/ollama.go', color: '#9be3ff', step: 6 },
      { id: 'ingest-bm25', detail: 'ingest-vectorize', x: 845, y: 245, w: 145, h: 76, icon: '🔤', name: 'BM25 Sparse', sub: 'Snowball stemmer', pkg: 'embedding/sparse.go', color: '#ff6332', step: 6 },
      { id: 'ingest-upsert', detail: 'ingest-upsert', x: 1015, y: 145, w: 110, h: 76, icon: '🗄️', name: 'Qdrant gRPC', sub: 'Dual vectors', pkg: 'qdrant/client.go', color: '#a7f3d0', step: 7 }
    ],
    connections: [
      { id: 'conn-0-1', from: 'ingest-source', to: 'ingest-walker', d: 'M 150 183 L 185 183', color: 'yellow', marker: 'arrowYellow', speed: 1 },
      { id: 'conn-1-2', from: 'ingest-walker', to: 'ingest-hasher', d: 'M 315 183 L 350 183', color: 'yellow', marker: 'arrowYellow', speed: 1 },
      { id: 'conn-2-3', from: 'ingest-hasher', to: 'ingest-chunker', d: 'M 485 183 L 520 183', color: 'pink', marker: 'arrowPink', speed: 1 },
      { id: 'conn-3-4', from: 'ingest-chunker', to: 'ingest-enrich', d: 'M 650 183 L 685 183', color: 'mint', marker: 'arrowMint', speed: 1 },
      { id: 'conn-4-5a', from: 'ingest-enrich', to: 'ingest-ollama', d: 'M 810 170 C 825 170, 825 83, 845 83', color: 'blue', marker: 'arrowBlue', speed: 0.9 },
      { id: 'conn-4-5b', from: 'ingest-enrich', to: 'ingest-bm25', d: 'M 810 196 C 825 196, 825 283, 845 283', color: 'orange', marker: 'arrowOrange', speed: 0.9 },
      { id: 'conn-5a-6', from: 'ingest-ollama', to: 'ingest-upsert', d: 'M 990 83 C 1005 83, 1005 170, 1015 170', color: 'blue', marker: 'arrowMint', speed: 0.9 },
      { id: 'conn-5b-6', from: 'ingest-bm25', to: 'ingest-upsert', d: 'M 990 283 C 1005 283, 1005 196, 1015 196', color: 'orange', marker: 'arrowMint', speed: 0.9 }
    ]
  },

  search: {
    nodes: [
      { id: 'search-query', detail: 'search-query', x: 25, y: 145, w: 130, h: 76, icon: '💬', name: 'User Query', sub: 'CLI Terminal', pkg: 'cmd/ragcodepilot', color: '#ffde59', step: 1 },
      { id: 'search-ollama', detail: 'search-encode', x: 195, y: 45, w: 140, h: 76, icon: '🧠', name: 'Ollama Query', sub: '768d Dense Vector', pkg: 'embedding/ollama.go', color: '#9be3ff', step: 2 },
      { id: 'search-bm25', detail: 'search-bm25', x: 195, y: 245, w: 140, h: 76, icon: '🔤', name: 'BM25 Stemmer', sub: 'Query Tokens', pkg: 'embedding/sparse.go', color: '#ff6332', step: 2 },
      { id: 'search-filter', detail: 'search-filter', x: 375, y: 145, w: 125, h: 76, icon: '🎯', name: 'Payload Filter', sub: 'Lang & Repo tags', pkg: 'qdrant/client.go', color: '#ffadc6', step: 3 },
      { id: 'search-qdrant', detail: 'search-lookup', x: 535, y: 145, w: 130, h: 76, icon: '🔍', name: 'Vector Lookup', sub: 'Parallel Top-K', pkg: 'qdrant gRPC', color: '#9be3ff', step: 4 },
      { id: 'search-rrf', detail: 'search-rrf', x: 700, y: 145, w: 130, h: 76, icon: '⚖️', name: 'RRF Fusion', sub: 'Rank Merge (k=60)', pkg: 'qdrant/client.go', color: '#a7f3d0', step: 5 },
      { id: 'search-context', detail: 'search-context', x: 865, y: 145, w: 125, h: 76, icon: '📄', name: 'Context Chunks', sub: 'Top-K assembly', pkg: 'search/searcher.go', color: '#ffde59', step: 6 },
      { id: 'search-results', detail: 'search-results', x: 1000, y: 45, w: 125, h: 76, icon: '💻', name: 'Code Results', sub: 'Default: print code', pkg: 'Go CLI · stdout', color: '#a7f3d0', step: 7 },
      { id: 'search-answer', detail: 'search-answer', x: 1000, y: 245, w: 125, h: 76, icon: '🤖', name: 'Ollama LLM', sub: '--answer only', pkg: 'HTTP /api/chat', color: '#d8b4fe', step: 7 }
    ],
    connections: [
      { id: 'sconn-0-1a', from: 'search-query', to: 'search-ollama', d: 'M 155 170 C 175 170, 175 83, 195 83', color: 'blue', marker: 'arrowBlue', speed: 1 },
      { id: 'sconn-0-1b', from: 'search-query', to: 'search-bm25', d: 'M 155 196 C 175 196, 175 283, 195 283', color: 'orange', marker: 'arrowOrange', speed: 1 },
      { id: 'sconn-1a-2', from: 'search-ollama', to: 'search-filter', d: 'M 335 83 C 355 83, 355 170, 375 170', color: 'blue', marker: 'arrowPink', speed: 1 },
      { id: 'sconn-1b-2', from: 'search-bm25', to: 'search-filter', d: 'M 335 283 C 355 283, 355 196, 375 196', color: 'orange', marker: 'arrowPink', speed: 1 },
      { id: 'sconn-2-3', from: 'search-filter', to: 'search-qdrant', d: 'M 500 183 L 535 183', color: 'pink', marker: 'arrowBlue', speed: 1 },
      { id: 'sconn-3-4', from: 'search-qdrant', to: 'search-rrf', d: 'M 665 183 L 700 183', color: 'blue', marker: 'arrowMint', speed: 1 },
      { id: 'sconn-4-5', from: 'search-rrf', to: 'search-context', d: 'M 830 183 L 865 183', color: 'mint', marker: 'arrowYellow', speed: 1 },
      { id: 'sconn-default', from: 'search-context', to: 'search-results', d: 'M 928 145 L 928 83 L 1000 83', color: 'mint', marker: 'arrowMint', speed: 1 },
      { id: 'sconn-answer', from: 'search-context', to: 'search-answer', d: 'M 928 221 L 928 283 L 1000 283', color: 'pink', marker: 'arrowPink', speed: 1 }
    ]
  }
};

function initPipeline() {
  const canvasWrapper = document.getElementById('pipelineCanvasWrapper');
  const svgPathsLayer = document.getElementById('svgPathsLayer');
  const svgParticlesLayer = document.getElementById('svgParticlesLayer');
  const svgNodesLayer = document.getElementById('svgNodesLayer');
  const flowTabBtns = document.querySelectorAll('.flow-tab-btn');
  const tickerText = document.getElementById('tickerText');

  // Animation controls
  const playPauseBtn = document.getElementById('streamPlayPauseBtn');
  const playPauseIcon = document.getElementById('playPauseIcon');
  const playPauseText = document.getElementById('playPauseText');
  const speedBtn = document.getElementById('streamSpeedBtn');
  const speedText = document.getElementById('speedText');
  const stepBtn = document.getElementById('streamStepBtn');
  const resetBtn = document.getElementById('streamResetBtn');

  let currentFlow = 'ingestion';
  let isPlaying = !prefersReducedMotion();
  let speedMultiplier = 1;
  let animFrameId = null;
  let activeNodeIndex = 0;
  const mobileList = document.getElementById('pipelineMobileList');

  // Particle tracking
  let particles = [];
  let pathElements = {};

  function loadFlow(flowKey) {
    currentFlow = flowKey;
    activeNodeIndex = 0;
    particles = [];
    pathElements = {};

    const flowDef = canvasFlowDefinitions[flowKey];
    document.getElementById('svgLabelsLayer').innerHTML = flowKey === 'search' ? `
      <text x="677" y="121" class="svg-boundary-label" text-anchor="middle">QDRANT · ONE QUERY RPC</text>
      <text x="943" y="64" class="svg-route-label">DEFAULT</text>
      <text x="942" y="310" class="svg-route-label">OPTIONAL</text>` : '';

    // 1. Render SVG Paths
    svgPathsLayer.innerHTML = flowDef.connections.map(conn => `
      <path id="${conn.id}" class="flow-bus-line bus-${conn.color}" d="${conn.d}" marker-end="url(#${conn.marker})" />
    `).join('');

    // Cache path DOM elements
    flowDef.connections.forEach(conn => {
      const el = document.getElementById(conn.id);
      if (el) pathElements[conn.id] = { el, length: el.getTotalLength(), conn };
    });

    // 2. Render SVG Nodes (Neobrutalist cards)
    svgNodesLayer.innerHTML = flowDef.nodes.map((node, idx) => `
      <g class="canvas-node-group ${idx === 0 ? 'active' : ''}" id="cnode-${node.id}" data-node-id="${node.id}" transform="translate(${node.x}, ${node.y})" tabindex="0" role="button" aria-pressed="${idx === 0}" aria-label="${node.name}: ${node.sub}">
        <!-- Card Drop Shadow & Border -->
        <rect class="canvas-node-rect" width="${node.w}" height="${node.h}" />

        <!-- Header color strip -->
        <rect class="node-header-bg" x="0" y="0" width="${node.w}" height="18" rx="10" fill="${node.color}" stroke="none" />
        <rect x="0" y="10" width="${node.w}" height="8" fill="${node.color}" stroke="none" />
        <line x1="0" y1="18" x2="${node.w}" y2="18" stroke="#000" stroke-width="1.5" />

        <!-- Node Icon & Title -->
        <text x="8" y="13" font-family="'JetBrains Mono', monospace" font-size="9" font-weight="800" fill="#000">STAGE ${node.step}</text>
        <text x="10" y="38" font-size="16">${node.icon}</text>
        <text x="32" y="36" font-family="'Plus Jakarta Sans', sans-serif" font-size="11" font-weight="800" fill="var(--text-primary)">${node.name}</text>
        <text x="10" y="52" font-family="'Plus Jakarta Sans', sans-serif" font-size="9.5" font-weight="600" fill="var(--text-secondary)">${node.sub}</text>
        <text x="10" y="66" font-family="'JetBrains Mono', monospace" font-size="8" font-weight="700" fill="var(--text-muted)">${node.pkg}</text>

        <!-- Port connector pins -->
        <circle cx="0" cy="${node.h / 2}" r="3.5" fill="${node.color}" stroke="#000" stroke-width="1.5" />
        <circle cx="${node.w}" cy="${node.h / 2}" r="3.5" fill="${node.color}" stroke="#000" stroke-width="1.5" />
      </g>
    `).join('');

    // Attach click + keyboard handlers to canvas nodes
    flowDef.nodes.forEach((node, idx) => {
      const g = document.getElementById(`cnode-${node.id}`);
      if (g) {
        g.addEventListener('click', () => {
          selectCanvasNode(idx);
        });
        g.addEventListener('keydown', (e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            selectCanvasNode(idx);
          }
        });
      }
    });

    // Initialize particles
    flowDef.connections.forEach(conn => {
      particles.push({
        connId: conn.id,
        progress: Math.random() * 0.8, // stagger particles
        speed: (conn.speed || 1) * 0.0035,
        color: conn.color === 'yellow' ? '#ffde59' : conn.color === 'blue' ? '#38bdf8' : conn.color === 'orange' ? '#ff6332' : conn.color === 'pink' ? '#ffadc6' : '#a7f3d0'
      });
    });

    renderMobileList();

    // Select initial node
    selectCanvasNode(0);
    updateTicker();
  }

  function renderMobileList() {
    if (!mobileList) return;
    const flowDef = canvasFlowDefinitions[currentFlow];
    mobileList.innerHTML = flowDef.nodes.map((node, idx) => `
      <li>
        <button type="button" class="pipeline-mobile-step${idx === activeNodeIndex ? ' active' : ''}" data-idx="${idx}" aria-pressed="${idx === activeNodeIndex}">
          <span class="pipeline-mobile-stage">STAGE ${node.step}</span>
          <span class="pipeline-mobile-name">${node.icon} ${node.name}</span>
          <span class="pipeline-mobile-sub">${node.sub} · ${node.pkg}</span>
        </button>
      </li>
    `).join('');
    mobileList.querySelectorAll('.pipeline-mobile-step').forEach(btn => {
      btn.addEventListener('click', () => selectCanvasNode(Number(btn.getAttribute('data-idx'))));
    });
  }

  function selectCanvasNode(idx) {
    activeNodeIndex = idx;
    const flowDef = canvasFlowDefinitions[currentFlow];
    const node = flowDef.nodes[idx];
    if (!node) return;

    // Update active class on SVG nodes
    document.querySelectorAll('.canvas-node-group').forEach(n => {
      n.classList.remove('active');
      n.setAttribute('aria-pressed', 'false');
    });
    const activeG = document.getElementById(`cnode-${node.id}`);
    if (activeG) {
      activeG.classList.add('active');
      activeG.setAttribute('aria-pressed', 'true');
      if (!prefersReducedMotion()) {
        activeG.classList.add('pulse-hit');
        setTimeout(() => activeG.classList.remove('pulse-hit'), 600);
      }
    }
    if (mobileList) {
      mobileList.querySelectorAll('.pipeline-mobile-step').forEach((btn, i) => {
        btn.classList.toggle('active', i === idx);
        btn.setAttribute('aria-pressed', String(i === idx));
      });
    }

    // Lookup corresponding deep inspector data via the node's explicit detail id
    const detailedList = pipelineData[currentFlow] || [];
    const matchedData = detailedList.find(d => d.id === node.detail) || detailedList[0];

    if (matchedData) {
      document.getElementById('inspectTitle').innerHTML = `${matchedData.icon} ${matchedData.title}`;
      document.getElementById('inspectDesc').textContent = matchedData.desc;
      document.getElementById('inspectSymbol').textContent = matchedData.symbol;
      document.getElementById('inspectPackage').textContent = matchedData.pkg;
      document.getElementById('inspectInput').textContent = matchedData.input;
      document.getElementById('inspectOutput').textContent = matchedData.output;
      document.getElementById('inspectCodeSnippet').textContent = matchedData.code;
      document.getElementById('inspectSourceLink').href = sourceUrl(matchedData.pkg === 'root filesystem' ? 'cmd/ragcodepilot/main.go' : matchedData.pkg);
    }

    updateTicker();
  }

  function updateTicker() {
    const flowDef = canvasFlowDefinitions[currentFlow];
    const node = flowDef.nodes[activeNodeIndex];
    if (currentFlow === 'ingestion') {
      tickerText.textContent = `STREAM: [${node.name}] Active ➔ Processing ${node.pkg} ➔ Ingesting chunks to Qdrant (gRPC:6334)...`;
    } else {
      tickerText.textContent = `SELECTED: ${node.name} · Default: ranked code · Optional --answer: HTTP /api/chat`;
    }
  }

  function stopAnimation() {
    isPlaying = false;
    if (animFrameId) {
      cancelAnimationFrame(animFrameId);
      animFrameId = null;
    }
    svgParticlesLayer.innerHTML = '';
    playPauseIcon.textContent = '▶️';
    playPauseText.textContent = 'Play Animation';
    playPauseBtn.classList.remove('nb-btn-yellow');
    canvasWrapper.classList.add('paused');
  }

  function startAnimation() {
    if (isPlaying && animFrameId) return;
    isPlaying = true;
    playPauseIcon.textContent = '⏸️';
    playPauseText.textContent = 'Pause Animation';
    playPauseBtn.classList.add('nb-btn-yellow');
    canvasWrapper.classList.remove('paused');
    function animate() {
      if (!isPlaying) {
        animFrameId = null;
        return;
      }
      let particlesSvg = '';
      particles.forEach(p => {
        p.progress += p.speed * speedMultiplier;
        if (p.progress >= 1) {
          p.progress = 0;
          const pathObj = pathElements[p.connId];
          if (pathObj && !prefersReducedMotion()) {
            const targetEl = document.getElementById(`cnode-${pathObj.conn.to}`);
            if (targetEl && Math.random() < 0.3) {
              targetEl.classList.add('pulse-hit');
              setTimeout(() => targetEl.classList.remove('pulse-hit'), 600);
            }
          }
        }
        const pathObj = pathElements[p.connId];
        if (pathObj && pathObj.el) {
          const pt = pathObj.el.getPointAtLength(p.progress * pathObj.length);
          particlesSvg += `
            <g transform="translate(${pt.x}, ${pt.y})">
              <circle r="6" fill="${p.color}" stroke="#000" stroke-width="1.5" filter="url(#particleGlow)" />
              <circle r="2.5" fill="#fff" />
            </g>
          `;
        }
      });
      svgParticlesLayer.innerHTML = particlesSvg;
      animFrameId = requestAnimationFrame(animate);
    }
    animFrameId = requestAnimationFrame(animate);
  }

  // Play / Pause Toggle
  playPauseBtn.addEventListener('click', () => {
    if (isPlaying) stopAnimation();
    else startAnimation();
  });

  // Speed Toggle (1x / 2x)
  speedBtn.addEventListener('click', () => {
    if (speedMultiplier === 1) {
      speedMultiplier = 2;
      speedText.textContent = '⚡ 2x Speed';
      canvasWrapper.classList.add('speed-2x');
    } else {
      speedMultiplier = 1;
      speedText.textContent = '⚡ 1x Speed';
      canvasWrapper.classList.remove('speed-2x');
    }
  });

  // Step Next Button
  stepBtn.addEventListener('click', () => {
    if (isPlaying) stopAnimation();

    const flowDef = canvasFlowDefinitions[currentFlow];
    activeNodeIndex = (activeNodeIndex + 1) % flowDef.nodes.length;
    selectCanvasNode(activeNodeIndex);
  });

  // Reset Button
  resetBtn.addEventListener('click', () => {
    particles.forEach(p => p.progress = 0);
    selectCanvasNode(0);
  });

  // Flow Switcher Tabs (Ingestion vs Retrieval)
  flowTabBtns.forEach(btn => {
    btn.setAttribute('aria-pressed', String(btn.classList.contains('active')));
    btn.addEventListener('click', () => {
      flowTabBtns.forEach(b => {
        b.classList.remove('active');
        b.classList.remove('nb-btn-yellow');
        b.setAttribute('aria-pressed', 'false');
      });
      btn.classList.add('active');
      btn.classList.add('nb-btn-yellow');
      btn.setAttribute('aria-pressed', 'true');
      loadFlow(btn.getAttribute('data-flow'));
    });
  });

  // Initial load
  loadFlow('ingestion');
  if (isPlaying) startAnimation();
  else stopAnimation();
  window.matchMedia('(prefers-reduced-motion: reduce)').addEventListener('change', event => {
    if (event.matches) stopAnimation();
  });
}

function initNav() {
  const toggle = document.getElementById('navToggleBtn');
  const nav = document.getElementById('siteNav');
  if (!toggle || !nav) return;

  toggle.addEventListener('click', () => {
    const open = nav.classList.toggle('is-open');
    toggle.setAttribute('aria-expanded', open ? 'true' : 'false');
    toggle.textContent = open ? 'Close' : 'Menu';
  });

  nav.querySelectorAll('a').forEach(link => {
    link.addEventListener('click', () => {
      nav.classList.remove('is-open');
      toggle.setAttribute('aria-expanded', 'false');
      toggle.textContent = 'Menu';
    });
  });
}

/* ==========================================================================
   3. Live RAG Simulator & Playground
   ========================================================================== */
const mockKnowledgeBase = {
  "how does chunking work?": {
    denseVec: "[0.021, -0.054, 0.118, 0.082, -0.192, ... +763 floats]",
    sparseTerms: 'Example query terms: "chunking" (1.0), "chunk" (1.0), "work" (1.0)',
    answer: "ragcodepilot uses a two-tier chunking architecture:\n\n1. **Go AST Function-Level Chunker** (`internal/ingest/chunker_go.go`) [1]: For Go files, it parses the complete syntax tree using `go/parser`. It isolates function and method declarations along with their receiver, signature, parameter types, and doc comments as units when they fit; declarations longer than 80 lines are split, and syntax errors trigger the generic fallback.\n\n2. **Generic Sliding Window Chunker** (`internal/ingest/chunker.go`) [2]: For other languages (Rust, Python, Shell), it slices files into 40-line windows with a 5-line overlap, using regex pattern matching to extract enclosing symbol names.\n\nChunk enrichment [3] then prepends the file path, language, and chunk type/name metadata to the text before vectorization.",
    citations: [
      { text: "[1] internal/ingest/chunker_go.go:24-105", link: "#" },
      { text: "[2] internal/ingest/chunker.go:32-87", link: "#" },
      { text: "[3] internal/ingest/enrichment.go:18-37", link: "#" }
    ],
    results: [
      {
        file: "internal/ingest/chunker_go.go",
        lines: "24-105",
        name: "chunkGoFile",
        lang: "go",
        type: "function",
        denseScore: 0.892,
        sparseScore: 0.745,
        code: `func chunkGoFile(filePath, repoRoot, repo string, chunkSize, overlap int, cfg *config.Config) ([]model.CodeChunk, error) {
    fset := gotoken.NewFileSet()
    file, parseErr := parser.ParseFile(fset, filePath, src, parser.ParseComments)
    if parseErr != nil {
        return chunkGeneric(filePath, repoRoot, repo, chunkSize, overlap, cfg)
    }
    for _, decl := range file.Decls {
        switch d := decl.(type) {
        case *ast.FuncDecl:
            chunks = append(chunks, namedGoChunks(..., "function", d.Name.Name)...)
        case *ast.GenDecl:
            // type / interface specs → named chunks
        }
    }
    return chunks, nil
}`
      },
      {
        file: "internal/ingest/chunker.go",
        lines: "32-87",
        name: "chunkGeneric (Sliding Window)",
        lang: "go",
        type: "function",
        denseScore: 0.841,
        sparseScore: 0.710,
        code: `// chunkGeneric splits non-Go code files into sliding windows with overlap
func chunkGeneric(filePath, repoRoot, repo string, chunkSize, overlap int, cfg *config.Config) ([]model.CodeChunk, error) {
    lines := strings.Split(string(content), "\\n")

    for start := 0; start < len(lines); start += chunkSize - overlap {
        // build a chunk from lines[start : start+chunkSize],
        // skipping an overlap-only tail chunk at EOF
    }
}`
      },
      {
        file: "internal/ingest/enrichment.go",
        lines: "18-37",
        name: "enrichForEmbedding",
        lang: "go",
        type: "function",
        denseScore: 0.812,
        sparseScore: 0.650,
        code: `func enrichForEmbedding(chunk model.CodeChunk) string {
    var b strings.Builder
    fmt.Fprintf(&b, "File: %s\\n", chunk.FilePath)
    fmt.Fprintf(&b, "Language: %s\\n", chunk.Language)
    label := chunkTypeLabel(chunk.ChunkType)
    if chunk.Name != "" {
        fmt.Fprintf(&b, "%s: %s\\n", label, chunk.Name)
    } else {
        fmt.Fprintf(&b, "Type: %s\\n", label)
    }
    b.WriteString("\\n")
    b.WriteString(chunk.Content)
    return b.String()
}`
      }
    ]
  },

  "where is ChunkFile defined": {
    denseVec: "[0.012, -0.098, 0.045, 0.142, ... +764 floats]",
    sparseTerms: '"chunkfile" (1.0), "defin" (1.0)',
    answer: "`ChunkFile` is defined in `internal/ingest/chunker.go:20-28` [1]. It routes Go files to the AST chunker and everything else to the generic sliding-window chunker (40-line window with 5-line overlap). The AST-based Go chunking itself is `chunkGoFile` in `internal/ingest/chunker_go.go:24` [2].",
    citations: [
      { text: "[1] internal/ingest/chunker.go:20-28", link: "#" },
      { text: "[2] internal/ingest/chunker_go.go:24-105", link: "#" }
    ],
    results: [
      {
        file: "internal/ingest/chunker.go",
        lines: "20-28",
        name: "ChunkFile",
        lang: "go",
        type: "function",
        denseScore: 0.710,
        sparseScore: 0.965,
        code: `// ChunkFile routes Go files to the AST chunker, others to the sliding window
func ChunkFile(filePath, repoRoot, repo string, chunkSize, overlap int, cfg *config.Config) ([]model.CodeChunk, error) {
    if cfg.DetectLanguage(filePath) == "go" {
        return chunkGoFile(filePath, repoRoot, repo, chunkSize, overlap, cfg)
    }
    return chunkGeneric(filePath, repoRoot, repo, chunkSize, overlap, cfg)
}`
      },
      {
        file: "internal/ingest/pipeline.go",
        lines: "190-200",
        name: "pipeline chunk dispatch",
        lang: "go",
        type: "call site",
        denseScore: 0.680,
        sparseScore: 0.750,
        code: `// The ingestion pipeline invokes the chunker for every non-skipped file
chunks, err := ChunkFile(file, absPath, repoName, p.chunkSize, p.chunkOverlap, p.cfg)
if err != nil {
    return fmt.Errorf("chunking %s: %w", file, err)
}`
      }
    ]
  },

  "reciprocal rank fusion implementation": {
    denseVec: "[0.056, 0.112, -0.044, 0.091, ... +764 floats]",
    sparseTerms: '"reciproc" (1.0), "rank" (1.0), "fusion" (1.0)',
    answer: "Hybrid fusion is executed server-side inside Qdrant (`internal/qdrant/client.go`) [1]. Two prefetch stages — dense cosine over the 768d vectors and sparse BM25 — are combined with Reciprocal Rank Fusion using the constant `rrfK = 60` [2]:\n\n`Score(chunk) = 1 / (60 + Rank_dense) + 1 / (60 + Rank_sparse)`\n\nFusion combines positions rather than incompatible raw scores. Positions start at 0 in Qdrant; a candidate absent from a list receives no contribution from it.",
    citations: [
      { text: "[1] internal/qdrant/client.go:336-358", link: "#" },
      { text: "[2] internal/qdrant/client.go:267-269", link: "#" }
    ],
    results: [
      {
        file: "internal/qdrant/client.go",
        lines: "336-358",
        name: "hybrid QueryPoints (RRF)",
        lang: "go",
        type: "query builder",
        denseScore: 0.865,
        sparseScore: 0.880,
        code: `// Hybrid mode: two prefetch stages fused by server-side RRF (k=60)
prefetchLimit := limit * 2
queryPoints = &pb.QueryPoints{
    CollectionName: collection,
    Prefetch: []*pb.PrefetchQuery{
        {
            Query:  pb.NewQueryDense(denseVector),
            Using:  pb.PtrOf("dense"),
            Limit:  pb.PtrOf(prefetchLimit),
            Filter: filter,
        },
        {
            Query:  pb.NewQuerySparse(sparseVector.Indices, sparseVector.Values),
            Using:  pb.PtrOf("sparse"),
            Limit:  pb.PtrOf(prefetchLimit),
            Filter: filter,
        },
    },
    Query:       pb.NewQueryRRF(&pb.Rrf{K: pb.PtrOf(uint32(rrfK))}),
    Limit:       pb.PtrOf(limit),
    WithPayload: pb.NewWithPayload(true),
}`
      }
    ]
  },

  "incremental re-indexing change detection": {
    denseVec: "[0.034, -0.012, 0.155, -0.076, ... +764 floats]",
    sparseTerms: '"increment" (1.0), "reindex" (1.0), "detect" (1.0)',
    answer: "Change detection is file-hash + index version, but it is not per-file embed skip when the corpus moves:\n\n1. **No-op path**: if every file hash and `index_version` match, indexing returns immediately [1][2].\n2. **Any change**: BM25 IDF is corpus-wide, so the pipeline re-chunks and re-embeds **all current files**, then deletes stale points for deleted/renamed paths.\n3. **`--watch`** runs that same pipeline on each debounced save — not a daemon that embeds only the touched file.",
    citations: [
      { text: "[1] internal/ingest/hasher.go:11-18", link: "#" },
      { text: "[2] internal/ingest/pipeline.go", link: "#" }
    ],
    results: [
      {
        file: "internal/ingest/hasher.go",
        lines: "11-18",
        name: "HashFile",
        lang: "go",
        type: "function",
        denseScore: 0.872,
        sparseScore: 0.820,
        code: `// HashFile returns the hex-encoded SHA-256 hash of the file at the given path.
func HashFile(path string) (string, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return "", fmt.Errorf("reading file for hashing: %w", err)
    }
    sum := sha256.Sum256(data)
    return hex.EncodeToString(sum[:]), nil
}`
      }
    ]
  },

  "what happens when embedding dimension doesn't match": {
    denseVec: "[-0.015, 0.088, 0.124, 0.042, ... +764 floats]",
    sparseTerms: '"embed" (1.0), "dimens" (1.0), "match" (1.0)',
    answer: "In `internal/embedding/validate.go` [1], ragcodepilot validates vector batches before upserting or querying. The search path [2] calls `ValidateCollectionVectorSize` to check the query vector against the collection dimension at search time. If a collection was created with 768 dimensions (nomic-embed-text) but the query embedder returns a different dimension (e.g., 1536 from a different embedding model), the CLI returns an error before lookup. Use the matching model or intentionally rebuild the collection for the new model; deletion should not be the first automatic response.",
    citations: [
      { text: "[1] internal/embedding/validate.go:14-38", link: "#" },
      { text: "[2] internal/search/searcher.go:109-112", link: "#" }
    ],
    results: [
      {
        file: "internal/embedding/validate.go",
        lines: "14-38",
        name: "ValidateVectorBatch",
        lang: "go",
        type: "function",
        denseScore: 0.880,
        sparseScore: 0.810,
        code: `// ValidateVectorBatch checks that a batch of vectors is valid and consistent.
//
// Rules:
//   - The batch must not be empty.
//   - No vector may be empty (zero-length).
//   - All vectors in the batch must have the same dimension.
//   - If expectedDim > 0, all vectors must match that dimension.
func ValidateVectorBatch(vectors [][]float32, expectedDim int) (int, error) {
    if len(vectors) == 0 { return 0, fmt.Errorf("empty vector batch") }
    dim := len(vectors[0])
    if dim == 0 { return 0, fmt.Errorf("vector 0 is empty") }
    for i := 1; i < len(vectors); i++ {
        if len(vectors[i]) != dim {
            return 0, fmt.Errorf("inconsistent dimensions in batch: vector 0 has %d, vector %d has %d", dim, i, len(vectors[i]))
        }
    }
    if expectedDim > 0 && dim != expectedDim {
        return 0, fmt.Errorf("dimension mismatch: expected %d, got %d", expectedDim, dim)
    }
    return dim, nil
}`
      }
    ]
  }
};

// Only the small formatting vocabulary in our authored examples is rendered.
// Escape first: neither query text nor source code can introduce HTML.
function formatExampleAnswer(text) {
  const inline = value => escapeHtml(value)
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  return text.split(/\n\n+/).map(paragraph => {
    const lines = paragraph.split('\n');
    if (lines.every(line => /^\d+\. /.test(line))) {
      const first = Number(lines[0].match(/^\d+/)[0]);
      return `<ol start="${first}">${lines.map(line => `<li>${inline(line.replace(/^\d+\. /, ''))}</li>`).join('')}</ol>`;
    }
    return `<p>${inline(paragraph).replace(/\n/g, '<br>')}</p>`;
  }).join('');
}

function initSimulator() {
  const el = id => document.getElementById(id);
  const queryInput = el('simQueryInput');
  const answerToggle = el('simAnswerToggleBtn');
  const answerBox = el('simAnswerBox');
  const modes = [...document.querySelectorAll('#modeToggleGroup [data-mode]')];
  let answerEnabled = false;
  let mode = 'hybrid';

  function render() {
    const query = queryInput.value.trim().toLowerCase();
    const key = Object.keys(mockKnowledgeBase).find(k => k.toLowerCase() === query);
    const data = key ? mockKnowledgeBase[key] : null;
    const dense = mode !== 'sparse';
    const sparse = mode !== 'dense';
    const hybrid = mode === 'hybrid';

    modes.forEach(button => {
      button.classList.toggle('active', button.dataset.mode === mode);
      button.setAttribute('aria-pressed', String(button.dataset.mode === mode));
    });
    answerToggle.setAttribute('aria-pressed', String(answerEnabled));
    answerToggle.classList.toggle('nb-btn-mint', answerEnabled);
    answerToggle.textContent = `💬 --answer ${answerEnabled ? 'ON' : 'OFF'}`;
    el('simEmptyMsg').hidden = Boolean(data);
    answerBox.classList.toggle('active', Boolean(data && answerEnabled));
    // Always clear previous state, including when no fixture matches.
    el('simAnswerText').replaceChildren();
    el('simCitationsList').replaceChildren();
    el('simResultsList').replaceChildren();

    [['simDenseCard', dense], ['simSparseCard', sparse], ['simFusionCard', hybrid]].forEach(([id, runs]) => {
      el(id).classList.toggle('is-skipped', !data || !runs);
    });
    el('simStageEmbed').textContent = !data ? 'No demo selected.' : dense ? data.denseVec : 'Skipped — sparse-only search does not call /api/embed.';
    el('simStageSparse').textContent = !data ? 'No demo selected.' : sparse ? data.sparseTerms : 'Skipped — dense-only search does not tokenize a sparse query.';
    el('simStageRrf').textContent = !data ? 'No demo selected.' : hybrid ? 'Sum 1/(60 + position) for each list. First position = 0.' : 'Skipped — a single retrieval list needs no fusion.';
    el('simStageTiming').textContent = 'Not measured. This page runs no model or database requests.';
    el('simStageStatus').textContent = data ? `${mode.toUpperCase()} EXAMPLE` : 'NO SCRIPTED DEMO';
    el('simStageStatus').className = `nb-badge ${data ? 'mint' : 'pink'}`;
    el('simExecutionPath').textContent = data
      ? `${dense ? 'Ollama embedding' : 'No embedding call'} → ${hybrid ? 'dense + sparse lookup → RRF' : mode + ' lookup'} → ${answerEnabled ? 'top chunks → optional Ollama /api/chat → example answer + sources' : 'ranked code results (no LLM call)'}`
      : 'No scripted result or answer. Choose a preset to continue.';
    el('resultsCountBadge').textContent = '0 Results';
    if (!data) return;

    // Rank only the illustrative fixture candidates. Qdrant performs real fusion.
    const results = data.results.map((r, sourceIndex) => ({ ...r, sourceIndex }));
    const denseOrder = [...results].sort((a, b) => b.denseScore - a.denseScore);
    const sparseOrder = [...results].sort((a, b) => b.sparseScore - a.sparseScore);
    results.forEach(r => {
      r.fusedScore = 1 / (60 + denseOrder.indexOf(r)) + 1 / (60 + sparseOrder.indexOf(r));
    });
    results.sort((a, b) => mode === 'dense' ? b.denseScore - a.denseScore : mode === 'sparse' ? b.sparseScore - a.sparseScore : b.fusedScore - a.fusedScore);
    el('resultsCountBadge').textContent = `${results.length} Results · ${mode.toUpperCase()}`;
    el('simResultsList').innerHTML = results.map((r, rank) => `
      <article class="result-card" id="sim-source-${r.sourceIndex}" tabindex="-1">
        <div class="result-card-header">
          <div class="result-file"><span class="nb-badge mint">#${rank + 1}</span><span>${escapeHtml(r.file)}:${escapeHtml(r.lines)}</span><span class="nb-badge">${escapeHtml(r.name)}</span></div>
          <div class="score-breakdown"><span class="nb-badge ${hybrid ? 'yellow' : dense ? 'blue' : 'orange'}">${hybrid ? `RRF: ${r.fusedScore.toFixed(5)}` : dense ? `Cosine: ${r.denseScore}` : `BM25: ${r.sparseScore}`}</span><span>Illustrative score</span></div>
          <a href="${sourceUrl(r.file, r.lines)}" target="_blank" rel="noopener">View source ↗</a>
        </div><div class="result-code-view">${escapeHtml(r.code)}</div>
      </article>`).join('');

    if (answerEnabled) {
      el('simAnswerText').innerHTML = formatExampleAnswer(data.answer);
      data.citations.forEach(citation => {
        const match = citation.text.match(/^\[\d+\] ([^: ]+)(?::([\d-]+))?$/);
        if (!match) return;
        const [, file, lines = ''] = match;
        const index = data.results.findIndex(r => r.file === file && (!lines || r.lines === lines));
        const link = document.createElement('a');
        link.className = 'citation-pill';
        link.textContent = citation.text + (index < 0 ? ' ↗' : ' ↓');
        if (index >= 0) {
          link.href = `#sim-source-${index}`;
          link.addEventListener('click', event => {
            event.preventDefault();
            const target = el(`sim-source-${index}`);
            target.focus({ preventScroll: true });
            target.scrollIntoView({ behavior: prefersReducedMotion() ? 'instant' : 'smooth', block: 'center' });
          });
        } else {
          link.href = sourceUrl(file, lines);
          link.target = '_blank';
          link.rel = 'noopener';
        }
        el('simCitationsList').append(link);
      });
    }
  }

  modes.forEach(button => button.addEventListener('click', () => { mode = button.dataset.mode; render(); }));
  answerToggle.addEventListener('click', () => { answerEnabled = !answerEnabled; render(); });
  document.querySelectorAll('.preset-btn').forEach(button => button.addEventListener('click', () => {
    queryInput.value = button.dataset.query;
    render();
  }));
  el('simRunBtn').addEventListener('click', render);
  queryInput.addEventListener('keydown', event => { if (event.key === 'Enter') render(); });
  render();
}

/* ==========================================================================
   4. Technical Deep Dives
   ========================================================================== */
function initDeepDives() {
  const tabBtns = document.querySelectorAll('.dd-tab-btn');
  const panels = document.querySelectorAll('.deep-dive-panel');

  tabBtns.forEach(btn => {
    btn.setAttribute('aria-pressed', String(btn.classList.contains('active')));
    btn.addEventListener('click', () => {
      tabBtns.forEach(b => {
        b.classList.remove('active');
        b.classList.remove('nb-btn-yellow');
        b.setAttribute('aria-pressed', 'false');
      });
      btn.classList.add('active');
      btn.classList.add('nb-btn-yellow');
      btn.setAttribute('aria-pressed', 'true');

      const targetId = `dd-${btn.getAttribute('data-tab')}`;
      panels.forEach(p => {
        if (p.id === targetId) {
          p.classList.add('active');
        } else {
          p.classList.remove('active');
        }
      });
    });
  });
}

function initWorkedExamples() {
  const success = document.getElementById('walkSuccess');
  const failure = document.getElementById('walkFailure');
  function showScenario(failed) {
    success.setAttribute('aria-pressed', String(!failed));
    failure.setAttribute('aria-pressed', String(failed));
    success.classList.toggle('nb-btn-yellow', !failed);
    failure.classList.toggle('nb-btn-yellow', failed);
    document.getElementById('walkFailureMessage').hidden = !failed;
    document.getElementById('walkRetrievalText').hidden = failed;
    document.getElementById('walkRecordedResult').hidden = failed;
    document.getElementById('walkQueryTitle').textContent = failed ? '4. Validate the query dimension' : '4. Retrieve the stored chunk';
    document.getElementById('walkOutputStage').hidden = failed;
    document.getElementById('walkQueryStage').classList.toggle('has-failure', failed);
    document.getElementById('walkScenarioStatus').textContent = failed
      ? 'Failure scenario: the existing index remains stored, but validation stops this search before lookup or generation.'
      : 'Success: index → retrieve → code results → optional cited answer.';
  }
  success.addEventListener('click', () => showScenario(false));
  failure.addEventListener('click', () => showScenario(true));

  const candidates = [
    { name: 'Conceptual overview', dense: 0, sparse: null },
    { name: 'ChunkFile definition', dense: 1, sparse: 0 },
    { name: 'Pipeline call site', dense: 2, sparse: 1 }
  ];
  const contribution = position => position === null ? 0 : 1 / (60 + position);
  function showRanks(mode) {
    document.querySelectorAll('[data-rrf-mode]').forEach(button => {
      const active = button.dataset.rrfMode === mode;
      button.setAttribute('aria-pressed', String(active));
      button.classList.toggle('nb-btn-yellow', active);
    });
    const rows = candidates.map(candidate => ({ ...candidate, score: contribution(candidate.dense) + contribution(candidate.sparse) }));
    rows.sort((a, b) => mode === 'hybrid' ? b.score - a.score : (a[mode] ?? Infinity) - (b[mode] ?? Infinity));
    document.getElementById('rrfExampleBody').innerHTML = rows.map((row, i) => {
      const formula = [row.dense, row.sparse].map(position => position === null ? '0 (absent)' : `1/${60 + position}`).join(' + ');
      return `<tr><th scope="row">${row.name}</th><td>${row.dense ?? 'Absent'}</td><td>${row.sparse ?? 'Absent'}</td><td>${formula} = ${row.score.toFixed(5)}</td><td>${mode !== 'hybrid' && row[mode] === null ? 'Not retrieved' : `#${i + 1}`}</td></tr>`;
    }).join('');
    document.getElementById('rrfExplanation').textContent = mode === 'hybrid'
      ? 'ChunkFile wins: appearing near the top of both lists beats appearing first in only one. No raw cosine or BM25 scores are added.'
      : `${mode === 'dense' ? 'Dense ranks the conceptual overview first.' : 'Sparse ranks the exact ChunkFile definition first; the overview is absent.'} RRF is skipped in this mode; the calculation column stays visible only for comparison.`;
  }
  document.querySelectorAll('[data-rrf-mode]').forEach(button => button.addEventListener('click', () => showRanks(button.dataset.rrfMode)));
  showRanks('hybrid');
}

/* ==========================================================================
   5. Evaluation Scoreboard Table
   Outcomes are generated from the complete saved retrieval report.
   ========================================================================== */
const goldenQueries = SITE_EVIDENCE.retrieval_report.queries.map(query => {
  const negative = query.type === 'negative';
  const hit = negative ? query.negative.pass : query.hit_at_5;
  const outcome = negative
    ? `${hit ? 'Pass' : 'Fail'} — ${query.negative.score_kind} threshold ${query.negative.threshold}`
    : query.hit_at_1 ? 'Hit at rank #1' : query.hit_at_5 ? 'Hit in top 5 (not #1)' : 'Miss in top 5';
  return { query: query.query, type: query.type, file: query.top_results[0]?.file_path || 'No results', hit, outcome };
});

function initEvalTable() {
  const tbody = document.getElementById('evalTableBody');
  tbody.innerHTML = goldenQueries.map(q => {
    let typeBadge = 'yellow';
    if (q.type === 'concept') typeBadge = 'blue';
    if (q.type === 'behavior') typeBadge = 'orange';
    if (q.type === 'negative') typeBadge = 'pink';

    let outcomeBadge;
    if (q.type === 'negative') {
      outcomeBadge = q.hit
        ? `<span class="nb-badge mint">${escapeHtml(q.outcome)}</span>`
        : `<span class="nb-badge pink">${escapeHtml(q.outcome)}</span>`;
    } else if (q.hit) {
      outcomeBadge = `<strong style="color: #10b981;">${escapeHtml(q.outcome)}</strong>`;
    } else {
      outcomeBadge = `<strong style="color: #e11d48;">${escapeHtml(q.outcome)}</strong>`;
    }

    return `
      <tr>
        <td style="font-weight: 700;">"${escapeHtml(q.query)}"</td>
        <td><span class="nb-badge ${typeBadge}">${escapeHtml(q.type)}</span></td>
        <td><code>${escapeHtml(q.file)}</code></td>
        <td>${outcomeBadge}</td>
      </tr>
    `;
  }).join('');
}

/* ==========================================================================
   6. Interactive Terminal Emulator
   ========================================================================== */
const terminalScripts = {
  "search-answer": [
    "$ go run ./cmd/ragcodepilot search --answer \"how does chunking work?\"",
    "Warming up generative model (first call may take a while)...",
    "",
    "Answer: Go files use go/parser to form named function, method, type, and interface chunks [1]. Declarations over 80 lines are split. Other languages, and Go parse failures, use 40-line windows with 5-line overlap [2].",
    "",
    "Sources (abbreviated illustration):",
    "[1] internal/ingest/chunker_go.go — chunkGoFile",
    "[2] internal/ingest/chunker.go — ChunkFile / chunkGeneric",
    "",
    "This transcript is illustrative. No model or database is called here; no latency is measured."
  ],

  "hybrid-search": [
    "$ go run ./cmd/ragcodepilot search --mode hybrid --limit 3 \"embedding interface\"",
    "[info] Mode: HYBRID (Dense + BM25 RRF, k=60)",
    "[info] Ollama nomic-embed-text (768d) query vector generated (illustrative output)",
    "",
    "RANK #1  [RRF: 0.0328]  internal/embedding/embedder.go",
    "  type Embedder interface {",
    "      Embed(ctx context.Context, texts []string) ([][]float32, error)",
    "      Dimension() int",
    "  }",
    "",
    "RANK #2  [RRF: 0.0315]  internal/embedding/ollama.go:45-78",
    "  func (e *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {",
    "      // HTTP client calling /api/embed",
    "  }",
    "",
    "RANK #3  [RRF: 0.0298]  internal/embedding/validate.go:15-40",
    "  func ValidateVectorBatch(vectors [][]float32, expectedDim int) (int, error)"
  ],

  "index": [
    "$ go run ./cmd/ragcodepilot index --language go .",
    "Using Ollama embedder (model: nomic-embed-text, url: http://localhost:11434)",
    "Filtering to languages: go",
    "Found 28 source files in ragsearch",
    "Change detection: 0 unchanged, 0 changed, 28 new, 0 stale, 0 index-version refresh",
    "Generated 199 chunks from 28 files",
    "Indexed 199/199 chunks",
    "Successfully indexed 199 chunks into collection \"code_chunks\""
  ],

  "watch": [
    "$ go run ./cmd/ragcodepilot index --language go --watch .",
    "Using Ollama embedder (model: nomic-embed-text)",
    "Found 28 source files in ragsearch",
    "Change detection: 28 unchanged, 0 changed, 0 new, 0 stale, 0 index-version refresh",
    "Everything up to date — nothing to index",
    "[watch] listening for changes (same pipeline as one-shot index)...",
    "",
    "[event] WRITE internal/ingest/chunker_go.go",
    "Change detection: 0 unchanged, 1 changed, 0 new, 0 stale, 0 index-version refresh",
    "Generated 199 chunks from 28 files",
    "[note] any file change re-embeds ALL current files (BM25 IDF is corpus-wide)",
    "Successfully indexed 199 chunks into collection \"code_chunks\""
  ],

  "eval": [
    '$ go run ./cmd/ragcodepilot eval --mode hybrid --output human',
    'Recorded report summary (not a live execution):',
    `Source: ${SITE_EVIDENCE.retrieval}`,
    `Run: ${SITE_EVIDENCE.retrieval_report.run_id}`,
    ...Object.entries(SITE_EVIDENCE.retrieval_report.aggregate).map(([key, value]) =>
      `${key}: ${Number.isInteger(value) ? value : value.toFixed(3)}`)
  ]
};

function initTerminal() {
  const terminalBody = document.getElementById('terminalOutput');
  const tabBtns = document.querySelectorAll('.terminal-tab-btn');
  const copyBtn = document.getElementById('copyTerminalCmdBtn');
  let currentCmd = 'search-answer';
  let animationInterval = null;

  function renderTerminal(cmdKey) {
    currentCmd = cmdKey;
    if (animationInterval) clearInterval(animationInterval);

    const lines = terminalScripts[cmdKey];
    terminalBody.textContent = '';
    if (prefersReducedMotion()) {
      terminalBody.textContent = lines.join('\n');
      terminalBody.scrollTop = terminalBody.scrollHeight;
      return;
    }
    let lineIdx = 0;

    animationInterval = setInterval(() => {
      if (lineIdx < lines.length) {
        terminalBody.textContent += lines[lineIdx] + '\n';
        terminalBody.scrollTop = terminalBody.scrollHeight;
        lineIdx++;
      } else {
        clearInterval(animationInterval);
      }
    }, 45);
  }

  tabBtns.forEach(btn => {
    btn.setAttribute('aria-pressed', String(btn.classList.contains('active')));
    btn.addEventListener('click', () => {
      tabBtns.forEach(b => { b.classList.remove('active'); b.setAttribute('aria-pressed', 'false'); });
      btn.classList.add('active');
      btn.setAttribute('aria-pressed', 'true');
      renderTerminal(btn.getAttribute('data-cmd'));
    });
  });

  copyBtn.addEventListener('click', () => {
    const lines = terminalScripts[currentCmd];
    const cmdLine = lines[0].replace('$ ', '');
    copyCommand(cmdLine, copyBtn);
  });

  renderTerminal('search-answer');
}

/* ==========================================================================
   7. Quick Actions
   ========================================================================== */
function initQuickActions() {
  const copyBtn = document.getElementById('copyCliQuickstartBtn');
  if (copyBtn) {
    copyBtn.addEventListener('click', () => {
      copyCommand(document.getElementById('quickstartCommand').textContent, copyBtn);
    });
  }
}

async function copyCommand(text, button) {
  const original = button.textContent;
  try {
    await navigator.clipboard.writeText(text);
    button.textContent = 'Copied! ✓';
    document.getElementById('copyStatus').textContent = 'Command copied to clipboard.';
  } catch {
    button.textContent = 'Select command to copy';
    document.getElementById('copyStatus').textContent = 'Clipboard unavailable. Select and copy the displayed command manually.';
  }
  setTimeout(() => { button.textContent = original; }, 2000);
}

function escapeHtml(str) {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

/* ==========================================================================
   8. Nav Scrollspy (keeps the active nav pill in sync with scroll position)
   ========================================================================== */
function initScrollSpy() {
  const navLinks = document.querySelectorAll('.nav-pills .nav-link');
  if (!navLinks.length || !('IntersectionObserver' in window)) return;

  const linkBySection = new Map();
  navLinks.forEach(link => {
    const hash = link.getAttribute('href');
    if (hash && hash.startsWith('#')) {
      const section = document.querySelector(hash);
      if (section) linkBySection.set(section, link);
    }
  });

  const observer = new IntersectionObserver(entries => {
    entries.forEach(entry => {
      if (!entry.isIntersecting) return;
      const activeLink = linkBySection.get(entry.target);
      if (!activeLink) return;
      navLinks.forEach(l => l.classList.remove('active'));
      activeLink.classList.add('active');
    });
  }, { rootMargin: '-35% 0px -55% 0px', threshold: 0 });

  linkBySection.forEach((_, section) => observer.observe(section));
}
