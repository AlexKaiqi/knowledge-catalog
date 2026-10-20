"""Isolated resource-access/v1 teaching source; no production authentication."""

import argparse
import json
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


class Source(BaseHTTPRequestHandler):
    def log_message(self, *_args):
        pass

    def reply(self, status, payload):
        raw = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_POST(self):
        if self.path != "/v1/access":
            self.reply(404, {"error": "unknown path"})
            return
        try:
            size = int(self.headers.get("Content-Length", "0"))
            if not 0 < size <= 65536:
                raise ValueError("invalid request length")
            request = json.loads(self.rfile.read(size))
            address = request["binding"]["address"]
            if (request["operation"], request["call"]) != ("lookup", "lookup"):
                raise ValueError("unsupported operation")
            if address.get("aspectName") != "runtime" or address.get("memberKey") != "health":
                raise ValueError("expected runtime/health member")
            state = json.loads(self.server.state_path.read_text())
            if not isinstance(state["status"], str) or not isinstance(state["revision"], str):
                raise ValueError("status and revision must be strings")
        except (ValueError, KeyError, TypeError, OSError) as error:
            self.reply(400, {"error": str(error)})
            return
        if state.get("deny"):
            self.reply(403, {"error": "source denial demonstration"})
        elif state.get("fail"):
            self.reply(503, {"error": "source failure demonstration"})
        else:
            self.reply(200, {
                "value": {"status": state["status"]},
                "basis": {
                    "bindingGeneration": "onboarding-health-v1",
                    "consistency": "bounded",
                    "sourceRevision": state["revision"],
                    "observedAt": datetime.now(timezone.utc).isoformat(),
                },
            })


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--state", type=Path, required=True)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=7391)
    args = parser.parse_args()
    # Validate before listening; reread each request to demonstrate changing state.
    json.loads(args.state.read_text())
    server = ThreadingHTTPServer((args.host, args.port), Source)
    server.state_path = args.state
    print("http://%s:%d" % server.server_address, flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
