# 权限模型：按仓隔离、组合不发权

日期：2026-09-02
范围：谁能对哪份知识执行哪类 `kc` 动作。具体命令、规则字段、默认值和认证配置以 `cli/command.go`、allow 实现与测试为准。

本文回答：为什么安全边界默认是 Repository，为什么 Workspace 组合不能扩大授权，以及知识仓中的外部授权快照为什么不能替代外部系统实时强制。

---

## 1. 主张

1. Repository 是知识的默认安全和治理边界。
2. Workspace 只组合成员，不授予成员读权。
3. 协议授权直接使用已有 `kc` 动作，不再发明平行动作枚举。
4. 外部业务系统的受保护操作仍由外部系统当场强制。
5. 外部授权快照可以作为知识，但不能反向成为知识仓 ACL。

默认粒度：

| 问题 | 决策 |
|---|---|
| 正文读边界 | 整个 Repository |
| 写约束 | 可进一步限制 ref 或 Address |
| 敏感度差异 | 真正构成安全边界时拆 Repository |
| Workspace | 每次读逐成员求值，不发权 |
| 外部业务授权 | 外部系统实时强制 |
| 外部授权快照 | `permissions` Aspect，属于 SOURCE 知识 |

---

## 2. 三套不能混合的权限

| 层 | 回答 | 权威 |
|---|---|---|
| Store 门禁 | 谁能碰 remote、目录、clone/push | Git 托管、文件系统、部署凭证 |
| Knowledge Catalog 授权 | 谁能对 Repository/Catalog 执行某个 `kc` 动作 | 部署侧 allow policy |
| 外部操作强制 | 谁能在业务系统执行 SELECT、发布、运行任务等动作 | 外部系统当场决策 |

```text
Agent ── kc knowledge read ──→ Knowledge Repository
  │
  └── protected action ──→ External System
```

Catalog 不在外部操作路径上。能浏览关于某资源的知识，不等于能使用该资源。

Bound State READ 先通过 Workspace/Repository 的 `read-workspace` 授权，才允许进入
`knowledge/serving.StateLookup`；lookup 请求继续携带已经建立的 `principal/onBehalfOf`，由墙外
runtime 对外部数据访问再次强制。KC 的仓读取授权不能替代源系统授权，runtime 拒绝时不得回退
到 Repository 中的 `null` 占位或旧缓存。

### 2.1 外部授权快照是知识

`permissions` Aspect 描述“某次观测时外部系统对谁开放了什么”。它与 facts、relations 等 SOURCE Aspect 同构：有 Address、commit、producedAt 和 provenance，也允许落后。

三条边界：

1. 可以进入 Canonical，供说明、候选过滤和审计。
2. 不要求与外部系统实时一致；必须暴露观测时间和来源。
3. 真正执行动作时仍问外部系统。副本说允许、外部系统说拒绝，最终必须拒绝。

谁能看见这份快照，只由知识仓授权决定；不能读取 Aspect 内容后再决定“你是否有权读取它”。检索面继续使用 Schema AccessHints，通常不把 GRANT 正文当全文文档。

---

## 3. 第一性原理

### 3.1 为什么 ACL 边界等于 Repository

Repository 不只是文件目录，而是一张完整的 Snapshot 图：clone、log、diff、backup、projection 和 retention 都围绕它工作。

如果同一版本图内的两部分具有不同读者集合或历史可见性，那么“能打开这张图”本身已经扩大授权。对象/file ACL 会迫使每次历史读取、索引、备份和迁移都做正确裁剪；Git clone 更无法隐藏部分历史。

按 Repository 隔离后，问题退化为：

```text
principal × action × repository → allow | deny
```

Workspace 有 N 个成员时逐仓求值，复杂度与治理边界数量相关，而不是与对象数量相关。

### 3.2 Pin 不冻结授权

ResolvedWorkspace 固定本次数据坐标，不赋予未来访问权。每次命令按当前规则重新求值；否则一次旧 pin 会变成永久 capability，无法撤权。

### 3.3 配方不发权

`repo-add` 表示本机能够打开 Store；WorkspaceDefinition 表示配方希望组合成员；allow policy 才表示 principal 当前能执行动作。三者不能合并。

主动分享的便携配方可能让接收者知道某个 Repository identity 存在，这不同于从中心 Catalog 枚举无权资源。内容仍由成员仓保护。

---

## 4. 拆仓谓词

只有以下四个维度一致时，知识才适合放进同一 Repository：

| 维度 | 强行同仓的后果 |
|---|---|
| 读 ACL | clone、历史或索引泄漏 |
| 所有权/写权威 | 独立断言被当成覆盖冲突 |
| published ref 节奏 | 不同发布周期绑在一起 |
| 历史可见性 | 无法只隐藏旧 revision |

不应仅因源系统、文件类型、微服务、消费者数量、某个 Agent 或外部系统单条 GRANT 而拆仓。

