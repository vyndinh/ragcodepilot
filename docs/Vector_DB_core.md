Use this as your **knowledge map** for building your own vector database.

A vector DB is not just “store vectors and search them.” It is a combination of:

Machine learning embeddings  
\+ vector similarity search  
\+ indexing algorithms  
\+ database storage  
\+ metadata filtering  
\+ query planning  
\+ durability  
\+ performance engineering  
\+ distributed systems, later

The knowledge you need can be divided into layers.

# **1\. Embeddings and vector representation**

Before building the DB, you need to understand what the vectors represent.

## **What to know**

An embedding is a numeric representation of something:

text \-\> \[0.12, \-0.88, 0.45, ...\]  
image \-\> \[0.09, 0.31, \-0.67, ...\]  
code \-\> \[0.76, \-0.21, 0.13, ...\]

The vector database does **not** usually understand the original text, image, or code. It only receives vectors and finds similar vectors.

You should understand:

embedding model  
embedding dimension  
dense vector  
sparse vector  
multi-vector representation  
normalization  
semantic similarity

Example:

"how to recover WAL after crash?"

may produce a vector close to code chunks about:

restore log  
replay operations  
recover storage  
load snapshot

Even if the exact words are different.

## **Important concepts**

### **Dimension**

A vector has fixed length:

\[0.1, 0.2, 0.3\]     dimension \= 3  
\[... 768 numbers\]   dimension \= 768  
\[... 1536 numbers\]  dimension \= 1536

Inside one collection, vectors usually need the same dimension.

### **Dense vector**

Most modern embedding models produce dense vectors:

\[0.021, \-0.441, 0.883, 0.012, ...\]

Most values are non-zero.

### **Sparse vector**

Sparse vectors have many zero values and are often used for keyword-like retrieval:

{  
 15: 0.8,  
 203: 0.4,  
 991: 0.9  
}

You can start with **dense vectors only**.

# **2\. Vector math**

This is the first technical foundation.

A vector database needs to compare vectors. That means you need similarity or distance functions.

## **Dot product**

dot(a, b) \= a1\*b1 \+ a2\*b2 \+ ... \+ an\*bn

Used when larger score means more similar.

Example:

func Dot(a, b \[\]float32) float32 {

   var sum float32

   for i := range a {

       sum \+= a\[i\] \* b\[i\]

   }

   return sum

}

## **Cosine similarity**

Cosine measures the angle between vectors:

cosine(a, b) \= dot(a, b) / (norm(a) \* norm(b))

If vectors are normalized to length 1, then:

cosine(a, b) \== dot(a, b)

This is important because many vector DBs normalize vectors to make search faster.

## **Euclidean distance / L2 distance**

L2(a, b) \= sqrt((a1-b1)^2 \+ (a2-b2)^2 \+ ... \+ (an-bn)^2)

For ranking, you can often skip `sqrt`:

squared L2 \= (a1-b1)^2 \+ ... \+ (an-bn)^2

because square root does not change ordering.

## **What you should implement first**

Implement:

Dot

Cosine

L2 squared

Normalize

And test them carefully.

# **3\. Exact nearest-neighbor search**

Before learning advanced indexes, understand brute-force search.

Given:

query vector q

database vectors v1, v2, v3, ...

topK \= 10

The simplest search is:

for every vector:

   score \= similarity(q, vector)

   keep best topK

This is called:

exact search

brute-force search

flat search

linear scan

## **Why you need this**

Because brute-force search gives you the **ground truth**.

Later, when you build HNSW or another ANN index, you compare the approximate result against brute-force result.

## **Example**

Flat search:

 accurate but slow

HNSW search:

 fast but approximate

To measure HNSW quality:

Recall@10 \= how many of the true top 10 did HNSW find?

Example:

Flat top 10: \[1, 5, 7, 9, 11, 20, 31, 44, 50, 60\]

HNSW top 10: \[1, 5, 7, 11, 20, 31, 80, 90, 91, 92\]

Overlap \= 6

Recall@10 \= 6 / 10 \= 0.6

---

# **4\. Top-K algorithms**

A vector DB usually returns the best `K` results.

Naive approach:

calculate all scores

sort all scores

return first K

This works but is wasteful.

Better approach:

use a heap of size K

## **What to know**

You should understand:

