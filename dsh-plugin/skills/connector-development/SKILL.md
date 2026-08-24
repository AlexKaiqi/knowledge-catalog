---
name: connector-development
description: Develop and test one source Connector package in a shared public Connector repository. Use for adding a new observed source or changing its source-to-knowledge translation; not for ordinary Knowledge Catalog operations.
---

# Connector development

Work in the authoritative public Connector Git repository already mounted as
the current working directory. Read repository-local instructions when
present, then read
[references/connector-contract.md](references/connector-contract.md) before
editing. If `connectors/` is missing, create that directory in this repository;
do not invent a different layout or a second repository.

Create exactly one flat package at `connectors/<connector-id>/`. Use a stable,
lowercase ID such as `billing-invoice`; when one business has several
integrations, create independently owned IDs rather than nested directories.
The manifest `metadata.id` must equal the directory name and
`metadata.owner` must name the responsible team.

Before implementation, determine from the request or existing repository
context:

- how to read the source and which source keys provide stable identity;
- whether the observation is FULL current state or KEYED partial state;
- target Repository/ref and the narrow Address Scope;
- desired manual or scheduled maintenance frequency.

Ask only when one of those choices materially changes deletion safety,
identity, authorization or the target. Never infer FULL coverage from a
partial API, and never use reconcile for KEYED coverage.

Implement the source read and translation behind the stdin/stdout ABI in the
contract reference. Connector code emits observations and desired Addresses;
it must not invoke KC, commit knowledge, persist Host state or choose a runtime
principal. Keep secrets out of code, manifests, fixtures, output and
checkpoints.

Add deterministic fixture tests covering stable IDs, values and coverage. Run
the manifest's declared test command. Commit and push only when the user asks
you to publish the working-copy change. The execution service, not the user's
workspace, synchronizes the public Repo and makes the Connector discoverable.
If `connector-host` is available after synchronization, run `validate` and a
preview. Discovery leaves the Connector paused.

Do not activate a Connector or changed generation unless the user explicitly
requests deployment after reviewing the preview. When handing off, report the
Connector ID, owner, source, target Scope, maintenance policy, test result,
Git commit/push status, synchronized generation/preview when available, and
the exact remaining activation command.
