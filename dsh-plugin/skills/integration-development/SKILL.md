---
name: integration-development
description: Create or change Connector, Collector, Observer, or Resource Access implementation code. Do not use to operate an existing integration.
---

# Integration development

Use only when the user asks to create, debug or change integration code. Read
repository instructions and
[the runtime contract](references/runtime-contract.md) before implementation.

Keep one integration package at `connectors/<integration-id>/` with its owner,
target Repository, Address scope, build/test command and schedule in the
manifest.

- Collector: reconcile source state and emit Writer observations. It must not
  invoke KC or write git.
- Observer: send change notice only; do not commit knowledge.
- Resource Access: serve the origin URL (`resource-access/v1`); do not write
  the repository or send notice.

Keep secrets and source data out of the repository and output. Add deterministic
tests for IDs, deletion, access inputs and observation cuts. Run declared tests.
Commit or push only when explicitly requested.
