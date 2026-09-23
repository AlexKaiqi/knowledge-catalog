# 产品 CLI 重构（已落地）

应然设计见 [`docs/CLI.md`](../docs/CLI.md)。公开 argv 以 [`surface.go`](surface.go) 为准，现行操作语义以 [`SURFACE.md`](SURFACE.md) 为准。本文保留重构目标、否决项和迁移对照，不是文档图 owner。

不复制 HTTP URL 到 argv。HTTP / typed client 仍按资源分层；CLI 不镜像每条集合 GET。不做旧 argv 兼容层。

## 落地标准（2026-09-09）

1. **用例先行**：先改验收（`command_test.go`、场景树、边界红例），钉死通过标准，再改实现，最后整体验收。禁止先改实现再补测试。
2. **删干净**：退役的 argv、handler、help 段落、场景步骤整段删除，不留别名、不留「暂时还能用」、不留兼容分支。
3. **不妥协**：无历史包袱；旧 argv 只出现在 `TestRemovedCommandsAreRejected` 与迁移对照注释里，不构成恢复理由。

验收锚点：`TestProductCLIRefactorDefinesTheExactPublicSurface`（58 条）+ `TestRemovedCommandsAreRejected`。落地顺序见 §14。

设计依据：[`docs/COMPOSITION.md`](../docs/COMPOSITION.md)、[`docs/PERMISSIONS.md`](../docs/PERMISSIONS.md)、[`docs/reviewed/terminology.md`](../docs/reviewed/terminology.md)。协议动词、错误码、字段形状不在本文重贴。

## Goal

产品 CLI 只服务四条真人路径：**进一间房、读一份知识、往一个仓写、把源挂上并给人权。** 其余能走 HTTP / 进阶分组的，根 help 和最短旅程都不出现。

命令名说「做什么」，`--repo` / `--dataset` / `--object` 说「哪一份」。Client 已经知道的 Server、身份、当前 Catalog，不再当日常 flag。

同一轮收口读得懂的输出：登录回执、`admission show`、`kc show` 身份库存、`schema list` 分页、help 三层。一起落地一起验收。

## Non-Goals

- 不按岗位建命令树；`help consume|write|compose` 仍是旅程，不是身份。
- 不把供给、Catalog 登记、知识发布、发权焊成一次隐式初始化。
- 不把当前 Catalog 写进 login token，不做成 Workspace pin，Server 不猜默认 Catalog。
- 不把 `attach` 解释成已发布，也不随 attach / create 送出 `knowledge.read`。
- 不为 CLI 变短删除 HTTP 集合路由。
- 不在 KC 里托管申请/审批队列；发权只有 `grant add`。
- 不把 README 扩成领域分类、owner、质量线、投影热度，也不展成 Catalog title/summary。
- 不取消 Client 内部保存入口，不取消凭证按 Server 隔离，也不把入口地址塞进 login token。
- 知识面 argv 去掉 `knowledge` 前缀（`search` / `read` / `schema list` …）；内部操作名、HTTP `/knowledge/v1` 与授权词 `knowledge.*` 仍留在知识面。
- 单源消费不要求先造冻结文件；多源用命名 Dataset。产品 argv 不出现 `kc pin` / `--pin`。

---

## 用户脑子里只有这些

| 我想… | 命令 | 操作数 |
|---|---|---|
| 证明我是谁 | `login` / `whoami` | 无（入口已在 Client） |
| 我能干什么、找谁批 | `admission show` | 无 |
| 进哪间房 | `catalog list` → `catalog use` | 一间可见时自动 `use` |
| 这间房有什么源 | `kc show` | 无 |
| 读 / 搜 | `schema list\|search\|read` | `--repo` 或 `--dataset`；对象用 `--object`；`schema list` 只认 `--repo` |
| 新开一个仓并发布 | `create` → `writer commit\|put` | `--repo`；写仍要 `--command-id` |
| 让这间房能用这个仓 | `attach --repo` | 不发读权 |
| 让别人能读 | `grant add --repo … --action knowledge.read,…` | 仓是范围，不是 `kc repo` |
| 两个仓一起搜 | `dataset define` 再 `search --dataset` | 单源禁止把 Dataset 当日常 |

没有「浏览全部对象」「每次带 catalog」「按岗位进子壳」这些场景，就不做那些命令。

