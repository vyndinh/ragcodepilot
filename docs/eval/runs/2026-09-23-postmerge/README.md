# Post-merge pinned-corpus recheck — 2026-09-23

Ran against merged main `b3483b0` with representation
`sparse-bm25-snowball-ident-v2+go-types-v2-identity`, fresh Qdrant collections,
and the unchanged self/chi labels.

## Point survival

| Corpus | Generated | Stored | Unique IDs | Lost/duplicate |
|---|---:|---:|---:|---:|
| Self | 250 | 250 | 250 | 0 |
| chi v5.2.3 | 382 | 382 | 382 | 0 |

## Retrieval results

| Corpus | hit@5 | file recall@5 | file recall@10 | negative pass | query errors |
|---|---:|---:|---:|---:|---:|
| Self full | 34/35 = 0.9714 | 0.8162 | 0.8886 | 2/4 = 0.5000 | 0 |
| Self structural | 15/16 = 0.9375 | 0.6917 | 0.8188 | N/A | 0 |
| chi full | 16/16 = 1.0000 | 0.9688 | 0.9688 | 1/4 = 0.2500 | 0 |
| chi structural | 4/4 = 1.0000 | 0.8750 | 0.8750 | N/A | 0 |

## Coverage and negative audit

The self full run has `11` positive cases with incomplete expected-file coverage at top five; the exact cases are in [coverage.json](coverage.json). The chi full run has the frozen file-coverage result and a source-level audit: the post-merge point set contains all predeclared source ranges for `11`/16 positives at top five. The [coverage audit](coverage.json) lists missing files
and exact source lines at top five, top ten, and anywhere in the stored index.

Negative failures are unchanged and recorded in [negative-failures.json](negative-failures.json).
The self run retains its two known RRF failures; chi retains its three known
absent-feature negative failures. No threshold or label was changed to improve a
score.

[Raw full and structural reports](.) plus [summary.json](summary.json) preserve
all aggregates and point-survival checks. This closes the roadmap requirement to
re-evaluate both pinned corpora after the merged migration fix. It does not claim
external generalization, answer faithfulness, or large-corpus capacity.
