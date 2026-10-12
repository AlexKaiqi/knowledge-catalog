package server

import (
	"fmt"
	"kc/identity"
	inv "kc/invocation"
	"kc/invoke"
	store "kc/snapshot/service"
	"net/http"
	"strings"
)

func newLakeFSAuthenticator(origin string, client *http.Client) (identity.HTTPAuthenticator, error) {
	return store.NewLakeFSAuthenticator(origin, client)
}
func validateLakeFSConsoleRequest(r *http.Request) error {
	return store.ValidateLakeFSConsoleRequest(r)
}
func init() {
	inv.RegisterAuthenticator("lakefs", func(flags map[string]FlagValue) (inv.HTTPAuthenticator, error) {
		origin := strings.TrimSpace(invoke.FlagString(flags, "auth-url"))
		if origin == "" {
			return nil, fmt.Errorf("--auth-url is required")
		}
		return newLakeFSAuthenticator(origin, nil)
	})
}
