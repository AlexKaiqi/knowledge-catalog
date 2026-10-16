# TASKS-MIGRATE-01 · 把 TASK.md 拆进 tasks/ 并退役

## 目标

根目录不再有 `TASK.md`。原来的每条任务各成一个文件；未完成的要么关闭并写明去向，要么转成待做；
所有提到 `TASK.md` 的规则、守卫和提示改指向 `tasks/`。

## 依据

- [工程质量闭环](../../doc/engineering/quality.md)：债的登记位置。
- 根目录 `AGENTS.md` 的提交前自检与交付第 4 条。

## 验收

- `make check-docs` 检查任务目录布局，根目录出现 `TASK.md` 时报错。
- 全仓 `TASK.md` 只出现在本文件与已归档的历史条目里。
- 原文未完成的条目与登记的债，在 `tasks/` 里都能找到去向。

## 前置

无（2026-10-15 用户中止对象模型重构的最终验收后执行）。

## 不做

不改写已归档条目的结论和词汇，只搬运、调整标题层级和相对链接。

## 交付

2026-10-15。原 `TASK.md` 2096 行、约 500KB，原文保留在 git 历史中。

- 归档：119 个历史条目各成 `done/<ID>.md`（含第 4 节的 RESEARCH-01、02）。正文逐字保留，只做了两处机械调整：
  `####` 标题上提两级，仓内相对链接按新位置改写。两条都叫 `DATASET-04` 的条目，后一条改名为
  `DATASET-04-CONSOLE-UI`。
- 关闭：46 条未完成或无勾选的条目归档时末尾加 `## 关闭`，写明原因与去向。按原因分四类：
  `REFACTOR-OBJECT-MODEL-01` 在验收前被中止，剩余部分拆成新待做；重构前按旧结构写的认领被后续重构
  取代；里程碑方向已在 `doc/roadmap.md` 的“未到”；评审问题已由现行设计回答。
- 转为待做：`GITEA-LOGIN-RETIRE-01`（原阶段 2b）、`ARCH-LAYOUT-01`（原阶段 7）、`FINAL-ACCEPTANCE-01`
  （原 A1–A6，A2、A3 按新设计改写）、`DEBT-SCENE-META-TESTNAMES-01`、`DEBT-EMPTY-PRINCIPAL-01`
  （原 QUALITY-03）、`DEBT-DATASET-DEFINE-IDEMPOTENCY-01`（原 CAP-ORG-01 遗留）、
  `DEBT-UI-REDIRECT-STATUS-01`（原 CLI-OWNERSHIP-CLEANUP-01 遗留），以及仍未裁决的 `REVIEW-01`
  （`.data/scale/CASES.md` 引用它）。原阶段 4a 剩余的字段 `access` 并入 `INDEX-DECLARATION-01`。
- 删除不归档：根文件的前言、第 2 节“普通用户自助闭环的产品目标”和第 4 节候选表。前者是任务约定，
  已写进 `tasks/README.md`；后两者是方向描述，已由 `doc/vision.md` 与 `doc/roadmap.md` 承担。
- 守卫：`scripts/check-docs` 去掉根目录的 `TASK.md` 白名单，新增任务目录检查。对 `TASK.md`
  这份“历史账本”的两处旧词与旧 URN 豁免（`internal/arch`）改为只豁免 `tasks/done/`，待做与进行中的
  任务仍受检查。豁免对象从一个文件换成一个目录，内容是同一批历史条目，范围等价。
- 已验证：见本轮交付说明中的检查结果。
