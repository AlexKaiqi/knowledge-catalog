package jsonfile

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestLockedSerializesReadModifyWriteOnOneFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "counter.json")
	if err := Write(path, []int{}); err != nil {
		t.Fatal(err)
	}
	const writers = 32
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(value int) {
			defer wg.Done()
			errs <- Locked(path, func(absolute string) error {
				var values []int
				if err := Read(absolute, &values); err != nil {
					return err
				}
				return Write(absolute, append(values, value))
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var values []int
	if err := Read(path, &values); err != nil {
		t.Fatal(err)
	}
	if len(values) != writers {
		t.Fatalf("concurrent updates were lost: %d of %d kept", len(values), writers)
	}
}
