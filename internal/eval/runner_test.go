package eval

import (
	"context"
	"strings"
	"testing"
)

func TestRunnerRunRejectsLimitBelowDefault(t *testing.T) {
	t.Parallel()

	ds := &Dataset{
		Queries: []Query{
			{
				ID:    "q1",
				Query: "how does chunking work?",
				Type:  TypeConcept,
				Expected: Expected{
					Files: []string{"internal/ingest/chunker.go"},
				},
			},
		},
	}

	r := &Runner{
		Limit: DefaultLimit - 1,
	}

	_, err := r.Run(context.Background(), "docs/eval/golden.yaml", ds)
	if err == nil {
		t.Fatal("expected error for limit below default")
	}
	if !strings.Contains(err.Error(), "limit must be >=") {
		t.Fatalf("unexpected error: %v", err)
	}
}
