# Build a file-backed service observer

This is a disposable copy of the shared public Connector development
repository. Load `connector-development`, then read `CONNECTOR_SPEC.md` before
editing.

Create `connectors/service-observer/` containing:

- `connector.yaml` conforming to `connector.kc/v1alpha1`;
- a Python 3 standard-library command `observer.py`;
- unit tests under `tests/`.

The command must read `RunRequest` JSON from stdin and observe
`../../sources/services.json`. It must output a FULL STATE reconcile batch to:

- repository `kr://agent/public/services`;
- ref `refs/heads/main`;
- Aspect `observed`;
- object IDs `Service:<stable service key>`.

Each desired value must contain exactly `key`, `name`, and `owner`. Persist the
prior Address digests in `nextCheckpoint.observed` so later runs can update and
remove safely. Use deterministic compact sorted JSON and SHA-256 for digests.

Declare both manual and `schedule every: 1m` triggers, a 30 second timeout, and
a test command using Python unittest. Do not write credentials or invoke KC
inside the Connector command.

Run the unit tests. Do not activate or deploy the Connector. Finish by printing
exactly `CONNECTOR_DEVELOPED=service-observer`.
