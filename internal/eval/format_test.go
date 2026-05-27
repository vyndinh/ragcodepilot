package eval

import (
	"strings"
	"testing"
)

// When a run has no positive queries (e.g. `eval --type negative`), the
// positive-scoped metrics must read "n/a", not a misleading 0.00.
func TestFormatHuman_NoPositiveQueriesShowsNA(t *testing.T) {
	t.Parallel()

	r := &Report{
		Aggregate: Aggregate{Queries: 4, Positive: 0, Negative: 4, NegativePassRate: 1.0},
		Answer: &AnswerAggregate{
			Generator: "ollama/qwen2.5-coder:7b", Generated: 4,
			WellFormedRate: 1.0, RefusalRateNegative: 1.0,
			// CitedRate / AllCitationsValidRate are 0 because there are no positives.
		},
	}

	out := FormatHuman(r)

	if !strings.Contains(out, "Retrieval metrics (positive queries only): n/a") {
		t.Errorf("retrieval block should say n/a with no positive queries\n%s", out)
	}
	if !strings.Contains(out, "cited rate (positive):    n/a") {
		t.Errorf("cited rate should be n/a with no positive queries\n%s", out)
	}
	if !strings.Contains(out, "refusal rate (negative):  1.00") {
		t.Errorf("refusal rate should show a real value when negatives exist\n%s", out)
	}
}

// With positive queries present, the real numbers are printed (no n/a).
func TestFormatHuman_PositiveQueriesShowNumbers(t *testing.T) {
	t.Parallel()

	r := &Report{
		Aggregate: Aggregate{Queries: 5, Positive: 5, HitAt5: 0.8, MRRAt5: 0.66},
	}

	out := FormatHuman(r)

	if strings.Contains(out, "n/a") {
		t.Errorf("should not show n/a when positives exist\n%s", out)
	}
	if !strings.Contains(out, "hit@5:        0.80") {
		t.Errorf("expected hit@5 value in output\n%s", out)
	}
}
