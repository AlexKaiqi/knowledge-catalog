package liveaccess

import (
	"context"
	"encoding/json"
	"errors"
	"kc/kernel"
	"net/http"
	"strings"
)

// callerCredentials is the inbound proof that already crossed the Object
// Server authentication boundary. It is forwarded to resource-access/v1 so the
// trusted applicable origin can enforce source-side authorization. Tokens
// never enter Schema, flags, evidence, or the JSON body.
type callerCredentials struct {
	Authorization string
	TaiIdentity   string
}

type callerCredentialsKey struct{}

func ContextWithCallerCredentials(ctx context.Context, creds callerCredentials) context.Context {
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

func CallerCredentialsFromHeader(header http.Header) callerCredentials {
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

// SafeFailure keeps the public reason while removing the current request's
// proof. Source errors and diagnostics must not become another credential path.
func SafeFailure(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, secret := range credentialSecrets(callerCredentialsFromContext(ctx)) {
		message = strings.ReplaceAll(message, secret, "[redacted]")
	}
	if message == err.Error() {
		return err
	}
	if fault := kernel.AsIngress(err); fault != nil {
		copy := *fault
		copy.Message = fault.Message
		for _, secret := range credentialSecrets(callerCredentialsFromContext(ctx)) {
			copy.Message = strings.ReplaceAll(copy.Message, secret, "[redacted]")
		}
		return &copy
	}
	return errors.New(message)
}

// CheckCredentialDisclosure rejects a source result containing this request's
// authentication proof before the value can be adopted or retained.
func CheckCredentialDisclosure(ctx context.Context, value any) error {
	secrets := credentialSecrets(callerCredentialsFromContext(ctx))
	if len(secrets) == 0 {
		return nil
	}
	body, err := json.Marshal(value)
	if err != nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess returned an unencodable value")
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess returned an invalid JSON value")
	}
	if containsCredential(decoded, secrets) {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess result contains authentication proof")
	}
	return nil
}

func containsCredential(value any, secrets []string) bool {
	switch item := value.(type) {
	case string:
		for _, secret := range secrets {
			if strings.Contains(item, secret) {
				return true
			}
		}
	case map[string]any:
		for key, child := range item {
			if containsCredential(key, secrets) || containsCredential(child, secrets) {
				return true
			}
		}
	case []any:
		for _, child := range item {
			if containsCredential(child, secrets) {
				return true
			}
		}
	}
	return false
}

func credentialSecrets(creds callerCredentials) []string {
	var out []string
	for _, value := range []string{creds.Authorization, creds.TaiIdentity} {
		if value != "" {
			out = append(out, value)
		}
	}
	if _, token, ok := strings.Cut(creds.Authorization, " "); ok && token != "" {
		out = append(out, token)
	}
	return out
}
