package client

import snapshotclient "kc/snapshot/client"

// Factories reuse the one SDK assembled with this Client's current identity
// transport. They do not retain a second Snapshot algorithm or DTO family.
func (c *Client) SnapshotService() snapshotclient.Service {
	return c.snapshots.SnapshotService()
}

func (c *Client) WorkspaceFilesService() snapshotclient.FilesService {
	return c.snapshots.WorkspaceFilesService()
}

func (c *Client) AdminService() snapshotclient.AccessService {
	return c.snapshots.AdminService()
}
