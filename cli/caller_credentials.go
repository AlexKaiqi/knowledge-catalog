package cli

import (
	"context"
	"net/http"
	"strings"
)

// callerCredentials is the inbound proof that already crossed the Knowledge
// Server authentication boundary. It is forwarded to resource-access/v1 so the
// origin can enforce source-side authorization. The accessor may ignore these
// headers; KC still sends them. Tokens never enter Schema, flags, evidence, or
// the JSON body.
type callerCredentials struct {
	Authorization string
	TaiIdentity   string
}

type callerCredentialsKey struct{}

func contextWithCallerCredentials(ctx context.Context, creds callerCredentials) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if creds.Authorization == "" && creds.TaiIdentity == "" {
		return ctx
	}
	return context.WithValue(ctx, callerCredentialsKey{}, creds)
}

func callerCredentialsFromContext(ctx context.Context) callerCredentials {
	if ctx == nil {
		return callerCredentials{}
	}
	creds, _ := ctx.Value(callerCredentialsKey{}).(callerCredentials)
	return creds
}

func callerCredentialsFromHeader(header http.Header) callerCredentials {
	if header == nil {
		return callerCredentials{}
	}
	return callerCredentials{
		Authorization: strings.TrimSpace(header.Get("Authorization")),
		TaiIdentity:   strings.TrimSpace(header.Get("X-Tai-Identity")),
	}
}

func applyCallerCredentials(req *http.Request, creds callerCredentials) {
	if req == nil {
		return
	}
	if creds.Authorization != "" {
		req.Header.Set("Authorization", creds.Authorization)
	}
	if creds.TaiIdentity != "" {
		req.Header.Set("X-Tai-Identity", creds.TaiIdentity)
	}
}
