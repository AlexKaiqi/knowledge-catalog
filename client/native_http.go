package client

import (
	"encoding/json"
	"net/http"

	"kc/kernel"
)

// The native gateway preserves lakeFS's {message} envelope and adds a KC code
// only when product authentication or authorization rejected the request.
func nativeJSONError(status int, raw []byte) error {
	var fault kernel.IngressError
	if json.Unmarshal(raw, &fault) != nil || fault.Message == "" {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "native API returned HTTP %d", status)
	}
	if fault.Code == "" {
		fault.Code = kernel.ErrTemporaryUnavailable
		switch status {
		case http.StatusBadRequest:
			fault.Code = kernel.ErrUsageInvalid
		case http.StatusUnauthorized:
			fault.Code = kernel.ErrUnauthenticated
		case http.StatusForbidden:
			fault.Code = kernel.ErrForbidden
		case http.StatusNotFound, http.StatusConflict:
			fault.Code = kernel.ErrPreconditionFailed
		}
	}
	return &fault
}
