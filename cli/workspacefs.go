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
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
	"kc/workspacefs"
)

type workspaceFSConfig struct {
	home      string
	server    string
	catalogID string
	workspace string
	root      string
	principal string
	pin       string
	debug     bool
	view      string
}

type workspaceFSMount struct {
	Path       string              `json:"path"`
	Mountpoint string              `json:"mountpoint"`
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
}

type workspaceFSManifest struct {
	WorkspaceID string                    `json:"workspaceId"`
	PinID       string                    `json:"pinId"`
	Pin         catalog.ResolvedWorkspace `json:"pin"`
	Root        string                    `json:"root"`
	ReadOnly    bool                      `json:"readOnly"`
	Mounts      []workspaceFSMount        `json:"mounts"`
}

// workspaceFSProjection is the frozen, authorized input used by embedded
// application tests. Public kcfs obtains the same information through the
// Workspace File Gateway.
type workspaceFSProjection struct {
	home       string
	root       string
	workspace  string
	flags      map[string]FlagValue
	homeState  *Home
	catalog    *catalog.Catalog
	definition catalog.WorkspaceDefinition
	resolved   catalog.ResolvedWorkspace
	mounts     []catalog.VirtualMount
}

// RunWorkspaceFS is the entrypoint used by cmd/kcfs. Host mounting is kept
// outside the kc verb table because it must execute on the user's Linux host;
// unlike protocol commands it must never be mirrored as a domain HTTP route.
func RunWorkspaceFS(argv []string, stdout, stderr io.Writer) int {
	if len(argv) > 0 && argv[0] == "--" {
		argv = argv[1:]
	}
	mode := ""
	if len(argv) > 0 && !strings.HasPrefix(argv[0], "-") {
		mode = argv[0]
		argv = argv[1:]
	}
	if mode == "help" || mode == "--help" || mode == "-h" || mode == "" {
		_, _ = io.WriteString(stdout, workspaceFSHelp)
		return 0
	}
	if mode == "stop" {
		return stopWorkspaceFS(argv, stderr)
	}
	if mode == "daemon-mount" {
		return daemonMountWorkspaceFS(argv, stdout, stderr)
	}
	if mode != "mount" && mode != "plan" {
		writeWorkspaceFSError(stderr, fmt.Errorf("unknown kcfs command %s", mode))
		return 2
	}
	config, err := parseWorkspaceFSConfig(mode, argv, stderr)
	if err != nil {
		writeWorkspaceFSError(stderr, err)
		return 2
	}
	plan, manifest, closeHome, err := prepareWorkspaceFS(config)
	if err != nil {
		writeWorkspaceFSError(stderr, err)
		return 1
	}
	defer closeHome()
	if mode == "plan" {
		writeWorkspaceFSJSON(stdout, manifest)
		return 0
	}
	handle, err := workspacefs.MountAll(plan, workspacefs.Options{Debug: config.debug})
	if err != nil {
		writeWorkspaceFSError(stderr, err)
		return 1
	}
	writeWorkspaceFSJSON(stdout, manifest)
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
			writeWorkspaceFSError(stderr, err)
			return 1
		}
		<-done
	case <-done:
	}
	return 0
}

func parseWorkspaceFSConfig(mode string, argv []string, stderr io.Writer) (workspaceFSConfig, error) {
	set := flag.NewFlagSet("kcfs "+mode, flag.ContinueOnError)
	set.SetOutput(stderr)
	config := workspaceFSConfig{}
	set.StringVar(&config.server, "server", strings.TrimSpace(os.Getenv("KC_SERVER_URL")), "KC service URL for remote lazy reads")
	set.StringVar(&config.catalogID, "catalog", "", "Catalog id (defaults to the home's first Catalog)")
	set.StringVar(&config.workspace, "workspace", "", "Workspace id")
	set.StringVar(&config.root, "root", "", "existing user project root")
	set.StringVar(&config.principal, "as", "", "principal used for Workspace and repository read grants")
	set.StringVar(&config.pin, "pin", "", "ResolvedWorkspace JSON or file to replay")
	set.BoolVar(&config.debug, "debug", false, "enable go-fuse protocol logging")
	set.StringVar(&config.view, "view", "repository", "file view: repository or semantic")
	if err := set.Parse(argv); err != nil {
		return workspaceFSConfig{}, err
	}
	if set.NArg() != 0 {
		return workspaceFSConfig{}, fmt.Errorf("unexpected argument %s", set.Arg(0))
	}
	if strings.TrimSpace(config.workspace) == "" {
		return workspaceFSConfig{}, fmt.Errorf("missing --workspace")
	}
	if strings.TrimSpace(config.root) == "" {
		return workspaceFSConfig{}, fmt.Errorf("missing --root")
	}
	if config.view != "repository" && config.view != "semantic" {
		return workspaceFSConfig{}, fmt.Errorf("--view must be repository or semantic")
	}
	return config, nil
}

