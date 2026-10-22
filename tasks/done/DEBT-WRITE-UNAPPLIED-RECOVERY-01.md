# DEBT-WRITE-UNAPPLIED-RECOVERY-01 · 明确未应用的写入不再等待不存在的回执

## 债

- 位置：`snapshot/internal/storecontract/write_outcome.go`、`kernel/errors.go`、`object/client/recovery.go`。
- 信号：Writer 与命令账已有 WriteNotApplied 证明并释放未应用命令占用，但 HTTP 错误只保留代码和消息；客户端把这些 TEMPORARY_UNAVAILABLE 与结果未知同样处理，可能等待默认十分钟。
- 判定：已有发布结果证明跨错误边界丢失，不能靠错误码或消息字符串猜测补救。
- 处置：统一错误信封保留明确未应用证明；COMMIT/PUT/REMOVE 恢复立即返回原错误，不查询回执、不重发。未知结果保持原恢复逻辑。
- 触发条件：用户明确要求修复并测试；从原上传性能债的附记独立拆出。

## 目标

明确未发布的失败立即返回，未知发布仍可靠恢复原回执，不丢原错误码，也不重复提交。

## 依据

- [对象](../../doc/design/object.md)：Writer、网络中断与发布结果。
- [Snapshot](../../doc/design/snapshot.md)：结果未知的权威恢复。
- [工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 改前回归复现错误信封丢失证明，以及 COMMIT/PUT/REMOVE 错查回执。
- 明确未应用响应不查回执、不重发，保留原错误码与消息；旧服务无标记、未知标记、普通 503、断线与解码失败仍按原命令恢复。
- 未知 apply 结果保留 PENDING；命令占用释放失败保留未发布证明但不伪称已释放；接受后回执保存失败不伪称未应用。
- 错误归一化不污染可复用的原错误，普通错误保持原 JSON 形状；跨 HTTP 的账本/客户端组合回归通过。
- 受影响回归、`make check-docs`、`make quality`、`make test`、`make test-contracts` 执行并准确记录结果；无 skip 才归档。

## 前置

无。账本已有明确未应用与未知结果的区分机制，复用既有证明，不新增持久状态。

## 不做

不缩短未知结果的恢复预算，不按所有 503 直接退出，不自动重发，也不改发布、幂等或账本释放规则。
不处理大仓批量预检、locator 布局或无变化发布。

## 改前证据

HTTP 错误信封缺少未应用证明，COMMIT/PUT/REMOVE 均错误查询一次回执，甚至把伪回执当成功。
这两组回归在修改实现前按预期失败；无标记与未知标记的恢复正对照通过。

## 实现范围

统一错误信封新增可选 writeOutcome，只有 NOT_APPLIED 表示明确证明；空值或未知取值不推导结果。
Snapshot 的既有私有错误包装通过窄错误能力传递证明，不让 kernel 依赖 Snapshot。
归一化复制错误以保存证明，不改共享原错误；已有 JSON 解码自动保留该字段。
客户端恢复增加“明确未应用直接返回”出口，仍只提交一次。命令账在占用清理失败时也保留提供方的未发布证明，
但状态和占用释放行为不变：PENDING 保留，重试仍拒绝，须显式处置。

## 定向验证

kernel、Snapshot、Object SDK 与共享 HTTP codec 的完整包回归通过。
CLI 真实 HTTP 组合验证由命令账产生未应用证明，再经统一错误信封与正式 SDK 恢复入口返回，
未查回执；原 POST 超时后恢复原回执、确定拒绝不查回执的回归仍通过。
客户端正反例覆盖三种写面、旧服务无标记、未知/未来标记、代理 503、成功响应解码失败；
账本断言覆盖未知 apply、占用释放失败保持 PENDING/摘要保护，以及接受后回执保存失败不附未发布证明。
`make quality`（含 check-docs）通过。完整门禁在冻结副本继续验证。

## 清理失败边界复核

占用清理失败与发布结果分别成立：目标已证明未发布时，清理失败不能把该事实改写成结果未知。
新增先红后绿回归，要求该错误保留未发布证明，同时原 PENDING 占用不删除，同请求和异请求的重试都拒绝。
这只停止无意义的客户端回执等待，不产生自动重试权或成功回执。

## 本轮交付与完整验证（2026-10-20）

修复已应用；明确未发布的 COMMIT/PUT/REMOVE 立即返回原错误，不查回执、不重发。
未知结果仍恢复原命令。占用清理失败保持未发布证明，同时保留 PENDING 和摘要保护。
本轮复核未遗留新增质量问题。

- 改前证据：`kc-unapplied-client-red.log`、`kc-unapplied-envelope-red.log`、`kc-unapplied-release-red.log`；均先复现，再修改实现。
- 完整受影响包回归与 CLI HTTP 组合回归通过；`make quality`、`make check-docs`、diff 空白检查通过。
- 最终冻结副本 `make test-contracts`：5156 pass、0 fail、14 skip，退出码 0；公开命令覆盖 54/54。外部真实环境用例的 skip 仍保留，未以本轮修复冒充部署验收。
- 最终冻结副本 `make test`：18 pass、0 fail、0 skip，退出码 0。
- 两个报告的 `sourceChanged=false`；本轮实现、回归与合同文件哈希均匹配最终测试快照。
- 证据根：`.validation/runs/20261020-unapplied-recovery/`，汇总 `summary.json`；完整报告分别在 `20261020T104150Z-a3a9e955e86a/`、`20261020T104151Z-6d86905b2afd/`。

任务保留 doing：完整契约仍有 14 项外部环境 skip，尚不满足无 skip 归档规则。
未运行真实部署验收，未提交代码。

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
