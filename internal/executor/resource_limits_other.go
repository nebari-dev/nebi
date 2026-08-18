//go:build !unix

package executor

import "os"

type diskUsageTracker struct{}

func newDiskUsageTracker() *diskUsageTracker {
	return &diskUsageTracker{}
}

func (t *diskUsageTracker) size(info os.FileInfo) int64 {
	return info.Size()
}
