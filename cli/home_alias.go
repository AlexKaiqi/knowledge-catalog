package cli

import (
	"kc/catalog"
	"kc/home"
	"kc/kernel"
	"kc/snapshot"
)

// Home assembly lives in kc/home. Transport (CLI argv / HTTP handlers) keeps
// the names it already uses so verbs do not import the composition root in
// every file.
type (
	Home        = home.Home
	HomeFile    = home.HomeFile
	HomeCatalog = home.HomeCatalog
	HomeRepo    = home.HomeRepo
	StoresFile  = home.StoresFile
	LayoutFile  = home.LayoutFile
)

var (
	Open                      = home.Open
	ReadHome                  = home.ReadHome
	ReadStores                = home.ReadStores
	WriteStores               = home.WriteStores
	PublicStores              = home.PublicStores
	InitHome                  = home.InitHome
	AddCatalog                = home.AddCatalog
	AddRepository             = home.AddRepository
	PersistControl            = home.PersistControl
	EncodeRepoDir             = home.EncodeRepoDir
	NormalizeCatalogID        = home.NormalizeCatalogID
	DefaultStores             = home.DefaultStores
	DefaultLayout             = home.DefaultLayout
	ResolveStoreDir           = home.ResolveStoreDir
	EnsureSystemRepository    = home.EnsureSystemRepository
	PublishSystemRepository   = home.PublishSystemRepository
	OpenCatalogs              = home.OpenCatalogs
	LayoutPath                = home.LayoutPath
	StoresPath                = home.StoresPath
	NormalizeIndexDriver      = home.NormalizeIndexDriver
	AuthorizeSystemRepository = home.AuthorizeSystemRepository
	SystemRepositoryStatus    = home.SystemRepositoryStatus
)

const DefaultCheckoutsDir = home.DefaultCheckoutsDir

func homeReady(dir string) bool { return home.Ready(dir) }

func missingHome(dir string) error { return home.Missing(dir) }

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

func layoutPath(dir string) string { return home.LayoutPath(dir) }

func storesPath(dir string) string { return home.StoresPath(dir) }

func openCatalogs(dir string, file HomeFile, store *snapshot.Registry) (map[string]*catalog.Catalog, map[string]*catalog.Registry, error) {
	return home.OpenCatalogs(dir, file, store)
}

func normalizeIndexDriver(raw string) string { return home.NormalizeIndexDriver(raw) }

func authorizeSystemRepository(action, repositoryID, principal string) (bool, error) {
	return home.AuthorizeSystemRepository(action, repositoryID, principal)
}

func systemRepositoryStatus(commit kernel.CommitID) map[string]any {
	return home.SystemRepositoryStatus(commit)
}
