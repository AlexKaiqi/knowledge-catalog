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
//  2. client_secret stays on Server for token introspection and the fixed
//     OAuth2 token broker. The client receives user credentials only.
//  3. Snapshot provisioning uses the Store's separately configured service
//     credential; a user's KC token is not a universal downstream credential.
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
	// OAuth2Base is the deployment's Taihu OAuth2 base URL.
	OAuth2Base string `json:"oauth2_base"`
	Resource   string `json:"resource,omitempty"`
	Scope      string `json:"scope,omitempty"`
	AppName    string `json:"app_name,omitempty"`
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
	oauth2Base = strings.TrimRight(oauth2Base, "/")
	// Strip trailing /oauth2 if present
	oauth2Base = strings.TrimSuffix(oauth2Base, "/oauth2")

	return &ServiceIdentity{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Principal:    principal,
		OAuth2Base:   oauth2Base,
		Resource:     strings.TrimSpace(os.Getenv("KC_LOGIN_RESOURCE")),
		Scope:        strings.TrimSpace(os.Getenv("KC_LOGIN_SCOPE")),
		AppName:      strings.TrimSpace(os.Getenv("KC_LOGIN_APP_NAME")),
	}
}

func (s *ServiceIdentity) String() string {
	if s == nil {
		return "<no service identity>"
	}
	return fmt.Sprintf("ServiceIdentity{client_id=%s, principal=%s}", s.ClientID, s.Principal)
}
