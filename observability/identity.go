package observability

import (
	"fmt"
	"strings"
	"unicode"
)

// Authenticator turns a transport-specific assertion into the identity context
// used by authorization and evidence.
type Authenticator interface {
	Authenticate(IdentityAssertion) (IdentityContext, error)
}

type IdentityAssertion struct {
	Principal  string
	OnBehalfOf string
}

// PassThroughAuthenticator validates shape but does not prove identity or delegation.
type PassThroughAuthenticator struct{}

func (PassThroughAuthenticator) Authenticate(assertion IdentityAssertion) (IdentityContext, error) {
	context := IdentityContext{Principal: assertion.Principal, OnBehalfOf: assertion.OnBehalfOf}
	return context, context.Validate()
}

type IdentityContext struct {
	Principal  string `json:"principal"`
	OnBehalfOf string `json:"onBehalfOf,omitempty"`
}

func (c IdentityContext) Validate() error {
	if err := validateIdentity("principal", c.Principal, true); err != nil {
		return err
	}
	return validateIdentity("onBehalfOf", c.OnBehalfOf, false)
}

func validateIdentity(name, value string, required bool) error {
	if value == "" {
		if required {
			return fmt.Errorf("%s is required", name)
		}
		return nil
	}
	if strings.TrimSpace(value) != value || len(value) > 256 {
		return fmt.Errorf("%s must be a trimmed identity of at most 256 bytes", name)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s must not contain control characters", name)
		}
	}
	return nil
}
