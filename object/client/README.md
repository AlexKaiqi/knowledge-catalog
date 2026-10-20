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

Dependencies: Object public contracts, Snapshot Dataset coordinate types,
the shared HTTP codec, and the standard library. No KC CLI, client, Home,
Invocation, Discovery, or LiveAccess implementation dependencies.

Commit and maintenance recovery submit once. A protocol error carrying
`writeOutcome: "NOT_APPLIED"` returns immediately with its original error code;
it never queries a receipt or resubmits. An omitted/unrecognized outcome, a lost
response, or response decoding failure remains uncertain and uses the existing
original-command receipt recovery budget. A 503 or message text alone is not proof.
