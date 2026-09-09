# 产品 CLI 重构（未落地）

落地前：公开 argv 仍以 [`surface.go`](surface.go) 为准，现行操作语义仍以 [`SURFACE.md`](SURFACE.md) 为准。本文是已对齐的产品 CLI 计划；重构完成后再决定是否并入 SURFACE、以及如何清理本文。

不复制 HTTP URL 到 argv。HTTP / typed client 仍按资源分层；CLI 不镜像每条集合 GET。不做「每次传 `--catalog`」兼容层。

设计依据：[`docs/COMPOSITION.md`](../docs/COMPOSITION.md)、[`docs/PERMISSIONS.md`](../docs/PERMISSIONS.md)、[`docs/TERMINOLOGY.md`](../docs/TERMINOLOGY.md)。协议动词、错误码、字段形状不在本文重贴。

## Goal

选定 Catalog 之后，日常命令不再写 catalog、不再有 repo 导航层。用户从命令名判断在做什么，从 `--repo` / `--pin` / `--object` 判断对哪份知识。

同一轮收口五处「看完还是不懂」的产品面：Client 入口与登录回执（第 1 节）、`admission show` 的权限视图（第 4 节）、`kc show` 里的源说明、`knowledge schema list` 的分页与 commit、子命令 help 的渐进式披露（第 7 节）。这些不改 argv 层级，但和上面一样属于「用户读得懂」的合同，一起落地一起验收。

## Non-Goals

- 不按岗位建命令树；`help consume|write|compose` 仍是旅程，不是身份。
- 不把供给、Catalog 登记、知识发布、发权焊成一次隐式初始化。
- 不把当前 Catalog 写进 login token，不做成 Workspace pin，Server 不猜默认 Catalog。
- 不把 `attach` 解释成已发布，也不随 attach / create 送出 `knowledge.read`。
- 本轮产品 CLI 不挂 `share *`、`connection *`、`--mine`、`create --repo --command-id`。
- 不为 CLI 变短删除 HTTP 集合路由。
- 不在 KC 里托管申请/审批队列；发权只有管理员命令一条路。
- 不把源说明信封扩成领域分类、owner、质量线、投影热度。
- 不取消 Client 内部保存入口，不取消凭证按 Server 隔离，也不把入口地址塞进 login token。

## 上下文

| 上下文 | 怎么来 | 之后 |
|---|---|---|
| 当前 Catalog | `kc catalog use <id>`（一间可见时可自动固定） | 组合命令不再带 catalog 操作数 |
| 能打开的源 | `kc create --name` 或 `kc create --url --credential-file` | `--repo`；还不是 Catalog 成员 |
| 这间房可访问该仓知识 | `kc attach --repo` | `kc show` 可见 |
| 这次任务的版本 | `kc workspace pin --out` | `--pin` |

`--catalog` 只留在 `grant add` 的规则范围（Catalog 级动作）。`--repo` 是操作数，不是命令前缀。不要 `kc repo …` 分组。

## 对象怎么变

```text
create          平台供给或连上自有仓
                → KC 能打开这份 Snapshot；不是 Catalog 成员

attach --repo   当前 Catalog 承认该源
                → 这间房可以访问该仓知识，kc show 可见
                → 不要求已发布内容，不发 knowledge.read

writer          往 --repo 发布知识

grant           谁能读/写（管理员发权；仓级用 --repo）
```

`create --url` 不是 attach。连接和登记是两步。

---

## 1. 进哪间 Catalog

| 现行 | 处置 | 新产品 | 操作语义 |
|---|---|---|---|
| `catalog list` | 留 | `kc catalog list` | 可见 Catalog，只 `{id}` |
| （无） | 加 | `kc catalog use <id>` | Client 按 Server 固定当前 Catalog |
| `catalog show [<id>]` | 换成独立命令 | `kc show` | 当前组合态：已 attach 的源 + 命名知识集。不是对象目录，不是 git 历史 |
| `catalog audit` | 留，无 catalog 操作数 | `kc catalog audit` | 登记表 git |
| `catalog archive` | 留，无 catalog 操作数 | `kc catalog archive` | 整间 Catalog 只读历史 |

`kc show` 源行 `{id, profile, title?, summary?, schemaCount?}`；知识集行 `{workspaceId, revision, repositories[]}`。未 attach 的仓不出现。

### 1.1 Client 入口与登录

Client 入口和用户登录是两层：

