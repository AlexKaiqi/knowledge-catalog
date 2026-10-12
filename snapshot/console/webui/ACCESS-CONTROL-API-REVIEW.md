# Access Control API review

复核日期：2026-10-10。本文件记录完整控制台的权限接线与能力差距，
不是新的授权设计或第二份协议权威。前端仍固定在 Apache-2.0 的 lakeFS
`v1.58.0`，不升级依赖；KC 使用自己的身份与授权模型。

## 范围与合同依据

Goal 是保留原控制台全部管理入口，并把 KC 既有的主体查权、显式授权规则列表、
新增与撤销接入控制台。身份仍由原 lakeFS 登录会话经 KC Server 验证，
每次 KC 请求独立检查当前授权；原平台管理员身份不赋予 KC 管理权限。
身份映射与同源部署边界见 [接线说明](KC-VENDOR.md)、
[控制台验证范围](CONSOLE-VALIDATION.md) 和 [Dataset review](DATASET-API-REVIEW.md)。

依据 `docs/graph` 的 `authentication-authorization-and-repository-security` owner
及 Service → Permissions 直接关系，沿用
[权限 owner](../../../docs/reviewed/permissions.md) 与 [Service owner](../../../docs/reviewed/service.md)。
Non-Goals 是新增 KC 用户目录、组、角色、命名策略对象、对象 ACL、deny 语言、
隐式授权继承、lakeFS 与 KC 授权同步，或申请/审批队列。
对应不变量为 [AUTH-01 / AUTH-02 / AUTH-03 / API-01](../../../docs/reviewed/architecture-invariants.md)。

已选定形状与执行边界以 [AllowRule / AllowQuery](../../../snapshot/access/rule.go)、
[Snapshot 授权管理](../../../snapshot/access/grants.go)、[Client GrantRequest](../../../client/management.go)、
[AdmissionResult](../../../client/admission_sharing.go)、[Server 管理路由](../../../server/service_management_routes.go)
和 [CLI 查权/发权合同](../../../cli/README.md) 为准。没有为 UI 方便新增授权权威。
规则库存返回 [GrantList](../../../snapshot/access/grants.go) 的 `rules`，不再公开整个
`AllowFile` 内务账。Client 的范围列举复用 GrantRequest 的已有坐标，
不新增授权规则或动作求值协议。

## 官方对照与版本边界

