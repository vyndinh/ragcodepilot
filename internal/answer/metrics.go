package answer

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// citationPattern matches bracketed citation markers like [1] or [12]. It is
// intentionally strict — only digits inside brackets — so prose like "[see
// below]" is not counted as a citation.
var citationPattern = regexp.MustCompile(`\[(\d+)\]`)

// ParseCitations extracts the unique citation numbers referenced in an answer,
// in ascending order. "[1] and [3], also [1]" → [1, 3]. Returns an empty slice
// when there are no citations.
func ParseCitations(answer string) []int {
	matches := citationPattern.FindAllStringSubmatch(answer, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(matches))
	out := make([]int, 0, len(matches))
	for _, m := range matches {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// ValidateCitations splits citation numbers into those that point at a real
// chunk (1 <= n <= numChunks) and those that don't (dangling references, e.g.
// the model cited [6] when only 5 chunks were provided). numChunks is the count
// of chunks placed in the prompt.
func ValidateCitations(citations []int, numChunks int) (valid, invalid int) {
	for _, n := range citations {
		if n >= 1 && n <= numChunks {
			valid++
		} else {
			invalid++
		}
	}
	return valid, invalid
}

// IsWellFormed reports whether an answer is non-empty after trimming. It is the
// floor for "the generator produced something usable."
func IsWellFormed(answer string) bool {
	return strings.TrimSpace(answer) != ""
}

// refusalMarkers are lowercase phrases that signal the model declined to answer
// because the retrieved chunks were insufficient. The v0 system prompt instructs
// the model to "say so explicitly … do not invent details," so genuine refusals
// tend to contain one of these. This is a HEURISTIC, not ground truth: it can
// miss a creatively-worded refusal or fire on an answer that merely mentions a
// phrase. It is good enough for a reported diagnostic (the hallucination floor
// on negative queries), not a hard gate.
var refusalMarkers = []string{
	"not enough information",
	"insufficient information",
	"do not contain enough",
	"don't contain enough",
	"does not contain enough",
	"doesn't contain enough",
	"do not contain",
	"don't contain",
	"does not contain",
	"doesn't contain",
	"no relevant information",
	"no information",
	"cannot answer",
	"can't answer",
	"unable to answer",
	"not provided",
	"not enough context",
	"cannot determine",
	"can't determine",
	// Phrasings observed from qwen2.5-coder on negative queries that the list
	// above missed. The model often says a thing "is not implemented/found in
	// the provided chunks" rather than the chunks "do not contain" it.
	"not implemented in",
	"not found in the",
	"no mention of",
	"does not exist in",
	"do not include",
	"does not include",
}

// IsRefusal reports whether an answer appears to decline answering for lack of
// grounded context. See refusalMarkers for the heuristic's limitations.
func IsRefusal(answer string) bool {
	lower := strings.ToLower(answer)
	for _, marker := range refusalMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
