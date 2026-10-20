// Package plugin assembles the complete declarative index contribution.
package plugin

import (
	"path/filepath"

	indexcapability "kc/declarativeindex/capability"
	"kc/declarativeindex/control"
	projection "kc/declarativeindex/projection"
	discoverydatasets "kc/discovery/datasets"
	indexplugins "kc/discovery/plugin"
	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot/dataset"
)

// Catalog is the capability catalog this compiled entry answers. A deployment
// maps it to one instance; declarations name it to choose their interpreter.
const Catalog = indexcapability.ManifestName + "@" + indexcapability.ManifestVersion

type Options struct {
	// RegisteredModels comes from the same validated engine deployment configuration.
	RegisteredModels []string
	Directory        string
	Engine           func(string) index.EngineOpener
	Lookup           func(kernel.RepositoryID) (objectmodel.Repository, error)
	Datasets         dataset.Service
	Hydrator         objectmodel.Hydrator
	// Manifest is this instance's capability manifest; nil uses the compiled
	// default with the validated RegisteredModels.
	Manifest *indexcapability.Manifest
}

// InstanceManifest is the manifest an instance opened with options publishes.
func (options Options) InstanceManifest() indexcapability.Manifest {
	if options.Manifest != nil {
		return indexcapability.Manifest(options.Manifest.Describe())
	}
	return indexcapability.DefaultManifest(options.RegisteredModels...)
}

func Entry(options Options) indexplugins.IndexPluginEntry {
	manifest := options.InstanceManifest()
	affordance := indexcapability.AffordanceFor(manifest)
	return indexplugins.IndexPluginEntry{Affordance: affordance, Open: func(scope indexplugins.PluginScope) (indexplugins.IndexContribution, error) {
		directory, namespace := options.Directory, ""
		// main adopts the previously selected physical resources and validators.
		if scope.Instance != "main" {
			directory = filepath.Join(directory, "plugins", scope.Name, scope.Instance)
			namespace = scope.Name + "/" + scope.Instance
		}
		idx := projection.NewIndexEngine(directory, options.Engine(namespace))
		scope.Own(idx.Close)
		idx.SetHydrator(options.Hydrator)
		idx.SetDeclarationManifest(manifest.Check, manifest.Describe().Digest)
		if options.Datasets != nil {
			idx.SetDatasetRevisionFence(options.Datasets.WithCurrentRevision)
		}
		repositories := append([]kernel.RepositoryID(nil), scope.Repositories...)
		allowed := map[kernel.RepositoryID]bool{}
		for _, id := range repositories {
			allowed[id] = true
		}
		controller, err := control.NewController(idx, control.NewTargetStore(filepath.Join(directory, "controller.db")), func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			admitted := allowed[id]
			if scope.ContainsRepository != nil {
				admitted = scope.ContainsRepository(id)
			}
			if !admitted {
				return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Repository is outside this plugin instance")
			}
			return options.Lookup(id)
		})
		if err != nil {
			return indexplugins.IndexContribution{}, err
		}
		controller.SetInventory(func() ([]kernel.RepositoryID, error) {
			if scope.RepositoryInventory != nil {
				return append([]kernel.RepositoryID(nil), scope.RepositoryInventory()...), nil
			}
			return append([]kernel.RepositoryID(nil), repositories...), nil
		})
		controller.SetServingBases(discoverydatasets.ServingBasisDemand{Datasets: options.Datasets}.Bases)
		return indexplugins.IndexContribution{Queries: idx, Maintenance: controller, Capabilities: manifest,
			Start: controller.Start, Stop: controller.Close,
		}, nil
	}}
}