func prepareWorkspaceFS(config workspaceFSConfig) (workspacefs.Plan, workspaceFSManifest, func(), error) {
	root, err := filepath.Abs(config.root)
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, fmt.Errorf("resolve workspace root %s: %w", config.root, err)
	}
	if strings.TrimSpace(config.server) == "" {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, kernel.Fail(kernel.ErrUsageInvalid,
			"kcfs requires KC Server; set --server or KC_SERVER_URL")
	}
	return prepareRemoteWorkspaceFS(config, root)
}

// prepareEmbeddedWorkspaceFS is a conformance-test seam, not a product
// transport. It proves plan semantics against the shared application services.
func prepareEmbeddedWorkspaceFS(config workspaceFSConfig) (workspacefs.Plan, workspaceFSManifest, func(), error) {
	root, err := filepath.Abs(config.root)
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, fmt.Errorf("resolve workspace root %s: %w", config.root, err)
	}
	home, err := filepath.Abs(config.home)
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	flags := map[string]FlagValue{
		"home":      home,
		"workspace": config.workspace,
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
	if err := authorize(home, "workspace.resolve", flags, nil); err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	ws, err := Open(home)
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
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
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	def, err := effectiveWorkspace(ws, home, cat, config.workspace, flags)
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	resolved, err := resolveOrReplay(ws, home, cat, config.workspace, flags)
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	mounts, err := catalog.ListVirtualMountsAt(def, resolved)
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	visibleDef, visibleResolved, visibleMounts, err := filterVirtualWorkspace(def, resolved, mounts, func(repository kernel.RepositoryID) (bool, error) {
		return workspaceFSMayReadRepository(home, flags, string(repository))
	})
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	projection := workspaceFSProjection{
		home: home, root: root, workspace: config.workspace, flags: flags,
		homeState: ws, catalog: cat, definition: visibleDef, resolved: visibleResolved,
		mounts: visibleMounts,
	}
	plan, manifest, err := projection.build()
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	keepOpen = true
	return plan, manifest, closeHome, nil
}

