#!/usr/bin/env python3
"""Compare exact Edge governed-query files without retyping returned rows."""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from dataclasses import dataclass
from decimal import Decimal, InvalidOperation, localcontext
from pathlib import Path
from typing import Any, Iterable


CONTRACT_VERSION = "edge-governed-query-v1"
ARTIFACT_TYPE = "governed_comparison_v1"
RATIO_PRECISION = 38


class ComparisonError(ValueError):
    """A supplied file or comparison specification is unsafe to compare."""


@dataclass(frozen=True)
class QueryInput:
    role: str
    path: Path
    raw: bytes
    envelope: dict[str, Any]
    precise_rows: list[dict[str, Any]]

    @property
    def query(self) -> dict[str, Any]:
        return self.envelope["query"]

    @property
    def query_id(self) -> str:
        return self.query["id"]


def _require(condition: bool, message: str) -> None:
    if not condition:
        raise ComparisonError(message)


def _no_duplicate_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise ComparisonError(f"input contains duplicate JSON key {key!r}")
        result[key] = value
    return result


def _reject_nonfinite_constant(value: str) -> None:
    raise ComparisonError(f"input contains non-finite JSON number {value}")


def _load_query(path_text: str, role: str) -> QueryInput:
    path = Path(path_text)
    _require(path.is_file(), f"{role} input is not a file: {path}")
    raw = path.read_bytes()
    try:
        envelope = json.loads(
            raw,
            object_pairs_hook=_no_duplicate_object,
            parse_constant=_reject_nonfinite_constant,
        )
        precise = json.loads(
            raw,
            parse_float=Decimal,
            object_pairs_hook=_no_duplicate_object,
            parse_constant=_reject_nonfinite_constant,
        )
    except (json.JSONDecodeError, UnicodeDecodeError) as exc:
        raise ComparisonError(f"{role} input is malformed JSON: {path}") from exc

    _require(isinstance(envelope, dict) and isinstance(precise, dict), f"{role} input must be a JSON object")
    _require(envelope.get("contract_version") == CONTRACT_VERSION, f"{role} input has the wrong contract_version")
    _require(not envelope.get("rows_preview_only"), f"{role} input is a bounded preview; pass its exact input_file instead")
    _require("input_file" not in envelope, f"{role} input is a tool envelope; pass the referenced exact input_file instead")
    for field in ("enterprise_id", "edge_node_id"):
        _require(isinstance(envelope.get(field), str) and bool(envelope[field]), f"{role} input is missing {field}")

    catalog = envelope.get("catalog")
    _require(isinstance(catalog, dict), f"{role} input is missing catalog")
    for field in ("version", "freshness_token"):
        _require(isinstance(catalog.get(field), str) and bool(catalog[field]), f"{role} catalog is missing {field}")

    query = envelope.get("query")
    _require(isinstance(query, dict), f"{role} input is missing query")
    _require(isinstance(query.get("id"), str) and bool(query["id"]), f"{role} query is missing id")
    _require(isinstance(query.get("sql"), str) and bool(query["sql"].strip()), f"{role} query is missing sql")
    _require(type(query.get("rows_returned")) is int and query["rows_returned"] >= 0, f"{role} query has invalid rows_returned")
    _require(type(query.get("applied_limit")) is int and query["applied_limit"] >= 1, f"{role} query has invalid applied_limit")
    _require(type(query.get("truncated")) is bool, f"{role} query has invalid truncated flag")

    columns = envelope.get("columns")
    rows = envelope.get("rows")
    precise_rows = precise.get("rows")
    _require(isinstance(columns, list), f"{role} input has invalid columns")
    _require(isinstance(rows, list) and isinstance(precise_rows, list), f"{role} input has invalid rows")
    _require(query["rows_returned"] == len(rows), f"{role} rows_returned does not match rows length")
    column_names: list[str] = []
    for column in columns:
        _require(isinstance(column, dict), f"{role} input contains an invalid column definition")
        _require(isinstance(column.get("name"), str) and bool(column["name"]), f"{role} input contains an unnamed column")
        _require(isinstance(column.get("type"), str) and bool(column["type"]), f"{role} column {column['name']} is missing type")
        column_names.append(column["name"])
    _require(len(column_names) == len(set(column_names)), f"{role} input contains duplicate column names")
    declared = set(column_names)
    for index, (row, precise_row) in enumerate(zip(rows, precise_rows)):
        _require(isinstance(row, dict) and isinstance(precise_row, dict), f"{role} row {index} is not an object")
        _require(set(row).issubset(declared), f"{role} row {index} contains an undeclared column")

    limits = envelope.get("limits")
    _require(isinstance(limits, dict), f"{role} input is missing source limits")
    _require(isinstance(limits.get("coverage"), dict), f"{role} limits.coverage is invalid")
    _require(isinstance(limits.get("read_consistency"), str) and bool(limits["read_consistency"]), f"{role} limits.read_consistency is invalid")
    evidence = envelope.get("evidence")
    _require(isinstance(evidence, dict) and isinstance(evidence.get("receipt_sha256"), str), f"{role} input is missing evidence receipt")
    return QueryInput(role=role, path=path, raw=raw, envelope=envelope, precise_rows=precise_rows)


