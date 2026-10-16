#!/usr/bin/env python3
"""Fixed destinations for the isolated trial; credentials stay in this process."""

from dataclasses import dataclass, field
import http.client
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import os
import socket
import threading
from urllib.parse import unquote, urlsplit


MAX_REQUEST_BYTES = 32 * 1024 * 1024
HOP_HEADERS = {
    "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
    "te", "trailer", "transfer-encoding", "upgrade", "proxy-connection",
}
IDENTITY_HEADERS = {
    "authorization", "cookie", "x-kc-as", "x-kc-on-behalf-of",
    "x-kc-onbehalfof", "x-kc-console",
}


@dataclass(frozen=True)
class GatewayConfig:
    kc_upstream_url: str
    model_base_url: str
    principal: str
    model_api_key: str = field(repr=False)
    timeout_seconds: float = 120
    max_request_bytes: int = MAX_REQUEST_BYTES

    @classmethod
    def from_env(cls):
        return cls(
            kc_upstream_url=os.environ.get("KC_ZK_UPSTREAM_URL", "http://kc-server:7380"),
            model_base_url=os.environ.get(
                "ZK_MODEL_BASE_URL", "https://ark.cn-beijing.volces.com/api/plan/v3"
            ),
            principal=os.environ["ZK_PRINCIPAL"],
            model_api_key=os.environ["ZK_MODEL_API_KEY"],
            timeout_seconds=float(os.environ.get("ZK_PROXY_TIMEOUT_SECONDS", "120")),
        )

    def validate(self):
        kc = fixed_target(self.kc_upstream_url)
        model = fixed_target(self.model_base_url)
        if kc.path not in ("", "/"):
            raise ValueError("KC upstream must have no path prefix")
        if not self.principal or any(ord(c) < 33 or ord(c) == 127 for c in self.principal):
            raise ValueError("A valid fixed principal is required")
        if not self.model_api_key or any(ord(c) < 32 or ord(c) == 127 for c in self.model_api_key):
            raise ValueError("A valid model credential is required")
        if self.timeout_seconds <= 0 or not 0 < self.max_request_bytes <= MAX_REQUEST_BYTES:
            raise ValueError("Invalid gateway limits")
        if not valid_request_target(model.path or "/"):
            raise ValueError("Invalid model upstream path")


def fixed_target(url):
    target = urlsplit(url)
    if (target.scheme not in ("http", "https") or not target.hostname
            or target.username is not None or target.password is not None
            or target.query or target.fragment):
        raise ValueError("Upstream must be one fixed HTTP origin and path")
    # Validate the port during startup, rather than on the first request.
    _ = target.port
    return target


def valid_request_target(target):
    if not target.startswith("/") or target.startswith("//"):
        return False
    if any(ord(char) < 33 or ord(char) == 127 for char in target) or "\\" in target:
        return False
    parsed = urlsplit(target)
    if parsed.scheme or parsed.netloc or parsed.fragment:
        return False
    decoded = parsed.path
    for _ in range(5):
        if (decoded.startswith("//") or "\\" in decoded
                or any(ord(char) < 32 or ord(char) == 127 for char in decoded)
                or any(segment in (".", "..") for segment in decoded.split("/"))):
            return False
        next_decoded = unquote(decoded)
        if next_decoded == decoded:
            return True
        decoded = next_decoded
    return False


def without_hop_headers(headers):
    excluded = set(HOP_HEADERS)
    for name, value in headers:
        if name.lower() == "connection":
            excluded.update(token.strip().lower() for token in value.split(","))
    return [(name, value) for name, value in headers if name.lower() not in excluded]


def redacted_chunks(response, secret):
    """Retain only a possible partial secret between reads; preserve SSE bytes."""
    pending = b""
    replacement = b"[REDACTED]"
    while True:
        chunk = response.read1(64 * 1024)
        if not chunk:
            yield pending.replace(secret, replacement)
            return
        pending += chunk
        while True:
            found = pending.find(secret)
            if found >= 0:
                yield pending[:found] + replacement
                pending = pending[found + len(secret):]
                continue
            # Do not delay ordinary SSE output: retain only a suffix that can
            # actually be the start of the credential, rather than len(key).
            keep = min(len(secret) - 1, len(pending))
            while keep and not pending.endswith(secret[:keep]):
                keep -= 1
            if keep:
                yield pending[:-keep]
                pending = pending[-keep:]
            else:
                yield pending
                pending = b""
            break


