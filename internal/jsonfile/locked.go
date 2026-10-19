package jsonfile

import (
	"path/filepath"
	"sync"
)

// pathLocks holds one process-local mutex per absolute path. Atomic rename
// keeps each write whole, but a read-modify-write would still lose a
// concurrent update; callers that derive the next content from the current
// file hold the path lock across that whole step.
var pathLocks sync.Map

// Locked runs action while holding the process-local lock for path, passing
// the absolute path it locked. It serializes read-modify-write of one file
// only; unrelated files never wait on each other.
func Locked(path string, action func(absolute string) error) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	lock, _ := pathLocks.LoadOrStore(absolute, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	return action(absolute)
}
