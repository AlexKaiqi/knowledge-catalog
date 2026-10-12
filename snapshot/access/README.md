# snapshot/access

Explicit grants for Snapshot Store services: principal, action and resource
scope. The package has no dependency on Home, Invocation, Knowledge, Server or
an index. Actions are opaque semantic names; the matcher does not translate
knowledge actions or infer authority from native lakeFS roles.

`Rule` and `GrantList` retain the existing JSON representation. Repository,
Catalog and Dataset are exact selectors. Ref and Aspect are exact optional
qualifiers; Object retains its existing `path.Match` pattern behavior as an
opaque application qualifier. Object is **not** the source file path of a
Dataset and is not interpreted as a knowledge object identity. Dataset file
coordinates and the accepted publication determine file selection independently.
Legacy command-to-action mappings remain in the KC adapter. Rule decoding
preserves the established retired-field rejection and on-read
`dataset.consume` to `file.read` migration.

`Match` is the shared explicit-rule evaluator. It does not evaluate public
Catalog discovery, authenticated repository defaults, local-owner bypass,
temporary knowledge definitions or application delivery policy. Those policies
remain in the caller and use the same evaluator.

`Service` provides authenticated inventory, add and revoke operations through a
`RuleStore`. The caller supplies a verified principal. Scoped administration
uses explicit coordinates; revoke derives its target from the durable rule ID.
Unknown and ambiguous IDs remain unscoped before authorization. Ref/Object/
Aspect constrained management rules fail closed; this package does not invent
glob-containment or permission delegation semantics. Read and manage are
separate actions, and native administrator status implies neither.

`RuleStore.UpdateRules` serializes authorization and mutation together, preserves
state on a callback error and acknowledges only a successful durable update.
The KC adapter updates the Rules portion of the original `allow.json`, preserving
provisioning, admission and identity migration receipts. There is no second
policy ledger, initialization-on-read or rule restoration on restart.
`AddRule`/`RemoveRule` expose the same domain operations for the existing trusted
CLI operation boundary; they do not authorize a caller themselves. Grant IDs
remain random and opaque, and ambiguous legacy IDs cannot be revoked.

Inventory lists explicit rules only. It does not claim to describe effective
permissions and does not expose private deployment ledger records. Pins and
cached content never cache an authorization decision: services evaluate the
current rules for every new request.
