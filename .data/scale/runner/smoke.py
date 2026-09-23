#!/usr/bin/env python3
"""Scale scene smoke runner: drive the performance scene tree end to end.

这不是压测 runner，而是 scenes/ 用例树的**可运行性验证**（SMOKE）：
- 只经公开 HTTP surface 施压（writer / knowledge / operations / catalog）；
- 只绑定 env 配置指向的已部署 KC Server，不启动、不编排任何容器；
- 以远小于资格档的量级阶梯执行 upload / object-size / read / browse /
  index-update 五族探针，验证链路跑通、计数对账、指标计算与判读正确。

指标口径与 scenes/metrics.yaml 对齐；本 runner 只覆盖其中 client-timer 可测子集。
"""
from __future__ import annotations

import argparse
import json
import math
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT.parent / "scenes"))
from perf_tree import load_yaml  # 受限 YAML 子集解析器，与 scenes 工具同源

SCALES = ["S0", "S1", "S2", "S3", "S4", "S5"]


# ---------------------------------------------------------------- 指标计算
def quantile(samples, q):
    """最近秩（nearest-rank）分位数：sorted[ceil(q*n)-1]。samples 为空报错。"""
    if not samples:
        raise ValueError("quantile of empty samples")
    ordered = sorted(samples)
    rank = math.ceil(q * len(ordered))
    if rank < 1:
        rank = 1
    return ordered[rank - 1]


def quantiles_ms(samples_ms):
    """返回 P50/P95/P99 并保证单调；输入秒或毫秒都按同一口径处理。"""
    p50 = quantile(samples_ms, 0.50)
    p95 = quantile(samples_ms, 0.95)
    p99 = quantile(samples_ms, 0.99)
    if not (p50 <= p95 <= p99):
        raise ValueError(f"quantiles not monotonic: {p50} {p95} {p99}")
    return {"p50": p50, "p95": p95, "p99": p99, "n": len(samples_ms)}


def rate(count, elapsed_s):
    """单位时间计数；elapsed<=0 视为无效而不是除零。"""
    if elapsed_s <= 0:
        raise ValueError("non-positive elapsed window")
    return count / elapsed_s


def mean(samples):
    return sum(samples) / len(samples)


def self_test():
    """用已知输入验证指标计算逻辑；这是 --self-test 的判定合同。"""
    failures = []

    def check(name, got, want):
        if got != want:
            failures.append(f"{name}: got {got!r}, want {want!r}")

    # 最近秩分位数：1..100 的 P95 是 95（ceil(0.95*100)=95）
    check("quantile p95 1..100", quantile(range(1, 101), 0.95), 95)
    check("quantile p50 1..100", quantile(range(1, 101), 0.50), 50)
    check("quantile p99 1..100", quantile(range(1, 101), 0.99), 99)
    # 单样本：任何分位数都是它自身
    check("quantile single", quantile([42.0], 0.99), 42.0)
    # 最近秩向上取整：5 样本的 P95 = ceil(4.75)=5 → 第 5 个
    check("quantile nearest-rank round-up", quantile([1, 2, 3, 4, 5], 0.95), 5)
    # 未排序输入必须被排序后取值
    check("quantile sorts input", quantile([9, 1, 5], 0.50), 5)
    # 空样本必须报错而不是返回 0
    try:
        quantile([], 0.5)
        failures.append("quantile empty: did not raise")
    except ValueError:
        pass
    # 分位数单调性合同
    q = quantiles_ms([3.0, 1.0, 2.0, 5.0, 4.0] * 20)
    check("quantiles monotonic", q["p50"] <= q["p95"] <= q["p99"], True)
    check("quantiles n", q["n"], 100)
    # 速率：100 个事件 / 20 秒 = 5 events/s；<=0 窗口报错
    check("rate basic", rate(100, 20.0), 5.0)
    try:
        rate(10, 0.0)
        failures.append("rate zero window: did not raise")
    except ValueError:
        pass
    # 均值
    check("mean", mean([1.0, 2.0, 3.0, 4.0]), 2.5)
    # 字节吞吐：2048 bytes / 2 s = 1024 bytes/s
    check("bytes rate", rate(2048, 2.0), 1024.0)

    if failures:
        print("\n".join(failures), file=sys.stderr)
        return 1
    print("metric math self-test: PASS (quantile nearest-rank, monotonicity, rate, mean)")
    return 0


