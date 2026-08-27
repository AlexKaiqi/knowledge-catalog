#!/usr/bin/env python3
"""Reconcile live MySQL metadata after a signal or periodic FULL trigger.

Collector owns checkpoint and observation semantics. All source I/O goes
through ``MySQLAdapter``; source-key/object translation goes through
``mapping``. It never calls KC or writes a Repository.
"""

from __future__ import annotations

import json
import sys
from typing import Any

from adapter import MySQLAdapter
import domain
import mapping


SOURCE_REF = "mysql://fixture/tpch"


def collect(adapter: MySQLAdapter) -> tuple[list[dict[str, Any]], dict[str, str], str]:
    tables = [json.loads(line) for line in adapter.list_tables()]
    columns = [json.loads(line) for line in adapter.describe_all_columns()]
    jobs = [json.loads(line) for line in adapter.list_jobs()]
    desired, source_mapping = mapping.translate(tables, columns, jobs)
    return desired, source_mapping, adapter.captured_at()


def _table_name(source_key: str) -> str:
    prefix = "mysql:fixture:table:tpch."
    if not source_key.startswith(prefix) or source_key == prefix:
        raise ValueError(f"unsupported targeted invalidation key {source_key}")
    return source_key[len(prefix):]


def _column_prefix(table_key: str) -> str:
    return table_key.replace("mysql:fixture:table:", "mysql:fixture:column:") + "."


def _in_table_scope(source_key: str, table_keys: set[str]) -> bool:
    return any(
        source_key == table_key
        or source_key.startswith(table_key + "#")
        or source_key.startswith(_column_prefix(table_key))
        for table_key in table_keys
    )


def collect_targeted(
    adapter: MySQLAdapter,
    table_keys: set[str],
) -> tuple[list[dict[str, Any]], dict[str, str], str]:
    tables = []
    columns = []
    for source_key in sorted(table_keys):
        table = _table_name(source_key)
        tables.extend(json.loads(line) for line in adapter.describe_table(table))
        columns.extend(json.loads(line) for line in adapter.describe_schema(table))
    if not tables:
        return [], {}, adapter.captured_at()
    translated, source_mapping = mapping.translate(tables, columns, [])
    desired = [
        item for item in translated
        if _in_table_scope(str(item.get("sourceKey", "")), table_keys)
    ]
    scoped_mapping = {
        key: value for key, value in source_mapping.items()
        if _in_table_scope(key, table_keys)
    }
    return desired, scoped_mapping, adapter.captured_at()


def observed_declaration(item: dict[str, Any]) -> dict[str, Any]:
    result = {
        "address": item["address"],
        "digest": domain.canonical_digest(item["value"]),
        "declarationDigest": domain.declaration_digest(item.get("schemaRef", "")),
    }
    if item.get("sourceKey"):
        result["sourceKey"] = item["sourceKey"]
    return result


def targeted_checkpoint(
    checkpoint: dict[str, Any],
    desired: list[dict[str, Any]],
    source_mapping: dict[str, str],
    table_keys: set[str],
    captured_at: str,
) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    prior_observed = checkpoint.get("observed") or []
    if prior_observed and checkpoint.get("version") != 3:
        raise ValueError("targeted invalidation requires a v3 checkpoint; run one explicit full reconcile")
    observed = [
        item for item in prior_observed
        if _in_table_scope(str(item.get("sourceKey", "")), table_keys)
    ]
    merged_observed = [
        item for item in prior_observed
        if not _in_table_scope(str(item.get("sourceKey", "")), table_keys)
    ] + [observed_declaration(item) for item in desired]
    merged_mapping = {
        key: value for key, value in (checkpoint.get("sourceKeyMap") or {}).items()
        if not _in_table_scope(key, table_keys)
    }
    merged_mapping.update(source_mapping)
    return observed, {
        "version": 3,
        "observed": merged_observed,
        "sourceKeyMap": merged_mapping,
        "capturedAt": captured_at,
    }


def main() -> int:
    try:
        request = json.load(sys.stdin)
        checkpoint = request.get("checkpoint") or {}
        signal = request.get("signal") or {"kind": "manual"}
        adapter = MySQLAdapter()
        targeted_keys = set(signal.get("keys") or []) if signal.get("kind") == "invalidation" else set()
        if targeted_keys:
            desired, source_mapping, captured_at = collect_targeted(adapter, targeted_keys)
            observed, next_checkpoint = targeted_checkpoint(
                checkpoint, desired, source_mapping, targeted_keys, captured_at,
            )
            coverage = {"kind": "KEYS", "keys": sorted(targeted_keys)}
            message = "reconcile targeted MySQL metadata after invalidation"
        else:
            desired, source_mapping, captured_at = collect(adapter)
            observed = checkpoint.get("observed") or []
            next_checkpoint = {
                "version": 3,
                "observed": [observed_declaration(item) for item in desired],
                "sourceKeyMap": source_mapping,
                "capturedAt": captured_at,
            }
            coverage = {"kind": "FULL"}
            message = "explicitly reconcile current MySQL metadata"
        output = {
            "observation": {
                "sourceRefs": [SOURCE_REF],
                "observedAt": captured_at,
                "representation": "STATE",
                "coverage": coverage,
                "trigger": signal,
            },
            "mode": "reconcile",
            "desired": desired,
            "observed": observed,
            "nextCheckpoint": next_checkpoint,
            "message": message,
        }
        json.dump(output, sys.stdout, separators=(",", ":"), ensure_ascii=False)
        sys.stdout.write("\n")
        return 0
    except Exception as error:
        print(f"mysql-tpch collector: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
