package embedding

import "fmt"

// ValidateVectorBatch checks that a batch of vectors is valid and consistent.
//
// Rules:
//   - The batch must not be empty.
//   - No vector may be empty (zero-length).
//   - All vectors in the batch must have the same dimension.
//   - If expectedDim > 0, all vectors must match that dimension.
//
// Returns the detected dimension on success.
func ValidateVectorBatch(vectors [][]float32, expectedDim int) (int, error) {
	if len(vectors) == 0 {
		return 0, fmt.Errorf("empty vector batch")
	}

	dim := len(vectors[0])
	if dim == 0 {
		return 0, fmt.Errorf("vector 0 is empty (zero-length)")
	}

	for i := 1; i < len(vectors); i++ {
		if len(vectors[i]) == 0 {
			return 0, fmt.Errorf("vector %d is empty (zero-length)", i)
		}
		if len(vectors[i]) != dim {
			return 0, fmt.Errorf("inconsistent dimensions in batch: vector 0 has %d, vector %d has %d", dim, i, len(vectors[i]))
		}
	}

	if expectedDim > 0 && dim != expectedDim {
		return 0, fmt.Errorf("dimension mismatch: expected %d, got %d", expectedDim, dim)
	}

	return dim, nil
}
