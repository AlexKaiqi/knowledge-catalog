# Object HTTP SDK

The SDK owns typed Object requests and HTTP routes. Callers inject the existing
authenticated transport; this package does not save sessions, assert principals,
open repositories, or interpret authorization and maintenance scope.

Reference strings, including JSON Pointer fragments, travel as request data.
READ/PUT/REMOVE accept only a complete object Reference. Dataset identity and
source version qualifiers remain separate; repository, object, Aspect, Member,
include and exclude are not alternate content coordinates. PUT/REMOVE pass
selection to the service; their client does not infer maintenance units from
locator depth. Commit requests submit an atomic single-repository ChangeSet.
Receipt queries only inspect the original command result.

`ReadRequest.Basis` accepts the opaque durable `basis` reference returned by a
previous read. It excludes repository, Dataset, pin, definition and version
qualifiers. The object selection must match; omitting Shape retains the original
shape. The server rechecks current authorization and fixed bindings on every
replay; the SDK never interprets or expands the stored coordinates. Read results
include scoped local `state` independently of external observations.
Temporary definitions also return replayable bases. Replay preserves their
fixed file selection and checks current delivery grants; a caller-supplied
label cannot borrow a named publication's authority.

Dependencies: Object public contracts, Snapshot Dataset coordinate types,
the shared HTTP codec, and the standard library. No KC CLI, client, Home,
Invocation, Discovery, or LiveAccess implementation dependencies.

Commit and maintenance recovery submit once. A protocol error carrying
`writeOutcome: "NOT_APPLIED"` returns immediately with its original error code;
it never queries a receipt or resubmits. An omitted/unrecognized outcome, a lost
response, or response decoding failure remains uncertain and uses the existing
original-command receipt recovery budget. A 503 or message text alone is not proof.

`Compare` posts `ComparisonRequest` to `POST /object/v1/objects:diff`.
`Reference` and optional `Shape` select the same object content on both sides;
`From` and `To` each contain exactly one `object.ComparisonView`: `basis`,
`repository` plus `commit`, or `dataset` plus a positive `revision`. A retained
shaped basis requires its matching Shape explicitly for comparison. Views never
imply latest or source-history access. Each side revalidates current grants and
accepted file scope independently. `ObjectComparison` retains both coordinate
vectors and scoped local states; cross-scope comparisons omit `equal` and report
provable Unit entry/exit instead of claiming a source deletion. Full Unit values
are compared even when Reference or Shape projects fields; no external value
resolution is performed. These response coordinates are not opaque replay tokens.

New COMMIT/MERGE results may include Boolean `result.changed`. The Go type is
`object.ChangeState`; its zero value preserves unknown for legacy receipts,
while known false and true remain distinct and comparable receipt values.
Receipt recovery keeps this original verdict, including false after a lost
response; it never infers a verdict from equal commit coordinates or current
content. ReceiptRef identifies the original command/request, rather than only
its commit. Existing persisted receipt identities replay unchanged.

`Log` accepts an opaque durable `Continuation` that independently restores the
original object, Repository/Dataset scope, ref, source bindings and commits.
Scope fields may be omitted on continuation; supplied coordinates must match.
The server rechecks current source history permission and Dataset consumption
on every page. New pages report `complete` and `scanned`; an unfinished page
always has an advancing continuation, even when `logs` is empty. Continue until
`complete:true`. A missing, corrupt or unavailable basis fails without following
latest. Revision-only legacy cursors are not accepted as durable scan proofs.
