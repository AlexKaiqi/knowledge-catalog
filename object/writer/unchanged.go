package writer

import (
	"bytes"

	"kc/kernel"
	"kc/snapshot"
)

// Compare every planned literal effect, including paths, declaration headers,
// provenance and derived locators. A local-value digest cannot prove this.
func treeEffectsUnchanged(basis snapshot.TreeReader, at kernel.CommitID, changes []snapshot.TreeChange) (bool, error) {
	if at == "" {
		return false, nil
	}
	for _, change := range changes {
		before, err := basis.ReadFile(change.Path, at)
		if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
			if change.Remove {
				continue
			}
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if change.Remove || !bytes.Equal(before, change.Content) {
			return false, nil
		}
	}
	return true, nil
}