min heap

max heap

priority queue

topK selection

partial sorting

For similarity scores where larger is better:

Use a min-heap of size K.

The smallest score in the heap is the current worst result.

Algorithm:

for each candidate:

   score \= similarity(query, candidate)

   if heap size \< K:

       push result

   else if score \> heap.min:

       pop heap.min

       push result

At the end, the heap contains the top K results.

---

# **5\. Data model of a vector DB**

You need to understand the core objects.

A simple vector DB has:

Database

 Collection

   Point

     ID

     Vector

     Payload

## **Point**

A point is one stored item:

{

 "id": "doc-123",

 "vector": \[0.12, \-0.88, 0.45\],

 "payload": {

   "repo": "qdrant",

   "language": "rust",

   "path": "src/storage/wal.rs"

 }

}

## **Collection**

A collection groups points with the same vector configuration:

{

 "name": "code",

 "dimension": 768,

 "metric": "cosine"

}

## **Payload**

Payload is metadata used for filtering and returning useful information.

Example:

{

 "language": "rust",

 "repo": "my-vector-db",

 "file": "segment.rs",

 "start\_line": 10,

 "end\_line": 80

}

You need payload because users rarely ask only:

find similar vectors

They usually ask:

find similar vectors where language \= rust

find similar vectors where repo \= qdrant

find similar vectors where date \> 2025-01-01

---

# **6\. Basic database operations**

Your vector DB should support these operations first:

CreateCollection

DeleteCollection

UpsertPoint

DeletePoint

GetPoint

Search

Scroll/List

Count

## **Upsert**

Upsert means:

insert if not exists

update if already exists

This is common in vector DBs because embeddings may be regenerated.

## **Delete**

Deletion is more complex than it looks.

In memory, delete is easy:

delete map\[id\]

On disk, delete is harder because you do not want to rewrite a huge file every time.

So databases often use:

tombstone

deleted bitmap

compaction later

Example:

Delete point 100

 \-\> mark point 100 as deleted

 \-\> search skips it

 \-\> compaction removes it later

---

# **7\. Metadata filtering**

This is one of the most important parts of a real vector database.

A query may look like:

{

 "vector": \[0.1, 0.2, 0.3\],

 "top\_k": 10,

 "filter": {

   "language": "rust",

   "repo": "my-db"

 }

}

The DB must combine:

vector similarity

\+ metadata condition

## **Filter types to understand**

Start with:

equals

not equals

in list

range

exists

prefix

and

or

not

Examples:

language \= "rust"

repo in \["qdrant", "milvus"\]

stars \> 1000

path starts with "src/index"

language \= "rust" AND repo \= "qdrant"

## **Simple implementation**

First version:

for every point:

   if payload matches filter:

       calculate vector score

This is easy and correct.

## **Optimized implementation**

Later, build payload indexes.

Example keyword index:

language \= rust \-\> {id1, id7, id9}

repo \= qdrant   \-\> {id1, id2, id7}

For filter:

language \= rust AND repo \= qdrant

You compute:

{ id1, id7, id9 } ∩ { id1, id2, id7 } \= { id1, id7 }

Then search only those candidates.

Knowledge needed:

inverted index

set intersection

bitmap

roaring bitmap

cardinality estimation

query planning

---

# **8\. Query planning**

A vector database needs to decide **how** to execute a query.

Example query:

topK \= 10

filter: language \= rust

Suppose you have 10 million points.

Case 1:

language \= rust matches 100 points

Best plan:

use payload index first

then brute-force scan 100 vectors

Case 2:

language \= rust matches 8 million points

Best plan:

use vector index first

then filter results

Case 3:

language \= rust matches 1 million points

Maybe use:

vector index with over-fetching

or filtered ANN

## **Important idea**

A vector DB query planner chooses between:

pre-filter

post-filter

hybrid search

exact search

approximate search

Simple rule:

If filter is very selective:

   use payload index first, then exact vector search

If filter is broad:

   use ANN index first, then filter

If filter is medium:

   over-fetch from ANN, then filter

---

# **9\. Indexing basics**

Brute-force search is accurate but slow for large datasets.

For 1 million vectors with 768 dimensions:

1,000,000 \* 768 operations per query

That is expensive.

So vector DBs use indexes.

There are two main categories:

exact index