经验判断：

- 两仓经常必须在同一次变更中一起修改，治理边界可能划错，应考虑合并。
- 两仓只是对同一主题有独立断言，应保持独立并在读侧并存。
- 同一 ACL 下仅写范围不同，用 Address 级写约束，不新建仓。
- 同一对象存在真正的敏感信息差，优先在受限仓写另一条知识并引用公共对象，不做 Aspect 级读 ACL。

---

## 5. Git 能解决什么

Git 擅长整仓访问、commit/ref、expected-old CAS、candidate branch 和评审路由；不擅长请求时身份、跨仓 Workspace、部分历史隐藏和对象级读授权。

因此：

- Git remote ACL 是 Store 门禁，可以与 KC 授权同向使用；
- Git author 或可伪造的环境字段不能作为可信 principal；
- CODEOWNERS 是评审路由，不是读隔离；
- Agent 若持有成员仓 clone，对象级只读已经失去意义；
- Store 替换时，授权语义不能依赖 GitHub 特有概念。

---

## 6. 业界调研与取舍

| 系统 | 借鉴 | 取舍 |
|---|---|---|
| Microsoft Purview | Collection 作为 metadata security boundary，Catalog 负责发现 | Repository 对应安全边界，Workspace 对应组合面 |
| Unity Catalog | 隔离单元与表级数据特权分开 | 外部 SELECT GRANT 不进入 KC allow |
| Dataplex | attach 外部资产而不复制 | Workspace 引用成员，不搬运正文 |
| dbt Mesh | project 是所有权边界，跨项目引用 | 经常协同修改说明边界可能过细 |
| GitHub/GitLab | Repository ACL、branch protection | 不把 CODEOWNERS 当文件读 ACL |
| DataHub | Policy 可对 Domain/instance 做细过滤 | 灵活但查询时授权和继承成本更高 |
| Atlas/Ranger | 元数据与业务特权由不同系统负责 | `permissions` Aspect 与实时强制分开 |
| Solid | 数据保留在原权威 | 资源级 ACL 复杂度不适合作为默认模型 |

不采用父级授权自动继承。Scope 不是目录优先级；Workspace union 对每个成员独立求值。

---

## 7. 决策

### 7.1 身份与动作

principal 来自 Client 的显式本地身份或可信认证 facade 注入；所有业务请求都跨过 Server 认证/授权边界，不存在直接打开 Home 的 owner bypass。协议动作使用稳定 semantic action；组和角色属于 IdP，不在知识协议里再造对象树。

Catalog 改动和 Repository 写入沿各自权威历史记录；成功读通常不写 Canonical。request/trace 只作为审计指针，不变成身份真相。

### 7.2 Workspace 读取

消费读先判断 principal 是否能使用该 Workspace，再对本次 pin 中每个 Repository 求值读取权限。无权成员不能因为被写进配方而变得可见。

是否对无权成员静默裁剪取决于调用面和防旁路策略；但 coverage 必须诚实，不能把授权裁剪后的结果宣称为完整宇宙。

当前 facade 的具体策略：

- `SEARCH` 有 completeness/claims 信封，因此可跳过无权成员，但必须返回 `partial`，且不在 SearchView 中暴露被跳过成员；Discovery 只接受 Repository 级读权，object 级规则不能授权未知对象发现。
- `READ` / `RESOLVE` / `RELATIONS` / `LOG` / `GET_PROVENANCE` 等裸数组或裸值结果没有 coverage 信封；成员读权不完整时 fail closed，返回 `FORBIDDEN`，不能把拒绝伪装成空结果。`RELATIONS` 的返回对象身份不能由 endpoint 的 object 级授权推出，因此要求成员仓级读权。对象 RESOLVE 是 `kc knowledge resolve`，授权复用 `knowledge.read`，不经 Catalog pin。
- `catalog workspace resolve`、`describe-access` 会暴露完整 pin 或成员元数据，因此要求全部成员的 Repository 读权。
- Workspace File Gateway / kcfs 是显式文件组合面，按各自响应中的 mount/entry 信息报告可见成员；不得把其输出当完整知识 SEARCH。

### 7.3 认证与授权分开

认证回答“是谁”，allow policy 回答“能做什么”。每个业务请求都携带凭证；Server
不创建会话资源，Pin 也不绑定身份。Taihu 部署参数见 [`DEPLOY_AUTH.md`](DEPLOY_AUTH.md)；
传输头见 [`SERVICE_ARCHITECTURE.md`](SERVICE_ARCHITECTURE.md) §8.1。

#### 配对

Client 与 Server 只有两种合法配对，错配失败关闭：

| 配对 | Server | Client 只发送 |
|---|---|---|
| 测试 / 本机夹具 | `kc serve --auth local` | `X-Kc-As` |
| 产品 | `--auth taihu` 或 `--auth gitea` | `Authorization` |

