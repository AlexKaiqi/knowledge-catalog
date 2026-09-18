package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"kc/catalog"
	kcclient "kc/client"
	"kc/datasetfs"
	"kc/kernel"
	"kc/snapshot"
)

type datasetFSConfig struct {
	home      string
	server    string
	catalogID string
	dataset   string
	root      string
	principal string
	pin       string
	debug     bool
	view      string
}

type datasetFSMount struct {
	Path       string              `json:"path"`
	Mountpoint string              `json:"mountpoint"`
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
}

type datasetFSManifest struct {
	SetID    string                       `json:"setId"`
	PinID    string                       `json:"pinId"`
	Pin      catalog.ResolvedKnowledgeSet `json:"pin"`
	Root     string                       `json:"root"`
	ReadOnly bool                         `json:"readOnly"`
	Mounts   []datasetFSMount             `json:"mounts"`
}

// datasetFSProjection is the frozen, authorized input used by embedded
// application tests. Public kcfs obtains the same information through the
// Knowledge Set File Gateway.
type datasetFSProjection struct {
	home       string
	root       string
	dataset    string
	flags      map[string]FlagValue
	homeState  *Home
	catalog    *catalog.Catalog
	definition catalog.KnowledgeSet
	resolved   catalog.ResolvedKnowledgeSet
	mounts     []catalog.VirtualMount
}

// RunKnowledgeSetFS is the entrypoint used by cmd/kcfs. Host mounting is kept
// outside the kc verb table because it must execute on the user's Linux host;
// unlike protocol commands it must never be mirrored as a domain HTTP route.
func RunKnowledgeSetFS(argv []string, stdout, stderr io.Writer) int {
	if len(argv) > 0 && argv[0] == "--" {
		argv = argv[1:]
	}
	mode := ""
	if len(argv) > 0 && !strings.HasPrefix(argv[0], "-") {
		mode = argv[0]
		argv = argv[1:]
	}
	if mode == "help" || mode == "--help" || mode == "-h" || mode == "" {
		_, _ = io.WriteString(stdout, datasetFSHelp)
		return 0
	}
	if mode == "stop" {
		return stopKnowledgeSetFS(argv, stderr)
	}
	if mode == "daemon-mount" {
		return daemonMountKnowledgeSetFS(argv, stdout, stderr)
	}
	if mode != "mount" && mode != "plan" {
		writeKnowledgeSetFSError(stderr, fmt.Errorf("unknown kcfs command %s", mode))
		return 2
	}
	config, err := parseKnowledgeSetFSConfig(mode, argv, stderr)
	if err != nil {
		writeKnowledgeSetFSError(stderr, err)
		return 2
	}
	plan, manifest, closeHome, err := prepareKnowledgeSetFS(config)
	if err != nil {
		writeKnowledgeSetFSError(stderr, err)
		return 1
	}
	defer closeHome()
	if mode == "plan" {
		writeKnowledgeSetFSJSON(stdout, manifest)
		return 0
	}
	handle, err := datasetfs.MountAll(plan, datasetfs.Options{Debug: config.debug})
	if err != nil {
		writeKnowledgeSetFSError(stderr, err)
		return 1
	}
	writeKnowledgeSetFSJSON(stdout, manifest)
	done := make(chan struct{})
	go func() {
		handle.Wait()
		close(done)
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case <-signals:
		if err := handle.Unmount(); err != nil {
			writeKnowledgeSetFSError(stderr, err)
			return 1
		}
		<-done
	case <-done:
	}
	return 0
}

