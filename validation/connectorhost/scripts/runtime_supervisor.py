#!/usr/bin/env python3
"""Local acceptance supervisor: owns one exact Integration Host process."""

from __future__ import annotations

import json
import os
import signal
import subprocess
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

HOST_BIN = os.environ["INTEGRATION_HOST_BIN"]
HOST_HOME = os.environ["INTEGRATION_HOST_HOME"]
HOST_LISTEN = os.environ["INTEGRATION_HOST_LISTEN"]
SOURCE = os.environ["PAYMENT_OPS_SOURCE"]
LOG = Path(os.environ["INTEGRATION_HOST_LOG"])
SUPERVISOR_LISTEN = os.environ["INTEGRATION_SUPERVISOR_LISTEN"]
child: subprocess.Popen[bytes] | None = None
log_handle = None


def state() -> dict:
    running = child is not None and child.poll() is None
    return {
        "ok": True,
        "running": running,
        "pid": child.pid if running else None,
        "exitCode": None if child is None or running else child.returncode,
        "listen": HOST_LISTEN,
    }


def start() -> dict:
    global child, log_handle
    if child is not None and child.poll() is None:
        return state()
    LOG.parent.mkdir(parents=True, exist_ok=True)
    log_handle = LOG.open("ab", buffering=0)
    env = os.environ.copy()
    env["PAYMENT_OPS_SOURCE"] = SOURCE
    child = subprocess.Popen(
        [HOST_BIN, "--home", HOST_HOME, "serve", "--listen", HOST_LISTEN],
        env=env,
        stdin=subprocess.DEVNULL,
        stdout=log_handle,
        stderr=subprocess.STDOUT,
    )
    return state()


def stop() -> None:
    global child, log_handle
    if child is not None and child.poll() is None:
        child.terminate()
        try:
            child.wait(timeout=5)
        except subprocess.TimeoutExpired:
            child.kill()
            child.wait(timeout=5)
    if log_handle is not None:
        log_handle.close()
        log_handle = None


class Handler(BaseHTTPRequestHandler):
    def reply(self, status: int, body: dict) -> None:
        raw = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self) -> None:
        if self.path in ("/health", "/status"):
            self.reply(200, state())
        else:
            self.reply(404, {"error": "not found"})

    def do_POST(self) -> None:
        if self.path == "/start":
            self.reply(200, start())
        else:
            self.reply(404, {"error": "not found"})

    def log_message(self, _format: str, *_args: object) -> None:
        return


def shutdown(_signum: int, _frame: object) -> None:
    stop()
    raise SystemExit(0)


def main() -> int:
    signal.signal(signal.SIGTERM, shutdown)
    signal.signal(signal.SIGINT, shutdown)
    host, port = SUPERVISOR_LISTEN.rsplit(":", 1)
    server = ThreadingHTTPServer((host, int(port)), Handler)
    try:
        server.serve_forever()
    finally:
        stop()
    return 0


if __name__ == "__main__":
    sys.exit(main())
