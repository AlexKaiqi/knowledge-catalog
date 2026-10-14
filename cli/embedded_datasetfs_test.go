package cli

import (
	"fmt"

	apphome "kc/home"
	inv "kc/invocation"
	snapshot "kc/snapshot"

	"path/filepath"
	"strings"

	"kc/datasetfs"
	"kc/kernel"
	"kc/snapshot/dataset"
	"path"
	"sort"
)

// datasetFSProjection is the frozen, authorized input used by embedded
// application tests. Public kcfs obtains the same information through the
// Dataset File Gateway.
type datasetFSProjection struct {
	home       string
	root       string
	dataset    string
	flags      map[string]FlagValue
	homeState  *apphome.Home
	datasets   dataset.Service
	definition dataset.Dataset
	resolved   dataset.ResolvedDataset
	mounts     []dataset.VirtualMount
}

func (p datasetFSProjection) recordDatasetFSRead(mount dataset.VirtualMount, relative string, data []byte, readErr error) ([]byte, error) {
	virtualPath := path.Join(mount.Path, relative)
	result := map[string]any{"path": virtualPath, "repository": mount.Repository, "commit": mount.Commit}
	readFlags := make(map[string]FlagValue, len(p.flags)+2)
	for name, value := range p.flags {
		readFlags[name] = value
	}
	readFlags["path"] = virtualPath
	readFlags["_action"] = "file.read"
	if _, _, err := inv.RecordObjectAccess(p.home, "file-read", readFlags, result, readErr); err != nil && readErr == nil {
		return nil, err
	}
	return data, nil
}

func (p datasetFSProjection) build() (datasetfs.Plan, datasetFSManifest, error) {
	// Per-file entries (U10) are not projected into the host filesystem yet:
	// mixing eager files into a lazy directory mount is an unshipped datasetfs
	// capability. Fail loudly instead of silently shrinking the delivered
	// tree; use the file gateway or `kc dataset clone` for such datasets.
	for _, item := range p.resolved.Items {
		if item.Kind == dataset.DatasetItemFile {
			return datasetfs.Plan{}, datasetFSManifest{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"dataset %s publishes per-file entries that the file-system projection does not deliver yet; use the file gateway or kc dataset clone", p.dataset)
		}
	}
	plan := datasetfs.Plan{SetID: p.dataset, PinID: p.resolved.PinID, Root: p.root}
	manifest := datasetFSManifest{SetID: p.dataset, PinID: p.resolved.PinID, Pin: p.resolved, Root: p.root, ReadOnly: true, Mounts: []datasetFSMount{}}
	for _, mount := range p.mounts {
		store, ok := p.homeState.Store.Get(mount.Repository)
		if !ok {
			return datasetfs.Plan{}, datasetFSManifest{}, kernel.Fail(kernel.ErrUsageInvalid, "repository %s is not attached", mount.Repository)
		}
		tree, ok := snapshot.TreeReaderOf(store)
		if !ok {
			return datasetfs.Plan{}, datasetFSManifest{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"repository %s does not support raw path reads required by kcfs", mount.Repository)
		}
		directory, ok := snapshot.DirectoryReaderOf(store)
		if !ok {
			return datasetfs.Plan{}, datasetFSManifest{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"repository %s does not support lazy directory reads required by kcfs", mount.Repository)
		}
		mountCopy := mount
		one := datasetfs.Mount{
			Path: mount.Path, Repository: string(mount.Repository), Commit: string(mount.Commit),
			Directory: &datasetfs.Directory{
				List: func(relativeDirectory string) ([]datasetfs.DirectoryEntry, error) {
					repositoryDirectory, err := inv.DatasetFSRepositoryPath(mountCopy.SubPath, relativeDirectory)
					if err != nil {
						return nil, err
					}
					return readAllDirectoryPages(directory, mountCopy.Commit, repositoryDirectory)
				},
				Read: func(relativeFile string) ([]byte, error) {
					repositoryPath, err := inv.DatasetFSRepositoryPath(mountCopy.SubPath, relativeFile)
					if err == nil {
						var data []byte
						data, err = tree.ReadFile(repositoryPath, mountCopy.Commit)
						if err == nil {
							return p.recordDatasetFSRead(mountCopy, relativeFile, data, nil)
						}
					}
					_, _ = p.recordDatasetFSRead(mountCopy, relativeFile, nil, err)
					return nil, err
				},
			},
		}
		plan.Mounts = append(plan.Mounts, one)
		manifest.Mounts = append(manifest.Mounts, datasetFSMount{
			Path: mount.Path, Mountpoint: filepath.Join(p.root, filepath.FromSlash(mount.Path)),
			Repository: mount.Repository, Commit: mount.Commit,
		})
	}
	if _, err := plan.Validate(); err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, err
	}
	return plan, manifest, nil
}

