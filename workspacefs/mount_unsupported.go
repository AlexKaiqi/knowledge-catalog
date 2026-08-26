//go:build !linux

package workspacefs

import "fmt"

type Options struct {
	Debug bool
}

type Session struct{}

func MountAll(plan Plan, _ Options) (*Session, error) {
	return nil, fmt.Errorf("kcfs host mounts require Linux with /dev/fuse and fusermount3")
}

func (s *Session) Wait() {}

func (s *Session) Unmount() error { return nil }
