import json
import subprocess
import sys
import tempfile
import unittest
from copy import deepcopy
from pathlib import Path


SCRIPT = Path(__file__).with_name("governed_compare.py")


def query(query_id, rows, columns, *, enterprise="ent-a", edge="edge-a", catalog="cat-a", freshness="fresh-a", truncated=False):
    return {
        "contract_version": "edge-governed-query-v1",
        "enterprise_id": enterprise,
        "edge_node_id": edge,
        "catalog": {"version": catalog, "freshness_token": freshness, "generated_at": "2026-09-12T00:00:00Z"},
        "query": {
            "id": query_id,
            "sql": f"SELECT * FROM source_{query_id}",
            "executed_at": "2026-09-12T00:00:01Z",
            "duration_ms": 2,
            "rows_returned": len(rows),
            "applied_limit": 1000,
            "truncated": truncated,
        },
        "columns": [{"name": name, "type": type_name} for name, type_name in columns],
        "rows": rows,
        "limits": {
            "coverage": {
                f"source_{query_id}": {
                    "status": "ready",
                    "row_count": len(rows),
                    "watermark": {"period": query_id},
                    "grain": "explicit synthetic key",
                }
            },
            "read_consistency": "live query; Catalog freshness is provenance, not a database snapshot",
            "quality_flags": [],
            "assumption_notes": [f"unit variant for {query_id}"],
            "missing_dependencies": [],
        },
        "evidence": {
            "receipt_sha256": "sha256:" + query_id * 8,
            "governed_objects": [f"source_{query_id}"],
        },
    }


