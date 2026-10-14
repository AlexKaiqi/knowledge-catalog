// Package plugin assembles the complete declarative index contribution.
package plugin

import (
	"path/filepath"

	indexcli "kc/declarativeindex/cli"
	"kc/declarativeindex/control"
	projection "kc/declarativeindex/projection"
	discoverydatasets "kc/discovery/datasets"
	indexplugins "kc/discovery/plugin"
	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot/dataset"
)

type Options struct {
	Directory string
	Engine    func(string) index.EngineOpener
	Lookup    func(kernel.RepositoryID) (objectmodel.Repository, error)
	Datasets  dataset.Service
	Hydrator  objectmodel.Hydrator
}

func Entry(options Options) indexplugins.IndexPluginEntry {
	affordance := indexcli.Affordance()
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
		repositories := append([]kernel.RepositoryID(nil), scope.Repositories...)
		allowed := map[kernel.RepositoryID]bool{}
		for _, id := range repositories {
			allowed[id] = true
		}
		controller, err := control.NewController(idx, control.NewTargetStore(filepath.Join(directory, "controller.db")), func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			if !allowed[id] {
				return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Repository is outside this plugin instance")
			}
			return options.Lookup(id)
		})
		if err != nil {
			return indexplugins.IndexContribution{}, err
		}
		controller.SetInventory(func() ([]kernel.RepositoryID, error) {
			return append([]kernel.RepositoryID(nil), repositories...), nil
		})
		controller.SetServingBases(discoverydatasets.ServingBasisDemand{Datasets: options.Datasets}.Bases)
		return indexplugins.IndexContribution{Queries: idx, Maintenance: controller,
			Start: controller.Start, Stop: controller.Close,
		}, nil
	}}
}