def _selected_columns(item: QueryInput, names: Iterable[str], kind: str) -> None:
    declared = {column["name"] for column in item.envelope["columns"]}
    for name in names:
        _require(name in declared, f"{item.role} {kind} column is not declared: {name}")
        for index, row in enumerate(item.precise_rows):
            _require(name in row, f"{item.role} row {index} is missing selected {kind} column {name}")


def _key_token(value: Any) -> tuple[str, str]:
    if value is None:
        raise ComparisonError("comparison keys must be non-null")
    if type(value) is bool:
        return ("bool", "true" if value else "false")
    if type(value) is int:
        return ("integer", str(value))
    if isinstance(value, Decimal):
        _require(value.is_finite(), "comparison keys cannot be non-finite numbers")
        return ("number", str(value))
    if isinstance(value, str):
        return ("string", value)
    raise ComparisonError(f"comparison keys must be scalar JSON values, got {type(value).__name__}")


def _index_rows(item: QueryInput, keys: list[str]) -> tuple[dict[tuple[tuple[str, str], ...], dict[str, Any]], list[tuple[tuple[str, str], ...]]]:
    indexed: dict[tuple[tuple[str, str], ...], dict[str, Any]] = {}
    order: list[tuple[tuple[str, str], ...]] = []
    for index, row in enumerate(item.precise_rows):
        try:
            token = tuple(_key_token(row[key]) for key in keys)
        except ComparisonError as exc:
            raise ComparisonError(f"{item.role} row {index}: {exc}") from exc
        if token in indexed:
            display = {key: _json_value(row[key]) for key in keys}
            raise ComparisonError(
                f"{item.role} query {item.query_id} has duplicate key {json.dumps(display, ensure_ascii=False)}; "
                "preaggregate at the Catalog-defined fact grain before comparing"
            )
        indexed[token] = row
        order.append(token)
    return indexed, order


def _decimal(value: Any, *, role: str, column: str) -> Decimal | None:
    if value is None:
        return None
    if type(value) is bool:
        raise ComparisonError(f"{role} column {column} contains a boolean, not a number")
    if type(value) is int:
        result = Decimal(value)
    elif isinstance(value, Decimal):
        result = value
    elif isinstance(value, str):
        try:
            result = Decimal(value)
        except InvalidOperation as exc:
            raise ComparisonError(f"{role} column {column} contains malformed number {value!r}") from exc
    else:
        raise ComparisonError(f"{role} column {column} contains unsupported numeric value {value!r}")
    if not result.is_finite():
        raise ComparisonError(f"{role} column {column} contains a non-finite number")
    return result


def _json_value(value: Any) -> Any:
    if isinstance(value, Decimal):
        return str(value)
    return value


def _decimal_text(value: Decimal) -> str:
    return format(value, "f")


