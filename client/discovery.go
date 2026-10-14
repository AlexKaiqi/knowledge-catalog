package client

import (
	declarativeindexclient "kc/declarativeindex/client"
	discoveryclient "kc/discovery/client"
)

func (c *Client) DiscoveryService() discoveryclient.Client               { return c.discovery }
func (c *Client) DeclarativeIndexService() declarativeindexclient.Client { return c.declarativeIndex }
