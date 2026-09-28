#!/usr/bin/env python3
"""Scale ladder runner (L1): execute the upload-family ladder × transport
pairing through the public HTTP surface.

与 smoke.py 的边界不同：smoke 只验证场景树可运行；ladder runner 真正执行
probe-upload-filecount-curve 的量级阶梯，并按登记表的 transport 维度对
staging（presigned 三次往返，默认）与 bulk（对象上传单次往返，LAKEFS-04
ChangeSet bulkIngest 显式 opt-in）同 seed 配对出线。

边界（与 CASES.md §3 一致，但 L1 只覆盖其中可 client-timer 的子集）：
- 只绑定 env 配置指向的已部署 KC Server，不启动、不编排任何容器；
- 只经公开 HTTP surface（/catalog/v1 /writer/v1 /knowledge/v1）；
- closed-loop 串行（concurrency=1），到达模型扩展属于 L3（arrival-models）；
- 发压端 CPU 采样只覆盖 runner 进程自身（os.times，含子进程），机器级
  resource-sample 归 L2；
- bulk 配对点的有效性依赖被测 Server 构建包含 LAKEFS-04：旧构建会静默忽略
  bulkIngest，报告把 /health 原文存档，判读时先核对构建。
- 门槛重登记前（scale-benchmark §10 仍为历史参考）本 runner 不做资格判定，
  只出曲线与正确性对账；资格判定归资格档运行。
"""
from __future__ import annotations

import argparse
import json
import os
import platform
import sys
import time
import urllib.parse
from pathlib import Path

ROOT = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT.parent / "scenes"))
from perf_tree import load_yaml  # 受限 YAML 子集解析器，与 scenes 工具同源
from smoke import Client, quantile, quantiles_ms, rate

DEFAULT_TRANSPORTS = ["staging", "bulk"]


def cpu_percent(before, after, wall):
    """采样窗口内的发压端进程 CPU 占用率（%）。

    before/after 是 os.times() 的前四项 (user, system, children_user,
    children_system)；wall 是同一窗口的 perf_counter 秒数——不用
    os.times()[4]：macOS 上其分辨率粗到小窗口会得到非正增量。
    wall<=0 视为无效。
    """
    cpu = (after[0] - before[0]) + (after[1] - before[1])
    cpu += (after[2] - before[2]) + (after[3] - before[3])
    if wall <= 0:
        raise ValueError("non-positive sampling window")
    return cpu / wall * 100.0


def pairing_delta(points, files):
    """同阶梯点下 bulk 相对 staging 的配对增益。

    返回 (staging 行, bulk 行)；两侧任一缺失或失败即返回 (None, None)——
    配对只在两条通路都成功时才有意义。
    """
    by_transport = {p["transport"]: p for p in points if p["files"] == files and p["ok"]}
    staging, bulk = by_transport.get("staging"), by_transport.get("bulk")
    if staging is None or bulk is None:
        return None, None
    return staging, bulk


