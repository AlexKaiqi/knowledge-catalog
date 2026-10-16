# identity/

Service identity binding, independent of Catalog and Snapshot. Public human
principal is `CanonicalUsername`; it is never a display alias for a numeric
provider principal. The validator accepts 1–39 lowercase ASCII letters/digits
and single internal `-`, `_`, `.` separators, rejecting normalization that would
silently rename an account.

`VerifiedUser` contains username, provider, issuer and stable subject after a
trusted authenticator validates a credential. `Bind` persistently reserves both
the username and external identity in `stateDir/identities.json`. Same identity
replays; a different subject/provider/issuer claiming the name, or the same
subject presenting a new name, fails closed. Bindings are not deleted on logout
or provider account deletion. No credentials or grants enter this file.

`Initialize` belongs to explicit deployment initialization; `Bind` and `Validate`
never recreate missing or damaged state. The reference service serializes file
updates by absolute path within a process and uses fsync plus atomic replacement.
Multiple active processes must not share this mutable service state directory.
Instance replacement restores the same durable file.

Normal login never moves legacy provider-prefixed (`taihu:<username>`) grants or
reapplies initial grants. Migration is an explicit operator operation over a
verified binding and the current authorization state; old deleted grants must
remain absent. An issuer switch or username change cannot use login as migration.

`MigrateLegacyAlias` is an operator-only step in that migration. Its
`legacyAliases` entry must refer to an identical verified binding in the same
atomic identity file. `ResolvePrincipal` lets historical managed ownership use
the canonical username without changing original requests, repository IDs,
allocation IDs or their digests. Normal `Bind` never infers these aliases.
Installing an alias grants no capability; the application separately rewrites
only still-existing rules and atomically records its migration decision. A
crash between alias installation and that rewrite is completed by repeating the
same explicit migration.

## Shared HTTP identity

`HTTPIdentity` and `HTTPAuthenticator` describe verified transport claims for the
KC service and independently assembled Snapshot Store service. `VerifierOrigin` and `DecodeVerifierUser`
provide one bounded verification-response path for configured providers; they
do not authenticate a request, grant access or interpret native administrator
roles. Consumers now include `snapshot/service`; the same binding authority and
account-lifecycle limits still apply.

`WrapAuthenticator` applies the same durable `Bind` boundary to either service.
An independent Snapshot service initializes its binding authority explicitly,
wraps a configured verifier with the existing binding path and passes that
verifier to its `IdentityOptions`. The fixture flag permits initialization only
for explicitly controlled fixtures; normal authentication never creates missing
production identity state or restores revoked grants.

## Authentication

Authentication is a replaceable capability owned by this directory. `Register`,
`Lookup`, `Names` and `Resolve` form the authenticator registry; a registered
name is the value of `kc serve --auth` and of deployment `auth`. `Authenticate`
is the single request check that `server/` and `snapshot/service/` both call: it
applies the pairing (`Accepts`), rejects self-asserted identity under a
verifying authenticator, and runs the caller-supplied `Check` before the
authenticator. `BrowserRequestCheck` (console marker, Fetch Metadata, JSON
mutations for cookie requests) is a service-wide browser check, not part of any
authenticator. An authenticator that brokers browser login and token exchange
exposes a `LoginBroker`; the Server's `/identity/v1/token` and
`/identity/v1/authorize` routes call it without knowing the provider.

| Package | Role |
|---|---|
| `identity/local` | `--auth local`: accepts only `X-Kc-As`; an ordinary authenticator that must be selected explicitly |
| `identity/taihu` | `--auth taihu`: gateway HMAC and introspection, service application identity, login broker |
| `identity/builtin` | links the built-in authenticators; imported only by assembly roots |
| `identity/identitytest` | the shared test authenticator double; package tests do not declare their own |

Adding an authenticator means one `identity/<name>` package that registers
itself and is listed in `builtin`; `server/`, `snapshot/service/` and
authorization do not change (`TestRegisteredAuthenticatorReplacesProviderWithoutServerChange`).
An authenticator answers only "who is this request". It never decides how KC
reaches storage: KC calls lakeFS with its private service credential after
authentication and authorization, and caller proofs are never forwarded.
`TestAuthenticatorsAreConfinedToIdentity`, `TestAuthenticatorDoublesComeFromIdentitytest`
and the `identity/builtin` / `identity/identitytest` layer rules keep this shape.
