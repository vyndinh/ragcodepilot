/* ==========================================================================
   ragcodepilot — Modern Neobrutalism Interactive Application
   ========================================================================== */

document.addEventListener('DOMContentLoaded', () => {
  initTheme();
  initNav();
  initPipeline();
  initSimulator();
  initDeepDives();
  initEvalTable();
  initTerminal();
  initQuickActions();
  initScrollSpy();
});

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
      desc: 'The indexing entry point: a local checkout of a Git repository. Nothing ever leaves this machine — walking, hashing, chunking, and embedding all run locally under the language and skip rules defined in config.yaml.',
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
      desc: 'Uses go/parser to extract function/method declarations and named type/interface specs. Remaining imports and vars become block chunks. Syntax errors fall back to the generic sliding window.',
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
      desc: 'Prepends File, Language, and Function/Type/Interface (or Type: Block) before embedding. Qdrant still stores the raw code; only the embedder sees the header.',
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
      desc: 'Calls Ollama HTTP API (nomic-embed-text) to generate 768-dimensional dense semantic vectors. Concurrently builds a BM25 sparse vector with Snowball stemming and CRC32-hashed terms (internal/embedding/sparse.go).',
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
      symbol: 'UpsertChunks(ctx, collection, points)',
      pkg: 'internal/qdrant/client.go',
      input: 'Multi-vector points with payloads',
      output: 'Qdrant points stored & indexes synced',
      desc: 'Batches chunks into Qdrant via high-performance gRPC. Stores dual vectors ("dense" + "sparse") and indexes payload fields (file_hash, language, repo, index_version). Also deletes stale orphaned chunks.',
      code: `// UpsertChunks stores dual-vector points in Qdrant over gRPC
func (c *Client) UpsertChunks(ctx context.Context, coll string, points []*Point) error {
    qdrantPoints := make([]*qdrant.PointStruct, len(points))
    for i, p := range points {
        qdrantPoints[i] = &qdrant.PointStruct{
            Id: &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: p.ID}},
            Vectors: &qdrant.Vectors{
                VectorsOptions: &qdrant.Vectors_Vectors{
                    Vectors: &qdrant.NamedVectors{
                        Vectors: map[string]*qdrant.Vector{
                            "dense":  {Data: p.DenseVector},
                            "sparse": {Data: p.SparseVector},
                        },
                    },
                },
            },
            Payload: p.Payload,
        }
    }
    return c.grpc.Upsert(ctx, coll, qdrantPoints)
}`
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
      output: 'Sparse vector (CRC32 term hashes + BM25 weights)',
      desc: 'Splits the query on word boundaries and camelCase/handle_case shapes, stems tokens with Snowball, hashes them to 32-bit term IDs, and weights them with BM25 IDF statistics computed over the indexed corpus.',
      code: `// TokenizeQuery produces the sparse BM25 representation of a query
func TokenizeQuery(query string) SparseVector {
    tokens := tokenize(query, true) // includes Snowball stems
    return sparseFromTokens(tokens)
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
      desc: 'Fusion runs server-side inside Qdrant in the same gRPC call: Reciprocal Rank Fusion with rrfK = 60 combines the dense and sparse rank orders, Score = 1/(60 + R_dense) + 1/(60 + R_sparse). No client-side score normalization needed.',
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
    // POST /api/generate { model: "qwen2.5-coder:7b", ... }
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
      { id: 'search-answer', detail: 'search-answer', x: 1020, y: 145, w: 105, h: 76, icon: '🤖', name: 'Ollama LLM', sub: 'qwen2.5-coder:7b', pkg: 'answer/ollama.go', color: '#d8b4fe', step: 7 }
    ],
    connections: [
      { id: 'sconn-0-1a', from: 'search-query', to: 'search-ollama', d: 'M 155 170 C 175 170, 175 83, 195 83', color: 'blue', marker: 'arrowBlue', speed: 1 },
      { id: 'sconn-0-1b', from: 'search-query', to: 'search-bm25', d: 'M 155 196 C 175 196, 175 283, 195 283', color: 'orange', marker: 'arrowOrange', speed: 1 },
      { id: 'sconn-1a-2', from: 'search-ollama', to: 'search-filter', d: 'M 335 83 C 355 83, 355 170, 375 170', color: 'blue', marker: 'arrowPink', speed: 1 },
      { id: 'sconn-1b-2', from: 'search-bm25', to: 'search-filter', d: 'M 335 283 C 355 283, 355 196, 375 196', color: 'orange', marker: 'arrowPink', speed: 1 },
      { id: 'sconn-2-3', from: 'search-filter', to: 'search-qdrant', d: 'M 500 183 L 535 183', color: 'pink', marker: 'arrowBlue', speed: 1 },
      { id: 'sconn-3-4', from: 'search-qdrant', to: 'search-rrf', d: 'M 665 183 L 700 183', color: 'blue', marker: 'arrowMint', speed: 1 },
      { id: 'sconn-4-5', from: 'search-rrf', to: 'search-context', d: 'M 830 183 L 865 183', color: 'mint', marker: 'arrowYellow', speed: 1 },
      { id: 'sconn-5-6', from: 'search-context', to: 'search-answer', d: 'M 990 183 L 1020 183', color: 'yellow', marker: 'arrowPink', speed: 1 }
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
      <g class="canvas-node-group ${idx === 0 ? 'active' : ''}" id="cnode-${node.id}" data-node-id="${node.id}" transform="translate(${node.x}, ${node.y})" tabindex="0" role="button" aria-label="${node.name}: ${node.sub}">
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
        <button type="button" class="pipeline-mobile-step${idx === activeNodeIndex ? ' active' : ''}" data-idx="${idx}">
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
    document.querySelectorAll('.canvas-node-group').forEach(n => n.classList.remove('active'));
    const activeG = document.getElementById(`cnode-${node.id}`);
    if (activeG) {
      activeG.classList.add('active');
      if (!prefersReducedMotion()) {
        activeG.classList.add('pulse-hit');
        setTimeout(() => activeG.classList.remove('pulse-hit'), 600);
      }
    }
    if (mobileList) {
      mobileList.querySelectorAll('.pipeline-mobile-step').forEach((btn, i) => {
        btn.classList.toggle('active', i === idx);
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
    }

    updateTicker();
  }

  function updateTicker() {
    const flowDef = canvasFlowDefinitions[currentFlow];
    const node = flowDef.nodes[activeNodeIndex];
    if (currentFlow === 'ingestion') {
      tickerText.textContent = `STREAM: [${node.name}] Active ➔ Processing ${node.pkg} ➔ Ingesting chunks to Qdrant (gRPC:6334)...`;
    } else {
      tickerText.textContent = `STREAM: [${node.name}] Active ➔ RAG Pipeline ➔ Hybrid RRF Fusion (k=60) ➔ qwen2.5-coder:7b...`;
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
    btn.addEventListener('click', () => {
      flowTabBtns.forEach(b => {
        b.classList.remove('active');
        b.classList.remove('nb-btn-yellow');
      });
      btn.classList.add('active');
      btn.classList.add('nb-btn-yellow');
      loadFlow(btn.getAttribute('data-flow'));
    });
  });

  // Initial load
  loadFlow('ingestion');
  if (isPlaying) startAnimation();
  else stopAnimation();
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
    denseScore: 0.892,
    sparseScore: 0.745,
    rrfScore: 0.0328,
    denseVec: "[0.021, -0.054, 0.118, 0.082, -0.192, ... +763 floats]",
    sparseTerms: '"chunk" (1.82), "work" (0.94), "ast" (1.45)',
    timing: "Total: 124ms (Embed: 38ms, Qdrant: 22ms, LLM: 64ms)",
    answer: "ragcodepilot uses a two-tier chunking architecture:\n\n1. **Go AST Function-Level Chunker** (`internal/ingest/chunker_go.go`) [1]: For Go files, it parses the complete syntax tree using `go/parser`. It isolates function and method declarations along with their receiver, signature, parameter types, and doc comments as complete cohesive units.\n\n2. **Generic Sliding Window Chunker** (`internal/ingest/chunker.go`) [2]: For other languages (Rust, Python, Shell), it slices files into 40-line windows with a 5-line overlap, using regex pattern matching to extract enclosing symbol names.\n\nChunk enrichment [3] then prepends the file path, package, and signature metadata to the text before vectorization.",
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
        rrfRank: 1,
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
        rrfRank: 2,
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
        rrfRank: 3,
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
    denseScore: 0.710,
    sparseScore: 0.965,
    rrfScore: 0.0325,
    denseVec: "[0.012, -0.098, 0.045, 0.142, ... +764 floats]",
    sparseTerms: '"chunkfile" (3.12), "defin" (1.05)',
    timing: "Total: 98ms (Embed: 31ms, Qdrant: 18ms, LLM: 49ms)",
    answer: "`ChunkFile` is defined in `internal/ingest/chunker.go:23-28` [1]. It routes Go files to the AST chunker and everything else to the generic sliding-window chunker (40-line window with 5-line overlap). The AST-based Go chunking itself is `chunkGoFile` in `internal/ingest/chunker_go.go:24` [2].",
    citations: [
      { text: "[1] internal/ingest/chunker.go:23-28", link: "#" },
      { text: "[2] internal/ingest/chunker_go.go:24-105", link: "#" }
    ],
    results: [
      {
        file: "internal/ingest/chunker.go",
        lines: "23-28",
        name: "ChunkFile",
        lang: "go",
        type: "function",
        denseScore: 0.710,
        sparseScore: 0.965,
        rrfRank: 1,
        code: `// ChunkFile routes Go files to the AST chunker, others to the sliding window
