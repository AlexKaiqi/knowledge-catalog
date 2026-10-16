# DATASET-01 · 文件层 Dataset 与消费授权（取代 KSET-01）

KSET-01（Workspace→知识集、不改组合语义）不再认领。消费组合改为文件清单 Dataset；授权与组合分成两份整理稿，**验收通过后才升格进文档图并改 owner**。

- [x] 当时两份 Dataset 整理稿已完成；后续文档重构由 DOC-21 接手，当前替换稿见 [`Dataset`](../../doc/design/dataset.md) 与 [`权限体系`](../../doc/design/permissions.md)。本条历史完成记录不表示替换稿已完成交接或验收。
- [x] 实现与 owner 升格已落地：发布冻 commit（无 store / 解不出 selector 失败关闭）、Items 为消费真相（空清单不放行整仓）、`file.read`×Dataset、Serving/SEARCH 按清单 path 过滤（hydrate 前）。公开动词是 `dataset define|retire|overlay`；消费是 `file.read`；管理/解析是 `dataset.manage` / `dataset.resolve`。产品面只有 `--dataset`、`KC_DATASET`、HTTP `/datasets`、JSON `datasets`；不保留 `kset` argv、`--kset`、`KC_KSET`、`kset.consume` 或 HTTP `/ksets`。

**Goal：** 跨仓只读切片是命名、版本化的文件清单；检索只跟 latest；消费者 `file.read` 挂在 Dataset 上，维护仍挂 Repository。

**Non-Goals：** 不接 lakeFS Datasets API；不把 Dataset 做成 Snapshot 仓；不为每个 `vN` 建索引；不把 path/Aspect 做成 allow 资源。

**不变量（目标，升格后写入不变量表）：** 清单 item 发布时钉 commit；消费 pin 冻文件清单；`file.read`×Dataset 与仓 `knowledge.read` 互不蕴含；pin 不锁权限；SEARCH 不超出服务版 path。

**选定与否决：** 选定文件层 Dataset 收掉仓名单 kset；消费 action 是 Dataset 上的 `file.read`。否决双轨 kset+Dataset、消费再要整仓 `knowledge.read`、每文件一行 grant。
