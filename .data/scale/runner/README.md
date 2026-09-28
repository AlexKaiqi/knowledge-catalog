# Scale smoke runner

`smoke.py` 是 scenes/ 用例树的**可运行性验证**，不是压测，也不是资格结论：

- 只绑定 env 配置指向的已部署 KC Server（`/readyz` 必须先就绪）；**不启动、
  不编排任何容器**。本地被测环境怎么起（Gitea 权威 / lakeFS 部署）由部署方
  脚本负责，与 runner 无关。
- 只经公开 HTTP surface：`/catalog/v1`（托管建仓）、`/writer/v1`（commit/receipt/
  head）、`/knowledge/v1`（read/search/schemas:list）、`/operations/v1`
  （projections:describe）。
- 执行探针子集（量级阶梯远小于资格档）：
  `bulk-loaded.construct`、`probe-upload-filecount-curve`（含 command-id 幂等
  重放检查）、`probe-object-size-curve`、`probe-read-latency-curve`、
  `probe-browse-list-curve`、`probe-index-update-latency-curve`
  （commit→search 可见轮询）。
- 指标口径引用 `scenes/metrics.yaml` 的 id；计算逻辑（最近秩分位数、单调性、
  速率）由 `--self-test` 用已知输入验证。
- 结果写 `<evidenceDir>/smoke-<runId>/`：`manifest.json`、`samples.ndjson`、
  `report.json`（含 `expectationChecks` 与 `PASSED/FAILED/INVALID` 总体判定）。
  smoke 期望值来自 env 配置 `expectations:`，是本地冒烟阈值，**不是**
  `docs/reviewed/scale-benchmark.md` 资格线。

## 运行

```bash
python3 .data/scale/runner/smoke.py --self-test
python3 .data/scale/runner/smoke.py --env <env.yaml>
```

env 配置沿用 [`../scenes/env.example.yaml`](../scenes/env.example.yaml) 的形状；
真实配置不提交。本仓库未实现完整 load runner（到达模型、长窗口、故障注入等
能力缺口见 `scenes/README.md` §4 与各探针 `requires`）。

## Ladder runner（L1）

`ladder.py` 是第一个真阶梯执行器：执行 `probe-upload-filecount-curve` 的
`uploadFileCount × transport` 配对（staging=presigned 三次往返，bulk=对象上传
单次往返，ChangeSet `bulkIngest` 显式 opt-in），每测量点独立 scratch 仓、全量
回读对账、duplicate command-id 重放检查，报告同阶梯点 bulk/staging 配对增益。

- 边界：closed-loop 串行（concurrency=1）；只覆盖 client-timer 可测子集；
  发压端 CPU 采样仅 runner 进程自身（`os.times`，机器级 resource-sample 归 L2）；
  门槛重登记前不做资格判定，只出曲线与正确性对账。
- bulk 配对点的有效性依赖被测 Server 构建包含 LAKEFS-04。实测旧构建对
  `bulkIngest` 是**响亮拒绝**（strict decode → `USAGE_INVALID`），不是静默
  忽略——报告 FAILED 即指向构建过期，`manifest.json` 存档 `/health` 原文。
- 证据写 `<evidenceDir>/ladder-<runId>/`：`manifest.json`（CASES §3 参数集，
  缺项显式标注）、`samples.ndjson`（逐点原始行）、`report.json`（曲线、配对
  增益、正确性检查、PASSED/FAILED/INVALID）。

```bash
python3 .data/scale/runner/ladder.py --self-test
python3 .data/scale/runner/ladder.py --env <env.yaml> [--max-files 100]
```

## 冒烟期观察（本地部署实测，非资格结论）

- **投影收敛语义**：`POST /operations/v1/projections:sync` 把一个仓收敛成
  *钉住的 serving basis*（ pinned 记录，本地 ~325 文档实测墙钟 3.5–4 分钟）；
  live HEAD 车道的可检索性由该仓的下一次内容唤醒驱动（写入后秒级），多数
  情况下 sync 返回后下一个 reconcile tick 也会完成。冒烟报告把两者分开登记：
  `index.catchup.sync_wall_ms` 与 `index.recovery_after_burst_ms`。
- **objectCount 口径**：`projections:describe` 的 objectCount 只数可检索知识
  文档，不含 schema 对象。
- **部署卫生**：全局 reconcile 的 `applyPending` 串行执行且遇错即返回；一个带
  不可达 Bound State origin 的残留仓会让其它仓的后台追平被饿死（本机冒烟环境
  早期探针残留触发过）。压测环境不应留不可达的 Bound State schema。
- **清理语义**：公开面只有 detach（catalog 成员移除）；介质侧 Gitea 物理仓与
  OpenSearch 投影索引留在部署方，由部署方生命周期负责。

