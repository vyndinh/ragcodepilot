package eval

import (
	"context"
	"fmt"
	"time"

	"github.com/dinhvy/ragcodepilot/internal/model"
	"github.com/dinhvy/ragcodepilot/internal/search"
)

// DefaultLimit is the per-query result limit used when none is specified.
// 10 covers hit@5, MRR@5, and recall@10 without extra work.
const DefaultLimit = 10

// QueryResult captures the per-query outcome of an eval run.
type QueryResult struct {
	ID         string               `json:"id"`
	Type       QueryType            `json:"type,omitempty"`
	Query      string               `json:"query"`
	HitAt1     bool                 `json:"hit_at_1"`
	HitAt3     bool                 `json:"hit_at_3"`
	HitAt5     bool                 `json:"hit_at_5"`
	MRRAt5     float64              `json:"mrr_at_5"`
	RecallAt10 float64              `json:"recall_at_10"`
	TopScore   float32              `json:"top_score"`
	Negative   *NegativeResult      `json:"negative,omitempty"`
	EmbedMS    int64                `json:"embed_ms"`
	QdrantMS   int64                `json:"qdrant_ms"`
	TotalMS    int64                `json:"total_ms"`
	Results    []resultSummary      `json:"top_results,omitempty"`
	Error      string               `json:"error,omitempty"`
	expected   Expected             // kept for aggregation; not serialized
	rawResults []model.SearchResult // kept for aggregation; not serialized
}

// NegativeResult holds the pass/fail outcome for a negative query.
type NegativeResult struct {
	Threshold float32 `json:"threshold"`
	Pass      bool    `json:"pass"`
}

