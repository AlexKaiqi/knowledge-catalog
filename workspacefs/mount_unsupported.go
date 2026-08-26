//go:build !linux

package workspacefs

import "fmt"

type Options struct {
	Debug bool
}

type MountHandle struct{}

func MountAll(plan Plan, _ Options) (*MountHandle, error) {
	return nil, fmt.Errorf("kcfs host mounts require Linux with /dev/fuse and fusermount3")
}

func (h *MountHandle) Wait() {}

func (h *MountHandle) Unmount() error { return nil }
