"""Qinghe shop MySQL catalog for walk Resource Access / Collector / Observer.

Bound State is schema-level: any Table entity under this database is a stats
coordinate. Collector translates INFORMATION_SCHEMA into table-meta units.
Observer notices stats for every live table. Protocol compose without MySQL
does not import this path.
"""

from __future__ import annotations

from dataclasses import dataclass, field
import hashlib
import json
import os
import re
import subprocess
from typing import Any

TABLE_IDENT = re.compile(r"^[A-Za-z0-9_]+$")
TABLE_OBJECT = re.compile(r"^table/([A-Za-z0-9_]+)\.([A-Za-z0-9_]+)$")
INT_TYPES = {"int", "tinyint", "smallint", "mediumint", "bigint"}


class ShopError(RuntimeError):
    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


@dataclass
class Column:
    name: str
    data_type: str
    primary_key: bool


@dataclass
class Table:
    name: str
    columns: list[Column] = field(default_factory=list)

    @property
    def object_id(self) -> str:
        return table_object_id(self.name)

    @property
    def primary_keys(self) -> list[str]:
        return [column.name for column in self.columns if column.primary_key]


@dataclass
class Job:
    name: str
    enabled: bool
    interval_value: str
    interval_field: str
    source_code: str
    description: str

    @property
    def object_id(self) -> str:
        return f"data-job/{database()}.{self.name}"


@dataclass
class Catalog:
    tables: list[Table] = field(default_factory=list)
    jobs: list[Job] = field(default_factory=list)

    def fingerprint(self) -> str:
        payload = json.dumps(
            {
                "tables": [
                    {
                        "name": table.name,
                        "columns": [
                            {
                                "name": column.name,
                                "data_type": column.data_type,
                                "primary_key": column.primary_key,
                            }
                            for column in table.columns
                        ],
                    }
                    for table in self.tables
                ],
                "jobs": [
                    {
                        "name": job.name,
                        "enabled": job.enabled,
                        "interval_value": job.interval_value,
                        "interval_field": job.interval_field,
                        "source_code": job.source_code,
                        "description": job.description,
                    }
                    for job in self.jobs
                ],
            },
            separators=(",", ":"),
            ensure_ascii=False,
        )
        return hashlib.sha256(payload.encode()).hexdigest()


def mysql_configured() -> bool:
    return bool(os.environ.get("KC_MYSQL_HOST", "").strip() and os.environ.get("KC_MYSQL_PASSWORD", "").strip())


def database() -> str:
    name = os.environ.get("KC_MYSQL_DATABASE", "shop").strip() or "shop"
    if not TABLE_IDENT.match(name):
        raise ShopError("USAGE_INVALID", f"unsupported database identifier {name}")
    return name


def table_object_id(table: str) -> str:
    if not TABLE_IDENT.match(table):
        raise ShopError("USAGE_INVALID", f"unsupported table identifier {table}")
    return f"table/{database()}.{table}"


def table_from_object_id(object_id: str) -> str | None:
    matched = TABLE_OBJECT.match(object_id)
    if not matched:
        return None
    db_name, table = matched.group(1), matched.group(2)
    if db_name != database():
        return None
    return table


def mysql_lines(sql: str) -> list[str]:
    host = os.environ.get("KC_MYSQL_HOST", "").strip()
    port = os.environ.get("KC_MYSQL_PORT", "3306").strip()
    user = os.environ.get("KC_MYSQL_USER", "root").strip()
    password = os.environ.get("KC_MYSQL_PASSWORD", "").strip()
    env = dict(os.environ)
    env["MYSQL_PWD"] = password
    completed = subprocess.run(
        [
            "mysql", f"--host={host}", f"--port={port}", f"--user={user}",
            f"--database={database()}", "--batch", "--raw", "--skip-column-names",
            "--ssl=0", "--execute", sql,
        ],
        capture_output=True,
        text=True,
        env=env,
    )
    if completed.returncode != 0:
        detail = (completed.stderr or completed.stdout or "mysql failed").strip()
        raise ShopError("TEMPORARY_UNAVAILABLE", f"MySQL lookup failed: {detail}")
    return [line for line in completed.stdout.splitlines() if line.strip()]