approximate index

## **Exact index**

Flat index:

store all vectors

scan all vectors

return exact topK

Good for:

small datasets

ground truth

testing

high recall

Bad for:

large datasets

low latency

## **Approximate nearest neighbor index**

Approximate indexes trade some accuracy for speed.

They may not always return the true nearest neighbors, but they are much faster.

Common ANN families:

HNSW

IVF

PQ

IVF-PQ

LSH

DiskANN-style graph indexes

ScaNN-style partitioning

For your own vector DB, learn and implement:

Flat first

HNSW second

IVF third, optional

PQ later, optional

---

# **10\. HNSW knowledge**

HNSW means:

Hierarchical Navigable Small World graph

It is one of the most important vector search algorithms.

## **Mental model**

Imagine every vector as a node in a graph.

Similar vectors are connected.

Search works by walking through the graph:

start from an entry point

move to neighbors closer to the query

keep improving

return nearest nodes found

HNSW has multiple layers:

Layer 3: very few nodes, long-range navigation

Layer 2: more nodes

Layer 1: more nodes

Layer 0: all nodes, detailed search

Search starts at the top layer and moves downward.

## **Concepts to understand**

node

edge

neighbor list

entry point

layer

random level generation

M

efConstruction

efSearch

candidate queue

visited set

graph connectivity

recall

## **Important parameters**

### **M**

Maximum number of neighbors per node.

Higher `M` usually means:

better recall

more memory

slower indexing

### **efConstruction**

How much work to do during insert/index build.

Higher `efConstruction` usually means:

better graph quality

slower inserts

### **efSearch**

How much work to do during search.

Higher `efSearch` usually means:

better recall

slower query

## **What to implement**

Start with a simple HNSW:

single vector per point

no delete first

no filtering first

in-memory only

cosine or L2 only

Then add:

delete support

persistence

payload filtering

segment-level HNSW

---

# **11\. IVF knowledge**

IVF means:

Inverted File Index

The idea:

cluster vectors into groups

search only the most relevant groups

Example:

1 million vectors

divide into 1,000 clusters

query searches 10 closest clusters

This avoids scanning all vectors.

## **Concepts to know**

k-means clustering

centroids

posting lists

nlist

nprobe

coarse quantizer

Important parameters:

nlist  \= number of clusters

nprobe \= number of clusters searched per query

Higher `nprobe`:

better recall

slower search

IVF is useful to learn after HNSW.

---

# **12\. Quantization knowledge**

Quantization reduces memory.

Vectors can be large.

Example:

1 million vectors

dimension \= 768

float32 \= 4 bytes

1,000,000 \* 768 \* 4 \= 3.07 GB

That is just raw vectors, without indexes or payloads.

Quantization compresses vectors.

## **Types to know**

float32

float16

int8 quantization

scalar quantization

product quantization

binary quantization

## **Product Quantization**

PQ splits a vector into chunks and compresses each chunk.

Example:

768-dimensional vector

split into 96 chunks of 8 dimensions

encode each chunk using small codebook

PQ is harder than HNSW, so do not start with it.

Learn it when you care about:

large scale

memory reduction

disk-based search

billions of vectors

---

# **13\. Vector storage**

You need to understand how vectors are stored in memory and on disk.

## **In-memory storage**

Simple version:

map\[PointID\]\[\]float32

Better version:

\[\]\[\]float32

Even better:

flat \[\]float32

Example:

vectors \= \[

 vector0\_dim0,

 vector0\_dim1,

 vector0\_dim2,

 vector1\_dim0,

 vector1\_dim1,

 vector1\_dim2,

\]

For fixed dimension:

offset := internalID \* dim

vector := vectors\[offset : offset+dim\]

This is faster and more cache-friendly.

## **Internal IDs**

User IDs may be strings:

"doc-abc-123"

But internally, you usually want integer IDs:

0, 1, 2, 3, ...

So you need ID mapping:

external ID \-\> internal ID

internal ID \-\> external ID

Example:

externalToInternal map\[string\]uint64

internalToExternal \[\]string

## **Disk storage**

Simple version:

vectors.bin

payloads.jsonl

ids.json

deleted.bin

For fixed dimension float32 vectors:

vector size in bytes \= dim \* 4

offset \= internalID \* dim \* 4

This makes random access possible.

