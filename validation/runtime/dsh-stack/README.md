# Independent DSH + Knowledge Catalog stack

This is a disposable scene-side runtime for testing the current root protocol.
It does not add a runtime or source connector to `kc`.

## Topology

```text
browser :17400 -> independent DSH web container -> kc :7380
                                                  |-- Gitea 1.26 :3000 (Snapshot authority)
                                                  `-- Elasticsearch :9200 (derived retrieval)
Gitea -> PostgreSQL (Gitea's own application database)
```

The stack bootstraps two Gitea identities. `kc-admin` owns the three private
Git repositories and is the repository-adapter/service administrator.
`dsh-agent` is the DSH login identity and receives `read-workspace`, explicit
`read` on the three member repositories, and `put,commit` on its personal
repository. The `commit` grant is what authorizes routed raw VFS writes.
The per-repository reads are intentional: Workspace membership does not grant
access to a repository. PATs live only in the private
`kc-secrets` Docker volume.

StarRocks is deliberately absent: the repository currently exposes only an
explicit capability stub. MySQL remains the external TPC-H source fixture in
the adjacent `../compose.yaml`; it is not a Catalog store.

## Start and verify

From this directory:

```bash
cp .env.example .env                 # optional port/credential overrides
docker compose up --build -d --wait
./verify.sh
```

Open:

- DSH: <http://127.0.0.1:17400>
- kc console: <http://127.0.0.1:17380>
- Gitea: <http://127.0.0.1:13000>
- Elasticsearch: <http://127.0.0.1:19200>

The DSH UI can select Catalog `kr://acme/catalog` and Workspace `warehouse`.
All host-facing ports are loopback-only. For DSH, an internal TCP relay preserves
DSH's required `127.0.0.1` bind while making the web UI reachable through the
container port. Its default sandbox mode is `workspace-write` and can be
overridden with `DSH_PERMISSION_MODE`.
Without a model credential the UI, plugin composition, authenticated VFS and
stores are still testable. Set `DEEPSEEK_API_KEY` (or the appropriate DSH
credential configuration) before startup for real Agent conversations.

The verifier checks:

1. Gitea health and the three private Snapshot repositories;
2. authenticated `whoami`, Workspace resolution and Elasticsearch-backed search;
3. routed VFS write/read into the personal Gitea repository;
4. DSH composition contains `loom-control` and `loom-fs`;
5. Workspace pin and personal content survive a `kc` container restart.

## Operate and reset

```bash
docker compose ps
docker compose logs -f kc dsh
docker compose down                 # retain all data
docker compose down -v              # destructive: reset the disposable stack
```

To run a headless Agent after configuring a model credential:

```bash
docker compose run --rm dsh \
  dsh --profile headless \
  --patch /opt/dsh-loom/scripts/deepseek-official.patch.yml \
  '使用 kc 工具读取 warehouse，并说明 orders 和 GMV。'
```