func parseKnowledgeSetFSConfig(mode string, argv []string, stderr io.Writer) (datasetFSConfig, error) {
	set := flag.NewFlagSet("kcfs "+mode, flag.ContinueOnError)
	set.SetOutput(stderr)
	config := datasetFSConfig{}
	set.StringVar(&config.server, "server", remoteServerURL(nil), "KC service URL for remote lazy reads")
	set.StringVar(&config.catalogID, "catalog", "", "Catalog id (defaults to the home's first Catalog)")
	set.StringVar(&config.dataset, "dataset", "", "knowledge set id")
	set.StringVar(&config.root, "root", "", "existing user project root")
	set.StringVar(&config.principal, "as", "", "principal used for Workspace and repository read grants")
	set.StringVar(&config.pin, "pin", "", "ResolvedKnowledgeSet JSON or file to replay")
	set.BoolVar(&config.debug, "debug", false, "enable go-fuse protocol logging")
	set.StringVar(&config.view, "view", "repository", "file view: repository or semantic")
	if err := set.Parse(argv); err != nil {
		return datasetFSConfig{}, err
	}
	if set.NArg() != 0 {
		return datasetFSConfig{}, fmt.Errorf("unexpected argument %s", set.Arg(0))
	}
	explicit := map[string]bool{}
	set.Visit(func(flag *flag.Flag) { explicit[flag.Name] = true })
	if explicit["pin"] || strings.TrimSpace(config.pin) != "" {
		return datasetFSConfig{}, kernel.Fail(kernel.ErrUsageInvalid, "kcfs rejects --pin; pass --dataset")
	}
	if strings.TrimSpace(config.dataset) == "" {
		return datasetFSConfig{}, fmt.Errorf("missing --dataset")
	}
	if strings.TrimSpace(config.root) == "" {
		return datasetFSConfig{}, fmt.Errorf("missing --root")
	}
	if config.view != "repository" && config.view != "semantic" {
		return datasetFSConfig{}, fmt.Errorf("--view must be repository or semantic")
	}
	return config, nil
}

func prepareKnowledgeSetFS(config datasetFSConfig) (datasetfs.Plan, datasetFSManifest, func(), error) {
	root, err := filepath.Abs(config.root)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, fmt.Errorf("resolve workspace root %s: %w", config.root, err)
	}
	if strings.TrimSpace(config.server) == "" {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, kernel.Fail(kernel.ErrUsageInvalid,
			"kcfs requires KC Server; set --server or KC_SERVER_URL")
	}
	return prepareRemoteKnowledgeSetFS(config, root)
}

// prepareEmbeddedKnowledgeSetFS is a conformance-test seam, not a product
// transport. It proves plan semantics against the shared application services.
func prepareEmbeddedKnowledgeSetFS(config datasetFSConfig) (datasetfs.Plan, datasetFSManifest, func(), error) {
	root, err := filepath.Abs(config.root)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, fmt.Errorf("resolve workspace root %s: %w", config.root, err)
	}
	home, err := filepath.Abs(config.home)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	flags := map[string]FlagValue{
		"home":    home,
		"dataset": config.dataset,
	}
	if config.catalogID != "" {
		flags["catalog"] = config.catalogID
	}
	if config.principal != "" {
		flags["as"] = config.principal
	}
	if config.pin != "" {
		flags["pin"] = config.pin
	}
	if err := authorize(home, "dataset.resolve", flags, nil); err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	ws, err := Open(home)
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
	cat, err := pickCatalog(ws, flags)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	def, err := effectiveWorkspace(ws, home, cat, config.dataset, flags)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	resolved, err := resolveOrReplay(ws, home, cat, config.dataset, flags)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	mounts, err := catalog.ListVirtualMountsAt(def, resolved)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	visibleDef, visibleResolved, visibleMounts, err := filterVirtualWorkspace(def, resolved, mounts, func(repository kernel.RepositoryID) (bool, error) {
		return datasetFSMayReadRepository(home, flags, string(repository), ws)
	})
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	projection := datasetFSProjection{
		home: home, root: root, dataset: config.dataset, flags: flags,
		homeState: ws, catalog: cat, definition: visibleDef, resolved: visibleResolved,
		mounts: visibleMounts,
	}
	plan, manifest, err := projection.build()
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	keepOpen = true
	return plan, manifest, closeHome, nil
}

