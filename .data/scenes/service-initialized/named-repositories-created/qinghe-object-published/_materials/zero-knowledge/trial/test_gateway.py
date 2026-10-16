"""HTTP integration checks for the trial's identity and model boundary."""

import http.client
import importlib.util
import json
from pathlib import Path
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


MODULE_PATH = Path(__file__).with_name("gateway.py")
spec = importlib.util.spec_from_file_location("trial_gateway", MODULE_PATH)
gateway = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = gateway
spec.loader.exec_module(gateway)


class UpstreamHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_args):
        pass

    def do_GET(self):
        self.do_POST()

    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        self.server.seen.append((self.command, self.path, dict(self.headers), body))
        if self.path.endswith("/redirect") or body == b"redirect":
            self.send_response(302)
            self.send_header("Location", self.server.redirect_url)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        if self.path == "/denied":
            payload = b'{"error":{"code":"FORBIDDEN","message":"denied"}}'
            self.send_response(403)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)
            return
        if body == b"sse":
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Transfer-Encoding", "chunked")
            self.end_headers()
            for chunk in (b'data: {"delta":"one"}\n\n', b"data: [DONE]\n\n"):
                self.wfile.write(f"{len(chunk):X}\r\n".encode() + chunk + b"\r\n")
                self.wfile.flush()
            self.wfile.write(b"0\r\n\r\n")
            return
        if body == b"echo-secret":
            payload = ("upstream echoed " + self.server.test_key).encode()
        else:
            payload = json.dumps({"principal": self.headers.get("X-Kc-As")}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)


class GatewayHTTPTest(unittest.TestCase):
    def setUp(self):
        self.upstream = ThreadingHTTPServer(("127.0.0.1", 0), UpstreamHandler)
        self.upstream.daemon_threads = True
        self.upstream.seen = []
        self.upstream.test_key = "test-secret-known-only-to-gateway"
        self.upstream.redirect_url = "http://127.0.0.1:1/forbidden-target"
        address = f"http://127.0.0.1:{self.upstream.server_port}"
        config = gateway.GatewayConfig(
            kc_upstream_url=address,
            model_base_url=address + "/api/plan/v3",
            principal="zk-colleague",
            model_api_key=self.upstream.test_key,
            timeout_seconds=2,
            max_request_bytes=1024,
        )
        self.kc, self.model = gateway.create_servers(
            config, host="127.0.0.1", kc_port=0, model_port=0
        )
        self.servers = [self.upstream, self.kc, self.model]
        self.threads = []
        for server in self.servers:
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            self.threads.append(thread)

    def tearDown(self):
        for server in reversed(self.servers):
            server.shutdown()
            server.server_close()
        for thread in self.threads:
            thread.join(timeout=3)

    def request(self, server, method, path, body=None, headers=None):
        connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=3)
        try:
            connection.request(method, path, body=body, headers=headers or {})
            response = connection.getresponse()
            return response.status, dict(response.getheaders()), response.read()
        finally:
            connection.close()

    def test_kc_overwrites_all_client_identity_and_keeps_request_semantics(self):
        status, _headers, body = self.request(
            self.kc, "POST", "/objects?query=orders", b'{"request":"x"}',
            {"Authorization": "Bearer attacker", "Cookie": "admin=yes", "X-Kc-As": "admin",
             "X-Kc-On-Behalf-Of": "admin", "Content-Type": "application/json",
             "Connection": "x-remove", "X-Remove": "must-not-forward"},
        )
        self.assertEqual(status, 200)
        self.assertEqual(json.loads(body)["principal"], "zk-colleague")
        method, path, headers, received_body = self.upstream.seen[-1]
        headers = {key.lower(): value for key, value in headers.items()}
        self.assertEqual((method, path, received_body),
                         ("POST", "/objects?query=orders", b'{"request":"x"}'))
        self.assertEqual(headers["x-kc-as"], "zk-colleague")
        for name in ("authorization", "cookie", "x-kc-on-behalf-of", "x-remove"):
            self.assertNotIn(name, headers)

    def test_model_key_is_injected_only_at_fixed_upstream(self):
        status, _headers, body = self.request(
            self.model, "POST", "/chat/completions", b"hello",
            {"Authorization": "Bearer dummy", "Cookie": "no", "X-Kc-As": "admin"},
        )
        self.assertEqual(status, 200)
        method, path, headers, received_body = self.upstream.seen[-1]
        headers = {key.lower(): value for key, value in headers.items()}
        self.assertEqual((method, path, received_body),
                         ("POST", "/api/plan/v3/chat/completions", b"hello"))
        self.assertEqual(headers["authorization"], "Bearer " + self.upstream.test_key)
        self.assertNotIn("cookie", headers)
        self.assertNotIn("x-kc-as", headers)
        self.assertNotIn(self.upstream.test_key.encode(), body)

    def test_model_error_cannot_echo_the_private_key(self):
        status, _headers, body = self.request(self.model, "POST", "/chat/completions", b"echo-secret")
        self.assertEqual(status, 200)
        self.assertNotIn(self.upstream.test_key.encode(), body)
        self.assertIn(b"[REDACTED]", body)

    def test_model_rejects_other_methods_paths_and_queries_before_upstream(self):
        for method, path in (("GET", "/chat/completions"), ("POST", "/identity/v1/whoami"),
                             ("POST", "/chat/completions?token=not-allowed"),
                             ("POST", "http://kc-server:7380/identity/v1/whoami"),
                             ("POST", "//kc-server:7380/identity/v1/whoami")):
            with self.subTest(method=method, path=path):
                status, _headers, _body = self.request(self.model, method, path, b"x")
                self.assertGreaterEqual(status, 400)
        self.assertEqual(self.upstream.seen, [])

    def test_kc_rejects_connect_absolute_paths_and_traversal(self):
        for method, path in (("CONNECT", "kc-server:7380"), ("GET", "http://elsewhere/private"),
                             ("GET", "//elsewhere/private"), ("GET", "/a/../private"),
                             ("GET", "/a/%2e%2e/private"), ("GET", "/a\\private")):
            with self.subTest(method=method, path=path):
                status, _headers, _body = self.request(self.kc, method, path)
                self.assertGreaterEqual(status, 400)
        self.assertEqual(self.upstream.seen, [])

    def test_model_redirect_is_not_followed_or_exposed(self):
        status, headers, body = self.request(self.model, "POST", "/chat/completions", b"redirect")
        self.assertEqual(status, 502)
        self.assertNotIn("Location", headers)
        self.assertNotIn(b"forbidden-target", body)
        self.assertEqual(len(self.upstream.seen), 1)

    def test_chunked_sse_is_unchanged_and_downstream_completes(self):
        status, headers, body = self.request(self.model, "POST", "/chat/completions", b"sse")
        self.assertEqual(status, 200)
        self.assertEqual(headers["Content-Type"], "text/event-stream")
        self.assertNotIn("Transfer-Encoding", headers)
        self.assertEqual(body, b'data: {"delta":"one"}\n\ndata: [DONE]\n\n')

    def test_kc_preserves_http_error_status_and_body(self):
        status, _headers, body = self.request(self.kc, "GET", "/denied")
        self.assertEqual(status, 403)
        self.assertEqual(json.loads(body)["error"]["code"], "FORBIDDEN")

    def test_request_size_is_rejected_without_upstream_access(self):
        status, _headers, _body = self.request(self.model, "POST", "/chat/completions", b"x" * 1025)
        self.assertEqual(status, 413)
        self.assertEqual(self.upstream.seen, [])


if __name__ == "__main__":
    unittest.main()
