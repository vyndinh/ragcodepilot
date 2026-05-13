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
	fmt.Fprintf(&b, "Run ID:     %s\n", r.RunID)
	fmt.Fprintf(&b, "Queries:    %d (positive %d, negative %d, errors %d)\n",
		r.Aggregate.Queries, r.Aggregate.Positive, r.Aggregate.Negative, r.Aggregate.Errors)

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Retrieval metrics (positive queries only):")
	fmt.Fprintf(&b, "  hit@1:        %.2f\n", r.Aggregate.HitAt1)
	fmt.Fprintf(&b, "  hit@3:        %.2f\n", r.Aggregate.HitAt3)
	fmt.Fprintf(&b, "  hit@5:        %.2f\n", r.Aggregate.HitAt5)
	fmt.Fprintf(&b, "  MRR@5:        %.2f\n", r.Aggregate.MRRAt5)
	fmt.Fprintf(&b, "  recall@10:    %.2f\n", r.Aggregate.RecallAt10)

	if r.Aggregate.Negative > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "Negative queries pass rate: %.2f\n", r.Aggregate.NegativePassRate)
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
