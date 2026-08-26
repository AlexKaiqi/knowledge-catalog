package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"kc/catalog"
	"kc/kernel"
	"kc/snapshot"
	"kc/workspacefs"
)

type workspaceFSConfig struct {
	home      string
	catalogID string
	workspace string
	root      string
	principal string
	pin       string
	debug     bool
}

type workspaceFSMount struct {
	Path       string              `json:"path"`
	Mountpoint string              `json:"mountpoint"`
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	Files      int                 `json:"files"`
}

type workspaceFSManifest struct {
	WorkspaceID string             `json:"workspaceId"`
	PinID       string             `json:"pinId"`
	Root        string             `json:"root"`
	ReadOnly    bool               `json:"readOnly"`
	Mounts      []workspaceFSMount `json:"mounts"`
}

// RunWorkspaceFS is the entrypoint used by cmd/kcfs. Host mounting is kept
// outside the kc verb table because it must execute on the user's Linux host;
// unlike protocol verbs it must never be mirrored as POST /v1/<verb>.
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
	if mode != "mount" && mode != "plan" {
		writeWorkspaceFSError(stderr, fmt.Errorf("unknown kcfs command %s", mode))
		return 2
	}
	config, err := parseWorkspaceFSConfig(mode, argv, stderr)
	if err != nil {
		writeWorkspaceFSError(stderr, err)
		return 2
	}
	plan, manifest, closeView, err := prepareWorkspaceFS(config)
	if err != nil {
		writeWorkspaceFSError(stderr, err)
		return 1
	}
	defer closeView()
	if mode == "plan" {
		writeWorkspaceFSJSON(stdout, manifest)
		return 0
	}
	session, err := workspacefs.MountAll(plan, workspacefs.Options{Debug: config.debug})
	if err != nil {
		writeWorkspaceFSError(stderr, err)
		return 1
	}
	writeWorkspaceFSJSON(stdout, manifest)
	done := make(chan struct{})
	go func() {
		session.Wait()
		close(done)
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case <-signals:
		if err := session.Unmount(); err != nil {
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
	set.StringVar(&config.home, "home", ".kc", "kc home directory")
	set.StringVar(&config.catalogID, "catalog", "", "Catalog id (defaults to the home's first Catalog)")
	set.StringVar(&config.workspace, "workspace", "", "Workspace id")
	set.StringVar(&config.root, "root", "", "existing user project root")
	set.StringVar(&config.principal, "as", "", "principal used for Workspace and repository read grants")
	set.StringVar(&config.pin, "pin", "", "ResolvedWorkspace JSON or file to replay")
	set.BoolVar(&config.debug, "debug", false, "enable go-fuse protocol logging")
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
	return config, nil
}

func prepareWorkspaceFS(config workspaceFSConfig) (workspacefs.Plan, workspaceFSManifest, func(), error) {
	home, err := filepath.Abs(config.home)
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	root, err := filepath.Abs(config.root)
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, fmt.Errorf("resolve workspace root %s: %w", config.root, err)
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
	if err := authorize(home, "read-workspace", flags); err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	ws, err := Open(home)
	if err != nil {
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	closeView := func() { _ = ws.Close() }
	cat, err := pickCatalog(ws, flags)
	if err != nil {
		closeView()
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	def, err := effectiveWorkspace(ws, home, cat, config.workspace, flags)
	if err != nil {
		closeView()
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	resolved, err := resolveOrReplay(ws, home, cat, config.workspace, flags)
	if err != nil {
		closeView()
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	mounts, err := catalog.ListVirtualMountsAt(def, resolved)
	if err != nil {
		closeView()
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	entries, err := cat.ListVirtualFilesAt(def, resolved)
	if err != nil {
		closeView()
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	plan := workspacefs.Plan{WorkspaceID: config.workspace, PinID: resolved.PinID, Root: root}
	manifest := workspaceFSManifest{WorkspaceID: config.workspace, PinID: resolved.PinID, Root: root, ReadOnly: true, Mounts: []workspaceFSMount{}}
	for _, mount := range mounts {
		allowed, err := workspaceFSMayReadRepository(home, flags, string(mount.Repository))
		if err != nil {
			closeView()
			return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
		}
		if !allowed {
			continue
		}
		store, ok := ws.Store.Get(mount.Repository)
		if !ok {
			closeView()
			return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, kernel.Fail(kernel.ErrUsageInvalid, "repository %s is not mounted", mount.Repository)
		}
		if _, ok := snapshot.TreeStoreOf(store); !ok {
			closeView()
			return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"repository %s does not support raw path reads required by kcfs", mount.Repository)
		}
		one := workspacefs.Mount{Path: mount.Path, Repository: string(mount.Repository), Commit: string(mount.Commit)}
		for _, entry := range entries {
			if entry.Repository != mount.Repository {
				continue
			}
			rel, ok := relativeMountPath(mount.Path, entry.Path)
			if !ok {
				continue
			}
			if rel == "" {
				closeView()
				return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, fmt.Errorf(
					"mount %s maps a single file to its mountpoint; kcfs currently mounts directory subtrees", mount.Path)
			}
			virtualPath := entry.Path
			one.Files = append(one.Files, workspacefs.File{
				Path: rel,
				Read: func() ([]byte, error) {
					file, err := cat.ReadVirtualFileAt(def, resolved, virtualPath)
					result := map[string]any{
						"path": virtualPath, "repository": mount.Repository, "commit": mount.Commit,
					}
					readFlags := make(map[string]FlagValue, len(flags)+1)
					for name, value := range flags {
						readFlags[name] = value
					}
					readFlags["path"] = virtualPath
					if accessErr := recordKnowledgeAccess(home, "vfs-read", readFlags, result, err); accessErr != nil && err == nil {
						return nil, accessErr
					}
					if err != nil {
						return nil, err
					}
					return file.Content, nil
				},
			})
		}
		plan.Mounts = append(plan.Mounts, one)
		manifest.Mounts = append(manifest.Mounts, workspaceFSMount{
			Path: mount.Path, Mountpoint: filepath.Join(root, filepath.FromSlash(mount.Path)),
			Repository: mount.Repository, Commit: mount.Commit, Files: len(one.Files),
		})
	}
	if _, err := plan.Validate(); err != nil {
		closeView()
		return workspacefs.Plan{}, workspaceFSManifest{}, func() {}, err
	}
	return plan, manifest, closeView, nil
}

func workspaceFSMayReadRepository(home string, flags map[string]FlagValue, repository string) (bool, error) {
	if ownerBypass(flags) {
		return true, nil
	}
	file, err := ReadAllow(home)
	if err != nil {
		return false, err
	}
	_, ok := MatchAllow(file.Rules, AllowQuery{
		Principal: FlagString(flags, "as"), Cmd: "read", Repo: repository,
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

const workspaceFSHelp = `kcfs mounts a resolved Knowledge Catalog Workspace into an existing Linux project.

Usage:
  kcfs plan  --home <dir> [--catalog <id>] --workspace <id> --root <project>
  kcfs mount --home <dir> [--catalog <id>] --workspace <id> --root <project>

Each Workspace source Path becomes an independent read-only FUSE mount below
--root. The process resolves selectors once, prints the pin and mount manifest,
then serves until SIGINT or SIGTERM. A mountpoint must be absent or empty.

Linux requirements: /dev/fuse and fusermount3 (usually the distro fuse3 package).
`
