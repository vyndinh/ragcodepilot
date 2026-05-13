package qdrant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dinhvy/ragcodepilot/internal/model"
	pb "github.com/qdrant/go-client/qdrant"
)

func TestClient_EnsureCollectionCreatesMissingCollection(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{}
	client := &Client{conn: sdk}

	if err := client.EnsureCollection(context.Background(), "code_chunks", 384); err != nil {
		t.Fatalf("EnsureCollection() unexpected error: %v", err)
	}
	if sdk.existsCalls != 1 {
		t.Fatalf("CollectionExists calls = %d, want 1", sdk.existsCalls)
	}
	if sdk.infoCalls != 0 {
		t.Fatalf("GetCollectionInfo calls = %d, want 0", sdk.infoCalls)
	}
	if sdk.createCalls != 1 {
		t.Fatalf("CreateCollection calls = %d, want 1", sdk.createCalls)
	}
	if got := sdk.created.GetCollectionName(); got != "code_chunks" {
		t.Fatalf("created collection = %q, want code_chunks", got)
	}
	params := sdk.created.GetVectorsConfig().GetParamsMap().GetMap()["dense"]
	if params == nil {
		t.Fatal("expected named 'dense' vector in created collection")
	}
	if got := params.GetSize(); got != 384 {
		t.Fatalf("created vector size = %d, want 384", got)
	}
	if got := params.GetDistance(); got != pb.Distance_Cosine {
		t.Fatalf("created distance = %s, want %s", got, pb.Distance_Cosine)
	}
	// Verify payload indexes were created for filtered fields.
	if sdk.fieldIndexCalls != 3 {
		t.Fatalf("CreateFieldIndex calls = %d, want 3", sdk.fieldIndexCalls)
	}
	wantFields := []string{"repo", "language", "file_path"}
	for i, want := range wantFields {
		if i >= len(sdk.fieldIndexFields) {
			t.Fatalf("missing field index for %q", want)
		}
		if sdk.fieldIndexFields[i] != want {
			t.Fatalf("field index[%d] = %q, want %q", i, sdk.fieldIndexFields[i], want)
		}
	}
}

func TestClient_EnsureCollectionAcceptsMatchingExistingDimension(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{
		exists: true,
		info:   collectionInfoWithVectorSize(384),
	}
	client := &Client{conn: sdk}

	if err := client.EnsureCollection(context.Background(), "code_chunks", 384); err != nil {
		t.Fatalf("EnsureCollection() unexpected error: %v", err)
	}
	if sdk.existsCalls != 1 {
		t.Fatalf("CollectionExists calls = %d, want 1", sdk.existsCalls)
	}
	if sdk.infoCalls != 1 {
		t.Fatalf("GetCollectionInfo calls = %d, want 1", sdk.infoCalls)
	}
	if sdk.createCalls != 0 {
		t.Fatalf("CreateCollection calls = %d, want 0", sdk.createCalls)
	}
	// Payload indexes are no longer ensured here — Pipeline.Run() calls
	// EnsurePayloadIndexes separately to avoid redundant gRPC round-trips.
	if sdk.fieldIndexCalls != 0 {
		t.Fatalf("CreateFieldIndex calls = %d, want 0", sdk.fieldIndexCalls)
	}
}

func TestClient_EnsureCollectionRejectsMismatchedExistingDimension(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{
		exists: true,
		info:   collectionInfoWithVectorSize(768),
	}
	client := &Client{conn: sdk}

	err := client.EnsureCollection(context.Background(), "code_chunks", 384)
	if err == nil {
		t.Fatalf("expected error")
	}
	want := `collection "code_chunks" uses 768-dimensional vectors, but the current embedder produces 384-dimensional vectors`
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want substring %q", err.Error(), want)
	}
	if sdk.createCalls != 0 {
		t.Fatalf("CreateCollection calls = %d, want 0", sdk.createCalls)
	}
}

func TestClient_EnsureCollectionRejectsLegacyUnnamedSchema(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{
		exists: true,
		info:   collectionInfoLegacy(384),
	}
	client := &Client{conn: sdk}

	err := client.EnsureCollection(context.Background(), "code_chunks", 384)
	if err == nil {
		t.Fatal("expected error for legacy unnamed-vector schema")
	}
	if !strings.Contains(err.Error(), "legacy unnamed-vector schema") {
		t.Fatalf("error = %q, want substring about legacy schema", err.Error())
	}
	if !strings.Contains(err.Error(), "delete and re-index") {
		t.Fatalf("error = %q, want substring about delete and re-index", err.Error())
	}
}

