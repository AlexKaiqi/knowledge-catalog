package repofile

import (
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// AssertPublicationFiles checks a fixed publication's literal file boundary.
// Writer control records are not content; every other path must be expected.
// This is an explicit publication/restore check, never a consumer fallback.
func AssertPublicationFiles(tree snapshot.TreeReader, commit kernel.CommitID, operations []knowledge.Operation) error {
	expected := make(map[string]bool, len(operations))
	for _, operation := range operations {
		expected[operation.PathHint] = true
	}
	files, err := tree.ListFiles(commit)
	if err != nil {
		return err
	}
	for _, file := range files {
		if strings.HasPrefix(file, ".kc/") {
			continue
		}
		if !expected[file] {
			return kernel.Fail(kernel.ErrPreconditionFailed, "unexpected publication file %s", file)
		}
		delete(expected, file)
	}
	if len(expected) != 0 {
		return kernel.Fail(kernel.ErrPreconditionFailed, "publication is missing %d files", len(expected))
	}
	return nil
}