def mysql_json(sql: str) -> Any:
    lines = mysql_lines(sql)
    if len(lines) != 1:
        raise ShopError("USAGE_INVALID", "MySQL JSON result is incomplete")
    try:
        return json.loads(lines[0])
    except json.JSONDecodeError as error:
        raise ShopError("USAGE_INVALID", f"MySQL JSON result is invalid: {error}") from error


def native_type(data_type: str) -> str:
    lowered = data_type.strip().lower()
    if lowered in INT_TYPES:
        return "integer"
    return lowered


def list_tables() -> list[str]:
    names = mysql_lines(
        "SELECT TABLE_NAME FROM information_schema.TABLES "
        "WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE' "
        "ORDER BY TABLE_NAME"
    )
    return [name for name in names if TABLE_IDENT.match(name)]


def table_exists(table: str) -> bool:
    if not TABLE_IDENT.match(table):
        return False
    found = mysql_lines(
        "SELECT TABLE_NAME FROM information_schema.TABLES "
        f"WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE' AND TABLE_NAME = '{table}'"
    )
    return found == [table]


def row_count(table: str) -> int:
    if not TABLE_IDENT.match(table):
        raise ShopError("USAGE_INVALID", f"unsupported table identifier {table}")
    values = mysql_lines(f"SELECT COUNT(*) FROM `{table}`")
    if len(values) != 1:
        raise ShopError("USAGE_INVALID", f"MySQL row count for {table} is incomplete")
    return int(values[0])


def observed_at() -> str:
    values = mysql_lines("SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%fZ')")
    return values[0] if values else "2026-08-27T14:00:00Z"


def stats_value(table: str) -> dict[str, Any]:
    return {"rowCount": row_count(table), "table": table}


def stats_fingerprint() -> str:
    counts = {name: row_count(name) for name in list_tables()}
    return hashlib.sha256(json.dumps(counts, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def load_catalog() -> Catalog:
    catalog = Catalog()
    for table_name in list_tables():
        table = Table(name=table_name)
        rows = mysql_lines(
            "SELECT CONCAT_WS('\t', COLUMN_NAME, DATA_TYPE, COLUMN_KEY) "
            "FROM information_schema.COLUMNS "
            f"WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = '{table_name}' "
            "ORDER BY ORDINAL_POSITION"
        )
        for row in rows:
            parts = row.split("\t")
            if len(parts) != 3 or not TABLE_IDENT.match(parts[0]):
                continue
            table.columns.append(
                Column(name=parts[0], data_type=native_type(parts[1]), primary_key=parts[2] == "PRI")
            )
        catalog.tables.append(table)
    for job_name in mysql_lines(
        "SELECT EVENT_NAME FROM information_schema.EVENTS "
        "WHERE EVENT_SCHEMA = DATABASE() ORDER BY EVENT_NAME"
    ):
        if not TABLE_IDENT.match(job_name):
            continue
        payload = mysql_json(
            "SELECT JSON_OBJECT("
            "'status', STATUS, "
            "'intervalValue', CAST(INTERVAL_VALUE AS CHAR), "
            "'intervalField', INTERVAL_FIELD, "
            "'sourceCode', EVENT_DEFINITION, "
            "'description', EVENT_COMMENT"
            ") FROM information_schema.EVENTS "
            f"WHERE EVENT_SCHEMA = DATABASE() AND EVENT_NAME = '{job_name}'"
        )
        if not isinstance(payload, dict):
            continue
        catalog.jobs.append(
            Job(
                name=job_name,
                enabled=str(payload.get("status", "")).upper() == "ENABLED",
                interval_value=str(payload.get("intervalValue") or "1"),
                interval_field=str(payload.get("intervalField") or "DAY"),
                source_code=str(payload.get("sourceCode") or "").strip(),
                description=str(payload.get("description") or "").strip(),
            )
        )
    return catalog
