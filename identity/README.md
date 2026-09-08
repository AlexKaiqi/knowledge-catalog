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

Normal login never moves legacy `gitea:<id>` / `taihu:<username>` grants or
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
