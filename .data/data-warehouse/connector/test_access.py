import unittest

import access


class FakeAdapter:
    def __init__(self):
        self.sql = ""

    def query(self, sql):
        self.sql = sql
        return ["1"]

    def captured_at(self):
        return "2026-08-28T00:00:00.000000Z"


def request(**overrides):
    value = {
        "descriptor": {
            "objectId": access.DESCRIPTOR_ID,
            "repository": "kr://dw/physical",
            "commit": "commit-1",
        },
        "runtime": access.RUNTIME,
        "protocol": access.PROTOCOL,
        "operation": "query",
        "input": {"sql": "SELECT COUNT(*) FROM customer"},
    }
    value.update(overrides)
    return value


class AccessTest(unittest.TestCase):
    def test_executes_declared_read_only_sql_capability(self):
        adapter = FakeAdapter()
        result = access.execute_request(request(), principal="analyst", adapter=adapter)
        self.assertEqual(adapter.sql, "SELECT COUNT(*) FROM customer")
        self.assertEqual(result["result"], {"rows": ["1"], "rowCount": 1})
        self.assertEqual(result["basis"]["descriptor"]["commit"], "commit-1")

    def test_requires_pinned_descriptor_and_identity(self):
        with self.assertRaisesRegex(access.AccessError, "principal"):
            access.execute_request(request(), principal="", adapter=FakeAdapter())
        with self.assertRaisesRegex(access.AccessError, "pinned SQL ResourceDescriptor"):
            access.execute_request(
                request(descriptor={"objectId": "resource/other"}),
                principal="analyst",
                adapter=FakeAdapter(),
            )

    def test_rejects_undeclared_operation(self):
        with self.assertRaisesRegex(access.AccessError, "only the query operation"):
            access.execute_request(
                request(operation="execute"), principal="analyst", adapter=FakeAdapter(),
            )


if __name__ == "__main__":
    unittest.main()