- **Client 入口：** 发现或固定域名配置。当前解析顺序是显式 `--server` → `KC_SERVER_URL` → 已保存的 `client.json`；后续可在不改变登录合同的前提下补部署发现。
- **登录：** 只验证并保存身份凭证。Server 不建立登录会话，每个请求仍重新携带并验证凭证。

登录过程可以使用入口做认证模式发现与 `whoami` 校验，也会在本机 `client.json` 记住默认入口；token / local 会话继续按完整 Server URL 隔离。这些是 Client 内部状态，不是登录回执。

| 现行输出 | 新产品输出 |
|---|---|
| token / Taihu 成功：`{status, server, principal, ...}` | `{status: "authenticated", principal, ...凭证事实}` |
| local 成功：`{status, server, principal, mode}` | `{status: "authenticated", principal, mode: "local"}` |
| logout：`{status: "logged out", server}` | `{status: "logged out"}` |
| 浏览器起步返回内部 `request_uri` 等字段 | 只返回用户要访问的授权 URL 与 `kc login --wait` 下一步 |

登录 stdout 不得出现 `server`；脚本若要确认当前请求入口，应查询 Client 配置/状态，而不是解析登录结果。`logout` 只清本机对应入口下的凭证，Server 不接收“用户下线”状态。

---

## 2. 发现面删除

| 现行 | 处置 | 已被谁覆盖 |
|---|---|---|
| `catalog repo list` | 删 | `kc show`.repositories |
| `catalog repo list --mine` | 删 | `create --name` 同名重试；回执已有管理地址 |
| `catalog repo show` | 不加 | `show.repositories` 一行 |
| `workspace list` | 删 | `kc show`.workspaces |
| `workspace show` | 产品 CLI 删 | 与 `workspaces[]` 同形；HTTP GET 可留 |
| `knowledge search --catalog` | 不进主路径 | 搜用 `--pin` 或 `--repo` |

---

## 3. 源：能打开 → 被这间 Catalog 访问

| 现行 | 处置 | 新产品 | 操作语义 |
|---|---|---|---|
| `catalog repo create --name` | 合并为 create，**不登记** | `kc create --name [--store]` | 平台供给空仓、存连接、按策略给创建者写/回读。**show 没有** |
| `catalog repo create --repo --command-id` | **产品 CLI 不挂** | typed HTTP / 测试 | 调用方自填仓身份与幂等键；人用 `--name`，Server 生成坐标 |
| `catalog repo connect` | 并进 create，**不登记** | `kc create --url --credential-file` | 验证自有仓、存私有连接、KC 能打开。仓可为空。**show 没有** |
| `catalog repo attach` | 只保留 `--repo` | `kc attach --repo` | 这间 Catalog 可以访问该仓知识，且 show 可见。连接必须已在。不建仓、不改 HEAD、不要求已发布、不发读权 |
| `catalog repo archive` | 待钉名 | `kc detach --repo`（建议） | 从本 Catalog 拿下；不删 Snapshot |

`--name` 与 `--url` 互斥。当前 Catalog 约束谁能 create、身份怎么生成；成员名单只在 attach 时写。同一源可再 attach 进另一间 Catalog。

现行 `attach` / `connect` 要求 published HEAD / 非空 Snapshot，与「可访问 ≠ 已发布」不符，落地时要松开。

`--command-id` 仍用在 `writer commit|put|remove`（这一笔发布的幂等键），不出现在产品 `create`。

---

## 4. 授权

不要 `kc repo grant` / `kc repo share`。授权是发动作，仓是范围。

创建者在 `create` 时已按部署策略拿到写/回读，不必再给自己授权。`attach` 不发读权。

| 现行 | 处置 | 新产品 | 操作语义 |
|---|---|---|---|
| `admin grant add` | 去掉 `admin` 前缀 | `kc grant add --principal <user> --action <actions> --repo <id>` | 给该仓发稳定动作（如 `knowledge.read`） |
| 同上，Catalog 范围 | 同上 | `kc grant add --principal <user> --action <actions> --catalog <id>` | 私有 Catalog 的库存发现等；`--repo` 与 `--catalog` 二选一 |
| `admin grant list` | 去掉 `admin` 前缀 | `kc grant list` | 当前规则 |
| `admin grant remove` | 去掉 `admin` 前缀 | `kc grant remove --id <rule-id>` | 撤一条；旧 pin 不能绕过撤权 |
| `catalog repo share *` | 本轮不挂 | — | 创建者自助给人读是另一条路；先走 `grant` |
| `catalog repo connection *` | 本轮不挂 | — | 自有仓换凭证，不是发现/挂载 |

