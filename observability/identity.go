package observability

import "kc/kernel"

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
	context := IdentityContext(assertion)
	return context, context.Validate()
}

// IdentityContext aliases the protocol-base evidence shape (B13 ruling A).
type IdentityContext = kernel.IdentityContext