def handler_for(config, model):
    target = fixed_target(config.model_base_url if model else config.kc_upstream_url)

    class ProxyHandler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def log_message(self, *_args):
            # BaseHTTPRequestHandler logs request lines, which may contain
            # secrets in a query. The trial intentionally records no traffic.
            pass

        def setup(self):
            super().setup()
            self.connection.settimeout(config.timeout_seconds)

        def fail(self, status, message):
            payload = (message + "\n").encode()
            self.send_response(status)
            self.send_header("Content-Type", "text/plain; charset=utf-8")
            self.send_header("Content-Length", str(len(payload)))
            self.send_header("Connection", "close")
            self.end_headers()
            if self.command != "HEAD":
                self.wfile.write(payload)
            self.close_connection = True

        def request_body(self):
            if self.headers.get_all("Transfer-Encoding"):
                self.fail(400, "Chunked request bodies are not supported")
                return None
            lengths = self.headers.get_all("Content-Length", [])
            if len(lengths) > 1 or (lengths and not lengths[0].isdigit()):
                self.fail(400, "Invalid request length")
                return None
            if model and not lengths:
                self.fail(411, "Content-Length is required")
                return None
            length = int(lengths[0]) if lengths else 0
            if length > config.max_request_bytes:
                self.fail(413, "Request body is too large")
                return None
            try:
                body = self.rfile.read(length)
            except (TimeoutError, socket.timeout):
                self.fail(408, "Request body timed out")
                return None
            if len(body) != length:
                self.fail(400, "Incomplete request body")
                return None
            return body

        def relay(self):
            # parse_request normalizes a leading //; inspect the original
            # target so an absolute/network-path request cannot hide there.
            request_target = self.requestline.split()[1]
            if not valid_request_target(request_target):
                self.fail(400, "Only a normal origin-form path is allowed")
                return
            if model and (self.command != "POST" or request_target != "/chat/completions"):
                self.fail(405 if self.command != "POST" else 404, "Model route is not allowed")
                return
            body = self.request_body()
            if body is None:
                return
            headers = {}
            for name, value in without_hop_headers(list(self.headers.items())):
                if name.lower() in IDENTITY_HEADERS | {"host", "content-length", "expect"}:
                    continue
                if any(ord(c) < 32 and c != "\t" or ord(c) == 127 for c in value):
                    self.fail(400, "Invalid request header")
                    return
                headers[name] = value
            headers["Content-Length"] = str(len(body))
            headers["Connection"] = "close"
            if model:
                headers["Authorization"] = "Bearer " + config.model_api_key
                path = target.path.rstrip("/") + "/chat/completions"
            else:
                headers["X-Kc-As"] = config.principal
                path = request_target
            connection_type = (http.client.HTTPSConnection if target.scheme == "https"
                               else http.client.HTTPConnection)
            connection = connection_type(target.hostname, target.port, timeout=config.timeout_seconds)
            sent_headers = False
            try:
                connection.request(self.command, path, body=body, headers=headers)
                response = connection.getresponse()
                if model and 300 <= response.status < 400:
                    self.fail(502, "Model upstream redirect was refused")
                    return
                self.send_response(response.status)
                for name, value in without_hop_headers(response.getheaders()):
                    lowered = name.lower()
                    if lowered in {"server", "date"}:
                        continue
                    if lowered == "content-length" and (model or response.chunked):
                        continue
                    if model:
                        value = value.replace(config.model_api_key, "[REDACTED]")
                    self.send_header(name, value)
                self.send_header("Connection", "close")
                self.end_headers()
                self.close_connection = True
                sent_headers = True
                if self.command != "HEAD":
                    chunks = (redacted_chunks(response, config.model_api_key.encode()) if model
                              else iter(lambda: response.read1(64 * 1024), b""))
                    for chunk in chunks:
                        if chunk:
                            self.wfile.write(chunk)
                            self.wfile.flush()
            except (TimeoutError, socket.timeout):
                if not sent_headers:
                    self.fail(504, "Upstream timed out")
            except (OSError, http.client.HTTPException, ValueError):
                if not sent_headers:
                    self.fail(502, "Upstream is unavailable")
            finally:
                self.close_connection = True
                connection.close()

        do_GET = relay
        do_HEAD = relay
        do_POST = relay
        do_PUT = relay
        do_PATCH = relay
        do_DELETE = relay
        do_OPTIONS = relay

        def do_CONNECT(self):
            self.fail(405, "CONNECT is not allowed")

    return ProxyHandler


def create_servers(config, host="0.0.0.0", kc_port=7380, model_port=8080):
    config.validate()
    kc = ThreadingHTTPServer((host, kc_port), handler_for(config, model=False))
    try:
        model = ThreadingHTTPServer((host, model_port), handler_for(config, model=True))
    except Exception:
        kc.server_close()
        raise
    kc.daemon_threads = model.daemon_threads = True
    return kc, model


def main():
    try:
        servers = create_servers(GatewayConfig.from_env())
    except (KeyError, ValueError, OSError):
        raise SystemExit("Gateway configuration or listen address is invalid") from None
    worker = threading.Thread(target=servers[0].serve_forever, daemon=True)
    worker.start()
    try:
        servers[1].serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        servers[0].shutdown()
        for server in servers:
            server.server_close()


if __name__ == "__main__":
    main()
