package embedding

import (
	"testing"
)

func TestValidateVectorBatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		vectors     [][]float32
		expectedDim int
		wantDim     int
		wantErr     bool
	}{
		{
			name:        "valid batch with 768-dim vectors",
			vectors:     [][]float32{make([]float32, 768), make([]float32, 768)},
			expectedDim: 0,
			wantDim:     768,
		},
		{
			name:        "valid batch with 384-dim vectors",
			vectors:     [][]float32{make([]float32, 384)},
			expectedDim: 0,
			wantDim:     384,
		},
		{
			name:        "valid batch matching expected dimension",
			vectors:     [][]float32{make([]float32, 768), make([]float32, 768)},
			expectedDim: 768,
			wantDim:     768,
		},
		{
			name:    "empty batch",
			vectors: [][]float32{},
			wantErr: true,
		},
		{
			name:    "nil batch",
			vectors: nil,
			wantErr: true,
		},
		{
			name:    "first vector is empty",
			vectors: [][]float32{{}},
			wantErr: true,
		},
		{
			name:    "second vector is empty",
			vectors: [][]float32{make([]float32, 768), {}},
			wantErr: true,
		},
		{
			name:    "inconsistent dimensions within batch",
			vectors: [][]float32{make([]float32, 768), make([]float32, 384)},
			wantErr: true,
		},
		{
			name:        "dimension mismatch with expected",
			vectors:     [][]float32{make([]float32, 384)},
			expectedDim: 768,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dim, err := ValidateVectorBatch(tt.vectors, tt.expectedDim)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got dim=%d", dim)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if dim != tt.wantDim {
				t.Fatalf("got dim=%d, want %d", dim, tt.wantDim)
			}
		})
	}
}