---

## 操作数范式

和 `git` / `gh` 一样：**上下文进配置，操作数只表示这次要对准的东西。**

| 层 | 谁记住 | 日常命令还写吗 |
|---|---|---|
| Server 入口 | `client.json` / `KC_SERVER_URL` / 偶发 `--server` | 否（不是 `login` 专属参数） |
| 身份 | `login` 会话 | 否 |
| 当前 Catalog | `catalog use` | **否**（唯一例外：`grant add --catalog` 表示规则范围，与 `--repo` 二选一） |
| 哪个仓 | `--repo` | 是 |
| 已发布 Dataset | `--dataset` | 多源消费 / `kcfs` |
| 哪个对象 | `--object` | 实例动词是 |
| 哪一笔写 | `--command-id` | 仅 `writer commit\|put\|remove` |
| 部署文件 | `--config` | 仅 `deployment *` 与 `serve` |

知识 / 写 **不接受** `--catalog`、`--source`、`--pin`。Writer、`diff` 与 `schema list` 拒绝 `--dataset`。`--dataset` 与 `--repo` 混用继续非法。

单仓跟分支：默认 published ref。考古才加 `--commit`（或 `--ref`），不进消费教程。

```text
create          平台供给或连上自有仓 → KC 能打开；不是 Catalog 成员
attach --repo   当前 Catalog 承认该源 → show 可见；不要求已发布；不发 knowledge.read
writer          往 --repo 发布知识
grant           谁能读/写（仓级用 --repo）
```

`create --url` 不是 attach。连接和登记是两步。

---

## 1. 进哪间 Catalog

| 现行 | 处置 | 新产品 | 档 | 操作语义 |
|---|---|---|---|---|
| `catalog list` | 留 | `kc catalog list` | 主 | 可见 Catalog，只 `{id}` |
| （无） | 加 | `kc catalog use <id>` | 主 | Client 按 Server 固定当前 Catalog；一间可见则自动 |
| `catalog show [<id>]` | 换成独立命令 | `kc show` | 主 | 已 attach 的源 + 命名知识集。不是对象目录，不是 git 历史 |
| `catalog audit` | 留，无 catalog 操作数 | `kc catalog audit` | 进阶 | 登记表 git；根 help 不出现 |
| `catalog archive` | 留，无 catalog 操作数 | `kc catalog archive` | 进阶 | 整间 Catalog 只读历史；根 help 不出现 |

`kc show` 源行 `{id, schemaCount?}`。知识集行 `{setId, revision, repositories[]}`。未 attach 的仓不出现。README 走 `kc read --aspect readme` / SEARCH。

### 1.1 Client 入口与登录

入口和登录是两层：入口靠发现或固定域名（`--server` → `KC_SERVER_URL` → `client.json`）；登录只验证并保存身份。Server 不建会话，每个请求仍重新带凭证。

登录会用入口做认证模式发现与 `whoami` 校验，并在本机记住默认入口；会话按完整 Server URL 隔离。这些是 Client 内部状态，不是登录回执，也不是 `login` 的操作数。日常不写 `--server`；只有 `--mode local` 才需要 `--as`。

| 现行输出 | 新产品输出 |
|---|---|
| token / Taihu 成功带 `server` | `{status: "authenticated", principal, …凭证事实}` |
| local 成功带 `server` | `{status: "authenticated", principal, mode: "local"}` |
| logout 带 `server` | `{status: "logged out"}` |
| 浏览器起步返回内部 `request_uri` | 只返回授权 URL 与 `kc login --wait` |

```text
kc login [--mode taihu|token|local]     # 日常不写 --server；local 才 --as
kc logout
kc whoami
kc admission show
```

---

## 2. 发现面删除

| 现行 | 处置 | 已被谁覆盖 |
|---|---|---|
| `catalog repo list` / `--mine` / `catalog repo show` | 删 | `kc show`.repositories；`--mine` 用 `create --name` 同名重试 |
| `workspace list` / `workspace show` | 删 | `kc show`.workspaces；HTTP GET 可留 |
| `search --catalog` | **删** | Catalog 不是搜索范围 |
| 知识/写命令上的 `--catalog` / `--source` / `--pin` | **拒绝** | `--repo` 或 `--dataset` |
| `workspace use` | 不加 | 不与 Catalog 抢当前上下文 |