func TestClient_EnsureCollectionWrapsExistenceError(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{
		existsErr: errors.New("connection refused"),
	}
	client := &Client{conn: sdk}

	err := client.EnsureCollection(context.Background(), "code_chunks", 384)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "checking collection code_chunks: connection refused") {
		t.Fatalf("error = %q, want wrapped existence error", err.Error())
	}
	if sdk.infoCalls != 0 {
		t.Fatalf("GetCollectionInfo calls = %d, want 0", sdk.infoCalls)
	}
	if sdk.createCalls != 0 {
		t.Fatalf("CreateCollection calls = %d, want 0", sdk.createCalls)
	}
}

func TestClient_EnsureCollectionWrapsInfoError(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{
		exists:  true,
		infoErr: errors.New("unavailable"),
	}
	client := &Client{conn: sdk}

	err := client.EnsureCollection(context.Background(), "code_chunks", 384)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "getting collection info for code_chunks: unavailable") {
		t.Fatalf("error = %q, want wrapped info error", err.Error())
	}
	if sdk.createCalls != 0 {
		t.Fatalf("CreateCollection calls = %d, want 0", sdk.createCalls)
	}
}

type fakeSDKClient struct {
	exists      bool
	existsErr   error
	info        *pb.CollectionInfo
	infoErr     error
	createErr   error
	existsCalls int
	infoCalls   int
	createCalls int
	created     *pb.CreateCollection

	// Field index recording.
	fieldIndexCalls  int
	fieldIndexFields []string
	fieldIndexErr    error

	// Query recording.
	queryResult []*pb.ScoredPoint
	queryErr    error
	queryCalls  int
	queriedReq  *pb.QueryPoints

	// Scroll recording.
	scrollPages  []scrollPage         // multi-page results; takes precedence if non-empty
	scrollResult []*pb.RetrievedPoint // single-page fallback
	scrollErr    error
	scrollCalls  int
	scrolledReq  *pb.ScrollPoints

	// Upsert recording.
	upsertCalls  int
	upsertedReqs []*pb.UpsertPoints

	// Delete recording.
	deleteErr   error
	deleteCalls int
	deletedReq  *pb.DeletePoints
}

func (f *fakeSDKClient) Close() error {
	return nil
}

func (f *fakeSDKClient) CollectionExists(_ context.Context, _ string) (bool, error) {
	f.existsCalls++
	return f.exists, f.existsErr
}

func (f *fakeSDKClient) GetCollectionInfo(_ context.Context, _ string) (*pb.CollectionInfo, error) {
	f.infoCalls++
	return f.info, f.infoErr
}

func (f *fakeSDKClient) CreateCollection(_ context.Context, request *pb.CreateCollection) error {
	f.createCalls++
	f.created = request
	return f.createErr
}

func (f *fakeSDKClient) CreateFieldIndex(_ context.Context, request *pb.CreateFieldIndexCollection) (*pb.UpdateResult, error) {
	f.fieldIndexCalls++
	f.fieldIndexFields = append(f.fieldIndexFields, request.GetFieldName())
	return &pb.UpdateResult{}, f.fieldIndexErr
}

func (f *fakeSDKClient) Upsert(_ context.Context, req *pb.UpsertPoints) (*pb.UpdateResult, error) {
	f.upsertCalls++
	f.upsertedReqs = append(f.upsertedReqs, req)
	return &pb.UpdateResult{}, nil
}

func (f *fakeSDKClient) Query(_ context.Context, req *pb.QueryPoints) ([]*pb.ScoredPoint, error) {
	f.queryCalls++
	f.queriedReq = req
	return f.queryResult, f.queryErr
}

