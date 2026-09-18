#!/usr/bin/env python3
"""Collector: reconcile source catalog and emit Writer updates to table-meta."""

from __future__ import annotations

import argparse
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import subprocess
import sys
import tempfile
import threading
import time
from typing import Any

import shop
import source

REPOSITORY = os.environ.get("KC_REPOSITORY", "kr://scene/knowledge")
PLATFORM_REF = "platform/mysql"


def captured_at() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def json_source_units(current: dict[str, Any]) -> list[dict[str, Any]]:
    units = []
    for object_id, value in (current.get("entities") or {}).items():
        units.append({
            "address": {"kind": "Aspect", "objectId": object_id, "aspectName": "properties"},
            "value": {"name": object_id.rsplit(":", 1)[-1]},
            "sourceKey": f"scene:{object_id}",
        })
    return units


def canonical_file(object_id: str, aspect: str, schema_ref: str, value: Any, kind: str = "Aspect") -> str:
    body = json.dumps(value, indent=2, ensure_ascii=False)
    header = [
        "---",
        f"object_id: {object_id}",
    ]
    if aspect:
        header.append(f"aspect_name: {aspect}")
    header.extend([
        f"kind: {kind}",
        f"schema_ref: {schema_ref}",
        "---",
        body,
        "",
    ])
    return "\n".join(header)


def catalog_units(catalog: shop.Catalog) -> list[dict[str, Any]]:
    db = shop.database()
    units: list[dict[str, Any]] = []
    for table in catalog.tables:
        units.append({
            "object_id": table.object_id,
            "aspect": "properties",
            "schema_ref": "schema/table.properties",
            "path": f"tables/{table.name}.properties.yaml",
            "value": {
                "entityType": "Table",
                "name": table.name,
                "qualifiedName": f"{db}.{table.name}",
                "nativeId": f"{db}.{table.name}",
                "nativeKind": "TABLE",
                "nativeType": "BASE TABLE",
                "platformRef": PLATFORM_REF,
            },
        })
        units.append({
            "object_id": table.object_id,
            "aspect": "schema",
            "schema_ref": "schema/table.schema",
            "path": f"tables/{table.name}.schema.yaml",
            "value": {
                "columnCount": len(table.columns),
                "primaryKeyColumnRefs": [
                    f"column/{db}.{table.name}.{column}" for column in table.primary_keys
                ],
            },
        })
        for column in table.columns:
            object_id = f"column/{db}.{table.name}.{column.name}"
            units.append({
                "object_id": object_id,
                "aspect": "properties",
                "schema_ref": "schema/column.properties",
                "path": f"columns/{table.name}.{column.name}.properties.yaml",
                "value": {
                    "entityType": "Column",
                    "name": column.name,
                    "qualifiedName": f"{db}.{table.name}.{column.name}",
                    "nativeId": f"{db}.{table.name}.{column.name}",
                    "nativeKind": "COLUMN",
                    "nativeType": column.data_type,
                    "platformRef": PLATFORM_REF,
                },
            })
    for job in catalog.jobs:
        units.append({
            "object_id": job.object_id,
            "aspect": "properties",
            "schema_ref": "schema/data-job.properties",
            "path": f"data-jobs/{job.name}.properties.yaml",
            "value": {
                "entityType": "DataJob",
                "name": job.name,
                "qualifiedName": f"{db}.{job.name}",
                "nativeId": f"{db}.{job.name}",
                "nativeKind": "EVENT",
                "nativeType": "MYSQL_EVENT",
                "platformRef": PLATFORM_REF,
            },
        })
        units.append({
            "object_id": job.object_id,
            "aspect": "definition",
            "schema_ref": "schema/data-job.definition",
            "path": f"data-jobs/{job.name}.definition.yaml",
            "value": {
                "language": "SQL",
                "sourceCode": job.source_code,
                "schedule": {
                    "type": "RECURRING",
                    "intervalValue": job.interval_value,
                    "intervalField": job.interval_field,
                },
                "enabled": job.enabled,
                "description": job.description,
            },
        })
    return units


def desired_units() -> tuple[list[dict[str, Any]], str]:
    if shop.mysql_configured():
        catalog = shop.load_catalog()
        units = catalog_units(catalog)
        return units, catalog.fingerprint()
    current = source.load()
    return json_source_units(current), str(current.get("revision", ""))


def write_enabled() -> bool:
    return os.environ.get("KC_COLLECTOR_WRITE", "1").strip().lower() not in {"0", "false", "no"}


def kc_command(*args: str) -> subprocess.CompletedProcess[str]:
    server = os.environ.get("KC_SERVER_URL", "").strip()
    command = ["kc"]
    if server:
        command.extend(["--server", server])
    command.extend(args)
    return subprocess.run(command, check=False, capture_output=True, text=True)


def schema_ready() -> bool:
    if not shop.mysql_configured():
        return True
    completed = kc_command("schema", "list", "--repo", REPOSITORY)
    if completed.returncode != 0:
        return False
    try:
        payload = json.loads(completed.stdout or "{}")
    except json.JSONDecodeError:
        return False
    items = payload.get("items") or payload.get("schemas") or payload
    if isinstance(items, dict):
        items = items.get("items") or []
    if not isinstance(items, list):
        return False
    for item in items:
        object_id = ""
        if isinstance(item, dict):
            object_id = str(item.get("objectId") or item.get("id") or "")
        elif isinstance(item, str):
            object_id = item
        if object_id == "schema/table.properties":
            return True
    return "schema/table.properties" in (completed.stdout or "")


