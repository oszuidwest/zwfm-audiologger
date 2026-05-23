package utils

import (
	"context"
	"testing"
	"time"
)

func TestFFmpegCommandPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "record",
			got:  RecordCommand(context.Background(), "/custom/ffmpeg", "https://stream.example.com", time.Second, "out.mkv").Args[0],
			want: "/custom/ffmpeg",
		},
		{
			name: "remux",
			got:  RemuxCommand("/custom/ffmpeg", "in.mkv", "out.mp3").Args[0],
			want: "/custom/ffmpeg",
		},
		{
			name: "probe",
			got:  ProbeCommand(context.Background(), "/custom/ffprobe", "in.mp3").Args[0],
			want: "/custom/ffprobe",
		},
		{
			name: "silence detect",
			got:  SilenceDetectCommand(context.Background(), "/custom/ffmpeg", "in.mp3", -40, 5).Args[0],
			want: "/custom/ffmpeg",
		},
		{
			name: "audio stats",
			got:  AudioStatsCommand(context.Background(), "/custom/ffmpeg", "in.mp3").Args[0],
			want: "/custom/ffmpeg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Fatalf("command path = %q, want %q", tt.got, tt.want)
			}
		})
	}
}