func (f *fakeSDKClient) ScrollAndOffset(_ context.Context, req *pb.ScrollPoints) ([]*pb.RetrievedPoint, *pb.PointId, error) {
	f.scrollCalls++
	f.scrolledReq = req

	// Multi-page support: return the page matching the current call index.
	if len(f.scrollPages) > 0 {
		idx := f.scrollCalls - 1
		if idx >= len(f.scrollPages) {
			return nil, nil, f.scrollErr
		}
		page := f.scrollPages[idx]
		return page.points, page.nextOffset, f.scrollErr
	}

	return f.scrollResult, nil, f.scrollErr
}

func (f *fakeSDKClient) Delete(_ context.Context, req *pb.DeletePoints) (*pb.UpdateResult, error) {
	f.deleteCalls++
	f.deletedReq = req
	return &pb.UpdateResult{}, f.deleteErr
}

func (f *fakeSDKClient) ListCollections(context.Context) ([]string, error) {
	panic("unexpected ListCollections call")
}

func (f *fakeSDKClient) DeleteCollection(context.Context, string) error {
	panic("unexpected DeleteCollection call")
}

func collectionInfoWithVectorSize(size uint64) *pb.CollectionInfo {
	return &pb.CollectionInfo{
		Config: &pb.CollectionConfig{
			Params: &pb.CollectionParams{
				VectorsConfig: pb.NewVectorsConfigMap(map[string]*pb.VectorParams{
					"dense": {
						Size:     size,
						Distance: pb.Distance_Cosine,
					},
				}),
			},
		},
	}
}

// collectionInfoLegacy returns a CollectionInfo with the old unnamed-vector
// schema. Used to test legacy schema detection and migration error.
func collectionInfoLegacy(size uint64) *pb.CollectionInfo {
	return &pb.CollectionInfo{
		Config: &pb.CollectionConfig{
			Params: &pb.CollectionParams{
				VectorsConfig: pb.NewVectorsConfig(&pb.VectorParams{
					Size:     size,
					Distance: pb.Distance_Cosine,
				}),
			},
		},
	}
}

// --- Search filter tests ---

func TestClient_SearchNoFilters(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{}
	client := &Client{conn: sdk}

	_, err := client.Search(context.Background(), "code_chunks", []float32{1, 2, 3}, 5, nil, nil)
	if err != nil {
		t.Fatalf("Search() unexpected error: %v", err)
	}
	if sdk.queryCalls != 1 {
		t.Fatalf("Query calls = %d, want 1", sdk.queryCalls)
	}
	if sdk.queriedReq.Filter != nil {
		t.Fatal("expected nil filter when no languages or repos")
	}
	if got := sdk.queriedReq.GetUsing(); got != "dense" {
		t.Fatalf("Using = %q, want \"dense\"", got)
	}
}

func TestClient_SearchLanguageOnly(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{}
	client := &Client{conn: sdk}

	_, err := client.Search(context.Background(), "code_chunks", []float32{1, 2, 3}, 5, []string{"go", "rust"}, nil)
	if err != nil {
		t.Fatalf("Search() unexpected error: %v", err)
	}

	filter := sdk.queriedReq.Filter
	if filter == nil {
		t.Fatal("expected non-nil filter")
	}
	if len(filter.Must) != 1 {
		t.Fatalf("Must conditions = %d, want 1", len(filter.Must))
	}
	innerFilter := filter.Must[0].GetFilter()
	if innerFilter == nil {
		t.Fatal("expected nested filter in Must[0]")
	}
	if len(innerFilter.Should) != 2 {
		t.Fatalf("language Should conditions = %d, want 2", len(innerFilter.Should))
	}
}

func TestClient_SearchRepoOnly(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{}
	client := &Client{conn: sdk}

	_, err := client.Search(context.Background(), "code_chunks", []float32{1, 2, 3}, 5, nil, []string{"ragcodepilot"})
	if err != nil {
		t.Fatalf("Search() unexpected error: %v", err)
	}

	filter := sdk.queriedReq.Filter
	if filter == nil {
		t.Fatal("expected non-nil filter")
	}
	if len(filter.Must) != 1 {
		t.Fatalf("Must conditions = %d, want 1", len(filter.Must))
	}
	innerFilter := filter.Must[0].GetFilter()
	if innerFilter == nil {
		t.Fatal("expected nested filter in Must[0]")
	}
	if len(innerFilter.Should) != 1 {
		t.Fatalf("repo Should conditions = %d, want 1", len(innerFilter.Should))
	}
}

