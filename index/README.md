# index/

**③ 检索派生**：仓之上的可丢投影（K-19）。配方来自 ② 的 `schema/*` AccessHints，编在 ① 钉死的 commit 上。不是协议对象，也不进 Writer / Catalog 核心。挂 git（⓪）不经过本包。

```text
Writer / ControlPlane     不 import 本包
Catalog.Hook              进程内挂载点（AfterSnapshot / AfterPin / …）
index.Index.AfterSnapshot CLI 组装成 Hook；Catalog 不 import 本包
index.Index               本地走 local.OpenSQLite；规模化走 scale.OpenElasticsearch（MATCH）+ 列投影（比较走 StarRocks stub）
index.Engine              物理引擎；schema 不写引擎名
                          Redis 目标是热尾缓存，不是比较、不是仓
```

出站 `kc hook-add`（`hook/`）是用户脚本/HTTP，不是本包。

一把索引对应一个 `(repository, basisCommit)`，外加该仓 `schema/*`。**不要按 View 建表。** live 工作投影跟着 `AfterSnapshot` / `Ensure`；消费 `SearchAt` 在 pin 上另开一份，不回绕 live。`IndexPlan` 只是 Generation 配方：SEARCH 扇出到各成员已有索引，同仓同 commit 的多个 View 共用一份物理投影。对象子集用查询 AND，不要 `view_id` 复制整列。查询入口是 `reader.SearchRequest`（原子算子，隐式 AND），不是 RQL。

本地检索：`stores.yaml` 写 `profile: local` 与 `index: sqlite`。规模化全文：`profile: scale` 与 `index: elasticsearch`。比较/列投影走 StarRocks，不走 Redis。命中后仍回读 Canonical。Redis 目标是 APPEND 热尾缓存，不是权威，也不是比较引擎。local profile 拒绝 Redis。

**声明式：** 唯一配方是该仓 pinned commit 上的 `schema/*` AccessHints（`access[]` + `type`）。`IndexSpec` 是编译结果。没有 Hint 不得把整包 JSON 灌进 FTS；`schema/*` 对象是配方不是文档；对象上的 `schema_ref` 选出用哪一份 schema。`DESCRIBE_INDEX` 返回编译后的 fields / lanes。物理引擎名不进 schema。

字段声明仍是 schema 上的检索面；`CheckSearch` / `AllowsOp` 决定某算子能不能打到该 path。

| Cause | 何时 | 怎么更新 |
|---|---|---|
| `content` | 知识对象 PUT/REMOVE，AccessHints 没变 | 只 upsert/delete 这些 `object_id` |
| `schema` | `schema/*` 上的 AccessHints 变了 | 按新 Spec 全量重抽 |

`COMMIT` / `merge` 在 Store 上发 Snapshot；Catalog 在构造时订阅，再打 `AfterSnapshot`。facade 只 `AddHook`，不补通知。PROPOSAL 不发。Pin / promote 不改成员正文，Sink 不挪 basis。