---

# **14\. Payload storage**

Payload can be stored separately from vectors.

Example:

vectors.bin       raw float32 vectors

payloads.jsonl    metadata

ids.bin           ID mapping

deleted.bin       tombstones

Payload can be JSON at first.

Later, you may want:

binary encoding

columnar payload storage

dictionary encoding

payload indexes

compression

Payload is often bigger and more irregular than vectors.

Example payload:

{

 "repo": "my-db",

 "language": "go",

 "path": "internal/index/hnsw/search.go",

 "symbol": "Search",

 "start\_line": 20,

 "end\_line": 90,

 "text": "func Search(...) ..."

}

The `text` field can be large, so you may eventually store it outside the hot query path.

---

# **15\. Segments**

Segments are very important.

Instead of one giant storage file, split data into pieces:

collection/

 segment-0001/

 segment-0002/

 segment-0003/

Each segment contains:

vectors

payloads

ID mapping

deleted bitmap

vector index

payload indexes

## **Why segments matter**

Segments make it easier to handle:

writes

deletes

compaction

index building

snapshots

parallel search

memory management

## **Mutable and sealed segments**

A common design:

Mutable segment:

 accepts new writes

Sealed segment:

 read-only or mostly read-only

 optimized for search

 can have HNSW index

Write flow:

new point \-\> mutable segment

mutable segment gets large \-\> flush to disk

build index \-\> sealed segment

start new mutable segment

Search flow:

search all relevant segments

merge topK results

return global topK

This is one of the biggest architecture ideas in vector DB design.

---

# **16\. Write-ahead log, WAL**

A database needs durability.

If the process crashes after a write, the data should not disappear.

A WAL records operations before applying them.

Example WAL:

{"op":"upsert","id":"1","vector":\[...\],"payload":{...}}

{"op":"delete","id":"1"}

Startup recovery:

load existing segments

read WAL

replay operations

restore latest state

## **Knowledge needed**

append-only file

fsync

crash recovery

log replay

checkpoint

snapshot

log truncation

idempotency

## **Important rule**

The WAL should be written before acknowledging success to the user.

Simplified flow:

1\. receive upsert

2\. append operation to WAL

3\. flush WAL

4\. apply to memory

5\. return success

Later you can batch WAL flushes for performance.

---

# **17\. Snapshots and manifests**

A manifest describes what files belong to the database.

Example:

{

 "collection": "code",

 "dimension": 768,

 "metric": "cosine",

 "segments": \[

   "segment-0001",

   "segment-0002"

 \],

 "active\_segment": "segment-0003"

}

A snapshot is a stable copy of the database at a point in time.

You need to understand:

atomic file rename

manifest versioning

snapshot isolation

backup

restore

Important pattern:

write new manifest to temporary file

fsync

rename temp file to manifest.json

This avoids corrupting the manifest during crashes.

---

# **18\. Deletes and compaction**

Deletes should usually be lazy.

Instead of immediately removing the vector from disk:

mark as deleted

skip during search

remove later during compaction

This is called a tombstone.

## **Compaction**

Compaction rewrites data to remove deleted points and merge small segments.

Example:

Before:

 segment-1: 10,000 points, 3,000 deleted

 segment-2: 4,000 points, 2,000 deleted

 segment-3: 1,000 points, 0 deleted

After:

 segment-4: 10,000 live points

Compaction process:

1\. choose old segments

2\. read live points

3\. write new segment

4\. build new index

5\. atomically update manifest

6\. remove old segments

Knowledge needed:

tombstones

garbage collection

merge policy

write amplification

read amplification

space amplification

atomic replacement

---

# **19\. Memory mapping and file I/O**

For large vector DBs, vectors may not all fit in RAM.

You need to understand:

file I/O

buffered I/O

random access

sequential access

mmap

page cache

disk latency

SSD behavior

## **mmap**

Memory-mapped files let the OS load file pages into memory on demand.

Useful for:

large vector files

read-heavy workloads

fast startup

sharing memory between processes

But mmap also introduces complexity:

page faults

OS-dependent behavior

harder error handling

careful file layout needed

For your first version, normal file I/O is enough.

Later, use mmap for sealed vector segments.

---

# **20\. Concurrency**

A vector DB handles many reads and writes.

You need to understand:

goroutines

