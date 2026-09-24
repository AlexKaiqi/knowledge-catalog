# 基础重构验收合同

> 状态：Dolt adapter 已按[架构总览](core-architecture.md)的裁定退役删除；本文保留历史选型与实测记录，其中 Dolt 相关入口、命令与合同不再存在于代码中。

日期：2026-09-10
定位：验证入口。本文只定义“怎样证明基础重构已经完成”，不拥有协议、分层、provider
能力或公开 API。目标与执行序见根 [TASK.md](../../TASK.md)；设计事实以
各行 owner 为准；测试保留与运行证据规则以 [`test-catalog.md`](test-catalog.md) §0 为准。

本轮范围固定为 `DOLT-01`、`DOC-14/16/17/18/19` 与 `APP-CORE-01`。动态 State、规模资格、
组件选型、多实例和所有 `REVIEW-*` 不在本轮完成分母。

---

## 1. 判定规则

每个验收项必须同时具备：

1. owner 已选定的目标与禁止观察；
2. 会在旧实现上失败的反例；
3. 目标 provider/transport 的真实执行，不以 interface 存在代替；
4. 同一源码指纹下的定向合同、边界守卫和普通套件结果；
5. pass / fail / skip / 未执行分别计数，未登记 skip 和环境缺失均不算通过。

测试名在对应函数存在之前只属于本文的**计划验证入口**，不得写入
[`architecture-invariants.md`](architecture-invariants.md)。实现后才把实际测试名登记为
不变量证据。不同 revision、节点或日期的结果不得拼成一次“全绿”。

## 2. 验收矩阵

### RA-01 · Dolt 会话复用（已随 Dolt adapter 退役）

- **状态：** 已退役，不是待办。`snapshot/dolt` 与 `knowledge/dolt` 已从仓中删除；本条保留
  为历史验收记录，不再有对应实现与验证入口，也不得为满足本条重新引入 Dolt。
- **原 owner：** [架构总览](core-architecture.md)；条目 `DOLT-01`（随 adapter 退役）。
- **原成功观察：** 同一 Repository 生命周期内 20 次只读查询不再启动 20 个 Dolt 进程；
  变更命令先释放会话并继续保持单写者；`Close` 可重复并回收进程。
- **原验证入口：** 进程复用、多行语句与诊断对齐、诊断不污染下一次结果三组会话用例，
  以及 `snapshot/dolt`、`knowledge/dolt` 定向包与 session race——全部随包删除。

### RA-02 · Provider 合同与等价性

- **owner：** [架构总览](core-architecture.md)；
  门槛直接引用 [`provider-contract-validation.md`](provider-contract-validation.md) 的
  `PV-01`…`PV-12`，本文不复制阈值。
- **成功观察：** native provider 实际运行共享 `RepositoryContract` /
  `WriterContract`；文件型与 native provider 对同一 Operation 序列按步骤索引比较身份外的
  公开观察，差异为空。Dolt 是首个取证的 native provider，已随 adapter 退役删除；
  等价性门槛保留给未来的 native provider。
- **禁止观察：** 用不同 commit 标识直接比较；缺 live 环境静默 skip；只验证 interface
  存在；provider 分支进入 Catalog、Reader、Writer 或 CLI。
- **必跑：** 等价性对拍按 [`provider-contract-validation.md`](provider-contract-validation.md)
  的流程执行；严格 live 模式用 `KC_REQUIRE_LIVE_ADAPTERS=1`。当前仓内没有 native
  Knowledge provider，该门槛暂无可执行用例，登记为缺口而不是已满足。

### RA-03 · 有界点写、点读与变化识别

- **owner：** [`scale-architecture.md`](scale-architecture.md) 的成本约束、
  [架构总览](core-architecture.md) 的变化能力；
  条目 `DOC-17`。
- **成功观察：** 在固定单对象操作下，将仓从小档放大至少一个数量级后：
  authority 读取调用数、定位页数与解码字节保持同一常数上界；单对象变化识别只返回受影响
  身份；定位结构丢失时通过明确维护入口有界重建。
- **禁止观察：** `ListFiles`/对象全量分页/全 manifest 解码；吞掉变化能力错误；索引层把
  能力缺失改成全量 rebuild；消费请求触发维护扫描。
- **验证入口：** `TestSingleObjectPutCostIsIndependentOfRepositorySize`、
  `TestSingleObjectReadDecodeBytesAreIndependentOfRepositorySize`、
  `TestChangedObjectIDsFailsClosedWhenNativeChangesFail`、
  `TestEnsureFailsClosedWhenIncrementalChangeLookupFails`、
  `TestExplicitLocatorRebuildRecoversLegacyLayout`。

### RA-04 · 能力冻结与 raw tree 隔离

- **owner：** [架构总览](core-architecture.md) 与
  [架构总览](core-architecture.md)；条目 `DOC-14`、`DOC-18`。
- **成功观察：** composition root 一次解析并冻结读/定位/变化能力；请求路径只消费冻结
  结果；native Knowledge 装配不满足 raw tree 写；缺能力返回稳定协议错误。
