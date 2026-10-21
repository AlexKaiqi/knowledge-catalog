#!/usr/bin/env python3
"""Self-test for the live origin. Run: python3 test_origin.py"""

from __future__ import annotations

import http.client
import json
from pathlib import Path
import threading
import unittest
import urllib.error
import urllib.request
from urllib.parse import quote

import origin as live

HERE = Path(__file__).parent


def unit_path(address: str) -> str:
    return "/control/units/" + quote(address, safe="/")


class LiveOriginTest(unittest.TestCase):
    def setUp(self) -> None:
        self.origin = live.Origin(live.load_units(HERE / "units.json"), "gen-test", "live-origin")
        self.server = live.serve(self.origin, "127.0.0.1", 0)
        self.base = f"http://127.0.0.1:{self.server.server_address[1]}"
        threading.Thread(target=self.server.serve_forever, daemon=True).start()

    def tearDown(self) -> None:
        self.server.shutdown()
        self.server.server_close()

    def call(self, method: str, path: str, body: dict | None = None, headers: dict | None = None):
        data = None if body is None else json.dumps(body).encode()
        request = urllib.request.Request(self.base + path, data=data, method=method, headers=headers or {})
        try:
            with urllib.request.urlopen(request, timeout=5) as response:
                return response.status, json.loads(response.read())
        except urllib.error.HTTPError as error:
            with error:
                return error.code, json.loads(error.read())

    def lookup(self, object_id: str, aspect: str, member: str = "", principal: str = "agent:reader"):
        address = {"kind": "member" if member else "aspect", "objectId": object_id, "aspectName": aspect}
        if member:
            address["memberKey"] = member
        body = {"binding": {"address": address, "descriptorRef": "ops/warehouse-stats",
                            "declarationDigest": "sha256:decl"},
                "protocol": live.PROTOCOL, "operation": "lookup", "call": "lookup",
                "identity": {"principal": principal}}
        headers = {"X-Resource-Principal": principal} if principal else {}
        return self.call("POST", "/v1/access", body, headers)

    def test_member_value_with_basis(self) -> None:
        status, body = self.lookup("table/shop.orders", "runtime", "row-count")
        self.assertEqual(status, 200)
        self.assertEqual(body["value"]["value"], 6)
        self.assertEqual(body["basis"]["bindingGeneration"], "gen-test")
        self.assertEqual(body["basis"]["sourceRevision"], "rev-1")
        self.assertTrue(body["basis"]["observedAt"])

    def test_aspect_value(self) -> None:
        status, body = self.lookup("table/shop.customers", "live-stats")
        self.assertEqual(status, 200)
        self.assertEqual(body["value"], {"table": "customers", "rowCount": 4})

    def test_scripted_failures_are_per_unit(self) -> None:
        unit = "table/shop.orders#/runtime/freshness-lag"
        status, _ = self.call("PUT", unit_path(unit), {"behavior": "error", "errorCode": "TEMPORARY_UNAVAILABLE"})
        self.assertEqual(status, 200)
        status, body = self.lookup("table/shop.orders", "runtime", "freshness-lag")
        self.assertEqual((status, body["error"]["code"]), (503, "TEMPORARY_UNAVAILABLE"))
        status, body = self.lookup("table/shop.orders", "runtime", "row-count")
        self.assertEqual((status, body["basis"]["sourceRevision"]), (200, "rev-2"))

    def test_null_invalid_and_bad_basis(self) -> None:
        unit = "table/shop.orders#/runtime/row-count"
        self.call("PUT", unit_path(unit), {"behavior": "null"})
        self.assertIsNone(self.lookup("table/shop.orders", "runtime", "row-count")[1]["value"])
        self.call("PUT", unit_path(unit), {"behavior": "invalid", "value": {"value": "six"}})
        self.assertEqual(self.lookup("table/shop.orders", "runtime", "row-count")[1]["value"], {"value": "six"})
        self.call("PUT", unit_path(unit), {"behavior": "bad-basis", "value": {"value": 6}})
        self.assertNotIn("bindingGeneration", self.lookup("table/shop.orders", "runtime", "row-count")[1]["basis"])

    def test_unknown_unit_and_missing_principal(self) -> None:
        status, body = self.lookup("table/shop.orders", "runtime", "nope")
        self.assertEqual((status, body["error"]["code"]), (404, "OBJECT_REF_UNRESOLVED"))
        status, body = self.lookup("table/shop.orders", "runtime", "row-count", principal="")
        self.assertEqual((status, body["error"]["code"]), (401, "UNAUTHENTICATED"))

    def test_calls_record_identity_and_declaration(self) -> None:
        self.lookup("table/shop.orders", "runtime", "row-count", principal="alice")
        calls = self.call("GET", "/control/calls")[1]["calls"]
        self.assertEqual(len(calls), 1)
        self.assertEqual(calls[0]["address"], "table/shop.orders#/runtime/row-count")
        self.assertEqual(calls[0]["descriptorRef"], "ops/warehouse-stats")
        self.assertEqual(calls[0]["declarationDigest"], "sha256:decl")
        self.assertEqual(calls[0]["headers"]["X-Resource-Principal"], "alice")
        self.call("DELETE", "/control/calls")
        self.assertEqual(self.call("GET", "/control/calls")[1]["calls"], [])

    def test_deny_principals_rejects_only_the_listed_caller(self) -> None:
        unit = "table/shop.orders#/runtime/freshness-lag"
        self.call("PUT", unit_path(unit), {"value": {"value": 120}, "denyPrincipals": ["reader-b"]})
        status, body = self.lookup("table/shop.orders", "runtime", "freshness-lag", principal="reader-b")
        self.assertEqual((status, body["error"]["code"]), (403, "FORBIDDEN"))
        status, body = self.lookup("table/shop.orders", "runtime", "freshness-lag", principal="reader-a")
        self.assertEqual((status, body["value"]), (200, {"value": 120}))
        status, _ = self.call("PUT", unit_path(unit), {"value": {}, "denyPrincipals": "reader-b"})
        self.assertEqual(status, 400)

    def test_unreachable_unit_drops_the_connection(self) -> None:
        unit = "table/shop.orders#/runtime/row-count"
        self.call("PUT", unit_path(unit), {"behavior": "unreachable"})
        with self.assertRaises((urllib.error.URLError, ConnectionError, http.client.HTTPException)):
            self.lookup("table/shop.orders", "runtime", "row-count")
        self.assertEqual(self.lookup("table/shop.orders", "runtime", "freshness-lag")[0], 200)
        calls = self.call("GET", "/control/calls")[1]["calls"]
        self.assertEqual([call["address"] for call in calls], [unit, "table/shop.orders#/runtime/freshness-lag"])

    def test_reset_restores_initial_units(self) -> None:
        self.call("DELETE", unit_path("table/shop.orders#/runtime/row-count"))
        self.assertEqual(self.lookup("table/shop.orders", "runtime", "row-count")[0], 404)
        self.call("POST", "/control/reset", {})
        status, body = self.lookup("table/shop.orders", "runtime", "row-count")
        self.assertEqual((status, body["basis"]["sourceRevision"]), (200, "rev-1"))

    def test_rejects_malformed_unit_script(self) -> None:
        status, body = self.call("PUT", unit_path("table/x#/a"), {"behavior": "explode"})
        self.assertEqual((status, body["error"]["code"]), (400, "USAGE_INVALID"))

    def test_slow_unit_still_answers_after_delay(self) -> None:
        unit = "table/shop.orders#/runtime/row-count"
        self.call("PUT", unit_path(unit), {"behavior": "slow", "delayMs": 50, "value": {"value": 7}})
        status, body = self.lookup("table/shop.orders", "runtime", "row-count")
        self.assertEqual((status, body["value"]), (200, {"value": 7}))


if __name__ == "__main__":
    unittest.main()
