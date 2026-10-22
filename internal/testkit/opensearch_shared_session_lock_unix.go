//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package testkit

import (
	"os"
	"path/filepath"
	"syscall"
)

// sharedSearchSessionLockSupported reports whether this platform can coordinate
// the packages of one run against a shared instance.
const sharedSearchSessionLockSupported = true

// lockSharedSearchSession serializes session state changes across processes.
// The lock file is never removed: a new inode would let two processes each
// believe they hold the session.
func lockSharedSearchSession(dir string) (func(), error) {
	file, err := os.OpenFile(filepath.Join(dir, sharedSearchSessionLockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}

// tryLockExclusive reports whether this process took the lease, rather than
// failing: a held lease means its owner is still running.
func tryLockExclusive(file *os.File) (bool, error) {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	switch err {
	case nil:
		return true, nil
	case syscall.EWOULDBLOCK:
		return false, nil
	default:
		return false, err
	}
}