mutex

RWMutex

atomic values

channels

context cancellation

background workers

## **Read/write design**

Search should happen while writes continue.

Simple version:

one global mutex

Better version:

collection-level lock

segment-level lock

copy-on-write manifest

immutable sealed segments

A good design:

sealed segments are immutable

mutable segment has lock

manifest is read atomically

search gets current segment list

search runs without blocking most writes

## **Background workers**

You may need workers for:

index building

segment flushing

compaction

snapshot creation

embedding generation

Be careful with:

race conditions

partial writes

cancellation

deadlocks

---

# **21\. API design**

Your vector DB needs a usable interface.

Start with HTTP or CLI.

## **Basic HTTP API**

POST /collections

DELETE /collections/{name}

PUT /collections/{name}/points

DELETE /collections/{name}/points/{id}

GET /collections/{name}/points/{id}

POST /collections/{name}/search

POST /collections/{name}/scroll

## **Search request**

{

 "vector": \[0.1, 0.2, 0.3\],

 "top\_k": 10,

 "filter": {

   "language": "rust"

 },

 "include\_payload": true,

 "include\_vector": false

}

## **Search response**

{

 "results": \[

   {

     "id": "point-1",

     "score": 0.87,

     "payload": {

       "language": "rust",

       "path": "src/index/hnsw.rs"

     }

   }

 \]

}

Important API concepts:

pagination

timeouts

request validation

dimension validation

error handling

batch upserts

batch deletes

---

# **22\. Benchmarking**

You cannot understand a vector DB without benchmarks.

Measure:

search latency

index build time

insert throughput

memory usage

disk usage

recall

QPS

p50 latency

p95 latency

p99 latency

## **Recall benchmark**

Use flat search as ground truth.

recall@K \= overlap(approx topK, exact topK) / K

Example:

exact top10 \= \[1,2,3,4,5,6,7,8,9,10\]

hnsw top10  \= \[1,2,3,4,5,20,30,40,50,60\]

recall@10 \= 5 / 10 \= 0.5

## **Latency benchmark**

Measure:

average latency

p50

p95

p99

The average can hide bad tail latency.

Example:

p50 \= 5 ms

p95 \= 40 ms

p99 \= 200 ms

This means most queries are fast, but some are very slow.

## **Dataset sizes to test**

Start with:

1,000 vectors

10,000 vectors

100,000 vectors

1,000,000 vectors

Use generated random vectors first.

Then use real embeddings later.

---

# **23\. Evaluation of search quality**

Vector DB performance is not only speed.

You also need quality.

Important metrics:

Recall@K

Precision@K

MRR

NDCG

latency vs recall

memory vs recall

For database internals, `Recall@K` is most important.

For application search, you may care about:

Did the user find the right code?

Did the correct document appear in top 5?

Did the answer generator use the right context?

---

# **24\. Hybrid search**

Many real systems combine:

vector search

\+ keyword search

\+ metadata filters

Example:

query: "WAL recovery"

Keyword search may find exact terms:

WAL

recovery

replay

Vector search may find semantically related terms:

restore log

crash restore

operation replay

Hybrid search combines both.

Knowledge needed:

BM25

inverted index

sparse vectors

score normalization

rank fusion

reciprocal rank fusion

reranking

You do not need hybrid search in v1, but it is very useful for code search.

---

# **25\. Reranking**

The first vector search result may not be the best final answer.

A common pipeline:

1\. retrieve top 100 with vector DB

2\. rerank top 100 with stronger model

3\. return top 10

This matters for applications like:

RAG

code search

question answering

recommendation

For building the DB itself, reranking is not required.

For building an application on top of the DB, it is important.

---

# **26\. RAG knowledge**

A vector DB is commonly used for RAG:

Retrieval-Augmented Generation

Basic RAG flow:

user question

\-\> embed question

\-\> search vector DB

\-\> retrieve relevant chunks

\-\> send chunks to LLM

\-\> generate answer

To understand vector DB applications, learn:

chunking

embedding

retrieval

reranking

context window

grounding

citations

hallucination reduction

For your Rust-to-Go refactor goal, RAG can help you ask questions about the Rust repository.

Example:

"Which files implement segment compaction?"

"Where is HNSW graph persistence handled?"

"Which Rust trait corresponds to vector storage?"

