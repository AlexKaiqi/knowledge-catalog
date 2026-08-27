#!/usr/bin/env python3
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/health":
            self.send_error(404)
            return
        self._json(200, {"ok": True})

    def do_POST(self):
        if self.path != "/v1/access":
            self.send_error(404)
            return
        try:
            length = int(self.headers.get("content-length", "0"))
            request = json.loads(self.rfile.read(length))
        except (ValueError, json.JSONDecodeError):
            self._json(400, {"error": {"message": "invalid JSON"}})
            return
        binding = request.get("binding", {})
        identity = request.get("identity", {})
        if (
            request.get("operation") != "lookup"
            or request.get("call") != "health.lookup"
            or not identity.get("principal")
        ):
            self._json(422, {"error": {"message": "unexpected State request"}})
            return
        self._json(
            200,
            {
                "value": {"status": "healthy", "runtime": "docker"},
                "basis": {
                    "bindingGeneration": "docker-runtime-v1",
                    "consistency": "repeatable",
                    "sourceRevision": "docker-health-1",
                    "observedAt": "2026-08-27T14:00:00Z",
                },
            },
        )

    def log_message(self, _format, *_args):
        return

    def _json(self, status, value):
        body = json.dumps(value, separators=(",", ":")).encode()
        self.send_response(status)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


ThreadingHTTPServer(("0.0.0.0", 8090), Handler).serve_forever()
