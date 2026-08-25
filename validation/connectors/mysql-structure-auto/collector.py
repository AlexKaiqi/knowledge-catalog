#!/usr/bin/env python3
"""Scheduled MySQL structure Collector for the DW-04 acceptance scenario."""

from __future__ import annotations

import hashlib
import json
import os
import subprocess
import sys
from typing import Any


SOURCE_REF = "mysql://127.0.0.1:13306/tpch"


TABLE_QUERY = """
SELECT JSON_OBJECT(
  'tableSchema', TABLE_SCHEMA,
  'tableName', TABLE_NAME,
  'tableType', TABLE_TYPE,
  'engine', ENGINE,
  'tableComment', TABLE_COMMENT,
  'tableCollation', TABLE_COLLATION
)
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = 'tpch'
ORDER BY TABLE_NAME;
"""


COLUMN_QUERY = """
SELECT JSON_OBJECT(
  'tableSchema', TABLE_SCHEMA,
  'tableName', TABLE_NAME,
  'columnName', COLUMN_NAME,
  'ordinalPosition', ORDINAL_POSITION,
  'columnDefault', COLUMN_DEFAULT,
  'isNullable', IS_NULLABLE,
  'dataType', DATA_TYPE,
  'columnType', COLUMN_TYPE,
  'characterMaximumLength', CHARACTER_MAXIMUM_LENGTH,
  'numericPrecision', NUMERIC_PRECISION,
  'numericScale', NUMERIC_SCALE,
  'columnKey', COLUMN_KEY,
  'extra', EXTRA,
  'columnComment', COLUMN_COMMENT
)
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = 'tpch'
ORDER BY TABLE_NAME, ORDINAL_POSITION;
"""


CAPTURED_AT_QUERY = (
    "SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%fZ');"
)


def canonical_digest(value: Any) -> str:
    encoded = json.dumps(
        value, sort_keys=True, separators=(",", ":"), ensure_ascii=False
    ).encode("utf-8")
    return hashlib.sha256(encoded).hexdigest()


def stable_object_id(kind: str, source_key: str) -> str:
    digest = hashlib.sha256(source_key.encode("utf-8")).hexdigest()[:24]
    return f"dw-{kind}-{digest}"


def table_key(source_ref: str, schema: str, table: str) -> str:
    return f"{source_ref.rstrip('/')}/table/{schema}/{table}"


def column_key(source_ref: str, schema: str, table: str, column: str) -> str:
    return f"{source_ref.rstrip('/')}/column/{schema}/{table}/{column}"


def address(object_id: str) -> dict[str, str]:
    return {"kind": "Aspect", "objectId": object_id, "aspectName": "structure"}


def validate_snapshot(
    tables: list[dict[str, Any]], columns: list[dict[str, Any]]
) -> None:
    if not tables or not columns:
        raise ValueError("MySQL structure observation is empty")
    table_keys = [
        (item["tableSchema"], item["tableName"])
        for item in tables
    ]
    if len(table_keys) != len(set(table_keys)):
        raise ValueError("MySQL structure observation contains duplicate tables")
    visible_column_tables = {
        (item["tableSchema"], item["tableName"])
        for item in columns
    }
    missing = sorted(set(table_keys) - visible_column_tables)
    if missing:
        names = ", ".join(f"{schema}.{table}" for schema, table in missing)
        raise ValueError(f"visible tables have no visible columns: {names}")


