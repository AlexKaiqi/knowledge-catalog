package home

import (
	"kc/home/config"
)

// The deployment configuration is owned by home/config: its shapes, its
// single-document parsing and its static validation live there, so reading a
// configuration cannot reach a source, an engine or a maintenance worker.
//
// These aliases keep the existing import paths working for cli, server and the
// fixture deployment, and each one names the same type rather than a second
// copy. The composition root below still owns what a configuration cannot do:
// opening authorities, restoring resources and assembling the object graph.
type (
	DeploymentConfig        = config.DeploymentConfig
	EvidenceConfig          = config.EvidenceConfig
	RegistryBinding         = config.RegistryBinding
	RepositoryBinding       = config.RepositoryBinding
	AdmissionConfig         = config.AdmissionConfig
	ConnectionPolicy        = config.ConnectionPolicy
	RepositoryAccess        = config.RepositoryAccess
	DatasetAccess           = config.DatasetAccess
	ManagedRepositoryConfig = config.ManagedRepositoryConfig
	StoresFile              = config.StoresFile
	LayoutFile              = config.LayoutFile
	HydrationCacheConfig    = config.HydrationCacheConfig
)

const (
	defaultRepositoryDriver = config.DefaultRepositoryDriver
	defaultIndexDriver      = config.DefaultIndexDriver
	defaultReposDir         = config.DefaultReposDir
	defaultRegistryDir      = config.DefaultRegistryDir
	defaultProjectionsDir   = config.DefaultProjectionsDir
	DefaultCheckoutsDir     = config.DefaultCheckoutsDir
)

var (
	// ReadDeployment parses and statically validates one deployment document.
	ReadDeployment = config.ReadDeployment
	// ReadStores loads layout.yaml and stores.yaml for one home.
	ReadStores = config.ReadStores
	// WriteStores persists the store selection without secrets.
	WriteStores = config.WriteStores
	// DefaultStores is the local profile's store selection.
	DefaultStores = config.DefaultStores
	// DefaultLayout is the local profile's directory set.
	DefaultLayout = config.DefaultLayout
	// StoresPath is where a home keeps its store selection.
	StoresPath = config.StoresPath
	// LayoutPath is where a home keeps its directory set.
	LayoutPath = config.LayoutPath
	// NormalizeRepositoryID folds the accepted repository id spellings.
	NormalizeRepositoryID = config.NormalizeRepositoryID
	// EncodeRepoDir maps a repository id to one directory name.
	EncodeRepoDir = config.EncodeRepoDir
	// NormalizeIndexDriver folds the accepted index driver spellings.
	NormalizeIndexDriver = config.NormalizeIndexDriver
	// ResolveStoreDir joins a layout directory with a home directory.
	ResolveStoreDir = config.ResolveStoreDir
)

// registryHomeRepo maps a configured Registry binding to the composition
// root's local view. It stays a function because the configuration types are
// aliases owned by home/config, and opening an authority is this root's job.
func registryHomeRepo(binding RegistryBinding) HomeRepo {
	return HomeRepo{ID: binding.ID, Driver: binding.Driver, DSN: binding.DSN, Dir: binding.Dir}
}

// repositoryHomeRepo maps a configured member binding to the same local view.
func repositoryHomeRepo(binding RepositoryBinding) HomeRepo {
	return HomeRepo{ID: binding.ID, Driver: binding.Driver, DSN: binding.DSN, Dir: binding.Dir}
}