动作名仍是 `admin.grants.manage` / `admin.grants.read`；命令路径不再叫 `admin`。谁能跑仍是持有该动作的主体。`admission show` 列出可向谁申请。

典型仓级消费权：

```text
kc grant add --repo <id> --principal <user> \
  --action knowledge.read,knowledge.search,knowledge.schema.read
```

### 4.1 查权与申请入口

`kc admission show` 只回答两件事：**我现在有什么权**、**该向谁申请**。KC 不办申请队列，申请是外部流程；外部审批通过后调的仍是上面那条 `grant add`。

| 现行 | 处置 | 新产品 | 操作语义 |
|---|---|---|---|
| `admission show` | 换返回体 | `kc admission show` | 列调用方自己的规则 + 申请入口。不是状态机，不列别人的权 |
| `admission request` | **删** | — | 自助发权取消；发权只有管理员命令 |

`GET /identity/v1/admission`（`POST` 去掉）返回：

```json
{
  "principal": "kaiqidong",
  "grants": [
    {
      "id": "rule-id",
      "actions": ["catalog.repositories.create"],
      "catalog": "kr://example/catalog",
      "repository": "kr://…",
      "sharedBy": "alice"
    }
  ],
  "request": {
    "url": "https://itsm.example/kc-access",
    "administrators": ["agent:operator"]
  }
}
```

- `grants`：管理员发的 + 别人 share 的，都只含调用方本人；空是 `[]`。`sharedBy` 只在分享来的规则上出现。
- `request.administrators`：当前持有 `admin.grants.manage` 的主体（含 bootstrap 写 `*` 的那个）。
- `request.url`：部署可选；没有 `admission` 块时省略该字段，查询照样可用。
- 删掉 `status` / `eligible` / `actions` / `currentActions` 这套状态机字段。

部署配置只剩一个键，旧的 `enabled` / `catalog` / `principals` / `actions` 失败关闭：

```yaml
admission:
  requestURL: https://itsm.example/kc-access
```

登录和 `show` 都不写 `allow.json`。

---

## 5. Workspace（多源时才出现）

**产品定义：** 多源时，声明「用哪些 repo、各跟哪条分支（+ 可选 mount）」。

- **范围：** 成员 repo 列表（须已 attach）。
- **版本（用户面）：** 每条 `--source <repo>[=<selector>]` 或命名配方里的 selector；默认跟该源 published 分支（如 `refs/heads/main`）。用户换版本 = **换分支或重新打开任务**，不是日常挑 commit hash。
- **实现面：** 打开一次多源任务时，各 selector 解析一次并冻在任务内（`V-01`）；JSON 里的 `{repo→commit}` 是传输/重放格式，不进 help 主叙事。
- **不是：** 当前 Catalog、托管工作区、默认 search scope、单源日常路径。

| 现行 | 处置 | 新产品 | 操作语义 |
|---|---|---|---|
| `workspace pin --source …` | 留，help 改口径 | `kc workspace pin --source <repo>[=<selector>] …` | 临时多源配方；各源跟声明分支；打开任务 |
| `workspace pin --workspace <id>` | 留 | 同上 | 采用命名配方（compose 已 `define` 的范围） |
| `workspace pin --out` | 留（进阶） | 同上 | 跨进程/重开放任务上下文；不是「选某一版 commit」教程 |
| `workspace check` | 留（进阶） | `kc workspace check` | 已打开任务是否仍 attach |
| `workspace overlay` | 留（compose/进阶） | `kc workspace overlay` | Client 个人 overlay，不写 Catalog |
| `workspace define` / `retire` | 留（compose） | `kc workspace define` / `retire` | 发布 / 停用**命名**多源配方（各成员 selector + 可选 mount） |
| `workspace use` | 不加 | — | 不与 Catalog 抢「当前上下文」 |

**何时用：** 单源 → 只 `knowledge … --repo`（默认 ref）。**多源**且要在同一任务里 search/read（或 `kcfs` mount）→ 才 `workspace pin`。命名 `define` 是 compose 侧可复用模板，不是消费前置条件。

---

## 6. 基本不改

