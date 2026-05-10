package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dinhvy/ragcodepilot/internal/config"
	"github.com/dinhvy/ragcodepilot/internal/embedding"
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
		qdrantHost := fs.String("qdrant-host", "localhost", "Qdrant host")
		qdrantPort := fs.Int("qdrant-port", 6334, "Qdrant gRPC port")
		embedderType := fs.String("embedder", "ollama", "Embedder to use: ollama, fake")
		ollamaURL := fs.String("ollama-url", "http://localhost:11434", "Ollama server URL")
		ollamaModel := fs.String("ollama-model", "nomic-embed-text", "Ollama embedding model")
		_ = fs.Parse(os.Args[2:])

		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "error: query is required")
			fs.Usage()
			os.Exit(1)
		}

		query := fs.Arg(0)
		languages := parseLanguageFilter(*language)
		repos := parseLanguageFilter(*repo) // same CSV parsing logic

		emb, err := resolveEmbedder(*embedderType, *ollamaURL, *ollamaModel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if err := runSearch(query, *collection, languages, repos, *limit, *qdrantHost, *qdrantPort, emb); err != nil {
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
			fs.Parse(os.Args[3:])

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
  -embedder string       Embedder to use: ollama, fake (default "ollama")
  -ollama-url string     Ollama server URL (default "http://localhost:11434")
  -ollama-model string   Ollama embedding model (default "nomic-embed-text")
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
		fmt.Printf("Using Ollama embedder (model: %s, url: %s)\n", ollamaModel, ollamaURL)
		return embedding.NewOllamaEmbedder(ollamaURL, ollamaModel), nil
	case "fake":
		fmt.Println("Using fake embedder (random vectors — search results will not be meaningful)")
		return embedding.NewFakeEmbedder(384), nil
	default:
		return nil, fmt.Errorf("unknown embedder %q (supported: ollama, fake)", embedderType)
	}
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

func runSearch(query, collection string, languages, repos []string, limit int, qdrantHost string, qdrantPort int, embedder embedding.Embedder) error {
	ctx := context.Background()

	client, err := qdrant.NewClient(qdrantHost, qdrantPort)
	if err != nil {
		return fmt.Errorf("connecting to qdrant: %w", err)
	}
	defer func() { _ = client.Close() }()

	searcher := search.NewSearcher(client, embedder)
	results, err := searcher.Search(ctx, collection, query, uint64(limit), languages, repos)
	if err != nil {
		return err
	}

	fmt.Print(search.FormatResults(results))
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

func runCollectionsDelete(name, qdrantHost string, qdrantPort int) error {
	ctx := context.Background()

	client, err := qdrant.NewClient(qdrantHost, qdrantPort)
	if err != nil {
		return fmt.Errorf("connecting to qdrant: %w", err)
	}
	defer client.Close()

	if err := client.DeleteCollection(ctx, name); err != nil {
		return err
	}
	fmt.Printf("Deleted collection %q\n", name)
	return nil
}
