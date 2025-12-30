package ffmpeg

import (
	"testing"
)

func TestTruncateOutput(t *testing.T) {
	tests := []struct {
		name   string
		input  []byte
		maxLen int
		want   string
	}{
		{
			name:   "short input",
			input:  []byte("hello"),
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length",
			input:  []byte("hello"),
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "truncated",
			input:  []byte("hello world"),
			maxLen: 5,
			want:   "hello... (truncated)",
		},
		{
			name:   "empty input",
			input:  []byte(""),
			maxLen: 10,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateOutput(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("TruncateOutput(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}