def _working_precision(values: Iterable[Decimal], row_count: int) -> int:
    integer_digits = 1
    fractional_digits = 0
    for value in values:
        digits = len(value.as_tuple().digits)
        exponent = value.as_tuple().exponent
        integer_digits = max(integer_digits, digits + exponent, 1)
        fractional_digits = max(fractional_digits, -exponent, 0)
    return max(96, integer_digits + fractional_digits + len(str(max(row_count, 1))) + 32)


def _units(items: list[str]) -> dict[str, str]:
    result: dict[str, str] = {}
    for item in items:
        name, separator, unit = item.partition("=")
        _require(bool(separator) and bool(name) and bool(unit), f"invalid --unit {item!r}; use COLUMN=UNIT")
        _require(name not in result, f"duplicate unit declaration for {name}")
        result[name] = unit
    return result


def _input_provenance(item: QueryInput) -> dict[str, Any]:
    query = item.query
    catalog = item.envelope["catalog"]
    return {
        "role": item.role,
        "path": str(item.path),
        "file_sha256": hashlib.sha256(item.raw).hexdigest(),
        "query_id": query["id"],
        "catalog_version": catalog["version"],
        "freshness_token": catalog["freshness_token"],
        "sql": query["sql"],
        "rows_returned": query["rows_returned"],
        "applied_limit": query["applied_limit"],
        "truncated": query["truncated"],
        "columns": item.envelope["columns"],
        "limits": item.envelope["limits"],
        "evidence": item.envelope["evidence"],
    }


def _scope(left: QueryInput, right: QueryInput) -> dict[str, Any]:
    partial = left.query["truncated"] or right.query["truncated"]
    return {
        "basis": "supplied_returned_rows",
        "status": "partial_due_to_truncated_input" if partial else "complete_returned_rows",
        "partial": partial,
        "business_universe_complete": False,
        "limits_coverage_semantics": "source object observations, not measured query population coverage",
        "disclosure": "Results are bounded to the rows returned by the supplied queries; query filters and source limits still apply.",
    }


def _provenance_comparison(left: QueryInput, right: QueryInput) -> dict[str, Any]:
    same_catalog = left.envelope["catalog"]["version"] == right.envelope["catalog"]["version"]
    same_freshness = left.envelope["catalog"]["freshness_token"] == right.envelope["catalog"]["freshness_token"]
    return {
        "same_catalog_version": same_catalog,
        "same_freshness_token": same_freshness,
        "interpretation_required": not (same_catalog and same_freshness),
        "disclosure": "Catalog and freshness differences require Agent interpretation; matching Catalog metadata does not prove a database snapshot.",
    }


def _base_artifact(mode: str, left: QueryInput, right: QueryInput, keys: list[str]) -> dict[str, Any]:
    _require(
        left.envelope["enterprise_id"] == right.envelope["enterprise_id"]
        and left.envelope["edge_node_id"] == right.envelope["edge_node_id"],
        "inputs belong to different enterprise or Edge node identities",
    )
    return {
        "artifact_type": ARTIFACT_TYPE,
        "mode": mode,
        "enterprise_id": left.envelope["enterprise_id"],
        "edge_node_id": left.envelope["edge_node_id"],
        "inputs": [_input_provenance(left), _input_provenance(right)],
        "scope": _scope(left, right),
        "provenance_comparison": _provenance_comparison(left, right),
        "spec": {"keys": keys},
    }


def _ordered_union(
    left: dict[tuple[tuple[str, str], ...], dict[str, Any]],
    left_order: list[tuple[tuple[str, str], ...]],
    right: dict[tuple[tuple[str, str], ...], dict[str, Any]],
    right_order: list[tuple[tuple[str, str], ...]],
) -> list[tuple[tuple[str, str], ...]]:
    return left_order + [token for token in right_order if token not in left]


def _coverage(left: dict[Any, Any], right: dict[Any, Any]) -> dict[str, int]:
    left_keys, right_keys = set(left), set(right)
    return {
        "both": len(left_keys & right_keys),
        "left_only": len(left_keys - right_keys),
        "right_only": len(right_keys - left_keys),
        "total_keys": len(left_keys | right_keys),
    }


def _key_display(row: dict[str, Any], keys: list[str]) -> dict[str, Any]:
    return {key: _json_value(row[key]) for key in keys}


