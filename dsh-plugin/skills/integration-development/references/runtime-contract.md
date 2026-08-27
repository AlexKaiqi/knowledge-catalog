# Integration runtime contract

The reference runtime synchronizes the authoritative integration Git repository
and discovers flat packages at `connectors/*/connector.yaml`. The historical
manifest filename remains an implementation detail; the package itself may
contain one Collector and one access command.

## Manifest

```yaml
apiVersion: connector.kc/v1alpha1
kind: Connector
metadata:
  id: payment-ops
  description: Payment API operational knowledge and Trace access
  owner: payments-platform
spec:
  command: [python3, collector.py]
  test:
    command: [python3, -m, unittest, discover, -s, tests]
  maintenance:
    representation: current-state
    triggers:
      - kind: manual
      - kind: schedule
        every: 10s
  target:
    repository: kr://acme/payments/operations
    ref: refs/heads/main
    scope:
      aspects: [observed]
      objectPrefix: "Service:"
  access:
    protocol: resource-access/v1
    command: [python3, access.py]
    operations: [status, window, lookup]
    timeout: 30s
  runtime:
    timeout: 30s
```

## Collector process

The runtime writes one JSON request to stdin. The command writes exactly one
FULL or KEYED observation containing `mode`, `desired`, optional `observed`, and
`nextCheckpoint`. FULL reconcile may remove prior Addresses; KEYED observations
may only patch. Every desired Address must be inside the manifest scope.

```json
{
  "observation": {
    "sourceRefs": ["payments://operations"],
    "observedAt": "2026-08-24T10:00:00Z",
    "representation": "STATE",
    "coverage": {"kind": "FULL"}
  },
  "mode": "reconcile",
  "desired": [{
    "address": {"kind":"Aspect","objectId":"Service:payment-api","aspectName":"observed"},
    "value": {"name":"Payment API","owner":"payments-platform"},
    "sourceKey": "payments://operations/services/payment-api"
  }],
  "observed": [],
  "nextCheckpoint": {},
  "message": "collect payment service state"
}
```

## Access process

The runtime writes one JSON request to stdin:

```json
{
  "operation": "lookup",
  "input": {"traceId":"trace-001"},
  "identity": {"principal":"consumer","agent":"dsh-loom","session":"session-1","requestId":"ask-1"},
  "descriptor": {"objectId":"resource/traces/payment-api","repository":"kr://acme/payments/operations","commit":"..."}
}
```

The command writes exactly one JSON result. Windowed output should include a
stable source `cut`; lookup should return an empty record list for a miss rather
than silently switching to a broader search.

When Knowledge Serving performs an ordinary State Binding READ, it calls the
runtime service at `POST /v1/access`, selecting declared `lookup` (or legacy
`read`). The response must distinguish the dynamic value from its observation
basis:

```json
{
  "value": {"status": "healthy"},
  "basis": {
    "bindingGeneration": "payment-ops-v3",
    "consistency": "repeatable",
    "sourceRevision": "revision-1042",
    "observedAt": "2026-08-27T10:00:00Z"
  }
}
```

Returning a bare result without `value` and `basis` is not a valid State READ
response. It remains valid for explicit non-READ resource operations whose
result contract is operation-specific.

The runtime, not package code, records the access trace and runtime generation.
