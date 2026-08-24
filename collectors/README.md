# Legacy TypeScript collector prototype

This tree is retained as historical scene work from before the protocol moved
to the repository-root Go packages. Several files still import the retired
TypeScript `src/**` protocol and are therefore not an executable validation
surface after `main` is merged.

Do not restore or evolve protocol contracts here. Current executable inbound
translation lives under `validation/cmd/`; it produces a ChangeSet preview and
enters Knowledge Catalog through `kc commit --changeset` / Writer. Any useful
source parsing or source-key behavior migrated from this prototype must keep
that same boundary.