---

# **27\. Code chunking**

Because your project involves understanding a Rust repository, code chunking is important.

Bad chunking:

split every 1000 characters

Better chunking:

split by function

split by struct

split by impl block

split by module

keep file path and line numbers

Payload should include:

{

 "repo": "source-db",

 "language": "rust",

 "path": "src/index/hnsw.rs",

 "symbol": "search\_layer",

 "chunk\_type": "function",

 "start\_line": 120,

 "end\_line": 190

}

Knowledge needed:

AST parsing

tree-sitter

function-level chunking

symbol extraction

line ranges

dependency graph

---

# **28\. Internal ID mapping**

This is small but very important.

User gives IDs like:

"src/index/hnsw.rs:Search:120"

But the index wants small integer IDs:

0

1

2

3

So you need:

external ID \-\> internal ID

internal ID \-\> external ID

Why?

Because arrays are faster than maps.

Instead of:

map\[string\]\[\]float32

You want:

vectors \[\]float32

And access by offset:

offset := internalID \* dimension

This requires stable internal IDs.

---

# **29\. Schema and validation**

A collection needs configuration.

Example:

{

 "name": "code",

 "dimension": 768,

 "metric": "cosine",

 "payload\_schema": {

   "language": "keyword",

   "repo": "keyword",

   "start\_line": "integer",

   "created\_at": "datetime"

 }

}

Validation rules:

vector dimension must match collection dimension

metric must be supported

point ID must be valid

payload field type should match schema

topK must be reasonable

filter field must exist or be allowed

Without validation, bugs become hard to debug.

---

# **30\. Error handling**

A database must produce clear errors.

Examples:

collection not found

dimension mismatch

invalid filter

point not found

WAL write failed

segment corrupted

index not loaded

In Go, create typed errors:

var ErrCollectionNotFound \= errors.New("collection not found")

var ErrDimensionMismatch \= errors.New("dimension mismatch")

Or richer errors:

type DimensionMismatchError struct {

   Expected int

   Got      int

}

---

# **31\. Crash safety**

This is advanced but essential for a real DB.

You need to think:

What happens if process crashes during write?

What happens if crash happens while writing segment?

What happens if manifest is half-written?

What happens if WAL is partially written?

What happens if compaction crashes midway?

Basic rules:

append WAL before applying write

write new files before deleting old files

use temp files

use atomic rename

keep manifest versioned

make recovery idempotent

Example safe segment write:

1\. write segment-0004.tmp/

2\. fsync files

3\. rename segment-0004.tmp to segment-0004

4\. write manifest.tmp

5\. rename manifest.tmp to manifest.json

6\. old segments can now be deleted

---

# **32\. Index persistence**

When you build HNSW, you need to save and load the graph.

HNSW data to persist:

nodes

levels

neighbors per layer

entry point

M

efConstruction

distance metric

internal ID mapping

deleted flags

You need to decide file format:

JSON for debugging

binary for performance

Start with JSON or gob.

Later design binary files:

magic header

version

dimension

metric

node count

offset table

neighbor lists

checksum

Important knowledge:

binary encoding

endianness

versioning

checksums

backward compatibility

---

# **33\. File formats**

At first, use simple formats:

manifest.json

payloads.jsonl

wal.jsonl

vectors.bin

Later, learn better binary format design.

A binary file may have:

magic bytes

version

metadata header

record count

fixed-width records

variable-width section

checksum

Example:

VECDB01

dimension: 768

count: 100000

metric: cosine

vectors...

checksum...

Knowledge needed:

serialization

binary layout

alignment

checksums

compression

forward compatibility

backward compatibility

---

# **34\. Observability**

You need visibility into the DB.

Add metrics:

number of collections

number of points

number of segments

deleted point count

WAL size

search latency

upsert latency

compaction duration

index build duration

memory usage

Add logs:

collection created

segment flushed

WAL replay started

WAL replay completed

compaction started

compaction completed

index build failed

Add debug APIs:

GET /debug/collections

GET /debug/segments

GET /debug/index

This will help you understand your own system.

---

# **35\. Testing knowledge**

You need different test types.

## **Unit tests**

For small functions:

cosine similarity

L2 distance

filter matching

heap topK

ID mapping

## **Integration tests**

For full flows:

create collection

insert points

