# 声明式索引五类数仓实体 E2E（2026-08-24）

## 结论

本地 SQLite 声明式索引验收通过。测试从 Writer 写入开始，跨过 Snapshot、Catalog Hook、Index、Workspace pin，最终由索引定位并回读 Canonical；不是只调用 `SpecFromReport` 的单元测试。

覆盖实体：`Table`、`Column`、`Metric`、`Dimension`、`Measure`。

## 覆盖链路

1. 在 physical、semantic 两个知识 Repo 写入五份 `schema/*`。
2. 五类实体的 Aspect 通过 `schema_ref` 绑定各自 Schema。
3. Workspace 在两个 pinned Repo commit 上生成两份投影配方，合计编译五份 Schema。
4. 验证全文、精确过滤、范围比较和排序均只使用 AccessHints 声明过的字段。
5. 验证未声明的 `unit` 查询返回 `CAPABILITY_UNSATISFIED`，不退化成 JSON contains。
6. 普通知识变化后投影以 `incremental/content` 更新。
7. Measure AccessHints 增加 `unit: filter` 后投影以 `rebuild/schema` 重建，新查询随即生效。
8. 旧 Workspace pin 仍返回旧 Metric 内容，并且不会回退 live 投影。
9. Table 命中结果包含未入索引的 `canonical_note`，证明命中后回读 Canonical。

可重复测试：[`validation/workbench/declarative_index_test.go`](../workbench/declarative_index_test.go)。

## 本次执行

```bash
KC_DECLARATIVE_INDEX_REPORT=.data/validation/declarative-index-five-entities.json \
  go test ./validation/workbench \
  -run TestDeclarativeIndexFiveWarehouseEntities -count=1 -v

go test ./validation/workbench -count=1
```

结果：专项测试 PASS，完整 workbench 回归 PASS。机器可读结果写入 ignored 文件 `.data/validation/declarative-index-five-entities.json`，其中记录本次 commit、实体命中、同步 mode/cause、旧 pin 与 Canonical 断言。

## 边界

- 本次物理引擎是 `local.OpenSQLite`。
- Elasticsearch 真实服务不在本次执行环境中，不把 skip 当 PASS。
- StarRocks 列索引仍是 stub，不在本次支持声明内。
- 这份记录是产品功能验收；`kc record-validation` 仍只用于 Proposal merge gate 证据，不是测试运行器。
