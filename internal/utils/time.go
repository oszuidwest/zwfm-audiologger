package utils

import (
	"fmt"
	"time"
)

// FormatDuration formats a duration for FFmpeg (HH:MM:SS.mmm format)
func FormatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	milliseconds := int(d.Milliseconds()) % 1000

	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, milliseconds)
}

// FormatTimestamp formats a time for file naming (YYYY-MM-DD_HH format)
func FormatTimestamp(t time.Time) string {
	return t.Format("2006-01-02_15")
}