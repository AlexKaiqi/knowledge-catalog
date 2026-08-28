#!/usr/bin/env python3
"""Resource-access provider for the fixture's read-only MySQL SQL capability.

The ResourceDescriptor is versioned knowledge. This process owns the live
endpoint, credentials, source call, and observation evidence. It can serve the
platform ``resource-access/v1`` HTTP contract or process one request on stdin.
"""

from __future__ import annotations

import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import sys
from typing import Any

from adapter import MySQLAdapter


DESCRIPTOR_ID = "resource/mysql-tpch-sql"
RUNTIME = "mysql-tpch"
PROTOCOL = "resource-access/v1"
OPERATION = "query"
CALL = "mysql.query"
GENERATION = "mysql-tpch-fixture-v1"


class AccessError(ValueError):
    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


def execute_request(
    request: dict[str, Any],
    *,
    principal: str,
    adapter: MySQLAdapter | None = None,
) -> dict[str, Any]:
    if not principal.strip():
        raise AccessError("UNAUTHENTICATED", "resource principal is required")
    if request.get("runtime") != RUNTIME or request.get("protocol") != PROTOCOL:
        raise AccessError("CAPABILITY_UNSATISFIED", "unexpected runtime or protocol")
    if request.get("operation") != OPERATION:
        raise AccessError("CAPABILITY_UNSATISFIED", "only the query operation is supported")
    call = request.get("call")
    if call not in (None, CALL):
        raise AccessError("CAPABILITY_UNSATISFIED", "unexpected runtime call")
    descriptor = request.get("descriptor")
    if not isinstance(descriptor, dict) or descriptor.get("objectId") != DESCRIPTOR_ID:
        raise AccessError("KNOWLEDGE_REF_UNRESOLVED", "the pinned SQL ResourceDescriptor is required")
    if not str(descriptor.get("repository", "")).strip() or not str(descriptor.get("commit", "")).strip():
        raise AccessError("PRECONDITION_FAILED", "descriptor repository and commit are required")
    operation_input = request.get("input")
    if not isinstance(operation_input, dict):
        raise AccessError("USAGE_INVALID", "query input must be an object")
    sql = operation_input.get("sql")
    if not isinstance(sql, str) or not sql.strip():
        raise AccessError("USAGE_INVALID", "query input.sql must be a non-empty string")

    source = adapter or MySQLAdapter()
    try:
        rows = source.query(sql)
        observed_at = source.captured_at()
    except ValueError as error:
        raise AccessError("USAGE_INVALID", str(error)) from error
    return {
        "operation": OPERATION,
        "result": {"rows": rows, "rowCount": len(rows)},
        "basis": {
            "runtimeGeneration": GENERATION,
            "consistency": "source-read",
            "observedAt": observed_at,
            "descriptor": {
                "objectId": DESCRIPTOR_ID,
                "repository": descriptor["repository"],
                "commit": descriptor["commit"],
            },
        },
    }


class Handler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        if self.path != "/health":
            self.send_error(404)
            return
        self._json(200, {"ok": True, "runtime": RUNTIME, "protocol": PROTOCOL})

    def do_POST(self) -> None:
        if self.path != "/v1/access":
            self.send_error(404)
            return
        try:
            length = int(self.headers.get("content-length", "0"))
            request = json.loads(self.rfile.read(length))
            if not isinstance(request, dict):
                raise ValueError("request must be an object")
            result = execute_request(
                request,
                principal=self.headers.get("X-Resource-Principal", ""),
            )
        except AccessError as error:
            self._json(422, {"error": {"code": error.code, "message": str(error)}})
            return
        except (ValueError, json.JSONDecodeError) as error:
            self._json(400, {"error": {"code": "USAGE_INVALID", "message": str(error)}})
            return
        except Exception as error:
            self._json(503, {"error": {"code": "TEMPORARY_UNAVAILABLE", "message": str(error)}})
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


def _listen(value: str) -> tuple[str, int]:
    host, separator, raw_port = value.rpartition(":")
    if not separator or not host or not raw_port.isdigit():
        raise argparse.ArgumentTypeError("listen must be host:port")
    return host, int(raw_port)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--listen", type=_listen)
    args = parser.parse_args()
    if args.listen:
        ThreadingHTTPServer(args.listen, Handler).serve_forever()
        return 0
    try:
        request = json.load(sys.stdin)
        principal = str((request.get("identity") or {}).get("principal", ""))
        json.dump(execute_request(request, principal=principal), sys.stdout, separators=(",", ":"))
        sys.stdout.write("\n")
        return 0
    except AccessError as error:
        json.dump({"error": {"code": error.code, "message": str(error)}}, sys.stdout, separators=(",", ":"))
        sys.stdout.write("\n")
        return 1
    except Exception as error:
        print(f"mysql-tpch access: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
