package cli_test

import (
	"strings"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

func testObjectReference(t *testing.T, repository, id, locator string) string {
	t.Helper()
	reference, err := objectmodel.FormatReference(objectmodel.Reference{
		Repository: kernel.RepositoryID(repository), Object: objectmodel.ObjectID(id), Locator: locator,
	})
	if err != nil {
		t.Fatal(err)
	}
	return reference
}

func testObjectLocator(keys ...string) string {
	locator := ""
	for _, key := range keys {
		locator += "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
	}
	return locator
}
