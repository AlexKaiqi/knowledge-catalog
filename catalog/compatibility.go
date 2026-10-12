// Package catalog preserves the historical import path for the Snapshot Store
// Dataset module. All state, publication, and file composition belong to
// kc/snapshot/dataset; this package owns no second implementation.
//
// New file-level consumers import kc/snapshot/dataset directly.
package catalog

import (
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

type (
	Catalog               = dataset.Catalog
	CatalogCommit         = dataset.CatalogCommit
	CatalogHistory        = dataset.CatalogHistory
	CatalogLogQuery       = dataset.CatalogLogQuery
	CatalogState          = dataset.CatalogState
	DatasetDeliveredTree  = dataset.DatasetDeliveredTree
	DatasetItem           = dataset.DatasetItem
	Hook                  = dataset.Hook
	KnowledgeSet          = dataset.KnowledgeSet
	KnowledgeSetCheck     = dataset.KnowledgeSetCheck
	KnowledgeSetFileMount = dataset.KnowledgeSetFileMount
	KnowledgeSetIssue     = dataset.KnowledgeSetIssue
	KnowledgeSetMount     = dataset.KnowledgeSetMount
	KnowledgeSetOverlay   = dataset.KnowledgeSetOverlay
	KnowledgeSetRecipe    = dataset.KnowledgeSetRecipe
	KnowledgeSetSource    = dataset.KnowledgeSetSource
	MountRoute            = dataset.MountRoute
	Registry              = dataset.Registry
	ResolvedKnowledgeSet  = dataset.ResolvedKnowledgeSet
	Snapshot              = dataset.Snapshot
	VirtualFile           = dataset.VirtualFile
	VirtualMount          = dataset.VirtualMount
)

const (
	DatasetItemPrefix    = dataset.DatasetItemPrefix
	DatasetItemFile      = dataset.DatasetItemFile
	DefaultCatalogID     = dataset.DefaultCatalogID
	KnowledgeSetFileName = dataset.KnowledgeSetFileName
)

// EmptyCatalogState preserves the historical empty value. Assigning it does not
// change the Dataset module default; see README.md for this migration limitation.
var EmptyCatalogState = dataset.EmptyCatalogState

func CatalogFile() string {
	return dataset.CatalogFile()
}

func CatalogsPath(home string) string {
	return dataset.CatalogsPath(home)
}

func CreateRemoteRegistry(rootDir, catalogID, remote, ref string) (*Registry, error) {
	return dataset.CreateRemoteRegistry(rootDir, catalogID, remote, ref)
}

func CreateSnapshotRegistry(store snapshot.Store, catalogID, ref string) (*Registry, error) {
	return dataset.CreateSnapshotRegistry(store, catalogID, ref)
}

func DatasetFileItems(items []DatasetItem) []DatasetItem {
	return dataset.DatasetFileItems(items)
}

func DatasetPathAllowed(items []DatasetItem, repository kernel.RepositoryID, path string) bool {
	return dataset.DatasetPathAllowed(items, repository, path)
}

func DatasetVersionRef(revision int) string {
	return dataset.DatasetVersionRef(revision)
}

func DecodeJSON(body []byte, dest any) error {
	return dataset.DecodeJSON(body, dest)
}

func FormatKnowledgeSetRecipe(rec KnowledgeSetRecipe) ([]byte, error) {
	return dataset.FormatKnowledgeSetRecipe(rec)
}

func HasMountPaths(sources []KnowledgeSetSource) bool {
	return dataset.HasMountPaths(sources)
}

func HashResolved(setID string, sources []KnowledgeSetSource, repos map[kernel.RepositoryID]kernel.CommitID) string {
	return dataset.HashResolved(setID, sources, repos)
}

func KnowledgeSetYAML(setID string) string {
	return dataset.KnowledgeSetYAML(setID)
}

func ListVirtualMountsAt(def KnowledgeSet, resolved ResolvedKnowledgeSet) ([]VirtualMount, error) {
	return dataset.ListVirtualMountsAt(def, resolved)
}

func MergeOverlay(base KnowledgeSet, overlay KnowledgeSetOverlay) (KnowledgeSet, error) {
	return dataset.MergeOverlay(base, overlay)
}

func MountPath(path string) *string {
	return dataset.MountPath(path)
}

func NewCatalog(store *snapshot.Registry, registry *Registry) (*Catalog, error) {
	return dataset.NewCatalog(store, registry)
}

func NewDatasetDeliveredTree(items []DatasetItem) *DatasetDeliveredTree {
	return dataset.NewDatasetDeliveredTree(items)
}

func NewRegistry(rootDir string, catalogID string) (*Registry, error) {
	return dataset.NewRegistry(rootDir, catalogID)
}

func NormalizeCatalogState(state CatalogState) CatalogState {
	return dataset.NormalizeCatalogState(state)
}

func NormalizeMountPath(p string) string {
	return dataset.NormalizeMountPath(p)
}

func OpenRemoteRegistry(rootDir, catalogID, remote, ref string) (*Registry, error) {
	return dataset.OpenRemoteRegistry(rootDir, catalogID, remote, ref)
}

func OpenSnapshotRegistry(store snapshot.Store, catalogID, ref string) (*Registry, error) {
	return dataset.OpenSnapshotRegistry(store, catalogID, ref)
}

func OverlayFile(home, principal, setID string) string {
	return dataset.OverlayFile(home, principal, setID)
}

func ParseKnowledgeSetOverlay(raw []byte) (KnowledgeSetOverlay, error) {
	return dataset.ParseKnowledgeSetOverlay(raw)
}

func ParseKnowledgeSetRecipe(raw []byte) (KnowledgeSetRecipe, error) {
	return dataset.ParseKnowledgeSetRecipe(raw)
}

func PeekID(rootDir string) (string, error) {
	return dataset.PeekID(rootDir)
}

func PeekStamp(rootDir string) (string, error) {
	return dataset.PeekStamp(rootDir)
}

func RecipeFromKnowledgeSet(def KnowledgeSet) (KnowledgeSetRecipe, bool) {
	return dataset.RecipeFromKnowledgeSet(def)
}

func ReplayDefinition(def KnowledgeSet, pin ResolvedKnowledgeSet) (ResolvedKnowledgeSet, error) {
	return dataset.ReplayDefinition(def, pin)
}

func RepositoryFile(repositoryID string) string {
	return dataset.RepositoryFile(repositoryID)
}

func RequireAllMountsDeclared(sources []KnowledgeSetSource) error {
	return dataset.RequireAllMountsDeclared(sources)
}

func RootFirst(sources []KnowledgeSetSource) []KnowledgeSetSource {
	return dataset.RootFirst(sources)
}

func RootMount(sources []KnowledgeSetSource) *KnowledgeSetSource {
	return dataset.RootMount(sources)
}

func RouteMount(def KnowledgeSet, workspacePath string) (MountRoute, error) {
	return dataset.RouteMount(def, workspacePath)
}

func RouteMounts(def KnowledgeSet, workspacePaths []string) (map[kernel.RepositoryID][]MountRoute, error) {
	return dataset.RouteMounts(def, workspacePaths)
}

func ValidateKnowledgeSet(def KnowledgeSet) error {
	return dataset.ValidateKnowledgeSet(def)
}
