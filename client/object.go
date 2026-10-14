package client

import objectclient "kc/object/client"

// ObjectService reuses the caller's authenticated transport and the single Object SDK.
func (c *Client) ObjectService() objectclient.Client { return c.objects }
