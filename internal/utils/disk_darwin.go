//go:build darwin

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

	return availableBytes(path, stat.Bavail, uint64(stat.Bsize))
}
