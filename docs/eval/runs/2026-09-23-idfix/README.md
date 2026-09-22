# Chunk-ID collision fix recheck — 2026-09-23

The committed fix `047e56e` changes Go representation versioning to
`sparse-bm25-snowball-ident-v2+go-types-v2-identity`. Fresh collections retain
all generated chunks: self **250/250** and chi **382/382**. The original chi run
had 382 generated and 371 stored points, losing 11 chunks across seven ID groups.

## Verification

- Same self and chi query labels were reused; no labels, thresholds, or expected
  files changed.
- Full and structural evaluations for both corpora completed with zero query errors.
- Chi metrics remain file hit@5 16/16, file recall@5 0.96875, and negative pass
  1/4. The known source-coverage gaps and negative failures remain evidence, not
  silently reclassified successes.
- Representation refresh deletes old file points before reindexing when hashes
  match but the representation version changes. This prevents obsolete v1 IDs from
  surviving a v2 migration.

| Corpus | Full hit@5 | Full recall@5 | Negative pass | Stored / generated |
|---|---:|---:|---:|---:|
| Self | 34/35 = 0.9714 | 0.8162 | 2/4 = 0.5000 | 250 / 250 |
| chi v5.2.3 | 16/16 = 1.0000 | 0.96875 | 1/4 = 0.2500 | 382 / 382 |

[Summary and hashes](summary.json) and the four raw reports are retained here.
The collections are local inspection artifacts; this record does not claim
large-corpus performance or answer faithfulness.
