package utils

import "testing"

func TestExtensionForCodec(t *testing.T) {
	tests := []struct {
		name  string
		codec string
		want  string
	}{
		{name: "mp3 exact", codec: "mp3", want: ".mp3"},
		{name: "mp3 variant", codec: "mp3float", want: ".mp3"},
		{name: "aac", codec: "aac", want: ".aac"},
		{name: "aac latm", codec: "aac_latm", want: ".aac"},
		{name: "vorbis", codec: "vorbis", want: ".ogg"},
		{name: "opus", codec: "opus", want: ".opus"},
		{name: "flac", codec: "flac", want: ".flac"},
		{name: "unknown", codec: "pcm_s16le", want: ".pcm_s16le"},
		{name: "uppercase", codec: "OPUS", want: ".opus"},
		{name: "empty", codec: "", want: ".mp3"},
		{name: "unsafe", codec: "../bad", want: ".mp3"},
		{name: "non ascii", codec: "μlaw", want: ".mp3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extensionForCodec(tt.codec); got != tt.want {
				t.Errorf("extensionForCodec(%q) = %q, want %q", tt.codec, got, tt.want)
			}
		})
	}
}

func TestContentType(t *testing.T) {
	tests := []struct {
		name string
		ext  string
		want string
	}{
		{name: "mp3", ext: ".mp3", want: "audio/mpeg"},
		{name: "aac", ext: ".aac", want: "audio/aac"},
		{name: "m4a", ext: ".m4a", want: "audio/aac"},
		{name: "ogg", ext: ".ogg", want: "audio/ogg"},
		{name: "opus", ext: ".opus", want: "audio/opus"},
		{name: "flac", ext: ".flac", want: "audio/flac"},
		{name: "wav", ext: ".wav", want: "audio/wav"},
		{name: "case insensitive", ext: ".OPUS", want: "audio/opus"},
		{name: "unknown", ext: ".bin", want: "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContentType(tt.ext); got != tt.want {
				t.Errorf("ContentType(%q) = %q, want %q", tt.ext, got, tt.want)
			}
		})
	}
}