func TestClient_SearchLanguageAndRepo(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{}
	client := &Client{conn: sdk}

	_, err := client.Search(context.Background(), "code_chunks", []float32{1, 2, 3}, 5, []string{"go"}, []string{"ragcodepilot", "other"})
	if err != nil {
		t.Fatalf("Search() unexpected error: %v", err)
	}

	filter := sdk.queriedReq.Filter
	if filter == nil {
		t.Fatal("expected non-nil filter")
	}
	if len(filter.Must) != 2 {
		t.Fatalf("Must conditions = %d, want 2 (language AND repo)", len(filter.Must))
	}

	// First Must = language filter (1 Should condition).
	langFilter := filter.Must[0].GetFilter()
	if langFilter == nil || len(langFilter.Should) != 1 {
		t.Fatalf("expected 1 language Should condition")
	}

	// Second Must = repo filter (2 Should conditions).
	repoFilter := filter.Must[1].GetFilter()
	if repoFilter == nil || len(repoFilter.Should) != 2 {
		t.Fatalf("expected 2 repo Should conditions")
	}
}

func TestClient_SearchReturnsResults(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{
		queryResult: []*pb.ScoredPoint{
			{
				Score: 0.95,
				Payload: map[string]*pb.Value{
					"repo":       {Kind: &pb.Value_StringValue{StringValue: "ragcodepilot"}},
					"file_path":  {Kind: &pb.Value_StringValue{StringValue: "main.go"}},
					"language":   {Kind: &pb.Value_StringValue{StringValue: "go"}},
					"chunk_type": {Kind: &pb.Value_StringValue{StringValue: "function"}},
					"name":       {Kind: &pb.Value_StringValue{StringValue: "Run"}},
					"content":    {Kind: &pb.Value_StringValue{StringValue: "func Run() {}"}},
					"start_line": {Kind: &pb.Value_IntegerValue{IntegerValue: 10}},
					"end_line":   {Kind: &pb.Value_IntegerValue{IntegerValue: 20}},
				},
			},
		},
	}
	client := &Client{conn: sdk}

	results, err := client.Search(context.Background(), "code_chunks", []float32{1, 2, 3}, 5, nil, nil)
	if err != nil {
		t.Fatalf("Search() unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results count = %d, want 1", len(results))
	}
	r := results[0]
	if r.Score != 0.95 {
		t.Errorf("score = %f, want 0.95", r.Score)
	}
	if r.Chunk.Repo != "ragcodepilot" {
		t.Errorf("repo = %q, want ragcodepilot", r.Chunk.Repo)
	}
	if r.Chunk.Name != "Run" {
		t.Errorf("name = %q, want Run", r.Chunk.Name)
	}
}

// scrollPage holds points and the next offset for a single scroll page.
type scrollPage struct {
	points     []*pb.RetrievedPoint
	nextOffset *pb.PointId
}

// --- EnsurePayloadIndexes tests ---

func TestClient_EnsurePayloadIndexesCreatesIndexesForExistingCollection(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{exists: true}
	client := &Client{conn: sdk}

	if err := client.EnsurePayloadIndexes(context.Background(), "code_chunks"); err != nil {
		t.Fatalf("EnsurePayloadIndexes() unexpected error: %v", err)
	}
	if sdk.fieldIndexCalls != 3 {
		t.Fatalf("CreateFieldIndex calls = %d, want 3", sdk.fieldIndexCalls)
	}
	wantFields := []string{"repo", "language", "file_path"}
	for i, want := range wantFields {
		if sdk.fieldIndexFields[i] != want {
			t.Fatalf("field index[%d] = %q, want %q", i, sdk.fieldIndexFields[i], want)
		}
	}
}

func TestClient_EnsurePayloadIndexesNoOpForMissingCollection(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{exists: false}
	client := &Client{conn: sdk}

	if err := client.EnsurePayloadIndexes(context.Background(), "code_chunks"); err != nil {
		t.Fatalf("EnsurePayloadIndexes() unexpected error: %v", err)
	}
	if sdk.fieldIndexCalls != 0 {
		t.Fatalf("CreateFieldIndex calls = %d, want 0 for missing collection", sdk.fieldIndexCalls)
	}
}

// --- ScrollFileHashes tests ---

