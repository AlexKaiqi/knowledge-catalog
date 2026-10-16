# DEBT-DESCRIPTOR-READ-RESOLUTION-01 · 取值描述解析使用只读能力并保留来源故障

## 债

- 位置：`object/writer/value_source.go` 的跨仓取值描述解析、`treecodec.go` 的纯读取辅助函数。
- 信号：真实部署的业务 Writer 发布外部取值声明时，扫描到 runtime System 只读包装后因缺写接口拒绝；同一解析器又把远仓 HEAD 失败或空版本当作不存在，可能遗漏第二个描述持有仓。
- 判定：依赖读取误要求写能力，并把未知来源状态压成不存在，均破坏唯一取值描述核验。
- 处置：纯读路径只要求 Snapshot 只读端口；HEAD 与正文故障失败关闭，不跳过 System、不丢失歧义、不改变正常 locator 未命中语义。
- 触发条件：`ACCEPT-HARNESS-01` 真实部署首次构造跨仓外部取值前态。

## 目标

Writer 可以读取只读来源完成依赖核验，来源不确定或多仓持有相同描述时保持拒绝，不推进目标版本。

## 依据

- [对象](../../doc/design/object.md#系统仓)。
- [LiveAccess 声明](../../doc/design/liveaccess.md#声明)。
- [Snapshot 合同](../../doc/design/snapshot.md#合同)。

## 验收

- 完整 System 出版物经真实只读包装后，第三仓描述可以支持业务声明的 Ingest 预览与 Writer 提交；System 写接口保持不暴露且显式写拒绝。
- System locator 读取故障、另一持有仓 HEAD 或正文故障、空 HEAD 与重复持有均拒绝提交并保持目标 HEAD 不变。
- 受影响合同与统一快照门禁通过，真实部署继续验证原前态。

## 前置

无。

## 不做

不开放 System 写能力，不修改目录预览来源路径，不修改索引声明生产代码，不跳过有故障的来源。

## 关闭

2026-10-16 登记后立即并入正在进行的 `ACCEPT-HARNESS-01`，共用真实部署验收与依赖解析前态。两阶段红绿及受影响回归见 `.validation/runs/20261016-accept-runtime-repair/`；真实部署与统一门禁由主任务收口。
