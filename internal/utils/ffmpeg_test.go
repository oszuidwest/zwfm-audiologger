package utils

import (
	"os/exec"
	"testing"
	"time"
)

func TestFFmpegCommandPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  *exec.Cmd
		want string
	}{
		{
			name: "record",
			cmd:  RecordCommand(t.Context(), "/custom/ffmpeg", "https://stream.example.com", time.Second, "out.mkv"),
			want: "/custom/ffmpeg",
		},
		{
			name: "remux",
			cmd:  RemuxCommand(t.Context(), "/custom/ffmpeg", "in.mkv", "out.mp3"),
			want: "/custom/ffmpeg",
		},
		{
			name: "probe",
			cmd:  ProbeCommand(t.Context(), "/custom/ffprobe", "in.mp3"),
			want: "/custom/ffprobe",
		},
		{
			name: "silence detect",
			cmd:  SilenceDetectCommand(t.Context(), "/custom/ffmpeg", "in.mp3", -40, 5),
			want: "/custom/ffmpeg",
		},
		{
			name: "audio stats",
			cmd:  AudioStatsCommand(t.Context(), "/custom/ffmpeg", "in.mp3"),
			want: "/custom/ffmpeg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.cmd.Args[0] != tt.want {
				t.Errorf("Args[0] = %q, want %q", tt.cmd.Args[0], tt.want)
			}
			if tt.cmd.Path != tt.want {
				t.Errorf("Path = %q, want %q", tt.cmd.Path, tt.want)
			}
		})
	}
}
