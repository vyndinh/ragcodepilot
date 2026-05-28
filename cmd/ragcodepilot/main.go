package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dinhvy/ragcodepilot/internal/answer"
	"github.com/dinhvy/ragcodepilot/internal/config"
	"github.com/dinhvy/ragcodepilot/internal/embedding"
	"github.com/dinhvy/ragcodepilot/internal/eval"
	"github.com/dinhvy/ragcodepilot/internal/ingest"
	"github.com/dinhvy/ragcodepilot/internal/qdrant"
	"github.com/dinhvy/ragcodepilot/internal/search"
)

var version = "dev"

const defaultConfigPath = "config.yaml"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "index":
		fs := flag.NewFlagSet("index", flag.ExitOnError)
		collection := fs.String("collection", "code_chunks", "Qdrant collection name")
		language := fs.String("language", "", "Comma-separated language filter (e.g., go,rust)")
		qdrantHost := fs.String("qdrant-host", "localhost", "Qdrant host")
		qdrantPort := fs.Int("qdrant-port", 6334, "Qdrant gRPC port")
		embedderType := fs.String("embedder", "ollama", "Embedder to use: ollama, fake")
		ollamaURL := fs.String("ollama-url", "http://localhost:11434", "Ollama server URL")
		ollamaModel := fs.String("ollama-model", "nomic-embed-text", "Ollama embedding model")
		_ = fs.Parse(os.Args[2:])

		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "error: repo-path is required")
			fs.Usage()
			os.Exit(1)
		}

		repoPath := fs.Arg(0)
		languages := parseLanguageFilter(*language)

		emb, err := resolveEmbedder(*embedderType, *ollamaURL, *ollamaModel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if err := runIndex(repoPath, *collection, languages, *qdrantHost, *qdrantPort, emb); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

	case "search":
		fs := flag.NewFlagSet("search", flag.ExitOnError)
		collection := fs.String("collection", "code_chunks", "Qdrant collection name")
		language := fs.String("language", "", "Comma-separated language filter (e.g., go,rust)")
		repo := fs.String("repo", "", "Comma-separated repo name filter (e.g., ragcodepilot,myproject)")
		limit := fs.Int("limit", 5, "Maximum number of results")
		mode := fs.String("mode", string(search.DefaultSearchMode), "Search mode: dense, sparse, hybrid")
		qdrantHost := fs.String("qdrant-host", "localhost", "Qdrant host")
		qdrantPort := fs.Int("qdrant-port", 6334, "Qdrant gRPC port")
		embedderType := fs.String("embedder", "ollama", "Embedder to use: ollama, fake")
		ollamaURL := fs.String("ollama-url", "http://localhost:11434", "Ollama server URL")
		ollamaModel := fs.String("ollama-model", "nomic-embed-text", "Ollama embedding model")
		answerMode := fs.Bool("answer", false, "Generate an answer from the retrieved chunks (RAG mode)")
		generatorType := fs.String("generator", "ollama", "Generator for --answer: ollama, fake")
		generativeModel := fs.String("ollama-generative-model", answer.DefaultGenerativeModel, "Ollama generative model for --answer")
		answerLimit := fs.Int("answer-limit", answer.DefaultAnswerLimit, "Number of top chunks fed to the generator for --answer")
		_ = fs.Parse(os.Args[2:])

		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "error: query is required")
			fs.Usage()
			os.Exit(1)
		}

		query := fs.Arg(0)
		languages := parseLanguageFilter(*language)
		repos := parseLanguageFilter(*repo) // same CSV parsing logic
		if *limit <= 0 {
			fmt.Fprintf(os.Stderr, "error: --limit must be > 0 (got %d)\n", *limit)
			os.Exit(1)
		}
		searchMode, err := search.ParseSearchMode(*mode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		emb, err := resolveEmbedder(*embedderType, *ollamaURL, *ollamaModel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		var gen answer.Generator
		if *answerMode {
			gen, err = resolveGenerator(*generatorType, *ollamaURL, *generativeModel)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		}

		if err := runSearch(query, *collection, languages, repos, searchMode, *limit, *answerLimit, *qdrantHost, *qdrantPort, emb, gen); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

	case "collections":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "error: subcommand required (list, delete)")
			printUsage()
			os.Exit(1)
		}
		switch os.Args[2] {
		case "list":
			fs := flag.NewFlagSet("collections list", flag.ExitOnError)
			qdrantHost := fs.String("qdrant-host", "localhost", "Qdrant host")
			qdrantPort := fs.Int("qdrant-port", 6334, "Qdrant gRPC port")
			_ = fs.Parse(os.Args[3:])

			if err := runCollectionsList(*qdrantHost, *qdrantPort); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}

		case "delete":
			fs := flag.NewFlagSet("collections delete", flag.ExitOnError)
			qdrantHost := fs.String("qdrant-host", "localhost", "Qdrant host")
			qdrantPort := fs.Int("qdrant-port", 6334, "Qdrant gRPC port")
			_ = fs.Parse(os.Args[3:])

			if fs.NArg() < 1 {
				fmt.Fprintln(os.Stderr, "error: collection name is required")
				fs.Usage()
				os.Exit(1)
			}

			if err := runCollectionsDelete(fs.Arg(0), *qdrantHost, *qdrantPort); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}

		default:
			fmt.Fprintf(os.Stderr, "unknown collections subcommand: %s\n", os.Args[2])
			os.Exit(1)
		}

	case "eval":
		fs := flag.NewFlagSet("eval", flag.ExitOnError)
		dataset := fs.String("dataset", "docs/eval/golden.yaml", "Path to the golden YAML dataset")
		collection := fs.String("collection", "code_chunks", "Qdrant collection name")
		output := fs.String("output", "human", "Output format: human, json")
		limit := fs.Int("limit", eval.DefaultLimit, "Per-query result limit (must be >= 10 for recall@10)")
		typeFilter := fs.String("type", "", "Filter queries by type (navigation, concept, behavior, negative)")
		subtypeFilter := fs.String("subtype", "", "Filter queries by subtype (e.g. structural under navigation)")
		mode := fs.String("mode", string(search.DefaultSearchMode), "Search mode: dense, sparse, hybrid")
		qdrantHost := fs.String("qdrant-host", "localhost", "Qdrant host")
		qdrantPort := fs.Int("qdrant-port", 6334, "Qdrant gRPC port")
		embedderType := fs.String("embedder", "ollama", "Embedder to use: ollama, fake")
		ollamaURL := fs.String("ollama-url", "http://localhost:11434", "Ollama server URL")
		ollamaModel := fs.String("ollama-model", "nomic-embed-text", "Ollama embedding model")
		answerMode := fs.Bool("answer", false, "Also generate answers and score reference-free answer metrics")
		generatorType := fs.String("generator", "ollama", "Generator for --answer: ollama, fake")
		generativeModel := fs.String("ollama-generative-model", answer.DefaultGenerativeModel, "Ollama generative model for --answer")
		answerLimit := fs.Int("answer-limit", answer.DefaultAnswerLimit, "Number of top chunks fed to the generator for --answer (retrieval metrics still use --limit)")
		_ = fs.Parse(os.Args[2:])

		if *limit <= 0 {
			fmt.Fprintf(os.Stderr, "error: --limit must be > 0 (got %d)\n", *limit)
			os.Exit(1)
		}

		searchMode, err := search.ParseSearchMode(*mode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		emb, err := resolveEmbedder(*embedderType, *ollamaURL, *ollamaModel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		var gen answer.Generator
		genName := ""
		if *answerMode {
			gen, err = resolveGenerator(*generatorType, *ollamaURL, *generativeModel)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			genName = generatorName(*generatorType, *generativeModel)
		}

		if err := runEval(*dataset, *collection, *output, *limit, *answerLimit, *typeFilter, *subtypeFilter, searchMode, *qdrantHost, *qdrantPort, *embedderType, *ollamaModel, emb, gen, genName); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

	case "version":
		fmt.Printf("ragcodepilot %s\n", version)

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`ragcodepilot - semantic code search powered by vector database

Usage:
  ragcodepilot index <repo-path> [flags]       Index a code repository
  ragcodepilot search <query> [flags]          Search indexed code
  ragcodepilot eval [flags]                    Run retrieval evaluation against a golden dataset
  ragcodepilot collections list [flags]        List all collections
  ragcodepilot collections delete <name> [flags]  Delete a collection
  ragcodepilot version                         Print version

Index flags:
  -collection string     Qdrant collection name (default "code_chunks")
  -language string       Comma-separated language filter (e.g., go,rust)
  -embedder string       Embedder to use: ollama, fake (default "ollama")
  -ollama-url string     Ollama server URL (default "http://localhost:11434")
  -ollama-model string   Ollama embedding model (default "nomic-embed-text")
  -qdrant-host string    Qdrant host (default "localhost")
  -qdrant-port int       Qdrant gRPC port (default 6334)

Search flags:
  -collection string     Qdrant collection name (default "code_chunks")
  -language string       Comma-separated language filter (e.g., go,rust)
  -repo string           Comma-separated repo name filter (e.g., ragcodepilot)
  -limit int             Maximum number of results (default 5)
  -mode string           Search mode: dense, sparse, hybrid (default "hybrid")
  -embedder string       Embedder to use: ollama, fake (default "ollama")
  -ollama-url string     Ollama server URL (default "http://localhost:11434")
  -ollama-model string   Ollama embedding model (default "nomic-embed-text")
  -answer                Generate an answer from the retrieved chunks (RAG mode)
  -generator string      Generator for --answer: ollama, fake (default "ollama")
  -ollama-generative-model string  Ollama generative model for --answer (default "qwen2.5-coder:7b")
  -answer-limit int      Number of top chunks fed to the generator for --answer (default 5)
  -qdrant-host string    Qdrant host (default "localhost")
  -qdrant-port int       Qdrant gRPC port (default 6334)

Eval flags:
  -dataset string        Path to the golden YAML dataset (default "docs/eval/golden.yaml")
  -collection string     Qdrant collection name (default "code_chunks")
  -output string         Output format: human, json (default "human")
  -limit int             Per-query result limit (default 10)
  -type string           Filter queries by type (navigation, concept, behavior, negative)
  -subtype string        Filter queries by subtype (e.g. structural)
  -mode string           Search mode: dense, sparse, hybrid (default "hybrid")
  -embedder string       Embedder to use: ollama, fake (default "ollama")
  -ollama-url string     Ollama server URL (default "http://localhost:11434")
  -ollama-model string   Ollama embedding model (default "nomic-embed-text")
  -answer                Also generate answers and score reference-free answer metrics
  -generator string      Generator for --answer: ollama, fake (default "ollama")
  -ollama-generative-model string  Ollama generative model for --answer (default "qwen2.5-coder:7b")
  -answer-limit int      Top chunks fed to the generator for --answer; retrieval metrics still use --limit (default 5)
  -qdrant-host string    Qdrant host (default "localhost")
  -qdrant-port int       Qdrant gRPC port (default 6334)`)
}

