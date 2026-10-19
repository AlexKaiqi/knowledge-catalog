package httpjson

import "net/http"

// MinIdleConnsPerHost is the idle pool floor for KC's own HTTP fan-outs.
const MinIdleConnsPerHost = 64

// PooledTransport returns a fresh transport whose idle pool covers perHost
// concurrent requests to one origin (at least MinIdleConnsPerHost). Go's
// default keeps only two idle connections per host, so any bounded fan-out
// would reopen a TCP connection on almost every request and can exhaust
// local ephemeral ports on large uploads.
func PooledTransport(perHost int) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if perHost < MinIdleConnsPerHost {
		perHost = MinIdleConnsPerHost
	}
	transport.MaxIdleConnsPerHost = perHost
	if transport.MaxIdleConns < perHost*2 {
		transport.MaxIdleConns = perHost * 2
	}
	return transport
}