[v1.58.0 官方 OpenAPI](https://github.com/treeverse/lakeFS/blob/v1.58.0/api/swagger.yml)
有 Users、Groups、Policies、用户/组策略关联、组成员、Credentials 与组 ACL 接口。
其 Policy Statement 为 `effect`、`resource`、`action`，支持 allow/deny；
ACL 使用 Read / Write / Super / Admin 级别。固定版 Statement 没有当前文档的
condition 字段；不能把较新条件策略当作 vendored 前端已有能力。

当前官方 [RBAC 文档](https://docs.lakefs.io/security/rbac/) 描述用户/组关联策略、
deny 优先以及条件/ABAC，并标明产品版本与 edition 限制。
当前 [Community Access Control](https://docs.lakefs.io/security/access-control-lists/)
说明社区版为单管理员，ACL server 已移除；额外用户、组、策略与 SSO 属于企业能力。
当前产品说明不回写 v1.58.0 的历史合同，Swagger 中存在接口也不证明当前部署支持它。

| 官方能力 | 本控制台接线 | KC 判定 |
|---|---|---|
| Users / Groups / group membership | 原 Administration 页面与 `/api/v1/auth/*` 直接复用原 lakeFS 后端和当前用户会话。 | KC principal 是核验后的主体标识，不是可在本地创建的 lakeFS 用户；组/角色归外部身份系统。 |
| Policies / user & group attachments / effective policies | 保留原页面与原 API；原 API 返回的 edition / RBAC 配置仍决定能力。 | 不翻译成 KC Rule，不将 native policy 或 Admin 自动导入 KC。 |
| Credentials | 原凭证页继续由 lakeFS 管理原访问凭据。 | 不注入 Server adapter 管理密钥，不生成第二套 Dataset 登录凭据。 |
| ACL Read / Write / Super / Admin | 保留原后端支持的原入口。 | KC 不采用这组角色等级；显示明确的 semantic actions 与资源约束。 |
| KC 主体自查 | 复用 `GET /identity/v1/admission`，展示当前 principal、显式规则摘要与可选外部申请地址。 | 不是全部有效权限求值结果；不会托管申请或审批状态。 |
| KC 规则列表 / 新增 / 撤销 | 复用 `GET/POST /admin/v1/grants` 与 `DELETE /admin/v1/grants/{grant}`，由 Server 授权。 | 操作仍经共享 typed executor；没有客户端求值或前端授权账。 |

## 选定的最小 UI 范围

KC Access Control 明确标识 KC 授权范围，原 lakeFS Administration 入口与页面保留。
自查展示当前已核验 principal 和每条显式规则的完整约束；可选外部申请链接只是导航。
管理表展示规则 ID、principal、actions、全服务、Repository 或 Dataset，以及 Dataset、ref、
object、aspect 和共享来源。空的可选约束必须解释为不限制该维度，不能表现为“没有权限”。

新增表单要求 principal、actions，以及显式全服务 / Repository / Dataset 范围；
可复用其余现有约束。常用动作提示不组成新角色，不封闭已选定的语义动作语言。
原请求字段为 `repository`，规则响应字段为 `repo`，不静默丢失这一映射。
撤销前显示目标规则的身份与范围，按 ID 请求并重新读取当前状态。
既有 API 没有 edit/upsert/批量原子替换，UI 不承诺原地编辑或撤销再发放的原子性。

Repository 与可选 Dataset 的显式加载范围通过 HTTP query 传给 Server；
principal / action 的列筛选是已返回规则的本地视图。两者分别展示，
服务端决定可见范围，不能用隐藏行替代授权。
401 回到唯一原登录；403 显示当前身份权限不足，不触发第二登录；其他失败保留具体错误。
只持有 manage 权而没有 read 权的主体仍可提交明确范围的新增规则；列表 403
不阻断保存。保存成功后若重新列举失败，应分别呈现新增成功与列表不可读。
Cookie 请求沿用控制台标记、JSON 写请求与同源保护，不放开 CORS。
不展示 `AllowFile` 中 provisioning / migration / admission 内务回执为授权规则。

## 本轮发现的可复用缺口

下列修复由 Go 管理合同实现承接，具体完成与验证状态以 [TASK](../../../TASK.md) 和本轮检查记录为准。
本文件不以单元夹具结果声称真实 Access Control 或原生企业 RBAC 已验收。

| 位置 | 信号 | 判定 / 处置 | 触发条件 |
|---|---|---|---|
| `NextRuleID` / `DropByID` | 原 `len(rules)+1` 在撤销中间规则后复用尚存在的 ID；相同 ID 撤销会删除多条规则。 | 新规则采用随机 opaque ID，不改已有 ID；旧重复 ID 的撤销 fail closed。增加新增→撤销→新增与歧义撤销回归。 | 日常授权维护发生删除后新增，或加载旧重复 ID。 |
| `adminGrantList` / `VerbGrantList` | 原范围授权后返回整个规则文件，泄露其他 Repository / Dataset 规则与内务回执。 | HTTP 库存返回 `GrantList{rules}`；显式范围过滤和管理动作按同一入口检查，不返回私有回执。query 单值白名单覆盖已有约束，action 在此是库存筛选，不是权限求值。 | 使用范围受限的 `admin.grants.read`。 |
| `adminGrantRemove` / StageHome 授权 | 原 DELETE 仅传 ID，授权退回客户端默认范围；实际目标规则的范围未参与检查。 | 按已保存目标规则推导授权坐标，未知/不可管理 ID fail closed；CLI 与 HTTP 保持同一受控入口。 | 范围管理员撤销规则、跨 全服务、Repository 或 Dataset 请求。 |
| `AdmissionGrant` / `admissionOverview` | 原自查摘要省略 Dataset/ref/object/aspect，受限授权看起来像全范围授权。 | 保留公开规则已经持有的约束，不从摘要推断更宽权限；补受限规则序列化与当前主体隔离回归。 | 当前主体具有任意约束规则。 |

## 剩余差距与 review 登记

这些项不代表新授权方案已选定，也不能用 native lakeFS 管理 API 填成 KC 权限事实。

| ID / 位置 | 信号 | 判定 | 处置 | 触发条件 |
|---|---|---|---|---|
| A-01 · `AuthorizeGrantAdministration` 的受限管理委托 | 原 object 字符串匹配不是规则 pattern 包含证明，例如 `data/?` 对请求字符串 `data/*`。当前修复只承接 服务/Repository/Dataset 的明确管理范围，带 ref/object/aspect 限制的管理规则不参与放行。 | 当前服务不隐含任意 Repository 管理权；复杂约束管理能力仍未实现，不能重新退回字符串匹配。 | 保持复杂管理约束 fail closed；完整规则包含、委托再委托与组合条件留给 owner review，不加入前端推理。 | 需要受限管理员按 ref、object glob 或 aspect 发放/列举/撤销规则。 |
| A-02 · `VerbAllowed` / CLI grant list 查询 | 旧 CLI 问题只求值显式 `MatchAllow`，未合并仓已认证默认、库存默认发现与 Dataset 专属授权。HTTP 库存已与此求值分开。 | `allow` 命中不等于整条产品请求有效权限；失败也不证明没有默认消费权。 | 本 UI 不提供权限模拟；CLI 有 action 的旧求值保留显式规则含义并要求全局库存读取权，远程/本地复用同一 evaluator；没有 action 则列举范围库存。统一有效权限自查需复用真实动作入口并选定完整坐标与结果说明合同。 | 用户需要预判某个实际操作，或要求 CLI/HTTP 的完整有效权限解释。 |
| A-03 · `AdmissionResult` / 部署访问政策 | grants 只列显式规则，仓默认消费策略与库存默认发现不会成为伪造的规则；管理员联系列表也不说明每人的可管理范围。 | 摘要不是全部权限或审批承诺。 | UI 标为显式授权，并保留外部申请入口；若要有效权限/管理员范围说明，先定义可公开政策与资源上下文。 | 需要解释默认消费权、发现权或向特定范围管理员申请。 |
| A-04 · `AllowFile` / GrantRequest / 管理 API | 没有独立的 KC 用户目录、组/角色/命名策略、deny/条件语言、规则修改事务或列表分页合同。 | 与官方 Users/Groups/Policies 有产品差异，不能按同名 UI 偷建第二份身份/授权权威。 | 原能力继续直连 lakeFS；KC 只展示和管理已有显式规则。规则数量增长时先补服务端有界查询；复杂策略、修改原子性须先 review。 | 大规模授权维护、批量变更或外部身份组织治理需求。 |
| A-05 · `ValidateActions` / 公开动作面 | 语法校验不是可枚举动作注册表；semantic action、前缀通配与部署初始 `*` 的可接受位置有差异。 | 下拉列表不能伪装成全量权限定义，也不能把 native 角色等级当 action。 | 常用提示引用公开动作面，保留高级语法输入与后端拒绝；动作发现/封闭动作目录需独立合同。 | 希望部署动态展示全量动作、校验未知语义或组合权限模板。 |
| A-06 · `server.auth_lakefs` / `identity.Bind` | native id + 秒级 creation_date 无法区分同名同秒重建；邮箱等 native ID 可能不满足 KC 当前用户名合同。 | 单次登录可验证当前原会话，不等于完整不可重派身份生命周期已闭环。 | 依既有接线说明与 TASK 身份债处理账号回收；不隐式恢复旧授权，不 normalize 不兼容 ID；显式身份关联仍未实现。 | 账号删除/重建、原身份来源迁移或企业账号命名不兼容。 |
| A-07 · 原 Administration / 外部 IdP 与真实部署 | 原页保留、API 对照和 isolated 回归不证明企业 RBAC、SSO/SLO 或生产授权变更已实际通过。 | 实现范围与部署验收必须分别记录。 | 按真实后端 edition 和独立部署合同验证，使用当前用户自己的原身份；不把 mock 或 Community 的升级入口算成管理成功。 | 获得对应企业/IdP 测试配置或执行生产接线验收。 |

本轮选择复用 Server 的真实主体、现有 Rule 与管理入口；否决同步 native policy、
创建 KC 组/角色、前端计算权威、对象 ACL、借管理员凭据和内置审批队列。
