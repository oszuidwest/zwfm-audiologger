//go:build linux

package utils

import (
	"fmt"
	"syscall"
)

// AvailableDiskBytes returns bytes available to unprivileged users on the
// filesystem containing the given path.
func AvailableDiskBytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	if stat.Bsize < 0 {
		return 0, fmt.Errorf("statfs %s returned negative block size %d", path, stat.Bsize)
	}

	blockSize := uint64(stat.Bsize)

	return availableBytes(path, stat.Bavail, blockSize)
}
