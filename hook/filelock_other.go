//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package hook

import "os"

// The process-local lock remains active on platforms without syscall.Flock.
// Atomic replacement still prevents a partially written retry file.
func lockFile(_ *os.File) (func(), error) {
	return func() {}, nil
}

func syncDirectory(_ string) error { return nil }