def self_test():
    """用已知输入验证 ladder 特有计算逻辑（指标数学复用 smoke --self-test）。"""
    failures = []
    if smoke_self_test() != 0:
        failures.append("smoke metric math self-test failed")

    def check(name, got, want, tol=0.0):
        ok = abs(got - want) <= tol if tol else got == want
        if not ok:
            failures.append(f"{name}: got {got!r}, want {want!r}")

    # CPU 占用率：0.5s CPU / 1.0s wall = 50%；wall 由 perf_counter 单独提供
    check("cpu percent half", cpu_percent((0.0, 0.0, 0.0, 0.0), (0.3, 0.2, 0.0, 0.0), 1.0), 50.0)
    # 全部增量落在 children（runner 用子进程发压时的形态）
    check("cpu percent children", cpu_percent((1.0, 1.0, 0.0, 0.0), (1.0, 1.0, 0.6, 0.4), 2.0), 50.0)
    # 零/负窗口必须报错而不是返回 0
    for bad_wall in (0.0, -1.0):
        try:
            cpu_percent((0.0, 0.0, 0.0, 0.0), (0.3, 0.2, 0.0, 0.0), bad_wall)
            failures.append("cpu percent zero window: did not raise")
            break
        except ValueError:
            pass
    # 配对：同 files 的两条通路各归各位；缺 bulk 或含失败点时返回 (None, None)
    points = [
        {"transport": "staging", "files": 10, "ok": True, "commitMs": 100.0},
        {"transport": "bulk", "files": 10, "ok": True, "commitMs": 40.0},
        {"transport": "staging", "files": 100, "ok": True, "commitMs": 900.0},
    ]
    staging, bulk = pairing_delta(points, 10)
    check("pairing picks staging", staging["commitMs"], 100.0)
    check("pairing picks bulk", bulk["commitMs"], 40.0)
    check("pairing missing transport", pairing_delta(points, 100), (None, None))

    if failures:
        print("\n".join(failures), file=sys.stderr)
        return 1
    print("ladder self-test: PASS (cpu percent, transport pairing, smoke metric math)")
    return 0


def smoke_self_test():
    """复用 smoke 的指标数学自测，保证共用函数口径一致。"""
    import io
    import contextlib

    buf = io.StringIO()
    with contextlib.redirect_stdout(buf):
        code = smoke_main_module().self_test()
    return code


def smoke_main_module():
    import smoke

    return smoke


