# DEBT-OBJECT-HISTORY-TRUNCATION-01 · 修复长历史截断与伪引入点

## 债

- 位置：`snapshot/internal/lakefsimpl` 的 CommitHistory；`object/reader/repository_history.go` 的 Log 与 missingStatus。
- 信号：lakeFS 只返回一个物理页，Reader 固定取最近 10000 个提交；窗口尾部被当作真实引入点，早期删除还可能被解释成从未存在。
- 判定：现有历史读取的正确性缺陷，与新增版本比较接口无关。
- 处置：lakeFS 遵守请求总条数并跟随服务端分页；Reader 分块连续访问固定 first-parent 链，只在状态变化或真实终点证明后返回引入点。
- 触发条件：用户明确要求先修复并运行测试，本轮独立处理。

## 目标

超过 lakeFS 单页和原 10000 提交窗口的历史不漏修订、不伪造引入点；删除与重建、A→B→A 和修订续页保留真实变化。

## 依据

- [对象版本与读写](../../doc/design/object-versioning.md)：历史与连续扫描。
- [Snapshot](../../doc/design/snapshot.md)：不可变提交与版本复核。
- [工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 改前新增回归复现单页截断、错误引入点、窗口外删除遗漏以及无关旧故障阻塞已证明首页。
- lakeFS 分页保留 first-parent、顺序与请求总上限；坏游标、无进展、后续页错误与取消不返回部分成功。
- 超过 10000 提交的 Reader 回归返回真实引入点，逐页不重复不漏项，删除状态与历史终点正确。
- 受影响回归、`make check-docs`、`make quality`、`make test`、`make test-contracts` 执行并记录准确结果；无 skip 才归档。

## 前置

无。`OBJECT-VERSIONING-01` 的存量历史正确性部分独立拆出，不启动其它版本接口的实现。

## 不做

不新增公开游标格式、扫描预算结果或状态摘要接口，不实现 head 缓存或历史加速索引；
有界扫描预算与可携带未完成状态段的分页仍由 `OBJECT-VERSIONING-01` 承接。
本轮保持现有修订分页合同，扫描服从请求上下文，按需在证明足够修订后停止。

## 改前证据

新增 Reader 四项回归与 lakeFS 分页回归在修改实现前均复现预期失败：
真实引入 c00006/c00005 被误报为 c00021、早期删除遗漏、旧读取错误阻塞首页；
请求 2100 个提交仅返回 1000，后续页错误与坏游标被忽略。原始日志保留到本轮 `.validation/` 证据中。

## 实现复核

lakeFS 跟随物理分页直至请求总量或真实终点，拒绝缺失、重复和循环游标；
后续页失败或取消不返回部分成功。Reader 共用分块遍历器承接 LOG 与缺失状态判定，
证明相邻值或声明状态变化后才输出引入修订；真实根前的缺失不冒充删除。
返回条数满足后停止更老的正文读取。短提供方批次仍从末尾固定提交继续探测，
不能因批次较短就认定历史结束，相关新反例先红后绿。

包回归覆盖 >10000 提交、修订分页、A→B→A、删除重建、真实根引入、声明独立变化、
只取首页不访问无关旧故障、坏分页与后续页取消。未新增公开请求或响应字段。
有界扫描预算和可恢复未完成状态段仍在 `OBJECT-VERSIONING-01`；
本轮不把正确遍历写成性能优化或真实部署容量结论。

## 交付（2026-10-20）

实现范围：已修复 lakeFS 单页截断、Reader 10000 提交窗口、短批次伪终点、窗口外删除误判，
保持值与声明的真实引入修订及既有修订分页。十个本项实现、测试和 README 文件均与固定验证副本哈希一致。

已验证：新增回归先红后绿；完整 Reader/lakeFS 包回归与三个 CLI 历史消费旅程通过；
固定副本的 `make quality`（含 check-docs）通过；`make test` 18 pass / 0 fail / 0 skip，
`make test-contracts` 5142 pass / 0 fail / 14 外部 live skip，54/54 公开命令的端到端边界覆盖通过。
两轮完整测试 sourceChanged=false。副本起初缺忽略的 Python 环境，公开练习启动失败；复用原工作区
既有 `.venv` 后该用例和最终完整契约通过，未安装依赖，失败尝试不拼入最终通过结果。

红绿日志、冻结清单与完整报告保留于 `.validation/runs/20261020-long-history/`。
未运行真实部署或规模压测，不作容量/吞吐结论；未提交。缺陷已修复，因完整契约仍有外部 live skip，
任务按无 skip 归档规则保留 doing。预算与新增游标机制仍由 `OBJECT-VERSIONING-01` 承接。

## 交付（2026-10-21）

实现已在工作区落地并经当前源码复核（`object/trusted_schema.go` 的一致性检查覆盖
`TrustedSchemaOperations()` 完整集合；历史授权每个来源读取前单独检查 `object.history.read`；
未应用写入证明经统一错误信封传递）。

当前源码统一门禁：`make check-docs` PASS、`make quality` 通过
（`units=86 propagation=13.69%`，ratchet held）、`make test` 0 FAIL / 0 SKIP、
`make test-contracts` 0 FAIL / 18 SKIP，e2e `53/53 successful`。

18 项 SKIP 经逐条核对全部为既有环境分级项（lakeFS 独立部署、太湖认证、Luna 重排、
HTTP state runtime 独立栈、规模与吞吐），不是本项遗留的断言缺口。
按既定判断，环境分级跳过不计入本项阻塞。
