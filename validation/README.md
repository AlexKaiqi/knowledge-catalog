# Data Warehouse Validation

This directory is the black-box validation surface for the data-warehouse use
case. It lives in the `scene/data-warehouse` worktree and consumes the protocol
implementation merged from `main`; it does not evolve protocol contracts here.

Loom is tracked as a map of user jobs, not as one demo path. It covers Catalog
lifecycle, Repo onboarding, publishing and sync, Workspace composition,
discovery, real agent entry, personal authoring, governed publishing,
authorization, update awareness, failure recovery, and operations. The full
map and its representative cross-capability journeys are in
[VALIDATION_MAP.md](VALIDATION_MAP.md); executed evidence and explicit product
boundaries are in [PROGRESS.md](PROGRESS.md).
The role-by-role DSH prompts, operator checks, and expected states are recorded
in [docs/DSH_LOOM_OPERATOR_RUNBOOK.md](docs/DSH_LOOM_OPERATOR_RUNBOOK.md).
The Computer Use execution evidence from 2026-08-24 is recorded in
[docs/DSH_LOOM_UI_TEST_2026-08-24.md](docs/DSH_LOOM_UI_TEST_2026-08-24.md).
The browser-driven, role-separated coverage of every public `kc` action is in
[docs/DSH_BROWSER_ALL_ACTIONS_2026-08-24.md](docs/DSH_BROWSER_ALL_ACTIONS_2026-08-24.md).
The wall-side connector development and execution reference lives in
[connectorhost/](connectorhost/); it calls the protocol facade rather than
adding a connector runtime to `kc`.
The Payment API role-separated DSH acceptance, including ResourceDescriptor
access and an automatically collected external update, is recorded in
[docs/RESOURCE_DESCRIPTOR_DSH_E2E_2026-08-24.md](docs/RESOURCE_DESCRIPTOR_DSH_E2E_2026-08-24.md).
TPC-H values remain domain oracles under `fixtures/tpch-sf001/expected/`; they
do not determine whether Loom's composition, pinning, routing, authorization,
adapter, or recovery capabilities are covered.

The fixture is TPC-H SF0.01. User journeys, their goals and observable outcomes
live in the declarative [scenario library](scenarios/README.md). Technical
nodes own executable entry points and fixed oracles, but are only reusable
actions inside those journeys. `playbook.sh` is the compatibility entry point.

## Run

```bash
./validation/playbook.sh DW-00
./validation/playbook.sh DW-01
./validation/playbook.sh DW-02
./validation/playbook.sh DW-03
./validation/playbook.sh DW-04
./validation/playbook.sh WORKBENCH
./validation/playbook.sh REALISTIC-KNOWLEDGE
./validation/playbook.sh DECLARATIVE-INDEX
./validation/playbook.sh USER-PUBLISHED-INDEX
./validation/playbook.sh PRODUCER
./validation/playbook.sh CONSUMER
./validation/playbook.sh all
```

`WORKBENCH` runs the company workbench scenario under
`validation/workbench/`. Its proposal, Workspace, provenance, federation and
lifecycle assertions are scene acceptance tests; the protocol implementation
continues to come from `main`.

`REALISTIC-KNOWLEDGE` builds a connected physical-to-semantic warehouse graph:
MySQL assets, derived tables, ETL job/task definitions, task IO and column
mappings, data-plane permission snapshots, classifications, quality rules,
MetricView/Dimension/Measure/Metric knowledge, and pinned ETL run history. It
proves that GMV can be traced to exact source columns without treating join
evidence as lineage or a `permissions` Aspect as `kc allow`.

`DW-00` downloads a pinned DuckDB CLI into the ignored root `.data/` cache,
generates the official TPC-H SF0.01 dataset, and compares the observed values
with `fixtures/tpch-sf001/expected/dw00.json`. To use an existing binary:

```bash
DUCKDB_BIN=/absolute/path/to/duckdb ./validation/playbook.sh DW-00
```

The generated database and actual result are placed below
`.data/datawarehouse/`. They are disposable and are never canonical knowledge.

`DW-01` additionally requires a running Docker daemon. It starts an isolated,
digest-pinned MySQL 8.4.8 service and removes it after the node finishes. Set
`KC_DW_KEEP_MYSQL=1` to retain the container for diagnosis.

`DW-02` is independently executable and prepares its `DW-01` dependency before
collecting column profile, join, and annotation evidence from the same MySQL.

`DW-03` prepares `DW-02`, performs one real row update at the fixed MySQL binlog
coordinate `mysql-bin.000003:687`, appends the row event, advances a
connector-owned checkpoint only after the profile commit, and proves duplicate
and regressed positions do not move either cursor.

`DW-04` prepares the real MySQL structure fixture, deploys
`connectors/mysql-structure-auto` through the wall-side Integration Host and
activates its one-second schedule. The scheduler performs both knowledge
writes: first 69 physical Addresses, then a FULL reconcile after real `ADD`,
`MODIFY` and `DROP` DDL. The exact delta is one addition, three updates, one
removal and 65 unchanged Addresses. A fresh Workspace pin observes the new
structure while the saved pin still reproduces the old columns. Design and
evidence details are in
[`docs/AUTOMATIC_PHYSICAL_STRUCTURE.md`](docs/AUTOMATIC_PHYSICAL_STRUCTURE.md).

`USER-PUBLISHED-INDEX` exercises the actual authoring and governance surfaces,
not only the index package. It checks out a Workspace, replaces and deletes
knowledge files, publishes them with `commit --workspace`, and then publishes a
second update through proposal/preview/validate/merge. Every fresh search must
match the new Canonical commit without `index-sync`; replaced terms and removed
objects must disappear. An injected index failure additionally proves that the
knowledge commit remains published, lag is visible, and a later Ensure repairs
the derived projection.

## Node contract

Each node must define all of the following before implementation:

1. exact initial state;
2. one trigger or operation;
3. exact observable output and state delta;
4. invariants that must not change;
5. an executable oracle;
6. prerequisites and upstream node IDs.

Keep exact domain outputs in `fixtures/tpch-sf001/expected/`. Map any result
back to the system capability it actually proves in
[PROGRESS.md](PROGRESS.md); a correct fixture result must not upgrade an
unrelated Loom capability.
