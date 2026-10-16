# DEBT-SHARED-SEARCH-CLEANUP-01 · 共享搜索测试实例的容量与夹具清理

## 债

- 位置：`scripts/testsuite.sh` 的共享 OpenSearch 复用入口、场景投影夹具和共享测试实例。
- 信号：2026-10-15 复用现有本地测试实例执行 `make test`，索引创建返回 HTTP 400：已有 1000/1000 个开放分片，无法新增 2 个分片；记录见 `.validation/runs/20261015T095823Z-9fe4ee9ed7e3/report.json`。
- 判定：共享测试容量已耗尽，未证明这次用例泄漏全部遗留分片；不是访问证据修复导致的领域失败。`ACCEPT-HARNESS-01` 已要求夹具按用例清理，这里保留本次具体容量证据。
- 处置：核对夹具资源所有权和退出清理；共享实例复用前明确容量条件，测试仅清理本次拥有的资源，不通过删共享索引或提高上限掩盖问题。本轮用现有镜像的独立临时测试流程复跑。
- 触发条件：再次复用共享搜索实例跑场景，或扩大场景规模前。

## 目标

共享搜索测试能持续运行，既有业务与其它用例资源不受清理影响。

## 依据

- [测试与验证](../../doc/engineering/testing.md)。
- [工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 重复运行场景后，本次用例拥有的索引及分片恢复到运行前水平。
- 失败与中断清理不删除其它命名空间；容量不足的复用入口有明确诊断。
- 相关夹具回归、`make check-docs`、`make quality` 通过。

## 前置

与 `ACCEPT-HARNESS-01` 的夹具清理工作协同。

## 不做

不清空现有共享测试集群，不改变部署资源限额。

## 交付

### 实现范围（2026-10-15，本轮未完成）

- 新增 `scripts/opensearch_fixture.py` 及离线回归：复用前按 eligible data nodes 与开放索引 primary×(replicas+1) 计容量，包含初始化/未分配副本，不用 active shard 数误判；不足时给明确诊断，不删共享资源、不提高阈值。
- 每次包装运行生成 `kc-test-<32 hex>-`，子进程退出/失败/中断后枚举该前缀并逐个精确删除；SIGTERM 有界宽限后结束忽略 TERM 的本次子进程组，处理 Popen 前信号窗口。离线测试自身超时也只结束其创建的进程组。
- 仅提供环境变量不代表索引归属成立。已有脏文件的待应用补丁 `shared-search-integration.patch` 同时修改 `scripts/testsuite.sh` 包装入口和 `cli/scene_projection_namespace_test.go`：后者校验完整 RUN_PREFIX、拼接每个 namespace 的随机后缀，其代理对请求路径、批量正文与 clone 目标统一加前缀。尚未授权应用，所以真实 `make test` 索引当前没有经该入口完成贯通。
- 补丁还将 clone 目标在发送前加入资源集合，覆盖“创建已成功但响应丢失”的清理路径；该 Go 路径定向回归与真实重复运行尚未完成，不能把 Python 假 HTTP 的清理测试替代它。

### 已验证范围

- `scripts/opensearch_fixture_test.py` 5 条、`scripts/accept_search_resources_test.py` 2 条离线回归全绿且无 skip，见 `.validation/runs/20261015T120400Z-accept-harness/final-0.log`。容量/TERM review 红绿分别在 `search-review-red.log`、`search-review-green.log`；验收清理红绿在 `accept-search-red.log`、`accept-search-green.log`。
- 只证明标准库辅助器与精确所有权规则，不证明真实共享 OpenSearch 重复运行前后分片恢复；本轮没有删除任何现有共享索引，没有改变限额，也没有进行部署数据清理。
- Make/Go 入口集成待授权；`make test`、完整架构组与全量质量门未在本线独立运行。协同验收只读预检因现有 Server 503 阻断，117 项均未执行，见 `.validation/runs/20261015T125526Z-8f9962049123/`。任务保持 doing，不能宣称共享实例持续运行验收通过。

## 本轮最终统一门禁（2026-10-15）

统一证据与命令见 门禁索引（`.validation/runs/20261015T120241Z-task-orchestration/final-gates.json`）。最新 `make quality`（含 check-docs）通过；完整架构 648 pass/1 fail/0 skip（术语，归 DEBT-TERMINOLOGY-TRIAL-01）；`make test` 18 pass/2 fail/0 skip（同一 Dataset 文本能力场景及父级，归 INDEX）；`make test-contracts` 的 component 为 2349 pass/67 fail/6 skip，boundary/e2e 未启动。上述四次均源码稳定；合同失败分组见 报告（`.validation/runs/20261015T135236Z-03378168af4a/findings.md`），分别归 INDEX、库存消费债及回环 HTTP 资源调查，不由本任务替其它线修复。

最终相关定向结果及真实部署边界分别保留在任务专项证据与门禁索引：定向通过不替代完整入口，现有 local 栈 readyz 503，117 项真实验收均 error/not_started。全量门禁尚未全绿，依本轮约定保留 doing；未提交或安装依赖，未降低检查阈值或重置已有环境。

## 2026-10-16 追加交付

### 实现

- 已把包装入口落入 `scripts/testsuite.sh`：lakeFS 组先由容量/清理辅助器分配本次完整前缀，再进入实际 Go 场景。`cli/scene_projection_namespace_test.go` 校验 RUN_PREFIX，并保留每个世界的随机后缀；请求路径、bulk action 元数据和 clone 目标贯通该所有权。
- clone 目标在发请求前登记；即使上游创建后返回 502，目标仍被精确清理。保留原 payload/PIT/查询不改的回归与旧冻结副本逻辑。
- 新增 Go 所有权和 clone 失败反例，先出现实际失败后修绿；新增实际 testsuite 脚本回归，证明运行前缀进入模拟测试创建的物理索引、测试退出 73 原样传播、只删除该索引并恢复原集合。

### 验证与剩余

- `namespace-integration-red.log` → `namespace-integration-final.jsonl`（3 项 pass）；`testsuite-wrapper-red.log` → `testsuite-wrapper-green.log`（6 项 pass）；均无 skip，不把假 HTTP 回归替代真实 OpenSearch 持续运行证据。
- 真实共享实例只读容量检查为 999/1000 个开放分片，余 1，低于入口所需至少 8；见 `shared-capacity.log`。明确诊断后不启动测试、不删除遗留索引、不调整上限。受此阻断，本轮未进行共享实例的重复场景与分片恢复验收。
- 证据目录 `.validation/runs/20261016T012005Z-accept-harness`；源码写入完成后交由主线在稳定窗口统一运行全量门禁，本线没有运行完整架构组、`make quality` 或 `make test`，没有移 done。

## 关闭

2026-10-16 并入 `ACCEPT-HARNESS-01`：位置、信号与剩余验收在该任务的“并入”一节继续推进。本文件已有的交付与证据保留为历史，不代表验收通过。