class GovernedCompareCLITest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)

    def tearDown(self):
        self.temp.cleanup()

    def write(self, name, payload):
        path = self.root / name
        path.write_text(json.dumps(payload, ensure_ascii=False), encoding="utf-8")
        return path

    def run_cli(self, *args):
        return subprocess.run(
            [sys.executable, str(SCRIPT), *map(str, args)],
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )

    def test_period_preserves_large_decimals_unknown_zero_negative_and_unequal_keys(self):
        columns = [("维度编码", "String"), ("含税金额", "Decimal(76,9)")]
        baseline = self.write(
            "baseline.json",
            query(
                "base",
                [
                    {"维度编码": "同键大数", "含税金额": "1234567890123456789012345678901234567890.123456789"},
                    {"维度编码": "指数跨度", "含税金额": "1E+100"},
                    {"维度编码": "零基线", "含税金额": "0"},
                    {"维度编码": "负基线", "含税金额": "-10"},
                    {"维度编码": "基期缺值", "含税金额": None},
                    {"维度编码": "仅基期", "含税金额": "7"},
                ],
                columns,
            ),
        )
        current = self.write(
            "current.json",
            query(
                "curr",
                [
                    {"维度编码": "同键大数", "含税金额": "1234567890123456789012345678901234567891.000000001"},
                    {"维度编码": "指数跨度", "含税金额": "1E+100"},
                    {"维度编码": "零基线", "含税金额": "5"},
                    {"维度编码": "负基线", "含税金额": "-5"},
                    {"维度编码": "基期缺值", "含税金额": "1"},
                    {"维度编码": "仅本期", "含税金额": "7"},
                ],
                columns,
                catalog="cat-b",
                freshness="fresh-b",
                truncated=True,
            ),
        )
        output = self.root / "period-output.json"
        completed = self.run_cli(
            "period",
            "--baseline", baseline,
            "--current", current,
            "--key", "维度编码",
            "--value", "含税金额",
            "--unit", "含税金额=元",
            "--baseline-period", "2025-08",
            "--current-period", "2026-08",
            "--output", output,
        )
        self.assertEqual(completed.returncode, 0, completed.stderr)
        compact = json.loads(completed.stdout)
        artifact = json.loads(output.read_text(encoding="utf-8"))

        self.assertEqual([item["query_id"] for item in artifact["inputs"]], ["base", "curr"])
        self.assertEqual([item["query_id"] for item in compact["inputs"]], ["base", "curr"])
        self.assertEqual(artifact["coverage"], {"both": 5, "left_only": 1, "right_only": 1, "total_keys": 7})
        self.assertEqual(artifact["scope"]["status"], "partial_due_to_truncated_input")
        self.assertFalse(artifact["scope"]["business_universe_complete"])
        self.assertFalse(artifact["provenance_comparison"]["same_catalog_version"])
        self.assertEqual(artifact["spec"]["declared_period_labels"], {"baseline": "2025-08", "current": "2026-08"})
        self.assertIn("caller-declared", artifact["spec"]["period_label_disclosure"])
        self.assertEqual(artifact["inputs"][0]["limits"]["read_consistency"], baseline_json := query("base", [], columns)["limits"]["read_consistency"])
        self.assertIn("not measured query population", artifact["scope"]["limits_coverage_semantics"])

        by_key = {row["key"]["维度编码"]: row for row in artifact["rows"]}
        large = by_key["同键大数"]["values"]["含税金额"]
        self.assertEqual(large["difference"], "0.876543212")
        self.assertEqual(by_key["指数跨度"]["values"]["含税金额"]["difference"], "0")
        self.assertEqual(large["unit"], "元")
        self.assertEqual(large["relative_change"]["unit"], "ratio")
        self.assertEqual(large["relative_change"]["precision_significant_digits"], 38)
        self.assertIsNone(by_key["零基线"]["values"]["含税金额"]["relative_change"]["value"])
        self.assertEqual(by_key["零基线"]["values"]["含税金额"]["relative_change"]["unknown_reason"], "zero_baseline")
        self.assertEqual(by_key["负基线"]["values"]["含税金额"]["difference"], "5")
        self.assertEqual(by_key["负基线"]["values"]["含税金额"]["relative_change"]["value"], "-0.5")
        self.assertIn("negative baseline", by_key["负基线"]["values"]["含税金额"]["relative_change"]["caveat"])
        self.assertIsNone(by_key["基期缺值"]["values"]["含税金额"]["difference"])
        self.assertEqual(by_key["仅基期"]["presence"], {"baseline": True, "current": False})
        self.assertEqual(by_key["仅本期"]["presence"], {"baseline": False, "current": True})
        summary = artifact["value_summaries"]["含税金额"]
        self.assertEqual(summary["baseline"]["unknown_count"], 1)
        self.assertEqual(summary["baseline"]["missing_key_count"], 1)
        self.assertEqual(summary["current"]["missing_key_count"], 1)
        self.assertEqual(summary["paired"]["known_count"], 4)
        self.assertEqual(summary["paired"]["unknown_count"], 1)
        self.assertEqual(summary["paired"]["known_difference_sum"], "10.876543212")
        self.assertEqual(
            summary["baseline"]["known_value_sum"],
            "10000000000000000000000000000000000000000000000000000000000001234567890123456789012345678901234567887.123456789",
        )

    def test_reconcile_reports_both_sides_and_null_separately_without_ratio(self):
        keys = [("门店键", "UInt64"), ("商品键", "String")]
        left = self.write(
            "库存.json",
            query(
                "库存",
                [
                    {"门店键": 9007199254740993, "商品键": "A", "库存数量": "10.500"},
                    {"门店键": 9007199254740993, "商品键": "B", "库存数量": None},
                ],
                keys + [("库存数量", "Decimal(38,3)")],
            ),
        )
        right = self.write(
            "销售.json",
            query(
                "销售",
                [
                    {"门店键": 9007199254740993, "商品键": "A", "售出重量": "2.500"},
                    {"门店键": 9007199254740993, "商品键": "C", "售出重量": "3.000"},
                ],
                keys + [("售出重量", "Decimal(38,3)")],
            ),
        )
        completed = self.run_cli(
            "reconcile",
            "--left", left,
            "--right", right,
            "--key", "门店键",
            "--key", "商品键",
            "--left-value", "库存数量",
            "--right-value", "售出重量",
            "--unit", "库存数量=公斤",
            "--unit", "售出重量=公斤",
        )
        self.assertEqual(completed.returncode, 0, completed.stderr)
        artifact = json.loads(completed.stdout)
        self.assertEqual(artifact["coverage"], {"both": 1, "left_only": 1, "right_only": 1, "total_keys": 3})
        self.assertEqual(artifact["set_summaries"]["matched"]["left"]["库存数量"]["known_value_sum"], "10.500")
        self.assertEqual(artifact["set_summaries"]["left_only"]["left"]["库存数量"]["unknown_count"], 1)
        self.assertEqual(artifact["set_summaries"]["right_only"]["right"]["售出重量"]["known_value_sum"], "3.000")
        by_product = {row["key"]["商品键"]: row for row in artifact["rows"]}
        self.assertEqual(by_product["B"]["left_values"]["库存数量"]["value"], None)
        self.assertEqual(by_product["B"]["presence"], {"left": True, "right": False})
        self.assertFalse(any("difference" in row for row in artifact["rows"]))
        self.assertFalse(any("relative_change" in row for row in artifact["rows"]))

    def test_same_amount_on_distinct_keys_remains_distinct_and_duplicate_keys_fail(self):
        columns = [("entity_code", "String"), ("metric_value", "Decimal(30,2)")]
        baseline = self.write(
            "variant-a.json",
            query("a", [{"entity_code": "A", "metric_value": "9.00"}, {"entity_code": "B", "metric_value": "9.00"}], columns),
        )
        current = self.write(
            "variant-b.json",
            query("b", [{"entity_code": "A", "metric_value": "10.00"}, {"entity_code": "B", "metric_value": "8.00"}], columns),
        )
        period_args = (
            "period", "--baseline", baseline, "--current", current,
            "--key", "entity_code", "--value", "metric_value",
            "--unit", "metric_value=件", "--baseline-period", "P0", "--current-period", "P1",
        )
        completed = self.run_cli(*period_args)
        self.assertEqual(completed.returncode, 0, completed.stderr)
        self.assertEqual(len(json.loads(completed.stdout)["rows"]), 2)

        duplicate_payload = query(
            "dup",
            [{"entity_code": "A", "metric_value": "1"}, {"entity_code": "A", "metric_value": "2"}],
            columns,
        )
        duplicate = self.write("duplicate.json", duplicate_payload)
        failed = self.run_cli(
            "period", "--baseline", duplicate, "--current", current,
            "--key", "entity_code", "--value", "metric_value",
            "--unit", "metric_value=件", "--baseline-period", "P0", "--current-period", "P1",
        )
        self.assertEqual(failed.returncode, 2)
        self.assertIn("duplicate key", failed.stderr)
        self.assertIn("preaggregate", failed.stderr)

    def test_rejects_identity_mismatch_preview_malformed_numbers_and_null_keys(self):
        columns = [("branch_id", "String"), ("net_value", "Decimal(38,4)")]
        valid_payload = query("valid", [{"branch_id": "A", "net_value": "1"}], columns)
        valid = self.write("valid.json", valid_payload)

        mismatched = self.write("other-enterprise.json", query("other", [{"branch_id": "A", "net_value": "2"}], columns, enterprise="ent-b"))
        common = ("--key", "branch_id", "--value", "net_value", "--unit", "net_value=USD", "--baseline-period", "P0", "--current-period", "P1")
        failed = self.run_cli("period", "--baseline", valid, "--current", mismatched, *common)
        self.assertEqual(failed.returncode, 2)
        self.assertIn("different enterprise", failed.stderr)

        preview_payload = deepcopy(valid_payload)
        preview_payload["rows_preview_only"] = True
        preview_payload["input_file"] = "/workspace/data/exact.json"
        preview = self.write("preview.json", preview_payload)
        failed = self.run_cli("period", "--baseline", preview, "--current", valid, *common)
        self.assertEqual(failed.returncode, 2)
        self.assertIn("bounded preview", failed.stderr)

        malformed = self.write("malformed-number.json", query("badnum", [{"branch_id": "A", "net_value": "not-a-number"}], columns))
        failed = self.run_cli("period", "--baseline", malformed, "--current", valid, *common)
        self.assertEqual(failed.returncode, 2)
        self.assertIn("malformed number", failed.stderr)

        null_key = self.write("null-key.json", query("nullkey", [{"branch_id": None, "net_value": "1"}], columns))
        failed = self.run_cli("period", "--baseline", null_key, "--current", valid, *common)
        self.assertEqual(failed.returncode, 2)
        self.assertIn("keys must be non-null", failed.stderr)

        for index, key in enumerate(["", "   ", "\t"]):
            with self.subTest(blank_key=repr(key)):
                blank = self.write(f"blank-key-{index}.json", query("blankkey", [{"branch_id": key, "net_value": "1"}], columns))
                failed = self.run_cli("period", "--baseline", blank, "--current", valid, *common)
                self.assertEqual(failed.returncode, 2)
                self.assertIn("keys must be non-empty", failed.stderr)

        bool_value = self.write("bool-value.json", query("boolval", [{"branch_id": "A", "net_value": True}], columns))
        failed = self.run_cli("period", "--baseline", bool_value, "--current", valid, *common)
        self.assertEqual(failed.returncode, 2)
        self.assertIn("boolean", failed.stderr)

        malformed_json = self.root / "malformed.json"
        malformed_json.write_text("{", encoding="utf-8")
        failed = self.run_cli("period", "--baseline", malformed_json, "--current", valid, *common)
        self.assertEqual(failed.returncode, 2)
        self.assertIn("malformed JSON", failed.stderr)

        duplicate_json = self.root / "duplicate-json-key.json"
        duplicate_json.write_text(
            json.dumps(valid_payload, ensure_ascii=False).replace('"enterprise_id": "ent-a"', '"enterprise_id": "ent-a", "enterprise_id": "ent-a"'),
            encoding="utf-8",
        )
        failed = self.run_cli("period", "--baseline", duplicate_json, "--current", valid, *common)
        self.assertEqual(failed.returncode, 2)
        self.assertIn("duplicate JSON key", failed.stderr)

        nonfinite_json = self.root / "nonfinite.json"
        nonfinite_json.write_text(
            json.dumps(valid_payload, ensure_ascii=False).replace('"net_value": "1"', '"net_value": NaN'),
            encoding="utf-8",
        )
        failed = self.run_cli("period", "--baseline", nonfinite_json, "--current", valid, *common)
        self.assertEqual(failed.returncode, 2)
        self.assertIn("non-finite JSON number", failed.stderr)


if __name__ == "__main__":
    unittest.main()
