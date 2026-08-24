# Local Connector Host MVP

This directory is a wall-side reference service. It is not part of the
Knowledge Catalog protocol runtime and it never imports Writer, Catalog or CLI
implementations. Connector output enters KC only through `POST /v1/commit`.

## Concepts

- **Observation** — a source-scoped statement seen at one time. It is not a
  universal fact.
- **Observer** — the logical `RunRequest -> Observation` stage that reads a
  source. It does not know KC targets or write.
- **Translator** — maps an Observation to Address-level desired knowledge.
- **Connector** — the repo-owned package combining Source, Observer,
  Translator, maintenance policy, target and Scope.
- **Host** — this local service: discovery, validation, scheduling, execution,
  Preview, Writer, checkpoint and history.
- **Run** — one execution pinned to a connector bundle digest and target base.

Only Connector, Host and Run are first-class MVP objects. Observer and
Translator may be separate functions or one executable.

## Shared public development repository

There is exactly one authoritative ordinary Git repository for public
Connector development. It is not a Knowledge Repository and need not be
internet-public. Users mount or clone it into their workbench, develop and push
normal Git changes. The execution service independently synchronizes the
configured ref into a Host-owned read copy and discovers exactly
`connectors/*/connector.yaml` there. It never executes a user's working copy.
Runtime state is kept under the Host `--home`, never in the public repository.

```text
public-connectors.git
└── connectors/
│   ├── billing-invoice/
│       ├── connector.yaml
│       ├── main.go
│       └── fixtures/
│   └── billing-payment/
```

The layout is deliberately flat: one directory is one Connector package and
independent ownership/runtime boundary. A business with several observations
uses IDs such as `billing-invoice` and `billing-payment`, not nested folders.
Files in an uncommitted user checkout are not registered. A Connector becomes
discoverable only after its commit is pushed to the public Repo and synchronized
by the service. Every newly discovered Connector is paused. Only `activate`
admits a validated generation to the scheduler.

Minimal manifest:

```yaml
apiVersion: connector.kc/v1alpha1
kind: Connector
metadata:
  id: file-observer             # must match the directory name
  description: Observe a source-owned JSON file
  owner: example-team           # required in the shared repo
spec:
  command: [go, run, ., --source, ../../sources/facts.json]
  test:
    command: [go, test, .]
  maintenance:
    representation: current-state
    freshness: 10m
    triggers:
      - kind: manual
      - kind: schedule
        every: 10m
  target:
    repository: kr://demo/public/facts
    ref: refs/heads/main
    scope:
      aspects: [observed]
      objectPrefix: "FileFact:"
  runtime:
    timeout: 30s
```

MVP support is deliberately narrow:

- `representation: current-state`;
- manual and fixed-duration schedule triggers;
- `STATE` Observation batches with `FULL` or `KEYED` coverage;
- patch or reconcile Snapshot output;
- one target Repository/ref and one declared Scope per Connector.

Secrets are inherited or resolved by the user-owned command. They must not be
written to the manifest, stdout, checkpoint or run history.
Private Git access uses the service environment's normal SSH agent or Git
credential helper; credentials must not be embedded in the configured Repo URL.

The Host assigns each package the non-configurable KC principal
`connector/<metadata.id>`. Grants therefore remain least-privilege even though
all packages share one code repository and one Host process.

## Process ABI

The Host writes one JSON `RunRequest` to stdin:

```json
{
  "runId": "run-...",
  "connectorId": "file-observer",
  "generationDigest": "sha256:...",
  "trigger": {"kind": "manual", "at": "2026-08-24T10:00:00Z"},
  "targetBaseCommit": "...",
  "checkpoint": {}
}
```

The command writes exactly one JSON `ConnectorOutput` to stdout:

```json
{
  "observation": {
    "sourceRefs": ["file://facts.json"],
    "observedAt": "2026-08-24T10:00:00Z",
    "representation": "STATE",
    "coverage": {"kind": "FULL"}
  },
  "mode": "reconcile",
  "desired": [
    {
      "address": {
        "kind": "Aspect",
        "objectId": "FileFact:alpha",
        "aspectName": "observed"
      },
      "value": {"key": "alpha", "value": "one"},
      "sourceKey": "file://facts.json#alpha"
    }
  ],
  "observed": [],
  "nextCheckpoint": {},
  "message": "observe file facts"
}
```

Rules:

1. `sourceRefs` and RFC3339 `observedAt` are mandatory.
2. `reconcile` requires `coverage.kind=FULL`; KEYED can only patch.
3. Every desired Address must be inside manifest Scope.
4. The command proposes `nextCheckpoint`; only the Host persists it.
5. Preview never commits and never advances checkpoint.
6. Empty successful runs may advance source checkpoint without creating a KC
   commit.
7. Writer failure never advances checkpoint.
8. Scheduled runs execute only the bundle digest pinned by `activate`; changed
   files require validation and reactivation.

## CLI

```bash
go build -o /tmp/connector-host ./validation/connectorhost/cmd/connector-host

/tmp/connector-host --home /tmp/connector-home repo-set \
  --repo https://git.example.com/platform/public-connectors.git \
  --ref refs/heads/main \
  --sync-every 30s \
  --kc-url http://127.0.0.1:7380

/tmp/connector-host --home /tmp/connector-home sync
/tmp/connector-host --home /tmp/connector-home list
/tmp/connector-host --home /tmp/connector-home validate --connector file-observer
/tmp/connector-host --home /tmp/connector-home run --connector file-observer --preview
/tmp/connector-host --home /tmp/connector-home run --connector file-observer
/tmp/connector-host --home /tmp/connector-home activate --connector file-observer
/tmp/connector-host --home /tmp/connector-home history --connector file-observer
/tmp/connector-host --home /tmp/connector-home serve --listen 127.0.0.1:7480
```

`serve` synchronizes the configured Repo on the declared interval before
scheduling due Connectors. The browser console at `http://127.0.0.1:7480/`
shows the synchronized commit and last sync state and provides Sync now,
discovery, preview, manual run, activate/pause and recent history. It is a local
MVP and uses the operating-system user as its permission boundary.

## Acceptance coverage

The Connector Host test suite covers:

- synchronization from an authoritative bare Git Repo into a separate
  Host-owned checkout;
- exclusion of uncommitted user changes and discovery after commit/push/sync;
- flat discovery of every package and isolation of an invalid neighbor;
- strict manifest discovery and connector-owned fixture tests;
- deterministic per-Connector KC principals in one shared Host;
- preview without Writer/checkpoint mutation;
- first FULL reconcile additions;
- update/add/remove on the next source observation;
- Writer failure and checkpoint immutability;
- successful retry;
- activation and generation pinning;
- scheduled execution;
- rejection of KEYED reconcile;
- browser dashboard and JSON API discovery.

The executable agent scenario in `scripts/e2e-agent.sh` additionally creates a
real public bare Repo plus a separate user checkout, starts a real KC, invokes
a real DSH coding agent in that initially connector-free checkout, pushes the
result, synchronizes the Host copy, validates and runs it, and checks the
resulting canonical knowledge and provenance.
