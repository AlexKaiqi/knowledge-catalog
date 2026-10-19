# README-ORDINARY-01 · readme 在系统中没有特殊语义

## 目标

`readme` 方面、`schema/core/readme/v1` 与文件名 `README.md` 在 Writer、Reader、codec 与协议信任根中都是
普通内容：任意多个对象可各带 `readme` 方面，按各自 `path_hint` 存放并可读回；没有 Writer 维护坐标的
`README.md` 与其它文件一样不是对象；Markdown 单元规则对所有方面一致。协议信任根只剩 Meta Schema 与
索引声明 Schema。

## 依据

- [对象](../../doc/design/object.md)：身份是 Address，路径不是身份；Schema 是对象。
- [概念](../../doc/concepts.md)：Aspect、Schema、path_hint。
- 2026-10-19 用户裁决：`README-01` 选定的“仓根 `README.md` 作为 git 直推解释的有界 path_hint、无 locator 时
  Reader 回退读仓根”已被否决，readme 在系统看来没有区别。`README-01` 已于 2026-10-15 未按原条目验收关闭。

## 验收

- `TestReadmeAspectIsOrdinary`：4 个对象的 readme 方面分别位于 `README.md`、`domains/ad/README.md`、
  `domains/xp/README.md`（另一 Schema）与默认路径，一次提交成功并各自读回。
- `TestGitCommittedReadmeWithoutLocatorIsNotAnObject`：直推的带 frontmatter 仓根 README 无 locator 时
  Read/ReadAddress/ReadReference 均 unresolved，对象分页为空。
- `TestReadmeIsNotReserved`、`TestReadmeSchemaIsNotInTheTrustRoot`、`TestAssertProtocolSchemaPublication`。
- `make check-docs`、`make quality`、`make test` 全绿无 skip。

## 前置

无。

## 不做

不改 System 仓出版物内容（readme Schema 与 `kc/system#readme` 实例照常发布，仅移到 `_schemas/` 普通内容）；
不改 Markdown 单元的 frontmatter 形状；不改真实仓包数据。

## 进展

- 删除的特殊处理：`object/readme.go`（`AssertReadmeBinding`、`IsReadmeAddress`、`ReadmeBody`）、
  `object/reader/readme_unit.go` 与 Reader 三处仓根回退（`objectUnitPathsMany`、`ObjectIDsPage`、
  `selectedReferenceUnits`）、`object/schema.go` 的 `CoreReadmeSchemaV1`/`ReadmeAspect`/`RepositoryReadmePath`/
  `SystemReadmeObjectID`、`object/format/layout.go` 的 readme 默认路径与 ingest 改址、codec 按 readme Schema
  判定 Markdown、Writer 对 readme 方面名与 Schema 的绑定校验、readme Schema 的 digest 锁定。
- 改为一致规则：codec 对所有方面按 `.md` 路径 + 值恰为 `{body}` 判定 Markdown；无 `path_hint` 的草稿以文件
  自身路径判定（`ParseAt`）；`AssertProtocolSchemaPublication` 只锁索引声明 Schema；readme Schema 文件由
  `object/trusted/` 移到 `systemrepo/content/_schemas/`。
- 删除的测试：`declarativeindex/projection/readme_git_internal_test.go` 与 `TestAssertReadmeBinding`，二者断言的
  是被否决的仓根直推解释与方面名保留，按用户裁决删除；其余测试中的常量引用改为字面量，断言不变。
- 先红：`TestReadmeAspectIsOrdinary` 在 HEAD 上报 `aspect readme requires schema`；
  `TestGitCommittedReadmeWithoutLocatorIsNotAnObject` 在 HEAD 副本上读到仓根 README 对象。改后两者通过；
  object、systemrepo、internal 包测试通过，`go vet ./...` 干净。
- 场景物料比对 `TestSceneSystemSchemaMaterialsMatchEmbed` 的 readme Schema 源路径随文件移动改为
  `systemrepo/content/_schemas/`；首次合同运行因旧路径失败，修正后通过。
- 真实仓包：原始 `business.tar.gz`（未删任何 README）在先提交 `base_mini.tar.gz` 的内存仓上一次提交成功，
  43 个 readme 方面全部从各自 `README.md` 路径读回；此前同包报 `OBJECT_ID_CONFLICT`。临时验证测试只在
  /tmp 副本运行，未入库。

## 交付

- 实现范围：见“进展”；未改 System 仓出版物内容与 Markdown frontmatter 形状。
- 已验证范围：基于 HEAD 仅含本轮文件的独立 worktree 中 `go vet ./...` 干净，`make check-docs`、
  `make quality`（coupling 14.31% 持平）、`make test`、`make test-contracts`（70 包通过、0 失败）全绿；
  本任务验收用例无 skip。合同运行中的 14 个 skip 均为既有 Live/Scale 环境开关用例，与本轮无关。
- 未验证：真实 lakeFS 部署上的 business 包上传（`make deploy-local-scenes` 等真实部署入口未运行）。