func TestClient_ScrollFileHashes(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{
		exists: true,
		scrollPages: []scrollPage{
			{
				points: []*pb.RetrievedPoint{
					{
						Payload: map[string]*pb.Value{
							"file_path": {Kind: &pb.Value_StringValue{StringValue: "internal/main.go"}},
							"file_hash": {Kind: &pb.Value_StringValue{StringValue: "abc123"}},
						},
					},
					{
						Payload: map[string]*pb.Value{
							"file_path": {Kind: &pb.Value_StringValue{StringValue: "internal/utils.go"}},
							"file_hash": {Kind: &pb.Value_StringValue{StringValue: "def456"}},
						},
					},
					// Duplicate file_path — should be deduplicated (last wins).
					{
						Payload: map[string]*pb.Value{
							"file_path": {Kind: &pb.Value_StringValue{StringValue: "internal/main.go"}},
							"file_hash": {Kind: &pb.Value_StringValue{StringValue: "abc123"}},
						},
					},
				},
				nextOffset: nil, // single page — done
			},
		},
	}
	client := &Client{conn: sdk}

	hashes, err := client.ScrollFileHashes(context.Background(), "code_chunks", "ragcodepilot", nil)
	if err != nil {
		t.Fatalf("ScrollFileHashes() unexpected error: %v", err)
	}

	if len(hashes) != 2 {
		t.Fatalf("expected 2 unique files, got %d", len(hashes))
	}
	if hashes["internal/main.go"] != "abc123" {
		t.Errorf("main.go hash = %q, want abc123", hashes["internal/main.go"])
	}
	if hashes["internal/utils.go"] != "def456" {
		t.Errorf("utils.go hash = %q, want def456", hashes["internal/utils.go"])
	}
}

func TestClient_ScrollFileHashesEmptyWhenNoCollection(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{exists: false}
	client := &Client{conn: sdk}

	hashes, err := client.ScrollFileHashes(context.Background(), "code_chunks", "ragcodepilot", nil)
	if err != nil {
		t.Fatalf("ScrollFileHashes() unexpected error: %v", err)
	}
	if len(hashes) != 0 {
		t.Fatalf("expected 0 hashes for missing collection, got %d", len(hashes))
	}
	if sdk.scrollCalls != 0 {
		t.Fatalf("Scroll calls = %d, want 0 (should not scroll missing collection)", sdk.scrollCalls)
	}
}

// --- DeleteByFilePaths tests ---

func TestClient_DeleteStaleChunksByFilePath(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{}
	client := &Client{conn: sdk}

	err := client.DeleteStaleChunksByFilePath(context.Background(), "code_chunks", "myrepo", "internal/main.go", "hash_new")
	if err != nil {
		t.Fatalf("DeleteStaleChunksByFilePath() unexpected error: %v", err)
	}
	if sdk.deleteCalls != 1 {
		t.Fatalf("Delete calls = %d, want 1", sdk.deleteCalls)
	}
	if !sdk.deletedReq.GetWait() {
		t.Fatal("expected Wait: true")
	}

	filter := sdk.deletedReq.GetPoints().GetFilter()
	if filter == nil {
		t.Fatal("expected non-nil filter")
	}
	// Must: repo + file_path
	if len(filter.Must) != 2 {
		t.Fatalf("Must conditions = %d, want 2 (repo + file_path)", len(filter.Must))
	}
	// MustNot: file_hash != current_hash (excludes freshly upserted chunks)
	if len(filter.MustNot) != 1 {
		t.Fatalf("MustNot conditions = %d, want 1 (file_hash exclusion)", len(filter.MustNot))
	}
}

func TestClient_DeleteByFilePaths(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{}
	client := &Client{conn: sdk}

	err := client.DeleteByFilePaths(context.Background(), "code_chunks", "ragcodepilot", []string{"main.go", "utils.go"})
	if err != nil {
		t.Fatalf("DeleteByFilePaths() unexpected error: %v", err)
	}
	if sdk.deleteCalls != 1 {
		t.Fatalf("Delete calls = %d, want 1", sdk.deleteCalls)
	}

	// Verify filter structure: Must[0] = repo match, Must[1] = Should(file_path matches).
	filter := sdk.deletedReq.GetPoints().GetFilter()
	if filter == nil {
		t.Fatal("expected non-nil filter")
	}
	if len(filter.Must) != 2 {
		t.Fatalf("Must conditions = %d, want 2", len(filter.Must))
	}
	fileFilter := filter.Must[1].GetFilter()
	if fileFilter == nil || len(fileFilter.Should) != 2 {
		t.Fatalf("expected 2 file_path Should conditions")
	}
}

