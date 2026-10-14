package cli

import (
	"context"
	"maps"

	"kc/internal/telemetry"
	inv "kc/invocation"
)

// The fixture shares argv preparation and validation with the product while
// keeping application execution entirely in the test binary.
func runWithTelemetryMode(argv []string, runtime *telemetry.Runtime, embedded bool) RunResult {
	if !embedded {
		return RunWithTelemetry(argv, runtime)
	}
	return decorateCLIUsage(argv, dispatchEmbeddedCLI(argv, runtime))
}

func dispatchEmbeddedCLI(argv []string, runtime *telemetry.Runtime) RunResult {
	request, completed := prepareCLI(argv)
	if completed != nil {
		return *completed
	}
	if request.path == "serve" {
		return runServe(request.flags)
	}
	if clientOperationPath(request.path) {
		request.flags = maps.Clone(request.flags)
		delete(request.flags, "home")
	}
	if err := validateCLIRequest(request); err != nil {
		return inv.ErrorResult(err)
	}
	if clientOperationPath(request.path) {
		return runClientOperationWithTelemetry(context.Background(), runtime, request.path, request.surface.Handler, request.flags)
	}
	if _, explicitServer := request.flags["server"]; explicitServer {
		if server := inv.RemoteServerURL(request.flags); server != "" {
			return encodeCLIProduct(request.path, request.flags, runRemoteCLI(context.Background(), server, request.path, request.flags))
		}
	}
	return encodeCLIProduct(request.path, request.flags, invokeWithTelemetry(context.Background(), runtime, request.surface.Handler, request.flags))
}
