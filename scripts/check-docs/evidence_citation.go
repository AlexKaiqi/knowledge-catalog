package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// A validation run id names a directory under .validation/runs/, which is
// gitignored and never checked in. A design document that cites one is pointing
// at something no reader can open: the run is pruned by retention windows and
// the evidence is gone permanently. Design documents state the conclusion and
// the contract that enforces it; run ids belong in task records, where the
// surrounding text also carries the date and scope that make a dead id readable
// as history. Task files are not scanned for this reason — their ids are
// historical annotations, and tasks/done/ is not rewritten.
var citedRunID = regexp.MustCompile(`\b\d{8}T\d{6}Z-[0-9a-f]{12}\b`)

func validateNoCitedRunIDs(root string, docs []string) error {
	var violations []string
	for _, doc := range docs {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(doc)))
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(raw), "\n") {
			ids := citedRunID.FindAllString(line, -1)
			if len(ids) == 0 {
				continue
			}
			sort.Strings(ids)
			violations = append(violations, fmt.Sprintf("%s: cites validation run %s (%s)",
				doc, strings.Join(ids, ", "), strings.TrimSpace(line)))
		}
	}
	if len(violations) > 0 {
		return fmt.Errorf("design documents cite validation runs that are never checked in; state the conclusion instead:\n  %s",
			strings.Join(violations, "\n  "))
	}
	return nil
}
