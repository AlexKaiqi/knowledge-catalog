package cli

import (
	"fmt"
	"os"
	"strings"
)

// ServiceIdentity represents the KC service's own Taihu application identity.
// Taihu IAM only supports authorization_code and refresh_token grants, so
// the service does NOT get a machine token. Instead:
//
//  1. The service's client_id is its identity — it's who the service is.
//  2. client_secret is used for token introspection (Basic Auth) and for
//     the OAuth2 login flow (kc login).
//  3. User tokens are forwarded directly to downstream services (Gitea,
//     MySQL runtime) — no token exchange needed.
//
// The service principal (e.g. "taihu:service:kc-prod") is used in allow.json
// and audit logs for service-to-service actions.
type ServiceIdentity struct {
	// ClientID is the Taihu-registered application identifier.
	ClientID string `json:"client_id"`
	// ClientSecret is the Taihu-registered application secret.
	ClientSecret string `json:"-" yaml:"-"`
	// Principal is the service's own identity string.
	Principal string `json:"principal"`
	// OAuth2Base is the Taihu OAuth2 base URL (default: http://iam.it.woa.com).
	OAuth2Base string `json:"oauth2_base"`
}

// ResolveServiceIdentity reads service identity from flags and environment.
// Returns nil when no service identity is configured.
func ResolveServiceIdentity(flags map[string]FlagValue) *ServiceIdentity {
	clientID := strings.TrimSpace(FlagString(flags, "service-client-id"))
	if clientID == "" {
		return nil
	}
	clientSecret := strings.TrimSpace(FlagString(flags, "service-client-secret"))
	if clientSecret == "" {
		clientSecret = strings.TrimSpace(os.Getenv("KC_SERVICE_CLIENT_SECRET"))
	}
	principal := strings.TrimSpace(FlagString(flags, "service-principal"))
	if principal == "" {
		principal = "taihu:service:" + clientID
	}
	oauth2Base := strings.TrimSpace(FlagString(flags, "auth-url"))
	if oauth2Base == "" {
		oauth2Base = "http://iam.it.woa.com"
	}
	oauth2Base = strings.TrimRight(oauth2Base, "/")
	// Strip trailing /oauth2 if present
	oauth2Base = strings.TrimSuffix(oauth2Base, "/oauth2")

	return &ServiceIdentity{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Principal:    principal,
		OAuth2Base:   oauth2Base,
	}
}

func (s *ServiceIdentity) String() string {
	if s == nil {
		return "<no service identity>"
	}
	return fmt.Sprintf("ServiceIdentity{client_id=%s, principal=%s}", s.ClientID, s.Principal)
}
