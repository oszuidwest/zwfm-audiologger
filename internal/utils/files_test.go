package utils

import (
	"math"
	"testing"
)

func TestAvailableBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    uint64
		blockSize uint64
		want      uint64
		wantErr   bool
	}{
		{
			name:      "calculates available bytes",
			blocks:    10,
			blockSize: 4096,
			want:      40960,
		},
		{
			name:      "allows maximum value",
			blocks:    math.MaxUint64,
			blockSize: 1,
			want:      math.MaxUint64,
		},
		{
			name:      "rejects overflow",
			blocks:    math.MaxUint64,
			blockSize: 2,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := availableBytes("/recordings", tt.blocks, tt.blockSize)
			if (err != nil) != tt.wantErr {
				t.Fatalf("availableBytes() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("availableBytes() = %d, want %d", got, tt.want)
			}
		})
	}
}