| 组 | 命令 | 操作数 |
|---|---|---|
| 身份 | `login` `logout` `whoami`（`admission show` argv 不变，返回体见 4.1） | — |
| 知识 | `knowledge schema list\|describe` `search` `read` `resolve` `relations` `provenance` `log` `binding show` `access` `invoke`（`schema list` 返回体见 7.2） | `--repo` / `--pin` / `--object` |
| 写 | `pack` `writer commit\|put\|remove\|head\|receipt` | `--repo`；commit/put/remove 仍要 `--command-id` |
| 治理 | `governance proposal\|preview\|validation …` | 同现合同 |
| 运维 | `operations …` | 同现合同 |
| 部署 | `deployment init\|status\|system publish\|identity migrate` | 只读 `--config` |
| 进程 | `serve` `help` `kcfs` | — |

---

## 7. 读得懂的输出（argv 不变，形状改）

### 7.1 源说明信封

`kc show` 现在把平台自己的 `kr://kc/system` 显示成 `profile: missing`——接入方看不出这个仓是什么、能不能写。系统仓必须自描述。

- 发布 System 对象 `schema/core/source-profile/v1`：信封只有 `title` + `summary`（`title` 走 `[text, filter]`，两者 required，`additionalProperties: false`）。领域分类、owner、质量线、投影热度不进这个对象。
- 平台自带 `kr://kc/system` 的 `core/source-profile`，随 System 仓发布，不是 git README：

```yaml
# Platform-authored source profile for kr://kc/system.
# Not a git README. Providers fill core/source-profile only in their own repos.
title: KC 协议仓
summary: 发布 Meta Schema 与核心协议 Schema，含源说明信封。已认证可读，运行时不可写。接入方在自己的知识仓发布 Schema 副本并填写源说明。
```

- 接入方在自己的知识仓发布同名对象；没发就还是 `profile: missing`，这是正常态不是错误。
- `kc show` 的源行据此填 `title` / `summary`（第 1 节的行形状）。

### 7.2 `knowledge schema list` 的分页与 commit

两处不符合一般预期，一起改：

| 现行字段 | 处置 | 理由 |
|---|---|---|
| `coverage{enumerated,total,complete}` | 删 | 自造分页词汇，没见过这种写法 |
| `exhausted` | 删 | 与 `continuation` 冗余 |
| `continuation` | 留 | **有就是还有下一页，省略就是最后一页**，这是通用游标写法 |
| 每条 schema 里的 `repository` / `commit` | 删 | commit 是仓级基准，顶层已有一份；每条重复一遍没道理 |

顶层保持 `{repository, commit, schemas[], continuation?}`。`reader.SchemaDescription` 相应去掉 `Repository` / `Commit` 两个字段。`--limit` 语义不变（`0` = 默认页，`>200` = `USAGE_INVALID`）。`knowledge log` / `audit` 的 `continuation` + `exhausted` 同形对齐，别只改一处。

### 7.3 help 渐进式披露

现在点进子命令层就没说明了。改成三层，每层只给该层该知道的：

| 层 | `kc help` | 内容 |
|---|---|---|
| 根 | `kc help` | 只列分组 + 一句话，不铺命令 |
| 分组 | `kc help <group>` | 该组每条子命令一行摘要 |
| 叶子 | `kc help <group> <verb>` | 用法、操作数、必填 flag |

`surface.go` 解析分组路径，帮助文本单独放 `help.go` / `help_catalog.go`，红例在 `help_test.go`。旅程页 `help consume|write|compose` 保持是旅程不是身份（见 Non-Goals）。

---

## 8. 最短路径

消费：

```text
kc login
kc catalog list
kc catalog use <id>                 # 一间可见时可省
kc show
kc knowledge schema list --repo <id>
kc knowledge read --repo <id> --object …
# 仅多源（或 kcfs）：声明各 repo 跟哪条分支，再在同一任务里 search/read
kc workspace pin --source <a>[=<selector>] --source <b>[=<selector>] [--out task.json]
kc knowledge search --pin task.json --query …
```

写入再挂上：

```text
kc create --name <名称>             # 或  kc create --url … --credential-file
kc pack --repo <id> --dir … --out changeset.json
kc writer commit --command-id … --changeset …
kc attach --repo <id>
kc grant add --repo <id> --principal <user> --action knowledge.read
```

运营预置源（配置里已有 binding）：

```text
kc attach --repo <id>
```

---

## 9. 语义对照

