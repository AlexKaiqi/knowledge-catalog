# access-handle-published

Bound State 的访问路径写在 Domain Schema Canonical frontmatter 的 `origin`。实体对象只要有身份；不另存 `null` Aspect。`kc access --object <实体ID> --aspect <name>` 用 origin + 实体 ID 取回该 Aspect。

接入方墙外三个容器在 `_materials/accessor/`（`compose.yaml` + 代码）：

- Resource Access：`access.py`，`POST {origin}/v1/access`。走查 MySQL 上 `schema/table.stats` 按实体 ID 覆盖全部 `table/{database}.{table}`
- Collector：`collector.py`，对账 INFORMATION_SCHEMA 后 Writer 更新 `table-meta`
- Observer：`observer.py`，对每张活表的 `stats` 发 change notice

本节点 construct 仍把 origin 写成 `http://127.0.0.1:9`，证明未起容器时 access 失败关闭。走查叶 `access-runtime-ready` 用 `goto.py` 起本目录 compose，并把 table-meta origin 写成 `http://resource-access:7390`。

ResourceDescriptor 仍是带操作输入的知识对象，给 `kc invoke`。

构建与探：`TestResolveBindingFromSchemaWithoutInstance`、`TestCatalogViewsChecksAndKnowledgeResolve`。
