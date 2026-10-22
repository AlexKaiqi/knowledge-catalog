package session

import (
	"context"
	"net/http"
	"strings"

	kcclient "kc/client/auth"
	"kc/kernel"
)

// remoteTokenAuthenticator sends only the credential. The server derives the
// principal from its trusted authenticator and rejects caller-supplied identity
// headers; the local Identity remains useful for CLI context and audit UX.
type remoteTokenAuthenticator struct{}

func (remoteTokenAuthenticator) Login(_ context.Context, request kcclient.LoginRequest) (kcclient.Session, error) {
	return kcclient.Session(request), nil
}

func (remoteTokenAuthenticator) Logout(context.Context, kcclient.Session) error { return nil }

func (remoteTokenAuthenticator) AuthenticateRequest(_ context.Context, session kcclient.Session, _ string, request *http.Request) error {
	request.Header.Set("Authorization", session.Authentication.Authorization)
	return nil
}

func discoverServerAuth(ctx context.Context, server string) (kcclient.AuthDiscovery, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := kcclient.New(kcclient.Config{BaseURL: server})
	if err != nil {
		return kcclient.AuthDiscovery{}, err
	}
	discovery, err := client.IdentityService().Discover(ctx)
	if err != nil {
		return kcclient.AuthDiscovery{}, err
	}
	if strings.TrimSpace(discovery.Mode) == "" {
		return kcclient.AuthDiscovery{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "kc server auth discovery omitted mode")
	}
	return discovery, nil
}

func bearerParts(raw string) (header, access string) {
	raw = strings.TrimSpace(raw)
	fields := strings.Fields(raw)
	if len(fields) == 2 && strings.EqualFold(fields[0], "bearer") {
		return "Bearer " + fields[1], fields[1]
	}
	return "Bearer " + raw, raw
}