- **禁止观察：** 不带 `ok` 的运行期类型断言、panic、按 provider 名称分支、native
  `ApplyTreeCommit` 旁路、以目录为单位跳过生产代码守卫。
- **验证入口：** `TestBaseAuthorityWithoutKnowledgeCapabilitiesFailsClosed`、
  `TestMissingProviderCapabilitiesFailWithoutPanic`、
  `TestArchitectureGuardIncludesProductionScripts`、
  `TestFixtureDeploymentUsesOnlyDeclaredHomeSeams`。native raw tree 写拒绝的专用用例随
  Dolt adapter 退役删除；该约束当前由能力冻结守卫与「仓内无 native Knowledge provider」
  的现状共同成立，重新引入 native provider 时必须恢复对应用例。

### RA-05 · 命令账本恢复与保留

- **owner：** [`scale-architecture.md`](scale-architecture.md) 的账本约束和 Writer
  公开合同；条目 `DOC-19`。
- **成功观察：** 预留后中断与提交后中断在重启后都返回确定结论；PENDING 可经显式、
  可审计入口解决或放弃；重放只在声明窗口内保证；清理不要求启动时加载全部历史；证据写失败
  不把已接受 commit 报告为失败。
- **禁止观察：** 永久占用 command ID；后台猜测提交结果；清理已在保证窗口内的回执；
  证据失败回滚 Canonical 或返回“提交失败”。
- **验证入口：** `TestCommandLogRecoversReservationBeforeCommit`、
  `TestCommandLogRecoversCommitBeforeReceipt`、`TestCommandLogRetentionIsBoundedAndKeepsPending`、
  `TestBoltCommandLogPrunesWithoutDeletingPending`、`TestCommandLogReadFailureCannotReapplyCommand`、
  `TestAcceptedCommitSurvivesEvidenceFailure`。

### RA-06 · Typed Application Core

- **owner：** [服务边界](service.md) `API-01` / §1.2 与
  [架构总览](core-architecture.md)；条目 `APP-CORE-01`。
- **成功观察：** CLI 和 HTTP 对同一用例构造 typed request 并进入同一个 executor；
  Repository、Workspace、Pin 等标识在 transport 边界成为 owner 命名类型；READ/SEARCH、
  Writer、Governance、Operations 的公开结果与协议错误在迁移前后逐观察等价。
- **禁止观察：** 应用包 import CLI parser、HTTP registry、具体 provider 或部署解析器；
  应用用例接收通用 verb、HTTP request 或 `map[string]FlagValue`；transport 复制授权、
  basis 选择、Canonical 回读或 Writer 规则。
- **验证入口：** `TestCLIAndHTTPUseSameTypedApplicationExecutor`、
  `TestApplicationCoreHasNoTransportOrProviderImports`、
  `TestApplicationRequestsUseOwnedIdentifierTypes`、`TestCommitExecutorPreservesTypedIntent`，
  并保留已有 `API-01` 全部证据。

## 3. 红绿执行纪律

每个 `RA-*` 独立保存：

```text
旧实现 + 新反例 = FAIL
最小实现 + 同一反例 = PASS
相关既有合同       = PASS
边界守卫           = PASS
```

如果反例在旧实现上已经通过，必须证明它确实覆盖禁止观察；不能只改名当红例。若实现期间发现
目标必须变化，先回到 owner 和根 `TASK.md`，不得用测试适配当前实现。

## 4. 最终命令与运行档

基础闭环的同 revision 总验收：

```bash
export PATH="$HOME/.local/go/bin:$PATH"

make check-docs
make validation-inventory
make check-validation
make check-surface
make test-boundary
make test

KC_DOLT_FORCE_DOCKER=1 KC_DOLT_DOCKER_IMAGE=dolthub/dolt:2.3.1 \
  go test ./snapshot/dolt ./knowledge/dolt -count=1 -v
make test-adapters
```

还必须运行本轮新增 parity、poison、有界计数、账本故障注入与 application-core 合同。race
只对改动并发状态的包执行，至少覆盖 `snapshot/dolt`、`snapshot/commandlog` 和新应用包。

`make test-all` 可以提供更宽证据，但不替代上面的严格 provider 档；与本轮无关的 live
服务缺失需逐项记为未验证，不能反过来阻止对基础闭环作精确判定。

## 5. 完成与回写

只有 `RA-01`…`RA-06` 全部通过，才可完成本轮。完成时：

1. [`TASK.md`](../../TASK.md) 逐项勾选实际完成的条目；
2. [`architecture-invariants.md`](architecture-invariants.md) 只登记已经存在并通过的证据；
3. [`test-catalog.md`](test-catalog.md) 登记执行范围、skip 和运行产物；
4. [`mvp-acceptance.md`](mvp-acceptance.md) 只更新实际产品可用范围；
5. 未执行、环境缺失和失败保持原状态，不把后续 `REVIEW-*`、动态、规模或组件选型计入完成。
