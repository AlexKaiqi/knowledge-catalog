# 发现与使用记录

六条任务，一条母体加五个用法。它们不是六批独立工作：后五条全部依赖第一条产出的使用记录。

| 任务 | 设计进度表位置 | 负责什么 |
|---|---|---|
| `DISCOVER-USAGE-01` | 近期 | 记下每次检索用到的成员与角色、被拒条件；回读关联查询；反馈只认自己搜到的回读；按成员汇总公开 |
| `DISCOVER-DATASET-FIND-01` | 近期 | 把“先找 Dataset”也记成可归因数据，按 Dataset 汇总给库存与 Dataset 维护者 |
| `DISCOVER-INTENT-01` | 未到 | 把检索调用串成可还原的轨迹，并可选收意图 |
| `DISCOVER-PROFILE-01` | 未到（分面与总数） | Dataset 修订按 Schema 字段的数据画像 |
| `DISCOVER-REPLAY-01` | 未到 | 用历史使用记录在另一个 Dataset 上回放，比较声明 |
| `DISCOVER-PACKAGE-01` | 未到 | 可选发现包：给任务说明，出命中对象，不给回答 |

## 顺序

1. `DISCOVER-USAGE-01` 先做。它是母体：轨迹挂在它的访问证据上，回放的输入是它积累的记录，
   找 Dataset 的归因是它的一种汇总口径。它自己的前置只有 `INDEX-SEARCH-SPACE-01`。

2. `DISCOVER-DATASET-FIND-01` 与 `DISCOVER-INTENT-01` 都只前置母体，可在母体交付后各自独立推进。
   两者改的是同一套记录的不同维度（前者按 Dataset 汇总，后者按 trace 串联），文件不重叠。

3. `DISCOVER-REPLAY-01` 前置母体；意图字段由 `DISCOVER-INTENT-01` 带入，但回放本身不依赖意图。

4. `DISCOVER-PACKAGE-01` 前置 `DISCOVER-INTENT-01`：它要把每一步原语调用写进轨迹，没有轨迹就无处落。

5. `DISCOVER-PROFILE-01` 独立于其余五条，只前置 `INDEX-SEARCH-SPACE-01`。它与母体同族是因为同属
   “读 Dataset 数据侧说明”，实现上没有共用。

## 为什么它们是六条而不是一条

每条都改不同的落点：母体改证据与汇总，找 Dataset 改库存列举的记录，轨迹改会话串联，画像改统计计算，
回放改只读对比，发现包改编排入口。合起来会得到一个既写记录又做统计还跑编排的任务，验收无法逐条判定。
按 `tasks/README.md` 的规则，合并只在“一句话讲不清”时做，这里六句都讲得清。

## 与索引主线的关系

这条线原本有一条 `INDEX-USAGE-01`，名字带 `INDEX` 但做的是 discovery 使用记录，不构建也不重写索引。
它于 2026-10-23 被删除，造成三处悬空：路线近期第 6 项的“按成员公开使用数据”没有承接、
`accept-10-usage.yaml` 四条规格与其在 `accept.py` 的归因指向不存在的任务、`FINAL-ACCEPTANCE-01` 据此
判定“规格每条有执行证据”无法满足。同日按 git `c219b744` 的原文恢复为 `DISCOVER-USAGE-01`，归入本族，
不再挂在索引主线名下。

索引主线仍只有三条，见 [../index-refactor/README.md](../index-refactor/README.md)。