func parseLanguageFilter(lang string) []string {
	if lang == "" {
		return nil
	}
	parts := strings.Split(lang, ",")
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func resolveEmbedder(embedderType, ollamaURL, ollamaModel string) (embedding.Embedder, error) {
	switch embedderType {
	case "ollama":
		fmt.Fprintf(os.Stderr, "Using Ollama embedder (model: %s, url: %s)\n", ollamaModel, ollamaURL)
		return embedding.NewOllamaEmbedder(ollamaURL, ollamaModel), nil
	case "fake":
		fmt.Fprintln(os.Stderr, "Using fake embedder (random vectors — search results will not be meaningful)")
		return embedding.NewFakeEmbedder(384), nil
	default:
		return nil, fmt.Errorf("unknown embedder %q (supported: ollama, fake)", embedderType)
	}
}

// resolveGenerator builds the answer Generator selected by --generator.
// NOTE: v1 moves this into internal/answer/ so the eventual REPL can share it
// (see docs/plan/phase5_v0_answer_mode.md). v0 keeps it in cmd/ for simplicity.
func resolveGenerator(generatorType, ollamaURL, generativeModel string) (answer.Generator, error) {
	switch generatorType {
	case "ollama":
		fmt.Fprintf(os.Stderr, "Using Ollama generator (model: %s, url: %s)\n", generativeModel, ollamaURL)
		return answer.NewOllamaGenerator(ollamaURL, generativeModel), nil
	case "fake":
		fmt.Fprintln(os.Stderr, "Using fake generator (canned response — for testing only)")
		return answer.NewFakeGenerator(""), nil
	default:
		return nil, fmt.Errorf("unknown generator %q (supported: ollama, fake)", generatorType)
	}
}

// generatorName returns a descriptive label for the report (e.g. "ollama/qwen2.5-coder:7b").
func generatorName(generatorType, generativeModel string) string {
	if generatorType == "ollama" {
		return fmt.Sprintf("ollama/%s", generativeModel)
	}
	return generatorType
}

func runIndex(repoPath, collection string, languages []string, qdrantHost string, qdrantPort int, embedder embedding.Embedder) error {
	ctx := context.Background()
	cfg, err := resolveIndexConfig()
	if err != nil {
		return err
	}

	client, err := qdrant.NewClient(qdrantHost, qdrantPort)
	if err != nil {
		return fmt.Errorf("connecting to qdrant: %w", err)
	}
	defer func() { _ = client.Close() }()

	pipeline := ingest.NewPipeline(cfg, embedder, client, collection, ingest.WithLanguages(languages))

	if len(languages) > 0 {
		fmt.Printf("Filtering to languages: %s\n", strings.Join(languages, ", "))
	}

	return pipeline.Run(ctx, repoPath)
}

func resolveIndexConfig() (*config.Config, error) {
	if _, err := os.Stat(defaultConfigPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config.Default(), nil
		}
		return nil, fmt.Errorf("checking %s: %w", defaultConfigPath, err)
	}

	cfg, err := config.Load(defaultConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", defaultConfigPath, err)
	}
	return cfg, nil
}

