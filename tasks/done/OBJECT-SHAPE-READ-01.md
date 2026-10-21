# OBJECT-SHAPE-READ-01 · 按 GraphQL 形状读取对象的一部分

## 目标

精确读取接受一个 GraphQL selection set 形状，结果与形状同构：方面、成员（`keys` 参数）和字段三层可选。
方面与成员层只凭固定版本的维护坐标规划单元，未选单元不读正文、未选取值来源不外呼；字段层在所选单元内裁剪。
选中但拿不到的位置为 null，并在错误列表按路径说明缺失、无权、冲突或范围外；每个所选单元带完整维护摘要。
CLI、HTTP 与 Go client 共用同一形状解析。先整读再裁剪的 `object.AspectSelector`、`SelectAspects`、
`Reader.Read` 的 selector 参数与无调用方的 `invocation.AspectSelectorFrom` 删除。

## 依据

- [对象 · 形状](../../doc/design/object.md#形状只读需要的部分)，用例 U11、U12、U21
- [LiveAccess](../../doc/design/liveaccess.md)：值形状一致，只为所选单元取值
- [发现](../../doc/design/discovery.md)：搜索回读仍取完整对象，不受形状影响

## 验收

- U21 有场景用例与回归测试：同构结果；未选单元零正文读取、零外呼；四类错误按路径区分且不与合法 null 混淆。
- 采用 Schema 时未知名字或层级错误整批拒绝；无 Schema 时报缺失；别名、fragment、directive、变量、
  空键、重复选择以及“定位符加形状”均拒绝。
- 所选单元摘要可直接用于条件更新，旧摘要在单元其他字段变化后被拒绝（与 U12 同口径）。
- Dataset 范围：范围外方面报范围外，不补读源仓（与 U7、U16 同口径）。
- 不带形状的读取行为与现有合同一致，原有 conformance 不削弱。
- `make check-docs`、`make quality`、`make test`、`make test-contracts` 全绿无 skip。

## 前置

无。GraphQL 文本解析引入 `github.com/vektah/gqlparser/v2 v2.5.30`（连带 `github.com/agnivade/levenshtein`），2026-10-16 经用户批准。

## 范围调整

- `object.AspectSelector`、`SelectAspects` 与各读取签名中的 selector 参数退场，改由 `tasks/todo/DEBT-ASPECT-SELECTOR-RETIRE-01.md`
  承接：生产代码已无非空调用方，但签名波及的调用方与并行任务改动重叠。本任务只删除无调用方的 `AspectSelectorFrom`。

## 进展

- 2026-10-16 形状解析与编译：`object/shape.go`（ParseShape、Selections、ProjectShapeFields、ShapeSchemaMismatches）。
- 执行：`object/service/shape_read.go` 每条选择各走一次引用读取（先规划、只读所选单元、只为所选单元外部取值），
  单元读出后立即按字段裁剪；无权、冲突、缺失按路径进 errors，其余失败整批拒绝；Schema 来自所选单元声明，
  请求范围外的 Schema 按开放处理。
- 接线：`invocation/shape_read.go`、`invocation/verbs_read.go`、`invocation/reference_target.go`（定位符与 `--shape` 互斥、
  按选择逐方面判可读持有仓）、`invocation/flags_hygiene.go`（`shape` 进 transport 词表）、CLI / HTTP / Go client 的 `shape` 字段与帮助。
- 修复：`VerbRead` 在 flag 规范化前取 reference，`--repo` 范围没带进形状读取，选择被当缺失、Schema 校验不执行；
  由 `TestShapedReadEnforcesTheAdoptedClosedSchema` 先红后绿。
- 设计补充（`doc/design/object.md` 形状一节）：封闭与开放 Schema 的区别、Schema 校验只到顶层且发生在所选单元读出之后、名字写法。

## 证据

- 定向回归（2026-10-16 全绿）：`TestParseShape*`、`TestProjectShapeFieldsSeparatesAbsentFromLegitimateNull`、
  `TestShapeSchemaMismatchesCoverNamesAndLevels`（object）；`TestShapeRead*`（object/service）；
  `TestShapeReadLoadsOnlySelectedUnitBodies`、`TestShapeInsideDatasetLocatesOutOfScopeAspectWithoutItsBody`（object/reader）；
  `TestShapeHydratesOnlySelectedBindingAspects`（liveaccess/service）；`TestShapedComposedReadReturnsOnlySelectedPartsWithTheirSources`、
  `TestShapedReadEnforcesTheAdoptedClosedSchema`、`TestShapedReadDigestDrivesConditionalUpdate`、
  `TestShapedReadRefusesLocatorAndInvalidShapesBeforeReading`（invocation）。
- 场景：`semantic-object-published/_probes/probe-shaped-read.feature`、`probe-shaped-read-refusals.feature`，
  `go test ./cli -run 'TestProductScenes/semantic-object-published$'` 绿；`tree.py --check-product / --check-views / --check-states` 通过。
- 包级：`go test ./object/... ./invocation/... ./liveaccess/... ./server/... ./cli/... ./internal/arch/... ./discovery/...` 绿。
- 门禁（2026-10-19）：`make check-docs`、`make quality`、`make test` 均 exit 0。
- `make test-contracts`：第一次完整运行唯一失败为 `object/service` 的 `TestExportedSurfaceMatchesAnchor`
  （新增 `AdoptedSchema`、`ShapeReadExecutor`、`ShapeReadRequest` 未登记），已补锚点与包 README，`go test ./object/service/` 绿。
  修复后重跑未开始执行：并行压测容器占用 Docker VM 内存，自动 OpenSearch 夹具预算不足（可用 931008512，需 1342177280 字节）。

### 收口（2026-10-19 第二次）

- 并行线全部停止、Docker VM 内存释放（可用约 2.7GB）后，在主工作区重跑全量门禁：
  `make check-docs` PASS、`make quality` 通过、`make test` exit 0、`make test-contracts` exit 0
  （`passed-with-skips`、`sourceChanged=false`、5010 项通过、0 失败、14 项测试级跳过均为既有外部
  live 用例，与冻结候选口径一致；报告 `.validation/runs/20261019T091635Z-12c55ff2bf3e/`）。
- 锚点修复后的 `TestExportedSurfaceMatchesAnchor` 随合同通过。全部验收项闭合，移入 `done/`。

## 不做

- 跨关系展开；形状作为对象发布或命名复用（已定为随请求写）；写入侧按形状部分更新（设计标待确认）。
- 搜索结果形状。
- 别名、fragment、directive、变量等 GraphQL 投影以外的能力。