---

## 3. 源：能打开 → 被这间 Catalog 访问

| 现行 | 处置 | 新产品 | 操作语义 |
|---|---|---|---|
| `catalog repo create --name` | 合并，**不登记** | `kc create --name [--store]` | 平台供给空仓。**show 没有** |
| `catalog repo create --repo --command-id` | **不挂** | typed HTTP / 测试 | 人用 `--name`，Server 生成坐标 |
| `catalog repo connect` | 并进 create，**不登记** | `kc create --url --credential-file` | 自有仓；与 `--name` 互斥。**show 没有** |
| `catalog repo attach` | 只留 `--repo` | `kc attach --repo` | 这间房可以访问该仓知识；连接必须已在 |
| `catalog repo archive` | 改名 | `kc detach --repo` | 从本 Catalog 拿下；不删 Snapshot |

`--name` 与 `--url` **同一次落地**、互斥。当前 Catalog 约束谁能 create；成员名单只在 attach 时写。同一源可再 attach 进另一间 Catalog。

现行 `attach` / `connect` 要求 published HEAD / 非空 Snapshot，落地时松开（可访问 ≠ 已发布）。

`--command-id` 只出现在 `writer commit|put|remove`。Client 不替人默默生成：重试是真实场景，静默生键会双发。

---

## 4. 授权

不要 `kc repo grant` / `kc repo share`。授权是发动作，仓是范围。

创建者在 `create` 时已按部署策略拿到写/回读。`attach` 不发读权。

| 现行 | 处置 | 新产品 |
|---|---|---|
| `admin grant add\|list\|remove` | 去掉 `admin` 前缀 | `kc grant add\|list\|remove` |
| Catalog 范围 | `--catalog` 仅此例外 | `grant add --principal --action --repo\|--catalog` 二选一 |
| `share *` / `connection *` | 本轮不挂 | 发权走 grant；换凭证走 HTTP |

动作名仍是 `admin.grants.manage` / `admin.grants.read`。谁能跑仍是持有该动作的主体。

```text
kc grant add --repo <id> --principal <user> \
  --action knowledge.read,knowledge.search,knowledge.schema.read
```

### 4.1 查权与申请入口

`kc admission show` 只回答：**我现在有什么权**、**该向谁申请**。不办申请队列；外部审批通过后调 `grant add`。删 `admission request`。

`GET /identity/v1/admission`（`POST` 去掉）：

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

- `grants`：只含调用方本人（管理员发的 + 别人 share 的）；空是 `[]`。`sharedBy` 只在分享来的规则上出现。
- `request.administrators`：当前持有 `admin.grants.manage` 的主体（含 bootstrap 的 `*`）。
- `request.url`：部署可选；没有 `admission` 块时省略，查询照样可用。
- 删掉 `status` / `eligible` / `actions` / `currentActions`。

```yaml
admission:
  requestURL: https://itsm.example/kc-access
```

旧的 `enabled` / `catalog` / `principals` / `actions` 失败关闭。登录和 `show` 都不写 `allow.json`。

---

## 5. Workspace（只有「两个仓同一任务」才出现）

**产品定义：** 多源时声明「用哪些 repo、各跟哪条分支（+ 可选 mount）」。

- 成员须已 attach。用户换版本 = **换分支或重新 `dataset define`**，不教 commit hash。
- 打开一次命令时各 selector 解析一次并冻住（`V-01`）；`{repo→commit}` 是回执字段，不进主叙事。
- **不是** 当前 Catalog、托管工作区、默认 search scope、单源日常路径。

| 命令 | 档 | 操作语义 |
|---|---|---|
| `dataset define` / `retire` | compose | 可复用配方 |
| `dataset overlay` | 进阶 | 个人 overlay，不写 Catalog；overlay 名不能拿去 `--dataset` 消费 |

单源消费 **禁止** 先造冻结文件。`kcfs plan|mount` 吃当前 Catalog + 登录会话 + `--dataset --root`；挂载时内部冻结，产品 argv 不出现 `--pin`。

### 5.1 Knowledge：主路径三条，其余进阶

知识命令仍是一组，help 主题还叫 `knowledge`；argv 去掉 `knowledge` 前缀。操作数：**拒绝** `--catalog` / `--source` / `--pin`。日常消费用 `--repo` 或 `--dataset`。

