// Package console provides the complete vendored lakeFS console and file
// Dataset/access extensions. API authorities are provided by the caller.
package console

import "net/http"

// Handler serves the independently built console under /ui/.
func Handler() http.Handler { return newHandler() }
