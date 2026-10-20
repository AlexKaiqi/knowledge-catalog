# SYSTEM-EXAMPLE-ANCHOR-01 · 判据指南用固定 revision 指回公开样例

## 目标

系统仓里讲"怎么判断"的正文，每一条判断都能让读者按固定 revision 看到实例，不需要自己去样例里翻找对应关系。

当前状态：`guides/first-publication` 已经做对了这件事（`kc dataset show autumn-launch-kit --revision 3`，随后同 revision 列目录、读 Schema 与实例）；但 `guides/model-from-materials` 通篇没有指向任何具体样例，"一个对象还是两个对象"这类判据在系统仓里只有抽象表述，读者无法核对。

## 依据

- [对象](../../doc/design/object.md)：对象边界、切面与成员、关系、Schema 复用的既有判定表述。
- 系统仓 `guides/first-publication`：以固定 revision 引用公开样例的既有写法。
- 公开样例 README：六个仓、三个 Dataset 的构成与各自演示的形态。

## 验收

- `model-from-materials` 的每个主要判断点至少指向一处具体样例坐标（Dataset 名 + revision + 路径），且该坐标在样例当前交付中可读；坐标写法与 `first-publication` 一致，固定 revision 而不跟随最新。
- 至少覆盖：同一名词在不同上下文指不同对象、表与业务实体是两个对象、需要单独寻址才做成员、关系与方面之别、Schema 复用与新建之别。
- 引用的 revision 与文件路径经实际读取核对存在，不在正文写"例如某仓有类似内容"这类无法验证的指代。
- `go test ./systemrepo/...` 与 `make check-docs` 全绿；样例侧不改动内容。

## 前置

无。改 `systemrepo/content/guides/model-from-materials/guide.md`，与 `SYSTEM-MODEL-GUIDE-02`、`SYSTEM-GUIDE-EXECUTABLE-01` 涉同一目录，按序执行。

## 不做

不改样例仓内容或为其新增对象来迁就正文；不在系统仓复制样例正文；不把 revision 写成会随样例前进而失效的浮动引用。

## 方案

保留与设计同源的判据段落，在指南中增加按五类判断组织的固定版本核对入口。
使用现有公开样例发布计划，在临时本地部署中经真实 Writer、Dataset 发布与文件读取接口验证坐标；
正文不宣称从材料中无法证明的建模动机，也不把三个语境说明误写成已发布的三个订单实例。

## 交付（2026-10-20）

指南新增五类判据的核对步骤，明确固定 `qinghe-analysis` r2、`store-oncall` r2、
`autumn-launch-kit` r3 的 18 个文件坐标，以及修订或路径不可读时的处置；未改样例内容。
对象边界的设计同源段保持不变，样例不替代使用者的边界判断。

`cli/model_guide_examples_test.go` 在本地临时部署中执行现有样例发布计划，经公开服务读取系统指南，
从正文提取坐标并逐项下载，核对回执的 Dataset、revision、path。五类独立断言分别核对语境说明、
商品/表身份与关系端点、成员键与普通字段、服务方面与活动关系，以及 Schema 复用/不同结构契约；
下载的 Schema 经公开解析与实例验证。不能以仅存在源文件或返回成功替代判断依据。

`go test ./cli -run '^TestPublicModelGuideExamplesSupportDecisions$' -count=1 -v`、
`go test ./systemrepo/... -count=1`、`make quality`（含 `make check-docs`）通过，无测试跳过。
证据：`.validation/remaining-20261020/model-examples-cli.log`、
`model-examples-systemrepo.log`、`model-examples-quality.log`。
验证使用测试专属 lakeFS HTTP 夹具及真实发布/读取链，未访问或改动既有服务、未部署到开发机、未提交。
