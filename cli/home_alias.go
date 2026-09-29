package cli

import (
	"kc/home"
)

// Home assembly lives in kc/home. Transport (CLI argv / HTTP handlers) keeps
// the names it already uses so verbs do not import the composition root in
// every file.
type (
	Home                 = home.Home
	HomeFile             = home.HomeFile
	HomeCatalog          = home.HomeCatalog
	HomeRepo             = home.HomeRepo
	StoresFile           = home.StoresFile
	LayoutFile           = home.LayoutFile
	RepositoryAccess     = home.RepositoryAccess
	RepositoryAccessFile = home.RepositoryAccessFile
)

var (
	Open                    = home.Open
	ReadHome                = home.ReadHome
	ReadStores              = home.ReadStores
	WriteStores             = home.WriteStores
	PublicStores            = home.PublicStores
	InitHome                = home.InitHome
	AddCatalog              = home.AddCatalog
	AddRepository           = home.AddRepository
	PersistControl          = home.PersistControl
	EncodeRepoDir           = home.EncodeRepoDir
	NormalizeCatalogID      = home.NormalizeCatalogID
	DefaultStores           = home.DefaultStores
	DefaultLayout           = home.DefaultLayout
	ResolveStoreDir         = home.ResolveStoreDir
	EnsureSystemRepository  = home.EnsureSystemRepository
	PublishSystemRepository = home.PublishSystemRepository
	OpenCatalogs            = home.OpenCatalogs
	LayoutPath              = home.LayoutPath
	StoresPath              = home.StoresPath
	NormalizeIndexDriver    = home.NormalizeIndexDriver
	SystemRepositoryStatus  = home.SystemRepositoryStatus
	ReadRepositoryAccess    = home.ReadRepositoryAccess
	WriteRepositoryAccess   = home.WriteRepositoryAccess
	EnsureRepositoryAccess  = home.EnsureRepositoryAccess
	SystemRepositoryAccess  = home.SystemRepositoryAccess
)

const DefaultCheckoutsDir = home.DefaultCheckoutsDir

func applyStoreFlags(file StoresFile, flags map[string]FlagValue) (StoresFile, error) {
	return home.ApplyStoreFlags(file, stringFlags(flags))
}

func stringFlags(flags map[string]FlagValue) map[string]string {
	out := map[string]string{}
	for name := range flags {
		out[name] = FlagString(flags, name)
	}
	return out
}
