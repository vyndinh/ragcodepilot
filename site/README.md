# Architecture explainer

This is a static teaching site with no runtime dependencies. Development checks use Playwright (Node.js 20+) and Python 3. It never queries Ollama or
Qdrant. Serve this directory from the repository root:

```sh
python3 -m http.server 8000 --bind 127.0.0.1 --directory site
```

Open `http://localhost:8000`. Fonts are included locally; external source links
are only followed when a reader opens them.

## Content and evidence

- `index.html`: deployment boundaries, the ChunkFile walkthrough, deep dives,
  recorded evaluation metrics, and terminal examples.
- `app.js`: pipeline inspectors, five simulator fixtures, worked RRF
  calculations, and interaction state. All simulator scores, vectors, and
  answers are illustrative. They are not replayed evaluation results.
- `styles.css`: both themes and responsive layouts. Wide source snippets and
  tables scroll within their own containers.
- Implementation source links use the repository and reviewed revision from
  `tools/site/evidence.json`. Update the revision and review date only after
  rechecking snippets, symbols, and lines; regeneration does not perform that review.
- `evidence.js` and `data/*.json` are generated from the saved reports, with
  source SHA-256 hashes. They supply all scoreboard figures, all query outcomes,
  the evaluation terminal summary, and the separate generation timings. Report
  download links refer to these exact local snapshots, independently of the
  implementation revision. Do not hand-edit generated files.
- Headline retrieval metrics come from the September 23 post-merge bundle:
  `docs/eval/runs/2026-09-23-postmerge/self-full.json` (39 queries, 35 positives).
  Its `chi-full.json` and `summary.json` supply external-repository results, point
  survival, and named coverage/negative failures. These describe source `b3483b0`,
  not the newer implementation-review revision. Raw report timestamps are UTC;
  the bundle's September 23 date is the local run-record date.
- `docs/eval/runs/2026-09-23-m3d/metrics.json` supplies a separate indexing smoke
  probe. Different clean/warm corpus sizes and single observations do not establish
  a controlled speedup, capacity, or full M3 acceptance.
- The separate, historical generation figures still come from
  `docs/eval/baseline_v7_structural_answer_al5.json` (16 structural queries).
  They describe different workloads and must not be combined into one timing.
- The status cards distinguish **implemented mechanisms**, **saved measurements**,
  and **still-unverified acceptance gates**. Content review on 2026-10-04 did not
  run a new backend benchmark. Keep source-review dates and report dates separate.
- RRF uses this project's k=60 and Qdrant's zero-based positions. Missing
  candidates contribute zero. The worked table calculates its illustrative
  scores in the browser.

## Verification after changes

From the repository root, regenerate whenever a source report, `go.mod`, or
`tools/site/evidence.json` changes:

```sh
python3 tools/site/build_evidence.py
python3 tools/site/build_evidence.py --check
```

Install and run the checks (the test runner starts and stops its own server on
127.0.0.1:8770; Qdrant and Ollama are not required):

```sh
cd tools/site
npm ci
npx playwright install chromium
npm test
```

The pinned dependency and lockfile make browser checks reproducible. `npm test`
runs the stale-artifact gate, JavaScript syntax check, an isolated drift test,
and nine Chromium browser tests. Playwright traces are retained on failures
under the ignored `tools/site/test-results/` directory.

`Website checks` runs on relevant pull requests. Pages deployment depends on
the same checks, including source-report changes. CI validates committed
artifacts instead of silently rebuilding stale evidence. Commit regenerated
files together with changes to their source reports.

The implementation revision is the version reviewed for the explanations, not
an assertion that an evaluation was run at that commit. Each report displays
its own recorded run date. Setup prerequisites derive the Go version from
`go.mod`; the setup commands are checked for copying, not executed by browser tests.

Browser checks:

1. Run each of the five presets in hybrid, dense, and sparse modes with answer
   mode on and off. Unused stages should say they are skipped.
2. Enable an answer, disable it, change the query, and enable it again. The new
   answer and references must match the new query.
3. Submit an unsupported or empty query, then toggle answer mode. No previous
   answer or citations should appear.
4. Follow citations to the matching result or pinned implementation. Verify
   source files and line ranges against the repository.
5. Switch the walkthrough between success and dimension mismatch. The failure
   must stop before retrieval and generation; the successful output is hidden.
6. Compare the RRF example's three modes, including the absent sparse candidate.
7. Inspect widths 320, 390, 768, 1024, and 1440px in both themes. The page must
   not overflow horizontally. Intentional table/code scrollers must be usable.
8. Check keyboard focus, Enter/Space on pipeline nodes, mobile navigation,
   reduced-motion startup, terminal tabs, and clipboard success/failure.
9. Inspect browser errors and network requests. Loading the site should only
   request its HTML, JavaScript, CSS, and local fonts.

No backend or model execution is needed to validate this site. Browser checks
do not establish fresh retrieval or answer-quality benchmark results.

## Verification recorded 2026-09-17

- JavaScript syntax and scoped whitespace checks passed.
- All 30 preset × retrieval mode × answer-toggle combinations passed; the
  stale-answer and unsupported-query toggle sequences were also checked.
- Both themes × five widths × four deep-dive panels (40 combinations) had no
  page-level horizontal overflow. Mobile simulator and dark-mode RRF layouts
  were visually inspected.
- Citation navigation/focus, keyboard pipeline selection, step controls,
  mobile navigation, reduced motion, terminal tabs, and both walkthrough
  scenarios passed. All static anchor targets and source paths resolved.
- Clipboard-unavailable fallback passed on the preview origin; a real
  successful clipboard write was not verified there.
- Browser console reported no errors. No live backend benchmark was run.

## Automated verification recorded 2026-09-17

The local Chromium suite passed all eight tests, covering stale answers, all
30 simulator combinations, citations, 40 responsive/theme/deep-dive
combinations, keyboard/mobile controls, the walkthrough, RRF, evidence display,
and local-only loading. Successful clipboard writes and the denied-clipboard
fallback both passed on localhost. The isolated evidence test proved that
changing a source report or corrupting a copied report fails `--check`.

For that original verification, hosted CI and deployment had not been run. The setup instructions were checked
against the repository's commands and defaults; installing services, pulling
models, and performing fresh-machine indexing remain outside these site tests.

## Content refresh — 2026-10-04

- Reviewed implementation explanations and source locations against `c54b7a5`:
  dense-cache reuse, writer ownership/run markers, source verification, Go
  declaration identity, Git ignore precedence, and error remedies.
- Updated pipeline inspectors, incremental deep dive, illustrative simulator and
  terminal output. Examples remain teaching fixtures, not a live service.
- The generator copies five source reports, hashes each, and rejects disagreement
  between retrieval aggregates and the verification summary. The isolated drift
  test exercises every report and generated snapshot.
- Browser checks additionally verify status boundaries, external failures,
  downloadable report equality, and revised dense-cache explanations.
- Frozen reports under `docs/eval/` are unchanged. The pending corpus-scale cache
  and retry-equivalence gates remain open; no new answer-quality claim is made.

The previously published `c54b7a5` revision passed hosted CI and Pages deployment.
This refresh still requires its own commit, hosted checks, and publication.

Local validation for this refresh: all nine Chromium tests passed, including both
themes at 320/390/768/1024/1440px, the evidence/status checks, and all simulator
mode combinations. Evidence generation/check, drift and summary-consistency
tests, JavaScript syntax, and whitespace checks passed. All 47 source references
were checked against the pinned implementation; desktop and mobile status-card
screenshots were inspected. No live indexing or answer benchmark was run.
