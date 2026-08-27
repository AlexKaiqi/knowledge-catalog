---
name: integration-development
description: Create or change Connector, Collector, or live-access implementation code. Do not use to operate an existing integration.
---

# Integration development

Use only when the user asks to create, debug or change integration code. Read
repository instructions and
[the runtime contract](references/runtime-contract.md) before implementation.

Keep one integration package at `connectors/<integration-id>/` with its owner,
target Repository, Address scope, build/test command and schedule in the
manifest.

- Collector: translate source state/events to the runtime contract. It must not
  invoke KC or write git.
- Access command: serve declared live operations using runtime-supplied identity
  and pinned Descriptor coordinates; never trust model-supplied endpoints or
  credentials.

Keep secrets and source data out of the repository and output. Add deterministic
tests for IDs, deletion, access inputs and observation cuts. Run declared tests.
Commit or push only when explicitly requested.
