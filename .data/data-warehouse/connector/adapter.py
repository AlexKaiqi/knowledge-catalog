#!/usr/bin/env python3
"""MySQL source adapter for metadata lookup and guarded runtime operations.

The adapter owns source I/O only. It does not mint object_id, diff knowledge,
manage checkpoints, or call KC Writer.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
from typing import Any


TABLE_QUERY = """
SELECT JSON_OBJECT(
  'tableSchema', TABLE_SCHEMA,
  'tableName', TABLE_NAME,
  'tableType', TABLE_TYPE,
  'tableComment', TABLE_COMMENT
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
  'isNullable', IS_NULLABLE,
  'dataType', DATA_TYPE,
  'columnType', COLUMN_TYPE,
  'columnKey', COLUMN_KEY
)
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = 'tpch'
ORDER BY TABLE_NAME, ORDINAL_POSITION;
"""

JOB_QUERY = """
SELECT JSON_OBJECT(
  'eventSchema', EVENT_SCHEMA,
  'eventName', EVENT_NAME,
  'eventType', EVENT_TYPE,
  'intervalValue', INTERVAL_VALUE,
  'intervalField', INTERVAL_FIELD,
  'status', STATUS,
  'eventDefinition', EVENT_DEFINITION,
  'eventComment', EVENT_COMMENT
)
FROM INFORMATION_SCHEMA.EVENTS
WHERE EVENT_SCHEMA = 'tpch'
ORDER BY EVENT_NAME;
"""

CAPTURED_AT_QUERY = "SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%fZ');"
READ_ONLY_SQL = re.compile(r"^\s*(SELECT|SHOW|DESCRIBE|DESC|EXPLAIN)\b", re.IGNORECASE)


class MySQLAdapter:
    def __init__(self) -> None:
        self.container = os.environ.get("KC_MYSQL_CONTAINER", "").strip()
        self.host = os.environ.get("KC_MYSQL_HOST", "").strip()
        self.port = os.environ.get("KC_MYSQL_PORT", "3306").strip()
        self.user = os.environ.get("KC_MYSQL_USER", "root").strip()
        self.password = os.environ.get("KC_MYSQL_PASSWORD", "").strip()
        self.database = os.environ.get("KC_MYSQL_DATABASE", "tpch").strip()
        if not self.password or (not self.container and not self.host):
            raise RuntimeError(
                "KC_MYSQL_PASSWORD and either KC_MYSQL_CONTAINER or KC_MYSQL_HOST are required"
            )

    def _command(self, sql: str) -> list[str]:
        mysql = [
            "mysql", f"--user={self.user}", f"--database={self.database}",
            "--batch", "--raw", "--skip-column-names", "--execute", sql,
        ]
        if self.container:
            return [
                "docker", "exec", "--env", f"MYSQL_PWD={self.password}",
                self.container, *mysql,
            ]
        return [
            "mysql", f"--host={self.host}", f"--port={self.port}",
            f"--user={self.user}", f"--database={self.database}",
            "--batch", "--raw", "--skip-column-names", "--execute", sql,
        ]

    def lines(self, sql: str) -> list[str]:
        env = dict(os.environ)
        env["MYSQL_PWD"] = self.password
        completed = subprocess.run(
            self._command(sql), check=True, capture_output=True, text=True, env=env
        )
        return [line for line in completed.stdout.splitlines() if line.strip()]

    def list_tables(self) -> list[str]:
        return self.lines(TABLE_QUERY)

    def describe_table(self, table: str) -> list[str]:
        escaped = table.replace("'", "''")
        return self.lines(
            TABLE_QUERY.replace(
                "WHERE TABLE_SCHEMA = 'tpch'",
                f"WHERE TABLE_SCHEMA = 'tpch' AND TABLE_NAME = '{escaped}'",
            )
        )

    def describe_all_columns(self) -> list[str]:
        return self.lines(COLUMN_QUERY)

    def list_jobs(self) -> list[str]:
        return self.lines(JOB_QUERY)

    def captured_at(self) -> str:
        values = self.lines(CAPTURED_AT_QUERY)
        if len(values) != 1:
            raise RuntimeError("MySQL observation timestamp is incomplete")
        return values[0]

    def describe_schema(self, table: str) -> list[str]:
        escaped = table.replace("'", "''")
        return self.lines(
            COLUMN_QUERY.replace(
                "WHERE TABLE_SCHEMA = 'tpch'",
                f"WHERE TABLE_SCHEMA = 'tpch' AND TABLE_NAME = '{escaped}'",
            )
        )

    def query(self, sql: str) -> list[str]:
        statement = sql.strip()
        if statement.endswith(";"):
            statement = statement[:-1].rstrip()
        if not READ_ONLY_SQL.match(statement) or ";" in statement:
            raise ValueError("query accepts only SELECT/SHOW/DESCRIBE/EXPLAIN")
        return self.lines(statement)

    def execute(self, sql: str) -> list[str]:
        return self.lines(sql)

    def call(self, operation: str, arguments: dict[str, Any]) -> Any:
        if operation == "listTables":
            return self.list_tables()
        if operation == "listJobs":
            return self.list_jobs()
        if operation == "describeSchema":
            return self.describe_schema(str(arguments["table"]))
        if operation == "describeTable":
            return self.describe_table(str(arguments["table"]))
        if operation == "query":
            return self.query(str(arguments["sql"]))
        if operation == "execute":
            return self.execute(str(arguments["sql"]))
        raise ValueError(f"unsupported MySQL adapter operation {operation}")


def main() -> int:
    try:
        request = json.load(sys.stdin)
        operation = str(request["operation"])
        result = MySQLAdapter().call(operation, request.get("arguments") or {})
        json.dump({"operation": operation, "result": result}, sys.stdout, separators=(",", ":"))
        sys.stdout.write("\n")
        return 0
    except Exception as error:
        print(f"mysql-tpch adapter: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
