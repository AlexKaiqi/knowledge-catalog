# Unified scenario library

`library.yaml` is the single inventory for data-warehouse validation. Its
public entries start from a user's goal and expected outcome. TPC-H SF0.01,
commands and protocol operations are reusable implementation actions, not user
scenarios.

There are three structures:

- `journey`: a user, goal, initial path and observable outcome;
- `action`: one executable technical operation with an exact oracle;
- `suite`: a test grouping of journeys; it is not itself a user scenario.

Actions may declare `needs`. Journeys and suites only compose IDs. The runner
expands them into one topological, de-duplicated action plan, so complex user
tasks do not copy shell or Go operations. Existing test implementations remain
close to their domain code; the library is the only place that composes them.

```bash
go run ./validation/cmd/scenario-runner --list
go run ./validation/cmd/scenario-runner --list --list-actions
go run ./validation/cmd/scenario-runner --plan all
go run ./validation/cmd/scenario-runner producer
go run ./validation/cmd/scenario-runner consumer
go run ./validation/cmd/scenario-runner all
```

The last command writes a machine-readable run report below the ignored
`.data/datawarehouse/scenarios/` directory. `validation/playbook.sh` is a
compatibility shim for the same runner.

When adding coverage, start with the user's job and pass condition. Add an
action only when the journey needs an operation or oracle that does not already
exist. If it exists, reference its ID rather than adding another script.
