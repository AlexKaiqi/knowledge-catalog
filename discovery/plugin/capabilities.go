package plugin

import (
	retrieval "kc/discovery"
	"kc/kernel"
	objectmodel "kc/object"
)

// IndexCapabilities is an instance's read-only declaration interpreter.
// Shape and Schema checks belong to Writer; this port checks instance support
// and reports whether replacing an existing member requires a rebuild.
type IndexCapabilities interface {
	Describe() retrieval.IndexManifest
	Check(objectmodel.IndexDeclaration, []retrieval.DeclarationInput) string
	Rebuilds(objectmodel.IndexDeclaration, objectmodel.IndexDeclaration) bool
}

// CapabilityManifests reads installed instance facts without executing an
// engine task, source read, maintenance or readiness probe.
func (p *IndexPlugins) CapabilityManifests(name string) ([]retrieval.IndexInstanceManifest, error) {
	out := []retrieval.IndexInstanceManifest{}
	if p != nil {
		for _, instance := range p.instances {
			if instance.selection.Name != name || nilContribution(instance.services.Capabilities) {
				continue
			}
			manifest := instance.services.Capabilities.Describe()
			out = append(out, retrieval.IndexInstanceManifest{Plugin: name, Instance: instance.selection.Instance, Catalog: instance.catalog, Manifest: manifest})
		}
	}
	if len(out) == 0 {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin %s has no installed declaration interpreter", name)
	}
	return out, nil
}

// DeclarationCapabilities selects by the declaration's manifest name. Caller
// requests never select an engine instance or change deployment routing.
func (p *IndexPlugins) DeclarationCapabilities(name, catalog string) (IndexCapabilities, PluginSelection, error) {
	if p != nil {
		for _, instance := range p.instances {
			if instance.selection.Name == name && instance.catalog == catalog && !nilContribution(instance.services.Capabilities) {
				return instance.services.Capabilities, instance.selection, nil
			}
		}
		// A version mismatch still reaches the same named interpreter so it
		// can report both versions; it never substitutes the installed version.
		for _, instance := range p.instances {
			if instance.selection.Name == name && !nilContribution(instance.services.Capabilities) {
				return instance.services.Capabilities, instance.selection, nil
			}
		}
	}
	return nil, PluginSelection{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin %s has no installed declaration interpreter for %s", name, catalog)
}
