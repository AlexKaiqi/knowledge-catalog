# Connector contract

The execution service synchronizes the authoritative public Connector Git
repository into its own read copy and discovers exactly
`connectors/*/connector.yaml`. A Connector directory is the unit of ownership,
testing, authorization, activation, checkpointing and history.

## Manifest

`metadata.id` must equal the directory name and use lowercase letters, digits
and single hyphens. `metadata.owner` is required.

```yaml
apiVersion: connector.kc/v1alpha1
kind: Connector
metadata:
  id: billing-invoice
  description: Observe invoices from the billing source
  owner: billing-platform
spec:
  command: [python3, observer.py]
  test:
    command: [python3, -m, unittest, discover, -s, tests]
  maintenance:
    representation: current-state
    freshness: 10m
    triggers:
      - kind: manual
      - kind: schedule
        every: 10m
  target:
    repository: kr://company/public/billing
    ref: refs/heads/main
    scope:
      aspects: [observed]
      objectPrefix: "Invoice:"
  runtime:
    timeout: 30s
```

The MVP supports current state, manual or fixed-duration scheduling, one target
Repository/ref/Scope, and `patch` or `reconcile`. Reconcile requires a FULL
observation; KEYED observations may only patch.

## Process boundary

The Host writes one JSON request to stdin:

```json
{"runId":"run-...","connectorId":"billing-invoice","generationDigest":"sha256:...","trigger":{"kind":"manual","at":"2026-08-24T10:00:00Z"},"targetBaseCommit":"...","checkpoint":{}}
```

The command writes exactly one JSON object to stdout:

```json
{
  "observation": {
    "sourceRefs": ["billing://invoices"],
    "observedAt": "2026-08-24T10:00:00Z",
    "representation": "STATE",
    "coverage": {"kind": "FULL"}
  },
  "mode": "reconcile",
  "desired": [{
    "address": {"kind": "Aspect", "objectId": "Invoice:123", "aspectName": "observed"},
    "value": {"number": "123", "state": "issued"},
    "sourceKey": "billing://invoices/123"
  }],
  "observed": [],
  "nextCheckpoint": {},
  "message": "observe billing invoices"
}
```

Every desired Address must be inside the declared Scope. `sourceRefs` and an
RFC3339 `observedAt` are mandatory. Connector code must not invoke KC. The Host
derives the runtime principal as `connector/<metadata.id>` and alone performs
Scope validation, Preview, commit and checkpoint advancement.

Secrets are resolved at runtime by the Connector command or its deployment
environment. Never write them to the manifest, stdout, checkpoint, fixtures or
history.

## Lifecycle

Creating a directory only changes the user's working copy. Registration occurs
after that change is committed and pushed to the public repository, then
synchronized by the execution service. A newly discovered Connector remains
paused. The operator runs `validate`, then a preview. `activate` pins the exact
bundle digest for scheduling. Later synchronized file changes stop scheduled
runs until the new generation is validated and explicitly activated.