# ---------------------------------------------------------------- runner
class LadderRun:
    def __init__(self, env, out_dir: Path, max_files=None):
        self.env = env
        self.out = out_dir
        self.max_files = max_files
        target = env["target"]
        self.client = Client(target["serverURL"], target["principal"])
        self.catalog = target["catalog"]
        self.store = target.get("managedStore", "")
        self.prefix = target.get("repositoryPrefix", "kc-perf-")
        self.ladders = (env.get("profile") or {}).get("ladders") or {}
        self.limits = env.get("limits") or {}
        self.transports = self.ladders.get("transport") or DEFAULT_TRANSPORTS
        ladder = self.ladders.get("uploadFileCount") or [1, 10, 100]
        if max_files is not None:
            ladder = [n for n in ladder if n <= max_files] or [min(ladder)]
        self.upload_ladder = ladder
        self.schema_id = "schema/perfbench.v1"
        self.runstamp = time.strftime("%Y%m%dT%H%M%S", time.gmtime())
        self.findings = []
        self.points = []

    # ---- 证据与记录
    def note(self, probe, severity, message):
        self.findings.append({"probe": probe, "severity": severity, "message": message})
        print(f"  [{severity}] {probe}: {message}")

    def flush(self, name, payload):
        (self.out / name).write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")

    def append_samples(self, rows):
        with (self.out / "samples.ndjson").open("a", encoding="utf-8") as fh:
            for row in rows:
                fh.write(json.dumps(row, ensure_ascii=False) + "\n")

    # ---- bind（environment-bound：CASES §3 必填参数逐项落 manifest）
    def bind(self):
        _, status, ready = self.client.call("GET", "/readyz", timeout=15)
        if status != 200:
            raise SystemExit(f"bind failed: /readyz -> {status}")
        _, status, who = self.client.call("GET", "/identity/v1/whoami", timeout=15)
        principal = who.get("principal", "") if status == 200 else ""
        if principal != self.env["target"]["principal"]:
            raise SystemExit(f"bind failed: whoami={principal!r} != declared principal")
        _, status, health = self.client.call("GET", "/health", timeout=15)
        manifest = {
            "kind": "scale-ladder-run",
            "startedAt": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "被测版本": {"health": health if status == 200 else "absent", "readyz": ready},
            "环境": {
                "platform": platform.platform(),
                "cpuCount": os.cpu_count(),
                "loadGenPid": os.getpid(),
                "cpuSamplingScope": "runner process only (os.times); machine-level sampling is L2",
            },
            "数据": {"schema": self.schema_id, "docShape": "bench/doc-XXXXXXXX BenchDoc body"},
            "负载": {
                "arrivalModel": "closed-loop sequential",
                "concurrency": 1,
                "uploadLadder": self.upload_ladder,
                "transports": self.transports,
                "maxFilesCap": self.max_files,
            },
            "provider": {"managedStore": self.store, "medium": "lakeFS"},
            "基线": "BASELINE (no approved prior run; thresholds not re-registered)",
            "laddersDeclared": self.ladders,
            "target": {"serverURL": self.client.base, "principal": principal, "catalog": self.catalog},
        }
        self.flush("manifest.json", manifest)
        print(f"bind: readyz=200 whoami={principal} ladder={self.upload_ladder} transports={self.transports}")

    # ---- 公开 surface 原语
    def head_commit(self):
        _, status, head = self.client.call("GET", f"/writer/v1/repositories/{self.repo_seg}/head")
        if status != 200:
            raise SystemExit("head unreadable")
        return head["commit"]

    def commit(self, command_id, operations, bulk=False):
        head = self.head_commit()
        change_set = {
            "targetRepository": self.repo,
            "targetRef": "",
            "baseCommit": head,
            "expectedTargetCommit": head,
            "operations": operations,
        }
        if bulk:
            change_set["bulkIngest"] = True
        start = time.perf_counter()
        times_before = os.times()[:4]
        _, status, resp = self.client.call(
            "POST",
            f"/writer/v1/repositories/{self.repo_seg}/commits",
            {"commandId": command_id, "changeSet": change_set},
            timeout=1800,
        )
        times_after = os.times()[:4]
        applied = status == 200 and resp.get("disposition") in ("APPLIED", "REPLAYED")
        elapsed = time.perf_counter() - start
        return {
            "elapsed": elapsed,
            "startedAt": start,
            "status": status,
            "disposition": resp.get("disposition"),
            "commitId": (resp.get("result") or {}).get("commitId"),
            "genCPUPercent": cpu_percent(times_before, times_after, elapsed),
            "raw": resp,
        }

    def doc_operation(self, index, body_bytes=256):
        body = ("payload %08d " % index) * max(1, body_bytes // 16)
        value = {"entity": "BenchDoc", "aspect": "doc",
                 "title": f"doc {index:08d}", "body": body}
        return {"op": "PUT",
                "address": {"kind": "Entity", "objectId": f"bench/doc-{index:08d}"},
                "value": value, "schemaRef": self.schema_id}

    def create_repository(self):
        name = f"{self.prefix}{self.runstamp}-{len(self.points):04d}"
        _, status, resp = self.client.call("POST", "/catalog/v1/repositories", {"name": name, "store": self.store})
        if status != 200:
            raise SystemExit(f"managed create failed: {resp}")
        self.repo = resp["repositoryId"]
        self.repo_seg = urllib.parse.quote(self.repo, safe="")
        _, status, _ = self.client.call(
            "POST",
            f"/catalog/v1/catalogs/{urllib.parse.quote(self.catalog, safe='')}/repositories",
            {"repository": self.repo},
        )
        if status not in (200, 201, 202):
            raise SystemExit(f"attach failed: {status}")
        _, status, _ = self.client.call("GET", f"/writer/v1/repositories/{self.repo_seg}/head")
        if status != 200:
            raise SystemExit("created repository head not readable")

    def detach_repository(self):
        _, status, _ = self.client.call(
            "DELETE",
            f"/catalog/v1/catalogs/{urllib.parse.quote(self.catalog, safe='')}/repositories/{self.repo_seg}")
        return status

    def put_schema(self):
        schema_value = {"entity": "BenchDoc", "pattern": "record",
                        "fields": {"title": {"type": "string", "required": True, "access": ["filter"]},
                                   "body": {"type": "string", "required": True, "access": ["text"]}}}
        result = self.commit(f"{self.prefix}{self.runstamp}-schema-{len(self.points):04d}",
                             [{"op": "PUT", "address": {"kind": "Entity", "objectId": self.schema_id},
                               "value": schema_value}])
        if result["disposition"] != "APPLIED":
            raise SystemExit(f"schema commit failed: {result['raw']}")

    # ---- 单测量点（probe spec：isolation=per-point-scratch-repository）
    def run_point(self, transport, n):
        self.create_repository()
        self.put_schema()
        ops = [self.doc_operation(i) for i in range(n)]
        command_id = f"{self.prefix}{self.runstamp}-{transport}-n{n:08d}"
        result = self.commit(command_id, ops, bulk=(transport == "bulk"))
        ok = result["disposition"] == "APPLIED"
        if not ok:
            self.note(f"{transport}-n{n}", "FAIL", f"commit: {json.dumps(result['raw'])[:200]}")

        receipt_ms = None
        if ok:
            r_start = time.perf_counter()
            _, r_status, _ = self.client.call("GET", f"/writer/v1/receipts/{command_id}")
            receipt_ms = (time.perf_counter() - r_start) * 1000
            if r_status != 200:
                self.note(f"{transport}-n{n}", "FAIL", f"receipt unreadable: {r_status}")

        # verify 阶段：全量回读计数（独立计时，不进 commit 时延指标）
        verify_start = time.perf_counter()
        read_ok = 0
        if ok:
            for i in range(n):
                _, r_status, _ = self.client.call(
                    "POST", "/knowledge/v1/objects:read",
                    {"repository": self.repo, "object": f"bench/doc-{i:08d}"})
                if r_status == 200:
                    read_ok += 1
        verify_seconds = time.perf_counter() - verify_start

        # duplicate command-id 重放：必须 REPLAYED 且 commit 不变（同 id 异 digest 是冲突族）
        replay_ok = None
        if ok:
            replay = self.commit(command_id, ops, bulk=(transport == "bulk"))
            replay_ok = replay["disposition"] == "REPLAYED" and replay["commitId"] == result["commitId"]
            if not replay_ok:
                self.note(f"{transport}-n{n}", "FAIL",
                          f"replay changed outcome: {replay['disposition']} {replay['commitId']}")

        lost = n - read_ok if ok else n
        cpu = result["genCPUPercent"]
        limit = self.limits.get("generatorCPUPercent")
        if limit is not None and cpu > limit:
            self.note(f"{transport}-n{n}", "INVALID",
                      f"load-gen CPU {cpu:.0f}% exceeded limit {limit}%")
        row = {
            "probe": "probe-upload-filecount-curve",
            "transport": transport, "files": n, "ok": ok,
            "commitMs": result["elapsed"] * 1000,
            "filesPerSecond": rate(n, result["elapsed"]) if ok and result["elapsed"] > 0 else 0.0,
            "receiptMs": receipt_ms,
            "verifySeconds": verify_seconds,
            "readOk": read_ok, "lost": lost,
            "replayOk": replay_ok,
            "genCPUPercent": cpu,
            "commandId": command_id, "commitId": result["commitId"],
        }
        self.append_samples([row])
        self.points.append(row)
        status = self.detach_repository()
        if status not in (200, 202, 204):
            self.note(f"{transport}-n{n}", "WARN", f"detach scratch repo -> {status}")
        print(f"  {transport:7s} n={n:<6d} commit={row['commitMs']:9.1f}ms "
              f"{row['filesPerSecond']:8.1f} files/s genCPU={cpu:5.1f}%")
        return row

    # ---- 汇总：曲线 + 配对增益 + 正确性判定（无资格门槛判定）
    def evaluate(self):
        curves = {}
        for transport in self.transports:
            rows = [p for p in self.points if p["transport"] == transport and p["ok"]]
            curves[transport] = {
                str(p["files"]): {"commitMs": p["commitMs"], "filesPerSecond": p["filesPerSecond"]}
                for p in rows
            }
            if rows:
                curves[transport]["write.commit.latency"] = quantiles_ms([p["commitMs"] for p in rows])
        pairing = {}
        for n in self.upload_ladder:
            staging, bulk = pairing_delta(self.points, n)
            if staging and bulk and staging["filesPerSecond"] > 0:
                pairing[str(n)] = {
                    "bulkSpeedup": bulk["filesPerSecond"] / staging["filesPerSecond"],
                    "stagingCommitMs": staging["commitMs"], "bulkCommitMs": bulk["commitMs"],
                }
        hard = [f for f in self.findings if f["severity"] == "FAIL"]
        invalid = [f for f in self.findings if f["severity"] == "INVALID"]
        checks = [
            {"metric": "correctness.lost_objects", "observed": sum(p["lost"] for p in self.points),
             "bound": 0, "ok": all(p["lost"] == 0 for p in self.points if p["ok"]) and self.points},
            {"metric": "write.duplicate_extra_revision",
             "observed": sum(0 if p["replayOk"] else 1 for p in self.points if p["replayOk"] is not None),
             "bound": 0, "ok": all(p["replayOk"] for p in self.points if p["replayOk"] is not None) and self.points},
        ]
        overall = "INVALID" if invalid else ("FAILED" if hard or any(not c["ok"] for c in checks) else "PASSED")
        report = {
            "kind": "scale-ladder-report",
            "runDir": str(self.out),
            "scope": "probe-upload-filecount-curve ladder x transport; curves and correctness only, no qualification verdict",
            "curves": curves,
            "transportPairing": pairing,
            "expectationChecks": checks,
            "findings": self.findings,
            "overall": overall,
        }
        self.flush("report.json", report)
        return report

    def run(self):
        self.out.mkdir(parents=True, exist_ok=True)
        self.bind()
        try:
            for transport in self.transports:
                for n in self.upload_ladder:
                    self.run_point(transport, n)
        finally:
            pass  # 每个测量点自带 detach；bind 失败时无可清理对象
        report = self.evaluate()
        print(f"overall: {report['overall']}")
        pairing = report["transportPairing"]
        if pairing:
            print(f"bulk vs staging speedup: {json.dumps({k: round(v['bulkSpeedup'], 2) for k, v in pairing.items()})}")
        return 0 if report["overall"] == "PASSED" else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--env", help="environment config (env.example.yaml shape)")
    parser.add_argument("--max-files", type=int, help="cap the upload ladder to files <= N (smoke-scale runs)")
    parser.add_argument("--out", help="evidence dir override (default runs/ladder-<runId>)")
    parser.add_argument("--self-test", action="store_true", help="verify pairing/CPU math and smoke metric math")
    args = parser.parse_args()
    if args.self_test:
        return self_test()
    if not args.env:
        parser.error("--env is required unless --self-test")
    env_path = Path(args.env)
    env = load_yaml(env_path.read_text(encoding="utf-8"))
    if not isinstance(env, dict) or "target" not in env:
        print("env config missing target section", file=sys.stderr)
        return 2
    scale = (env.get("profile") or {}).get("scale")
    if scale not in ["S0", "S1", "S2", "S3", "S4", "S5"]:
        print(f"profile.scale must be one of S0..S5", file=sys.stderr)
        return 2
    run_id = (env.get("run") or {}).get("runId") or ""
    if not run_id or run_id == "auto":
        run_id = time.strftime("%Y%m%dT%H%M%SZ", time.gmtime())
    evidence = (env.get("run") or {}).get("evidenceDir") or "../runs"
    out_dir = Path(args.out) if args.out else (env_path.resolve().parent / evidence / f"ladder-{run_id}").resolve()
    return LadderRun(env, out_dir, max_files=args.max_files).run()


if __name__ == "__main__":
    sys.exit(main())
