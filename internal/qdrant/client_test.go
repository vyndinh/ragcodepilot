package qdrant

import (
	"context"
	"errors"
	"strings"
	"testing"

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
	params := sdk.created.GetVectorsConfig().GetParams()
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

func (f *fakeSDKClient) Upsert(context.Context, *pb.UpsertPoints) (*pb.UpdateResult, error) {
	panic("unexpected Upsert call")
}

func (f *fakeSDKClient) Query(_ context.Context, req *pb.QueryPoints) ([]*pb.ScoredPoint, error) {
	f.queryCalls++
	f.queriedReq = req
	return f.queryResult, f.queryErr
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

	_, err := client.Search(context.Background(), "code_chunks", []float32{1, 2, 3}, 5, nil, []string{"ragsearch"})
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

	_, err := client.Search(context.Background(), "code_chunks", []float32{1, 2, 3}, 5, []string{"go"}, []string{"ragsearch", "other"})
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
					"repo":       {Kind: &pb.Value_StringValue{StringValue: "ragsearch"}},
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
	if r.Chunk.Repo != "ragsearch" {
		t.Errorf("repo = %q, want ragsearch", r.Chunk.Repo)
	}
	if r.Chunk.Name != "Run" {
		t.Errorf("name = %q, want Run", r.Chunk.Name)
	}
}
