package eval

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// QueryType tags a golden query with its retrieval-behavior category. Used to
// produce per-type aggregate metrics so a regression in one category is visible
// even if the overall score is fine.
type QueryType string

const (
	// TypeNavigation covers "where is X defined?" / exact-symbol queries.
	TypeNavigation QueryType = "navigation"
	// TypeConcept covers "how does Y work?" / explanatory queries.
	TypeConcept QueryType = "concept"
	// TypeBehavior covers "when does Z fail?" / runtime-behavior queries.
	TypeBehavior QueryType = "behavior"
	// TypeNegative covers queries that should NOT match well.
	TypeNegative QueryType = "negative"
)

// Filters holds the language and repo filters applied during search.
type Filters struct {
	Languages []string `yaml:"languages"`
	Repos     []string `yaml:"repos"`
}

// NegativeMatch holds expectations for a negative query — one that should not
// have a strong top result.
type NegativeMatch struct {
	// Top1ScoreBelow asserts the top-1 result's score is below this threshold.
	// 0 means no threshold check.
	Top1ScoreBelow float32 `yaml:"top1_score_below"`
}

// Query is a single golden query with expected results.
type Query struct {
	ID       string        `yaml:"id"`
	Query    string        `yaml:"query"`
	Type     QueryType     `yaml:"type"`
	Filters  Filters       `yaml:"filters"`
	Expected Expected      `yaml:"expected"`
	Negative NegativeMatch `yaml:"negative"`
}

// Dataset is the parsed golden YAML file.
type Dataset struct {
	Queries []Query `yaml:"queries"`
}

// LoadDataset reads a YAML golden dataset from disk and validates required
// fields. Returns an error on syntax errors, missing IDs, missing queries, or
// duplicate IDs.
func LoadDataset(path string) (*Dataset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading dataset %s: %w", path, err)
	}

	var ds Dataset
	if err := yaml.Unmarshal(data, &ds); err != nil {
		return nil, fmt.Errorf("parsing dataset %s: %w", path, err)
	}

	if len(ds.Queries) == 0 {
		return nil, fmt.Errorf("dataset %s has no queries", path)
	}

	seen := make(map[string]struct{}, len(ds.Queries))
	for i, q := range ds.Queries {
		if q.ID == "" {
			return nil, fmt.Errorf("query #%d in %s has no id", i+1, path)
		}
		if q.Query == "" {
			return nil, fmt.Errorf("query %q in %s has empty query text", q.ID, path)
		}
		if _, dup := seen[q.ID]; dup {
			return nil, fmt.Errorf("duplicate query id %q in %s", q.ID, path)
		}
		seen[q.ID] = struct{}{}

		// Negative queries don't need expected files/symbols; everything else
		// must declare at least one positive expectation.
		if q.Type != TypeNegative && !q.Expected.HasAny() {
			return nil, fmt.Errorf("query %q has no expected files or symbols", q.ID)
		}
	}

	return &ds, nil
}
