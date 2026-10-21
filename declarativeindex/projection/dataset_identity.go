package projection

import (
	"fmt"
	"sort"
	"strings"

	retrieval "kc/discovery"
	"kc/kernel"
	objectmodel "kc/object"
)

// datasetCheck extends the instance manifest check with the frozen binding
// identities of the revision (doc/design/liveaccess.md: only shared external
// values enter an index). A member whose input Schema is supplied by a
// per-caller or uninterpretable binding unit is unsupported as a whole, with
// those units in its reason; the revision's other members still build. The
// excluded units are never observed for the index. Preparation, overview and
// search derive the same check from the same pin.
func (idx *Index) datasetCheck(s datasetSpace) (retrieval.DeclarationCheck, map[string]bool, error) {
	identities, err := s.serving.BindingIdentities()
	if err != nil {
		return nil, nil, err
	}
	excluded := map[objectmodel.ObjectID][]string{}
	units := map[string]bool{}
	for _, binding := range identities {
		unit := string(binding.Repository) + ":" + objectmodel.AddressKey(binding.Address)
		label := string(binding.Repository) + ":" + objectmodel.AddressLabel(binding.Address)
		var reason string
		switch {
		case binding.Reason != "":
			reason = fmt.Sprintf("unit %s cannot be interpreted: %s", label, binding.Reason)
		case binding.Identity != objectmodel.IdentityShared:
			reason = fmt.Sprintf("unit %s uses %s ResourceDescriptor %s", label, binding.Identity, binding.Descriptor)
		default:
			continue
		}
		excluded[binding.Schema] = append(excluded[binding.Schema], reason)
		units[unit] = true
	}
	base := idx.check
	if len(excluded) == 0 {
		return base, units, nil
	}
	return func(declaration objectmodel.IndexDeclaration, inputs []retrieval.DeclarationInput) string {
		if base != nil {
			if reason := base(declaration, inputs); reason != "" {
				return reason
			}
		}
		schemas := map[objectmodel.ObjectID]bool{}
		var reasons []string
		for _, input := range inputs {
			if schemas[input.Field.Schema] {
				continue
			}
			schemas[input.Field.Schema] = true
			reasons = append(reasons, excluded[input.Field.Schema]...)
		}
		if len(reasons) == 0 {
			return ""
		}
		sort.Strings(reasons)
		return "only shared external values can enter a Dataset index; " + strings.Join(reasons, "; ")
	}, units, nil
}

// withoutUnobserved hides the excluded binding units from the observation
// source, so no per-caller value is fetched under the service identity.
func (s datasetSpace) withoutUnobserved(id kernel.RepositoryID, value objectmodel.ObjectValue) (objectmodel.ObjectValue, bool) {
	if len(s.unobserved) == 0 {
		return value, false
	}
	kept := make([]objectmodel.UnitDeclaration, 0, len(value.Declarations))
	for _, declaration := range value.Declarations {
		if !s.unobserved[string(id)+":"+objectmodel.AddressKey(declaration.Address)] {
			kept = append(kept, declaration)
		}
	}
	if len(kept) == len(value.Declarations) {
		return value, false
	}
	value.Declarations = kept
	return value, true
}

// restoreDeclarations keeps the fixed declaration of every unit, observed or
// not, in its original order; observed units carry their resolved digest.
func restoreDeclarations(original, observed []objectmodel.UnitDeclaration) []objectmodel.UnitDeclaration {
	resolved := make(map[string]objectmodel.UnitDeclaration, len(observed))
	for _, declaration := range observed {
		resolved[objectmodel.AddressKey(declaration.Address)] = declaration
	}
	out := make([]objectmodel.UnitDeclaration, 0, len(original))
	for _, declaration := range original {
		if replacement, ok := resolved[objectmodel.AddressKey(declaration.Address)]; ok {
			declaration = replacement
		}
		out = append(out, declaration)
	}
	return out
}
