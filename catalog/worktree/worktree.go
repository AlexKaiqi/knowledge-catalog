// Package worktree is the public shim for mount-checkout worktree semantics
// (CAP-ORG-01 B11, ruling C): the implementation lives in
// catalog/internal/worktreeimpl; the app seam keeps this stable path.
package worktree

import "kc/catalog/internal/worktreeimpl"

const MountCheckoutPinFile = worktreeimpl.MountCheckoutPinFile

type (
	// MountCheckout is one checked-out mount record.
	MountCheckout = worktreeimpl.MountCheckout
	// MountCheckoutPin is the persisted pin state.
	MountCheckoutPin = worktreeimpl.MountCheckoutPin
	// MountStatusReport reports one mount's sync state.
	MountStatusReport = worktreeimpl.MountStatusReport
	// MountSync is one mount sync outcome.
	MountSync = worktreeimpl.MountSync
	// MountWrite is one pending mount write.
	MountWrite = worktreeimpl.MountWrite
)

type SyncOutcome = worktreeimpl.SyncOutcome

const (
	SyncCheckedOut = worktreeimpl.SyncCheckedOut
	SyncUnchanged  = worktreeimpl.SyncUnchanged
	SyncAdvanced   = worktreeimpl.SyncAdvanced
	SyncBlocked    = worktreeimpl.SyncBlocked
	SyncSkipped    = worktreeimpl.SyncSkipped
)

var (
	CheckoutMounts            = worktreeimpl.CheckoutMounts
	CheckoutMountsAllowing    = worktreeimpl.CheckoutMountsAllowing
	CheckoutMountsAllowingDef = worktreeimpl.CheckoutMountsAllowingDef
	CollectMountChanges       = worktreeimpl.CollectMountChanges
	MountStatus               = worktreeimpl.MountStatus
	ReadMountCheckoutPin      = worktreeimpl.ReadMountCheckoutPin
	SyncMounts                = worktreeimpl.SyncMounts
	SyncMountsDef             = worktreeimpl.SyncMountsDef
	WriteMountCheckoutPin     = worktreeimpl.WriteMountCheckoutPin
)