func prepareRemoteKnowledgeSetFS(config datasetFSConfig, root string) (datasetfs.Plan, datasetFSManifest, func(), error) {
	ctx := context.Background()
	flags := map[string]FlagValue{}
	if config.principal != "" {
		flags["as"] = config.principal
	}
	client, err := newRemoteSessionClient(ctx, config.server, flags)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	coordinate := kcclient.WorkspaceFileCoordinate{Catalog: config.catalogID, Dataset: config.dataset, View: config.view}
	if strings.TrimSpace(config.pin) != "" {
		pin, err := datasetFSPin(config.pin)
		if err != nil {
			return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
		}
		resolvedPin, err := resolvedKnowledgeSetPin(pin)
		if err != nil {
			return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
		}
		coordinate.Pin = resolvedPin
	}
	var response kcclient.WorkspaceFileMountsResponse
	if err := client.WorkspaceFilesService().Mounts(ctx, kcclient.WorkspaceFileMountsRequest{WorkspaceFileCoordinate: coordinate}, kcclient.RequestOptions{}, &response); err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	if response.Pin.PinID == "" || response.Pin.SetID != config.dataset {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, kernel.Fail(kernel.ErrPreconditionFailed, "Knowledge Set File Gateway returned an invalid pin")
	}
	pinned, err := json.Marshal(response.Pin)
	if err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	coordinate.Pin = pinned
	plan := datasetfs.Plan{SetID: config.dataset, PinID: response.Pin.PinID, Root: root}
	manifest := datasetFSManifest{SetID: config.dataset, PinID: response.Pin.PinID, Pin: response.Pin, Root: root, ReadOnly: true, Mounts: []datasetFSMount{}}
	for _, mount := range response.Mounts {
		mountCopy := mount
		plan.Mounts = append(plan.Mounts, datasetfs.Mount{
			Path: mount.Path, Repository: string(mount.Repository), Commit: string(mount.Commit),
			Directory: &datasetfs.Directory{
				List: func(directory string) ([]datasetfs.DirectoryEntry, error) {
					return readRemoteDirectory(ctx, client, coordinate, mountCopy, directory, response.Pin.PinID)
				},
				Read: func(file string) ([]byte, error) {
					return readRemoteFile(ctx, client, coordinate, mountCopy, file, response.Pin.PinID)
				},
			},
		})
		manifest.Mounts = append(manifest.Mounts, datasetFSMount{
			Path: mount.Path, Mountpoint: filepath.Join(root, filepath.FromSlash(mount.Path)), Repository: mount.Repository, Commit: mount.Commit,
		})
	}
	if _, err := plan.Validate(); err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	closeClient := func() { _ = client.Logout(context.Background()) }
	return plan, manifest, closeClient, nil
}

// resolvedKnowledgeSetPin projects a client task pin onto the Gateway
// contract. pin --out keeps catalog/definition for the client; File Gateway
// DecodeJSON rejects unknown fields on ResolvedKnowledgeSet.
func resolvedKnowledgeSetPin(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	var saved taskKnowledgeSetPin
	if err := catalog.DecodeJSON(raw, &saved); err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--pin is not a KC task pin")
	}
	stripped, err := json.Marshal(saved.ResolvedKnowledgeSet)
	if err != nil {
		return nil, err
	}
	return stripped, nil
}

func datasetFSPin(value string) (json.RawMessage, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return nil, nil
	}
	if !strings.HasPrefix(raw, "{") {
		content, err := os.ReadFile(raw)
		if err != nil {
			return nil, err
		}
		raw = string(content)
	}
	if !json.Valid([]byte(raw)) {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--pin must contain ResolvedKnowledgeSet JSON or name a readable JSON file")
	}
	return json.RawMessage(raw), nil
}

func readRemoteDirectory(ctx context.Context, client *kcclient.Client, coordinate kcclient.WorkspaceFileCoordinate, mount catalog.VirtualMount, directory, pinID string) ([]datasetfs.DirectoryEntry, error) {
	entries := []datasetfs.DirectoryEntry{}
	continuation := ""
	for {
		var response kcclient.WorkspaceFileDirectoryResponse
		err := client.WorkspaceFilesService().Directory(ctx, kcclient.WorkspaceFileDirectoryRequest{
			WorkspaceFileCoordinate: coordinate, MountPath: mount.Path, Directory: directory, Limit: 256, Continuation: continuation,
		}, kcclient.RequestOptions{}, &response)
		if err != nil {
			return nil, err
		}
		if response.Pin.PinID != pinID || response.Mount.Path != mount.Path || response.Mount.Repository != mount.Repository || response.Mount.Commit != mount.Commit {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Knowledge Set File Gateway moved from the fixed mount pin")
		}
		for _, entry := range response.Entries {
			entries = append(entries, datasetfs.DirectoryEntry{Name: entry.Name, Directory: entry.Kind == "directory"})
		}
		if response.Exhausted {
			return entries, nil
		}
		if response.Continuation == "" || response.Continuation == continuation {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Knowledge Set File Gateway returned a non-advancing directory continuation")
		}
		continuation = response.Continuation
	}
}