日常只教这三条：

```text
kc schema list --repo <id> [--limit] [--continuation]
kc search --repo <id> --query …
kc search --dataset <id> --query …
kc read --repo <id> --object …
kc read --dataset <id> --object …
```

`schema list` **只认 `--repo`**（Schema 是仓目录，不是任务目录）。不挂公开 LIST，不挂 `search --catalog`。

人已经拿到 `--object` 之后才需要进阶：

| 命令 | 唯一场景 |
|---|---|
| `resolve` | 只要在不在、不要正文（脚本） |
| `relations` | 从该对象走一跳边；多源 `--object kc://repo/id` |
| `provenance` | 看来源信封 |
| `log` | 这对象各 digest 对应哪些 commit |
| `schema describe` | 写 typed search 之前看字段语义 |
| `binding show` | 看 Binding 声明，不取数 |
| `access` | Binding + `--aspect` 墙外观察 |
| `invoke` | Descriptor + `--operation --input` |

`access` / `invoke` 互斥，不要合成一条「智能」命令。

---

## 6. 产品闭集（三档）

上档出现在 `kc help` 和最短旅程；中档 `kc help <组>` 有一行；下档 argv 可留作协议/运维，**根 help 当没看见。** 这比「路径照抄、操作数减一减」更窄：主路径大约 20 条动词。

### 6.1 主路径（必须有场景）

**身份** — 见 1.1、4.1。不挂 `admission request`。

**组合**

```text
kc catalog list
kc catalog use <id>
kc show
kc create --name [--store]              # 与 --url 互斥；KC 能打开；show 还没有
kc create --url --credential-file
kc attach --repo
kc detach --repo
kc grant add --principal --action --repo|--catalog
kc grant list
kc grant remove --id
```

**知识** — 见 5.1 日常三条。

**发布**

```text
kc writer commit --command-id --repo --dir
kc writer put --command-id --repo --object --value|--file
kc writer remove --command-id --repo --object
kc writer head --repo
kc writer receipt --command-id
```

目录稿走 `writer commit --dir`；改一个对象才 `put`/`remove`。HTTP Writer 仍收 ChangeSet；`--changeset` 是进阶操作数。

**多源** — 见第 5 节；用命名 Dataset，不要消费者 pin 文件。

**进程**

```text
kc serve --config deployment.yaml [--listen]
kcfs plan  --dataset <id> --root <dir>
kcfs mount --dataset <id> --root <dir>
kc help | kc help <组> | kc help <组> <动词>
kc help consume|write|compose
```

**部署（部署方，不是普通用户）**

```text
kc deployment init|status|system publish --config
kc deployment identity migrate --config --file
```

### 6.2 进阶（有场景，但不教新手）

知识进阶见 5.1。`catalog audit` / `catalog archive` 见第 1 节。

治理——**只有「合入必须过闸」的仓才用**，不是日常 commit。Preview 对准一次已发布 Dataset，用 `--dataset`，不要 `--catalog`。本轮不加 proposal list（create 的 stdout 就是下一跳的 id）。

```text
kc governance proposal create   --repo --proposal-id --candidate …
kc governance preview create    --proposal --dataset
kc governance preview validate  --preview
kc governance validation record  --preview --suite --outcome
kc governance proposal merge    --proposal --preview [--validation]
```

运维——排障 / 观察方 / 审计，不进消费旅程。hook/gate 的 Catalog 范围 = 当前 `use`，不要再加 `--catalog`。`audit` 与 `log` / `catalog audit` 三套并存，叶子 help 各一句。

```text
kc operations projection describe|sync --repo [--commit]
kc operations projection notice
kc operations access-spec describe --repo | --dataset
kc operations hook add|list|remove
kc operations gate add|list|remove    # merge gate 必须 --repo
kc operations audit access|trace|hitmap
kc operations feedback record
```

### 6.3 本轮产品 CLI 明确不挂

| 不挂 | 原因 |
|---|---|
| `admission request`、`share *` | 发权只走 `grant add`；申请在站外 |
| `connection *`、`--mine`、`create --repo --command-id` | 无普通人机场景；协议/测试走 HTTP |
| `catalog repo *`、`workspace list\|show\|use` | `kc show` 已覆盖；不抢 Catalog 上下文 |
| `search --catalog` | 第三种范围，和目标冲突 |
| 知识/写上的 `--source` / `--catalog` / `--pin` | 逼出「`--repo` 或 `--dataset`」一条路 |
| HTTP rerank / retrieval-log / training | 继续 HTTP-only |
| 源说明上的分类 / owner / 热度 | README 只有 markdown `body`；不进库存 |

