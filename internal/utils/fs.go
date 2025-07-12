package utils

import (
	"os"
	"path/filepath"
)

func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func StreamDir(baseDir, streamName string) string {
	return filepath.Join(baseDir, streamName)
}

func RecordingPath(baseDir, streamName, timestamp string) string {
	return filepath.Join(StreamDir(baseDir, streamName), timestamp+".mp3")
}

func MetadataPath(baseDir, streamName, timestamp string) string {
	return filepath.Join(StreamDir(baseDir, streamName), timestamp+".meta")
}
