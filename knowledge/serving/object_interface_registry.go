package serving

import (
	"context"
	"sync"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
)

// InterfaceHandler is an independent capability implementation. It receives
// validated declarations; business clients and credentials remain outside core.
type InterfaceHandler interface {
	ValidateImplementation(context.Context, reader.ResolvedInterface) error
	Execute(context.Context, InterfaceCall) (InterfaceResult, error)
}

// InterfaceRegistration binds one handler release to exact immutable knowledge
// contracts and implementation schema structures. Field access declarations do
// not affect support matching. Supporting another implementation form adds a
// registration rather than a business type branch to dispatch.
type InterfaceRegistration struct {
	Contract                            knowledge.PinnedKnowledgeRef
	ContractDigest                      kernel.Digest
	ImplementationSchemaStructureDigest kernel.Digest
	HandlerRevision                     string
	Handler                             InterfaceHandler
}

// InterfaceRegistry is deployment wiring, not another knowledge authority.
// Registration is synchronized and rejects ambiguous support.
type InterfaceRegistry struct {
	mu            sync.RWMutex
	registrations []InterfaceRegistration
}

func NewInterfaceRegistry() *InterfaceRegistry { return &InterfaceRegistry{} }

func (r *InterfaceRegistry) Register(registration InterfaceRegistration) error {
	if r == nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "object interface handler registry is unavailable")
	}
	if registration.Contract.Repository == "" || registration.Contract.Commit == "" || registration.Contract.Object == "" || registration.ContractDigest == "" || registration.ImplementationSchemaStructureDigest == "" || registration.HandlerRevision == "" || registration.Handler == nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "interface registration requires pinned contract, contract/schema structure digests, handler revision and handler")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.registrations {
		if registrationKey(existing) == registrationKey(registration) {
			return kernel.Fail(kernel.ErrUsageInvalid, "interface handler support is ambiguous for %s", registration.Contract.Object)
		}
	}
	r.registrations = append(r.registrations, registration)
	return nil
}

func registrationKey(r InterfaceRegistration) string {
	return knowledge.FormatPinnedRef(r.Contract.Repository, r.Contract.Commit, r.Contract.Object) + "|" + string(r.ContractDigest) + "|" + string(r.ImplementationSchemaStructureDigest)
}

func (r *InterfaceRegistry) resolve(implementation reader.ResolvedInterface) (InterfaceRegistration, error) {
	if r == nil {
		return InterfaceRegistration{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "object interface handler registry is unavailable")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := registrationKey(InterfaceRegistration{Contract: implementation.Contract, ContractDigest: implementation.ContractDigest, ImplementationSchemaStructureDigest: implementation.ImplementationSchemaStructureDigest})
	for _, registration := range r.registrations {
		if registrationKey(registration) == key {
			return registration, nil
		}
	}
	return InterfaceRegistration{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "no handler supports interface %s and implementation schema at this fixed basis", implementation.Contract.Object)
}
