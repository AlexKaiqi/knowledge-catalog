# Dataset：跨仓文件清单

定位：整理稿。**还不是文档图节点**，不能覆盖 [`COMPOSITION.md`](../COMPOSITION.md)、[`TERMINOLOGY.md`](../TERMINOLOGY.md)。验收通过后再升格，并改写现行知识集（仓名单配方）合同。

本稿只定义 **Dataset 是什么**。谁能读这份清单见 [`dataset-authorization.md`](dataset-authorization.md)。两篇不得互相抄成一篇。

公开英文工作名是 Dataset（与 lakeFS 文件层精选视图同构）。升格时是否改回「知识集」只改词表，不改本文模型。所有 Repository 同质，不按接入方或内容类型分类。

---

## Goal

给消费面一个一级对象：把若干 Repository 上已存在的 **文件**（path 或 prefix）钉在具体 commit 上，命名、线性版本化、零拷贝，供只读消费。Catalog 组合层仍不解释 `object_id` / Aspect。

## Non-Goals

- 不拥有授权（另一篇）。
- 不写成第三个 Snapshot / Git 仓；不调用 lakeFS Enterprise Datasets API；不把 Iceberg REST、STS、S3 Gateway 交给 Client。
- 不认识知识身份，不用 Dataset 当 Writer target，不制造跨仓事务。
- 不为每个 `vN` 建检索索引（投影只跟当前服务版）。
- 不保留第二套「仓名单知识集」与 Dataset 并列。

## 硬性约束 / Invariants

- 成员是 `{repository, commit, path|prefix}`。发布时 source 必须是 commit，不能跟 live 分支。
- 一个 Dataset 内 target（消费者逻辑路径）唯一；两仓文件不得叠成覆盖。同一知识对象若两仓各有文件，两条 item，读侧并存。
- 每次发布单调 `v1, v2, …`；`latest` 指向最新已发布版。版本差的是指针清单。
- Catalog 只存不透明 path 指针，不出现 Aspect / 类型化 `object_id`（`L-01`）。
- 一次请求内 `latest` 只解一次（`V-01`）；分页不漂。
- 权威 blob 仍在源仓；被 Dataset 引用的已提交对象不得用 retention 清掉。
- 现行 `KS-01`（pin 只冻 `{仓 → commit}`）在升格时改为：消费 pin 冻的是 **某版本的文件清单**（从而导出 `{仓 → commit}`）。

## 选定方案 / 被否决方案

- 选定：文件层 Dataset，对齐 lakeFS 的 pointer + `vN` + `latest`；实现登记在 Catalog Snapshot 权威（与现行知识集同一落点、不同形状）。
- 选定：现行 kset / KnowledgeSet **收成** Dataset，不双轨。
- 选定：item 种类首版只有 `prefix` 与单文件 path；整仓消费用根 prefix。
- 选定：检索只维护 `latest` 对应的服务版；旧 `vN` 可 Snapshot 精确读，不进 SEARCH。
- 选定：投影按仓 + 服务 commit 建，Dataset 用 path 过滤，不按「版本 × Dataset」复制索引。
- 否决：对象清单进 Catalog；Dataset 当 Repository；每个版本一份 OpenSearch；跟分支的已发布版本。

## 接口契约 / 状态机

```text
草稿（可写 selector）
  → 发布：selector 解成 commit，写出不可变 vN
  → latest 指向该版
  → 日常消费 / SEARCH：服务版 = latest
  → 精确读 / 对账 / 指针 diff：指定 vN，走 Snapshot
```

登记、list、show 走 Catalog 协议缝（升格后的 `catalog` 类型与 README）。文件字节走固定 commit 的 `snapshot.TreeReader`。知识 Serving 只解释当前视图里实际读到的文件。写回仍只 `--repo`（`W-01`）。

参考实现落点（升格后改，不把现文件名当协议）：`catalog/` 登记与 resolve；File Gateway / `kcfs` 投影清单内 path。

---

## 1. 为什么是文件不是对象

组合层必须可裸用：没有 Schema 的树也能切、也能挂。Canonical 已是一 Address 一文件；切 path 即切那些单元的字节。身份仍由 Address 决定，路径不是 ID（`I-01`）。

不想对外的 Aspect：发布时不把该文件写入清单。这是策展，不是第二种授权模型。

## 2. 和 lakeFS Dataset

同构：命名视图、`(target, source@commit)`、线性版本、零拷贝、按名发现。不同构而不做：Iceberg item、安装级强制 metadata key、把 Dataset 名当成 S3 bucket、实现挂到 Graveler 产品 API。

## 3. 升格后对现行合同的替换

| 现行 | 目标 |
|---|---|
| `KnowledgeSet.sources` = 仓 + selector | item = 文件指针，发布冻 commit |
| `revision` 配方计数 | 系统生成的 `vN` |
| pin = `{仓 → commit}` | pin = 某版本文件清单 |
| `dataset define --source repo=main` | 根 prefix 的 Dataset，或显式子树 |

升格时改 [`COMPOSITION.md`](../COMPOSITION.md)、[`TERMINOLOGY.md`](../TERMINOLOGY.md)、[`ARCHITECTURE_INVARIANTS.md`](../ARCHITECTURE_INVARIANTS.md) 的 `KS-01`，以及 `catalog/` 公开类型。未升格前场景与代码仍以现行知识集为准。
