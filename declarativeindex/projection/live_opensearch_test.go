package projection_test

import (
	"kc/declarativeindex/internal/testfixture"
	index "kc/declarativeindex/projection"
	"testing"
)

func liveIndex(t *testing.T) *index.Index {
	t.Helper()
	idx := index.NewIndexEngine("", testfixture.LiveEngineOpener(t))
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}
