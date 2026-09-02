package cli

import (
	"kc/catalog/worktree"
	"kc/kernel"
)

func verbSync(cx *invocation) (any, error) {
	workspaceID, err := workspaceIDFlag(cx.Flags)
	if err != nil {
		return nil, err
	}
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	def, err := effectiveWorkspace(cx.WS, cx.Home, cat, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	dest, err := checkoutDest(cx.WS, cx.Home, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	syncs, err := worktree.SyncMountsDef(cat, def, dest)
	if err != nil {
		return nil, err
	}
	return map[string]any{"workspaceId": workspaceID, "dir": homeRel(cx.Home, dest), "mounts": syncs}, nil
}

func statusMounts(cx *invocation) (any, error) {
	workspaceID, err := workspaceIDFlag(cx.Flags)
	if err != nil {
		if to := cx.flag("to"); to != "" {
			pin, pinErr := worktree.ReadMountCheckoutPin(to)
			if pinErr != nil {
				return nil, pinErr
			}
			if pin == nil {
				return nil, kernel.Fail(kernel.ErrUsageInvalid, "%s has never been checked out; pass --workspace or run checkout first", to)
			}
			workspaceID = pin.WorkspaceID
			cx.Flags["workspace"] = workspaceID
		} else {
			return nil, err
		}
	}
	dest, err := checkoutDest(cx.WS, cx.Home, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	pin, err := worktree.ReadMountCheckoutPin(dest)
	if err != nil {
		return nil, err
	}
	if pin == nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "%s has never been checked out; use checkout first", dest)
	}
	reports, err := worktree.MountStatus(pin.Mounts)
	if err != nil {
		return nil, err
	}
	return map[string]any{"workspaceId": pin.WorkspaceID, "dir": homeRel(cx.Home, dest), "mounts": reports}, nil
}
