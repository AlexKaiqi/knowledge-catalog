//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package testkit

import (
	"fmt"
	"os"
)

// sharedSearchSessionLockSupported reports whether this platform can coordinate
// the packages of one run against a shared instance. Without flock two packages
// cannot tell whether a sibling is still asserting, and reclaiming on a guess
// deletes live indices, so the reclaim is left to the group entries that run one
// package set per guarded process.
const sharedSearchSessionLockSupported = false

func lockSharedSearchSession(_ string) (func(), error) {
	return nil, fmt.Errorf("shared search session locking is unavailable on this platform")
}

func tryLockExclusive(_ *os.File) (bool, error) {
	return false, fmt.Errorf("shared search session locking is unavailable on this platform")
}