| 说法 | 是 | 不是 |
|---|---|---|
| KC 能打开 | `create` 之后 Server 拿得到 Snapshot | Catalog 成员 |
| Catalog 可访问该仓知识 | `attach` | 已发布；有 `knowledge.read` |
| Catalog 里可见 | `kc show` 出现该源 | 对象 LIST |
| 已发布 | `writer` 在某 ref 上有知识 commit | attach |
| 可读正文 | 仓级 `knowledge.read`（`grant add --repo`） | attach、公开库存、Workspace 成员 |
| 单源跟哪条分支 | `--repo` + 默认/`--ref` | Workspace |
| 多源各跟哪条分支 | `workspace pin --source repo=selector …` | 用户日常挑 commit hash |
| 任务内版本一致 | 打开任务时各 selector 解析一次（`--pin` 重放） | 每条命令各自漂 HEAD |
| 读某一历史 commit | `knowledge … --commit`（考古/审计） | Workspace 默认故事 |
| 这一笔写/建的幂等键 | `writer … --command-id` | 产品 `create` 的人机参数 |

---

## 10. 现行路径 → 新产品（对照）

| 现行 argv | 新产品 argv |
|---|---|
| `kc catalog list` | `kc catalog list` |
| `kc catalog show` / `--catalog` / 位置 id | `kc catalog use` 之后 `kc show` |
| `kc catalog repo list` | （删，看 `kc show`） |
| `kc catalog repo list --mine` | （删） |
| `kc catalog repo create --name` | `kc create --name` |
| `kc catalog repo create --repo --command-id` | （产品 CLI 不挂） |
| `kc catalog repo connect --url …` | `kc create --url --credential-file` |
| `kc catalog repo attach --repo` | `kc attach --repo` |
| `kc catalog repo archive --repo` | `kc detach --repo`（待钉） |
| `kc catalog repo share *` | （本轮不挂） |
| `kc catalog repo connection *` | （本轮不挂） |
| `kc workspace list` / `workspace show` | （删，看 `kc show`） |
| `kc admin grant add\|list\|remove` | `kc grant add\|list\|remove` |
| `kc admission request` | （删，外部流程 + `kc grant add`） |
| `kc admission show` | 路径不变，返回体见 4.1 |
| `kc login` / `kc logout` | 路径不变，回执不再含 `server`，见 1.1 |
| `kc knowledge schema list` | 路径不变，返回体见 7.2 |
| 其余 `knowledge` / `writer` / `workspace pin` / `deployment` / `governance` / `operations` | 路径不变，组合类不再带 catalog 操作数 |

---

## 11. 否决

- `kc repo …` 分组（发现、授权、连接都不走这个前缀）
- 日常命令继续要 catalog 操作数；同时保留「每次传 catalog」与 `use` 两套说法
- `create` 顺带登记；`create --url` 当成 attach
- attach 要求 published HEAD / 非空仓才成功
- attach 或 create 隐含 `knowledge.read`
- 产品 `create` 要求调用方填 `--command-id`
- 把 Workspace 收进 Catalog 子命令，或增加 `workspace use`
- 把 Catalog 默认搜索写成主路径
- 把 Workspace 讲成「用户管理 commit」；日常叙事必须是 **分支 + 多源范围**
- 在 KC 内建申请队列 / 审批状态机；`admission show` 回 `DISABLED` 之类的状态字
- 登录或 `admission show` 顺手写 `allow.json`
- 把 Client 入口当成登录属性，在 login / logout 回执里返回 `server`
- 源说明信封承载领域分类、owner、质量线、投影热度
- 自造分页词汇（`coverage` / `exhausted`）；每条 schema 各带一份 `repository` / `commit`

---

## 12. 待钉（落地时选一个，不另开设计）

1. 从 Catalog 拿下：`kc detach --repo`（建议）还是 `kc archive --repo`。
2. `create --url` 与 `--name` 是否同一次落地。
3. 一间可见 Catalog 是否自动 `use`（建议：是）。
4. 分页要不要保留一个 `total`（建议：不要；总数在游标分页里不保证可得）。
5. System 仓的自描述除源说明信封外，是否还要发「什么是 Entity / Relation / Aspect」这层元知识（走查里提过 System 还不够 self-descriptive；建议本轮先只做信封，元知识另立一条）。

---

## 落地前仍有效的验收（现行面）

公开命令分母仍是当前 `surface.go`，直到重构改表。HTTP 分母仍是 `httpsurface.Patterns()`。场景树见 [`.data/scenes/README.md`](.data/scenes/README.md)。不通过删断言、skip、隐藏别名或夹具绕过产品入口换绿。

旧 argv 可留在退役拒绝测试与迁移对照里，不构成恢复兼容别名的理由。协议层 `RegisterRepository` 等公开 API 名称仍按各自包合同理解。