// runSearch runs hybrid retrieval. When gen is non-nil (--answer), it additionally
// synthesizes an answer from the top answerLimit retrieved chunks and prints it
// above the sources it used. When gen is nil, the output is byte-identical to the
// retrieval-only path.
func runSearch(query, collection string, languages, repos []string, mode search.SearchMode, limit, answerLimit int, qdrantHost string, qdrantPort int, embedder embedding.Embedder, gen answer.Generator) error {
	ctx := context.Background()

	client, err := qdrant.NewClient(qdrantHost, qdrantPort)
	if err != nil {
		return fmt.Errorf("connecting to qdrant: %w", err)
	}
	defer func() { _ = client.Close() }()

	searcher := search.NewSearcher(client, embedder)
	results, err := searcher.Search(ctx, collection, query, mode, uint64(limit), languages, repos)
	if err != nil {
		return err
	}

	if gen == nil {
		fmt.Print(search.FormatResults(results))
		return nil
	}

	// Pre-load the model (if the generator supports it) so the cold-start cost
	// is paid here, not bundled into the timed Generate call below.
	if w, ok := gen.(answer.Warmer); ok {
		fmt.Fprintln(os.Stderr, "Warming up generative model (first call may take a while)...")
		if err := w.Warmup(ctx); err != nil {
			return fmt.Errorf("warming up generator: %w", err)
		}
	}

	// Feed only the top answerLimit chunks to the generator; print exactly those
	// as Sources so the answer's [N] citations line up with what's shown.
	answerResults := results
	if answerLimit > 0 && answerLimit < len(answerResults) {
		answerResults = answerResults[:answerLimit]
	}

	prompt := answer.Prompt{Question: query, Chunks: answer.ContextsFromResults(answerResults)}
	text, err := gen.Generate(ctx, prompt)
	if err != nil {
		return fmt.Errorf("generating answer: %w", err)
	}

	fmt.Printf("Answer: %s\n\nSources:\n%s", text, search.FormatResults(answerResults))
	return nil
}

