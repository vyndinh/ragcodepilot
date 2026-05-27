package eval

import (
	"context"
	"fmt"
	"testing"

	"github.com/dinhvy/ragcodepilot/internal/answer"
	"github.com/dinhvy/ragcodepilot/internal/model"
)

func fakeResults(n int) []model.SearchResult {
	out := make([]model.SearchResult, n)
	for i := range out {
		out[i] = model.SearchResult{Chunk: model.CodeChunk{
			FilePath: fmt.Sprintf("f%d.go", i), StartLine: 1, EndLine: 2, Content: "x",
		}}
	}
	return out
}

func TestEvaluateAnswer_RespectsAnswerLimit(t *testing.T) {
	t.Parallel()

	gen := answer.NewFakeGenerator("see [1]")
	r := &Runner{Generator: gen, AnswerLimit: 2}

	ar := r.evaluateAnswer(context.Background(), "q", fakeResults(5))
	if ar.Error != "" {
		t.Fatalf("unexpected error: %s", ar.Error)
	}
	if len(gen.LastPrompt.Chunks) != 2 {
		t.Errorf("answer-limit 2 should feed 2 chunks, fed %d", len(gen.LastPrompt.Chunks))
	}
}

func TestEvaluateAnswer_ZeroLimitFeedsAll(t *testing.T) {
	t.Parallel()

	gen := answer.NewFakeGenerator("ok")
	r := &Runner{Generator: gen, AnswerLimit: 0} // 0 = use all retrieved

	_ = r.evaluateAnswer(context.Background(), "q", fakeResults(3))
	if len(gen.LastPrompt.Chunks) != 3 {
		t.Errorf("answer-limit 0 should feed all 3 chunks, fed %d", len(gen.LastPrompt.Chunks))
	}
}

func TestEvaluateAnswer_LimitAboveResultsFeedsAll(t *testing.T) {
	t.Parallel()

	gen := answer.NewFakeGenerator("ok")
	r := &Runner{Generator: gen, AnswerLimit: 10} // more than available

	_ = r.evaluateAnswer(context.Background(), "q", fakeResults(3))
	if len(gen.LastPrompt.Chunks) != 3 {
		t.Errorf("answer-limit above result count should feed all 3, fed %d", len(gen.LastPrompt.Chunks))
	}
}

func TestScoreAnswer(t *testing.T) {
	t.Parallel()

	t.Run("well-formed cited answer", func(t *testing.T) {
		t.Parallel()
		got := scoreAnswer("Change detection hashes files [1] and compares [2].", 5)
		if !got.WellFormed {
			t.Error("expected well-formed")
		}
		if got.Refused {
			t.Error("did not expect refusal")
		}
		if got.ValidCitations != 2 || got.InvalidCitations != 0 {
			t.Errorf("citations valid/invalid = %d/%d, want 2/0", got.ValidCitations, got.InvalidCitations)
		}
	})

	t.Run("dangling citation", func(t *testing.T) {
		t.Parallel()
		got := scoreAnswer("See [1] and [9].", 5) // only 5 chunks, [9] dangles
		if got.ValidCitations != 1 || got.InvalidCitations != 1 {
			t.Errorf("valid/invalid = %d/%d, want 1/1", got.ValidCitations, got.InvalidCitations)
		}
	})

	t.Run("refusal", func(t *testing.T) {
		t.Parallel()
		got := scoreAnswer("The chunks do not contain enough information to answer.", 5)
		if !got.Refused {
			t.Error("expected refusal")
		}
		if len(got.Citations) != 0 {
			t.Errorf("refusal should have no citations, got %v", got.Citations)
		}
	})

	t.Run("empty answer not well-formed", func(t *testing.T) {
		t.Parallel()
		got := scoreAnswer("   ", 5)
		if got.WellFormed {
			t.Error("blank answer should not be well-formed")
		}
	})
}

func TestAggregateAnswers(t *testing.T) {
	t.Parallel()

	queries := []QueryResult{
		// positive: well-formed, all citations valid
		{Type: TypeConcept, Answer: &AnswerResult{GenerateMS: 100, WellFormed: true, Citations: []int{1, 2}, ValidCitations: 2}},
		// positive: well-formed, one dangling citation
		{Type: TypeConcept, Answer: &AnswerResult{GenerateMS: 200, WellFormed: true, Citations: []int{1, 9}, ValidCitations: 1, InvalidCitations: 1}},
		// negative: refused (good)
		{Type: TypeNegative, Answer: &AnswerResult{GenerateMS: 50, WellFormed: true, Refused: true}},
		// negative: did NOT refuse (hallucination risk)
		{Type: TypeNegative, Answer: &AnswerResult{GenerateMS: 60, WellFormed: true, Citations: []int{1}, ValidCitations: 1}},
		// generation error
		{Type: TypeConcept, Answer: &AnswerResult{Error: "boom"}},
	}

	agg := aggregateAnswers(queries, "ollama/qwen2.5-coder:7b")
	if agg == nil {
		t.Fatal("expected non-nil aggregate")
	}
	if agg.Generator != "ollama/qwen2.5-coder:7b" {
		t.Errorf("generator = %q", agg.Generator)
	}
	if agg.Generated != 4 || agg.Errors != 1 {
		t.Errorf("generated/errors = %d/%d, want 4/1", agg.Generated, agg.Errors)
	}
	if agg.WellFormedRate != 1.0 {
		t.Errorf("well-formed rate = %.2f, want 1.00 (all 4 non-error answers well-formed)", agg.WellFormedRate)
	}
	// positive answers = 2; both cited → cited rate 1.0
	if agg.CitedRate != 1.0 {
		t.Errorf("cited rate = %.2f, want 1.00", agg.CitedRate)
	}
	// of 2 positive cited, 1 had no dangling refs → 0.5
	if agg.AllCitationsValidRate != 0.5 {
		t.Errorf("all-citations-valid rate = %.2f, want 0.50", agg.AllCitationsValidRate)
	}
	if agg.DanglingCitations != 1 {
		t.Errorf("dangling citations = %d, want 1", agg.DanglingCitations)
	}
	// 2 negatives, 1 refused → 0.5
	if agg.RefusalRateNegative != 0.5 {
		t.Errorf("refusal rate (negative) = %.2f, want 0.50", agg.RefusalRateNegative)
	}
}

func TestAggregateAnswers_NilWhenNoAnswers(t *testing.T) {
	t.Parallel()

	queries := []QueryResult{
		{Type: TypeConcept, HitAt5: true}, // retrieval-only, no Answer
	}
	if agg := aggregateAnswers(queries, ""); agg != nil {
		t.Errorf("expected nil aggregate for retrieval-only run, got %+v", agg)
	}
}
