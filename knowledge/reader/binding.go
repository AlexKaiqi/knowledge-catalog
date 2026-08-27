package reader

import (
	"strings"

	"kc/kernel"
	"kc/knowledge"
)

// ResolvedBinding is the complete stable access declaration resolved at one
// Repository commit. It contains no endpoint, credential or observation value.
type ResolvedBinding struct {
	Repository        kernel.RepositoryID                   `json:"repository"`
	DeclarationCommit kernel.CommitID                       `json:"declarationCommit"`
	Address           knowledge.Address                     `json:"address"`
	DeclarationDigest kernel.Digest                         `json:"declarationDigest"`
	Mode              knowledge.BindingMode                 `json:"mode"`
	Runtime           string                                `json:"runtime"`
	Protocol          string                                `json:"protocol"`
	Operations        map[string]knowledge.BindingOperation `json:"operations"`
	DescriptorRef     knowledge.ObjectID                    `json:"descriptorRef,omitempty"`
	DescriptorDigest  kernel.Digest                         `json:"descriptorDigest,omitempty"`
}

func (r *Reader) ResolveBinding(repositoryID kernel.RepositoryID, commit kernel.CommitID, address knowledge.Address) (ResolvedBinding, error) {
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return ResolvedBinding{}, err
	}
	return ResolveRepoBinding(repo, commit, address)
}

func ResolveRepoBinding(repo knowledge.Repository, commit kernel.CommitID, address knowledge.Address) (ResolvedBinding, error) {
	resolution, err := repo.ResolveAddress(address, commit)
	if err != nil {
		return ResolvedBinding{}, err
	}
	if resolution.Status != knowledge.StatusResolved {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "binding address %s is not resolved at %s", knowledge.AddressKey(address), commit)
	}
	source := resolution.ValueSource
	if source == nil || source.Kind != knowledge.ValueSourceBinding || source.Binding == nil {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "address %s has no binding value_source", knowledge.AddressKey(address))
	}
	if err := knowledge.ValidateValueSource(source); err != nil {
		return ResolvedBinding{}, err
	}
	binding := source.Binding
	out := ResolvedBinding{
		Repository:        repo.ID(),
		DeclarationCommit: commit,
		Address:           address,
		DeclarationDigest: resolution.DeclarationDigest,
		Mode:              binding.Mode,
		Runtime:           binding.Runtime,
		Protocol:          binding.Protocol,
		Operations:        copyOperations(binding.Operations),
		DescriptorRef:     binding.DescriptorRef,
	}
	if binding.DescriptorRef == "" {
		return out, nil
	}
	descriptor, err := repo.Read(binding.DescriptorRef, commit)
	if err != nil {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "descriptor %s is not resolved at %s", binding.DescriptorRef, commit)
	}
	value, ok := descriptor.Value.(map[string]any)
	if ok && value["kind"] != "ResourceDescriptor" {
		if definition, nested := value["definition"].(map[string]any); nested {
			value = definition
		}
	}
	if !ok || value["kind"] != "ResourceDescriptor" {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrUsageInvalid, "descriptor %s must be a ResourceDescriptor", binding.DescriptorRef)
	}
	out.Runtime = strings.TrimSpace(asString(value["runtime"]))
	out.Protocol = strings.TrimSpace(asString(value["protocol"]))
	out.Operations, err = descriptorOperations(value["access"])
	if err != nil {
		return ResolvedBinding{}, err
	}
	if out.Runtime == "" || out.Protocol == "" || len(out.Operations) == 0 {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s requires runtime, protocol and access", binding.DescriptorRef)
	}
	out.DescriptorDigest = kernel.CanonicalDigest(value)
	return out, nil
}

func copyOperations(source map[string]knowledge.BindingOperation) map[string]knowledge.BindingOperation {
	out := make(map[string]knowledge.BindingOperation, len(source))
	for name, operation := range source {
		out[name] = operation
	}
	return out
}

func descriptorOperations(raw any) (map[string]knowledge.BindingOperation, error) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor access must be an object")
	}
	out := map[string]knowledge.BindingOperation{}
	for name, rawOperation := range obj {
		operation, ok := rawOperation.(map[string]any)
		if !ok {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor operation %s must be an object", name)
		}
		call := strings.TrimSpace(asString(operation["call"]))
		if strings.TrimSpace(name) == "" || call == "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor operations require non-empty names and calls")
		}
		out[name] = knowledge.BindingOperation{Call: call}
	}
	return out, nil
}
