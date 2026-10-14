package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	inv "kc/invocation"

	snapshotclient "kc/snapshot/client"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	kcclient "kc/client"
	"kc/datasetfs"
	"kc/kernel"
	"kc/snapshot/dataset"
)

type datasetFSConfig struct {
	server    string
	dataset   string
	root      string
	principal string
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
	Pin      dataset.ResolvedKnowledgeSet `json:"pin"`
	Root     string                       `json:"root"`
	ReadOnly bool                         `json:"readOnly"`
	Mounts   []datasetFSMount             `json:"mounts"`
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
	var pin string
	set.StringVar(&config.server, "server", inv.RemoteServerURL(nil), "KC service URL for remote lazy reads")
	set.StringVar(&config.dataset, "dataset", "", "knowledge set id")
	set.StringVar(&config.root, "root", "", "existing user project root")
	set.StringVar(&config.principal, "as", "", "principal used for Workspace and repository read grants")
	set.StringVar(&pin, "pin", "", "ResolvedKnowledgeSet JSON or file to replay")
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
	if explicit["pin"] || strings.TrimSpace(pin) != "" {
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
	coordinate := snapshotclient.WorkspaceFileCoordinate{Dataset: config.dataset, View: config.view}
	var response snapshotclient.WorkspaceFileMountsResponse
	if err := client.WorkspaceFilesService().Mounts(ctx, snapshotclient.WorkspaceFileMountsRequest{WorkspaceFileCoordinate: coordinate}, snapshotclient.RequestOptions{}, &response); err != nil {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, err
	}
	if response.Pin.PinID == "" || response.Pin.SetID != config.dataset {
		return datasetfs.Plan{}, datasetFSManifest{}, func() {}, kernel.Fail(kernel.ErrPreconditionFailed, "Knowledge Set File Gateway returned an invalid pin")
	}
	// Per-file entries are not projected into the host filesystem yet; fail
	// loudly instead of silently shrinking the delivered tree.
	for _, item := range response.Pin.Items {
		if item.Kind == dataset.DatasetItemFile {
			return datasetfs.Plan{}, datasetFSManifest{}, func() {}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"dataset %s publishes per-file entries that the file-system projection does not deliver yet; use the file gateway or kc dataset clone", config.dataset)
		}
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

func readRemoteDirectory(ctx context.Context, client *kcclient.Client, coordinate snapshotclient.WorkspaceFileCoordinate, mount dataset.VirtualMount, directory, pinID string) ([]datasetfs.DirectoryEntry, error) {
	entries := []datasetfs.DirectoryEntry{}
	continuation := ""
	for {
		var response snapshotclient.WorkspaceFileDirectoryResponse
		err := client.WorkspaceFilesService().Directory(ctx, snapshotclient.WorkspaceFileDirectoryRequest{
			WorkspaceFileCoordinate: coordinate, MountPath: mount.Path, Directory: directory, Limit: 256, Continuation: continuation,
		}, snapshotclient.RequestOptions{}, &response)
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

func readRemoteFile(ctx context.Context, client *kcclient.Client, coordinate snapshotclient.WorkspaceFileCoordinate, mount dataset.VirtualMount, file, pinID string) ([]byte, error) {
	content := []byte{}
	var offset int64
	for {
		var response snapshotclient.WorkspaceFileReadResponse
		err := client.WorkspaceFilesService().Read(ctx, snapshotclient.WorkspaceFileReadRequest{
			WorkspaceFileCoordinate: coordinate, MountPath: mount.Path, File: file, Offset: offset, Length: 512 << 10,
		}, snapshotclient.RequestOptions{}, &response)
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

func writeKnowledgeSetFSJSON(w io.Writer, value any) {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func writeKnowledgeSetFSError(w io.Writer, err error) {
	writeKnowledgeSetFSJSON(w, kernel.FaultJSON(err))
}
