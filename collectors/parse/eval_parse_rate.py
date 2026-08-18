#!/usr/bin/env python3
"""Measure sqlglot parse rate on ETL table dump and hippo_sources."""

from __future__ import annotations

import argparse
import csv
import json
import logging
import sys

logging.getLogger("sqlglot").setLevel(logging.ERROR)
from collections import Counter
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from parse.hippo_sources import iter_sources, sql_from_hippo_content, type_counts  # noqa: E402
from parse.sql import ParseResult, is_blank, parse_sql  # noqa: E402

csv.field_size_limit(sys.maxsize)


def _record(result: ParseResult) -> dict:
    return {
        "ok": result.ok,
        "dialect": result.dialect,
        "skipped": result.skipped,
        "error_type": result.error_type,
        "reads": len(result.reads),
        "writes": len(result.writes),
        "statements": result.statements,
    }


def summarize(rows: list[dict]) -> dict:
    n = len(rows)
    ok = sum(1 for r in rows if r["ok"])
    skipped = sum(1 for r in rows if r.get("skipped"))
    attempted = n - skipped
    with_reads = sum(1 for r in rows if r["ok"] and r["reads"] > 0)
    with_writes = sum(1 for r in rows if r["ok"] and r["writes"] > 0)
    errors = Counter(r["error_type"] for r in rows if r.get("error_type"))
    skipped_why = Counter(r["skipped"] for r in rows if r.get("skipped"))
    dialects = Counter(r["dialect"] for r in rows if r.get("dialect"))
    return {
        "n": n,
        "attempted": attempted,
        "ok": ok,
        "parse_rate": round(ok / attempted, 4) if attempted else None,
        "ok_with_reads": with_reads,
        "ok_with_writes": with_writes,
        "skipped": skipped,
        "skipped_why": dict(skipped_why),
        "error_types": dict(errors),
        "dialects": dict(dialects),
    }


def eval_etl_csv(path: Path, limit: int | None, timeout: int) -> tuple[list[dict], dict]:
    rows: list[dict] = []
    with path.open(newline="", encoding="utf-8-sig") as fh:
        for i, row in enumerate(csv.DictReader(fh)):
            if limit is not None and i >= limit:
                break
            sql = row.get("source_sql") or row.get("task_content") or ""
            result = parse_sql(sql, timeout_sec=timeout)
            rec = _record(result)
            rec["id"] = row.get("task_id") or str(i)
            rec["kind"] = "etl_table"
            rows.append(rec)
    return rows, summarize(rows)


def eval_hippo_sources(source_type: str, password: str, limit: int | None, timeout: int) -> tuple[list[dict], dict, dict]:
    catalog = {"listed": 0, "with_file": 0, "missing_file": 0}
    rows: list[dict] = []
    for src in iter_sources(source_type, password, limit=limit):
        catalog["listed"] += 1
        if src.content is None:
            catalog["missing_file"] += 1
            continue
        catalog["with_file"] += 1
        sql = sql_from_hippo_content(src.content, source_type)
        result = parse_sql(sql or "", timeout_sec=timeout)
        rec = _record(result)
        rec["id"] = src.source_id
        rec["kind"] = source_type
        rec["space"] = src.space
        rows.append(rec)
    return rows, summarize(rows), catalog


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--etl-csv",
        default=str(Path(__file__).resolve().parents[2] / ".data" / "7541f7fedde64148a8d8fbe3f9e77f7f.csv"),
    )
    parser.add_argument("--limit", type=int, default=None, help="Max rows per corpus")
    parser.add_argument("--timeout", type=int, default=5)
    parser.add_argument("--hippo-probe", type=int, default=80, help="How many hippo_sources to open for file bodies")
    parser.add_argument("--es-password", default="")
    parser.add_argument(
        "--out",
        default=str(Path(__file__).resolve().parents[2] / ".data" / "parse-rate.json"),
    )
    args = parser.parse_args()

    report: dict = {"etl_table": None, "hippo_sources": {"types": {}, "query_sql": None, "etl_sql": None}}

    etl_path = Path(args.etl_csv)
    if etl_path.is_file():
        print(f"[etl] {etl_path}", flush=True)
        _rows, summary = eval_etl_csv(etl_path, args.limit, args.timeout)
        report["etl_table"] = summary
        print("[etl]", json.dumps(summary, ensure_ascii=False), flush=True)
    else:
        print(f"[etl] missing {etl_path}", flush=True)

    password = args.es_password or os_password()
    if password:
        try:
            types = type_counts(password)
            report["hippo_sources"]["types"] = types
            print("[hippo_sources] types", types, flush=True)
            for stype in ("query_sql", "etl_sql"):
                _rows, summary, catalog = eval_hippo_sources(
                    stype, password, args.hippo_probe if args.limit is None else min(args.limit, args.hippo_probe), args.timeout
                )
                report["hippo_sources"][stype] = {"catalog": catalog, "parse": summary}
                print(f"[hippo_sources {stype}]", catalog, summary, flush=True)
        except Exception as exc:
            report["hippo_sources"]["error"] = str(exc)
            print("[hippo_sources] error", exc, flush=True)
    else:
        print("[hippo_sources] skip (no ES password)", flush=True)

    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print("[out]", out)


def os_password() -> str:
    env = Path("/data/home/kaiqidong/Hippo/.env")
    if not env.is_file():
        return ""
    for line in env.read_text(encoding="utf-8").splitlines():
        if line.startswith("HIPPO_OS_PASSWORD="):
            return line.split("=", 1)[1].strip()
    return ""


if __name__ == "__main__":
    main()
