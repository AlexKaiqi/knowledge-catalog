# INDEX-READY-01 · 一次调用问清某个修订能不能用索引

## 目标

维护者与调用方能在不构建任何东西的前提下得到"这个 Dataset 修订现在能不能搜"的确定答案。

## 关闭

2026-10-22 并入 `INDEX-SURFACE-01`，未按本文件验收。

本文件的全部验收（未就绪返回 HTTP 200 且 `ready:false`、已就绪 `objectCount` 与描述接口一致、省略
`revision` 取当前服务版、CLI 与 HTTP 字段一一对应、`kc index --revision` 无 `--dataset` 返回
`USAGE_INVALID`、只有 `file.read` 可用且受 Dataset 发现范围约束）逐条保留在 `INDEX-SURFACE-01` 的验收里。

并入理由：本条与 `INDEX-SURFACE-01`、`DEBT-INDEX-READY-DIAGNOSE-01` 改同一批文件（`server/` 路由、
`cli/` 命令声明、`declarativeindex/projection/describe.go`、搜索视图、`server/routes_golden.txt`），
三者是同一个 `index` 命令家族的三部分：命名、就绪、诊断。分成三条时，每条都要声明"不做另外两条"，
读者要靠三个文件拼出一次改动。合并后一句话能说清：索引对外只有一个入口，就绪与诊断是它回答的两个问题。

原文中已由 `INDEX-SURFACE-01` 承接但表述变化的两处：

- 原 `ready:false` 不加 `reason` 字段、不加错误码、`BUILDING`/`FAILED` 不区分的约束，保留在
  `INDEX-SURFACE-01` 的"不做"里，语义未变。
- 原"就绪以外的诊断见 `DEBT-INDEX-READY-DIAGNOSE-01`"改为直接给 `indexes:describe`：该债同时并入，
  不再是"以后再说"。
