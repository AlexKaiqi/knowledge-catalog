# README-01 · README 是 Markdown 知识对象

- [ ] 本轮认领：README 是 frontmatter 知识对象；约定路径可被 ② 解释；默认 SEARCH 编该对象 `body`；已接入仓直推 published HEAD 后 ③ 能编进投影。`catalog show` 不派生 title/summary。owner 为 [`doc/design/object.md`](../../doc/design/object.md)、[`doc/concepts.md`](../../doc/concepts.md)、[`doc/design/dataset.md`](../../doc/design/dataset.md)、[`doc/design/discovery.md`](../../doc/design/discovery.md)。

**Goal：** 人写的 README 就是知识。展示仍叫 README；身份是 Address + `schema_ref`。ingestion control（面 3）只追 HEAD 并更新索引，不成为写面。

**Non-Goals：** 不实现 LLM concat；不把无 frontmatter 的文件当成知识；不让直推等于 Writer 校验；不在面 3 跑 Writer；不新增 Writer validate-only / 保护分支门（见 WRITE-VALIDATE）；不把 VFS 解释成 Catalog 成员条件；不改交付链；不把根 README 做成第二条检索代数；不把 README 展成 Catalog `title`/`summary`。

**不变量：** Address 是身份、路径只是 `path_hint`（`A-01`）；`catalog.read` 可见库存身份（`id` / `schemaCount`），README 正文仍要 `knowledge.read`；live 投影对 published HEAD 对账（`PC-01`）；1 与 3 彼此不调用；缺 README 不得由平台或模型补写，也不从库存抹仓。

**选定与否决：** 选定 `schema/core/readme/v1` Aspect `readme`、字段 `body`（`text`）。根 `README.md` 只当人写与 git 直推解释的 `path_hint`。Connector / HTTP / Agent 仍走 Writer。否决 `core/source-profile` 信封；否决库存 title/summary；否决无 frontmatter 即身份；否决文件 contains；否决 `ChangeSet → Writer → Snapshot → Ingestion → Index` 当一次写事务。

**怎么改：**

1. ② Schema + codec：`schema/core/readme/v1`；Markdown frontmatter 用 `entity`/`aspect`；正文是 `body`，不是 JSON 信封。
2. ② 解释：无 Writer locator 时仍按仓根 `README.md` 有界读取；`Read` / `Resolve` / 对象分页能看见该 Address。
3. ① 库存：`catalog show` 只列 `{id, schemaCount?}`；不从 README 派生 `title`/`summary`。读 README 走 `kc read --aspect readme`。
4. ③ 投影：`specAtCommit` 带上 `body` 的 `text`；git 直推的对象编进 `CompiledDoc.Text`。Controller 仍 Desire(HEAD)，失败不回滚仓。
5. VFS ≠ attach：文档钉死 Plain 只解释组合阶梯。

**接口指向：** `knowledge.CoreReadmeSchemaV1`、`object/readme.go`、`object/format` codec、`index.compileValue`。不新增 CLI。

**如何验证：** `make check-docs`；下列测试无 skip，再 `make test`。

- `TestSystemRepositoryPublishesReadmeSchemaAndInstance` / `TestAssertReadmeBinding`
- `TestParseMarkdownReadmeFrontmatter` / `TestIngestMarkdownReadmeFrontmatter` / `TestReadmeAspectIsMarkdownBody`
- `TestReadmeGitCommitWithoutWriterLocators` / `TestPlainMarkdownReadmeIsNotKnowledge`
- `TestCatalogShowRepositoriesStayIdentityAndReadmeIsKnowledge`
- `TestProjectionCompilerIndexesReadmeBodyAsText` / `TestGitPushedReadmeCompilesIntoAccessSpecText`
- `TestProductScenes/catalog-initialized`（库存身份）/ `TestProductScenes/system-schema-published`（`kc/system#readme`）

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。早于 2026-10-14 对象模型重构的认领，相关代码与设计已被后续重构改写（见 `0759640` 之后的提交），原范围按旧结构写成，不再继续。整体验收并入 `FINAL-ACCEPTANCE-01`，插件相关用例由验收规格 accept-01～13 取代。
