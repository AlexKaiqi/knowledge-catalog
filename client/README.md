# KC Client identity and authentication

`client` is the caller-side boundary for KC Server and other remote systems.
It owns three separate concepts:

- `Identity`: `principal` plus optional `onBehalfOf`, reused by authorization
  and access evidence;
- `Authentication`: an opaque credential value that protocol packages never
  inspect;
- `Session`: client-local login state, not a server resource and not a
  `WorkspaceSession`.

The default `PassThroughAuthenticator` only validates field shape. Every call
forwards `Authorization`, `X-Kc-As`, and `X-Kc-On-Behalf-Of`; it therefore fits
local development only. A production authenticator should verify login,
refresh or exchange audience-scoped credentials in `AuthenticateRequest`, and omit
client-asserted identity headers when the remote server derives identity from
the verified credential.

`Client.Do` can authenticate a request to KC or another system. `Client.Invoke`
targets the current `kc serve` compatibility API. Both re-read the current
client session for every request and propagate W3C trace context without adding
identity or secrets to baggage.

`MemorySessionStore` intentionally forgets credentials when the process exits.
Applications that need durable login must provide a `SessionStore` backed by an
OS keychain or Agent credential store; credentials must never enter Catalog,
Repository, Workspace pins, logs, or trace baggage.
