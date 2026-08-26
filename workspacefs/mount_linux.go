//go:build linux

package workspacefs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"sync"
	"syscall"
	"time"

	goFS "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Options controls the Linux FUSE adapter. The mounted data itself is always
// read-only and immutable at one Workspace pin.
type Options struct {
	Debug bool
}

// MountHandle owns all FUSE servers created for one Workspace pin.
type MountHandle struct {
	servers []*fuse.Server
	created []string
	once    sync.Once
	err     error
}

// MountAll attaches every declared Workspace mount to its own directory below
// Plan.Root. go-fuse owns FUSE protocol handling; this package only supplies
// the immutable tree and path-to-byte reader.
func MountAll(plan Plan, options Options) (*MountHandle, error) {
	targets, err := plan.Validate()
	if err != nil {
		return nil, err
	}
	handle := &MountHandle{}
	for _, target := range targets {
		created, err := prepareMountpoint(target.projectRoot, target.Mountpoint)
		if err != nil {
			handle.Unmount()
			return nil, err
		}
		handle.created = append(handle.created, created...)
		root := &dirNode{tree: target.root}
		oneSecond := time.Second
		server, err := goFS.Mount(target.Mountpoint, root, &goFS.Options{
			MountOptions: fuse.MountOptions{
				Debug:  options.Debug,
				FsName: "kcfs:" + target.Mount.Repository + "@" + target.Mount.Commit,
				Name:   "kcfs",
				Options: []string{
					"ro",
				},
			},
			EntryTimeout:    &oneSecond,
			AttrTimeout:     &oneSecond,
			NegativeTimeout: &oneSecond,
		})
		if err != nil {
			handle.Unmount()
			return nil, fmt.Errorf("mount %s at %s: %w", target.Mount.Repository, target.Mountpoint, err)
		}
		handle.servers = append(handle.servers, server)
	}
	return handle, nil
}

func prepareMountpoint(root, target string) ([]string, error) {
	created, err := missingDirectories(root, target)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(target)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return nil, err
		}
		return created, nil
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("mountpoint %s is not a directory", target)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return nil, err
	}
	if len(entries) != 0 {
		return nil, fmt.Errorf("mountpoint %s is not empty", target)
	}
	return nil, nil
}

// Wait blocks until every mounted server has stopped.
func (h *MountHandle) Wait() {
	for _, server := range h.servers {
		server.Wait()
	}
}

// Unmount detaches all mounts in reverse order and removes only mountpoint
// directories that this mount operation created and that remain empty.
func (h *MountHandle) Unmount() error {
	h.once.Do(func() {
		for i := len(h.servers) - 1; i >= 0; i-- {
			if err := h.servers[i].Unmount(); err != nil && h.err == nil {
				h.err = err
			}
		}
		for i := len(h.created) - 1; i >= 0; i-- {
			_ = os.Remove(h.created[i])
		}
	})
	return h.err
}

type dirNode struct {
	goFS.Inode
	tree *tree
}

func (n *dirNode) OnAdd(ctx context.Context) {
	for name, dir := range n.tree.dirs {
		child := n.NewPersistentInode(ctx, &dirNode{tree: dir}, goFS.StableAttr{Mode: syscall.S_IFDIR})
		n.AddChild(name, child, false)
	}
	for name, file := range n.tree.files {
		child := n.NewPersistentInode(ctx, &fileNode{file: file}, goFS.StableAttr{Mode: syscall.S_IFREG})
		n.AddChild(name, child, false)
	}
}

func (n *dirNode) Getattr(_ context.Context, _ goFS.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = syscall.S_IFDIR | 0o555
	return 0
}

func (n *dirNode) Readdir(_ context.Context) (goFS.DirStream, syscall.Errno) {
	entries := make([]fuse.DirEntry, 0, len(n.tree.dirs)+len(n.tree.files))
	for name := range n.tree.dirs {
		entries = append(entries, fuse.DirEntry{Name: name, Mode: syscall.S_IFDIR})
	}
	for name := range n.tree.files {
		entries = append(entries, fuse.DirEntry{Name: name, Mode: syscall.S_IFREG})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return goFS.NewListDirStream(entries), 0
}

type fileNode struct {
	goFS.Inode
	file File
	mu   sync.Mutex
	ok   bool
	data []byte
}

func (n *fileNode) load() ([]byte, syscall.Errno) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ok {
		return n.data, 0
	}
	data, err := n.file.Read()
	if err == nil {
		n.data, n.ok = data, true
		return n.data, 0
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil, syscall.ENOENT
	}
	if errors.Is(err, fs.ErrPermission) {
		return nil, syscall.EACCES
	}
	return nil, syscall.EIO
}

func (n *fileNode) Getattr(_ context.Context, _ goFS.FileHandle, out *fuse.AttrOut) syscall.Errno {
	data, errno := n.load()
	if errno != 0 {
		return errno
	}
	out.Mode = syscall.S_IFREG | 0o444
	out.Size = uint64(len(data))
	return 0
}

func (n *fileNode) Open(_ context.Context, flags uint32) (goFS.FileHandle, uint32, syscall.Errno) {
	if flags&syscall.O_ACCMODE != syscall.O_RDONLY {
		return nil, 0, syscall.EROFS
	}
	if _, errno := n.load(); errno != 0 {
		return nil, 0, errno
	}
	return n, fuse.FOPEN_KEEP_CACHE, 0
}

func (n *fileNode) Read(_ context.Context, _ goFS.FileHandle, _ []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	data, errno := n.load()
	if errno != 0 {
		return nil, errno
	}
	if off < 0 {
		return nil, syscall.EINVAL
	}
	if off >= int64(len(data)) {
		return fuse.ReadResultData(nil), 0
	}
	return fuse.ReadResultData(data[off:]), 0
}

var _ goFS.NodeOnAdder = (*dirNode)(nil)
var _ goFS.NodeGetattrer = (*dirNode)(nil)
var _ goFS.NodeReaddirer = (*dirNode)(nil)
var _ goFS.NodeGetattrer = (*fileNode)(nil)
var _ goFS.NodeOpener = (*fileNode)(nil)
var _ goFS.NodeReader = (*fileNode)(nil)
