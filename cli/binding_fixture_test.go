package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"kc/internal/bindingfixture"
)

// putStateBinding declares one State-backed aspect through the CLI: it
// publishes a ResourceDescriptor naming origin, then a binding unit at
// objectID/aspect that carries no value and names that descriptor.
func putStateBinding(t *testing.T, home, repositoryID, objectID, aspect, schemaRef, origin string) string {
	t.Helper()
	descriptor := "resource/" + strings.ReplaceAll(objectID, "/", "-") + "-" + aspect
	raw, err := json.Marshal(bindingfixture.Descriptor(origin))
	if err != nil {
		t.Fatal(err)
	}
	body(t, kc(home, "put", testObjectReference(t, repositoryID, descriptor, ""), "--repo", repositoryID, "--command-id", "descriptor-"+descriptor, "--value", string(raw)))
	source := `{"kind":"binding","binding":{"descriptorRef":"` + descriptor + `"}}`
	body(t, kc(home, "put", testObjectReference(t, repositoryID, objectID, "/"+aspect), "--repo", repositoryID, "--command-id", "binding-"+objectID+"-"+aspect, "--schema-ref", schemaRef, "--value-source", source))
	return descriptor
}
