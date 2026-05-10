# Search and vector database comparison (checked May 2026)

This compares four commonly discussed platforms for RAG, semantic search, hybrid search, and enterprise search: Qdrant, Milvus, OpenSearch, and Weaviate.

The main point: these products overlap more than they used to. Qdrant and Milvus are no longer vector-only in the strict sense because both support lexical/full-text retrieval through sparse vectors and BM25-style scoring. OpenSearch remains the strongest general-purpose full-text search and analytics engine. Weaviate remains one of the easiest options for native keyword-vector hybrid search.

## Quick overview

| Platform | Best fit | Important nuance |
| --- | --- | --- |
| Qdrant | Vector-first RAG, semantic search, low-latency retrieval, metadata filtering | Supports full-text filtering, BM25/sparse-vector text search, and dense+sparse hybrid queries. It is still not a full OpenSearch-style text analytics engine. |
| Milvus | Large-scale vector search, distributed embedding stores, high-throughput retrieval | Supports BM25-powered full-text search and dense+sparse hybrid retrieval. It is focused on vector retrieval, not broad document analytics. |
| OpenSearch | Full-text search, logs, analytics, dashboards, enterprise document search | Vector, neural, and hybrid search are supported, but OpenSearch is heavier operationally and must be tuned carefully for vector-heavy workloads. |
| Weaviate | RAG apps, developer-friendly hybrid search, knowledge bases | Supports BM25F keyword search, vector search, and hybrid fusion. It has less of the traditional search/analytics ecosystem than OpenSearch. |

## Full-text search and keyword capabilities

OpenSearch is the strongest text-search engine in this group. It supports BM25 by default, analyzers, fuzzy search, phrase queries, Query DSL, aggregations, dashboards, and log/document analytics.
                
Weaviate supports keyword search with BM25F, property weighting/boosting, selected-property search, search operators, tokenization, stopwords, and hybrid search. It is more capable than "BM25 only for hybrid ranking", but it is not a replacement for OpenSearch when you need mature full-text DSL, analytics, and dashboard workflows.

Milvus now supports BM25-powered full-text search. Current Milvus docs describe analyzers, a BM25 function, sparse-vector storage, a BM25 metric/index, raw-text query input, and relevance-ranked results. This is useful for RAG and hybrid retrieval, but Milvus should still be treated as a vector database rather than a general enterprise search engine.

Qdrant supports full-text filtering and full-text search through sparse vectors, including BM25-style ranking. It can combine semantic and lexical search through dense+sparse hybrid queries. The old statement that Qdrant has no true full-text index is no longer accurate. The more accurate limitation is that Qdrant's text-search feature set is retrieval-focused and not as broad as OpenSearch's full-text search, aggregations, and analytics stack.

## Hybrid search: vector plus keyword

OpenSearch provides strong hybrid search through hybrid queries and search pipelines that normalize/combine scores or apply rank fusion. This is a strong choice when keyword search, filters, aggregations, and analytics are primary requirements.

Weaviate has native hybrid search that runs vector search and BM25 search, then fuses the scores. Its alpha parameter makes it straightforward to bias results toward vector similarity or keyword relevance.

Milvus supports hybrid retrieval by storing dense and sparse vectors in the same collection and combining results with rankers. With BM25 full-text search, Milvus can support lexical plus semantic retrieval in a single vector-database workflow.

Qdrant supports hybrid search with dense and sparse vectors, prefetch queries, and fusion such as Reciprocal Rank Fusion (RRF). It should not be described as "filtering only" for hybrid search.

## Performance guidance

The old latency, QPS, and index-build figures should not be treated as generally correct. Public vector-database benchmarks are highly sensitive to:

- dataset size and dimensionality
- dense vs sparse vs hybrid query type
- target recall/precision
- HNSW/IVF/DiskANN/Faiss/Lucene settings
- quantization and memory layout
- filtering selectivity
- number of shards/replicas
- hardware, concurrency, transport protocol, and client/server placement