func TestClient_DeleteByFilePathsNoOpOnEmpty(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{}
	client := &Client{conn: sdk}

	err := client.DeleteByFilePaths(context.Background(), "code_chunks", "ragcodepilot", nil)
	if err != nil {
		t.Fatalf("DeleteByFilePaths() unexpected error: %v", err)
	}
	if sdk.deleteCalls != 0 {
		t.Fatalf("Delete calls = %d, want 0 for empty file paths", sdk.deleteCalls)
	}
}

// --- Upsert tests ---

func TestClient_UpsertUsesNamedDenseVector(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{}
	client := &Client{conn: sdk}

	chunks := []model.CodeChunk{{
		ID:       "00000000-0000-0000-0000-000000000001",
		Repo:     "testrepo",
		FilePath: "main.go",
		Language: "go",
		Content:  "func main() {}",
	}}
	vectors := [][]float32{{1.0, 2.0, 3.0}}

	err := client.Upsert(context.Background(), "code_chunks", chunks, vectors)
	if err != nil {
		t.Fatalf("Upsert() unexpected error: %v", err)
	}
	if sdk.upsertCalls != 1 {
		t.Fatalf("Upsert calls = %d, want 1", sdk.upsertCalls)
	}

	point := sdk.upsertedReqs[0].GetPoints()[0]
	vectorsMap := point.GetVectors().GetVectors().GetVectors()
	if vectorsMap == nil {
		t.Fatal("expected VectorsMap, got nil")
	}
	denseVec, ok := vectorsMap["dense"]
	if !ok {
		t.Fatal("expected 'dense' key in VectorsMap")
	}
	if len(denseVec.GetDense().GetData()) != 3 {
		t.Fatalf("dense vector length = %d, want 3", len(denseVec.GetDense().GetData()))
	}
}

func TestClient_UpsertBatchSplitting(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{}
	client := &Client{conn: sdk}

	// Create 130 chunks → expect 3 SDK calls: 64 + 64 + 2
	const totalChunks = 130
	chunks := make([]model.CodeChunk, totalChunks)
	vectors := make([][]float32, totalChunks)
	for i := range chunks {
		chunks[i] = model.CodeChunk{
			ID:       fmt.Sprintf("00000000-0000-0000-0000-%012d", i),
			Repo:     "testrepo",
			FilePath: fmt.Sprintf("file_%d.go", i),
			Language: "go",
			Content:  "func main() {}",
		}
		vectors[i] = []float32{1.0, 2.0, 3.0}
	}

	err := client.Upsert(context.Background(), "code_chunks", chunks, vectors)
	if err != nil {
		t.Fatalf("Upsert() unexpected error: %v", err)
	}

	// Verify batch count.
	if sdk.upsertCalls != 3 {
		t.Fatalf("Upsert SDK calls = %d, want 3", sdk.upsertCalls)
	}

	// Verify batch sizes.
	wantSizes := []int{64, 64, 2}
	for i, req := range sdk.upsertedReqs {
		got := len(req.GetPoints())
		if got != wantSizes[i] {
			t.Errorf("batch %d size = %d, want %d", i, got, wantSizes[i])
		}
	}

	// Verify total points cover all chunks.
	totalPoints := 0
	for _, req := range sdk.upsertedReqs {
		totalPoints += len(req.GetPoints())
		// Verify Wait is set on every batch.
		if !req.GetWait() {
			t.Error("expected Wait: true on upsert batch")
		}
	}
	if totalPoints != totalChunks {
		t.Fatalf("total upserted points = %d, want %d", totalPoints, totalChunks)
	}
}

// --- Multi-page scroll test ---

