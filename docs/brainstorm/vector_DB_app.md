## **Mental model: what a vector database really does**

A vector database stores objects like this:

Point {  
 id: "doc-123",  
 vector: \[0.12, \-0.31, 0.88, ...\],  
 payload: {  
   title: "Rust ownership notes",  
   language: "rust",  
   repo: "my-vector-db"  
 }  
}

Then you ask:

“Find the top 10 points whose vectors are closest to this query vector,  
but only where payload.language \= 'rust'.”

The database must solve five main problems:

1. **Vector math**: cosine similarity, dot product, Euclidean distance.  
2. **Nearest-neighbor search**: find closest vectors.  
3. **Indexing**: avoid scanning every vector when data becomes large.  
4. **Filtering**: combine vector search with metadata constraints.  
5. **Storage/reliability**: persist vectors, payloads, indexes, deletes, snapshots, WAL, compaction.

Faiss describes the core search problem as building an index over `d`\-dimensional vectors and then finding nearest vectors by distance or inner product; it also supports `k` nearest neighbors, range search, speed/precision tradeoffs, disk storage, and filtering by IDs [https://faiss.ai/](https://faiss.ai/). Qdrant models data as **collections** containing **points**, where each point has vectors plus payload, and vectors in a collection usually share dimensionality and a distance metric such as dot, cosine, Euclidean, or Manhattan. [https://qdrant.tech/documentation/manage-data/collections/](https://qdrant.tech/documentation/manage-data/collections/) 

The most important indexing idea to study deeply is **ANN**, approximate nearest-neighbor search. HNSW, one of the most popular algorithms, builds a multi-layer graph of vectors and searches from upper layers down to lower layers, trading perfect accuracy for much faster search. 

[https://arxiv.org/abs/1603.09320](https://arxiv.org/abs/1603.09320)

---

# **Best project idea for you: build a “mini Qdrant/Milvus for code search”**

Because your next goal is to refactor a Rust repository to Go, the best learning project is:

## **CodeVectorDB: a small vector database \+ semantic code search system**

You will build a system that can ingest source code repositories, chunk files/functions, create embeddings, store them in your own vector DB, and search them semantically.

Example queries:

"Where is WAL recovery implemented?"  
"Find code related to HNSW neighbor selection"  
"Show Rust modules that handle payload filtering"  
"Find equivalent logic to port into Go"

This project gives you two benefits:

First, you understand vector DB applications from the outside: embeddings, RAG, semantic search, metadata filters, ranking.

Second, you understand vector DB internals from the inside: vector storage, indexes, filtering, persistence, compaction, query execution.

---

# 

# 

# 

# 

# **System architecture**

Start with this architecture:

            ┌────────────────────┐  
            │  Source code repo                     │  
            └────────────────────┘  
                      │  
                      v  
            ┌────────────────────┐  
            │  Chunker/parser                         │  
            │  file/function AST                       │  
            └────────────────────┘  
                      │  
                      v  
            ┌────────────────────┐  
            │ Embedding generator                │  
            └────────────────────┘  
                      │  
                      v  
┌──────────────────────────────────────────┐  
│              Your Go Vector DB                                                               │  
│                                                                                                            │  
│  Collections                                                                                        │  
│  Segments                                                                                         │  
│  Vector storage                                                                                  │                                                         
│  Payload storage                                                                               │  
│  Flat index first                                                                                  │  
│  HNSW index later                                                                            │  
│  WAL \+ snapshot                                                                              │  
│  Payload filters                                                                                   │  
└──────────────────────────────────────────┘  
                    │  
                    v  
          ┌──────────────────┐  
          │ REST/gRPC/CLI API            │  
          └──────────────────┘

Your first version should not depend on a complex embedding model. Begin with generated vectors or simple local embeddings so you can focus on database internals. After the DB works, connect real embeddings.

---

# **Phase 1: understand vector DB by building a tiny flat-search database**

Goal: build the simplest possible vector DB.

## **Data model**

Implement these Go structs:

type PointID string

type Vector \[\]float32

type Payload map\[string\]any

type Point struct {  
   ID      PointID  
   Vector  Vector  
   Payload Payload  
}

type Collection struct {  
   Name      string  
   Dim       int  
   Metric    MetricType  
   Points    map\[PointID\]Point  
}

Supported metrics:

type MetricType string

const (  
   Cosine MetricType \= "cosine"  
   Dot    MetricType \= "dot"  
   L2     MetricType \= "l2"  
)

## **API v1**

Implement:

CreateCollection(name, dim, metric)  
Upsert(collection, point)  
Delete(collection, id)  
Search(collection, queryVector, topK)  
Get(collection, id)

Search should use brute force:

for every point:  
   score \= similarity(query, point.vector)  
   keep topK in min-heap

This teaches the most important base layer: a vector DB is just nearest-neighbor search plus storage and metadata.

Milvus describes an index as an extra structure built over data that speeds search but costs preprocessing time, memory, and sometimes recall; that is exactly why you should first build search without any ANN index. 

[https://milvus.io/docs/index-explained.md](https://milvus.io/docs/index-explained.md) 

---

# **Phase 2: add payload filtering**

Real vector DB queries are rarely pure vector search. They usually look like:

{  
 "vector": \[0.1, 0.2, 0.3\],  
 "top\_k": 10,  
 "filter": {  
   "repo": "qdrant",  
   "language": "rust",  
   "path\_prefix": "lib/segment"  
 }  
}

Implement filters:

field \== value  
field in \[...\]  
number range  
path prefix

Start with simple filtering:

for every point:  
   if payload matches filter:  
       compute vector score

Then add payload indexes:

type PayloadIndex struct {  
   Keyword map\[string\]map\[string\]map\[PointID\]struct{}  
}

Example:

repo \= "qdrant" \-\> {id1, id7, id9}  
language \= "rust" \-\> {id1, id2, id7}

Then the query planner can do:

candidateIDs \= intersection(repo=qdrant, language=rust)  
run vector search only on candidateIDs

Qdrant’s docs emphasize that payload indexes are useful for fields used in filtering, and that filterable vector search is a major challenge because vector indexes and payload indexes must work together. 

[https://qdrant.tech/documentation/manage-data/indexing/](https://qdrant.tech/documentation/manage-data/indexing/)

---

# **Phase 3: add segments**

Do not store everything in one giant map forever. Production vector DBs commonly split data into **segments**.

A segment is a self-contained unit:

Segment {  
 vector storage  
 payload storage  
 deleted bitmap  
 vector index  
 payload indexes  
}

Qdrant stores collection data in segments, and each segment has independent vector storage, payload storage, indexes, and ID mapping. 

[https://qdrant.tech/documentation/manage-data/storage/](https://qdrant.tech/documentation/manage-data/storage/)

Implement:

type Segment interface {  
   Upsert(point Point) error  
   Delete(id PointID) error  
   Search(query Vector, topK int, filter Filter) \[\]ScoredPoint  
   Flush() error  
}

Have two segment types:

MutableSegment   // accepts writes  
SealedSegment    // read mostly, optimized

When a mutable segment reaches a size threshold:

mutable segment \-\> flush \-\> sealed segment  
new mutable segment starts

This gives you the foundation for compaction, indexing, and persistence.

---

# **Phase 4: add persistence**

Now make the DB survive restart.

Implement these files:

data/  
 collections/  
   code/  
     manifest.json  
     wal.log  
     segments/  
       segment-0001/  
         vectors.bin  
         payloads.jsonl  
         deleted.bin  
         index.meta  
       segment-0002/  
         vectors.bin  
         payloads.jsonl  
         deleted.bin  
         index.meta

Start simple.

## **WAL format**

Use JSONL first:

{"op":"upsert","collection":"code","id":"1","vector":\[...\],"payload":{...}}  
{"op":"delete","collection":"code","id":"1"}

On startup:

load snapshot  
replay WAL  
rebuild in-memory indexes

Later you can replace JSONL with a binary format.

Milvus’s architecture overview describes WAL storage as the foundation for durability and consistency: writes are recorded before commit so the system can recover after failure. 

[https://milvus.io/docs/architecture\_overview.md](https://milvus.io/docs/architecture_overview.md)

---

# **Phase 5: add compaction**

Deletes should not immediately rewrite large files. Use tombstones first:

Delete(id) \-\> mark id as deleted  
Search() \-\> skip deleted ids

Then periodically compact:

read old segments  
remove deleted points  
merge small segments  
write new segment  
swap manifest atomically  
delete old segments

This teaches one of the most important database ideas: immutable/sealed files plus background cleanup.

---

# **Phase 6: implement HNSW**

Only after flat search, filters, segments, WAL, and compaction work should you implement HNSW.

HNSW concepts to learn:

M              max neighbors per node  
efConstruction size of candidate set during insert  
efSearch       size of candidate set during search  
layers         random level per vector  
entry point    starting node at top layer

Basic HNSW flow:

Insert vector:  
 1\. choose random max layer  
 2\. search from current entry point down upper layers  
 3\. at each layer, find nearest candidates  
 4\. connect new node to selected neighbors  
 5\. update neighbor lists

Search flow:

Search query:  
 1\. start from entry point at top layer  
 2\. greedily move closer to query  
 3\. descend layers  
 4\. at layer 0, explore efSearch candidates  
 5\. return topK

HNSW is worth studying because it is graph-based, incremental, tunable, and widely used. The original HNSW paper describes a hierarchical set of proximity graphs where upper layers help navigate quickly and lower layers refine the search. 

[https://arxiv.org/abs/1603.09320](https://arxiv.org/abs/1603.09320)

---

# **Phase 7: combine HNSW with filtering**

This is where vector DBs become truly interesting.

Naive filtered HNSW can fail:

Search HNSW \-\> get top 100 \-\> filter payload \-\> maybe only 1 remains

Better options:

Option A: pre-filter  
 Use payload index first, then search only candidates.

Option B: post-filter  
 Search vector index first, then filter results.

Option C: hybrid planner  
 If filter is very selective, use pre-filter \+ flat scan.  
 If filter is broad, use HNSW \+ post-filter.  
 If middle case, use filtered HNSW.

Qdrant’s documentation discusses this exact difficulty: separate payload and vector indexes do not fully solve filtered search, and strict filters can make the HNSW graph less useful; Qdrant addresses this with filterable HNSW graph extensions. 

Your simpler implementation can use a planner:

func ChoosePlan(filter Filter, estimatedCandidates int, totalPoints int) SearchPlan {  
   ratio := float64(estimatedCandidates) / float64(totalPoints)

   switch {  
   case ratio \< 0.05:  
       return PlanPayloadThenFlat  
   case ratio \> 0.40:  
       return PlanHNSWThenFilter  
   default:  
       return PlanHybridOverfetch  
   }  
}

For `PlanHybridOverfetch`, search HNSW with larger `efSearch` and request more than `topK`, then filter.

---

# **Phase 8: build the code-search application on top**

Now build the app that uses your DB.

## **Ingestion pipeline**

1\. Walk repository files  
2\. Ignore vendor/build/target/node\_modules  
3\. Split code into chunks:  
  \- file-level chunk  
  \- function-level chunk  
  \- struct/interface-level chunk  
4\. Generate embedding per chunk  
5\. Store point:  
  id \= hash(repo \+ path \+ symbol \+ range)  
  vector \= embedding  
  payload \= metadata

Payload example:

{  
 "repo": "qdrant",  
 "language": "rust",  
 "path": "lib/segment/src/vector\_storage.rs",  
 "symbol": "VectorStorage",  
 "start\_line": 42,  
 "end\_line": 120,  
 "chunk\_type": "struct"  
}

## **Search API**

POST /collections/code/search  
{  
 "query": "where does recovery from WAL happen?",  
 "top\_k": 10,  
 "filter": {  
   "repo": "qdrant",  
   "language": "rust"  
 }  
}

Response:

\[  
 {  
   "score": 0.83,  
   "path": "lib/collection/src/wal.rs",  
   "symbol": "recover",  
   "start\_line": 33,  
   "end\_line": 91,  
   "text": "..."  
 }  
\]

This application directly helps your later Rust-to-Go refactor because you can ask semantic questions about the Rust repo before porting.

---

# **Suggested learning roadmap**

## **Month 1: build the base vector DB**

### **Week 1: vector math and flat search**

Deliverables:

\- Go module initialized  
\- Collection creation  
\- Upsert/delete/get  
\- Cosine/dot/L2  
\- Brute-force topK search  
\- Unit tests  
\- Benchmark: 10K, 100K vectors

Key lesson:

Understand exact nearest-neighbor search before approximate search.

### **Week 2: payloads and filters**

Deliverables:

\- JSON payload support  
\- Filter language  
\- Keyword payload index  
\- Candidate estimation  
\- Query planner v1

Key lesson:

Vector DB \= vector search \+ structured filtering.

### **Week 3: persistence**

Deliverables:

\- WAL  
\- Snapshot  
\- Restart recovery  
\- Deleted tombstones  
\- Basic manifest file

Key lesson:

A vector DB is still a database. Durability matters.

### **Week 4: segments**

Deliverables:

\- Mutable segments  
\- Sealed segments  
\- Segment search fanout  
\- Segment merge/compaction  
\- Atomic manifest swap

Key lesson:

Segments make indexing, persistence, and compaction manageable.  
---

## **Month 2: build ANN and application layer**

### **Week 5-6: HNSW**

Deliverables:

\- HNSW insert  
\- HNSW search  
\- Config: M, efConstruction, efSearch  
\- Recall benchmark against flat search

Benchmark like this:

Flat search result \= ground truth  
HNSW result \= approximate result

Recall@10 \= overlap(HNSW top10, Flat top10) / 10

Track:

\- latency  
\- recall  
\- memory usage  
\- index build time

### **Week 7: filtered vector search**

Deliverables:

\- pre-filter \+ flat  
\- HNSW \+ post-filter  
\- hybrid overfetch  
\- planner based on selectivity

### **Week 8: code-search app**

Deliverables:

\- repo crawler  
\- code chunker  
\- embedding worker  
\- search CLI  
\- REST API

Example CLI:

vecdb ingest \--repo ./qdrant \--collection code  
vecdb search \--collection code "how does payload filtering work?" \--filter language=rust  
---

# **Go package structure**

Use a clean modular design:

vecdb/  
 cmd/  
   vecdb/  
     main.go

 internal/  
   api/  
     http.go  
     grpc.go

   collection/  
     collection.go  
     manifest.go

   segment/  
     segment.go  
     mutable.go  
     sealed.go  
     compaction.go

   vector/  
     distance.go  
     normalize.go

   index/  
     flat/  
       flat.go  
     hnsw/  
       graph.go  
       insert.go  
       search.go

   payload/  
     payload.go  
     filter.go  
     keyword\_index.go  
     planner.go

   storage/  
     wal.go  
     snapshot.go  
     mmap.go

   ingest/  
     repo\_walker.go  
     chunker.go  
     embedder.go

   benchmark/  
     recall.go  
     latency.go

Important interfaces:

type VectorIndex interface {  
   Add(id uint64, vector \[\]float32) error  
   Delete(id uint64) error  
   Search(query \[\]float32, topK int, opts SearchOptions) (\[\]ScoredID, error)  
   Save(path string) error  
   Load(path string) error  
}  
type Segment interface {  
   ID() string  
   Upsert(point Point) error  
   Delete(id PointID) error  
   Search(req SearchRequest) (\[\]ScoredPoint, error)  
   Flush() error  
   Size() int  
}  
type PayloadIndexer interface {  
   Index(point Point) error  
   Remove(id PointID) error  
   Estimate(filter Filter) int  
   CandidateIDs(filter Filter) RoaringBitmapOrSet  
}

You can start with Go maps. Later replace sets with roaring bitmaps.

---

# **What to learn from existing vector DBs**

Study these systems, but do not copy everything at once.

## **Faiss**

Study for:

\- vector index types  
\- brute-force vs IVF vs PQ  
\- recall/speed/memory tradeoffs  
\- benchmark methodology

Faiss is mainly a similarity-search/indexing library, not a full database with rich persistence and distributed metadata management. Its docs explicitly focus on efficient similarity search and clustering of dense vectors. 

[https://faiss.ai/](https://faiss.ai/)

## **Qdrant**

Study for:

\- collection/point/payload model  
\- segment architecture  
\- HNSW  
\- payload filtering  
\- filterable HNSW  
\- Rust implementation style

Qdrant’s storage design is especially valuable because it uses collection segments with independent vector storage, payload storage, indexes, and ID mapping. 

## **Milvus**

Study for:

\- distributed architecture  
\- access layer  
\- coordinator  
\- worker nodes  
\- object storage  
\- WAL  
\- compaction/index building

Milvus separates access, coordination, worker nodes, and storage; it also separates storage and compute and uses WAL for durability. 

---

# **When you refactor the Rust repository to Go**

Do **not** translate Rust line by line. Port the architecture.

Use this process:

## **Step 1: map the Rust repo**

Create a table:

Rust module                  Purpose                  Go package  
\---------------------------------------------------------------------------  
collection/                  collection lifecycle     internal/collection  
segment/                     vector/payload storage   internal/segment  
vector\_storage/              raw vector persistence   internal/storage  
index/hnsw/                  ANN graph index          internal/index/hnsw  
payload/                     filters/indexes          internal/payload  
wal/                         recovery log             internal/storage  
api/                         REST/gRPC                internal/api

## **Step 2: identify core traits**

Rust code often uses traits like:

trait VectorStorage {  
   fn get\_vector(\&self, id: PointOffsetType) \-\> &\[f32\];  
   fn insert\_vector(\&mut self, id: PointOffsetType, vector: Vec\<f32\>);  
}

In Go, convert them to interfaces:

type VectorStorage interface {  
   GetVector(id uint64) (\[\]float32, error)  
   InsertVector(id uint64, vector \[\]float32) error  
}

## **Step 3: port tests before implementation**

For each Rust module:

1\. Read Rust test.  
2\. Write equivalent Go test.  
3\. Implement minimal Go code to pass.  
4\. Benchmark against your mini implementation.

This prevents blind translation.

## **Step 4: port in this order**

Best order:

1\. distance functions  
2\. vector storage  
3\. ID mapping  
4\. payload filter model  
5\. flat index  
6\. segment abstraction  
7\. WAL/snapshot  
8\. HNSW  
9\. compaction  
10\. API layer  
11\. distributed pieces, if any

Do **not** start with distributed cluster logic. It will slow down learning.

---

# **Concrete first implementation target**

Your first serious milestone should be:

A local Go vector DB that can:

\- create one collection  
\- insert 100K vectors  
\- search topK with cosine similarity  
\- filter by repo/language/path  
\- persist to disk  
\- restart and recover  
\- expose HTTP API  
\- power semantic code search over one Git repository

That is enough to understand the heart of vector databases.

After that, add:

\- segment compaction  
\- HNSW  
\- recall benchmark  
\- filtered search planner  
\- snapshots  
\- mmap vector storage  
---

# **Recommended final project name**

Call it something like:

govec

or:

minivdb

Main command examples:

minivdb server \--data ./data

minivdb collection create code \--dim 768 \--metric cosine

minivdb ingest-repo \\  
 \--repo ./some-rust-vector-db \\  
 \--collection code \\  
 \--language rust

minivdb search \\  
 \--collection code \\  
 \--query "where is HNSW search implemented?" \\  
 \--filter repo=some-rust-vector-db  
---

# **The key mindset**

Build in this order:

Use vector DB \-\> build flat vector DB \-\> add database features \-\> add ANN \-\> build code search \-\> refactor Rust repo to Go.

The biggest mistake would be starting with HNSW or directly refactoring a large Rust database. First build the small version yourself. Then, when you read the Rust repo, every module will map to something you already understand.

