# PRODUCT-CONSISTENCY-01 · 对齐插件帮助、值班示例与动画依据

## 目标

LiveAccess 的公开说明符合部署装配、按声明取值的模型；产品书值班命令取得支撑其判断的手册与活动说明；
动画以本机实际材料核对，明确当前制作依据、版本轨迹及仓库可交付范围。

## 依据

- [插件](../../doc/design/plugins.md)、[LiveAccess](../../doc/design/liveaccess.md)：部署装配与单元取值声明。
- [Dataset](../../doc/design/dataset.md)：消费固定修订，更新需发布后采用。
- [产品书](../../doc/product.html)、[测试与验证](../../doc/engineering/testing.md)、[工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 先取得旧帮助、旧修订命令的失败证据，再修正；回归从公开帮助和实际示例读取验证声明与内容。
- 核对动画的当前脚本、时间线与公开命令；发现的不一致修正或以可复核的债务记录承接，不改写历史任务。
- 受影响回归、`make check-docs`、`make quality` 通过，无 skip。

## 前置

无。

## 不做

不变更插件路由、声明协议、Dataset 修订语义；不运行真实部署验收，不安装依赖。
工作区已有的 `invocation/schema_dataset.go` 与 `invocation/schema_list_readiness_test.go` 不属于本轮改动。
验收期间另出现的 Writer 本机路径修复及两条债的归档也不属于本轮改动，保留其工作区内容。

## 核查

- LiveAccess 帮助仍称来源仓配置插件；紧邻说明还有取值来源属于 Schema 的旧描述。
- `store-oncall` 修订 1 包含活动说明和旧重启建议；手册 v2 在修订 2 中交付。
- 当前本机存在 `.data/animation/`、`STYLE-LOCK.md`、`handdrawn-discovery/script.md` 与 `film.mjs`；
  `brief.md` 不存在。目录命中 `.gitignore` 的 `.data/*`，Git 未跟踪，不能以普通文件检索或干净检出判断其内容。

## 本轮修正

- LiveAccess 同源帮助改为部署装配、方面或成员声明、固定取值描述与 `runtime` / `protocol` 匹配；
  同时清理 `binding show` / `access` 帮助和 README 中 origin 来自 Schema、按仓配置插件的旧说法。
  索引说明限定 `shared` 外部值，并说明新观察回读补判。
- 产品书值班消费段统一固定 `store-oncall` 修订 2。新增回归直接执行 HTML 代码块，经 Server 读取
  手册 v2 与活动提示，检查 800ms、低于 1% 的错误率和不要仅因延迟升高重启的实际条件。
- 动画当前制作依据为 `.data/animation/handdrawn-discovery/script.md` 与 `film.mjs`。
  保留前半修订 1 的历史旅程，按公开夹具忠实展示旧手册原文；后半先提交手册 v2、发布 Dataset
  修订 2，再在同一修订内搜索与回读。LiveAccess 画面明确插件部署与按声明取值。
- 当前脚本、README、STYLE-LOCK、84.45 秒成片及三个预览同步更新。新增本地一致性脚本，核对真实原句、
  修订顺序、装配文案、5067 帧有限坐标以及源与媒体摘要；7 项通过。整片和三个预览全解码通过，
  关键帧已目视检查。材料仍只在本机，不以忽略文件或历史任务记录冒充可随 Git 检出恢复的来源。

## 验证记录

- 先红：旧帮助缺部署与声明条件且保留旧模型；产品书原命令实读手册 `revision: "1"`；动画原文、
  LiveAccess 装配、新修订发布及采用顺序的五项检查失败。修正后相应回归及媒体检查通过。
- 原工作区的受影响帮助 / 产品示例回归、完整架构守卫、`make check-docs`、`make quality` 均通过。
  首次 `make test` 为 18 pass / 0 fail / 0 skip，`sourceChanged=false`。
- 全局 `make test-contracts` 的测试执行为 4943 pass / 0 fail / 11 skip，公开命令覆盖 54/54；
  跳过项是需独立配置或外部环境的既有测试。运行期间并行 Writer 修改及债务归档使源码变化，
  正式报告为 `source-changed`，命令最终退出 2，不能记为同一候选的合同验收通过。
  原报告为 `.validation/runs/20261016T101432Z-9c4a105c23cf/report.json`；不改写这份失败证据。
- 隔离快照中的受影响回归为 68 pass / 0 fail / 0 skip，完整架构守卫为 680 pass / 0 fail / 0 skip，
  `make test` 为 18 pass / 0 fail / 0 skip，三份逐项报告均为 `sourceChanged=false`；
  `make quality`（含 `make check-docs`）通过。事件数含子项，不充当整个仓库的验收计数。
  当前工作区本轮拥有的 7 个源码 / 说明文件摘要与该快照一致。
  原日志、快照清单和复制的正式报告位于 `.validation/runs/20261016-product-consistency/`，
  正式报告副本在 `snapshot-evidence/` 下；临时源码副本在证据复制后回收。

本轮没有协议行为或公开形状扩张，没有弱化既有断言或新增 skip。说明失配、示例混版和动画采用顺序
已分别由对应回归或本地一致性检查承接；未安装依赖。全局合同未形成同候选全绿证据，任务暂留 `doing/`。

## 提交范围

后续已明确授权提交、推送。本次提交包含上述 7 个源码 / 说明文件和本任务记录，推送目标为现有分支的
`origin/refactor/object-model` 上游。动画仍是本机忽略材料，不进入本次 Git 提交；其它并行任务的改动保留。