// prepareEmbeddedDatasetFS is a conformance-test seam, not a product
// transport. It proves plan semantics against the shared application services.
func prepareEmbeddedDatasetFS(homeDir string, config datasetFSConfig) (datasetfs.Plan, datasetFSManifest, func(), error) {
	root, err := filepath.Abs(config.root)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, fmt.Errorf("resolve workspace root %s: %w", config.root, err)
	}
	home, err := filepath.Abs(homeDir)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	flags := map[string]FlagValue{
		"home":    home,
		"dataset": config.dataset,
	}
	if config.principal != "" {
		flags["as"] = config.principal
	}
	if err := inv.Authorize(home, "dataset.resolve", flags, nil); err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	ws, err := apphome.Open(home)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	closeHome := func() { _ = ws.Close() }
	keepOpen := false
	defer func() {
		if !keepOpen {
			closeHome()
		}
	}()
	cat, err := inv.PickDatasets(ws, flags)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	def, err := effectiveWorkspace(ws, home, cat, config.dataset, flags)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	resolved, err := inv.ResolveOrReplay(ws, home, cat, config.dataset, flags)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	mounts, err := dataset.ListVirtualMountsAt(def, resolved)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	visibleDef, visibleResolved, visibleMounts, err := filterVirtualWorkspace(def, resolved, mounts, func(repository kernel.RepositoryID) (bool, error) {
		return inv.DatasetFSMayReadRepository(home, flags, string(repository), ws)
	})
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	projection := datasetFSProjection{
		home: home, root: root, dataset: config.dataset, flags: flags,
		homeState: ws, datasets: cat, definition: visibleDef, resolved: visibleResolved,
		mounts: visibleMounts,
	}
	plan, manifest, err := projection.build()
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	keepOpen = true
	return plan, manifest, closeHome, nil
}

func readAllDirectoryPages(reader snapshot.DirectoryReader, commit kernel.CommitID, directory string) ([]datasetfs.DirectoryEntry, error) {
	var out []datasetfs.DirectoryEntry
	continuation := ""
	for {
		page, err := reader.ReadDirectory(snapshot.DirectoryRequest{Commit: commit, Directory: directory, Limit: 256, Continuation: continuation})
		if err != nil {
			return nil, err
		}
		if page.Generation != string(commit) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "directory provider returned generation %q for fixed commit %s", page.Generation, commit)
		}
		for _, entry := range page.Entries {
			if entry.Name == "" || strings.Contains(entry.Name, "/") || (entry.Kind != "file" && entry.Kind != "directory") {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "directory provider returned invalid direct child")
			}
			out = append(out, datasetfs.DirectoryEntry{Name: entry.Name, Directory: entry.Kind == "directory"})
		}
		if page.Exhausted {
			break
		}
		if page.Continuation == "" || page.Continuation == continuation {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "directory provider returned a non-advancing continuation")
		}
		continuation = page.Continuation
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func relativeMountPath(mountPath, virtualPath string) (string, bool) {
	mountPath = strings.Trim(path.Clean("/"+mountPath), "/")
	virtualPath = strings.Trim(path.Clean("/"+virtualPath), "/")
	if mountPath == "" {
		return virtualPath, true
	}
	if virtualPath == mountPath {
		return "", true
	}
	prefix := mountPath + "/"
	if !strings.HasPrefix(virtualPath, prefix) {
		return "", false
	}
	return strings.TrimPrefix(virtualPath, prefix), true
}