func ChunkFile(filePath, repoRoot, repo string, chunkSize, overlap int, cfg *config.Config) ([]model.CodeChunk, error) {
    if isGoFile(filePath) {
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
        rrfRank: 2,
        code: `// The ingestion pipeline invokes the chunker for every non-skipped file
chunks, err := ChunkFile(file, absPath, repoName, p.chunkSize, p.chunkOverlap, p.cfg)
if err != nil {
    return fmt.Errorf("chunking %s: %w", file, err)
}`
      }
    ]
  },

  "reciprocal rank fusion implementation": {
    denseScore: 0.865,
    sparseScore: 0.880,
    rrfScore: 0.0327,
    denseVec: "[0.056, 0.112, -0.044, 0.091, ... +764 floats]",
    sparseTerms: '"reciproc" (2.1), "rank" (1.8), "fusion" (2.4)',
    timing: "Total: 115ms (Embed: 35ms, Qdrant: 24ms, LLM: 56ms)",
    answer: "Hybrid fusion is executed server-side inside Qdrant (`internal/qdrant/client.go`) [1]. Two prefetch stages — dense cosine over the 768d vectors and sparse BM25 — are combined with Reciprocal Rank Fusion using the constant `rrfK = 60` [2]:\n\n`Score(chunk) = 1 / (60 + Rank_dense) + 1 / (60 + Rank_sparse)`\n\nThis prevents dense scores (which cluster near 0.8-0.9) from overwhelming sparse keyword spikes without requiring fragile score calibration.",
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
        rrfRank: 1,
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
    denseScore: 0.872,
    sparseScore: 0.820,
    rrfScore: 0.0326,
    denseVec: "[0.034, -0.012, 0.155, -0.076, ... +764 floats]",
    sparseTerms: '"increment" (1.9), "reindex" (2.2), "detect" (1.1)',
    timing: "Total: 120ms (Embed: 36ms, Qdrant: 26ms, LLM: 58ms)",
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
        rrfRank: 1,
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
    denseScore: 0.880,
    sparseScore: 0.810,
    rrfScore: 0.0324,
    denseVec: "[-0.015, 0.088, 0.124, 0.042, ... +764 floats]",
    sparseTerms: '"embed" (1.4), "dimens" (2.5), "match" (1.2)',
    timing: "Total: 105ms (Embed: 33ms, Qdrant: 22ms, LLM: 50ms)",
    answer: "In `internal/embedding/validate.go` [1], ragcodepilot performs pre-flight vector validation before upserting or querying, and `ValidateCollectionVectorSize` double-checks the query vector against the collection dimension at search time. If a collection was created with 768 dimensions (nomic-embed-text) but an embedder returns a different count (e.g., 1536 from OpenAI), the CLI halts with an explicit error explaining how to delete or re-create the collection via `ragcodepilot collections delete`.",
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
        rrfRank: 1,
        code: `// ValidateVectorBatch checks that a batch of vectors is valid and consistent.
//
// Rules:
//   - The batch must not be empty.
//   - No vector may be empty (zero-length).
//   - All vectors in the batch must have the same dimension.
//   - If expectedDim > 0, all vectors must match that dimension.
func ValidateVectorBatch(vectors [][]float32, expectedDim int) (int, error) {
    dim := len(vectors[0])
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

function initSimulator() {
  const queryInput = document.getElementById('simQueryInput');
  const runBtn = document.getElementById('simRunBtn');
  const answerToggleBtn = document.getElementById('simAnswerToggleBtn');
  const answerBox = document.getElementById('simAnswerBox');
  const answerText = document.getElementById('simAnswerText');
  const citationsList = document.getElementById('simCitationsList');
  const resultsList = document.getElementById('simResultsList');
  const resultsCountBadge = document.getElementById('resultsCountBadge');

  const stageEmbed = document.getElementById('simStageEmbed');
  const stageSparse = document.getElementById('simStageSparse');
  const stageRrf = document.getElementById('simStageRrf');
  const stageTiming = document.getElementById('simStageTiming');
  const stageStatus = document.getElementById('simStageStatus');

  let answerModeActive = true;
  let currentMode = 'hybrid'; // 'hybrid' | 'dense' | 'sparse'

  // Mode buttons
  const modeButtons = document.querySelectorAll('#modeToggleGroup .pill-toggle-btn');
  modeButtons.forEach(btn => {
    btn.addEventListener('click', () => {
      modeButtons.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      currentMode = btn.getAttribute('data-mode');
      runSimulation(queryInput.value.trim());
    });
  });

  // Answer mode toggle
  answerToggleBtn.addEventListener('click', () => {
    answerModeActive = !answerModeActive;
    if (answerModeActive) {
      answerToggleBtn.classList.add('nb-btn-mint');
      answerToggleBtn.innerHTML = '<span>💬 --answer ON</span>';
      answerBox.classList.add('active');
    } else {
      answerToggleBtn.classList.remove('nb-btn-mint');
      answerToggleBtn.innerHTML = '<span>💬 --answer OFF</span>';
      answerBox.classList.remove('active');
    }
  });

  // Preset query pills
  document.querySelectorAll('.preset-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      queryInput.value = btn.getAttribute('data-query');
      runSimulation(queryInput.value);
    });
  });

  runBtn.addEventListener('click', () => {
    runSimulation(queryInput.value.trim());
  });

  queryInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') runSimulation(queryInput.value.trim());
  });

  const emptyMsg = document.getElementById('simEmptyMsg');

  function runSimulation(query) {
    if (!query) query = "how does chunking work?";

    const keys = Object.keys(mockKnowledgeBase);
    const exact = keys.find(k => k.toLowerCase() === query.toLowerCase());
    const data = exact ? mockKnowledgeBase[exact] : null;

    if (!data) {
      if (emptyMsg) emptyMsg.hidden = false;
      stageStatus.textContent = "NO SCRIPTED DEMO";
      stageStatus.className = "nb-badge pink";
      stageEmbed.textContent = "—";
      stageSparse.textContent = "—";
      stageRrf.textContent = "—";
      stageTiming.textContent = "This playground does not call Qdrant or Ollama.";
      answerBox.classList.remove('active');
      resultsList.innerHTML = '';
      resultsCountBadge.textContent = "0 Results";
      return;
    }
    if (emptyMsg) emptyMsg.hidden = true;

    // Update stages
    stageEmbed.textContent = data.denseVec;
    stageSparse.textContent = data.sparseTerms;
    stageTiming.textContent = data.timing;

    if (currentMode === 'hybrid') {
      stageRrf.textContent = "Score = 1/(60+R_dense) + 1/(60+R_sparse)";
      stageStatus.textContent = "HYBRID RRF SUCCESS";
      stageStatus.className = "nb-badge mint";
    } else if (currentMode === 'dense') {
      stageRrf.textContent = "Cosine Similarity (Dense Only)";
      stageStatus.textContent = "DENSE LOOKUP SUCCESS";
      stageStatus.className = "nb-badge blue";
    } else {
      stageRrf.textContent = "BM25 Sparse Weight Matching";
      stageStatus.textContent = "SPARSE LOOKUP SUCCESS";
      stageStatus.className = "nb-badge orange";
    }

    // Update Answer Box
    if (answerModeActive) {
      answerBox.classList.add('active');
      answerText.textContent = data.answer;
      citationsList.innerHTML = data.citations.map(c => `
        <span class="citation-pill">${c.text}</span>
      `).join('');
    } else {
      answerBox.classList.remove('active');
    }

    // Sort results based on mode
    let results = [...data.results];
    if (currentMode === 'dense') {
      results.sort((a, b) => b.denseScore - a.denseScore);
    } else if (currentMode === 'sparse') {
      results.sort((a, b) => b.sparseScore - a.sparseScore);
    }

    resultsCountBadge.textContent = `${results.length} Results Ranked (${currentMode.toUpperCase()})`;

    // Render results cards
    resultsList.innerHTML = results.map((r, i) => {
      let scoreBadge = '';
      if (currentMode === 'hybrid') {
        scoreBadge = `<span class="nb-badge yellow">RRF Rank #${i + 1}</span> <span style="color: var(--text-muted);">Dense: ${r.denseScore} · BM25: ${r.sparseScore}</span>`;
      } else if (currentMode === 'dense') {
        scoreBadge = `<span class="nb-badge blue">Cosine Score: ${r.denseScore}</span>`;
      } else {
        scoreBadge = `<span class="nb-badge orange">BM25 Score: ${r.sparseScore}</span>`;
      }

      return `
        <div class="result-card">
          <div class="result-card-header">
            <div class="result-file">
              <span class="nb-badge ${i === 0 ? 'mint' : 'yellow'}">#${i + 1}</span>
              <span>📄 ${r.file}:${r.lines}</span>
              <span class="nb-badge" style="font-size: 0.7rem;">${r.name}</span>
            </div>
            <div class="score-breakdown">
              ${scoreBadge}
            </div>
          </div>
          <div class="result-code-view">${escapeHtml(r.code)}</div>
        </div>
      `;
    }).join('');
  }

  // Run initial simulation
  runSimulation("how does chunking work?");
}

