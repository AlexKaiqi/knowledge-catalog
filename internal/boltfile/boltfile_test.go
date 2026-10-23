package boltfile

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
	bolterrors "go.etcd.io/bbolt/errors"

	"kc/kernel"
)

func openForTest(t *testing.T, path string, options *bolt.Options) *Handle {
	t.Helper()
	handle, err := Open(path, 0o600, options)
	if err != nil {
		t.Fatalf("open %s: %v", filepath.Base(path), err)
	}
	if err := handle.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("values"))
		return err
	}); err != nil {
		_ = handle.Close()
		t.Fatalf("initialize %s: %v", filepath.Base(path), err)
	}
	return handle
}

func writeValue(t *testing.T, handle *Handle, value string) {
	t.Helper()
	if err := handle.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("values")).Put([]byte("k"), []byte(value))
	}); err != nil {
		t.Fatalf("write value: %v", err)
	}
}

func readValue(t *testing.T, handle *Handle) string {
	t.Helper()
	var value string
	if err := handle.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("values"))
		if bucket == nil {
			return errors.New("values bucket is missing")
		}
		value = string(bucket.Get([]byte("k")))
		return nil
	}); err != nil {
		t.Fatalf("read value: %v", err)
	}
	return value
}

// A ledger that opens per operation must not race itself: concurrent callers of
// one file inside one process all complete, and none of them is told that the
// request was malformed.
func TestConcurrentOpensOfOneLedgerComplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.db")
	seed := openForTest(t, path, &bolt.Options{Timeout: time.Second})
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed handle: %v", err)
	}

	const workers, rounds = 12, 8
	var wg sync.WaitGroup
	errs := make(chan error, workers*rounds)
	start := make(chan struct{})
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for round := 0; round < rounds; round++ {
				handle, err := Open(path, 0o600, &bolt.Options{Timeout: time.Second})
				if err != nil {
					errs <- err
					continue
				}
				if err := handle.View(func(tx *bolt.Tx) error {
					if tx.Bucket([]byte("values")) == nil {
						return errors.New("values bucket is missing")
					}
					return nil
				}); err != nil {
					errs <- err
				}
				if err := handle.Close(); err != nil {
					errs <- err
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if code := kernel.CodeOf(err); code == kernel.ErrUsageInvalid {
			t.Fatalf("concurrent open reported a request defect: %v", err)
		}
		t.Fatalf("concurrent open failed: %v", err)
	}
}

// Readers are the common case, and one reader must not wait behind another: a
// second read-only open reuses the handle the first one already holds.
func TestReadersShareOneOpenHandle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.db")
	first := openForTest(t, path, &bolt.Options{Timeout: time.Second})
	defer func() { _ = first.Close() }()

	second, err := Open(path, 0o600, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		t.Fatalf("second reader: %v", err)
	}
	defer func() { _ = second.Close() }()
	if second.DB != first.DB {
		t.Fatal("second reader opened the file again instead of sharing the process handle")
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first reader: %v", err)
	}
	// The second reader still owns its share, so the database stays open.
	writeValue(t, second, "after-first-close")
	if got := readValue(t, second); got != "after-first-close" {
		t.Fatalf("shared handle lost its values: %q", got)
	}
}

// A write needs the file to itself. While a reader holds a handle the write
// waits, and it succeeds once that reader leaves: a short wait is not a failure.
func TestWriterWaitsForReadersAndThenSucceeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.db")
	reader := openForTest(t, path, &bolt.Options{Timeout: time.Second})

	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		<-release
		if err := reader.Close(); err != nil {
			done <- err
			return
		}
		done <- nil
	}()
	time.AfterFunc(50*time.Millisecond, func() { close(release) })

	writer, err := Open(path, 0o600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("writer while a reader held the file: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("close reader: %v", err)
	}
	writeValue(t, writer, "from-writer")
	if got := readValue(t, writer); got != "from-writer" {
		t.Fatalf("writer handle has %q", got)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
}

// Waiting past the caller's own budget is a transient condition. The same
// request is safe to retry, so the answer must never be USAGE_INVALID.
func TestOpenThatOutlastsItsBudgetIsRetryable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.db")
	holder := openForTest(t, path, &bolt.Options{Timeout: time.Second})

	_, err := Open(path, 0o600, &bolt.Options{Timeout: 20 * time.Millisecond})
	if err == nil {
		t.Fatal("second writer succeeded while the first handle still owns the file")
	}
	if code := kernel.CodeOf(err); code != kernel.ErrTemporaryUnavailable {
		t.Fatalf("waiting open returned %q, want %q: %v", code, kernel.ErrTemporaryUnavailable, err)
	}
	if err := holder.Close(); err != nil {
		t.Fatalf("close holder: %v", err)
	}

	retried, err := Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		t.Fatalf("retry after release: %v", err)
	}
	if err := retried.Close(); err != nil {
		t.Fatalf("close retried handle: %v", err)
	}
}

// Only a lock timeout is transient. A missing database is a real failure to
// report, so it keeps its own identity instead of becoming retryable.
func TestClassifyKeepsNonTimeoutFailures(t *testing.T) {
	timeout := classify("ledger.db", bolterrors.ErrTimeout)
	if code := kernel.CodeOf(timeout); code != kernel.ErrTemporaryUnavailable {
		t.Fatalf("classified lock timeout as %q, want %q", code, kernel.ErrTemporaryUnavailable)
	}
	missing := classify("ledger.db", os.ErrNotExist)
	if code := kernel.CodeOf(missing); code != "" {
		t.Fatalf("classified a missing file as %q, want it left alone: %v", code, missing)
	}
	if !errors.Is(missing, os.ErrNotExist) {
		t.Fatalf("classified a missing file into %v", missing)
	}
}

// A failed open must not retain this process's entry for that path, otherwise a
// transient failure would block every later attempt forever.
func TestFailedOpenReleasesItsEntry(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "missing", "ledger.db")
	if _, err := Open(path, 0o600, &bolt.Options{Timeout: 50 * time.Millisecond}); err == nil {
		t.Fatal("opening a database under a missing directory succeeded")
	}
	if _, err := Open(path, 0o600, &bolt.Options{Timeout: 50 * time.Millisecond}); err != nil {
		if code := kernel.CodeOf(err); code == kernel.ErrTemporaryUnavailable {
			t.Fatalf("a failed open kept the path reserved: %v", err)
		}
	}
	if count := len(entries); count != 0 {
		t.Fatalf("entries retained after a failed open: %d", count)
	}
}

// Serving one file per observation revision must not accumulate one entry per
// revision: an entry is forgotten once its last share and waiter leave.
func TestEntriesAreForgottenAfterRelease(t *testing.T) {
	root := t.TempDir()
	for round := 0; round < 5; round++ {
		path := filepath.Join(root, "revision-"+string(rune('a'+round))+".db")
		handle := openForTest(t, path, &bolt.Options{Timeout: time.Second})
		if err := handle.Close(); err != nil {
			t.Fatalf("close %s: %v", path, err)
		}
	}
	if count := len(entries); count != 0 {
		t.Fatalf("entries retained after every handle closed: %d", count)
	}
}
