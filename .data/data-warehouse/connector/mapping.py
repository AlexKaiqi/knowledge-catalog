#!/usr/bin/env python3
"""Translate MySQL adapter rows into provider-owned Knowledge units."""

from __future__ import annotations

from typing import Any

import domain


CONNECTOR_ID = "mysql-tpch"
PLATFORM_KEY = "mysql:fixture:platform"
DATABASE_KEY = "mysql:fixture:database:tpch"
SCHEMA_KEY = "mysql:fixture:schema:tpch"


def table_key(schema: str, table: str) -> str:
    return f"mysql:fixture:table:{schema}.{table}"


def column_key(schema: str, table: str, column: str) -> str:
    return f"mysql:fixture:column:{schema}.{table}.{column}"


def job_key(schema: str, job: str) -> str:
    return f"mysql:fixture:data-job:{schema}.{job}"


def validate_snapshot(
    tables: list[dict[str, Any]],
    columns: list[dict[str, Any]],
    jobs: list[dict[str, Any]],
) -> None:
    if not tables or not columns:
        raise ValueError("MySQL structure observation is empty")
    table_names = [(item["tableSchema"], item["tableName"]) for item in tables]
    if len(table_names) != len(set(table_names)):
        raise ValueError("MySQL structure observation contains duplicate tables")
    column_names = [
        (item["tableSchema"], item["tableName"], item["columnName"])
        for item in columns
    ]
    if len(column_names) != len(set(column_names)):
        raise ValueError("MySQL structure observation contains duplicate columns")
    visible_column_tables = {(item["tableSchema"], item["tableName"]) for item in columns}
    missing = sorted(set(table_names) - visible_column_tables)
    if missing:
        names = ", ".join(f"{schema}.{table}" for schema, table in missing)
        raise ValueError(f"visible tables have no visible columns: {names}")
    job_names = [(item["eventSchema"], item["eventName"]) for item in jobs]
    if len(job_names) != len(set(job_names)):
        raise ValueError("MySQL structure observation contains duplicate jobs")


def translate(
    tables: list[dict[str, Any]],
    columns: list[dict[str, Any]],
    jobs: list[dict[str, Any]],
) -> tuple[list[dict[str, Any]], dict[str, str]]:
    validate_snapshot(tables, columns, jobs)
    grouped: dict[tuple[str, str], list[dict[str, Any]]] = {}
    for column in columns:
        grouped.setdefault((column["tableSchema"], column["tableName"]), []).append(column)
    translated_tables = []
    for table in sorted(tables, key=lambda item: (item["tableSchema"], item["tableName"])):
        schema = table["tableSchema"]
        name = table["tableName"]
        translated_columns = []
        primary_key = []
        for column in sorted(grouped[(schema, name)], key=lambda item: int(item["ordinalPosition"])):
            column_name = column["columnName"]
            translated_columns.append({
                "sourceKey": column_key(schema, name, column_name),
                "nativeId": f"{schema}.{name}.{column_name}",
                "name": column_name,
                "dataType": column["columnType"],
                "nullable": column["isNullable"] == "YES",
                "ordinal": int(column["ordinalPosition"]),
            })
            if column["columnKey"] == "PRI":
                primary_key.append(column_name)
        translated_tables.append({
            "sourceKey": table_key(schema, name),
            "nativeId": f"{schema}.{name}",
            "name": name,
            "nativeType": table["tableType"],
            "comment": table.get("tableComment", ""),
            "primaryKey": primary_key,
            "columns": translated_columns,
        })
    snapshot = {
        "connectorId": CONNECTOR_ID,
        "platform": {
            "sourceKey": PLATFORM_KEY,
            "nativeId": "mysql-fixture",
            "name": "MySQL TPC-H fixture",
            "environment": "TEST",
        },
        "databases": [{
            "sourceKey": DATABASE_KEY,
            "nativeId": "tpch",
            "name": "tpch",
            "schemas": [{
                "sourceKey": SCHEMA_KEY,
                "nativeId": "tpch",
                "name": "tpch",
                "tables": translated_tables,
                "jobs": [{
                    "sourceKey": job_key(item["eventSchema"], item["eventName"]),
                    "nativeId": f"{item['eventSchema']}.{item['eventName']}",
                    "name": item["eventName"],
                    "nativeType": "MYSQL_EVENT",
                    "language": "SQL",
                    "sourceCode": item["eventDefinition"],
                    "schedule": {
                        "type": item["eventType"],
                        "intervalValue": item["intervalValue"],
                        "intervalField": item["intervalField"],
                    },
                    "enabled": item["status"] == "ENABLED",
                    "description": item.get("eventComment") or "",
                } for item in sorted(jobs, key=lambda value: (value["eventSchema"], value["eventName"]))],
            }],
        }],
    }
    return domain.physical_desired(snapshot)
