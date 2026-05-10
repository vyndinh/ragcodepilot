Basic configuration for enabling your enterprise to use AI for knowledge extraction

A complete RAG system, or Retrieval-Augmented Generation system, is not a single application. It is a combination of multiple specialized components, each responsible for a distinct role in the overall knowledge-processing lifecycle. Below is the minimum architecture that AgentBook recommends for an enterprise to deploy an AI system for extracting knowledge from internal documents in a stable and effective way.

—

**RAG Server: The brain that orchestrates the entire system**

The RAG Server is the central component responsible for orchestrating the entire processing flow, from the moment a user asks a question to the moment they receive an answer. When a query is submitted, the RAG Server performs the steps sequentially: it calls the Embedding Server to vectorize the question, queries Qdrant to find the most relevant document passages, aggregates the context, and then calls the LLM Server to generate the final answer.

In addition to handling real-time questions, the RAG Server also manages the document ingestion pipeline: receiving files from users, splitting them into chunks, calling the Embedding Server to create vectors, and storing both vectors and metadata in the appropriate storage locations.

The RAG Server is typically allocated more RAM than the Embedding Server or Qdrant because it needs to hold multiple contexts at the same time, especially when processing many chat sessions or multiple documents concurrently. The LLM Server usually requires the most resources overall due to GPU and VRAM demands.

—

**Qdrant Server: The semantic memory of the system**

Qdrant is a specialized vector database that acts as the “semantic memory” of the entire system. Instead of performing traditional keyword-based search, Qdrant stores the vector embeddings of each document passage and enables search based on semantic similarity.

When a user asks, “What is the product warranty policy?”, Qdrant does not simply search for the exact word “warranty.” Instead, it compares the vector representation of the question against the stored vectors and returns the most semantically similar text passages, even if those passages use completely different wording. The meaning is captured by the embedding model; Qdrant's role is to find the nearest vectors efficiently. This is the core factor that differentiates RAG from conventional search tools.

Qdrant also supports metadata-based filtering, such as by department, document type, or time period. This helps enterprises precisely control the scope of knowledge that each user is allowed to access.

—

**MongoDB Server \+ MinIO: The comprehensive data storage layer**

This pair is responsible for storing all non-vector data in the system, including two very different types of data.

MongoDB stores structured document-style data: user information, conversation history, document metadata such as file name, uploader, creation date, and access permissions, as well as workspace and notebook configurations. Whenever a user returns to view chat history or the system needs to check ACL permissions before answering, MongoDB is queried first.

MinIO acts as S3-compatible object storage, specialized in storing raw files: PDFs, Word documents, Excel files, images, and any other types of documents uploaded by the enterprise. MinIO ensures that original files are always stored safely and can be retrieved at any time, independently of the vector processing pipeline.

These two components work in parallel and complement each other: MinIO stores the original files, MongoDB stores metadata and state, and Qdrant stores the semantic representation of the content.

—

**Embedding Server: The bridge between language and mathematics**

The Embedding Server runs an embedding model, typically BGE-M3 or an equivalent model, with a single but extremely important task: converting text into numerical vectors so that computers can calculate similarity between text passages.

This server is called at two different points in the workflow. The first is during document ingestion: each text chunk is converted into a vector and stored in Qdrant. The second is when a user asks a question: the question is also vectorized using the same model to ensure consistency in the vector space during comparison.

Separating the Embedding Server into its own service makes it easier to upgrade the embedding model without affecting other components. It also allows the service to scale independently as document volume grows.

—

**LLM Server: The heart of language generation**

The LLM Server is the primary component that requires a GPU, which is why on-premise RAG requires specialized hardware investment. This server runs a large language model, such as Qwen2.5-32B, DeepSeek-R1-32B, or an equivalent model quantized to Q4, and is responsible for generating the final answer. The Embedding Server can also benefit from GPU acceleration at high ingestion throughput, though CPU is sufficient for small to medium scale.

The important point to understand is that the LLM Server does not inherently “know” the enterprise’s documents. It only receives a prompt prepared by the RAG Server, including the user’s question and the relevant document passages retrieved from Qdrant, and then synthesizes them into a coherent answer in natural language.

With an RTX 4090 and 24 GB of VRAM, the system can run models ranging from 14B to 32B parameters in Q4 mode. This is sufficient for strong Vietnamese-language handling and suitable for most needs of small and medium-sized enterprises without relying on external APIs.

—

**Architecture overview**

The five components above work together as a tightly connected pipeline. Documents enter through the RAG Server, are vectorized by the Embedding Server, have their original files stored in MinIO, metadata stored in MongoDB, and vectors stored in Qdrant. Questions also go through the RAG Server, are processed semantically using the Embedding Server and Qdrant, and are then synthesized by the LLM Server into answers based on the enterprise’s real knowledge.

This entire system can run fully on-premise, with no data leaving the internal infrastructure. This meets the strict security requirements of financial, healthcare, and legal organizations. 

