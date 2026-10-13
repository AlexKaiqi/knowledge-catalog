package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifiedGatewayKeepsMachineTransportAndRejectsCrossOriginBrowser(t *testing.T) {
	for _, row := range []struct {
		name     string
		headers  http.Header
		rejected bool
	}{
		{"machine", http.Header{"X-Tai-Identity": {"verified-by-gateway"}, "Content-Type": {"application/json"}}, false},
		{"same-origin browser", http.Header{"X-Tai-Identity": {"verified-by-gateway"}, "Sec-Fetch-Site": {"same-origin"}, "X-Kc-Console": {"1"}, "Content-Type": {"application/json"}}, false},
		{"cross-origin browser", http.Header{"X-Tai-Identity": {"verified-by-gateway"}, "Sec-Fetch-Site": {"cross-site"}, "Content-Type": {"application/json"}}, true},
		{"older browser form", http.Header{"X-Tai-Identity": {"verified-by-gateway"}, "Origin": {"https://untrusted.test"}, "Content-Type": {"application/x-www-form-urlencoded"}}, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/snapshot/v1/datasets", nil)
			r.Header = row.headers
			if (ValidateLakeFSConsoleRequest(r) != nil) != row.rejected {
				t.Fatalf("unexpected request provenance result for %s", row.name)
			}
		})
	}
}
