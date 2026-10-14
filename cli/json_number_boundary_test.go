package cli

import (
	"strings"
	"testing"
)

func TestObjectCLIProductEncodingPreservesLargeInteger(t *testing.T) {
	result := encodeCLIProduct("read", nil, RunResult{Stdout: `{"reference":"urn:kc:object:r:sample%2FA","repository":"r","commit":"fixed","digest":"unit-digest","address":{"kind":"Entity","objectId":"sample/A"},"value":{"first":9007199254740992,"second":9007199254740993,"max":9223372036854775807}}`})
	for _, number := range []string{"9007199254740992", "9007199254740993", "9223372036854775807"} {
		if !strings.Contains(result.Stdout, number) {
			t.Fatalf("CLI product view rounded number %s: %s", number, result.Stdout)
		}
	}
}
