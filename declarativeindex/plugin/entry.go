// Package plugin assembles the complete declarative index contribution.
package plugin

import (
	"context"
	"path/filepath"
	"time"

	indexcapability "kc/declarativeindex/capability"
	"kc/declarativeindex/control"
	projection "kc/declarativeindex/projection"
	discoverydatasets "kc/discovery/datasets"
	indexplugins "kc/discovery/plugin"
	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
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
	Resources        func(string) (index.ResourceCoordinator, error)
	Lookup           func(kernel.RepositoryID) (objectmodel.Repository, error)
	Datasets         dataset.Service
	DatasetPin       func(dataset.ResolvedDataset) reader.DatasetPin
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
		var resources index.ResourceCoordinator
		if options.Resources != nil {
			var err error
			resources, err = options.Resources(namespace)
			if err != nil {
				return indexplugins.IndexContribution{}, err
			}
			if resources != nil {
				scope.Own(resources.Close)
			}
		}
		repositories := append([]kernel.RepositoryID(nil), scope.Repositories...)
		allowed := map[kernel.RepositoryID]bool{}
		for _, id := range repositories {
			allowed[id] = true
		}
		lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			admitted := allowed[id]
			if scope.ContainsRepository != nil {
				admitted = scope.ContainsRepository(id)
			}
			if !admitted {
				return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Repository is outside this plugin instance")
			}
			return options.Lookup(id)
		}
		if authority, ok := options.Datasets.(dataset.PublicationCoordinator); ok {
			publication := discoverydatasets.PublicationRecovery{Authority: authority, Definitions: options.Datasets}
			resolve := publication.Resolve
			referenced := publication.Referenced
			if references, ok := options.Datasets.(dataset.RevisionReferenceCoordinator); ok {
				observations := datasetObservationAuthority{references: references, publications: authority, owner: "declarative-index/" + scope.Name + "/" + scope.Instance + "/observations", now: time.Now}
				idx.SetDatasetObservationPublication(observations.begin, observations.evidence)
				resolve = routeResourceAuthority(observations.resolve, publication.Resolve)
				referenced = routeResourceAuthority(observations.referenced, publication.Referenced)
			}
			idx.SetResourceRecovery(resources, resolve)
			idx.SetPublicationReferenceLookup(referenced)
			idx.SetDatasetPublicationLookup(publication.Evidence)
			if options.DatasetPin != nil {
				idx.SetPublishedDatasetRecovery(func(ctx context.Context, setID string, revision int) error {
					return recoverPublishedDatasets(ctx, idx, options, publication, lookup, dataset.PublicationLookup{SetID: setID, Revision: revision})
				})
			}
		} else {
			idx.SetResourceRecovery(resources, nil)
		}
		controller, err := control.NewController(idx, control.NewTargetStore(filepath.Join(directory, "controller.db")), lookup)
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
