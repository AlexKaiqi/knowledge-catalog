# ZERO-DIFF-01 · 目录零差异发布的原始意图

## 目标

目录对照为零差异时，服务端能核验可复核的原始发布意图与固定基准，不能只接收客户端过滤后的空集合，
也不能由客户端单方面宣布成功。零差异发布走完整 Writer 校验，不因"看起来没变"而跳步。

客户端不按正文摘要删掉"相同"单元：完整解析全部声明单元后交同一 CommitIntent 核验，把"什么是意图"
留在服务端判定。首次基点由服务端固定，重试沿用原坐标与完整请求摘要。

## 依据

设计篇：[无变化写入](../../doc/design/object-versioning.md#无变化写入)判据与结果中"目录对照为零差异"
一句及"显式空操作集仍拒绝"；[接口与实施边界](../../doc/design/object-versioning.md#接口与实施边界)
"目录零差异发布"一行要求服务端核验可复核的原始意图与基准、不得退化为空批次成功。

## 已交付

### 目录完整意图

远程与嵌入目录提交完整解析所有声明单元，交同一 Writer CommitIntent 核验，不在客户端按正文摘要
删掉"相同"单元。首次基点由服务端固定；同命令重试使用原坐标和完整请求摘要，显式不同基点、bulk、
正文、Schema、路径、provenance 等修改继续拒绝。两份旧目录重放算法已删除，不保留私有原样转发。

目录 diff 仍是按固定基点的正文预览，与发布完整效果判定分开：预览可以说"零差异"，那不等于发布成功。

入口：[CLI 提交与目录解析](../../cli/remote.go)、[Writer 意图](../../object/writer/commit.go)、
[服务端路由](../../server/service_routes.go)；帮助文本见 [help_writer.go](../../cli/help_writer.go)。

## 验收

| 场景 | 判定 |
|---|---|
| 目录零差异发布 | 服务端可复核原始意图与固定基点；走完整 Writer 校验；不因零差异跳过授权、Schema 与前置 |
| 客户端过滤 | 客户端不上送空集冒充意图；全部声明单元完整上送，含被删单元与路径变化 |
| 显式空操作集 | 仍按非法请求拒绝，不因"零差异"获得例外 |
| 同命令重试 | 使用原坐标与完整请求摘要，返回原结果 |
| 意图被改动 | 显式不同基点、bulk、正文、Schema、路径、provenance 等修改继续拒绝，报幂等冲突 |
| 目录 diff 与发布判定 | 按固定基点的正文预览与发布完整效果判定分开，不互相顶替 |
| 旧算法 | 两份旧目录重放算法保持已删除状态，不复活私有原样转发 |

CLI 与 HTTP 各执行一次"目录零差异提交 → 可复核意图 → 重试返回原结果"，并覆盖意图改动被拒。

## 剩余

1. 同一冻结候选上的完整混合契约。`.validation/runs/20261020T190446Z-de77a7c8cc7a`
   （2940 通过、5 skip）与 `.validation/runs/20261020T191137Z-e49638a261b8`（271 通过、无失败或 skip）
   是批次证据，不是冻结候选验收。
2. 真实部署验收。
3. 与 `NO-CHANGE-01` 的交界需要一次联合回归：目录零差异提交在无变化出口与完整意图核验两侧
   都成功后，回执只出现一份 `changed:false`，不重复记账。

## 前置

无。目录意图核验在客户端与服务端之间完成，不依赖固定读取依据，可与本族其它任务并行。
与 `NO-CHANGE-01` 改同一批 Writer 文件，按候选顺序合入。

## 不做

不新增目录提交协议、bulk 语义或跨仓事务。不把目录 diff 改成发布判定。不做性能优化与成本目标。

## 交付

实现已完成，见本节；整体验收未完成，任务保持 `doing/`。

- 目录完整意图：源码冻结 `.validation/runs/20261020T190446Z-de77a7c8cc7a`：2940 通过、5 skip；
  最终公开联合 `.validation/runs/20261020T191137Z-e49638a261b8`：271 通过、无失败或 skip，
  覆盖目录、bulk、丢响应恢复、权限、固定依据、双侧比较、真实公开路由及帮助。
- 冻结候选 `.validation/runs/20261022T154324Z-93b18dd4deb1`（默认场景 18 通过、无 skip）、
  `.validation/runs/20261022T154436Z-5d9104e40a73`（5682 通过、0 失败、18 条既有环境 skip）、
  `.validation/runs/20261022T155925Z-48b3b9703db5`（真实部署专项 2 通过、无 skip），源码指纹
  `c967676f46a0a8c53b3dd3ceff862af8c7628a36f8679ce800899db629c70a0c`，三次 `sourceChanged=false`。
- 「目录零差异发布」由 `object-published` 节点 `probe-zero-diff-directory` 在候选上执行：目录内容与
  当前 head 一致时返回 `changed=false` 与 `REPLAYED`，head 不前移；旧 command id 不能承载新意图。
- 与 `NO-CHANGE-01` 的交界联合回归：`TestNoChangeWriterBulkKeepsRequestedDataPlaneWithoutCommit`、
  `TestNoChangeMergeStillReportsAdvancementWhenTreesAreEqual` 在同一候选上通过，后者确认内容相等不抑制
  经评审的发布推进。