旧 argv 只进退役拒绝测试，不做兼容别名。

---

## 7. 输出与 help

### 7.1 README

System 仓必须自描述。发布 `schema/core/readme/v1`：Aspect `readme`，字段 `body`（`access: [text]`），`additionalProperties: false`。

平台自带 `kr://kc/system` 的 README 知识单元：

```markdown
---
entity: kc/system
aspect: readme
schema_ref: schema/core/readme/v1
path_hint: README.md
---
# KC 协议仓

发布 Meta Schema 与核心协议 Schema。已认证可读，运行时不可写。
```

接入方没发就不存在这份知识对象，不是报错，也不是库存缺列。`kc show` 不从 README 派生 `title` / `summary`。

### 7.2 分页与 commit

| 现行字段 | 处置 |
|---|---|
| `coverage{enumerated,total,complete}` | 删 |
| `exhausted` | 删 |
| `continuation` | 留：有就是还有下一页，省略就是最后一页 |
| 每条 schema 里的 `repository` / `commit` | 删；顶层已有一份 |
| `total` | **不要**（游标分页不保证可得） |

顶层 `{repository, commit, schemas[], continuation?}`。每行点名实体（`entity`、可选 `description`、`objectId`）。合同走 READ，AccessHints 只走 `schema describe`。`--limit`：`0` = 默认页，`>200` = `USAGE_INVALID`。`log` 与 `catalog audit` / `operations audit` 游标同形，别只改一处。

### 7.3 help 三层

根分组就七块：**身份、组合、知识、发布、治理（进阶）、运维、部署。** `show` / `create` / `attach` / `grant` / `writer commit` / `serve` / `kcfs` 写在对应组下，不要再加 `kc repo`。

| 层 | 给什么 |
|---|---|
| `kc help` | 分组 + 一句话。**不铺命令、不铺 flag** |
| `kc help knowledge` | 该组每条一行：主路径在前，进阶在后 |
| `kc help read` | 用法、互斥操作数、必填 flag |

旅程页 `consume|write|compose` 只保留最短路径，**删掉**现行 `--dataset` 主叙事和 `search --catalog`。帮助文本在 `help.go` / `help_catalog.go`；红例在 `cli/kc_test.go`（`TestHelp` / `TestRoleHelp`）与 `cli/command_test.go`（分组 help）。

---

## 8. 最短旅程（根 help 只指向这里）

消费：

```text
kc login
kc catalog list
kc catalog use <id>          # 可省
kc show
kc schema list --repo <id>
kc read --repo <id> --object …
kc search --repo <id> --query …
# 仅当两个仓要同一任务：
kc dataset define agent --revision 1 --source a --source b
kc search --dataset agent --query …
```

写入：

```text
kc create --name <名称>                    # 或 --url … --credential-file
kc writer commit --command-id <id> --repo <id> --dir …
kc writer commit --command-id … --repo <id> --dir …
kc attach --repo <id>
kc grant add --repo <id> --principal <user> \
  --action knowledge.read,knowledge.search,knowledge.schema.read
```

运营预置源（binding 已在配置里）：只 `kc attach --repo`。

---

## 9. 语义对照

| 说法 | 是 | 不是 |
|---|---|---|
| KC 能打开 | `create` 之后 | 已经在这间房里 |
| 这间房能用这个仓 | `attach` | 已发布；已能读正文 |
| 看得见 | `kc show` | 对象列表 |
| 读得了正文 | `grant … knowledge.read` | attach |
| 搜/读这份知识 | `search` / `read` `--repo` 或 `--dataset` | `--catalog`；把 Dataset 当日常 SEARCH |
| 这一笔写 | `--command-id` | `create` 的人机参数 |
| 任务内版本一致 | 一条命令 resolve 一次；跨命令跟已发布 Dataset 或 `--repo --commit` | 消费者管理 pin 文件；每条命令各自漂 HEAD |

---

## 10. 现行路径 → 新产品

