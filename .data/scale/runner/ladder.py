#!/usr/bin/env python3
"""Scale ladder runner (L1+L2min): execute the upload-family ladder × transport
pairing through the public HTTP surface, with server-side attribution pulled
from the deployment's Prometheus.

与 smoke.py 的边界不同：smoke 只验证场景树可运行；ladder runner 真正执行
probe-upload-filecount-curve 的量级阶梯，并按登记表的 transport 维度对
staging（presigned 三次往返，默认）与 bulk（对象上传单次往返，LAKEFS-04
ChangeSet bulkIngest 显式 opt-in）同 seed 配对出线。

L2 最小归因：env 声明 observability.prometheusURL 时，runner 在整轮窗口上
批量拉 kc_writer_duration_seconds_{sum,count} 与
kc_snapshot_operation_duration_seconds_sum 的抓取样本，按每个测量点的
commit 窗口切增量，得到服务端分段（Writer 层、⓪ adapter 各操作），并按
transport 腿汇总。粒度 caveat：Prometheus 抓取间隔（默认 5s）内不含样本的
亚秒级点位出不了逐点分段，只有腿级总和；快点的逐点分解需要 trace 级数据。

边界（与 CASES.md §3 一致，但 L1 只覆盖其中可 client-timer 的子集）：
- 只绑定 env 配置指向的已部署 KC Server，不启动、不编排任何容器；
- 只经公开 HTTP surface（/catalog/v1 /writer/v1 /knowledge/v1）与只读
  Prometheus query API（可选）；
- closed-loop 串行（concurrency=1），到达模型扩展属于 L3（arrival-models）；
- 发压端 CPU 采样只覆盖 runner 进程自身（os.times，含子进程），机器级
  resource-sample 归 L2 其余部分；
- bulk 配对点的有效性依赖被测 Server 构建包含 LAKEFS-04：旧构建会响亮拒绝
  bulkIngest（strict decode → USAGE_INVALID），报告把 /health 原文存档。
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
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT.parent / "scenes"))
from perf_tree import load_yaml  # 受限 YAML 子集解析器，与 scenes 工具同源
from smoke import Client, quantile, quantiles_ms, rate

DEFAULT_TRANSPORTS = ["staging", "bulk"]
PROM_SERIES = [
    "kc_writer_duration_seconds_sum",
    "kc_writer_duration_seconds_count",
    "kc_snapshot_operation_duration_seconds_sum",
]


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


# ---------------------------------------------------------------- Prometheus
def series_increase(samples, t1, t2):
    """抓取样本网格上 (t1, t2] 窗口内的计数器增量（秒/次）。

    samples 是 query_range 返回的 [(unix_ts, value)]（升序）。取值规则：
    - 窗口起点基线 = t1 时刻或之前最后一个样本；没有基线时增量不可信，
      返回 None（宁缺毋错，不把窗口前半的流量记到本点头上）。
    - 相邻样本增量为负按计数器重置处理（增量记为当前值）。
    返回 (increase, inWindowSamples)；inWindowSamples 是落在 (t1, t2] 的
    样本数——为 0 表示该窗口短于抓取间隔，逐点分段不可分辨。
    """
    baseline = None
    in_window = 0
    for ts, value in samples:
        if ts <= t1:
            baseline = (ts, value)
            continue
        if ts > t2:
            break
        in_window += 1
    if baseline is None:
        return None, in_window
    total = 0.0
    previous = baseline[1]
    for ts, value in samples:
        if ts <= t1 or ts > t2:
            continue
        total += value - previous if value >= previous else value
        previous = value
    return total, in_window


class Prometheus:
    """只读 Prometheus query API 客户端（CASES §3：观测面全部只读）。"""

    def __init__(self, base_url):
        self.base = base_url.rstrip("/")

    def _get(self, path, params):
        url = self.base + path + "?" + urllib.parse.urlencode(params)
        with urllib.request.urlopen(url, timeout=30) as resp:
            payload = json.loads(resp.read().decode("utf-8"))
        if payload.get("status") != "success":
            raise ValueError(f"prometheus query failed: {payload}")
        return payload["data"]["result"]

    def query_range(self, query, start, end, step=5):
        rows = self._get("/api/v1/query_range",
                         {"query": query, "start": start, "end": end, "step": step})
        series = []
        for row in rows:
            labels = {k: v for k, v in row.get("metric", {}).items() if not k.startswith("__")}
            samples = [(float(ts), float(v)) for ts, v in row.get("values", [])]
            series.append({"labels": labels, "samples": samples})
        return series

    def window_series(self, queries, start, end, step=5):
        """批量拉一批 series 的整轮窗口样本：{series: [{labels, samples}]}。"""
        return {name: self.query_range(name, start, end, step) for name in queries}


def window_deltas(window_series, t1, t2):
    """按窗口切增量：{"writer": {sum, count, inWindow}, "snapshot": {op: {...}}}。

    只在窗口内有抓取样本（inWindow>0）的 series 出数；窗口短于抓取间隔时
    对应键缺省——报告据此区分「该点不可分辨」与「该段服务端耗时为 0」。
    """
    out = {"writer": {}, "snapshot": {}}
    for series in window_series.get("kc_writer_duration_seconds_sum", []):
        inc, n = series_increase(series["samples"], t1, t2)
        if inc is not None and n > 0:
            out["writer"]["sumSeconds"] = out["writer"].get("sumSeconds", 0.0) + inc
            out["writer"]["inWindow"] = max(out["writer"].get("inWindow", 0), n)
    for series in window_series.get("kc_writer_duration_seconds_count", []):
        inc, n = series_increase(series["samples"], t1, t2)
        if inc is not None and n > 0:
            out["writer"]["ops"] = out["writer"].get("ops", 0.0) + inc
    for series in window_series.get("kc_snapshot_operation_duration_seconds_sum", []):
        op = series["labels"].get("kc_operation", "unknown")
        inc, n = series_increase(series["samples"], t1, t2)
        if inc is not None and n > 0:
            slot = out["snapshot"].setdefault(op, {"sumSeconds": 0.0, "inWindow": 0})
            slot["sumSeconds"] += inc
            slot["inWindow"] = max(slot["inWindow"], n)
    return out


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

    # 计数器窗口增量：窗口 (10, 30] 内两个样本，基线取 t1 处或之前最后一个
    samples = [(5.0, 100.0), (10.0, 110.0), (15.0, 112.0), (25.0, 114.0), (35.0, 120.0)]
    inc, n = series_increase(samples, 10.0, 30.0)
    check("window increase", inc, 4.0)          # 110 -> 112 -> 114
    check("window in-window samples", n, 2)
    # 无基线（t1 之前没有样本）必须返回 None 而不是把前半窗口记进来
    check("window no baseline", series_increase(samples, 3.0, 15.0)[0], None)
    # 计数器重置：负增量按重置记当前值
    resets = [(10.0, 50.0), (20.0, 3.0), (30.0, 6.0)]
    inc, _ = series_increase(resets, 10.0, 30.0)
    check("counter reset", inc, 6.0)            # 重置后 3 -> 6
    # 窗口短于抓取间隔（无窗口内样本）时增量为 0 且不可分辨：出数口径与
    # window_deltas 的「无样本不出键」一致
    inc, n = series_increase(samples, 10.0, 12.0)
    check("narrow window", (inc, n), (0.0, 0))
    # window_deltas：writer 与 snapshot 各自归段，无样本不出键
    grid = {
        "kc_writer_duration_seconds_sum": [{"labels": {}, "samples": samples}],
        "kc_writer_duration_seconds_count": [{"labels": {}, "samples": samples}],
        "kc_snapshot_operation_duration_seconds_sum": [
            {"labels": {"kc_operation": "commit"}, "samples": samples},
        ],
    }
    deltas = window_deltas(grid, 10.0, 30.0)
    check("writer sum", deltas["writer"]["sumSeconds"], 4.0)
    check("writer ops", deltas["writer"]["ops"], 4.0)
    check("snapshot op", deltas["snapshot"]["commit"]["sumSeconds"], 4.0)
    check("empty window absent", "sumSeconds" in window_deltas(grid, 40.0, 45.0)["writer"], False)

    if failures:
        print("\n".join(failures), file=sys.stderr)
        return 1
    print("ladder self-test: PASS (cpu percent, transport pairing, window attribution, smoke metric math)")
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
        self.prom = None
        observability = env.get("observability") or {}
        prom_url = (observability.get("prometheusURL") or "").strip()
        if prom_url:
            self.prom = Prometheus(prom_url)

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
        started_epoch = time.time()
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
            "startedEpoch": started_epoch,
            "endedEpoch": time.time(),
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
            "commitWindow": [result["startedEpoch"], result["endedEpoch"]],
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

    # ---- L2 最小归因：整轮窗口批量拉样本，按点位窗口切服务端增量
    def attach_attribution(self):
        if self.prom is None:
            return
        if not self.points:
            return
        start = min(p["commitWindow"][0] for p in self.points) - 15
        end = max(p["commitWindow"][1] for p in self.points) + 15
        try:
            grid = self.prom.window_series(PROM_SERIES, start, end)
        except Exception as exc:  # 观测面可选：抓取失败降级为 WARN，不废运行
            self.note("attribution", "WARN", f"prometheus unavailable: {exc}")
            return
        for row in self.points:
            row["serverDeltas"] = window_deltas(grid, row["commitWindow"][0], row["commitWindow"][1])
        legs = {}
        for transport in self.transports:
            rows = [p for p in self.points if p["transport"] == transport and p["ok"]]
            if not rows:
                continue
            leg = {"points": len(rows), "resolvablePoints": 0, "clientWallSeconds": 0.0,
                   "writer": {"sumSeconds": 0.0, "ops": 0.0}, "snapshot": {}}
            for row in rows:
                leg["clientWallSeconds"] += row["commitMs"] / 1000
                deltas = row.get("serverDeltas") or {}
                writer = deltas.get("writer") or {}
                if writer.get("inWindow"):
                    leg["resolvablePoints"] += 1
                    leg["writer"]["sumSeconds"] += writer.get("sumSeconds", 0.0)
                    leg["writer"]["ops"] += writer.get("ops", 0.0)
                for op, slot in (deltas.get("snapshot") or {}).items():
                    target = leg["snapshot"].setdefault(op, {"sumSeconds": 0.0, "resolvablePoints": 0})
                    if slot.get("inWindow"):
                        target["sumSeconds"] += slot["sumSeconds"]
                        target["resolvablePoints"] += 1
            legs[transport] = leg
        self.observability = {
            "prometheusURL": self.prom.base,
            "windowSeries": PROM_SERIES,
            "caveat": ("scrape-granularity limited: sub-second commit windows may contain no "
                       "scrape sample; per-point segments appear only where inWindow>0, "
                       "leg totals sum resolvable points only"),
            "legs": legs,
        }
        # 补充口径：测量跨度内全部 KC 服务端操作（含每点 setup 的 schema 提交），
        # 不等于纯测量提交的分段——给报告一个总盘对照，避免腿级全 0 时无信息。
        first_t1 = min(p["commitWindow"][0] for p in self.points)
        last_t2 = max(p["commitWindow"][1] for p in self.points)
        self.observability["runWindow"] = {
            "span": [first_t1, last_t2],
            "note": ("all KC server operations within the measured-commit span, including "
                     "per-point setup; not a per-transport attribution"),
            "deltas": window_deltas(grid, first_t1, last_t2),
        }

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
            "scope": "probe-upload-filecount-curve ladder x transport; curves, correctness and best-effort server attribution; no qualification verdict",
            "curves": curves,
            "transportPairing": pairing,
            "observability": getattr(self, "observability", None),
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
        self.attach_attribution()
        report = self.evaluate()
        print(f"overall: {report['overall']}")
        pairing = report["transportPairing"]
        if pairing:
            print(f"bulk vs staging speedup: {json.dumps({k: round(v['bulkSpeedup'], 2) for k, v in pairing.items()})}")
        observability = report.get("observability") or {}
        for transport, leg in (observability.get("legs") or {}).items():
            writer = leg["writer"]
            share = (writer["sumSeconds"] / leg["clientWallSeconds"] * 100) if leg["clientWallSeconds"] else 0
            print(f"  {transport:7s} server writer share: {writer['sumSeconds']:7.2f}s "
                  f"of {leg['clientWallSeconds']:7.2f}s client wall ({share:5.1f}%, "
                  f"{leg['resolvablePoints']}/{leg['points']} points resolvable)")
        run_window = (observability.get("runWindow") or {}).get("deltas", {}).get("writer") or {}
        if run_window.get("sumSeconds") is not None:
            ops = run_window.get("ops", 0.0)
            print(f"  run window: KC writer total {run_window['sumSeconds']:.2f}s over {ops:.0f} ops "
                  f"(incl. per-point setup)")
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