search

delete point

restart DB

search again

## **Crash tests**

Simulate failure:

crash after WAL append

crash during segment write

crash during manifest update

crash during compaction

## **Property tests**

Useful for search correctness:

flat search should always return sorted results

deleted points should never appear

dimension mismatch should always fail

## **Benchmark tests**

In Go:

go test \-bench=.

Benchmark:

distance function

flat search

HNSW search

payload filter

WAL write

segment load

---

# **36\. Performance engineering**

Vector DB performance depends on low-level details.

You should learn:

CPU cache

memory layout

SIMD

batching

allocation reduction

object pooling

parallel search

lock contention

I/O patterns

## **Memory layout matters**

Slower:

\[\]\[\]float32

because each vector may be in a different memory location.

Faster:

\[\]float32

with fixed offsets.

Example:

offset := id \* dim

vec := vectors\[offset : offset+dim\]

## **Avoid allocations in hot path**

Search is hot path.

Avoid creating many temporary slices/maps during search.

Bad:

for every vector:

   create new result object

   create new temporary slice

Better:

reuse buffers

preallocate heap

use internal integer IDs

---

# **37\. SIMD and acceleration**

For serious performance, vector distance computation needs acceleration.

Concepts:

SIMD

AVX2

AVX-512

NEON

GPU acceleration

BLAS

batch matrix multiplication

In Go, pure Go may be slower than C++/Rust for SIMD-heavy workloads.

You can still build a good learning DB in Go.

Later options:

Go assembly

cgo to BLAS/Faiss

pure Go optimized loops

batch search

use float32 carefully

For learning, do not start with SIMD.

First make the design correct.

---

# **38\. Distributed systems, later**

Do not start here, but eventually learn it.

Large vector DBs may support:

sharding

replication

leader/follower

consensus

distributed query

distributed indexing

node failure recovery

rebalancing

## **Sharding**

Split data across nodes:

shard 1: points 0-999999

shard 2: points 1000000-1999999

shard 3: points 2000000-2999999

Search flow:

send query to all shards

each shard returns topK

coordinator merges global topK

## **Replication**

Keep multiple copies:

shard 1 replica A

shard 1 replica B

shard 1 replica C

Useful for:

high availability

read scaling

fault tolerance

Knowledge needed:

consistent hashing

Raft

leader election

replication log

quorum

eventual consistency

read consistency

write consistency

This is advanced. Build a single-node vector DB first.

---

# **39\. Security and multi-tenancy**

For a production vector DB, you need:

authentication

authorization

tenant isolation

API keys

rate limiting

quotas

encryption at rest

TLS

audit logs

For your learning project, skip this at first.

But understand that real vector DBs often need:

collection-level permissions

tenant-level storage isolation

resource limits

---

# **40\. Backup and restore**

A database must be recoverable.

Knowledge needed:

snapshot

incremental backup

restore

manifest

WAL replay

object storage

checksums

version compatibility

Simple backup:

pause writes briefly

copy data directory

resume writes

Better backup:

create consistent snapshot

copy snapshot files

continue writes

---

# **41\. Core Go knowledge you need**

Since you plan to build or refactor into Go, focus on these Go topics:

slices and arrays

maps

interfaces

struct layout

error handling

goroutines

sync.Mutex

sync.RWMutex

atomic.Value

context.Context

encoding/json

binary encoding

file I/O

testing

benchmarking

pprof

## **Very important Go packages**

container/heap

encoding/binary

encoding/json

errors

fmt

io

os

path/filepath

sort

sync

sync/atomic

context

net/http

testing

runtime/pprof

Later:

golang.org/x/sync/errgroup

github.com/RoaringBitmap/roaring

---

# **42\. Rust knowledge useful for refactoring to Go**

When reading a Rust vector DB repository, understand:

trait

struct

enum

impl

generic types

lifetimes

ownership

borrowing

Arc

Mutex

RwLock

Result

Option

async/await

modules

cargo workspace

Mapping Rust to Go:

Rust trait      \-\> Go interface

Rust struct     \-\> Go struct

Rust enum       \-\> Go tagged struct or interface

Rust Result\<T\>  \-\> (T, error)

Rust Option\<T\>  \-\> pointer, zero value, or bool pair

Rust Arc\<T\>     \-\> shared pointer/reference