def translate(
    tables: list[dict[str, Any]],
    columns: list[dict[str, Any]],
    source_ref: str = SOURCE_REF,
) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    tables = sorted(tables, key=lambda item: (item["tableSchema"], item["tableName"]))
    columns = sorted(
        columns,
        key=lambda item: (
            item["tableSchema"],
            item["tableName"],
            int(item["ordinalPosition"]),
        ),
    )
    column_counts: dict[str, int] = {}
    for column in columns:
        key = table_key(source_ref, column["tableSchema"], column["tableName"])
        column_counts[key] = column_counts.get(key, 0) + 1

    units: list[dict[str, Any]] = []
    observed: list[dict[str, Any]] = []
    table_ids: dict[str, str] = {}
    seen: set[str] = set()

    def append_unit(object_id: str, source_key: str, path: str, value: dict[str, Any]) -> None:
        key = f"Aspect\x00{object_id}\x00structure\x00"
        if key in seen:
            raise ValueError(f"duplicate translated Address {key}")
        seen.add(key)
        unit_address = address(object_id)
        units.append(
            {
                "address": unit_address,
                "value": value,
                "sourceKey": source_key,
                "pathHint": path,
            }
        )
        observed.append({"address": unit_address, "digest": canonical_digest(value)})

    for table in tables:
        key = table_key(source_ref, table["tableSchema"], table["tableName"])
        object_id = stable_object_id("table", key)
        table_ids[key] = object_id
        value = {
            "entityType": "table",
            "sourceKey": key,
            "schema": table["tableSchema"],
            "name": table["tableName"],
            "qualifiedName": f'{table["tableSchema"]}.{table["tableName"]}',
            "tableType": table["tableType"],
            "engine": table["engine"],
            "comment": table["tableComment"],
            "collation": table["tableCollation"],
            "columnCount": column_counts.get(key, 0),
        }
        append_unit(object_id, key, f"physical/tables/{object_id}.json", value)

    for column in columns:
        parent_key = table_key(
            source_ref, column["tableSchema"], column["tableName"]
        )
        if parent_key not in table_ids:
            raise ValueError(
                f'column {column["tableSchema"]}.{column["tableName"]}.{column["columnName"]} has no table record'
            )
        key = column_key(
            source_ref,
            column["tableSchema"],
            column["tableName"],
            column["columnName"],
        )
        object_id = stable_object_id("column", key)
        value = {
            "entityType": "column",
            "sourceKey": key,
            "parentObjectId": table_ids[parent_key],
            "schema": column["tableSchema"],
            "tableName": column["tableName"],
            "name": column["columnName"],
            "qualifiedName": (
                f'{column["tableSchema"]}.{column["tableName"]}.{column["columnName"]}'
            ),
            "ordinalPosition": int(column["ordinalPosition"]),
            "nullable": column["isNullable"] == "YES",
            "dataType": column["dataType"],
            "columnType": column["columnType"],
            "columnDefault": column["columnDefault"],
            "characterMaximumLength": column["characterMaximumLength"],
            "numericPrecision": column["numericPrecision"],
            "numericScale": column["numericScale"],
            "columnKey": column["columnKey"],
            "extra": column["extra"],
            "comment": column["columnComment"],
        }
        append_unit(object_id, key, f"physical/columns/{object_id}.json", value)

    return units, observed


def mysql_lines(sql: str) -> list[str]:
    container = os.environ.get("KC_MYSQL_CONTAINER", "").strip()
    password = os.environ.get("KC_MYSQL_PASSWORD", "").strip()
    if not container or not password:
        raise RuntimeError("KC_MYSQL_CONTAINER and KC_MYSQL_PASSWORD are required")
    command = [
        "docker",
        "exec",
        "--env",
        f"MYSQL_PWD={password}",
        container,
        "mysql",
        "--user=root",
        "--database=tpch",
        "--batch",
        "--raw",
        "--skip-column-names",
        "--execute",
        sql,
    ]
    completed = subprocess.run(command, check=True, capture_output=True, text=True)
    return [line for line in completed.stdout.splitlines() if line.strip()]


def collect() -> tuple[list[dict[str, Any]], list[dict[str, Any]], str]:
    tables = [json.loads(line) for line in mysql_lines(TABLE_QUERY)]
    columns = [json.loads(line) for line in mysql_lines(COLUMN_QUERY)]
    captured = mysql_lines(CAPTURED_AT_QUERY)
    if len(captured) != 1:
        raise RuntimeError("MySQL structure observation is incomplete")
    validate_snapshot(tables, columns)
    desired, observed = translate(tables, columns)
    return desired, observed, captured[0]


def main() -> int:
    try:
        request = json.load(sys.stdin)
        checkpoint = request.get("checkpoint") or {}
        prior = checkpoint.get("observed") or []
        desired, next_observed, captured_at = collect()
        output = {
            "observation": {
                "sourceRefs": [SOURCE_REF],
                "observedAt": captured_at,
                "representation": "STATE",
                "coverage": {"kind": "FULL"},
            },
            "mode": "reconcile",
            "desired": desired,
            "observed": prior,
            "nextCheckpoint": {"version": 1, "observed": next_observed},
            "message": "reconcile scheduled MySQL tpch structure",
        }
        json.dump(output, sys.stdout, separators=(",", ":"), ensure_ascii=False)
        sys.stdout.write("\n")
        return 0
    except Exception as error:  # process ABI reports diagnostics on stderr only
        print(f"mysql-structure-auto: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
