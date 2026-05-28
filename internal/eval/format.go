package eval

import (
	"fmt"
	"sort"
	"strings"
)

// FormatHuman renders a Report as a human-readable summary suitable for
// terminal output. Per-query failures are listed at the end.
func FormatHuman(r *Report) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Dataset:    %s\n", r.Dataset)
	fmt.Fprintf(&b, "Collection: %s\n", r.Collection)
	fmt.Fprintf(&b, "Embedder:   %s\n", r.Embedder)
	fmt.Fprintf(&b, "Mode:       %s\n", r.Mode)
	fmt.Fprintf(&b, "Run ID:     %s\n", r.RunID)
	fmt.Fprintf(&b, "Queries:    %d (positive %d, negative %d, errors %d)\n",
		r.Aggregate.Queries, r.Aggregate.Positive, r.Aggregate.Negative, r.Aggregate.Errors)

	fmt.Fprintln(&b)
	if r.Aggregate.Positive == 0 {
		// e.g. `eval --type negative`: the positive-scoped metrics are undefined,
		// so print n/a rather than a misleading 0.00.
		fmt.Fprintln(&b, "Retrieval metrics (positive queries only): n/a (no positive queries)")
	} else {
		fmt.Fprintln(&b, "Retrieval metrics (positive queries only):")
		fmt.Fprintf(&b, "  hit@1:        %.2f\n", r.Aggregate.HitAt1)
		fmt.Fprintf(&b, "  hit@3:        %.2f\n", r.Aggregate.HitAt3)
		fmt.Fprintf(&b, "  hit@5:        %.2f\n", r.Aggregate.HitAt5)
		fmt.Fprintf(&b, "  MRR@5:        %.2f\n", r.Aggregate.MRRAt5)
		fmt.Fprintf(&b, "  recall@5:     %.2f\n", r.Aggregate.RecallAt5)
		fmt.Fprintf(&b, "  recall@10:    %.2f\n", r.Aggregate.RecallAt10)
		// Diagnostic: a large recall@10−recall@5 gap means relevant chunks are
		// retrieved but ranked outside the top-5 → reranking has headroom. A small
		// gap means the misses are absent from the top-10 → embedding/chunking is
		// the floor (reranking can't surface what retrieval didn't return).
		if gap := r.Aggregate.RecallAt10 - r.Aggregate.RecallAt5; gap >= 0.10 {
			fmt.Fprintf(&b, "  recall gap:   %.2f (>=0.10 → reranking has headroom)\n", gap)
		} else {
			fmt.Fprintf(&b, "  recall gap:   %.2f (<0.10 → embedding/chunking is the floor)\n", gap)
		}
	}

	if r.Aggregate.Negative > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "Negative queries pass rate: %.2f\n", r.Aggregate.NegativePassRate)
	}

	if r.Answer != nil {
		a := r.Answer
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "Answer metrics (reference-free; generator: %s):\n", a.Generator)
		fmt.Fprintf(&b, "  generated:                %d (errors %d)\n", a.Generated, a.Errors)
		fmt.Fprintf(&b, "  well-formed rate:         %.2f\n", a.WellFormedRate)
		fmt.Fprintf(&b, "  cited rate (positive):    %s\n", rateOrNA(a.CitedRate, r.Aggregate.Positive > 0))
		fmt.Fprintf(&b, "  all-citations-valid:      %s\n", rateOrNA(a.AllCitationsValidRate, r.Aggregate.Positive > 0))
		fmt.Fprintf(&b, "  dangling citations:       %d\n", a.DanglingCitations)
		fmt.Fprintf(&b, "  refusal rate (negative):  %s\n", rateOrNA(a.RefusalRateNegative, r.Aggregate.Negative > 0))
		fmt.Fprintf(&b, "  generate p50/p95 (ms):    %d / %d\n", a.GenerateP50MS, a.GenerateP95MS)
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Latency (ms):")
	fmt.Fprintf(&b, "  total p50/p95:   %d / %d\n", r.Aggregate.LatencyTotalP50MS, r.Aggregate.LatencyTotalP95MS)
	fmt.Fprintf(&b, "  embed p50/p95:   %d / %d\n", r.Aggregate.LatencyEmbedP50MS, r.Aggregate.LatencyEmbedP95MS)
	fmt.Fprintf(&b, "  qdrant p50/p95:  %d / %d\n", r.Aggregate.LatencyQdrantP50MS, r.Aggregate.LatencyQdrantP95MS)

	if len(r.ByType) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "By type:")
		types := make([]string, 0, len(r.ByType))
		for t := range r.ByType {
			types = append(types, string(t))
		}
		sort.Strings(types)
		for _, t := range types {
			tb := r.ByType[QueryType(t)]
			if QueryType(t) == TypeNegative {
				fmt.Fprintf(&b, "  %-12s n=%d  pass_rate=%.2f\n", t, tb.Count, tb.NegativePassRate)
			} else {
				fmt.Fprintf(&b, "  %-12s n=%d  hit@5=%.2f  MRR@5=%.2f\n", t, tb.Count, tb.HitAt5, tb.MRRAt5)
			}
		}
	}

	// Failed queries (positive miss or negative fail or error).
	var failures []QueryResult
	for _, q := range r.Queries {
		if q.Error != "" {
			failures = append(failures, q)
			continue
		}
		if q.Type == TypeNegative {
			if q.Negative != nil && !q.Negative.Pass {
				failures = append(failures, q)
			}
			continue
		}
		if !q.HitAt5 {
			failures = append(failures, q)
		}
	}

	if len(failures) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "Failures (%d):\n", len(failures))
		for _, q := range failures {
			switch {
			case q.Error != "":
				fmt.Fprintf(&b, "  [%s] %s — ERROR: %s\n", q.Type, q.ID, q.Error)
			case q.Type == TypeNegative:
				fmt.Fprintf(&b, "  [negative] %s — top1 score %.4f >= threshold %.4f\n", q.ID, q.TopScore, q.Negative.Threshold)
			default:
				fmt.Fprintf(&b, "  [%s] %s — top-5 missed expected file/symbol; top1=%s (%.4f)\n",
					q.Type, q.ID, topFile(q), q.TopScore)
			}
		}
	}

	return b.String()
}

// rateOrNA formats a rate as a 2-decimal value, or "n/a" when the metric has no
// denominator (e.g. a positive-scoped rate under `--type negative`). This avoids
// a misleading 0.00 that reads as failure rather than "not applicable".
func rateOrNA(rate float64, applicable bool) string {
	if !applicable {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", rate)
}

func topFile(q QueryResult) string {
	if len(q.Results) == 0 {
		return "<no results>"
	}
	r := q.Results[0]
	if r.Name != "" {
		return fmt.Sprintf("%s::%s", r.FilePath, r.Name)
	}
	return r.FilePath
}