def write_catalog(units: list[dict[str, Any]], revision: str) -> None:
    if not write_enabled():
        return
    server = os.environ.get("KC_SERVER_URL", "").strip()
    principal = os.environ.get("KC_AS", "").strip()
    if not server or not principal:
        return
    if shop.mysql_configured() and not schema_ready():
        return
    if shop.mysql_configured():
        with tempfile.TemporaryDirectory(prefix="kc-collector-") as directory:
            for unit in units:
                path = os.path.join(directory, unit["path"])
                os.makedirs(os.path.dirname(path), exist_ok=True)
                with open(path, "w", encoding="utf-8") as handle:
                    handle.write(canonical_file(
                        unit["object_id"], unit["aspect"], unit["schema_ref"], unit["value"]
                    ))
            completed = kc_command(
                "writer", "commit",
                "--command-id", f"scene-collector-typedir-{revision[:16]}",
                "--repo", REPOSITORY,
                "--dir", directory,
            )
            if completed.returncode != 0:
                raise subprocess.CalledProcessError(
                    completed.returncode, completed.args, completed.stdout, completed.stderr
                )
        return
    for index, item in enumerate(units):
        address = item["address"]
        completed = kc_command(
            "writer", "put",
            "--command-id", f"scene-collector-{index}-{revision}",
            "--repo", REPOSITORY,
            "--object", address["objectId"],
            "--aspect", address["aspectName"],
            "--value", json.dumps(item["value"], separators=(",", ":"), ensure_ascii=False),
        )
        if completed.returncode != 0:
            raise subprocess.CalledProcessError(
                completed.returncode, completed.args, completed.stdout, completed.stderr
            )


def reconcile(request: dict[str, Any]) -> dict[str, Any]:
    units, revision = desired_units()
    now = captured_at()
    signal = request.get("signal") or {"kind": "manual"}
    desired = []
    for unit in units:
        if "address" in unit:
            desired.append(unit)
            continue
        desired.append({
            "address": {"kind": "Aspect", "objectId": unit["object_id"], "aspectName": unit["aspect"]},
            "value": unit["value"],
            "sourceKey": f"shop:{unit['object_id']}#{unit['aspect']}",
        })
    observation = {
        "observation": {
            "sourceRefs": ["scene://accessor"],
            "observedAt": now,
            "representation": "STATE",
            "coverage": {"kind": "FULL"},
            "trigger": signal,
        },
        "mode": "reconcile",
        "desired": [{"address": item["address"], "sourceKey": item.get("sourceKey")} for item in desired],
        "observed": request.get("checkpoint", {}).get("observed") or [],
        "nextCheckpoint": {
            "version": 1,
            "revision": revision,
            "observed": [{"sourceKey": item.get("sourceKey")} for item in desired],
            "capturedAt": now,
        },
        "message": "reconcile scene accessor source",
    }
    write_catalog(units, revision)
    return observation


def watch(interval: float) -> None:
    last = ""
    while True:
        try:
            _, revision = desired_units()
            if revision != last:
                if shop.mysql_configured() and not schema_ready():
                    time.sleep(interval)
                    continue
                reconcile({"signal": {"kind": "watch"}})
                last = revision
        except (shop.ShopError, subprocess.CalledProcessError, ValueError, json.JSONDecodeError):
            pass
        time.sleep(interval)


class Handler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        if self.path != "/health":
            self.send_error(404)
            return
        self._json(200, {"ok": True, "role": "collector"})

    def do_POST(self) -> None:
        if self.path != "/v1/reconcile":
            self.send_error(404)
            return
        try:
            length = int(self.headers.get("content-length", "0"))
            raw = self.rfile.read(length) if length else b"{}"
            request = json.loads(raw or b"{}")
            if not isinstance(request, dict):
                raise ValueError("request must be an object")
            self._json(200, reconcile(request))
        except (ValueError, json.JSONDecodeError, subprocess.CalledProcessError, shop.ShopError) as error:
            message = str(error)
            if isinstance(error, subprocess.CalledProcessError):
                message = (error.stderr or error.stdout or str(error)).strip()
            self._json(400, {"error": {"code": "USAGE_INVALID", "message": message}})

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
    parser.add_argument("--listen", type=listen)
    parser.add_argument("--interval", type=float, default=2.0)
    args = parser.parse_args()
    if args.listen:
        if os.environ.get("KC_SERVER_URL", "").strip() and write_enabled():
            thread = threading.Thread(target=watch, args=(args.interval,), daemon=True)
            thread.start()
        ThreadingHTTPServer(args.listen, Handler).serve_forever()
        return 0
    request = json.load(sys.stdin) if not sys.stdin.isatty() else {}
    if not isinstance(request, dict):
        print("collector: request must be an object", file=sys.stderr)
        return 1
    json.dump(reconcile(request), sys.stdout, separators=(",", ":"), ensure_ascii=False)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