# ---------------------------------------------------------------- HTTP 客户端
class Client:
    def __init__(self, server_url, principal):
        self.base = server_url.rstrip("/")
        self.headers = {"Content-Type": "application/json", "X-Kc-As": principal}

    def call(self, method, path, body=None, timeout=120):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(self.base + path, data=data, headers=self.headers, method=method)
        start = time.perf_counter()
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                payload = json.load(resp)
                return time.perf_counter() - start, resp.status, payload
        except urllib.error.HTTPError as exc:
            try:
                payload = json.load(exc)
            except Exception:
                payload = {"error": {"code": str(exc.code), "message": exc.reason}}
            return time.perf_counter() - start, exc.code, payload

    def repo_seg(self, repository):
        return urllib.parse.quote(repository, safe="")


# ---------------------------------------------------------------- runner
class SmokeRun:
    def __init__(self, env, out_dir: Path):
        self.env = env
        self.out = out_dir
        self.out.mkdir(parents=True, exist_ok=True)
        target = env["target"]
        self.client = Client(target["serverURL"], target["principal"])
        self.catalog = target["catalog"]
        self.store = target.get("managedStore", "")
        self.prefix = target.get("repositoryPrefix", "kc-perf-")
        self.ladders = (env.get("profile") or {}).get("ladders") or {}
        self.expectations = env.get("expectations") or {}
        self.repo = ""
        self.repo_seg = ""
        self.schema_id = "schema/perfbench.v1"
        self.findings = []      # (probe, severity, message)
        self.results = {}       # probe -> metrics dict

    # ---- 基础设施
    def note(self, probe, severity, message):
        self.findings.append({"probe": probe, "severity": severity, "message": message})
        print(f"  [{severity}] {probe}: {message}")

    def flush(self, name, payload):
        (self.out / name).write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")

    def append_samples(self, probe, rows):
        with (self.out / "samples.ndjson").open("a", encoding="utf-8") as fh:
            for row in rows:
                fh.write(json.dumps({"probe": probe, **row}, ensure_ascii=False) + "\n")

    # ---- bind（environment-bound construct）
    def bind(self):
        _, status, ready = self.client.call("GET", "/readyz", timeout=15)
        if status != 200:
            raise SystemExit(f"bind failed: /readyz -> {status}")
        _, status, who = self.client.call("GET", "/identity/v1/whoami", timeout=15)
        principal = who.get("principal", "") if status == 200 else ""
        if principal != self.env["target"]["principal"]:
            raise SystemExit(f"bind failed: whoami={principal!r} != declared principal")
        _, status, health = self.client.call("GET", "/health", timeout=15)
        versions = {"health": health if status == 200 else "absent", "readyz": ready}
        manifest = {
            "kind": "scale-scene-smoke",
            "startedAt": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "target": {"serverURL": self.client.base, "principal": principal, "catalog": self.catalog},
            "versions": versions,
            "profile": self.env.get("profile", {}),
            "ladders": self.ladders,
            "expectations": self.expectations,
            "scope": "upload/object-size/read/browse/index-update smoke subset; not qualification",
        }
        self.flush("manifest.json", manifest)
        print(f"bind: readyz=200 whoami={principal} catalog={self.catalog}")

    # ---- 构造：建仓（scale-repository-empty）
    def create_repository(self):
        self.runstamp = time.strftime('%Y%m%dT%H%M%S', time.gmtime())
        name = f"{self.prefix}{self.runstamp}"
        _, status, resp = self.client.call("POST", "/catalog/v1/repositories", {"name": name, "store": self.store})
        if status != 200:
            raise SystemExit(f"managed create failed: {resp}")
        self.repo = resp["repositoryId"]
        self.repo_seg = self.client.repo_seg(self.repo)
        # 托管建仓不自动登记 Catalog：按场景树 construct 显式 attach（scale-repository-empty）
        _, status, attach = self.client.call(
            "POST",
            f"/catalog/v1/catalogs/{urllib.parse.quote(self.catalog, safe='')}/repositories",
            {"repository": self.repo},
        )
        if status not in (200, 201, 202):
            raise SystemExit(f"attach failed: {attach}")
        _, status, _ = self.client.call("GET", f"/writer/v1/repositories/{self.repo_seg}/head")
        if status != 200:
            raise SystemExit("created repository head not readable")
        print(f"scale-repository-empty: {self.repo} (attached)")

    # ---- 写入原语
    def head_commit(self):
        _, status, head = self.client.call("GET", f"/writer/v1/repositories/{self.repo_seg}/head")
        if status != 200:
            raise SystemExit("head unreadable")
        return head["commit"]

    def commit(self, command_id, operations):
        start = time.perf_counter()
        _, status, resp = self.client.call(
            "POST",
            f"/writer/v1/repositories/{self.repo_seg}/commits",
            {
                # command ledger 在部署级共享：id 必须带每次 run 的唯一段
                "commandId": f"{self.prefix}{self.runstamp}-{command_id}",
                "changeSet": {
                    "targetRepository": self.repo,
                    "targetRef": "",
                    "baseCommit": self.head_commit(),
                    "expectedTargetCommit": self.head_commit(),
                    "operations": operations,
                },
            },
        )
        applied = status == 200 and resp.get("disposition") in ("APPLIED", "REPLAYED")
        commit_id = (resp.get("result") or {}).get("commitId")
        return {"elapsed": time.perf_counter() - start, "startedAt": start, "status": status,
                "disposition": resp.get("disposition"),
                "commitId": commit_id, "raw": resp}

    def doc_operation(self, index, body_bytes=None):
        body = ("payload %08d " % index) * max(1, body_bytes // 16) if body_bytes else f"payload {index:08d}"
        value = {"entity": "BenchDoc", "aspect": "doc",
                 "title": f"doc {index:08d}", "body": body}
        return {"op": "PUT",
                "address": {"kind": "Entity", "objectId": f"bench/doc-{index:08d}"},
                "value": value, "schemaRef": self.schema_id}

    # ---- 构造：schema + 批量导入（bulk-loaded construct 证据）
    def build_baseline(self, total_docs, batch):
        schema_value = {"entity": "BenchDoc", "pattern": "record",
                        "fields": {"title": {"type": "string", "required": True, "access": ["filter"]},
                                   "body": {"type": "string", "required": True, "access": ["text"]}}}
        result = self.commit(f"{self.prefix}schema-1",
                             [{"op": "PUT", "address": {"kind": "Entity", "objectId": self.schema_id},
                               "value": schema_value}])
        if result["disposition"] != "APPLIED":
            raise SystemExit(f"schema commit failed: {result['raw']}")
        self.next_doc = 0
        batches = []
        start = time.perf_counter()
        while self.next_doc < total_docs:
            take = min(batch, total_docs - self.next_doc)
            ops = []
            first = self.next_doc
            for _ in range(take):
                ops.append(self.doc_operation(self.next_doc))
                self.next_doc += 1
            r = self.commit(f"{self.prefix}bulk-{first:08d}", ops)
            if r["disposition"] != "APPLIED":
                raise SystemExit(f"bulk commit failed: {r['raw']}")
            batches.append({"firstDoc": first, "files": take, "commitSeconds": r["elapsed"]})
        wall = time.perf_counter() - start
        self.append_samples("bulk-loaded.construct", batches)
        metrics = {
            "cap.repo_objects": total_docs + 1,
            "write.upload.files_per_second": rate(total_docs, wall),
            "write.commit.latency": quantiles_ms([b["commitSeconds"] * 1000 for b in batches]),
        }
        self.results["bulk-loaded.construct"] = metrics
        print(f"bulk-loaded construct: {total_docs} docs in {len(batches)} commits, "
              f"{metrics['write.upload.files_per_second']:.1f} files/s")

    # ---- 探针：多文件上传曲线（probe-upload-filecount-curve）
    def probe_upload_curve(self):
        ladder = self.ladders.get("uploadFileCount") or [1, 10, 100]
        points = []
        for n in ladder:
            ops = []
            first = self.next_doc
            for _ in range(n):
                ops.append(self.doc_operation(self.next_doc))
                self.next_doc += 1
            cmd = f"{self.prefix}upload-{first:08d}-n{n}"
            result = self.commit(cmd, ops)
            if result["disposition"] != "APPLIED":
                self.note("probe-upload-filecount-curve", "FAIL", f"commit n={n}: {result['raw']}")
                return
            full_cmd = f"{self.prefix}{self.runstamp}-{cmd}"
            receipt_elapsed, status, _ = self.client.call("GET", f"/writer/v1/receipts/{full_cmd}")
            points.append({"files": n, "commitMs": result["elapsed"] * 1000,
                           "firstDoc": first, "commandId": full_cmd, "rawCommandId": cmd,
                           "receiptOk": status == 200,
                           "receiptMs": receipt_elapsed * 1000, "commitId": result["commitId"],
                           "operations": ops})
        # 幂等：用同一 ChangeSet 重放最后一个 commandId，必须 REPLAYED 且 commit 不变
        # （同 id 异 digest 是 IDEMPOTENCY_CONFLICT，所以重放必须原样重发 operations，
        # 且 commandId 与首次提交完全一致——commit() 只对 rawCommandId 加 run 前缀）
        last = points[-1]
        replay = self.commit(last["rawCommandId"], last["operations"])
        replay_ok = replay["disposition"] == "REPLAYED" and replay["commitId"] == last["commitId"]
        if not replay_ok:
            self.note("probe-upload-filecount-curve", "FAIL",
                      f"replay changed commit: {replay['disposition']} {replay['commitId']}")
        # 计数对账：本探针共上传 sum(ladder) 个文档，回读计数必须一致
        expect_docs = sum(p["files"] for p in points)
        self.append_samples("probe-upload-filecount-curve", points)
        curve = {str(p["files"]): {"commitMs": p["commitMs"],
                                   "filesPerSecond": rate(p["files"], p["commitMs"] / 1000)} for p in points}
        self.results["probe-upload-filecount-curve"] = {
            "write.commit.latency": quantiles_ms([p["commitMs"] for p in points]),
            "write.upload.files_per_second_by_point": curve,
            "write.duplicate_extra_revision": 0 if replay_ok else 1,
            "correctness.count_uploaded": expect_docs,
        }
        print(f"upload curve: {json.dumps(curve)}")

    # ---- 探针：对象大小曲线（probe-object-size-curve）
    def probe_object_size_curve(self):
        ladder = self.ladders.get("objectSizeBytes") or [1024, 65536, 1048576]
        points = []
        for size in ladder:
            index = self.next_doc
            self.next_doc += 1
            result = self.commit(f"{self.prefix}size-{size}",
                                 [self.doc_operation(index, body_bytes=size)])
            ok = result["disposition"] == "APPLIED"
            points.append({"sizeBytes": size, "ok": ok, "commitMs": result["elapsed"] * 1000,
                           "bytesPerSecond": rate(size, result["elapsed"]) if ok else 0.0,
                           "objectId": f"bench/doc-{index:08d}"})
            if not ok:
                self.note("probe-object-size-curve", "FAIL", f"size={size}: {result['raw']}")
        self.append_samples("probe-object-size-curve", points)
        ok_points = [p for p in points if p["ok"]]
        self.results["probe-object-size-curve"] = {
            "write.object_size.max_ok": max((p["sizeBytes"] for p in ok_points), default=0),
            "write.upload.bytes_per_second_by_point": {str(p["sizeBytes"]): p["bytesPerSecond"] for p in points},
            "write.error_rate": 1 - len(ok_points) / len(points) if points else 1.0,
        }
        print("object-size curve: " + json.dumps({str(p["sizeBytes"]): round(p["bytesPerSecond"]) for p in points}))

    # ---- 投影追平（projection-synced construct）
    # 优先走 operations 同步面显式收敛（这是该面的用途）；后台 reconcile tick
    # 在本部署实测会让新仓投影 5 分钟都不就绪，而一次同步收敛约 2.5 分钟。
    # 同步墙钟时间登记为 index.catchup.sync_wall_ms。
    def converge_projection(self, expected_docs, timeout_s=600):
        start = time.monotonic()
        deadline = start + timeout_s
        last_err = ""
        while time.monotonic() < deadline:
            _, status, desc = self.client.call("POST", "/operations/v1/projections:sync",
                                               {"repository": self.repo}, timeout=timeout_s)
            if status == 200:
                wall = (time.perf_counter() - start) * 1000
                return {"wall_ms": wall, "basis": desc.get("basisCommit", ""), "syncs": 1}
            last_err = json.dumps((desc or {}).get("error", {}), ensure_ascii=False)[:160]
            time.sleep(1.0)
        return {"wall_ms": None, "error": last_err or "sync timeout"}

    # ---- 探针：点读曲线（probe-read-latency-curve）
    def probe_read_latency(self, samples=60):
        hot = [f"bench/doc-{i:08d}" for i in range(min(8, self.next_doc))]
        rows = []
        for i in range(samples):
            obj = hot[i % len(hot)] if i % 4 == 0 else f"bench/doc-{(i * 7) % self.next_doc:08d}"
            elapsed, status, _ = self.client.call("POST", "/knowledge/v1/objects:read",
                                                  {"repository": self.repo, "object": obj})
            rows.append({"object": obj, "ok": status == 200, "ms": elapsed * 1000})
        self.append_samples("probe-read-latency-curve", rows)
        ok_ms = [r["ms"] for r in rows if r["ok"]]
        self.results["probe-read-latency-curve"] = {
            "read.point.latency": quantiles_ms(ok_ms) if ok_ms else {"p50": None, "p95": None, "p99": None, "n": 0},
            "write.error_rate": 1 - len(ok_ms) / len(rows),
        }
        print(f"read latency: p50={quantile(ok_ms, .5):.1f}ms p95={quantile(ok_ms, .95):.1f}ms p99={quantile(ok_ms, .99):.1f}ms")

    # ---- 写突发后 live HEAD 车道的 search 可用性。实测（本地部署）：
    #      a) operations sync 只收敛"钉住的 serving basis"（~2.5min 墙钟），
    #         不让 live HEAD 车道立即可 search；
    #      b) live 车道就绪由该仓的下一次内容唤醒驱动（写入后 ~7-17s），
    #         而全局 15s reconcile tick 在本部署被无关仓的不可达 Bound State
    #         序列化饿死（applyPending 串行、遇错即返回）。
    #      runner 先被动等一个 tick，再发一次最小内容唤醒（公开面合法操作），
    #      恢复耗时如实登记。
    def wait_search_available(self, timeout_s=120):
        start = time.monotonic()
        deadline = start + timeout_s
        while time.monotonic() < deadline:
            _, status, _ = self.client.call("POST", "/knowledge/v1/search",
                                            {"repository": self.repo, "query": "payload", "limit": 1})
            if status == 200:
                return (time.monotonic() - start) * 1000
            time.sleep(0.5)
        return None

    # ---- 探针：浏览分页曲线（probe-browse-list-curve）
    def probe_browse(self):
        wake_used = False
        recovery_ms = self.wait_search_available(timeout_s=20)
        if recovery_ms is None:
            index = self.next_doc
            self.next_doc += 1
            wake = self.commit(f"{self.prefix}wake-{index:08d}",
                               [self.doc_operation(index, body_bytes=512)])
            if wake["disposition"] != "APPLIED":
                self.note("probe-browse-list-curve", "INVALID",
                          f"live-lane wake commit failed: {wake['raw']}")
                self.results["probe-browse-list-curve"] = {"index.recovery_after_burst_ms": None}
                return
            wake_used = True
            recovery_ms = self.wait_search_available(timeout_s=120)
        if recovery_ms is None:
            self.note("probe-browse-list-curve", "INVALID", "search did not become available after wake")
            self.results["probe-browse-list-curve"] = {"index.recovery_after_burst_ms": None}
            return
        timed = []
        elapsed, st, _ = self.client.call("POST", "/knowledge/v1/schemas:list", {"repository": self.repo})
        if st == 200:
            timed.append(elapsed * 1000)
        pages = 0
        continuation = ""
        while pages < 5:
            body = {"repository": self.repo, "query": "payload", "limit": 10}
            if continuation:
                body["continuation"] = continuation
            elapsed, st, page = self.client.call("POST", "/knowledge/v1/search", body)
            pages += 1
            if st == 200:
                timed.append(elapsed * 1000)
            continuation = (page or {}).get("continuation") or ""
            if not continuation:
                break
        self.append_samples("probe-browse-list-curve",
                            [{"pages": pages, "recoveryMs": recovery_ms, "wakeUsed": wake_used}])
        self.results["probe-browse-list-curve"] = {
            "index.recovery_after_burst_ms": recovery_ms,
            "live_lane_needed_content_wake": 1 if wake_used else 0,
            "read.browse.list_latency": quantiles_ms(timed) if timed else {"n": 0},
            "pages_observed": pages,
        }
        p50 = quantile(timed, .5) if timed else float("nan")
        mode = "wake" if wake_used else "passive"
        print(f"browse: recovery({mode}) {recovery_ms:.0f}ms, {pages} pages, p50={p50:.1f}ms")

    # ---- 探针：索引更新时延（probe-index-update-latency-curve）
    def probe_index_update_latency(self):
        ladder = self.ladders.get("commitBatchFiles") or [1, 10]
        points = []
        for batch in ladder:
            ops = []
            first = self.next_doc
            for _ in range(batch):
                ops.append(self.doc_operation(self.next_doc, body_bytes=2048))
                self.next_doc += 1
            cmd = f"{self.prefix}idx-{first:08d}-b{batch}"
            result = self.commit(cmd, ops)
            if result["disposition"] != "APPLIED":
                self.note("probe-index-update-latency-curve", "FAIL", f"batch={batch}: {result['raw']}")
                continue
            marker = f"payload {first:08d}"
            deadline = time.time() + 30
            visible = None
            last_error = ""
            while time.time() < deadline:
                _, status, sr = self.client.call("POST", "/knowledge/v1/search",
                                                 {"repository": self.repo, "query": marker, "limit": 10})
                if status != 200:
                    # 投影重建窗口内 search 可能瞬态失败；记录并重试
                    last_error = json.dumps((sr or {}).get("error", {}), ensure_ascii=False)[:160]
                    time.sleep(0.5)
                    continue
                hits = sr.get("hits") or []
                if any(h.get("knowledge", {}).get("knowledgeRef", {}).get("object", "").endswith(f"doc-{first:08d}") for h in hits):
                    visible = time.perf_counter()
                    break
                time.sleep(0.25)
            if visible is None:
                self.note("probe-index-update-latency-curve", "WARN",
                          f"batch={batch}: not visible within 30s (last search error: {last_error or 'no hits'})")
                points.append({"batch": batch, "visibleMs": None})
            else:
                # commit→可见：从 commit 请求发出时刻起算（start 记录于 commit()）
                points.append({"batch": batch, "visibleMs": (visible - result["startedAt"]) * 1000})
        self.append_samples("probe-index-update-latency-curve", points)
        ok = [p["visibleMs"] for p in points if p["visibleMs"] is not None]
        self.results["probe-index-update-latency-curve"] = {
            "index.update.commit_to_visible": quantiles_ms(ok) if ok else {"n": 0},
            "by_batch": {str(p["batch"]): p["visibleMs"] for p in points},
        }
        print(f"index update: {json.dumps({str(p['batch']): (round(p['visibleMs']) if p['visibleMs'] else None) for p in points})} ms")

    # ---- 计数与摘要判定
    def verify_counts(self):
        _, status, desc = self.client.call("POST", "/operations/v1/projections:describe", {"repository": self.repo})
        # 投影 objectCount 只数可检索知识文档，不含 schema 对象（与 projection controller 口径一致）
        expected = self.next_doc
        if status != 200:
            self.note("counts", "INVALID", "projection describe unavailable")
            return None
        observed = desc.get("objectCount")
        ok = observed == expected
        self.note("counts", "OK" if ok else "FAIL", f"projection objectCount={observed} expected={expected}")
        return {"expected": expected, "observed": observed}

    # ---- 清理（cleanup 合同）
    def cleanup(self):
        mode = (self.env.get("cleanup") or {}).get("scratchRepositories", "delete")
        if mode != "delete":
            self.note("cleanup", "WARN", f"scratchRepositories={mode}; scratch repo retained: {self.repo}")
            return
        _, status, _ = self.client.call(
            "DELETE", f"/catalog/v1/catalogs/{urllib.parse.quote(self.catalog, safe='')}/repositories/{self.repo_seg}")
        self.note("cleanup", "OK" if status in (200, 202, 204) else "WARN",
                  f"detach scratch repo -> {status}（介质侧物理仓留在部署方，公开面无物理删除）")

    # ---- 汇总
    def evaluate(self):
        report = {
            "kind": "scale-scene-smoke-report",
            "runDir": str(self.out),
            "results": self.results,
            "findings": self.findings,
        }
        expectations = self.expectations or {}
        checks = []
        up = self.results.get("probe-upload-filecount-curve", {}).get("write.commit.latency", {})
        if up.get("p99") is not None:
            checks.append({"metric": "write.commit.latency.p99", "observed": up["p99"],
                           "bound": expectations.get("uploadCommitP99Ms", 5000),
                           "ok": up["p99"] <= expectations.get("uploadCommitP99Ms", 5000)})
        rd = self.results.get("probe-read-latency-curve", {}).get("read.point.latency", {})
        if rd.get("p99") is not None:
            checks.append({"metric": "read.point.latency.p99", "observed": rd["p99"],
                           "bound": expectations.get("readP99Ms", 1000),
                           "ok": rd["p99"] <= expectations.get("readP99Ms", 1000)})
        ix = self.results.get("probe-index-update-latency-curve", {}).get("index.update.commit_to_visible", {})
        if ix.get("p99") is not None:
            bound = expectations.get("indexVisibleP99Ms", 30000)
            checks.append({"metric": "index.update.commit_to_visible.p99", "observed": ix["p99"],
                           "bound": bound, "ok": ix["p99"] <= bound})
        rec = self.results.get("probe-browse-list-curve", {}).get("index.recovery_after_burst_ms")
        if rec is not None:
            bound = expectations.get("searchRecoveryAfterBurstMs", 90000)
            checks.append({"metric": "index.recovery_after_burst_ms", "observed": rec,
                           "bound": bound, "ok": rec <= bound})
        sync_wall = self.results.get("projection-synced.construct", {}).get("index.catchup.sync_wall_ms")
        if sync_wall is not None:
            bound = expectations.get("indexCatchupSyncWallMs", 420000)
            checks.append({"metric": "index.catchup.sync_wall_ms", "observed": sync_wall,
                           "bound": bound, "ok": sync_wall <= bound})
        dup = self.results.get("probe-upload-filecount-curve", {}).get("write.duplicate_extra_revision", 0)
        checks.append({"metric": "write.duplicate_extra_revision", "observed": dup, "bound": 0, "ok": dup == 0})
        hard = [f for f in self.findings if f["severity"] == "FAIL"]
        invalid = [f for f in self.findings if f["severity"] == "INVALID"]
        overall = "PASSED"
        if invalid:
            overall = "INVALID"
        elif hard or any(not c["ok"] for c in checks):
            overall = "FAILED"
        report["expectationChecks"] = checks
        report["overall"] = overall
        self.flush("report.json", report)
        return report

    def run(self):
        self.bind()
        self.create_repository()
        try:
            self.build_baseline(total_docs=200, batch=50)
            self.probe_upload_curve()
            self.probe_object_size_curve()
            conv = self.converge_projection(expected_docs=self.next_doc)
            self.results["projection-synced.construct"] = {
                "index.catchup.sync_wall_ms": conv.get("wall_ms"),
                "basis": conv.get("basis"),
            }
            if conv.get("wall_ms") is None:
                self.note("projection-synced", "INVALID",
                          f"projection sync did not converge: {conv.get('error')}")
            else:
                self.note("projection-synced", "OK",
                          f"sync wall {conv['wall_ms']/1000:.0f}s basis={conv.get('basis', '')[:12]}")
            self.probe_read_latency()
            self.probe_browse()
            self.probe_index_update_latency()
            counts = self.verify_counts()
            if counts:
                self.results["counts"] = counts
        finally:
            self.cleanup()
        report = self.evaluate()
        print(f"overall: {report['overall']}")
        return 0 if report["overall"] == "PASSED" else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--env", help="environment config (env.example.yaml shape)")
    parser.add_argument("--out", help="evidence dir override (default runs/<run-id>)")
    parser.add_argument("--self-test", action="store_true", help="verify metric math against known inputs")
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
    if scale not in SCALES:
        print(f"profile.scale must be one of {SCALES}", file=sys.stderr)
        return 2
    run_id = (env.get("run") or {}).get("runId") or ""
    if not run_id or run_id == "auto":
        run_id = time.strftime("%Y%m%dT%H%M%SZ", time.gmtime())
    evidence = (env.get("run") or {}).get("evidenceDir") or "../runs"
    out_dir = Path(args.out) if args.out else (env_path.resolve().parent / evidence / f"smoke-{run_id}").resolve()
    return SmokeRun(env, out_dir).run()


if __name__ == "__main__":
    sys.exit(main())