func prepareRemoteWorkspaceFS(config workspaceFSConfig, root string) (workspacefs.Plan, workspaceFSManifest, func(), error) {
	principal := strings.TrimSpace(config.principal)
	if principal == "" {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, kernel.Fail(kernel.ErrUnauthenticated, "remote kcfs requires --as")
	}
	authentication := strings.TrimSpace(os.Getenv("KC_AUTH_TOKEN"))
	var authenticator kcclient.Authenticator
	if authentication != "" {
		if !strings.Contains(authentication, " ") {
			authentication = "Bearer " + authentication
		}
		authenticator = remoteTokenAuthenticator{}
	}
	client, err := kcclient.New(kcclient.Config{BaseURL: config.server, Authenticator: authenticator})
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	ctx := context.Background()
	if _, err := client.Login(ctx, kcclient.LoginRequest{
		Identity: kcclient.Identity{Principal: principal}, Authentication: kcclient.Authentication{Authorization: authentication},
	}); err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	pin, err := workspaceFSPin(config.pin)
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	coordinate := kcclient.WorkspaceFileCoordinate{Catalog: config.catalogID, Workspace: config.workspace, Pin: pin, View: config.view}
	var response kcclient.WorkspaceFileMountsResponse
	if err := client.WorkspaceFilesService().Mounts(ctx, kcclient.WorkspaceFileMountsRequest{WorkspaceFileCoordinate: coordinate}, kcclient.RequestOptions{}, &response); err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	if response.Pin.PinID == "" || response.Pin.WorkspaceID != config.workspace {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, kernel.Fail(kernel.ErrPreconditionFailed, "Workspace File Gateway returned an invalid pin")
	}
	pinned, err := json.Marshal(response.Pin)
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	coordinate.Pin = pinned
	plan := workspacefs.Plan{WorkspaceID: config.workspace, PinID: response.Pin.PinID, Root: root}
	manifest := workspaceFSManifest{WorkspaceID: config.workspace, PinID: response.Pin.PinID, Pin: response.Pin, Root: root, ReadOnly: true, Mounts: []workspaceFSMount{}}
	for _, mount := range response.Mounts {
		mountCopy := mount
		plan.Mounts = append(plan.Mounts, workspacefs.Mount{
			Path: mount.Path, Repository: string(mount.Repository), Commit: string(mount.Commit),
			Directory: &workspacefs.Directory{
				List: func(directory string) ([]workspacefs.DirectoryEntry, error) {
					return readRemoteDirectory(ctx, client, coordinate, mountCopy, directory, response.Pin.PinID)
				},
				Read: func(file string) ([]byte, error) {
					return readRemoteFile(ctx, client, coordinate, mountCopy, file, response.Pin.PinID)
				},
			},
		})
		manifest.Mounts = append(manifest.Mounts, workspaceFSMount{
			Path: mount.Path, Mountpoint: filepath.Join(root, filepath.FromSlash(mount.Path)), Repository: mount.Repository, Commit: mount.Commit,
		})
	}
	if _, err := plan.Validate(); err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	closeClient := func() { _ = client.Logout(context.Background()) }
	return plan, manifest, closeClient, nil
}

func workspaceFSPin(value string) (json.RawMessage, error) {
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
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--pin must contain ResolvedWorkspace JSON or name a readable JSON file")
	}
	return json.RawMessage(raw), nil
}

func readRemoteDirectory(ctx context.Context, client *kcclient.Client, coordinate kcclient.WorkspaceFileCoordinate, mount catalog.VirtualMount, directory, pinID string) ([]workspacefs.DirectoryEntry, error) {
	entries := []workspacefs.DirectoryEntry{}
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
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Workspace File Gateway moved from the fixed mount pin")
		}
		for _, entry := range response.Entries {
			entries = append(entries, workspacefs.DirectoryEntry{Name: entry.Name, Directory: entry.Kind == "directory"})
		}
		if response.Exhausted {
			return entries, nil
		}
		if response.Continuation == "" || response.Continuation == continuation {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Workspace File Gateway returned a non-advancing directory continuation")
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
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Workspace File Gateway moved from the fixed file pin")
		}
		content = append(content, response.Content...)
		offset += int64(len(response.Content))
		if response.EOF {
			if response.TotalBytes != offset {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Workspace File Gateway returned an inconsistent file length")
			}
			return content, nil
		}
		if len(response.Content) == 0 {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Workspace File Gateway returned a non-advancing file range")
		}
	}
}