Rust RwLock     \-\> sync.RWMutex

Rust Vec\<T\>     \-\> \[\]T

Rust HashMap    \-\> map\[K\]V

Do not translate Rust line by line.

Translate architecture:

module responsibility

data flow

interfaces

storage layout

algorithms

tests

---

# **43\. Minimum knowledge for your first version**

To build your first working vector DB, you only need this:

1\. float32 vectors

2\. cosine similarity

3\. brute-force topK search

4\. collection and point model

5\. payload metadata

6\. simple filters

7\. JSON API

8\. in-memory map storage

9\. WAL persistence

10\. basic tests and benchmarks

This is enough for:

insert points

search similar vectors

filter by metadata

restart without losing data

Do not start with HNSW.

Do not start with distributed architecture.

Do not start with GPU search.

---

# **44\. Knowledge for version 2**

After v1 works, learn:

segments

deleted bitmap

compaction

payload indexes

internal integer IDs

flat binary vector storage

parallel search

HNSW

recall benchmarks

At this stage, your DB becomes much more realistic.

---

# **45\. Knowledge for version 3**

After v2 works, learn:

filtered HNSW

mmap

snapshot/restore

index persistence

hybrid search

reranking

quantization

better file formats

pprof optimization

---

# **46\. Knowledge for production-level vector DB**

This is advanced:

distributed sharding

replication

Raft or consensus

multi-tenancy

resource isolation

security

backup/restore

rolling upgrades

schema migration

observability

cloud deployment

Only study this after your single-node DB is solid.

---

# **Recommended learning order**

Follow this order:

1\. Vector math

2\. Flat search

3\. TopK heap

4\. Collection/point/payload model

5\. Metadata filtering

6\. In-memory DB

7\. WAL and recovery

8\. Segment architecture

9\. Tombstones and compaction

10\. Payload indexes

11\. HNSW

12\. Recall benchmarking

13\. Filtered vector search

14\. Binary storage and mmap

15\. Code-search application

16\. Rust-to-Go refactor

17\. Distributed vector DB design

---

# **Practical exercises**

## **Exercise 1: vector math**

Implement:

dot

cosine

l2

normalize

Test with small vectors.

---

## **Exercise 2: flat vector DB**

Build:

CreateCollection

Upsert

Delete

Search

Use:

map\[string\]Point

Search by brute force.

---

## **Exercise 3: payload filters**

Support:

field equals value

field in list

number range

AND

OR

---

## **Exercise 4: WAL**

Write every operation to:

wal.jsonl

Then restart the DB and replay the WAL.

---

## **Exercise 5: segments**

Split storage into:

mutable segment

sealed segments

Search all segments and merge results.

---

## **Exercise 6: HNSW**

Implement in memory first.

No persistence.

No delete.

No filtering.

Only after it works, add the missing parts.

---

## **Exercise 7: benchmark**

Compare:

flat search vs HNSW

Measure:

latency

recall@10

memory usage

index build time

---

## **Exercise 8: code search**

Use your vector DB to index code chunks.

Payload:

{

 "repo": "my-rust-db",

 "language": "rust",

 "path": "src/segment/mod.rs",

 "symbol": "Segment",

 "start\_line": 10,

 "end\_line": 80

}

Then search:

"where is HNSW graph search implemented?"

"where does WAL recovery happen?"

"which module handles payload filtering?"

---

# **Final checklist**

To say you understand vector DB internals, you should be able to explain and implement:

What is an embedding?

What is vector similarity?

Why normalize vectors?

How does brute-force topK search work?

Why do we need ANN?

How does HNSW search work?

What are M, efConstruction, and efSearch?

How do payload filters work?

What is an inverted index?

How do you combine filters with vector search?

What is a segment?

Why use tombstones?

What is compaction?

What is WAL?

How does crash recovery work?

How are vectors stored on disk?

Why use internal integer IDs?

How do you measure recall?

How do you benchmark latency?

How do you persist an ANN index?

How do you merge results from multiple segments?

How would you shard vector search across nodes?

The shortest path is:

Build exact search first.

Add database durability second.

Add segments third.

Add HNSW fourth.

Add filtering and query planning fifth.

Then build the code-search application on top.

That path will give you a deep understanding before you refactor a Rust vector DB repository into Go.

