package maintenance

import (
	"path"
	"strings"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// FileScopePager is the native-authority equivalent of directory traversal.
// It enumerates identities, not bodies, from selected file paths only.
type FileScopePager interface {
	FileScopeObjectIDs(kernel.CommitID, []string, []string, ScanRequest) (knowledge.ObjectIDPage, error)
}

func WalkScope(store snapshot.Store, repo knowledge.Repository, commit kernel.CommitID, prefixes, files []string, visit func(knowledge.ObjectID) error) error {
	for _, prefix := range prefixes {
		if prefix == "" {
			// Whole-repository publication explicitly permits full export,
			// including native authorities with no literal file representation.
			return WalkRepository(repo, commit, func(value knowledge.KnowledgeValue) error { return visit(value.Address.ObjectID) })
		}
	}
	if pager, ok := repo.(FileScopePager); ok {
		request := ScanRequest{Limit: DefaultScanLimit}
		for {
			page, err := pager.FileScopeObjectIDs(commit, prefixes, files, request)
			if err != nil {
				return err
			}
			for _, id := range page.ObjectIDs {
				if err := visit(id); err != nil {
					return err
				}
			}
			if page.Exhausted {
				return nil
			}
			if page.Continuation == "" || page.Continuation == request.Continuation {
				return kernel.Fail(kernel.ErrPreconditionFailed, "non-advancing scoped identity page")
			}
			request.Continuation = page.Continuation
		}
	}
	return WalkFileScope(store, commit, prefixes, files, visit)
}

// WalkFileScope locates identities only inside selected source subtrees during
// explicit export. It never lists or scans the rest of a large repository.
// Callers must check every unit of an identity before hydrating the object.
func WalkFileScope(store snapshot.Store, commit kernel.CommitID, prefixes, files []string, visit func(knowledge.ObjectID) error) error {
	tree, ok := snapshot.TreeReaderOf(store)
	if !ok {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "scoped export requires file access")
	}
	directory, ok := snapshot.DirectoryReaderOf(store)
	if !ok {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "scoped export requires directory paging")
	}
	seenFiles := map[string]bool{}
	seenIDs := map[knowledge.ObjectID]bool{}
	read := func(file string) error {
		if seenFiles[file] || file == ".kc" || strings.HasPrefix(file, ".kc/") {
			return nil
		}
		seenFiles[file] = true
		raw, err := tree.ReadFile(file, commit)
		if err != nil {
			return err
		}
		unit := repofile.Parse(string(raw))
		if unit == nil {
			return nil
		}
		if err := unit.DeclarationError(); err != nil {
			return err
		}
		if seenIDs[unit.ObjectID] {
			return nil
		}
		seenIDs[unit.ObjectID] = true
		return visit(unit.ObjectID)
	}
	for _, file := range files {
		if err := read(file); err != nil {
			return err
		}
	}
	seenDirs := map[string]bool{}
	var walk func(string) error
	walk = func(dir string) error {
		if seenDirs[dir] || dir == ".kc" || strings.HasPrefix(dir, ".kc/") {
			return nil
		}
		seenDirs[dir] = true
		request := snapshot.DirectoryRequest{Commit: commit, Directory: dir, Limit: 500}
		for {
			page, err := directory.ReadDirectory(request)
			if err != nil {
				return err
			}
			if page.Generation != string(commit) {
				return kernel.Fail(kernel.ErrPreconditionFailed, "export directory basis changed")
			}
			for _, entry := range page.Entries {
				if entry.Name == "" || entry.Name == "." || entry.Name == ".." || strings.ContainsAny(entry.Name, "/\\") {
					return kernel.Fail(kernel.ErrPreconditionFailed, "invalid directory entry")
				}
				file := path.Join(dir, entry.Name)
				if entry.Kind == "directory" {
					err = walk(file)
				} else {
					err = read(file)
				}
				if err != nil {
					return err
				}
			}
			if page.Exhausted {
				return nil
			}
			if page.Continuation == "" || page.Continuation == request.Continuation {
				return kernel.Fail(kernel.ErrPreconditionFailed, "non-advancing directory page")
			}
			request.Continuation = page.Continuation
		}
	}
	for _, prefix := range prefixes {
		if err := walk(prefix); err != nil {
			return err
		}
	}
	return nil
}