// resultSummary is the small payload we keep per result for the report — the
// fields a human or a script would actually want to inspect.
type resultSummary struct {
	FilePath  string  `json:"file_path"`
	Name      string  `json:"name,omitempty"`
	Score     float32 `json:"score"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
}

// Aggregate holds mean metrics and latency percentiles across all queries
// that produced results (errors and negatives are excluded from mean metrics).
type Aggregate struct {
	Queries            int     `json:"queries"`
	Positive           int     `json:"positive_queries"`
	Negative           int     `json:"negative_queries"`
	Errors             int     `json:"errors"`
	HitAt1             float64 `json:"hit_at_1"`
	HitAt3             float64 `json:"hit_at_3"`
	HitAt5             float64 `json:"hit_at_5"`
	MRRAt5             float64 `json:"mrr_at_5"`
	RecallAt10         float64 `json:"recall_at_10"`
	NegativePassRate   float64 `json:"negative_pass_rate"`
	LatencyTotalP50MS  int64   `json:"latency_total_p50_ms"`
	LatencyTotalP95MS  int64   `json:"latency_total_p95_ms"`
	LatencyEmbedP50MS  int64   `json:"latency_embed_p50_ms"`
	LatencyEmbedP95MS  int64   `json:"latency_embed_p95_ms"`
	LatencyQdrantP50MS int64   `json:"latency_qdrant_p50_ms"`
	LatencyQdrantP95MS int64   `json:"latency_qdrant_p95_ms"`
}

// TypeBreakdown aggregates metrics for one query type.
type TypeBreakdown struct {
	Count            int     `json:"count"`
	HitAt5           float64 `json:"hit_at_5,omitempty"`
	MRRAt5           float64 `json:"mrr_at_5,omitempty"`
	NegativePassRate float64 `json:"negative_pass_rate,omitempty"`
}

// Report is the full output of an eval run.
type Report struct {
	RunID      string                      `json:"run_id"`
	Dataset    string                      `json:"dataset"`
	Collection string                      `json:"collection"`
	Embedder   string                      `json:"embedder"`
	Mode       search.SearchMode           `json:"mode"`
	Limit      int                         `json:"limit"`
	Aggregate  Aggregate                   `json:"aggregate"`
	ByType     map[QueryType]TypeBreakdown `json:"by_type"`
	Queries    []QueryResult               `json:"queries"`
}

// Runner executes a dataset of golden queries through the production search
// path, capturing per-stage latencies and computing retrieval metrics. It
// deliberately runs through search.Searcher so any future search-layer
// behavior (validation, reranking, hybrid fusion) is included in the measured
// results.
type Runner struct {
	Searcher   *search.Searcher
	Collection string
	Limit      int

	// EmbedderName is used in the report; descriptive only.
	EmbedderName string
	Mode         search.SearchMode
}

// Run executes the dataset and returns a Report. Per-query errors are captured
// in the QueryResult, not returned — a single bad query should not abort the
// whole run.
func (r *Runner) Run(ctx context.Context, datasetPath string, ds *Dataset) (*Report, error) {
	if r.Limit < DefaultLimit {
		return nil, fmt.Errorf("limit must be >= %d for recall@10, got %d", DefaultLimit, r.Limit)
	}

	mode, err := search.ParseSearchMode(string(r.Mode))
	if err != nil {
		return nil, err
	}

	report := &Report{
		RunID:      time.Now().UTC().Format("2006-01-02T15-04-05Z"),
		Dataset:    datasetPath,
		Collection: r.Collection,
		Embedder:   r.EmbedderName,
		Mode:       mode,
		Limit:      r.Limit,
		ByType:     make(map[QueryType]TypeBreakdown),
		Queries:    make([]QueryResult, 0, len(ds.Queries)),
	}

	for _, q := range ds.Queries {
		qr := r.runQuery(ctx, q, mode)
		report.Queries = append(report.Queries, qr)
	}

	report.Aggregate = aggregate(report.Queries)
	report.ByType = breakdownByType(report.Queries)
	return report, nil
}

func (r *Runner) runQuery(ctx context.Context, q Query, mode search.SearchMode) QueryResult {
	qr := QueryResult{
		ID:       q.ID,
		Type:     q.Type,
		Query:    q.Query,
		expected: q.Expected,
	}

	results, timings, err := r.Searcher.SearchWithTimings(
		ctx, r.Collection, q.Query, mode, uint64(r.Limit), q.Filters.Languages, q.Filters.Repos,
	)
	qr.EmbedMS = timings.Embed.Milliseconds()
	qr.QdrantMS = timings.Qdrant.Milliseconds()
	qr.TotalMS = timings.Total.Milliseconds()
	if err != nil {
		qr.Error = err.Error()
		return qr
	}

	qr.rawResults = results
	qr.Results = summarize(results)

	if len(results) > 0 {
		qr.TopScore = results[0].Score
	}

	if q.Type == TypeNegative {
		thr := q.Negative.Top1ScoreBelow
		pass := len(results) == 0 || (thr > 0 && results[0].Score < thr)
		qr.Negative = &NegativeResult{Threshold: thr, Pass: pass}
		return qr
	}

	qr.HitAt1 = HitAtK(results, q.Expected, 1)
	qr.HitAt3 = HitAtK(results, q.Expected, 3)
	qr.HitAt5 = HitAtK(results, q.Expected, 5)
	qr.MRRAt5 = MRRAtK(results, q.Expected, 5)
	qr.RecallAt10 = RecallAtK(results, q.Expected, 10)
	return qr
}

func summarize(results []model.SearchResult) []resultSummary {
	out := make([]resultSummary, len(results))
	for i, r := range results {
		out[i] = resultSummary{
			FilePath:  r.Chunk.FilePath,
			Name:      r.Chunk.Name,
			Score:     r.Score,
			StartLine: r.Chunk.StartLine,
			EndLine:   r.Chunk.EndLine,
		}
	}
	return out
}

func aggregate(queries []QueryResult) Aggregate {
	agg := Aggregate{Queries: len(queries)}

	var (
		posCount        int
		negCount        int
		hit1Sum         int
		hit3Sum         int
		hit5Sum         int
		mrrSum          float64
		recallSum       float64
		negPass         int
		totalDurations  = make([]time.Duration, 0, len(queries))
		embedDurations  = make([]time.Duration, 0, len(queries))
		qdrantDurations = make([]time.Duration, 0, len(queries))
	)

	for _, q := range queries {
		if q.Error != "" {
			agg.Errors++
			continue
		}

		totalDurations = append(totalDurations, time.Duration(q.TotalMS)*time.Millisecond)
		embedDurations = append(embedDurations, time.Duration(q.EmbedMS)*time.Millisecond)
		qdrantDurations = append(qdrantDurations, time.Duration(q.QdrantMS)*time.Millisecond)

		if q.Type == TypeNegative {
			negCount++
			if q.Negative != nil && q.Negative.Pass {
				negPass++
			}
			continue
		}

		posCount++
		if q.HitAt1 {
			hit1Sum++
		}
		if q.HitAt3 {
			hit3Sum++
		}
		if q.HitAt5 {
			hit5Sum++
		}
		mrrSum += q.MRRAt5
		recallSum += q.RecallAt10
	}

	agg.Positive = posCount
	agg.Negative = negCount

	if posCount > 0 {
		agg.HitAt1 = float64(hit1Sum) / float64(posCount)
		agg.HitAt3 = float64(hit3Sum) / float64(posCount)
		agg.HitAt5 = float64(hit5Sum) / float64(posCount)
		agg.MRRAt5 = mrrSum / float64(posCount)
		agg.RecallAt10 = recallSum / float64(posCount)
	}
	if negCount > 0 {
		agg.NegativePassRate = float64(negPass) / float64(negCount)
	}

	agg.LatencyTotalP50MS = Percentile(totalDurations, 50).Milliseconds()
	agg.LatencyTotalP95MS = Percentile(totalDurations, 95).Milliseconds()
	agg.LatencyEmbedP50MS = Percentile(embedDurations, 50).Milliseconds()
	agg.LatencyEmbedP95MS = Percentile(embedDurations, 95).Milliseconds()
	agg.LatencyQdrantP50MS = Percentile(qdrantDurations, 50).Milliseconds()
	agg.LatencyQdrantP95MS = Percentile(qdrantDurations, 95).Milliseconds()

	return agg
}

func breakdownByType(queries []QueryResult) map[QueryType]TypeBreakdown {
	type acc struct {
		count    int
		hit5     int
		mrr      float64
		negPass  int
		negTotal int
	}
	counts := make(map[QueryType]*acc)
	for _, q := range queries {
		if q.Error != "" {
			continue
		}
		typ := q.Type
		if typ == "" {
			typ = "untyped"
		}
		a, ok := counts[typ]
		if !ok {
			a = &acc{}
			counts[typ] = a
		}
		a.count++
		if typ == TypeNegative {
			a.negTotal++
			if q.Negative != nil && q.Negative.Pass {
				a.negPass++
			}
			continue
		}
		if q.HitAt5 {
			a.hit5++
		}
		a.mrr += q.MRRAt5
	}

	out := make(map[QueryType]TypeBreakdown, len(counts))
	for typ, a := range counts {
		b := TypeBreakdown{Count: a.count}
		if typ == TypeNegative {
			if a.negTotal > 0 {
				b.NegativePassRate = float64(a.negPass) / float64(a.negTotal)
			}
		} else if a.count > 0 {
			b.HitAt5 = float64(a.hit5) / float64(a.count)
			b.MRRAt5 = a.mrr / float64(a.count)
		}
		out[typ] = b
	}
	return out
}
