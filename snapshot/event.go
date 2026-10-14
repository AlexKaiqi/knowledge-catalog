package snapshot

import "kc/kernel"

// Advanced is a pure layer ⓪ ref movement. Changed object identities are a
// layer ② concern and intentionally do not appear in this event.
type Advanced struct {
	Store Store
	From  kernel.CommitID
	To    kernel.CommitID
}
