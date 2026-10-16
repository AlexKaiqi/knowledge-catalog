# Dataset API review

复核日期：2026-10-10。对照的是 lakeFS 当前官方 Dataset 产品文档和 Enterprise SDK 公开
API；本目录的前端仍固定在 Apache-2.0 的 `v1.58.0`，未升级依赖或复制 Enterprise 实现。
Dataset 工作流通过 KC Snapshot 登记和 File Gateway 合同增加在完整原控制台上。

## 比较范围与权威

官方当前 [Dataset 文档](https://docs.lakefs.io/datasets/) 标明该功能属于 Team/Enterprise，
预置 Dataset 策略提及 `v1.89.0`，多仓来源提及 `v1.96.0`。这些说明不能用来宣称
[v1.58.0 的公开 OpenAPI](https://github.com/treeverse/lakeFS/blob/v1.58.0/api/swagger.yml) 已提供该产品：
该固定版合同没有顶层 Dataset CRUD、Dataset PR 或 metadata-key 路由。
原仓库、对象、提交、分支、标签、比较、仓 PR、Actions、Settings、Administration、
Setup 与原登录保留，依赖独立 lakeFS 后端和用户自己的 lakeFS 身份。
KC Server 不原生提供所有 lakeFS API；生产分流与凭证边界见 [KC-VENDOR](KC-VENDOR.md)。

本轮 Goal 是用 KC 已选定合同补齐可复用的控制台交互，清楚记录与官方 API 的差距。
Non-Goals 沿用 [Dataset owner](../../../doc/design/dataset.md) 的文件层边界与
[Service owner](../../../doc/design/service.md) 的同一应用入口：不增加发布权威、跨仓事务、
隐式发权、新的文件直读通道或声称 wire 兼容。对应不变量是
[KS-01 / KS-02 / AUTH-02 / API-01 / IX-01](../../../doc/engineering/architecture-invariants.md)。
接口锚点为 [Snapshot Dataset 合同](../../../snapshot/dataset/README.md)、
[Dataset 文件来源类型](../../../snapshot/dataset/definition.go)、
[DatasetItem](../../../snapshot/dataset/dataset.go) 和 [Server 管理详情](../../../server/README.md)。

## 能力对账

| 官方对照 | KC 当前合同与控制台结果 | 判定 |
|---|---|---|
| 顶层 Dataset 发现、创建、更新 | 新增 Datasets 顶层导航，登录后列出和发布 Dataset。名称在一套服务的唯一登记空间内有效。 | 既有 KC 能力已接入；保留命名语义差异。 |
| 不可变版本、固定来源 | 发布走 `DatasetPublisher` → `PrepareDataset` / `PublishDataset`；来源固定 commit，版本必须前进，已打开的 pin 保持原范围。KC revision 由请求显式指定，允许缺号。 | 复用完成；不假装为官方的系统递增版本号分配器。 |
| 文件 / 前缀条目、多仓数据 | KC `file` / `prefix` 条目对应单文件和目录范围，可跨仓、重命名、重组交付目录；同仓的多个片段共用一个固定 commit。冲突拒绝，不按顺序覆盖。 | 复用完成。官方 `object` 不是 KC 的 Object Entity；请求字段名不兼容。 |
| 表单 / JSON 编辑、发布前预览 | 控制台支持两种编辑模式，用既有 `datasets:resolve` 校验候选来源并预览固定提交；发布使用确认的 commit。任何定义变化使旧预览失效；未知 JSON 字段明确拒绝。 | 复用完成；预览不等于发布接受。 |
| History / 历史定义 | 原详情 GET 增加可选 revision 点查；返回真实发布版本页、该版来源与当前服务版。旧登记表只承认真实存在的当前版，不补造更早历史。 | 管理可见性缺口已修复；发布消息 / 作者 / 时间仍有差距（D-04）。 |
| 版本比较 | Compare 点查两个已接受版本，对交付条目展示新增、移除、修改，可查看完整来源定义。 | 复用完成；是客户端定义比较，不是官方结构化 diff endpoint，也不是文件正文 diff。 |
| 元数据 schema、description | KC Dataset 公开定义只承载身份、版本、来源、文件清单与生命周期，不含 Dataset description 或 installation-wide metadata keys。Object Schema 不能替代这个合同。 | 未实现（D-01）；不静默接收之后丢弃。 |
|Dataset PR 打开、修订、关闭、合并发布 | 保留原仓 PR 页；KC 的工作分支与条件合并只作用于一个源 Repository 的 Writer 写入，不是 Dataset 定义 PR。 |Dataset 审批未实现（D-02）；直接发布不宣称已审批。 |
| Iceberg table / namespace | KC Dataset 停在原始文件层，没有 table/namespace 条目或 Iceberg REST catalog 消费合同。选入 metadata.json 只是文件交付。 | 未实现（D-03）；不伪装成表级读取。 |
| 数据读取与授权 | 交付树、分页、Base64 字节预览和小文件下载统一走 KC File Gateway；pin 是坐标，各请求检查当前授权。退役保留发布记录而停止后续消费。 | 复用完成；KC 没有官方 Dataset`lakefs://` / S3 / Iceberg FQN 兼容消费通道（D-05）。 |

官方字段与路由依据：
[DatasetsApi](https://pydocs-enterprise-sdk.lakefs.io/docs/DatasetsApi.html)、
[DatasetDataItem](https://pydocs-enterprise-sdk.lakefs.io/docs/DatasetDataItem.html)。
两者仅用作能力比较，不成为 KC 路由或类型的第二份权威。

## 已补缺口与回归边界

历史来自 `dataset.State.DatasetVersions` 中的已接受记录；点查使用原有
`GET /snapshot/v1/datasets/{dataset}`，无新发布路由或 Client DTO。
`versions` 沿用整数数组，但不再无界返回所有历史：复用 `PageLimit` 的默认 50 /
上限 200 及 `limit` / `continuation` 词汇，游标绑定登记权威、Dataset 和服务版基点。
后续发布后要求重新打开历史；单个 revision 点查不受页面限制。
管理读取保留 [权限 owner](../../../doc/design/permissions.md) 的当前选择：登记库存对部署中的已认证主体可发现；登记审计仍检查 `registry.audit.read`，历史来源不赋予文件正文权。
进一步统一历史 / 来源可见性的策略仍是 owner 已记录的待 review 事项，本轮不暗中选定。

为这些改动增加的定向回归：

- `TestDatasetManagementViewSelectsOnlyAcceptedHistory` / `PreservesLegacyAcceptedCurrentVersion`：固定来源、缺号拒绝、当前退役与旧登记表。
- `TestDatasetManagementHistoryPagesAcceptedRevisionsWithHardLimit` / `ContinuationRejectsChangedBasis`：零值取默认、200 上限、缺号、跨 scope 与陈旧游标。
- `TestDatasetConsoleManagementHistoryAndByteContract` / `TestRegistryAuditRequiresCurrentGrant`：经 Server 的真实历史、分页、具体错误码、认证 / 登记审计授权、已发布原始字节与退役。

上述 Go 定向回归已执行到绿；前端、总合同、质量门禁与浏览器证据以本轮
[任务记录](../../../tasks/done/DATASET-04.md) 为准。夹具成功不代替生产同源网关、外部 SSO 或真实 lakeFS 部署验收。

## 剩余差距登记

下列项目是具体产品差距及其处置入口，不代表已立项实现。

| ID / 位置 | 信号 | 判定 | 处置 | 触发条件 |
|---|---|---|---|---|
| D-01 · `dataset.Dataset` / `server.catalogDatasetRequest` | 官方 [metadata-key](https://pydocs-enterprise-sdk.lakefs.io/docs/DatasetMetadataKey.html) 合同有类型、required、allowed values、allow multiple 与说明；KC Dataset 没有对应字段。 | 不能把 Object Schema 或 UI localStorage 充当 Dataset 元数据权威。 | 登记产品差距；JSON 不支持字段拒绝，在 Dataset owner 决定保存、验证、更新与治理所有权后再补公开合同。 | 用户需要 Dataset 用途说明或安装级必填元数据管理。 |
| D-02 · `writer merge` / `objectapp.DatasetPublisher` | 官方 [Dataset PR](https://pydocs-enterprise-sdk.lakefs.io/docs/DatasetPullRequest.html) 有定义候选、base version 与 stale/conflict 判定；KC 条件合并的对象是一个源 Repository 的工作分支。 | 源内容审批与 Dataset 发布审批不能混用。 | 保留原仓 PR，不添加无后端的 Dataset 审批按钮；后续在 owner 中选定审批范围、证据、基点与合并采用，终态仍请求唯一登记发布权威。 | 需要在发布 Dataset 定义前执行审批。 |
| D-03 · `dataset.DatasetItem` / File Gateway | 官方 Dataset 支持 table / namespace；KC 为 file / prefix。官方表消费依赖 [Iceberg REST catalog](https://docs.lakefs.io/iceberg/)。 | 文件指针不保证表语义、namespace 范围或数据文件完整性。 | 登记独立消费合同缺口；不将表解析塞入现有文件组合层，先选定上层能力、快照 / 文件保留及授权语义。 | 需要 Spark / Trino / PyIceberg 等按 Dataset 地址读表。 |
| D-04 · `dataset.Dataset` / `dataset.Log` / `invocation.RegistryLogEntries` | 官方 [DatasetVersion](https://pydocs-enterprise-sdk.lakefs.io/docs/DatasetVersion.html) 有发布 message、author、creation_date；KC 已接受定义未存这些字段。Registry audit 有提交作者 / 消息 / 证据关联，但包含退役等注册动作，无精确的发布版本时间合同。 | 不能将 registry commit 列表或客户端时钟伪造成 release 元信息。 | 当前 History 诚实显示真实版本号、来源与生命周期；后续可复用发布事件证据，需先选定与接受记录的持久关联及迁移合同。 | 需要逐版展示发布者、变更说明与时间或审计导出。 |
| D-05 · `dataset-files/v1` / 生产网关 | 官方 Dataset 使用 `lakefs://` / S3 与 Iceberg FQN；本控制台读取 KC File Gateway 字节。原 viewer 的 S3 路径只属于原 lakeFS 仓库页。 | 网关分流不使 KC Dataset 自动获得 lakeFS 消费协议兼容性。 | 按 KC-VENDOR 配置原控制台后端；若增加消费协议 adapter，仍需走同一发布范围与当前授权。 | 需要既有 S3 / lakeFS / Iceberg 客户端以 KC Dataset 坐标消费。 |

本轮选定复用已接受记录、typed executor 和 File Gateway。否决按1..N补造历史、
无界历史数组、前端自建发布账、注入平台管理员凭证、用源仓 PR 替代 Dataset 审批，
以及把普通文件交付宣称为 Iceberg 表消费。
