import hashlib
import unittest

import adapter
import collector
import domain
import mapping


class CollectorTest(unittest.TestCase):
    def rows(self):
        tables = [{
            "tableSchema": "tpch",
            "tableName": "orders",
            "tableType": "BASE TABLE",
            "tableComment": "TPC-H orders",
        }]
        columns = [{
            "tableSchema": "tpch",
            "tableName": "orders",
            "columnName": "o_orderkey",
            "ordinalPosition": 1,
            "isNullable": "NO",
            "dataType": "bigint",
            "columnType": "bigint",
            "columnKey": "PRI",
        }]
        jobs = [{
            "eventSchema": "tpch",
            "eventName": "inspect_urgent_orders",
            "eventType": "RECURRING",
            "intervalValue": "1",
            "intervalField": "DAY",
            "status": "DISABLED",
            "eventDefinition": "SELECT COUNT(*) FROM orders",
            "eventComment": "fixture job",
        }]
        return tables, columns, jobs

    def test_translates_live_rows_to_current_knowledge_contract(self):
        desired, source_mapping = mapping.translate(*self.rows())
        table_key = "mysql:fixture:table:tpch.orders"
        column_key = "mysql:fixture:column:tpch.orders.o_orderkey"
        self.assertEqual(len(desired), 13)
        self.assertEqual(
            source_mapping[table_key],
            "dw-mysql-tpch-table-" + hashlib.sha256(table_key.encode()).hexdigest()[:24],
        )
        self.assertEqual(
            source_mapping[column_key],
            "dw-mysql-tpch-column-" + hashlib.sha256(column_key.encode()).hexdigest()[:24],
        )
        column = next(
            item for item in desired
            if item["address"].get("aspectName") == "properties"
            and item["address"]["objectId"] == source_mapping[column_key]
        )
        self.assertEqual(column["schemaRef"], "schema/column.properties")
        self.assertEqual(column["value"]["entityType"], "Column")
        self.assertEqual(column["value"]["ordinal"], 1)
        self.assertFalse(any("valueSource" in item for item in desired))
        jobs = [
            item for item in desired
            if item["address"].get("aspectName") == "properties"
            and item["value"].get("entityType") == "DataJob"
        ]
        self.assertEqual(len(jobs), 1)
        job_id = jobs[0]["address"]["objectId"]
        definition = next(
            item for item in desired
            if item["address"]["objectId"] == job_id
            and item["address"].get("aspectName") == "definition"
        )
        self.assertEqual(definition["value"]["language"], "SQL")
        self.assertFalse(definition["value"]["enabled"])

    def test_canonical_digest_matches_kc_stable_json_shape(self):
        value = {"z": [2, 1], "a": {"nullable": False, "value": None}}
        self.assertEqual(
            domain.canonical_digest(value),
            "a7a2bd5aef2c0a0df882402c58967a7d911aefcb12cae77da1edaac10ed92889",
        )

    def test_rejects_visible_table_without_visible_columns(self):
        tables, columns, jobs = self.rows()
        tables.append({
            "tableSchema": "tpch", "tableName": "lineitem",
            "tableType": "BASE TABLE", "tableComment": "",
        })
        with self.assertRaisesRegex(ValueError, "tpch.lineitem"):
            mapping.validate_snapshot(tables, columns, jobs)

    def test_rejects_duplicate_table_rows(self):
        tables, columns, jobs = self.rows()
        with self.assertRaisesRegex(ValueError, "duplicate tables"):
            mapping.validate_snapshot(tables + tables, columns, jobs)

    def test_rejects_duplicate_column_rows(self):
        tables, columns, jobs = self.rows()
        with self.assertRaisesRegex(ValueError, "duplicate columns"):
            mapping.validate_snapshot(tables, columns + columns, jobs)

    def test_rejects_duplicate_job_rows(self):
        tables, columns, jobs = self.rows()
        with self.assertRaisesRegex(ValueError, "duplicate jobs"):
            mapping.validate_snapshot(tables, columns, jobs + jobs)

    def test_collector_calls_adapter_then_builds_checkpoint_declarations(self):
        tables, columns, jobs = self.rows()

        class FakeAdapter:
            def list_tables(self):
                import json
                return [json.dumps(item) for item in tables]

            def describe_all_columns(self):
                import json
                return [json.dumps(item) for item in columns]

            def list_jobs(self):
                import json
                return [json.dumps(item) for item in jobs]

            def captured_at(self):
                return "2026-08-27T00:00:00.000000Z"

        desired, _, captured_at = collector.collect(FakeAdapter())
        observed = collector.observed_declaration(next(
            item for item in desired if item["address"].get("aspectName") == "schema"
        ))
        self.assertEqual(captured_at, "2026-08-27T00:00:00.000000Z")
        self.assertTrue(observed["declarationDigest"])

    def test_adapter_rejects_mutation_on_query_surface(self):
        mysql = object.__new__(adapter.MySQLAdapter)
        with self.assertRaisesRegex(ValueError, "query accepts only"):
            mysql.query("DROP TABLE orders")
        with self.assertRaisesRegex(ValueError, "query accepts only"):
            mysql.query("SELECT 1; DROP TABLE orders")

    def test_grouped_column_relation_is_bounded_and_keeps_repeated_member_roles(self):
        members = [(f"column:{index}", f"Column:{index}") for index in range(600)]
        relations = domain.grouped_relation_units(
            "mysql-tpch", "contains", "table:orders", "Table:orders", members,
        )
        self.assertGreater(len(relations), 1)
        seen = set()
        for relation in relations:
            endpoints = relation["value"]["endpoints"]
            self.assertLessEqual(len(endpoints), 256)
            self.assertEqual(endpoints[0], {"role": "container", "objectRef": {"repository": "kr://dw/physical", "object": "Table:orders"}})
            for endpoint in endpoints[1:]:
                self.assertEqual(endpoint["role"], "member")
                self.assertEqual(endpoint["objectRef"]["repository"], "kr://dw/physical")
                seen.add(endpoint["objectRef"]["object"])
        self.assertEqual(len(seen), 600)

    def test_targeted_invalidation_does_not_call_global_enumeration(self):
        tables, columns, _ = self.rows()

        class TargetedAdapter:
            def list_tables(self):
                raise AssertionError("targeted invalidation must not enumerate tables")

            def describe_all_columns(self):
                raise AssertionError("targeted invalidation must not enumerate columns")

            def list_jobs(self):
                raise AssertionError("targeted invalidation must not enumerate jobs")

            def describe_table(self, table):
                import json
                self.assert_table = table
                return [json.dumps(item) for item in tables]

            def describe_schema(self, table):
                import json
                return [json.dumps(item) for item in columns]

            def captured_at(self):
                return "2026-08-27T00:00:00.000000Z"

        key = "mysql:fixture:table:tpch.orders"
        desired, source_mapping, _ = collector.collect_targeted(TargetedAdapter(), {key})
        self.assertEqual(len(desired), 4)
        self.assertTrue(all(collector._in_table_scope(item["sourceKey"], {key}) for item in desired))
        self.assertIn(key, source_mapping)

    def test_targeted_checkpoint_preserves_unkeyed_out_of_scope_declarations(self):
        key = "mysql:fixture:table:tpch.orders"
        old_scoped = {
            "address": {"kind": "Aspect", "objectId": "old", "aspectName": "properties"},
            "digest": "old", "sourceKey": key,
        }
        unrelated_unkeyed = {
            "address": {"kind": "Aspect", "objectId": "job", "aspectName": "definition"},
            "digest": "job",
        }
        replacement = {
            "address": {"kind": "Aspect", "objectId": "new", "aspectName": "properties"},
            "value": {"name": "orders"}, "sourceKey": key,
        }
        observed, checkpoint = collector.targeted_checkpoint(
            {"version": 3, "observed": [old_scoped, unrelated_unkeyed], "sourceKeyMap": {}},
            [replacement], {key: "new"}, {key}, "2026-08-27T00:00:00Z",
        )
        self.assertEqual(observed, [old_scoped])
        self.assertIn(unrelated_unkeyed, checkpoint["observed"])
        self.assertNotIn(old_scoped, checkpoint["observed"])
        self.assertEqual(checkpoint["sourceKeyMap"][key], "new")


if __name__ == "__main__":
    unittest.main()
