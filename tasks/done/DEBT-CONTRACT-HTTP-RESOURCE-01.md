# DEBT-CONTRACT-HTTP-RESOURCE-01 · 合同回环 HTTP 资源失败的诊断与可靠执行

## 债

- 位置：`scripts/testsuite.sh` 的 component 执行及其回环 HTTP 夹具；信号涉及 `internal/testkit` 的 lakeFS HTTP 夹具与 `home`、`server` 的 httptest 请求，不预先指定哪一处有缺陷。
- 信号：完整 `make test-contracts` 在固定源码状态运行时，31 个叶测试的输出直接报 `dial tcp 127.0.0.1:<port>: connect: can't assign requested address`；连同父级为 35 个失败事件。失败跨管理仓、系统仓、文件审计、身份与插件说明多个功能。证据 `.validation/runs/20261015T135236Z-03378168af4a/`，源码未变。
- 判定：合同验证可靠性与资源诊断债。错误由回环连接建立报告，但未证明端口耗尽、描述符不足、连接生命周期缺陷、并行规模或外部进程中的任一项是根因；不能把它认定为已证实的纯环境故障或产品实现缺陷。
- 处置：在保持断言、测试选择和系统资源限制不变的前提下，建立失败窗口的并发进程、监听/连接状态、端口及描述符峰值证据；对明确失败样本做隔离与原并发合同对照，按证据区分运行环境、夹具生命周期、HTTP 运输或应用实现，修复真正拥有该问题的层。已有 INDEX 与 Inventory 断言失败仍归原任务。
- 触发条件：再次执行完整 component/contracts，或回环 HTTP 资源分配错误复现时；观察与采样不终止未经授权的外部进程。

## 目标

完整 component 合同能够可靠执行；出现回环连接资源错误时，有足够证据定位来源并确定正确处置，不能用模糊“环境抖动”掩盖代码问题，也不能把一次压力下失败当成每个功能独立回归。component 失败后的 boundary/e2e 未启动范围明确报告。

## 依据

- [测试与验证](../../doc/engineering/testing.md)。
- [工程质量闭环](../../doc/engineering/quality.md)。

## 验收

按复现与否二选一结束：

- 复现：对资源错误的复现与对照都保留固定源码、命令、并发进程及失败窗口资源证据；明确根因与修复层，不只保存事后截面。修复后先前 31 个明确连接失败的叶测试在原并发下通过。
- 不复现：在固定源码快照、原并发与原测试选择下，累计 3 次完整 component 段均无回环连接分配错误，每次保留失败窗口同口径的采样证据（2026-10-16 L0 固定快照门禁计第 1 次）。满足后按“无法复现”关闭移到 `done/`，`## 关闭` 写明采样结论与重新登记的触发条件；之后再次复现即按原信号重新登记。
- 两种结束方式都要求：相关功能原断言保持，无新增 skip、无削弱断言。
- component 失败短路 boundary、e2e 时如实报告未启动范围；三段的完整执行归统一门禁，不作为本条结束条件，也不能用其他时段的独立架构运行补记被短路的段。
- 如仍有显式 live 测试跳过，按其独立入口和未验证范围如实报告；不通过降低 skip 统计或删除 live 守卫制造通过。
- 不通过放宽系统限制、永久降低断言要求或终止无关外部进程代替根因处置。全局门禁仍由统一入口执行。

## 前置

应用层相关文件若仍归在途任务，先遵循文件归属边界。现有 `INDEX-DECLARATION-01`、`DEBT-INVENTORY-CONSUMPTION-01` 的失败不能并入本条后宣称已处理。

## 不做

不安装依赖，不启动额外服务，不停止外部进程；不改系统资源限制，不新增 skip，不放宽断言，不把缺配置的真实部署验收混同进程内组件合同。

## 已有证据与未定因样本

- 完整运行：2349 pass / 67 fail / 6 test skip，`sourceChanged=false`；67 事件中只有 31 叶/35 事件直接含连接错误。
- component 的 79 个已启动包全部结束；因其失败，脚本没有启动 boundary/e2e。
- `TestBrowserAuthorizationUsesFixedDeploymentUpstream` 的 provider 503 与 `TestHTTPBeginShutdownCancelsStateThenDrainsBeforeResourceClose` 的 CatchUp 超时没有直接 connect 证据。两者及一个直接 connect 失败样本随后由根任务隔离复跑 3 pass / 0 fail / 0 skip，源码稳定，证据 `.validation/runs/20261015T135824Z-7a728cea4a88/report.json`。这只说明隔离可通过，不证明它们与明确连接错误同根因，暂保持未定因。
- 事后 netstat/ps/lsof 只能说明采样时状态，不能倒推出失败窗口峰值或哪一个进程导致错误。

## 本轮调查（2026-10-16）

开始按原并发合同采样失败窗口：记录子进程、回环连接状态与打开文件数，不修改系统限制或断言。统一门禁等待本轮源码与任务记录冻结后执行；首次调查沿既有进程内夹具与本轮独占 OpenSearch，不占用真实部署或共享搜索。完整合同若有显式 live skip，逐项登记其入口与未验范围。依据与目标沿本条既有测试/质量文档，不修改设计或产品行为。

- 2026-10-16 L0 固定快照门禁：索引（`.validation/runs/20261016T015828Z-l0-snapshot-gates/gate-index.md`）；原并发 56 次采样无连接分配错误，历史 31 叶全通过；根因仍未定，component 因主线失败而短路 boundary/e2e，保留 doing。

- 2026-10-16 分源门禁：索引（`.validation/runs/20261016-environment-recovery/gate-index.md`）；历史31叶已通过，最新合同资源窗口无连接/FD分配错误，原地址失败根因仍未定；v3合同唯一主线红、e2e因VM资源中断，保留 doing。

## 关闭（2026-10-16，无法复现）

- 按原验收累计三次固定源码、原测试选择及默认并发的完整 component：L0 的 79 包/56 次采样、v3 的 79 包/29 次采样、本轮首轮的 80 包/31 次采样。三轮各自 `sourceChanged=false`，历史 31 条明确连接错误的叶测试每轮全部通过，连接及 FD 分配错误均为 0；保留同口径 ps/lsof/netstat 过程证据。逐叶对照、命令、源码与采样路径见关闭证明（`.validation/runs/20261016-doing-validation/http-debt-closure-proof.json`）。
- 上述三轮中其它功能失败及其短路范围保留原报告，不将其改记为整体通过。修正一条搜索结果旧夹具后的最终完整合同三段均执行，4818 pass / 0 fail / 11 个显式 live skip，源码稳定；独立 live 补验及未验范围另见本轮汇总（`.validation/runs/20261016-doing-validation/summary.json`）。
- 根因仍未知；按本条“不复现”分支关闭，不声称已修复端口、FD 或 HTTP 生命周期缺陷。再次出现 `can't assign requested address` 或同类回环资源分配错误时，按原信号重新登记并保留失败窗口采样。
- 未改变默认并发、系统限制或原功能断言，未新增 skip，未终止无关进程。文档及质量门禁通过；未提交。