| 现行 argv | 新产品 argv |
|---|---|
| `kc catalog list` | 同 |
| `kc catalog show` / `--catalog` / 位置 id | `catalog use` 之后 `kc show` |
| `kc catalog repo list` / `--mine` | （删，看 `kc show`） |
| `kc catalog repo create --name` | `kc create --name` |
| `kc catalog repo create --repo --command-id` | （不挂） |
| `kc catalog repo connect --url …` | `kc create --url --credential-file` |
| `kc catalog repo attach --repo` | `kc attach --repo` |
| `kc catalog repo archive --repo` | `kc detach --repo` |
| `kc catalog repo share *` / `connection *` | （不挂） |
| `kc workspace list` / `show` | （删，看 `kc show`） |
| `kc admin grant *` | `kc grant *` |
| `kc admission request` | （删） |
| `kc login` / `logout` | 路径不变；回执无 `server`；help 不把 `--server` 写成 login 参数 |
| `kc search --catalog` | （删） |
| `kc search\|read * --catalog\|--source\|--pin` | （拒绝）→ `--repo` 或 `--dataset` |
| `kc schema list` | 只 `--repo`；返回体见 7.2 |
| `kc search\|read` | 主路径；`--repo` 或 `--dataset` |
| `kc resolve\|relations\|…` | 进阶；操作数同上 |
| `governance` / `operations` | 进阶；Preview 用 `--dataset`；hook/gate 吃当前 Catalog |

---

## 11. 否决

- `kc repo …` 分组
- 日常命令继续要 catalog 操作数；「每次传 catalog」与 `use` 两套并存
- `create` 顺带登记；`create --url` 当成 attach
- attach 要求 published HEAD / 非空仓；attach 或 create 隐含 `knowledge.read`
- 产品 `create` 要求 `--command-id`；Client 替 writer 默默生成 `--command-id`
- `workspace use`；消费者管理 pin 文件
- Catalog 当搜索范围；知识/写继续收 `--catalog` / `--source` / `--pin`
- `schema list --dataset`（Schema 不是任务目录）
- `--server` 写成 `login` 专属参数；登录回执带 `server`
- Workspace 讲成「用户管理 commit」或默认 search scope
- KC 内建申请队列；`admission show` 回 `DISABLED` 一类状态字；登录/`show` 写 `allow.json`
- 源说明信封承载分类 / owner / 热度；把 README 展成 Catalog title/summary
- 自造分页词（`coverage` / `exhausted` / `total`）；每条 schema 各带一份 commit
- 把 resolve / 治理 / 运维铺进根 help 或消费旅程
- 兼容别名恢复旧 argv

---

## 12. 已钉（本轮按这个落地，不另开设计）

1. 拿下源：**`kc detach --repo`**（`archive` 听起来像删历史）。
2. `create --name` 与 `--url` **同一次落地**，互斥。
3. 一间可见 Catalog：**自动 `use`**。
4. 分页：**不要 `total`**。
5. System 仓本轮发布 README 知识对象；「什么是 Entity」另立 `TASK.md` 的 SYSTEM-META。
6. 知识/写命令：**拒绝** `--dataset` / `--catalog` / `--source`。
7. `catalog audit` / `archive`、知识进阶、治理、运维：**进阶档**，根 help 当没看见。

---

## 13. 验收锚点（TDD 分母）

公开 argv 的精确闭集以 **`cli/command_test.go::TestProductCLIRefactorDefinesTheExactPublicSurface`** 为准：**61 条**路径（`serve` / `kcfs` 由场景树状态 + `TestSceneCatalogCoversPublicProductSurfaces` 另钉，不进 `cliSurface` map）。

退役 argv 以 **`TestRemovedCommandsAreRejected`** 为准：`admission request`、`admin grant *`、`catalog show`、`catalog repo *`（含 share/connection）、`workspace list|show` 等必须 `USAGE_INVALID`，不做兼容别名。

落地顺序：**先钉 surface 分母 → 扩展退役拒绝 → help/coverage 守卫 → 场景树 → 行为/E2E**。禁止删断言、skip 或夹具绕过产品入口换绿。

### 13.1 落地结果（2026-09-10）

