package gitea

import (
	"net/http"
	"testing"

	"kc/kernel"
)

func TestAPIErrorPreservesStatusAndClassifiesTransientFailure(t *testing.T) {
	err := &apiError{Method: http.MethodGet, Path: "/api/v1/repos/acme/core", Status: http.StatusBadGateway}
	if statusOf(err) != http.StatusBadGateway {
		t.Fatalf("status was lost: %d", statusOf(err))
	}
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("transient Gitea response was not typed: %v", err)
	}
}
