// Package qdrant wraps the Qdrant Go SDK and provides domain-specific methods
// for the ragcodepilot application.
package qdrant

import (
	"context"
	"fmt"

	pb "github.com/qdrant/go-client/qdrant"

	"github.com/dinhvy/ragcodepilot/internal/embedding"
	"github.com/dinhvy/ragcodepilot/internal/model"
)

type sdkClient interface {
	Close() error
	CollectionExists(ctx context.Context, collectionName string) (bool, error)
	GetCollectionInfo(ctx context.Context, collectionName string) (*pb.CollectionInfo, error)
	CreateCollection(ctx context.Context, request *pb.CreateCollection) error
	CreateFieldIndex(ctx context.Context, request *pb.CreateFieldIndexCollection) (*pb.UpdateResult, error)
	Upsert(ctx context.Context, request *pb.UpsertPoints) (*pb.UpdateResult, error)
	Query(ctx context.Context, request *pb.QueryPoints) ([]*pb.ScoredPoint, error)
	ScrollAndOffset(ctx context.Context, request *pb.ScrollPoints) ([]*pb.RetrievedPoint, *pb.PointId, error)
	Delete(ctx context.Context, request *pb.DeletePoints) (*pb.UpdateResult, error)
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

	// Create the collection with named vectors.
	err = c.conn.CreateCollection(ctx, &pb.CreateCollection{
		CollectionName: name,
		VectorsConfig: pb.NewVectorsConfigMap(map[string]*pb.VectorParams{
			"dense": {
				Size:     vectorSize,
				Distance: pb.Distance_Cosine,
			},
		}),
		SparseVectorsConfig: pb.NewSparseVectorsConfig(map[string]*pb.SparseVectorParams{
			"sparse": {},
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

// EnsurePayloadIndexes creates keyword indexes on commonly filtered payload
// fields if the collection exists. No-op if the collection does not exist.
// Call this before any filtered scroll or delete to guarantee efficient queries
// on collections created before payload indexes were introduced.
func (c *Client) EnsurePayloadIndexes(ctx context.Context, collection string) error {
	exists, err := c.conn.CollectionExists(ctx, collection)
	if err != nil {
		return fmt.Errorf("checking collection %s: %w", collection, err)
	}
	if !exists {
		return nil
	}
	return c.ensurePayloadIndexes(ctx, collection)
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
//
// Supports both named-vector (Phase 2+) and unnamed-vector (legacy) schemas.
// If the collection uses the legacy unnamed schema, it returns an error
// advising the user to delete and re-index.
func (c *Client) validateCollectionDimension(ctx context.Context, name string, expectedSize uint64) error {
	info, err := c.conn.GetCollectionInfo(ctx, name)
	if err != nil {
		return fmt.Errorf("getting collection info for %s: %w", name, err)
	}

	vectorsConfig := info.GetConfig().GetParams().GetVectorsConfig()

	// Named-vector schema: look up the "dense" key in the params map.
	if paramsMap := vectorsConfig.GetParamsMap(); paramsMap != nil {
		denseParams, ok := paramsMap.GetMap()["dense"]
		if !ok {
			return fmt.Errorf(
				"collection %q has named vectors but no \"dense\" vector; "+
					"delete the collection and re-index: ragcodepilot collections delete %s",
				name, name,
			)
		}
		actualSize := denseParams.GetSize()
		if actualSize != expectedSize {
			return fmt.Errorf(
				"collection %q uses %d-dimensional vectors, but the current embedder produces %d-dimensional vectors; "+
					"delete the collection and re-index, or use a different collection name",
				name, actualSize, expectedSize,
			)
		}
		return nil
	}

	// Legacy unnamed-vector schema: advise re-indexing.
	if params := vectorsConfig.GetParams(); params != nil {
		return fmt.Errorf(
			"collection %q uses the legacy unnamed-vector schema; "+
				"delete and re-index:\n  ragcodepilot collections delete %s\n  ragcodepilot index <repo-path>",
			name, name,
		)
	}

	return fmt.Errorf("collection %q has no vector configuration", name)
}

// ValidateCollectionVectorSize checks that the collection's vector dimension
// matches the given size. Returns a clear error on mismatch.
func (c *Client) ValidateCollectionVectorSize(ctx context.Context, name string, vectorSize uint64) error {
	return c.validateCollectionDimension(ctx, name, vectorSize)
}

// Upsert inserts or updates points in the collection.
// sparseVectors is optional; when nil, sparse vectors are omitted from points.
func (c *Client) Upsert(ctx context.Context, collection string, chunks []model.CodeChunk, vectors [][]float32, sparseVectors []embedding.SparseVector) error {
	if len(chunks) != len(vectors) {
		return fmt.Errorf("chunks and vectors length mismatch: %d vs %d", len(chunks), len(vectors))
	}
	if sparseVectors != nil && len(chunks) != len(sparseVectors) {
		return fmt.Errorf("chunks and sparse vectors length mismatch: %d vs %d", len(chunks), len(sparseVectors))
	}

	points := make([]*pb.PointStruct, len(chunks))
	for i, chunk := range chunks {
		vectorsMap := map[string]*pb.Vector{
			"dense": pb.NewVectorDense(vectors[i]),
		}
		if sparseVectors != nil {
			if len(sparseVectors[i].Indices) != len(sparseVectors[i].Values) {
				return fmt.Errorf("sparse vector %d indices/values length mismatch: %d vs %d", i, len(sparseVectors[i].Indices), len(sparseVectors[i].Values))
			}
			vectorsMap["sparse"] = pb.NewVectorSparse(sparseVectors[i].Indices, sparseVectors[i].Values)
		}

		points[i] = &pb.PointStruct{
			Id:      pb.NewID(chunk.ID),
			Vectors: pb.NewVectorsMap(vectorsMap),
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
				"file_hash":  chunk.FileHash,
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
			Wait:           pb.PtrOf(true),
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
		Query:          pb.NewQueryDense(queryVector),
		Using:          pb.PtrOf("dense"),
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
		FileHash:  getStringValue(payload, "file_hash"),
	}
}

// ScrollFileHashes retrieves all unique {file_path: file_hash} pairs for a given
// repo from the collection. Used for change detection during re-indexing.
//
// When languages is non-empty, only points matching those languages are returned.
// This prevents language-scoped re-indexing (e.g. --language go) from seeing
// points for other languages, which would otherwise be classified as stale and
// deleted.
//
// Paginates through all points to handle repos with more than one page of chunks.
func (c *Client) ScrollFileHashes(ctx context.Context, collection, repo string, languages []string) (map[string]string, error) {
	exists, err := c.conn.CollectionExists(ctx, collection)
	if err != nil {
		return nil, fmt.Errorf("checking collection %s: %w", collection, err)
	}
	if !exists {
		return make(map[string]string), nil
	}

	// Build scroll filter: repo is always required, language is optional.
	mustConditions := []*pb.Condition{pb.NewMatch("repo", repo)}
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

	const pageSize = 1000
	result := make(map[string]string)
	var offset *pb.PointId

	for {
		points, nextOffset, err := c.conn.ScrollAndOffset(ctx, &pb.ScrollPoints{
			CollectionName: collection,
			Filter:         &pb.Filter{Must: mustConditions},
			WithPayload:    pb.NewWithPayloadInclude("file_path", "file_hash"),
			Limit:          pb.PtrOf(uint32(pageSize)),
			Offset:         offset,
		})
		if err != nil {
			return nil, fmt.Errorf("scrolling file hashes for repo %s: %w", repo, err)
		}

		for _, point := range points {
			filePath := getStringValue(point.Payload, "file_path")
			fileHash := getStringValue(point.Payload, "file_hash")
			if filePath != "" {
				result[filePath] = fileHash
			}
		}

		// Qdrant returns nil next_page_offset when there are no more pages.
		if nextOffset == nil {
			break
		}
		offset = nextOffset
	}

	return result, nil
}

// DeleteStaleChunksByFilePath deletes points for a single file whose stored
// file_hash does not match currentHash. Used after upserting re-indexed chunks
// to remove only the orphaned old-hash chunks, leaving the freshly indexed
// new-hash chunks untouched.
func (c *Client) DeleteStaleChunksByFilePath(ctx context.Context, collection, repo, filePath, currentHash string) error {
	_, err := c.conn.Delete(ctx, &pb.DeletePoints{
		CollectionName: collection,
		Wait:           pb.PtrOf(true),
		Points: &pb.PointsSelector{
			PointsSelectorOneOf: &pb.PointsSelector_Filter{
				Filter: &pb.Filter{
					Must: []*pb.Condition{
						pb.NewMatch("repo", repo),
						pb.NewMatch("file_path", filePath),
					},
					MustNot: []*pb.Condition{
						pb.NewMatch("file_hash", currentHash),
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("deleting stale chunks for %s in repo %s: %w", filePath, repo, err)
	}
	return nil
}

// DeleteByFilePaths deletes all points in the collection that match the given
// repo and any of the specified file paths.
func (c *Client) DeleteByFilePaths(ctx context.Context, collection, repo string, filePaths []string) error {
	if len(filePaths) == 0 {
		return nil
	}

	fileConditions := make([]*pb.Condition, len(filePaths))
	for i, fp := range filePaths {
		fileConditions[i] = pb.NewMatch("file_path", fp)
	}

	_, err := c.conn.Delete(ctx, &pb.DeletePoints{
		CollectionName: collection,
		Wait:           pb.PtrOf(true),
		Points: &pb.PointsSelector{
			PointsSelectorOneOf: &pb.PointsSelector_Filter{
				Filter: &pb.Filter{
					Must: []*pb.Condition{
						pb.NewMatch("repo", repo),
						{
							ConditionOneOf: &pb.Condition_Filter{
								Filter: &pb.Filter{Should: fileConditions},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("deleting points for %d files in repo %s: %w", len(filePaths), repo, err)
	}

	return nil
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