func (p workspaceFSProjection) build() (workspacefs.Plan, workspaceFSManifest, error) {
	plan := workspacefs.Plan{WorkspaceID: p.workspace, PinID: p.resolved.PinID, Root: p.root}
	manifest := workspaceFSManifest{WorkspaceID: p.workspace, PinID: p.resolved.PinID, Pin: p.resolved, Root: p.root, ReadOnly: true, Mounts: []workspaceFSMount{}}
	for _, mount := range p.mounts {
		store, ok := p.homeState.Store.Get(mount.Repository)
		if !ok {
			return workspacefs.Plan{}, workspaceFSManifest{}, kernel.Fail(kernel.ErrUsageInvalid, "repository %s is not attached", mount.Repository)
		}
		tree, ok := snapshot.TreeStoreOf(store)
		if !ok {
			return workspacefs.Plan{}, workspaceFSManifest{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"repository %s does not support raw path reads required by kcfs", mount.Repository)
		}
		directory, ok := store.(snapshot.DirectoryReader)
		if !ok {
			return workspacefs.Plan{}, workspaceFSManifest{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"repository %s does not support lazy directory reads required by kcfs", mount.Repository)
		}
		mountCopy := mount
		one := workspacefs.Mount{
			Path: mount.Path, Repository: string(mount.Repository), Commit: string(mount.Commit),
			Directory: &workspacefs.Directory{
				List: func(relativeDirectory string) ([]workspacefs.DirectoryEntry, error) {
					repositoryDirectory, err := workspaceFSRepositoryPath(mountCopy.SubPath, relativeDirectory)
					if err != nil {
						return nil, err
					}
					return readAllDirectoryPages(directory, mountCopy.Commit, repositoryDirectory)
				},
				Read: func(relativeFile string) ([]byte, error) {
					repositoryPath, err := workspaceFSRepositoryPath(mountCopy.SubPath, relativeFile)
					if err == nil {
						var data []byte
						data, err = tree.ReadFile(repositoryPath, mountCopy.Commit)
						if err == nil {
							return p.recordWorkspaceFSRead(mountCopy, relativeFile, data, nil)
						}
					}
					_, _ = p.recordWorkspaceFSRead(mountCopy, relativeFile, nil, err)
					return nil, err
				},
			},
		}
		plan.Mounts = append(plan.Mounts, one)
		manifest.Mounts = append(manifest.Mounts, workspaceFSMount{
			Path: mount.Path, Mountpoint: filepath.Join(p.root, filepath.FromSlash(mount.Path)),
			Repository: mount.Repository, Commit: mount.Commit,
		})
	}
	if _, err := plan.Validate(); err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, err
	}
	return plan, manifest, nil
}

func workspaceFSRepositoryPath(subPath, relative string) (string, error) {
	joined := strings.Trim(path.Join(strings.Trim(subPath, "/"), strings.Trim(relative, "/")), "/")
	if relative != "" && (!validWorkspaceFSRelative(relative) || joined == "") {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "invalid mount-relative path %q", relative)
	}
	return joined, nil
}

func validWorkspaceFSRelative(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.HasPrefix(value, "../") &&
		!strings.HasPrefix(value, "/") && !strings.Contains(value, "\\") && !strings.ContainsRune(value, '\x00')
}

func readAllDirectoryPages(reader snapshot.DirectoryReader, commit kernel.CommitID, directory string) ([]workspacefs.DirectoryEntry, error) {
	var out []workspacefs.DirectoryEntry
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
			out = append(out, workspacefs.DirectoryEntry{Name: entry.Name, Directory: entry.Kind == "directory"})
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

func (p workspaceFSProjection) recordWorkspaceFSRead(mount catalog.VirtualMount, relative string, data []byte, readErr error) ([]byte, error) {
	virtualPath := path.Join(mount.Path, relative)
	result := map[string]any{"path": virtualPath, "repository": mount.Repository, "commit": mount.Commit}
	readFlags := make(map[string]FlagValue, len(p.flags)+2)
	for name, value := range p.flags {
		readFlags[name] = value
	}
	readFlags["path"] = virtualPath
	readFlags["_action"] = "file.read"
	if _, err := recordKnowledgeAccess(p.home, "file-read", readFlags, result, readErr); err != nil && readErr == nil {
		return nil, err
	}
	return data, nil
}

func workspaceFSMayReadRepository(home string, flags map[string]FlagValue, repository string) (bool, error) {
	if ownerBypass(flags) {
		return true, nil
	}
	if repository == string(knowledge.SystemRepositoryID) && FlagString(flags, "as") != "" {
		return true, nil
	}
	file, err := ReadAllow(home)
	if err != nil {
		return false, err
	}
	_, ok := MatchAllow(file.Rules, AllowQuery{
		Principal: FlagString(flags, "as"), Action: "file.read", Repo: repository,
	})
	return ok, nil
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

func writeWorkspaceFSJSON(w io.Writer, value any) {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func writeWorkspaceFSError(w io.Writer, err error) {
	writeWorkspaceFSJSON(w, kernel.FaultJSON(err))
}
