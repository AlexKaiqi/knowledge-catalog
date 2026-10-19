package writer

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	objectmodel "kc/object"
)

// ArchiveSubdirSeparator splits an archive path from the directory inside it
// that plays the repository root: `drafts.tar.gz//drafts`.
const ArchiveSubdirSeparator = "//"

// ingestReadConcurrency bounds concurrent file reads and parses of a
// directory. Opening each small file costs a round trip to the filesystem
// (and endpoint scanners on desktops); serial reads dominated large uploads.
var ingestReadConcurrency = func() int {
	n := runtime.GOMAXPROCS(0) * 2
	if n < 8 {
		n = 8
	}
	if n > 32 {
		n = 32
	}
	return n
}()

type ingestEntry struct {
	rel     string
	content []byte
	load    func() ([]byte, error)
}

type ingestedFile struct {
	op   objectmodel.Operation
	file IngestFile
}

// ingestSource lists the desired files in deterministic order. A directory is
// walked; a .tar/.tar.gz/.tgz file is streamed, optionally rooted at a
// directory inside it.
func ingestSource(input string) ([]ingestEntry, error) {
	archive, subdir, archived := splitArchiveInput(input)
	if archived {
		if info, err := os.Stat(archive); err == nil && info.Mode().IsRegular() {
			return archiveEntries(archive, subdir)
		}
		if subdir != "" {
			return nil, fmt.Errorf("%s is not a readable .tar, .tar.gz or .tgz archive", archive)
		}
	}
	return directoryEntries(input)
}

func splitArchiveInput(input string) (archive, subdir string, ok bool) {
	archive = input
	if i := strings.Index(input, ArchiveSubdirSeparator); i > 0 {
		archive, subdir = input[:i], strings.Trim(input[i+len(ArchiveSubdirSeparator):], "/")
	}
	lower := strings.ToLower(archive)
	for _, ext := range []string{".tar", ".tar.gz", ".tgz"} {
		if strings.HasSuffix(lower, ext) {
			return archive, subdir, true
		}
	}
	return input, "", false
}

func hiddenPath(rel string) bool {
	for _, part := range strings.Split(rel, "/") {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

func directoryEntries(dir string) ([]ingestEntry, error) {
	var entries []ingestEntry
	err := filepath.WalkDir(dir, func(full string, d fs.DirEntry, err error) error {
		if err != nil {
			// Report the walk failure in walk order, after every earlier file,
			// exactly where the former serial loop stopped.
			entries = append(entries, ingestEntry{load: func() ([]byte, error) { return nil, err }})
			return filepath.SkipAll
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() && full != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, full)
		if err != nil {
			return err
		}
		entries = append(entries, ingestEntry{rel: filepath.ToSlash(rel), load: func() ([]byte, error) { return os.ReadFile(full) }})
		return nil
	})
	return entries, err
}

func archiveEntries(archive, subdir string) ([]ingestEntry, error) {
	file, err := os.Open(archive)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var stream io.Reader = file
	if lower := strings.ToLower(archive); strings.HasSuffix(lower, ".gz") || strings.HasSuffix(lower, ".tgz") {
		gz, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", archive, err)
		}
		defer gz.Close()
		stream = gz
	}
	prefix := ""
	if subdir != "" {
		prefix = path.Clean(subdir) + "/"
	}
	seen := map[string]bool{}
	var entries []ingestEntry
	reader := tar.NewReader(stream)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", archive, err)
		}
		name := strings.TrimPrefix(header.Name, "./")
		if name == "" || name == "." {
			continue
		}
		clean := path.Clean(name)
		if path.IsAbs(name) || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("%s: entry %q escapes the archive root", archive, header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
		case tar.TypeXGlobalHeader:
			continue
		default:
			return nil, fmt.Errorf("%s: entry %q is not a regular file", archive, header.Name)
		}
		if !strings.HasPrefix(clean, prefix) {
			continue
		}
		rel := strings.TrimPrefix(clean, prefix)
		if hiddenPath(rel) {
			continue
		}
		if seen[rel] {
			return nil, fmt.Errorf("%s: entry %q appears more than once", archive, header.Name)
		}
		seen[rel] = true
		content, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", archive, err)
		}
		entries = append(entries, ingestEntry{rel: rel, content: content})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s has no files under %q", archive, subdir)
	}
	return entries, nil
}

// ingestEntries reads and parses entries with bounded concurrency. The
// result keeps entry order and the reported error is the first one in that
// order, exactly as a serial loop would report it.
func ingestEntries(entries []ingestEntry) ([]ingestedFile, error) {
	out := make([]ingestedFile, len(entries))
	errs := make([]error, len(entries))
	var next int64
	var failedAt atomic.Int64
	failedAt.Store(int64(len(entries)))
	workers := ingestReadConcurrency
	if workers > len(entries) {
		workers = len(entries)
	}
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for {
				i := int(atomic.AddInt64(&next, 1)) - 1
				if i >= len(entries) || int64(i) > failedAt.Load() {
					return
				}
				entry := entries[i]
				content := entry.content
				var err error
				if entry.load != nil {
					content, err = entry.load()
				}
				if err == nil {
					out[i].op, out[i].file, err = ingestFile(entry.rel, content)
				}
				if err != nil {
					errs[i] = err
					for {
						current := failedAt.Load()
						if int64(i) >= current || failedAt.CompareAndSwap(current, int64(i)) {
							break
						}
					}
				}
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
