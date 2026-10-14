package handler

import (
	"context"
	"sync"

	"kc/kernel"
	objectmodel "kc/object"
)

// InterfaceHandler is an independent capability implementation. It receives
// validated declarations; business clients and credentials remain outside core.
type InterfaceHandler interface {
	ValidateImplementation(context.Context, ResolvedInterface) error
	Execute(context.Context, InterfaceCall) (InterfaceResult, error)
}

// InterfaceRegistration binds one handler release to exact immutable object
// contracts and implementation schema structures. Field access declarations do
// not affect support matching. Supporting another implementation form adds a
// registration rather than a business type branch to dispatch.
type InterfaceRegistration struct {
	Contract                            objectmodel.PinnedObjectRef
	ContractDigest                      kernel.Digest
	ImplementationSchemaStructureDigest kernel.Digest
	HandlerRevision                     string
	Handler                             InterfaceHandler
}

// InterfaceRegistry is deployment wiring, not another object authority.
// Registration is synchronized and rejects ambiguous support.
type InterfaceRegistry struct {
	mu            sync.RWMutex
	registrations []InterfaceRegistration
}

func NewInterfaceRegistry() *InterfaceRegistry { return &InterfaceRegistry{} }

// Snapshot creates request-local registration state. Capability dispatch never
// mutates a deployment registry while selecting a default handler.
func (r *InterfaceRegistry) Snapshot() *InterfaceRegistry {
	copy := NewInterfaceRegistry()
	if r == nil {
		return copy
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	copy.registrations = append(copy.registrations, r.registrations...)
	return copy
}

func (r *InterfaceRegistry) Resolve(implementation ResolvedInterface) (InterfaceRegistration, error) {
	return r.resolve(implementation)
}

func (r *InterfaceRegistry) Register(registration InterfaceRegistration) error {
	if r == nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin handler registry is unavailable")
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
	return objectmodel.FormatPinnedRef(r.Contract.Repository, r.Contract.Commit, r.Contract.Object) + "|" + string(r.ContractDigest) + "|" + string(r.ImplementationSchemaStructureDigest)
}

func (r *InterfaceRegistry) resolve(implementation ResolvedInterface) (InterfaceRegistration, error) {
	if r == nil {
		return InterfaceRegistration{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin handler registry is unavailable")
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
