#!/usr/bin/env python3
"""Observer: watch Bound State and send change notice only."""

from __future__ import annotations

import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import threading
import time
import urllib.error
import urllib.request
from typing import Any

import shop
import source

REPOSITORY = os.environ.get("KC_NOTICE_REPOSITORY", os.environ.get("KC_REPOSITORY", "kr://scene/knowledge"))
SERVER = os.environ.get("KC_SERVER_URL", "").rstrip("/")
PRINCIPAL = os.environ.get("KC_AS", os.environ.get("KC_NOTICE_PRINCIPAL", "agent:observer"))
ASPECT = os.environ.get("KC_NOTICE_ASPECT", "stats")


def notice_for(object_id: str, revision: str) -> dict[str, Any]:
    return {
        "repository": REPOSITORY,
        "address": {"kind": "Aspect", "objectId": object_id, "aspectName": ASPECT},
        "sourceRevision": revision,
    }


def watched_entities() -> tuple[list[str], str]:
    if shop.mysql_configured():
        tables = shop.list_tables()
        return [shop.table_object_id(name) for name in tables], shop.stats_fingerprint()
    current = source.load()
    return list((current.get("entities") or {}).keys()), str(current.get("revision", ""))


def send(notice: dict[str, Any]) -> dict[str, Any]:
    if not SERVER:
        return {"skipped": True, "notice": notice}
    raw = json.dumps(notice, separators=(",", ":"), ensure_ascii=False).encode()
    request = urllib.request.Request(
        SERVER + "/operations/v1/projections:notice",
        data=raw,
        method="POST",
        headers={
            "Content-Type": "application/json",
            "Accept": "application/json",
            "X-Kc-As": PRINCIPAL,
        },
    )
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            body = response.read()
    except urllib.error.HTTPError as error:
        raise RuntimeError(error.read().decode() or str(error)) from error
    return json.loads(body) if body else {"ok": True}


def emit() -> list[dict[str, Any]]:
    object_ids, revision = watched_entities()
    return [send(notice_for(object_id, revision)) for object_id in object_ids]


class Handler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        if self.path != "/health":
            self.send_error(404)
            return
        self._json(200, {"ok": True, "role": "observer"})

    def do_POST(self) -> None:
        if self.path != "/v1/notice":
            self.send_error(404)
            return
        try:
            self._json(200, {"notices": emit()})
        except (ValueError, RuntimeError, json.JSONDecodeError, shop.ShopError) as error:
            self._json(400, {"error": {"code": "USAGE_INVALID", "message": str(error)}})

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
    parser.add_argument("--watch", action="store_true")
    parser.add_argument("--interval", type=float, default=2.0)
    args = parser.parse_args()
    if args.listen:
        if SERVER:
            thread = threading.Thread(target=watch, args=(args.interval,), daemon=True)
            thread.start()
        ThreadingHTTPServer(args.listen, Handler).serve_forever()
        return 0
    watch(args.interval, once=not args.watch)
    return 0


def watch(interval: float, once: bool = False) -> None:
    last = ""
    while True:
        try:
            _, revision = watched_entities()
        except shop.ShopError:
            if once:
                return
            time.sleep(interval)
            continue
        if revision != last:
            try:
                emit()
                last = revision
            except (RuntimeError, json.JSONDecodeError, shop.ShopError):
                pass
        if once:
            return
        time.sleep(interval)


if __name__ == "__main__":
    raise SystemExit(main())
