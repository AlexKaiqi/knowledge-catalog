#!/usr/bin/env python3
"""Resource Access: serve resource-access/v1 at the Schema origin."""

from __future__ import annotations

import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import re
from typing import Any

import shop
import source

PROTOCOL = "resource-access/v1"
DESCRIPTOR_ID = "resource/shop-sql"
QUERY_CALL = "mysql.query"
GENERATION = "mysql-shop-fixture-v1"
READ_ONLY_SQL = re.compile(r"^\s*(SELECT|SHOW|DESCRIBE|DESC|EXPLAIN)\b", re.IGNORECASE)


class AccessError(ValueError):
    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


def object_id_of(request: dict[str, Any]) -> str:
    binding = request.get("binding") or {}
    address = binding.get("address") or {}
    return str(address.get("objectId") or "Service:orders")


def mysql_configured() -> bool:
    return shop.mysql_configured()


def mysql_lines(sql: str) -> list[str]:
    try:
        return shop.mysql_lines(sql)
    except shop.ShopError as error:
        raise AccessError(error.code, str(error)) from error


def lookup(request: dict[str, Any], principal: str) -> dict[str, Any]:
    if not principal.strip():
        raise AccessError("UNAUTHENTICATED", "resource-access requires X-Resource-Principal")
    operation = str(request.get("operation") or "lookup")
    call = str(request.get("call") or "lookup")
    if operation not in {"lookup", "read"}:
        raise AccessError("CAPABILITY_UNSATISFIED", "State READ uses lookup")
    if call != "lookup" and not call.endswith(".lookup"):
        raise AccessError("CAPABILITY_UNSATISFIED", "unexpected call")
    object_id = object_id_of(request)
    if mysql_configured():
        table = shop.table_from_object_id(object_id)
        if table is not None:
            try:
                if not shop.table_exists(table):
                    raise AccessError("KNOWLEDGE_REF_UNRESOLVED", f"table {object_id} is not in the live shop catalog")
                return {
                    "value": shop.stats_value(table),
                    "basis": {
                        "bindingGeneration": GENERATION,
                        "consistency": "latest-only",
                        "observedAt": shop.observed_at(),
                    },
                }
            except shop.ShopError as error:
                raise AccessError(error.code, str(error)) from error
    current = source.load()
    value = dict(source.entity(object_id))
    if "status" in value:
        value.setdefault("runtime", "docker")
    return {
        "value": value,
        "basis": {
            "bindingGeneration": current["generation"],
            "consistency": "repeatable",
            "sourceRevision": current["revision"],
            "observedAt": "2026-08-27T14:00:00Z",
        },
    }


def execute_query(request: dict[str, Any], principal: str) -> dict[str, Any]:
    if not principal.strip():
        raise AccessError("UNAUTHENTICATED", "resource-access requires X-Resource-Principal")
    if not mysql_configured():
        raise AccessError("CAPABILITY_UNSATISFIED", "SQL query requires a live shop MySQL origin")
    if str(request.get("call") or QUERY_CALL) not in {QUERY_CALL, ""}:
        raise AccessError("CAPABILITY_UNSATISFIED", "unexpected runtime call")
    descriptor = request.get("descriptor")
    if not isinstance(descriptor, dict) or descriptor.get("objectId") != DESCRIPTOR_ID:
        raise AccessError("KNOWLEDGE_REF_UNRESOLVED", "the pinned SQL ResourceDescriptor is required")
    operation_input = request.get("input")
    if not isinstance(operation_input, dict):
        raise AccessError("USAGE_INVALID", "query input must be an object")
    sql = operation_input.get("sql")
    if not isinstance(sql, str) or not sql.strip():
        raise AccessError("USAGE_INVALID", "query input.sql must be a non-empty string")
    statement = sql.strip()
    if statement.endswith(";"):
        statement = statement[:-1].rstrip()
    if not READ_ONLY_SQL.match(statement) or ";" in statement:
        raise AccessError("USAGE_INVALID", "query accepts only one read-only SQL statement")
    try:
        rows = mysql_lines(statement)
        observed = shop.observed_at()
    except shop.ShopError as error:
        raise AccessError(error.code, str(error)) from error
    return {
        "operation": "query",
        "result": {"rows": rows, "rowCount": len(rows)},
        "basis": {
            "runtimeGeneration": GENERATION,
            "consistency": "source-read",
            "observedAt": observed,
            "descriptor": {
                "objectId": DESCRIPTOR_ID,
                "repository": descriptor.get("repository"),
                "commit": descriptor.get("commit"),
            },
        },
    }


def dispatch(request: dict[str, Any], principal: str) -> dict[str, Any]:
    operation = str(request.get("operation") or "lookup")
    if operation == "query":
        return execute_query(request, principal)
    return lookup(request, principal)


class Handler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        if self.path != "/health":
            self.send_error(404)
            return
        self._json(200, {"ok": True, "role": "resource-access", "protocol": PROTOCOL})

    def do_POST(self) -> None:
        if self.path != "/v1/access":
            self.send_error(404)
            return
        try:
            length = int(self.headers.get("content-length", "0"))
            request = json.loads(self.rfile.read(length))
            if not isinstance(request, dict):
                raise ValueError("request must be an object")
            result = dispatch(request, self.headers.get("X-Resource-Principal", ""))
        except AccessError as error:
            self._json(422, {"error": {"code": error.code, "message": str(error)}})
            return
        except (ValueError, json.JSONDecodeError) as error:
            self._json(400, {"error": {"code": "USAGE_INVALID", "message": str(error)}})
            return
        self._json(200, result)

    def log_message(self, _format: str, *_args: Any) -> None:
        return

    def _json(self, status: int, value: Any) -> None:
        body = json.dumps(value, separators=(",", ":"), ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def listen(value: str) -> tuple[str, int]:
    host, separator, raw_port = value.rpartition(":")
    if not separator or not host or not raw_port.isdigit():
        raise argparse.ArgumentTypeError("listen must be host:port")
    return host, int(raw_port)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--listen", type=listen, default=("0.0.0.0", 7390))
    args = parser.parse_args()
    ThreadingHTTPServer(args.listen, Handler).serve_forever()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
