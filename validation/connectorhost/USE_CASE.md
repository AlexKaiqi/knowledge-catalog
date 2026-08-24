# Connector Host MVP acceptance use case

## Story

A platform team owns a Knowledge Catalog. A service team owns an ordinary Git
repository and a JSON service registry. They ask a DSH coding agent to develop
a Connector in that repository. The team deploys nothing to KC: the local Host
mounts the repo, runs the Connector and delivers Address-level changes through
the public Writer protocol.

The scenario deliberately starts with no Connector files. The Agent must infer
and implement the Observer/Translator boundary from `CONNECTOR_SPEC.md` and the
task, including stable identity, FULL coverage, digests and checkpoint output.

## Coverage matrix

| Case | Stimulus | Expected Host result | Expected KC result |
|---|---|---|---|
| Discover | Agent creates `connectors/service-observer/connector.yaml` | Connector appears with bundle digest | none |
| Invalid manifest | Unknown field, bad Scope or mismatched directory ID | validation fails | none |
| Fixture | Translator unit test | conformance passes | none |
| Preview | first FULL observation | `PREVIEWED`, two additions | no commit, no checkpoint |
| Initial commit | same observation | `SUCCEEDED`, checkpoint v1 | two SOURCE Aspects |
| Empty | unchanged source | `EMPTY`, checkpoint advances | no new commit |
| Update | billing owner changes | one update | digest precondition + new commit |
| Addition | inventory service appears | one addition | `IF_ABSENT` PUT |
| Deletion | search disappears from FULL source | one removal | canonical unit removed |
| KEYED safety | reconcile output claims KEYED coverage | run rejected | no write/checkpoint |
| Source failure | command exits non-zero | `FAILED`, stderr recorded | no write/checkpoint |
| Writer failure | target base races | `FAILED` | no partial write/checkpoint |
| Retry | same source after transient failure | succeeds with stable command ID | one commit or replay |
| Checkpoint crash window | Writer succeeded, Host retries before state save | Writer replay | checkpoint catches up |
| Generation pin | code changes after activation | scheduled run rejected | old generation not silently replaced |
| Reactivation | validate and activate changed files | new digest becomes active | subsequent runs use new generation |
| Schedule | active connector becomes due | schedule Run is created | same Writer path as manual |
| Permission | principal lacks target Repo commit grant | Writer rejects | no checkpoint |
| Permission granted | exact Repo commit grant exists | run succeeds | audit stamps principal/rule |
| Browser | open Host console | state and history visible; authorized actions work | canonical state remains KC-owned |
| Provenance | inspect written object | Run links to source observation | SOURCE/sourceRefs/producedAt present |

## Agent acceptance

The real DSH task passes only when:

1. the Agent creates the Connector in the mounted ordinary repo;
2. its unit tests pass without changing the Host or KC;
3. Host validation accepts the manifest and command ABI;
4. preview leaves Writer and checkpoint unchanged;
5. the first real run creates the expected canonical objects;
6. a later FULL observation updates, adds and removes exactly the expected
   objects;
7. Host browser history shows both generations/runs and no hidden direct write;
8. KC read and provenance independently confirm the Host receipts.

## Product boundary asserted by the case

- DSH, Skill and MCP are Agent discovery/operation adapters, not Connector
  domain abstractions.
- Connector code and maintenance policy remain in the user repo.
- Host owns only local execution state.
- KC does not discover, deploy, schedule or execute Connector code.
- Every knowledge mutation still crosses the public Writer protocol.
