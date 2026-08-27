---
name: integration-development
description: Create or modify the implementation of a business Connector, Collector, or live access package. Do not use merely to operate an existing integration.
---

# Business integration development

Invoke this Skill only when the user asks to create, debug, or change integration
code. If a provider task supplies an existing manifest, Adapter, Collector, or
preview executable and asks to run it, use the Knowledge Catalog provider flow
instead: read its manifest/README and execute the supplied artifact without
inspecting implementation files or running package tests unless execution fails.

Work in the business-owned integration Git repository already mounted as the
current working directory. Read repository-local instructions first, then read
[references/runtime-contract.md](references/runtime-contract.md). Do not create
a Knowledge Repository or copy external source data into knowledge files.

Create one flat package at `connectors/<integration-id>/`. The directory is the
delivery, ownership and runtime version boundary. Its manifest must name the
business owner, build/test command, maintenance schedule, target Knowledge Repo
and narrow Address scope.

Implement only the parts requested:

- Collector command: read source-owned current state or events and translate
  them to the runtime's stdin/stdout contract. It must not invoke KC or write
  git; the runtime submits its output through Writer.
- Access command: serve operations such as status, window, lookup or search
  against the live source. It must accept the identity and pinned Descriptor
  coordinates supplied by the runtime and must not trust model-supplied
  endpoints or credentials.

Keep credentials out of the repository, manifest, fixtures, stdout and
checkpoints. Use environment-bound source locations or platform secret
references. Add deterministic fixture tests for stable IDs, deletion coverage,
access inputs and returned cuts.

Run the declared tests. Commit and push only when the user's task explicitly
asks to publish. Runtime synchronization, validation and activation happen
after push; package code must never self-register or self-activate.
