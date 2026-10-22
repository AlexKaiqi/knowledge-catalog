package cli

import (
	"context"

	kcclient "kc/client"
	clientsession "kc/client/session"
	"kc/internal/clientconfig"
)

// ClientServerURL resolves the delivered endpoint and current user selection.
// It contains no credential and may be saved with a caller's runtime config.
func ClientServerURL(server string) string {
	return clientconfig.ServerURL(server)
}

// NewSessionClient gives other KC inv.Command products the same per-server login,
// refresh, credential isolation and per-request reloading as kc and kcfs.
func NewSessionClient(ctx context.Context, server, principal string) (*kcclient.Client, error) {
	config, err := clientsession.Configure(ctx, ClientServerURL(server), clientsession.Selection{Principal: principal})
	if err != nil {
		return nil, err
	}
	return kcclient.New(config)
}
