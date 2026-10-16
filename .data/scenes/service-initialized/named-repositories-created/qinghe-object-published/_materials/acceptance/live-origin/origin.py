#!/usr/bin/env python3
"""Live origin: a small, controllable resource-access/v1 source for LiveAccess acceptance.

The service answers State lookups by unit address (object, aspect, optional member).
Each unit has a scripted behavior so acceptance cases can observe value, legal null,
source failure, timeout, schema-violating value and malformed basis independently.
Every lookup is recorded so cases can assert who was called, with which identity and
which pinned declaration. Only the Python standard library is used.
"""

from __future__ import annotations

import argparse
import copy
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import threading
import time
from typing import Any
from urllib.parse import unquote

PROTOCOL = "resource-access/v1"
BEHAVIORS = {"ok", "null", "error", "slow", "invalid", "bad-basis"}
RECORDED_HEADERS = (
    "Authorization",
    "X-Tai-Identity",
    "X-Resource-Principal",
    "X-Resource-On-Behalf-Of",
    "X-Resource-Request-Id",
)


class LookupError_(Exception):
    def __init__(self, status: int, code: str, message: str) -> None:
        super().__init__(message)
        self.status = status
        self.code = code


def unit_address(address: dict[str, Any]) -> str:
    object_id = str(address.get("objectId") or "").strip()
    aspect = str(address.get("aspectName") or "").strip()
    member = str(address.get("memberKey") or "").strip()
    if not object_id or not aspect:
        raise LookupError_(400, "USAGE_INVALID", "binding.address requires objectId and aspectName")
    return f"{object_id}#/{aspect}/{member}" if member else f"{object_id}#/{aspect}"


def now() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ")


class Origin:
    """Holds scripted units, the source revision and the call record."""

    def __init__(self, units: dict[str, dict[str, Any]], generation: str, name: str) -> None:
        self._lock = threading.Lock()
        self._initial = copy.deepcopy(units)
        self._units = copy.deepcopy(units)
        self._revision = 1
        self._calls: list[dict[str, Any]] = []
        self.generation = generation
        self.name = name
        for address, unit in units.items():
            validate_unit(address, unit)

    def reset(self) -> None:
        with self._lock:
            self._units = copy.deepcopy(self._initial)
            self._revision = 1
            self._calls = []

    def put_unit(self, address: str, unit: dict[str, Any]) -> int:
        validate_unit(address, unit)
        with self._lock:
            self._units[address] = copy.deepcopy(unit)
            self._revision += 1
            return self._revision

    def delete_unit(self, address: str) -> int:
        with self._lock:
            self._units.pop(address, None)
            self._revision += 1
            return self._revision

    def calls(self) -> list[dict[str, Any]]:
        with self._lock:
            return copy.deepcopy(self._calls)

    def clear_calls(self) -> None:
        with self._lock:
            self._calls = []

    def snapshot(self) -> dict[str, Any]:
        with self._lock:
            return {"name": self.name, "generation": self.generation, "revision": self._revision,
                    "units": copy.deepcopy(self._units)}

    def lookup(self, request: dict[str, Any], headers: dict[str, str]) -> dict[str, Any]:
        binding = request.get("binding")
        if not isinstance(binding, dict) or not isinstance(binding.get("address"), dict):
            raise LookupError_(400, "USAGE_INVALID", "request requires binding.address")
        address = unit_address(binding["address"])
        with self._lock:
            unit = copy.deepcopy(self._units.get(address))
            revision = self._revision
            self._calls.append({
                "origin": self.name,
                "address": address,
                "descriptorRef": binding.get("descriptorRef", ""),
                "descriptorDigest": binding.get("descriptorDigest", ""),
                "declarationDigest": binding.get("declarationDigest", ""),
                "declarationCommit": binding.get("declarationCommit", ""),
                "operation": request.get("operation", ""),
                "call": request.get("call", ""),
                "protocol": request.get("protocol", ""),
                "identity": request.get("identity", {}),
                "headers": {name: headers.get(name, "") for name in RECORDED_HEADERS if headers.get(name)},
                "at": now(),
            })
        if not headers.get("X-Resource-Principal", "").strip():
            raise LookupError_(401, "UNAUTHENTICATED", "resource-access requires X-Resource-Principal")
        if request.get("protocol") not in (None, "", PROTOCOL):
            raise LookupError_(422, "CAPABILITY_UNSATISFIED", f"unsupported protocol {request.get('protocol')!r}")
        if unit is None:
            raise LookupError_(404, "OBJECT_REF_UNRESOLVED", f"unit {address} is unknown to origin {self.name}")
        return respond(unit, self.generation, revision)


def validate_unit(address: str, unit: Any) -> None:
    if "#/" not in address:
        raise ValueError(f"unit address {address!r} must look like <object>#/<aspect>[/<member>]")
    if not isinstance(unit, dict):
        raise ValueError(f"unit {address} must be an object")
    behavior = unit.get("behavior", "ok")
    if behavior not in BEHAVIORS:
        raise ValueError(f"unit {address} behavior {behavior!r} not in {sorted(BEHAVIORS)}")
    if behavior in {"ok", "slow", "invalid"} and "value" not in unit:
        raise ValueError(f"unit {address} behavior {behavior} requires value")


