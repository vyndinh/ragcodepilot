// Package qdrant wraps the Qdrant Go SDK and provides domain-specific methods
// for the ragsearch application.
package qdrant

import (
	"context"
	"fmt"

	pb "github.com/qdrant/go-client/qdrant"

	"github.com/dinhvy/ragsearch/internal/model"
)

type sdkClient interface {
	Close() error
	CollectionExists(ctx context.Context, collectionName string) (bool, error)
	GetCollectionInfo(ctx context.Context, collectionName string) (*pb.CollectionInfo, error)
	CreateCollection(ctx context.Context, request *pb.CreateCollection) error
	CreateFieldIndex(ctx context.Context, request *pb.CreateFieldIndexCollection) (*pb.UpdateResult, error)
	Upsert(ctx context.Context, request *pb.UpsertPoints) (*pb.UpdateResult, error)
	Query(ctx context.Context, request *pb.QueryPoints) ([]*pb.ScoredPoint, error)
	ListCollections(ctx context.Context) ([]string, error)
	DeleteCollection(ctx context.Context, collectionName string) error
}

// Client wraps the Qdrant Go SDK client.
type Client struct {
	conn sdkClient
}

// NewClient creates a new Qdrant client connected to the given host and port.
func NewClient(host string, port int) (*Client, error) {
	conn, err := pb.NewClient(&pb.Config{
		Host: host,
		Port: port,
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to qdrant at %s:%d: %w", host, port, err)
	}
	return &Client{conn: conn}, nil
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// EnsureCollection creates a collection if it doesn't exist.
// If the collection already exists, it validates that the vector dimension matches.
func (c *Client) EnsureCollection(ctx context.Context, name string, vectorSize uint64) error {
	exists, err := c.conn.CollectionExists(ctx, name)
	if err != nil {
		return fmt.Errorf("checking collection %s: %w", name, err)
	}

	if exists {
		return c.validateCollectionDimension(ctx, name, vectorSize)
	}

	// Create the collection.
	err = c.conn.CreateCollection(ctx, &pb.CreateCollection{
		CollectionName: name,
		VectorsConfig: pb.NewVectorsConfig(&pb.VectorParams{
			Size:     vectorSize,
			Distance: pb.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("creating collection %s: %w", name, err)
	}

	// Create payload indexes for commonly filtered fields.
	if err := c.ensurePayloadIndexes(ctx, name); err != nil {
		return fmt.Errorf("creating payload indexes for %s: %w", name, err)
	}

	return nil
}

// payloadIndexFields lists the payload fields that should be indexed for
// efficient filtered search. All are keyword type (exact match).
var payloadIndexFields = []string{"repo", "language", "file_path"}

// ensurePayloadIndexes creates keyword indexes on frequently filtered payload
// fields. Qdrant is idempotent — calling this on an already-indexed field is a
// no-op.
func (c *Client) ensurePayloadIndexes(ctx context.Context, collection string) error {
	fieldType := pb.FieldType_FieldTypeKeyword
	for _, field := range payloadIndexFields {
		_, err := c.conn.CreateFieldIndex(ctx, &pb.CreateFieldIndexCollection{
			CollectionName: collection,
			FieldName:      field,
			FieldType:      &fieldType,
			Wait:           pb.PtrOf(true),
		})
		if err != nil {
			return fmt.Errorf("indexing field %s: %w", field, err)
		}
	}
	return nil
}

// validateCollectionDimension checks that an existing collection's vector
// dimension matches the expected size. Returns a clear error on mismatch.
func (c *Client) validateCollectionDimension(ctx context.Context, name string, expectedSize uint64) error {
	info, err := c.conn.GetCollectionInfo(ctx, name)
	if err != nil {
		return fmt.Errorf("getting collection info for %s: %w", name, err)
	}

	actualSize := info.GetConfig().GetParams().GetVectorsConfig().GetParams().GetSize()
	if actualSize != expectedSize {
		return fmt.Errorf(
			"collection %q uses %d-dimensional vectors, but the current embedder produces %d-dimensional vectors; "+
				"delete the collection and re-index, or use a different collection name",
			name, actualSize, expectedSize,
		)
	}

	return nil
}

// ValidateCollectionVectorSize checks that the collection's vector dimension
// matches the given size. Returns a clear error on mismatch.
func (c *Client) ValidateCollectionVectorSize(ctx context.Context, name string, vectorSize uint64) error {
	return c.validateCollectionDimension(ctx, name, vectorSize)
}

// Upsert inserts or updates points in the collection.
func (c *Client) Upsert(ctx context.Context, collection string, chunks []model.CodeChunk, vectors [][]float32) error {
	if len(chunks) != len(vectors) {
		return fmt.Errorf("chunks and vectors length mismatch: %d vs %d", len(chunks), len(vectors))
	}

	points := make([]*pb.PointStruct, len(chunks))
	for i, chunk := range chunks {
		points[i] = &pb.PointStruct{
			Id:      pb.NewID(chunk.ID),
			Vectors: pb.NewVectors(vectors[i]...),
			Payload: pb.NewValueMap(map[string]any{
				"repo":       chunk.Repo,
				"file_path":  chunk.FilePath,
				"language":   chunk.Language,
				"chunk_type": chunk.ChunkType,
				"name":       chunk.Name,
				"content":    chunk.Content,
				"start_line": chunk.StartLine,
				"end_line":   chunk.EndLine,
				"indexed_at": chunk.IndexedAt,
			}),
		}
	}

	// Upsert in batches of 64 to avoid large gRPC messages.
	const batchSize = 64
	for start := 0; start < len(points); start += batchSize {
		end := start + batchSize
		if end > len(points) {
			end = len(points)
		}

		_, err := c.conn.Upsert(ctx, &pb.UpsertPoints{
			CollectionName: collection,
			Points:         points[start:end],
		})
		if err != nil {
			return fmt.Errorf("upserting batch %d-%d: %w", start, end, err)
		}
	}

	return nil
}

// Search performs a vector similarity search and returns the top results.
func (c *Client) Search(ctx context.Context, collection string, queryVector []float32, limit uint64, languages, repos []string) ([]model.SearchResult, error) {
	queryPoints := &pb.QueryPoints{
		CollectionName: collection,
		Query:          pb.NewQuery(queryVector...),
		Limit:          pb.PtrOf(limit),
		WithPayload:    pb.NewWithPayload(true),
	}

	// Build filter: language and repo are AND-ed (Must).
	// Within each, multiple values are OR-ed (Should).
	var mustConditions []*pb.Condition

	if len(languages) > 0 {
		langConditions := make([]*pb.Condition, len(languages))
		for i, lang := range languages {
			langConditions[i] = pb.NewMatch("language", lang)
		}
		mustConditions = append(mustConditions, &pb.Condition{
			ConditionOneOf: &pb.Condition_Filter{
				Filter: &pb.Filter{Should: langConditions},
			},
		})
	}

	if len(repos) > 0 {
		repoConditions := make([]*pb.Condition, len(repos))
		for i, repo := range repos {
			repoConditions[i] = pb.NewMatch("repo", repo)
		}
		mustConditions = append(mustConditions, &pb.Condition{
			ConditionOneOf: &pb.Condition_Filter{
				Filter: &pb.Filter{Should: repoConditions},
			},
		})
	}

	if len(mustConditions) > 0 {
		queryPoints.Filter = &pb.Filter{Must: mustConditions}
	}

	response, err := c.conn.Query(ctx, queryPoints)
	if err != nil {
		return nil, fmt.Errorf("searching collection %s: %w", collection, err)
	}

	results := make([]model.SearchResult, 0, len(response))
	for _, point := range response {
		chunk := payloadToChunk(point.Payload)
		results = append(results, model.SearchResult{
			Chunk: chunk,
			Score: point.Score,
		})
	}

	return results, nil
}

// ListCollections returns the names of all collections.
func (c *Client) ListCollections(ctx context.Context) ([]string, error) {
	collections, err := c.conn.ListCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing collections: %w", err)
	}
	return collections, nil
}

// DeleteCollection deletes a collection by name.
func (c *Client) DeleteCollection(ctx context.Context, name string) error {
	err := c.conn.DeleteCollection(ctx, name)
	if err != nil {
		return fmt.Errorf("deleting collection %s: %w", name, err)
	}
	return nil
}

// payloadToChunk extracts a CodeChunk from a Qdrant point payload.
func payloadToChunk(payload map[string]*pb.Value) model.CodeChunk {
	return model.CodeChunk{
		Repo:      getStringValue(payload, "repo"),
		FilePath:  getStringValue(payload, "file_path"),
		Language:  getStringValue(payload, "language"),
		ChunkType: getStringValue(payload, "chunk_type"),
		Name:      getStringValue(payload, "name"),
		Content:   getStringValue(payload, "content"),
		StartLine: getIntValue(payload, "start_line"),
		EndLine:   getIntValue(payload, "end_line"),
		IndexedAt: getStringValue(payload, "indexed_at"),
	}
}

func getStringValue(payload map[string]*pb.Value, key string) string {
	if v, ok := payload[key]; ok {
		if sv := v.GetStringValue(); sv != "" {
			return sv
		}
	}
	return ""
}

func getIntValue(payload map[string]*pb.Value, key string) int {
	if v, ok := payload[key]; ok {
		return int(v.GetIntegerValue())
	}
	return 0
}