/* ==========================================================================
   4. Technical Deep Dives
   ========================================================================== */
function initDeepDives() {
  const tabBtns = document.querySelectorAll('.dd-tab-btn');
  const panels = document.querySelectorAll('.deep-dive-panel');

  tabBtns.forEach(btn => {
    btn.addEventListener('click', () => {
      tabBtns.forEach(b => {
        b.classList.remove('active');
        b.classList.remove('nb-btn-yellow');
      });
      btn.classList.add('active');
      btn.classList.add('nb-btn-yellow');

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

/* ==========================================================================
   5. Evaluation Scoreboard Table
   Sample rows from docs/eval/baseline_v8.json (hybrid mode, 2026-09-11,
   199 chunks). 34/35 positive queries hit@5. ChunkFile navigation is top-1
   after additive identifier tokens. Negative pass is 0.50 under RRF.
   ========================================================================== */
const goldenQueries = [
  {
    query: "how does code chunking work",
    type: "concept",
    file: "internal/ingest/chunker_go.go",
    outcome: "Hit at rank #1",
    hit: true
  },
  {
    query: "EnsureCollection function on Qdrant client",
    type: "navigation",
    file: "internal/qdrant/client.go",
    outcome: "Hit at rank #1",
    hit: true
  },
  {
    query: "how is code enriched before embedding",
    type: "concept",
    file: "internal/ingest/enrichment.go",
    outcome: "Hit at rank #1",
    hit: true
  },
  {
    query: "what happens when the embedder returns inconsistent vector dimensions",
    type: "behavior",
    file: "internal/embedding/validate.go",
    outcome: "Hit in top 5 (not #1)",
    hit: true
  },
  {
    query: "how does the system detect changed files when re-indexing",
    type: "behavior",
    file: "internal/ingest/pipeline.go",
    outcome: "Hit at rank #1",
    hit: true
  },
  {
    query: "where is HitAtK function implemented",
    type: "navigation",
    file: "internal/eval/metrics.go",
    outcome: "Hit at rank #1",
    hit: true
  },
  {
    query: "where is LoadDataset implemented",
    type: "navigation",
    file: "internal/eval/dataset.go",
    outcome: "Hit at rank #1",
    hit: true
  },
  {
    query: "where is ChunkFile defined",
    type: "navigation",
    file: "internal/ingest/chunker.go",
    outcome: "Hit at rank #1",
    hit: true
  },
  {
    query: "where is the OAuth middleware implemented",
    type: "negative",
    file: "— (not in corpus)",
    outcome: "Fail — dual-list RRF (ceiling 0.02)",
    hit: false
  }
];

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
        ? `<span class="nb-badge mint">${q.outcome}</span>`
        : `<span class="nb-badge pink">${q.outcome}</span>`;
    } else if (q.hit) {
      outcomeBadge = `<strong style="color: #10b981;">${q.outcome}</strong>`;
    } else {
      outcomeBadge = `<strong style="color: #e11d48;">${q.outcome}</strong>`;
    }

    return `
      <tr>
        <td style="font-weight: 700;">"${q.query}"</td>
        <td><span class="nb-badge ${typeBadge}">${q.type}</span></td>
        <td><code>${q.file}</code></td>
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
    "[info] Loading configuration from config.yaml...",
    "[info] Qdrant connected at localhost:6334 (collection: code_chunks)",
    "[info] Ollama embedding query via nomic-embed-text (768 dimensions)... [34ms]",
    "[info] Executing hybrid retrieval: Dense (Cosine) + Sparse (BM25) with RRF (k=60)... [22ms]",
    "[info] Top 3 chunks retrieved. Synthesizing answer via qwen2.5-coder:7b (temp=0.0)... [62ms]",
    "",
    "┌────────────────────────────────── GROUNDED ANSWER ──────────────────────────────────┐",
    "│ ragcodepilot implements a two-tier chunking architecture:                           │",
    "│                                                                                     │",
    "│ 1. Go AST Chunker (internal/ingest/chunker_go.go) [1]: Uses go/parser to extract     │",
    "│    complete function and method declarations with their signature, receiver, and    │",
    "│    docstrings without slicing across arbitrary line boundaries.                      │",
    "│                                                                                     │",
    "│ 2. Sliding Window Chunker (internal/ingest/chunker.go) [2]: Fallback for non-Go     │",
    "│    files using a 40-line window with 5-line overlap and regex symbol detection.     │",
    "│                                                                                     │",
    "│ Sources:                                                                            │",
    "│   [1] internal/ingest/chunker_go.go (func chunkGoFile)                               │",
    "│   [2] internal/ingest/chunker.go (func ChunkFile)                                   │",
    "└─────────────────────────────────────────────────────────────────────────────────────┘",
    "",
    "Done. (terminal demo — not a live run)"
  ],

  "hybrid-search": [
    "$ go run ./cmd/ragcodepilot search --mode hybrid --limit 3 \"embedding interface\"",
    "[info] Mode: HYBRID (Dense + BM25 RRF, k=60)",
    "[info] Ollama nomic-embed-text (768d) query vector generated in 31ms",
    "",
    "RANK #1  [RRF: 0.0328]  internal/embedding/embedder.go",
    "  type Embedder interface {",
    "      Embed(ctx context.Context, texts []string) ([][]float32, error)",
    "      Dimension() int",
    "  }",
    "",
    "RANK #2  [RRF: 0.0315]  internal/embedding/ollama.go:45-78",
    "  func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {",
    "      // HTTP client calling /api/embeddings",
    "  }",
    "",
    "RANK #3  [RRF: 0.0298]  internal/embedding/validate.go:15-40",
    "  func ValidateDimensions(actual, expected int, coll string) error"
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
    "$ go run ./cmd/ragcodepilot eval --mode hybrid --output json",
    "Dataset:    docs/eval/golden.yaml",
    "Collection: code_chunks",
    "Embedder:   ollama/nomic-embed-text",
    "Queries:    39 (positive 35, negative 4, errors 0)",
    "",
    "Retrieval Metrics (positive queries):",
    "  hit@1:        0.857  (85.7%)",
    "  hit@3:        0.971  (97.1%)",
    "  hit@5:        0.971  (97.1%)",
    "  MRR@5:        0.910",
    "  recall@5:     0.821",
    "  recall@10:    0.903",
    "",
    "Negative pass rate: 0.50  (RRF ceiling 0.02; 2 of 4 dual-list)",
    "",
    "Latency Percentiles (ms):",
    "  Total p50/p95:   20 / 151 ms",
    "  Embed p50/p95:   16 / 29 ms",
    "  Qdrant p50/p95:  2 / 8 ms",
    "",
    "Canonical baseline: docs/eval/baseline_v8.json"
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
    btn.addEventListener('click', () => {
      tabBtns.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      renderTerminal(btn.getAttribute('data-cmd'));
    });
  });

  copyBtn.addEventListener('click', () => {
    const lines = terminalScripts[currentCmd];
    const cmdLine = lines[0].replace('$ ', '');
    navigator.clipboard.writeText(cmdLine).then(() => {
      const orig = copyBtn.textContent;
      copyBtn.textContent = 'Copied! ✓';
      setTimeout(() => copyBtn.textContent = orig, 1800);
    });
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
      const text = 'go run ./cmd/ragcodepilot search --answer "how does chunking work?"';
      navigator.clipboard.writeText(text).then(() => {
        const orig = copyBtn.innerHTML;
        copyBtn.innerHTML = '<span>Copied Command! ✓</span>';
        setTimeout(() => copyBtn.innerHTML = orig, 2000);
      });
    });
  }
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
