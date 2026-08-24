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

## Repository contract

The Host discovers exactly `connectors/*/connector.yaml` under one mounted
user repository. Runtime state is kept under the Host `--home`, never in the
user repository.

```text
user-repo/
├── connectors/
│   └── file-observer/
│       ├── connector.yaml
│       ├── main.go
│       └── fixtures/
└── sources/
    └── facts.json
```

Minimal manifest:

```yaml
apiVersion: connector.kc/v1alpha1
kind: Connector
metadata:
  id: file-observer             # must match the directory name
  description: Observe a source-owned JSON file
  owner: example-team
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

/tmp/connector-host --home /tmp/connector-home mount \
  --repo /path/to/user-repo \
  --kc-url http://127.0.0.1:7380 \
  --as connector/file-observer

/tmp/connector-host --home /tmp/connector-home list
/tmp/connector-host --home /tmp/connector-home validate --connector file-observer
/tmp/connector-host --home /tmp/connector-home run --connector file-observer --preview
/tmp/connector-host --home /tmp/connector-home run --connector file-observer
/tmp/connector-host --home /tmp/connector-home activate --connector file-observer
/tmp/connector-host --home /tmp/connector-home history --connector file-observer
/tmp/connector-host --home /tmp/connector-home serve --listen 127.0.0.1:7480
```

The browser console at `http://127.0.0.1:7480/` provides discovery, preview,
manual run, activate/pause, status and recent history. It is a local MVP and
uses the operating-system user as its permission boundary.

## Acceptance coverage

`host_test.go` covers:

- strict manifest discovery and connector-owned fixture tests;
- preview without Writer/checkpoint mutation;
- first FULL reconcile additions;
- update/add/remove on the next source observation;
- Writer failure and checkpoint immutability;
- successful retry;
- activation and generation pinning;
- scheduled execution;
- rejection of KEYED reconcile;
- browser dashboard and JSON API discovery.

The executable agent scenario in `scripts/e2e-agent.sh` additionally starts a
real KC, invokes a real DSH coding agent in an initially connector-free user
repo, validates the generated Connector, runs it through the Host and checks
the resulting canonical knowledge and provenance.