def _empty_value_stats() -> dict[str, Any]:
    return {"known_count": 0, "unknown_count": 0, "missing_key_count": 0, "known_value_sum": Decimal(0)}


def _record_value(stats: dict[str, Any], value: Decimal | None, present: bool) -> None:
    if not present:
        stats["missing_key_count"] += 1
    elif value is None:
        stats["unknown_count"] += 1
    else:
        stats["known_count"] += 1
        stats["known_value_sum"] += value


def _render_stats(stats: dict[str, Any]) -> dict[str, Any]:
    return {
        "known_count": stats["known_count"],
        "unknown_count": stats["unknown_count"],
        "missing_key_count": stats["missing_key_count"],
        "known_value_sum": _decimal_text(stats["known_value_sum"]),
    }


def period_artifact(
    baseline: QueryInput,
    current: QueryInput,
    keys: list[str],
    values: list[str],
    units: dict[str, str],
    baseline_period: str,
    current_period: str,
) -> dict[str, Any]:
    _require(bool(keys), "period comparison requires at least one --key")
    _require(bool(values), "period comparison requires at least one --value")
    _require(len(keys) == len(set(keys)) and len(values) == len(set(values)), "key and value columns must be unique")
    _require(not (set(keys) & set(values)), "key and value columns must be distinct")
    for item in (baseline, current):
        _selected_columns(item, keys, "key")
        _selected_columns(item, values, "value")
    _require(set(units) == set(values), "every selected --value requires exactly one --unit COLUMN=UNIT")
    left, left_order = _index_rows(baseline, keys)
    right, right_order = _index_rows(current, keys)
    ordered = _ordered_union(left, left_order, right, right_order)

    parsed: dict[tuple[str, tuple[tuple[str, str], ...], str], Decimal | None] = {}
    all_numbers: list[Decimal] = []
    for role, indexed in (("baseline", left), ("current", right)):
        for token, row in indexed.items():
            for column in values:
                number = _decimal(row[column], role=role, column=column)
                parsed[(role, token, column)] = number
                if number is not None:
                    all_numbers.append(number)
    precision = _working_precision(all_numbers, len(ordered))

    rows: list[dict[str, Any]] = []
    summaries = {
        column: {"unit": units.get(column), "baseline": _empty_value_stats(), "current": _empty_value_stats(), "paired": {"known_count": 0, "unknown_count": 0, "difference_sum": Decimal(0)}}
        for column in values
    }
    with localcontext() as context:
        context.prec = precision
        for token in ordered:
            baseline_row, current_row = left.get(token), right.get(token)
            display_row = baseline_row if baseline_row is not None else current_row
            detail: dict[str, Any] = {
                "key": _key_display(display_row, keys),
                "presence": {"baseline": baseline_row is not None, "current": current_row is not None},
                "values": {},
            }
            for column in values:
                baseline_value = parsed.get(("baseline", token, column))
                current_value = parsed.get(("current", token, column))
                _record_value(summaries[column]["baseline"], baseline_value, baseline_row is not None)
                _record_value(summaries[column]["current"], current_value, current_row is not None)
                difference: Decimal | None = None
                relative: Decimal | None = None
                reason: str | None = None
                caveat: str | None = None
                difference_unknown = False
                if baseline_row is None or current_row is None:
                    reason = "key_missing_from_one_input"
                    difference_unknown = True
                elif baseline_value is None or current_value is None:
                    reason = "unknown_input_value"
                    difference_unknown = True
                else:
                    difference = current_value - baseline_value
                    summaries[column]["paired"]["known_count"] += 1
                    summaries[column]["paired"]["difference_sum"] += difference
                    if baseline_value == 0:
                        reason = "zero_baseline"
                    else:
                        with localcontext() as ratio_context:
                            ratio_context.prec = RATIO_PRECISION
                            relative = difference / baseline_value
                        if baseline_value < 0:
                            caveat = "negative baseline reverses the usual business interpretation of relative change"
                if difference_unknown and baseline_row is not None and current_row is not None:
                    summaries[column]["paired"]["unknown_count"] += 1
                detail["values"][column] = {
                    "unit": units.get(column),
                    "baseline": _decimal_text(baseline_value) if baseline_value is not None else None,
                    "current": _decimal_text(current_value) if current_value is not None else None,
                    "difference": _decimal_text(difference) if difference is not None else None,
                    "relative_change": {
                        "value": _decimal_text(relative) if relative is not None else None,
                        "unit": "ratio",
                        "precision_significant_digits": RATIO_PRECISION,
                        "unknown_reason": reason,
                        "caveat": caveat,
                    },
                }
            rows.append(detail)

    artifact = _base_artifact("period", baseline, current, keys)
    artifact["spec"].update({
        "values": values,
        "units": units,
        "declared_period_labels": {"baseline": baseline_period, "current": current_period},
        "period_label_disclosure": "Labels are caller-declared and are not parsed from or verified against SQL.",
        "relative_change_formula": "(current - baseline) / baseline",
    })
    artifact["coverage"] = _coverage(left, right)
    artifact["value_summaries"] = {
        column: {
            "unit": summary["unit"],
            "baseline": _render_stats(summary["baseline"]),
            "current": _render_stats(summary["current"]),
            "paired": {
                "known_count": summary["paired"]["known_count"],
                "unknown_count": summary["paired"]["unknown_count"],
                "known_difference_sum": _decimal_text(summary["paired"]["difference_sum"]),
            },
        }
        for column, summary in summaries.items()
    }
    artifact["rows"] = rows
    return artifact