func TestClient_ScrollFileHashesMultiPage(t *testing.T) {
	t.Parallel()

	page2Offset := pb.NewIDNum(1001)

	sdk := &fakeSDKClient{
		exists: true,
		scrollPages: []scrollPage{
			{
				points: []*pb.RetrievedPoint{
					{
						Payload: map[string]*pb.Value{
							"file_path": {Kind: &pb.Value_StringValue{StringValue: "file_a.go"}},
							"file_hash": {Kind: &pb.Value_StringValue{StringValue: "hash_a"}},
						},
					},
				},
				nextOffset: page2Offset, // more pages
			},
			{
				points: []*pb.RetrievedPoint{
					{
						Payload: map[string]*pb.Value{
							"file_path": {Kind: &pb.Value_StringValue{StringValue: "file_b.go"}},
							"file_hash": {Kind: &pb.Value_StringValue{StringValue: "hash_b"}},
						},
					},
				},
				nextOffset: nil, // last page
			},
		},
	}
	client := &Client{conn: sdk}

	hashes, err := client.ScrollFileHashes(context.Background(), "code_chunks", "ragcodepilot", nil)
	if err != nil {
		t.Fatalf("ScrollFileHashes() unexpected error: %v", err)
	}

	// Verify both pages were scrolled.
	if sdk.scrollCalls != 2 {
		t.Fatalf("Scroll calls = %d, want 2", sdk.scrollCalls)
	}

	// Verify both files collected.
	if len(hashes) != 2 {
		t.Fatalf("expected 2 unique files, got %d", len(hashes))
	}
	if hashes["file_a.go"] != "hash_a" {
		t.Errorf("file_a.go hash = %q, want hash_a", hashes["file_a.go"])
	}
	if hashes["file_b.go"] != "hash_b" {
		t.Errorf("file_b.go hash = %q, want hash_b", hashes["file_b.go"])
	}

	// Verify the second call used the offset from page 1.
	if sdk.scrolledReq.GetOffset().GetNum() != page2Offset.GetNum() {
		t.Fatalf("second scroll offset = %v, want %v", sdk.scrolledReq.GetOffset(), page2Offset)
	}
}

func TestClient_ScrollFileHashesWithLanguageFilter(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{
		exists: true,
		scrollPages: []scrollPage{
			{
				points: []*pb.RetrievedPoint{
					{
						Payload: map[string]*pb.Value{
							"file_path": {Kind: &pb.Value_StringValue{StringValue: "main.go"}},
							"file_hash": {Kind: &pb.Value_StringValue{StringValue: "go_hash"}},
						},
					},
				},
				nextOffset: nil,
			},
		},
	}
	client := &Client{conn: sdk}

	hashes, err := client.ScrollFileHashes(context.Background(), "code_chunks", "ragcodepilot", []string{"go", "rust"})
	if err != nil {
		t.Fatalf("ScrollFileHashes() unexpected error: %v", err)
	}
	if len(hashes) != 1 {
		t.Fatalf("expected 1 file, got %d", len(hashes))
	}

	// Verify the scroll filter includes both repo AND language conditions.
	filter := sdk.scrolledReq.GetFilter()
	if filter == nil {
		t.Fatal("expected non-nil filter")
	}
	// Must[0] = repo match, Must[1] = language Should-filter.
	if len(filter.Must) != 2 {
		t.Fatalf("Must conditions = %d, want 2 (repo + language)", len(filter.Must))
	}
	langFilter := filter.Must[1].GetFilter()
	if langFilter == nil {
		t.Fatal("expected nested language filter in Must[1]")
	}
	if len(langFilter.Should) != 2 {
		t.Fatalf("language Should conditions = %d, want 2", len(langFilter.Should))
	}
}

func TestClient_ScrollFileHashesNoLanguageFilterWhenEmpty(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDKClient{
		exists:       true,
		scrollResult: []*pb.RetrievedPoint{},
	}
	client := &Client{conn: sdk}

	_, err := client.ScrollFileHashes(context.Background(), "code_chunks", "ragcodepilot", nil)
	if err != nil {
		t.Fatalf("ScrollFileHashes() unexpected error: %v", err)
	}

	// Verify the scroll filter only has repo — no language filter.
	filter := sdk.scrolledReq.GetFilter()
	if filter == nil {
		t.Fatal("expected non-nil filter")
	}
	if len(filter.Must) != 1 {
		t.Fatalf("Must conditions = %d, want 1 (repo only)", len(filter.Must))
	}
}
