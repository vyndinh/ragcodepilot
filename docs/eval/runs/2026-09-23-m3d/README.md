# M3-D measurement probe — 2026-09-23

This controlled local probe validates the new `Index metrics:` line and dense-cache
accounting with Ollama `nomic-embed-text:latest` (digest recorded in
`metrics.json`). It is a smoke measurement, not a corpus-quality or capacity gate.

The clean run indexed one Go file and generated two chunks. The warm run added one
file, regenerated four chunks, reused the two unchanged dense vectors, and embedded
two new inputs:

| Run | Chunks | Dense calls / inputs | Cache hits / misses | Total |
|---|---:|---:|---:|---:|
| Clean | 2 | 1 / 2 | 0 / 2 | 1544 ms |
| Warm after one-file addition | 4 | 1 / 2 | 2 / 2 | 157 ms |

Both runs completed with source verification. Sparse vectors and complete-point
upserts still ran for the full current chunk scope. The warm result shows dense
work reduction; it does not imply sparse work or total indexing is proportional to
the diff.

[Raw metric values](metrics.json) are retained for later comparisons. The next
M3-D gate is to capture the same fields for pinned self and chi corpora and compare
retry output with a clean rebuild.