def _set_name(token: Any, left: dict[Any, Any], right: dict[Any, Any]) -> str:
    if token in left and token in right:
        return "matched"
    return "left_only" if token in left else "right_only"


def reconcile_artifact(
    left_input: QueryInput,
    right_input: QueryInput,
    keys: list[str],
    left_values: list[str],
    right_values: list[str],
    units: dict[str, str],
) -> dict[str, Any]:
    _require(bool(keys), "reconciliation requires at least one --key")
    _require(bool(left_values) or bool(right_values), "reconciliation requires at least one selected value column")
    _require(len(keys) == len(set(keys)), "key columns must be unique")
    _require(len(left_values) == len(set(left_values)) and len(right_values) == len(set(right_values)), "value columns must be unique per side")
    _require(not (set(keys) & (set(left_values) | set(right_values))), "key and value columns must be distinct")
    _selected_columns(left_input, keys, "key")
    _selected_columns(right_input, keys, "key")
    _selected_columns(left_input, left_values, "value")
    _selected_columns(right_input, right_values, "value")
    _require(
        set(units) == set(left_values) | set(right_values),
        "every selected value requires exactly one --unit COLUMN=UNIT",
    )
    left, left_order = _index_rows(left_input, keys)
    right, right_order = _index_rows(right_input, keys)
    ordered = _ordered_union(left, left_order, right, right_order)

    parsed: dict[tuple[str, Any, str], Decimal | None] = {}
    all_numbers: list[Decimal] = []
    for role, indexed, columns in (("left", left, left_values), ("right", right, right_values)):
        for token, row in indexed.items():
            for column in columns:
                number = _decimal(row[column], role=role, column=column)
                parsed[(role, token, column)] = number
                if number is not None:
                    all_numbers.append(number)
    precision = _working_precision(all_numbers, len(ordered))

    sets: dict[str, Any] = {}
    for name in ("matched", "left_only", "right_only"):
        sets[name] = {
            "key_count": 0,
            "left": {column: _empty_value_stats() for column in left_values},
            "right": {column: _empty_value_stats() for column in right_values},
        }
    rows: list[dict[str, Any]] = []
    with localcontext() as context:
        context.prec = precision
        for token in ordered:
            left_row, right_row = left.get(token), right.get(token)
            display_row = left_row if left_row is not None else right_row
            set_name = _set_name(token, left, right)
            sets[set_name]["key_count"] += 1
            detail: dict[str, Any] = {
                "key": _key_display(display_row, keys),
                "membership": set_name,
                "presence": {"left": left_row is not None, "right": right_row is not None},
                "left_values": {},
                "right_values": {},
            }
            for role, row, columns in (("left", left_row, left_values), ("right", right_row, right_values)):
                for column in columns:
                    value = parsed.get((role, token, column))
                    _record_value(sets[set_name][role][column], value, row is not None)
                    detail[f"{role}_values"][column] = {
                        "value": _decimal_text(value) if value is not None else None,
                        "unit": units.get(column),
                    }
            rows.append(detail)

    rendered_sets: dict[str, Any] = {}
    for set_name, summary in sets.items():
        rendered_sets[set_name] = {"key_count": summary["key_count"], "left": {}, "right": {}}
        for role, columns in (("left", left_values), ("right", right_values)):
            rendered_sets[set_name][role] = {
                column: {"unit": units.get(column), **_render_stats(summary[role][column])}
                for column in columns
            }

    artifact = _base_artifact("reconcile", left_input, right_input, keys)
    artifact["spec"].update({"left_values": left_values, "right_values": right_values, "units": units})
    artifact["coverage"] = _coverage(left, right)
    artifact["set_summaries"] = rendered_sets
    artifact["rows"] = rows
    return artifact