`kc serve` 必须带 `--auth`。省略不得静默变成 local。进程内
`HTTPHandler(home)` 仍是测试接缝，语义等于 local，不是产品默认。

local 不是匿名：空 `X-Kc-As` 仍是 `UNAUTHENTICATED`。local 拒绝 `Authorization`
和客户端自报的 `X-Kc-On-Behalf-Of`（未验证委托等于冒充）。产品配对拒绝
`X-Kc-As` 和客户端自报的 `onBehalfOf`。两种凭证同时出现，产品侧为 `FORBIDDEN`。

`--as` / `KC_AS` / `kc login --mode local --as` 只是 local 配对的测试捷径，不能
写成生产登录。产品身份只来自已验证认证器。

#### 三种主体，两种产品登录

复用已有 `principal` 与可选 `onBehalfOf`，不新增第三种协议对象。KC 按
`principal × action × repository` 授权；`onBehalfOf` 只进证据，不参与授权交集。
Agent 不得把用户写成 principal。

Taihu 用户名在 IdP 内唯一且不可变，因此也是 KC 用户 principal 的稳定标识。
allow.json 发权给 `taihu:<username>`。工号（introspection `sub` / 网关 `staff_id`）
只作为 `subject` 相关，不进入授权键。用户或委托 token 缺少 username 时认证失败关闭，
不得回退到工号。

| 种类 | principal | onBehalfOf | 怎样证明 |
|---|---|---|---|
| 用户 | `taihu:<username>`（Gitea 为 `gitea:<id>`） | 空 | 用户本人登录 |
| Agent 代理用户 | `agent:<id>` | `taihu:<username>` | 用户同意后由 token 携带 actor+subject；禁止 `kc login --as <user>` |
| 服务账号 | `service:<id>` | 空 | 已授权的机器主体；产品侧 Taihu `client_credentials`，测试侧 `--as service:bootstrap` |

`KC_SERVICE_CLIENT_SECRET` 与 `KC_TAIHU_HMAC_SECRET` 是 KC 资源方凭证，不是调用方
身份，只从部署环境注入。需要委托的测试不得用 local 自报 header，应注入 fake authenticator。

新的本机 Home 尚无 allow rule，因此提供唯一一个宿主级引导动作：

```bash
kc local grant bootstrap --home .kc --principal user:local-admin
```

它只在 allow 为空时创建首个全局管理 rule；一旦已有任何 rule 就失败关闭，不能覆盖治理状态。它与 `kc local init` / `repository attach` 一样属于宿主 bootstrap，不是第二套业务 API。首个 principal 建立后，后续 `kc admin grant ...` 也必须作为 Client 经 Server 执行。

启动命令、Taihu claim 名和当前覆盖状态由 CLI/HTTP 代码、`DEPLOY_AUTH.md` 与
`TEST_CATALOG.md` 维护，不在本文复制。

### 7.4 调用可观测性

调用身份冻结为两个字段：`principal` 是实际执行主体，`onBehalfOf` 是可选的被代理用户。Agent 代理用户时不能把用户冒充成 principal；KC 按 principal 授权，并完整记录两者。

成功、失败和拒绝的消费访问追加到访问证据库，每个命中都绑定固定 Repository、commit、object/Address。反馈按 trace 关联，hitmap 从访问账派生；这些都是过程证据，不写回 Canonical。查询走 `audit.read`；store 不按审计员身份再裁剪「谁访问过」。完整契约见 [`OBSERVABILITY.md`](OBSERVABILITY.md)。

---

## 8. 明确不做

- 不做 GitHub 式文件 ACL/CODEOWNERS 解释器。
- 不按 path 授权；路径不是知识身份。
- 不把 `permissions` Aspect 当 `kc knowledge read` 闸门。
- 不按表 GRANT、单个 Agent 或单个 Workspace 拆仓。
- 不把外部授权快照当外部操作放行依据。
- 不让 repo-add 或 define-workspace 隐式发权。
- 不在协议里建角色/组继承树。
- 不以对象级读 ACL 作为主模型。
- 不给 Agent 成员仓 clone 后再声称对象级只读。
- 不用跨仓事务掩盖错误的治理边界。

---

## 9. 代码是具体协议说明

- 命令与动作集合：`cli/command.go`
- allow 规则、求值和认证：`cli/` 对应实现与测试
- Workspace 逐成员读取：`knowledge/reader/serving.go`、CLI consume tests
- Catalog 可见性：`catalog/`、CLI catalog tests
- `permissions` Aspect：普通 Writer/Reader/Schema 路径
- Hook/Gate/外部资源边界：`HOOKS.md`、`GATES.md`、`CONNECTORS.md`
- 访问身份、trace/feedback 与 hitmap：`OBSERVABILITY.md`、`observability/`

规则字段、文件布局、启动命令和当前覆盖状态不再复制到设计文档；它们应只在代码、帮助文本、包 README 和测试目录中维护。