Use benchmark numbers only when the source includes exact dataset, hardware, index parameters, recall target, concurrency model, and query type. Vector-only numbers also should not be used to estimate hybrid-search latency, because hybrid search may run multiple retrieval paths plus score fusion or reranking.

Conservative performance summary:

| Platform | Practical expectation |
| --- | --- |
| Qdrant | Usually a strong low-latency vector-first choice, especially with payload filtering and dense/sparse retrieval. |
| Milvus | Strong for scale-out vector workloads, large collections, many index options, and high-throughput deployments when tuned correctly. |
| Weaviate | Competitive for RAG and hybrid search with simpler application integration; performance depends heavily on HNSW and hybrid settings. |
| OpenSearch | Best when text search and analytics dominate. Vector performance can be good, but it requires OpenSearch-specific tuning around k-NN engine choice, shard/segment behavior, warmup, and search pipelines. |

## Filtering and analytics

OpenSearch is the clear leader for aggregations, dashboards, log analytics, and traditional document-search workflows.

Qdrant is strong for metadata/payload filtering combined with vector retrieval, especially when filters are part of the normal search path.

Milvus and Weaviate both support filtering, but they are not analytics platforms in the same way OpenSearch is.

## Suitable use cases

Qdrant is suitable for chatbots, real-time RAG, semantic search, dense+sparse retrieval, and systems that need fast vector search with extensive metadata filtering.

Milvus is suitable for large embedding stores, high-throughput vector retrieval, distributed deployments, and systems that need to scale from millions to billions of vectors.

OpenSearch is suitable for enterprise search, document search, legal search, log analytics, dashboards, and use cases where full-text search and analytics are the main requirements with semantic/vector search added.

Weaviate is suitable for RAG and knowledge-base systems that need quick setup, native BM25/vector hybrid search, and developer-friendly APIs.

## Common deployment architectures

Two-system architecture: use Qdrant or Milvus as the vector store, and OpenSearch as the full-text search and analytics engine. This remains practical when you need both strong vector retrieval and strong text-search/analytics.

Single hybrid system: use OpenSearch if full-text search, aggregations, and analytics are the priority. Use Weaviate when simple native hybrid RAG is the priority. Use Qdrant or Milvus when vector-first retrieval is the priority and their BM25/sparse-vector support is enough for the keyword side.

Vector-first model: use Qdrant for low-latency vector retrieval and filtering; use Milvus for large-scale distributed vector workloads.

## Conclusion

No platform is best at everything.

OpenSearch is the strongest full-text search and analytics platform.

Qdrant and Milvus are usually the first systems to evaluate for vector-first retrieval, but they differ in operational model and scaling style.

Weaviate is a strong balanced choice for developer-friendly RAG and native hybrid search.

For large systems with demanding full-text and vector requirements, combining a vector database with OpenSearch is often the most flexible architecture.

## Sources checked

- Qdrant text search and hybrid search: https://qdrant.tech/documentation/search/text-search/
- Qdrant public vector benchmarks: https://qdrant.tech/benchmarks/
- Milvus full-text search: https://milvus.io/docs/full-text-search.md
- Milvus hybrid search tutorial: https://milvus.io/docs/hybrid_search_with_milvus.md
- OpenSearch BM25, vector, and hybrid search docs: https://docs.opensearch.org/latest/tutorials/vector-search/neural-search-tutorial/
- OpenSearch hybrid search: https://docs.opensearch.org/latest/vector-search/ai-search/hybrid-search/
- OpenSearch vector-search benchmark workload: https://docs.opensearch.org/latest/benchmark/workloads/vectorsearch/
- Weaviate BM25 keyword search: https://docs.weaviate.io/weaviate/search/bm25
- Weaviate hybrid search: https://docs.weaviate.io/weaviate/concepts/search/hybrid-search
- Weaviate inverted index configuration: https://docs.weaviate.io/weaviate/config-refs/indexing/inverted-index
