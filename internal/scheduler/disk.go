// Package scheduler handles scheduling for recordings and cleanup.
package scheduler

import (
	"syscall"
)

// diskInfo represents disk space information.
type diskInfo struct {
	TotalBytes     uint64
	AvailableBytes uint64
	UsedBytes      uint64
	FreePercent    float64
}

// getDiskSpace returns disk space information for the given path.
func getDiskSpace(path string) (*diskInfo, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, err
	}

	// Calculate bytes
	totalBytes := stat.Blocks * uint64(stat.Bsize)
	availableBytes := stat.Bavail * uint64(stat.Bsize)
	usedBytes := totalBytes - availableBytes

	// Calculate free percentage
	var freePercent float64
	if totalBytes > 0 {
		freePercent = (float64(availableBytes) / float64(totalBytes)) * 100
	}

	return &diskInfo{
		TotalBytes:     totalBytes,
		AvailableBytes: availableBytes,
		UsedBytes:      usedBytes,
		FreePercent:    freePercent,
	}, nil
}
