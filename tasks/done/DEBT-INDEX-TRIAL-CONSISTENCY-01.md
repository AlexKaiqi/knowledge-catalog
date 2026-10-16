# DEBT-INDEX-TRIAL-CONSISTENCY-01 · 索引试运行逐成员判定与原文件传输一致

## 债

- 位置：`home/index_trial.go`、`server/plugin_capabilities.go`、`discovery/cli/plugin.go`。
- 信号：2026-10-15 INDEX-DECLARATION-01接管增量评审复现四项问题：同批坏成员连带污染合法成员的状态；更新文件省略schema_ref时尚未继承就拒绝；畸形非空RawQuery被解析器丢弃后放行；非法UTF-8原文件经JSON替换字符后改变合法性判断。
- 判定：逐成员结果和Writer对同一文件判断存在真实差异，查询拒绝合同不完整。没有证明凭证或私有Schema泄漏。
- 处置：恢复索引任务后，将红例入库，复用Writer有效Schema判断、分离成员上下文，拒绝所有非空query及无法无损表示的文件；不改Writer形状规则或降低原断言。
- 触发条件：解除INDEX-DECLARATION-01接管后写入冲突，继续试运行入口收口时。

## 目标

一个无效成员不误报其它成员；继承SchemaRef的合法文件与Writer一致；原文件不被传输静默改写，拒绝参数边界完整。

## 依据

- [发现](../../doc/design/discovery.md)：逐成员支持情况、能力清单与无写入试运行。
- [对象](../../doc/design/object.md)：继承Schema与完整维护单元。
- [Affordance](../../doc/design/affordance.md)：公开说明和实际判定一致。
- [工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 四项反例分别先红再绿，不以未装解释器代替结构判断，也不让一个成员的错误位置成为另一个成员的原因。
- 混合合法/非法成员、已有声明省略schema_ref更新、非法百分号query与非法UTF-8文件均有真实入口回归。
- 保留现有权限/无主体/固定basis/无worker与HEAD或索引副作用断言。
- 复核正式HTTP闭集覆盖新端点，不只调整路由计数；另核对自定义Manifest数值option类型与checkOption分支是否一致（只读发现的待证线索，不预设缺陷）。
- 受影响测试、`make check-docs`、`make quality`通过，无skip。

## 前置

`INDEX-DECLARATION-01`接管后冲突归属已由用户裁决。红例位于 `.validation/runs/20261015T133000Z-plugin-capability-review/findings.md`，执行报告分别为 `.validation/runs/20261015T132424Z-94cb10afa5d3/`、`.validation/runs/20261015T132529Z-5749abb085cb/` 与 `.validation/runs/20261015T132538Z-cli-transport-review/`。

## 不做

不借本债提前开始Dataset检索空间，不新增登录方式或放宽授权，不修改他人恢复写入的文件。

## 关闭

2026-10-16 并入 `INDEX-DECLARATION-01`：位置、信号与剩余验收在该任务的“并入”一节继续推进。本文件已有的交付与证据保留为历史，不代表验收通过。
