package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"kc/kernel"
)

func TestDecodeJSONBodyRejectsOversizedRequest(t *testing.T) {
	request := &http.Request{Body: io.NopCloser(strings.NewReader(strings.Repeat(" ", maxRequestBodyBytes+1)))}
	if _, err := decodeJSONBody(request); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("oversized request code=%s err=%v", kernel.CodeOf(err), err)
	}
}