func readRemoteFile(ctx context.Context, client *kcclient.Client, coordinate kcclient.WorkspaceFileCoordinate, mount catalog.VirtualMount, file, pinID string) ([]byte, error) {
	content := []byte{}
	var offset int64
	for {
		var response kcclient.WorkspaceFileReadResponse
		err := client.WorkspaceFilesService().Read(ctx, kcclient.WorkspaceFileReadRequest{
			WorkspaceFileCoordinate: coordinate, MountPath: mount.Path, File: file, Offset: offset, Length: 512 << 10,
		}, kcclient.RequestOptions{}, &response)
		if err != nil {
			return nil, err
		}
		if response.Pin.PinID != pinID || response.Mount.Path != mount.Path || response.Mount.Repository != mount.Repository || response.Mount.Commit != mount.Commit || response.Offset != offset {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Knowledge Set File Gateway moved from the fixed file pin")
		}
		content = append(content, response.Content...)
		offset += int64(len(response.Content))
		if response.EOF {
			if response.TotalBytes != offset {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Knowledge Set File Gateway returned an inconsistent file length")
			}
			return content, nil
		}
		if len(response.Content) == 0 {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Knowledge Set File Gateway returned a non-advancing file range")
		}
	}
}

func (p datasetFSProjection) build() (datasetfs.Plan, datasetFSManifest, error) {
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
					repositoryDirectory, err := datasetFSRepositoryPath(mountCopy.SubPath, relativeDirectory)
					if err != nil {
						return nil, err
					}
					return readAllDirectoryPages(directory, mountCopy.Commit, repositoryDirectory)
				},
				Read: func(relativeFile string) ([]byte, error) {
					repositoryPath, err := datasetFSRepositoryPath(mountCopy.SubPath, relativeFile)
					if err == nil {
						var data []byte
						data, err = tree.ReadFile(repositoryPath, mountCopy.Commit)
						if err == nil {
							return p.recordKnowledgeSetFSRead(mountCopy, relativeFile, data, nil)
						}
					}
					_, _ = p.recordKnowledgeSetFSRead(mountCopy, relativeFile, nil, err)
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

func datasetFSRepositoryPath(subPath, relative string) (string, error) {
	joined := strings.Trim(path.Join(strings.Trim(subPath, "/"), strings.Trim(relative, "/")), "/")
	if relative != "" && (!validKnowledgeSetFSRelative(relative) || joined == "") {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "invalid mount-relative path %q", relative)
	}
	return joined, nil
}

func validKnowledgeSetFSRelative(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.HasPrefix(value, "../") &&
		!strings.HasPrefix(value, "/") && !strings.Contains(value, "\\") && !strings.ContainsRune(value, '\x00')
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

func (p datasetFSProjection) recordKnowledgeSetFSRead(mount catalog.VirtualMount, relative string, data []byte, readErr error) ([]byte, error) {
	virtualPath := path.Join(mount.Path, relative)
	result := map[string]any{"path": virtualPath, "repository": mount.Repository, "commit": mount.Commit}
	readFlags := make(map[string]FlagValue, len(p.flags)+2)
	for name, value := range p.flags {
		readFlags[name] = value
	}
	readFlags["path"] = virtualPath
	readFlags["_action"] = "file.read"
	if _, _, err := recordKnowledgeAccess(p.home, "file-read", readFlags, result, readErr); err != nil && readErr == nil {
		return nil, err
	}
	return data, nil
}

func datasetFSMayReadRepository(home string, flags map[string]FlagValue, repository string, opened ...*Home) (bool, error) {
	if ownerBypass(flags) {
		return true, nil
	}
	if allowedDatasetFileRead(home, flags) {
		return true, nil
	}
	catalogID := FlagString(flags, "catalog")
	if catalogID == "" {
		catalogID = FlagString(flags, "_default-catalog")
	}
	return PrincipalAllowed(home, FlagString(flags, "as"), "file.read", repository, catalogID, opened...), nil
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

func writeKnowledgeSetFSJSON(w io.Writer, value any) {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func writeKnowledgeSetFSError(w io.Writer, err error) {
	writeKnowledgeSetFSJSON(w, kernel.FaultJSON(err))
}
