package taihu

import (
	"fmt"
	"os"
	"strings"

	"kc/identity"
)

// Name is the `--auth` value of this authenticator.
const Name = "taihu"

func setting(settings identity.Settings, name, env string) string {
	value := strings.TrimSpace(settings.String(name))
	if value == "" && env != "" {
		value = strings.TrimSpace(os.Getenv(env))
	}
	return value
}

// factory reads `--auth-url` (introspection issuer), `--auth-hmac-secret` or
// KC_TAIHU_HMAC_SECRET (gateway identity), and the optional login application
// (`--service-client-id`, `--service-client-secret` or KC_SERVICE_CLIENT_SECRET,
// `--service-principal`, KC_LOGIN_RESOURCE/SCOPE/APP_NAME).
func factory(settings identity.Settings) (identity.HTTPAuthenticator, error) {
	base := strings.TrimSuffix(strings.TrimRight(setting(settings, "auth-url", ""), "/"), "/oauth2")
	if base != "" && !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return nil, fmt.Errorf("--auth-url must be a Taihu http(s) origin")
	}
	clientID := setting(settings, "service-client-id", "")
	clientSecret := setting(settings, "service-client-secret", "KC_SERVICE_CLIENT_SECRET")
	a, err := New(setting(settings, "auth-hmac-secret", "KC_TAIHU_HMAC_SECRET"), base, clientID, clientSecret, nil)
	if err != nil {
		return nil, err
	}
	if clientID != "" {
		a.principal = setting(settings, "service-principal", "")
		if a.principal == "" {
			a.principal = "taihu:service:" + clientID
		}
		a.broker = NewBroker(Application{
			ClientID: clientID, ClientSecret: clientSecret, OAuth2Base: base,
			Resource: strings.TrimSpace(os.Getenv("KC_LOGIN_RESOURCE")),
			Scope:    strings.TrimSpace(os.Getenv("KC_LOGIN_SCOPE")),
			AppName:  strings.TrimSpace(os.Getenv("KC_LOGIN_APP_NAME")),
		}, nil)
	}
	return a, nil
}

func init() { identity.Register(Name, factory) }
