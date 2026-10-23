# INDEX-SURFACE-01 · 索引的对外面：一套词、一次问清就绪

## 目标

用户能读到的索引能力只有一套词，并且能一次问清"这个修订现在能不能搜"。

## 关闭

2026-10-22 并入 `INDEX-PUBLISH-DECOUPLE-01`，未按本文件单独验收。

并入理由：本文件的就绪语义完全建立在"接受与就绪是两个状态"之上（原文"前置"一节自己写着要先等设计决定
1–11 落地）。这两件事又改同一批 `server/` 路由、`cli/` 命令声明、搜索视图与 `routes_golden.txt`。分开做
的结果是：先在旧同步语义上把 projection 改名成 index，等发布时序改了再回来修同一批文件里"就绪"的含义。
一次做完只需一次 allow 文件与场景授权迁移。

承接内容：

- 命令家族：`kc index --dataset <id> [--revision N]` 查就绪、`kc index prepare` 准备、`kc index` 无参走进度。
  验收第 28–30 条。
- 路由闭集：`indexes:ready`、`indexes:prepare`、`indexes:describe`、`indexes:status` 四条；旧
  `projections:describe`、`projections:sync` 删除；`routes_golden.txt` 由 88 条变为 90 条。验收第 32 条。
- 搜索字段：只有 `searchView.indexRevisions`，无 `projectionRevisions`。验收第 33 条。
- 授权词：`index.read`／`index.manage` 直接切换，不留过渡期双词。验收第 34 条。
- 就绪语义：`ready:false` 是正常答案（HTTP 200、无 `objectCount`、无错误码），内部状态名不外泄。
  验收第 3、30 条与设计决定 13。
- 诊断：`indexes:describe` 承接，处置方向选定"复用既有完整描述结构、只改路径与对外词汇"，而非新设计
  按需展开的窄响应。验收第 36、37 条。
- 待定项已定：`affordance.md` 的 U5、U7 里"未就绪"从短窗口变常态窗口后，交互改为"构建中，可查进度"，
  写在主任务的设计篇清单里；`concepts.md` 补"就绪"词条同样由主任务承担。

保留的约束（已写入主任务的"不做"）：

- 不把 `members`、`basis`、`coverage`、`state` 塞回 `indexes:ready`。
- 不给 `ready:false` 配 `reason` 字段或错误码。
- 不恢复 `projections:describe` 旧路径，不新增 `indexes:notice`，不给旧命令留别名或等价性回退。
- 不改内部包名、类型名与持久状态键。
- 改词不放宽也不收紧 `server/repository_readiness.go` 的检索状态判定。
