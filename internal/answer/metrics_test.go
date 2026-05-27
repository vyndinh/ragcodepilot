package answer

import (
	"reflect"
	"testing"
)

func TestParseCitations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		answer string
		want   []int
	}{
		{name: "none", answer: "no citations here", want: nil},
		{name: "single", answer: "see [1]", want: []int{1}},
		{name: "multiple sorted", answer: "[3] then [1] and [2]", want: []int{1, 2, 3}},
		{name: "dedupe", answer: "[1] and again [1]", want: []int{1}},
		{name: "multi-digit", answer: "ref [12]", want: []int{12}},
		{name: "ignores non-numeric brackets", answer: "[see] this [1]", want: []int{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ParseCitations(tt.answer)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseCitations(%q) = %v, want %v", tt.answer, got, tt.want)
			}
		})
	}
}

func TestValidateCitations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		citations   []int
		numChunks   int
		wantValid   int
		wantInvalid int
	}{
		{name: "all valid", citations: []int{1, 2, 3}, numChunks: 5, wantValid: 3, wantInvalid: 0},
		{name: "dangling high", citations: []int{1, 6}, numChunks: 5, wantValid: 1, wantInvalid: 1},
		{name: "zero is invalid", citations: []int{0, 1}, numChunks: 5, wantValid: 1, wantInvalid: 1},
		{name: "none", citations: nil, numChunks: 5, wantValid: 0, wantInvalid: 0},
		{name: "no chunks all invalid", citations: []int{1}, numChunks: 0, wantValid: 0, wantInvalid: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			valid, invalid := ValidateCitations(tt.citations, tt.numChunks)
			if valid != tt.wantValid || invalid != tt.wantInvalid {
				t.Errorf("ValidateCitations(%v, %d) = (%d, %d), want (%d, %d)",
					tt.citations, tt.numChunks, valid, invalid, tt.wantValid, tt.wantInvalid)
			}
		})
	}
}

func TestIsWellFormed(t *testing.T) {
	t.Parallel()

	if IsWellFormed("") || IsWellFormed("   \n\t ") {
		t.Error("blank/whitespace answers should not be well-formed")
	}
	if !IsWellFormed("an answer") {
		t.Error("non-empty answer should be well-formed")
	}
}

func TestIsRefusal(t *testing.T) {
	t.Parallel()

	refusals := []string{
		"The provided chunks do not contain enough information to answer.",
		"I cannot answer this based on the given context.",
		"There is no relevant information in the chunks.",
		"That detail is not provided in the supplied code.",
		// Real qwen2.5-coder refusals observed on negative golden queries.
		"The OAuth middleware is not implemented in the provided code chunks.",
		"The gRPC gateway HTTP router setup is not found in the provided code chunks.",
		"There is no mention of a Terraform module for production networking.",
	}
	for _, r := range refusals {
		if !IsRefusal(r) {
			t.Errorf("expected refusal for %q", r)
		}
	}

	answers := []string{
		"Change detection hashes file contents [1] and compares them [2].",
		"The Pipeline.Run method orchestrates walking, chunking, and upsert.",
	}
	for _, a := range answers {
		if IsRefusal(a) {
			t.Errorf("did not expect refusal for %q", a)
		}
	}
}