| 项 | 状态 |
|---|---|
| `surface.go` 61 条新路径与旧 argv 拒绝 | 已落地 |
| `catalog use`、create/attach/detach 分离与远程路由 | 已落地 |
| 知识/写操作数、kcfs `--dataset`、治理 Preview `--dataset` | 已落地 |
| login/logout、admission、Schema/log/audit 分页输出 | 已落地 |
| 三层 help、最短旅程、场景树与 E2E | 已落地 |

---

## 14. 落地顺序（实现依赖）

```text
(1) Client 上下文 【阻塞后续】
    catalog use → sessions/<server-hash>/catalog.json（复用 serverSessionPath + writeJSONFile）
    catalog list 一间可见 → 自动 use
    remoteCatalogID / runRemoteCLI 注入顺序：显式 --catalog > 持久化 use > list 推断

(2) 公开面 handler 接线（surface 已绿后的语义）
    show / attach / create / detach / grant* 独立 remote 路由
    pickCatalog 读 Client 当前 Catalog，不再要 argv --catalog

(3) 协议语义补齐
    create 去 RegisterRepository（--name / --url 同轮落地）
    attach 松开 published HEAD 前置
    detach = Catalog 成员移除（新 catalog API + HTTP + client；≠ repo archive）

(4) 操作数收紧
    rejectPublicSurfaceFlags：knowledge/写 拒绝 --catalog|--source|--pin；Writer/`schema list` 拒绝 --dataset
    schema list 仅 --repo；删 search --catalog / catalog_discovery 产品路径
    kcfs 必填 --dataset --root；治理 preview 用 --dataset；hook/gate 默认当前 Catalog

(5) 输出与 help 收口（§7）
    login/logout、admission、schema 分页、help 三层

(6) 场景树与 E2E
    .data/scenes/、TestGroupedCatalogViews*、consume/write 旅程改新 argv
```

---

## 15. 协议/实现缺口（不能 rename 了事）

| 缺口 | 现行 | 目标 | 主要触点 |
|---|---|---|---|
| **detach** | `verbArchiveRepo` → Snapshot archive | 从 Catalog registry **拿下**成员，不 archive Snapshot | `catalog/lifecycle.go`（无 Unregister）、HTTP、client |
| **create** | `CreateManagedRepository` / `ConnectRepository` 末尾 `RegisterRepository` | create **不登记**；attach 才登记 | `home/managed.go`、`home/connections.go` |
| **attach** | `AttachRepository` 要求 published HEAD | 可访问 ≠ 已发布 | `home/deployment_runtime.go` |
| **catalog use** | 无 Client 持久化；`remoteCatalogID` 每次 list 推断 | 持久化 + 自动 use | `cli/remote_session.go`、`cli/remote_management.go` |
| **grant --catalog** | 可与 current catalog 混淆 | 仅规则范围；**不** silent 注入 current | `cli/verbs_allow.go` |
| **Catalog 上下文四套机制** | `_default-catalog`、`KC_CATALOG`、task pin、remote 推断 | 统一为 `catalog use` 优先级表 | `cli/remote_task_context.go`、`cli/run.go` |

可复用：`readCatalogState`（show）、`catalogListOperation`、`verbAllow`、create 主体（named/connect HTTP）、`pinCommit`/`replayPin`、Client 会话持久化模式。

---

## 16. 风险（落地时盯）

1. **detach 无现成 API** — 不能直接把 `catalog repo archive` rename 成 `detach`。
2. **create 后 show 不可见** — 现有 journey 假设 create 即登记；场景须按 REFACTOR 重写。
3. **双轨 catalog 上下文** — DSH/kcfs/task mount 与 `catalog use` 优先级须写清，避免串环境。
4. **HTTP 与 argv 不同步** — HTTP 集合路由保留；产品 argv 删发现面不等于删 HTTP GET。
5. **discoveryWorkspaceId** — 删 `search --catalog` 后清理 `catalog_discovery.go` 与 help 半退役叙事。
6. **自动 use 副作用** — `catalog list` 写 Client 状态 vs HTTP `GET /catalogs` 纯读；CLI 可接受，README 说明即可。

---

## 落地前仍有效的验收（现行面）

HTTP 分母仍是 `httpsurface.Patterns()`。场景树见 [`.data/scenes/README.md`](.data/scenes/README.md)。

旧 argv 只进退役拒绝测试与迁移对照，不构成恢复兼容别名的理由。协议层 `RegisterRepository` 等公开 API 名称仍按各自包合同理解。