def _compact_output(artifact: dict[str, Any], output: Path) -> dict[str, Any]:
    compact = {
        "artifact_type": artifact["artifact_type"],
        "mode": artifact["mode"],
        "status": artifact["scope"]["status"],
        "output": str(output),
        "inputs": [{"role": item["role"], "query_id": item["query_id"]} for item in artifact["inputs"]],
        "coverage": artifact["coverage"],
    }
    if "value_summaries" in artifact:
        compact["value_summaries"] = artifact["value_summaries"]
    else:
        compact["set_summaries"] = artifact["set_summaries"]
    return compact


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Compare exact Edge /v1/query JSON files")
    subparsers = parser.add_subparsers(dest="mode", required=True)

    period = subparsers.add_parser("period", help="full-outer current versus baseline comparison")
    period.add_argument("--baseline", required=True)
    period.add_argument("--current", required=True)
    period.add_argument("--key", action="append", required=True)
    period.add_argument("--value", action="append", required=True)
    period.add_argument("--unit", action="append", default=[])
    period.add_argument("--baseline-period", required=True)
    period.add_argument("--current-period", required=True)
    period.add_argument("--output")

    reconcile = subparsers.add_parser("reconcile", help="two-sided key membership and value reconciliation")
    reconcile.add_argument("--left", required=True)
    reconcile.add_argument("--right", required=True)
    reconcile.add_argument("--key", action="append", required=True)
    reconcile.add_argument("--left-value", action="append", default=[])
    reconcile.add_argument("--right-value", action="append", default=[])
    reconcile.add_argument("--unit", action="append", default=[])
    reconcile.add_argument("--output")
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = _parser()
    args = parser.parse_args(argv)
    try:
        units = _units(args.unit)
        if args.mode == "period":
            baseline = _load_query(args.baseline, "baseline")
            current = _load_query(args.current, "current")
            artifact = period_artifact(
                baseline,
                current,
                args.key,
                args.value,
                units,
                args.baseline_period,
                args.current_period,
            )
            input_paths = {baseline.path.resolve(), current.path.resolve()}
        else:
            left = _load_query(args.left, "left")
            right = _load_query(args.right, "right")
            artifact = reconcile_artifact(left, right, args.key, args.left_value, args.right_value, units)
            input_paths = {left.path.resolve(), right.path.resolve()}
        encoded = json.dumps(artifact, ensure_ascii=False, indent=2, allow_nan=False) + "\n"
        if args.output:
            output = Path(args.output)
            _require(output.resolve() not in input_paths, "--output must not overwrite an input file")
            output.parent.mkdir(parents=True, exist_ok=True)
            output.write_text(encoded, encoding="utf-8")
            print(json.dumps(_compact_output(artifact, output), ensure_ascii=False, allow_nan=False))
        else:
            sys.stdout.write(encoded)
        return 0
    except (ComparisonError, OSError) as exc:
        print(f"governed_compare: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
