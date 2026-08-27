#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import random
from datetime import datetime, timezone
from pathlib import Path


PROFILES = {
    "S0": {"tables": 10_000, "columnsPerTable": 30},
    "S1": {"tables": 100_000, "columnsPerTable": 30},
    "S2": {"tables": 500_000, "columnsPerTable": 30},
    "S3": {"tables": 1_000_000, "columnsPerTable": 30},
}
HISTORY = {
    "H0": 100_000,
    "H1": 1_000_000,
    "H2": 5_000_000,
    "H3": 10_000_000,
    "H4": 20_000_000,
}


def write_json(path: Path, value: object) -> None:
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def table_key(index: int) -> str:
    return f"mysql:scale:table:warehouse.t_{index:07d}"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--profile", choices=PROFILES, required=True)
    parser.add_argument("--history", choices=HISTORY, required=True)
    parser.add_argument("--events", type=int, default=10_000)
    parser.add_argument("--seed", type=int, default=20260827)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    args.out.mkdir(parents=True, exist_ok=True)
    model = dict(PROFILES[args.profile])
    model.update({
        "profile": args.profile,
        "historyTier": args.history,
        "targetCommits": HISTORY[args.history],
        "objectsPerTable": 32,
        "groupedRelationsPerTable": 1,
        "maxRelationEndpoints": 256,
    })
    manifest = {
        "generatedAt": datetime.now(timezone.utc).isoformat(),
        "seed": args.seed,
        "profile": args.profile,
        "history": args.history,
        "events": args.events,
        "generatorVersion": 1,
    }
    write_json(args.out / "manifest.json", manifest)
    write_json(args.out / "model.json", model)

    with (args.out / "bootstrap.ndjson").open("w", encoding="utf-8") as output:
        for index in range(model["tables"]):
            output.write(json.dumps({
                "familyKey": table_key(index),
                "columns": model["columnsPerTable"],
                "ordinal": index,
            }, separators=(",", ":")) + "\n")

    rng = random.Random(args.seed)
    event_types = ["ALTER_COLUMN", "ADD_COLUMN", "DROP_COLUMN", "RENAME_TABLE"]
    with (args.out / "events.ndjson").open("w", encoding="utf-8") as output:
        for sequence in range(args.events):
            table = rng.randrange(model["tables"])
            output.write(json.dumps({
                "sequence": sequence + 1,
                "kind": "invalidation",
                "keys": [table_key(table)],
                "mutation": event_types[sequence % len(event_types)],
            }, separators=(",", ":")) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
