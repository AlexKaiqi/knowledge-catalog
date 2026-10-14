package object

import "kc/kernel"

type RequestContext struct {
	Identity  kernel.IdentityContext `json:"identity"`
	Trace     kernel.TraceContext    `json:"trace,omitempty"`
	RequestID string                 `json:"requestId,omitempty"`
}
