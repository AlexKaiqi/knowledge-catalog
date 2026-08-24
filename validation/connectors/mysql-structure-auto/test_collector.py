import hashlib
import json
import unittest

import collector


class CollectorTest(unittest.TestCase):
    def test_translates_stable_table_and_column_addresses(self):
        tables = [
            {
                "tableSchema": "tpch",
                "tableName": "orders",
                "tableType": "BASE TABLE",
                "engine": "InnoDB",
                "tableComment": "",
                "tableCollation": "utf8mb4_0900_ai_ci",
            }
        ]
        columns = [
            {
                "tableSchema": "tpch",
                "tableName": "orders",
                "columnName": "o_orderkey",
                "ordinalPosition": 1,
                "columnDefault": None,
                "isNullable": "NO",
                "dataType": "bigint",
                "columnType": "bigint",
                "characterMaximumLength": None,
                "numericPrecision": 19,
                "numericScale": 0,
                "columnKey": "PRI",
                "extra": "",
                "columnComment": "",
            }
        ]

        units, observed = collector.translate(tables, columns)

        self.assertEqual(len(units), 2)
        self.assertEqual(len(observed), 2)
        table_key = f"{collector.SOURCE_REF}/table/tpch/orders"
        column_key = f"{collector.SOURCE_REF}/column/tpch/orders/o_orderkey"
        self.assertEqual(
            units[0]["address"]["objectId"],
            "dw-table-" + hashlib.sha256(table_key.encode()).hexdigest()[:24],
        )
        self.assertEqual(
            units[1]["address"]["objectId"],
            "dw-column-" + hashlib.sha256(column_key.encode()).hexdigest()[:24],
        )
        self.assertEqual(units[0]["value"]["columnCount"], 1)
        self.assertEqual(
            observed[1]["digest"], collector.canonical_digest(units[1]["value"])
        )

    def test_canonical_digest_matches_compact_sorted_json(self):
        value = {"z": [2, 1], "a": {"nullable": False, "value": None}}
        encoded = json.dumps(value, sort_keys=True, separators=(",", ":")).encode()
        self.assertEqual(
            collector.canonical_digest(value), hashlib.sha256(encoded).hexdigest()
        )

    def test_rejects_visible_table_without_visible_columns(self):
        tables = [
            {"tableSchema": "tpch", "tableName": "orders"},
            {"tableSchema": "tpch", "tableName": "lineitem"},
        ]
        columns = [
            {
                "tableSchema": "tpch",
                "tableName": "orders",
                "columnName": "o_orderkey",
            }
        ]

        with self.assertRaisesRegex(
            ValueError, "visible tables have no visible columns: tpch.lineitem"
        ):
            collector.validate_snapshot(tables, columns)

    def test_rejects_duplicate_table_rows(self):
        tables = [
            {"tableSchema": "tpch", "tableName": "orders"},
            {"tableSchema": "tpch", "tableName": "orders"},
        ]
        columns = [
            {
                "tableSchema": "tpch",
                "tableName": "orders",
                "columnName": "o_orderkey",
            }
        ]

        with self.assertRaisesRegex(ValueError, "duplicate tables"):
            collector.validate_snapshot(tables, columns)


if __name__ == "__main__":
    unittest.main()
