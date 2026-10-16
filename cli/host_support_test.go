package cli

import (
	"context"
	"encoding/json"
	"fmt"
	localauth "kc/identity/local"
	"net/http"
	"net/http/httptest"

	"kc/identity"
	inv "kc/invocation"
	"kc/kernel"
	"kc/snapshot/access"
	snapshotclient "kc/snapshot/client"
	snapshotservice "kc/snapshot/service"
)

// The embedded fixture reports missing transports without supplying another
// implementation of the Snapshot product commands.
func snapshotServerRequired(_ *inv.Invocation) (any, error) {
	return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "this Snapshot command requires a KC Server; pass --server or set KC_SERVER_URL")
}

type admissionRuleStore struct{ home string }

func (s admissionRuleStore) ReadRules(context.Context) ([]access.Rule, error) {
	file, err := inv.ReadAllow(s.home)
	return file.Rules, err
}

func (admissionRuleStore) UpdateRules(context.Context, func([]access.Rule) ([]access.Rule, error)) error {
	return kernel.Fail(kernel.ErrForbidden, "admission test policy is read-only")
}

// Embedded tests use the real module HTTP implementation for the same
// admission result as production; this adapter contains no grant shaping.
func admissionOverview(cx *inv.Invocation) (snapshotclient.AdmissionResult, error) {
	requestURL := ""
	if cx.WS != nil && cx.WS.Deployment != nil && cx.WS.Deployment.Admission != nil {
		requestURL = cx.WS.Deployment.Admission.RequestURL
	}
	handler := snapshotservice.NewHandler(snapshotservice.Config{
		Identity: snapshotservice.IdentityOptions{Mode: "local", Authenticator: localauth.New()},
		Acquire: func(context.Context, identity.HTTPIdentity, string) (snapshotservice.Resources, func(), error) {
			return snapshotservice.Resources{Access: access.NewService(admissionRuleStore{home: cx.Home}), RequestURL: requestURL}, func() {}, nil
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/identity/v1/admission", nil)
	request.Header.Set("X-Kc-As", cx.Flag("as"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var result snapshotclient.AdmissionResult
	if response.Code != http.StatusOK {
		return result, fmt.Errorf("admission returned HTTP %d: %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		return result, err
	}
	return result, nil
}