func runCollectionsList(qdrantHost string, qdrantPort int) error {
	ctx := context.Background()

	client, err := qdrant.NewClient(qdrantHost, qdrantPort)
	if err != nil {
		return fmt.Errorf("connecting to qdrant: %w", err)
	}
	defer func() { _ = client.Close() }()

	collections, err := client.ListCollections(ctx)
	if err != nil {
		return err
	}
	if len(collections) == 0 {
		fmt.Println("No collections found.")
		return nil
	}
	for _, c := range collections {
		fmt.Println(c)
	}
	return nil
}

func runEval(datasetPath, collection, output string, limit, answerLimit int, typeFilter, subtypeFilter string, mode search.SearchMode, qdrantHost string, qdrantPort int, embedderType, model string, embedder embedding.Embedder, gen answer.Generator, genName string) error {
	ctx := context.Background()
	if limit < eval.DefaultLimit {
		return fmt.Errorf("invalid --limit %d: must be >= %d for recall@10", limit, eval.DefaultLimit)
	}

	ds, err := eval.LoadDataset(datasetPath)
	if err != nil {
		return err
	}

	if typeFilter != "" || subtypeFilter != "" {
		filtered := make([]eval.Query, 0, len(ds.Queries))
		for _, q := range ds.Queries {
			if typeFilter != "" && string(q.Type) != typeFilter {
				continue
			}
			if subtypeFilter != "" && q.Subtype != subtypeFilter {
				continue
			}
			filtered = append(filtered, q)
		}
		if len(filtered) == 0 {
			return fmt.Errorf("no queries match type=%q subtype=%q", typeFilter, subtypeFilter)
		}
		ds.Queries = filtered
	}

	client, err := qdrant.NewClient(qdrantHost, qdrantPort)
	if err != nil {
		return fmt.Errorf("connecting to qdrant: %w", err)
	}
	defer func() { _ = client.Close() }()

	embedderName := embedderType
	if embedderType == "ollama" {
		embedderName = fmt.Sprintf("ollama/%s", model)
	}

	if gen != nil {
		if w, ok := gen.(answer.Warmer); ok {
			fmt.Fprintln(os.Stderr, "Warming up generative model (first call may take a while)...")
			if err := w.Warmup(ctx); err != nil {
				return fmt.Errorf("warming up generator: %w", err)
			}
		}
	}

	searcher := search.NewSearcher(client, embedder)
	runner := &eval.Runner{
		Searcher:      searcher,
		Collection:    collection,
		Limit:         limit,
		EmbedderName:  embedderName,
		Mode:          mode,
		Generator:     gen,
		GeneratorName: genName,
		AnswerLimit:   answerLimit,
	}

	report, err := runner.Run(ctx, datasetPath, ds)
	if err != nil {
		return err
	}

	switch output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return fmt.Errorf("encoding report: %w", err)
		}
	case "human", "":
		fmt.Print(eval.FormatHuman(report))
	default:
		return fmt.Errorf("unknown output format %q (supported: human, json)", output)
	}

	if report.Aggregate.Errors > 0 {
		return fmt.Errorf("%d queries failed (see report)", report.Aggregate.Errors)
	}
	return nil
}

func runCollectionsDelete(name, qdrantHost string, qdrantPort int) error {
	ctx := context.Background()

	client, err := qdrant.NewClient(qdrantHost, qdrantPort)
	if err != nil {
		return fmt.Errorf("connecting to qdrant: %w", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.DeleteCollection(ctx, name); err != nil {
		return err
	}
	fmt.Printf("Deleted collection %q\n", name)
	return nil
}