def respond(unit: dict[str, Any], generation: str, revision: int) -> dict[str, Any]:
    behavior = unit.get("behavior", "ok")
    if behavior == "error":
        raise LookupError_(int(unit.get("status", 503)), str(unit.get("errorCode", "TEMPORARY_UNAVAILABLE")),
                           str(unit.get("message", "scripted source failure")))
    if behavior == "slow":
        time.sleep(int(unit.get("delayMs", 5000)) / 1000)
    basis = {
        "bindingGeneration": generation,
        "consistency": str(unit.get("consistency", "latest-only")),
        "sourceRevision": f"rev-{revision}",
        "observedAt": now(),
    }
    if behavior == "bad-basis":
        basis = {"consistency": basis["consistency"], "observedAt": basis["observedAt"]}
    value = None if behavior == "null" else copy.deepcopy(unit.get("value"))
    return {"value": value, "basis": basis}


def make_handler(origin: Origin) -> type[BaseHTTPRequestHandler]:
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:
            if self.path == "/health":
                self._json(200, {"ok": True, "role": "live-origin", "protocol": PROTOCOL,
                                 "name": origin.name, "generation": origin.generation})
            elif self.path == "/control/calls":
                self._json(200, {"calls": origin.calls()})
            elif self.path == "/control/units":
                self._json(200, origin.snapshot())
            else:
                self._json(404, {"error": {"code": "NOT_FOUND", "message": self.path}})

        def do_POST(self) -> None:
            if self.path == "/control/reset":
                origin.reset()
                self._json(200, origin.snapshot())
                return
            if self.path != "/v1/access":
                self._json(404, {"error": {"code": "NOT_FOUND", "message": self.path}})
                return
            try:
                request = self._body()
                result = origin.lookup(request, {name: self.headers.get(name, "") for name in RECORDED_HEADERS})
            except LookupError_ as error:
                self._json(error.status, {"error": {"code": error.code, "message": str(error)}})
                return
            except ValueError as error:
                self._json(400, {"error": {"code": "USAGE_INVALID", "message": str(error)}})
                return
            self._json(200, result)

        def do_PUT(self) -> None:
            address = self._unit_path()
            if address is None:
                return
            try:
                revision = origin.put_unit(address, self._body())
            except ValueError as error:
                self._json(400, {"error": {"code": "USAGE_INVALID", "message": str(error)}})
                return
            self._json(200, {"address": address, "revision": revision})

        def do_DELETE(self) -> None:
            if self.path == "/control/calls":
                origin.clear_calls()
                self._json(200, {"calls": []})
                return
            address = self._unit_path()
            if address is None:
                return
            self._json(200, {"address": address, "revision": origin.delete_unit(address)})

        def _unit_path(self) -> str | None:
            prefix = "/control/units/"
            if not self.path.startswith(prefix) or len(self.path) == len(prefix):
                self._json(404, {"error": {"code": "NOT_FOUND", "message": self.path}})
                return None
            return unquote(self.path[len(prefix):])

        def _body(self) -> dict[str, Any]:
            length = int(self.headers.get("content-length", "0"))
            try:
                value = json.loads(self.rfile.read(length) or b"{}")
            except json.JSONDecodeError as error:
                raise ValueError(f"body is not JSON: {error}") from error
            if not isinstance(value, dict):
                raise ValueError("body must be a JSON object")
            return value

        def log_message(self, _format: str, *_args: Any) -> None:
            return

        def _json(self, status: int, value: Any) -> None:
            body = json.dumps(value, separators=(",", ":"), ensure_ascii=False).encode()
            self.send_response(status)
            self.send_header("content-type", "application/json")
            self.send_header("content-length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    return Handler


def load_units(path: Path) -> dict[str, dict[str, Any]]:
    data = json.loads(path.read_text(encoding="utf-8"))
    units = data.get("units") if isinstance(data, dict) else None
    if not isinstance(units, dict):
        raise ValueError(f"{path}: expected an object with a 'units' map")
    return units


def serve(origin: Origin, host: str, port: int) -> ThreadingHTTPServer:
    return ThreadingHTTPServer((host, port), make_handler(origin))


def listen(value: str) -> tuple[str, int]:
    host, separator, raw_port = value.rpartition(":")
    if not separator or not host or not raw_port.isdigit():
        raise argparse.ArgumentTypeError("listen must be host:port")
    return host, int(raw_port)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--listen", type=listen, default=("127.0.0.1", 7400))
    parser.add_argument("--units", type=Path, default=Path(__file__).with_name("units.json"))
    parser.add_argument("--generation", default="live-origin-v1")
    parser.add_argument("--name", default="live-origin")
    args = parser.parse_args()
    origin = Origin(load_units(args.units), args.generation, args.name)
    serve(origin, *args.listen).serve_forever()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
