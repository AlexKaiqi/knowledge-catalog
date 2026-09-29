package cli

import (
	"context"
	kcclient "kc/client"
	inv "kc/invocation"
)

// ClientServerURL resolves the delivered endpoint and current user selection.
// It contains no credential and may be saved with a caller's runtime config.
func ClientServerURL(server string) string {
	return inv.RemoteServerURL(map[string]FlagValue{"server": server})
}

// NewSessionClient gives other KC inv.Command products the same per-server login,
// refresh, credential isolation and per-request reloading as kc and kcfs.
func NewSessionClient(ctx context.Context, server, principal string) (*kcclient.Client, error) {
	return newRemoteSessionClient(ctx, ClientServerURL(server), map[string]FlagValue{"as": principal})
}
