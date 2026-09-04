# CLI 重构方案

审查结论的落地稿，不是已生效合同。改完之前路径权威仍是 `surface.go`。
批时在节下写 `批：`。

本文是**完整目标面**：含不改名的命令。只列 delta 无法判断「到底要不要这层子命令」。

---

## Goal

人看见 `kc <名词> <动词> [哪一个]` 就能预期：在哪一类资源上、做什么、是不是已经进权威。Client 分组跟 Server 面对齐，但不把 URL 层级复制成命令层级。不发明第三种进程或角色。

## Non-Goals

- 不改协议动词、HTTP path、semantic action。
- 不按岗位做 help（无 `governor`）。
- 不把 `knowledge *` 拆成 `--workspace` / `--repo` 两套命令树。
- 不把打包 ChangeSet 做成 Server 写面。
- 不在本方案改授权求值。
- 不把命令穷尽清单写进 `docs/*.md`。

## 方法论（先尺子，再命令）

来源：你给的「看见就能预期」；本仓 Server/Client 切分；再加上通用 CLI（clig / kubectl / gh）里 KC 用得上的条。每条都要能用来否决一条命令。

### 你给的

| ID | 准则 | 检验 |
|---|---|---|
| U1 | 符合一般 CLI 范式，用户从命名和结构能预期干什么 | `list`/`show`/`get`/`describe` 是否作用在同一类资源；`show` 有没有「哪一个」 |
| U2 | 结构可类推 | 学会一条就能猜相邻的（同一名词下动词集合稳定） |

### 本仓已用、这里写死

| ID | 准则 | 检验 |
|---|---|---|
| N1 | 名词是用户对象，不是 URL 层级 | `workspace` 不必写成 `catalog workspace` |
| N2 | 子命令组 = 同一名词 ≥2 个动词；单动作不套空壳 | 禁止 `changeset create` 这种只有一个动词的壳 |
| N3 | Server 面名词下只放该面请求；Client 预处理不准挂进去 | `pack` 不准叫 `writer *` |
| N4 | 同 U1 的 list/show | 见上 |
| N5 | 动词不反训 | ingest/notify 若含义相反必须换词 |
| N6 | 协议已占用的词跟协议，不改成 REST 套话 | `define` / `register` / `attach` / Governance `preview` |
| N7 | 日常消费/写入 `kc` 后 ≤3 token；组合运维允许 4 | `knowledge read`、`workspace pin` 过；`catalog repo register` 过 |
| N8 | help 按面分组；最短路径按旅程；不发明角色主体 | 无 `governor`；`consume`/`write`/`compose` 是旅程不是 principal |

### 还该写上、方案里原先没当尺子的

| ID | 准则 | 为什么要 |
|---|---|---|
| M1 | **语法只有一套，例外是闭集。** 默认 `kc <名词> <动词> [id]`。顶层动词仅限：`help` `serve` `login` `logout` `whoami` `pack` | 否则有的像 git、有的像 kubectl，无法类推 |
| M2 | **一件事一条原语，糖必须能指回原语。** | `put` 是「单条 changeset + commit」；不能看起来像第三种写面 |
| M3 | **副作用类型能从命令上看出来。** 读 / 写入权威 / 只算不落盘 / 本机文件 / 打墙外，五类不要共用一个像「读」的词 | `pin` 像保存；`knowledge access` 像读，其实调外部 |
| M4 | **互斥模式要成为句法，不要靠两个 flag 偷偷 XOR。** | `--workspace` 与 `--repo` 现在只靠文档 |
| M5 | **同一类身份参数位置一致。** 资源 id 要么都是位置参数，要么都是 `--x`；不要 Catalog 用位置、object 用 flag、grant 用 `--id` 还各搞一套 | 可预期性的操作数版 |
| M6 | **全局正交项是全局 flag，不是子命令。** `--server`、身份、trace、`--request-id` | 已大体如此，help 骨架里没写 |
| M7 | **输出合同单一。** 机器默认 JSON；「给下一命令的文档」要么一律 stdout，要么一律 `--out`；错误也走同一信封 + 非零 exit | `pin` 走 stdout，`pack --out` 走文件，不一致 |
| M8 | **一词一义。** 同一英文词不能表示三本账或三种删除 | `audit` / `remove` / `preview` / `pin` |
| M9 | **无隐含副作用。** 一条命令只做名字里那件事 | `attach` 不得顺手 register |
| M10 | **默认可脚本。** 无确认提示、无 TTY 依赖；Agent 是第一用户 | 与「破坏性要 confirm」相反，本仓选脚本，破坏靠授权 fail closed |
| M11 | **分页和查询 flag 同名。** `--limit` `--continuation`；范围 `--catalog` `--workspace` `--repo` `--pin` `--object` | 已有，help 未钉 |
| M12 | **实现细节不进产品 argv/help。** 无 OpenSearch/Dolt/`--home` 出现在 consume/write 路径 | 已有测试；保持 |
| M13 | **错误不把运维命令教给消费路径。** SEARCH 失败不教 `projection sync` | 产品不变量，CLI 文案必须守 |
| M14 | **HTTP 有、CLI 无（或相反）必须是显式选择**，不是漏做 | rerank、writer proposals、四条证据查询 |
| M15 | **幂等键可见。** 写入要看得到 `--command-id` | 帮助骨架里没出现 |
| M16 | **help 树 = 命令树。** 总表节名与 argv 第一名词一致 | 现行「Writing and governance」不合格；目标面按 Catalog/Workspace/Writer… 排 |

## 选定 / 否决（结构）

| 选定 | 否决 |
|---|---|
| 打包 ChangeSet：顶层 `kc pack` | `kc writer ingest` / `kc writer pack`（假装是 Writer API） |
| Workspace 提升为 `kc workspace *` | 继续 `kc catalog workspace *`（4 token 且把知识集藏进 Catalog 子命令） |
| 仓作为 Catalog 成员：仍 `kc catalog repo *` | 再做一个顶层 `kc repository`（和 `--repo`、local attach 三套名词） |
| `kc whoami` | `kc identity whoami`（identity 下只剩一个读） |
| `kc knowledge access` 调墙外 | 顶层 `resource access`；或放在 operations |
| `kc help consume` / `write`；可选 `compose` | `governor` |
| `catalog show <id>` 仍返回现有库存文档 | 删 show；或本次顺手改 HTTP 变瘦 |
| attach 不再自动 register | ⓪ 和 ① 焊在一条宿主命令里 |

---

## 目标命令全表

`动作`：keep / rename / lift / flatten / behavior。  
`子命令`：这层嵌套是否成立。

### 0. 进程

| 现行 | 目标 | 动作 | 子命令 | HTTP | 语义 |
|---|---|---|---|---|---|
| `kc help [topic]` | `kc help [consume\|write\|compose]` | rename topic | 否 | 无 | 说明书。无 compose 则只前两个 |
| `kc serve` | 同左 | keep | 否 | 本进程 | 打开 Home，追 published HEAD 投影 |
| `kcfs plan` / `mount` | 同左 | keep | 是（kcfs 自己的两个动词） | `/workspace-files/v1/*` | 把已有 pin 挂成只读目录 |

### 1. 宿主 `kc local`（子命令组成立：全是本机、无 HTTP）

| 现行 | 目标 | 动作 | 子命令 | HTTP | 语义 |
|---|---|---|---|---|---|
| `local init` | 同左 | keep | 是 | 无 | 建 Home + 第一间 Catalog 登记表 |
| `local status` | 同左 | keep | 是 | 无 | 本机装配（仓/引擎/路径） |
| `local catalog attach` | 同左 | keep | 是 | 无 | 本机再打开一间登记表 |
| `local repository attach` | 同左 | **behavior**：只挂 Snapshot，不 register | 是 | 无 | ⓪ 权威。承认源走 `catalog repo register` |
| `local store show` | 同左 | keep | 是 | 无 | adapter / DSN（无密钥） |
| `local store set` | 同左 | keep | 是 | 无 | 改本机 store |
| `local grant bootstrap` | 同左 | keep | 是 | 无 | 空 allow 的一次性管理员 |
| `local system publish` | 同左 | keep | 是 | 无 | 把 `kr://kc/system` 写到可 clone 权威 |
| `local workspace overlay` | 同左 | keep | 是 | 无 | 本机按 principal 盖配方，不发布 |

`local` 这层子命令要：它标记「不是 Client」。下面每条才是具体宿主对象。

### 2. 身份

| 现行 | 目标 | 动作 | 子命令 | HTTP | 语义 |
|---|---|---|---|---|---|
| `login` | 同左 | keep | 否 | `GET /identity/v1/auth` + 本机会话 | 发现模式并配对 |
| `logout` | 同左 | keep | 否 | 无 | 清本机配对 |
| `identity whoami` | **`whoami`** | flatten | 否 | `GET /identity/v1/whoami` | 当前 principal |

login/logout/whoami 都是单动作，放顶层（和 `gh auth` 不同：我们没有多种 identity 动词要成组）。

### 3. Catalog 资源 `kc catalog`

子命令组成立：一间 Catalog 上有 list/show/audit/archive。

| 现行 | 目标 | 动作 | 子命令 | HTTP | 语义 |
|---|---|---|---|---|---|
| `catalog list` | 同左 | keep | 是 | `GET /catalogs` | 列可见 Catalog（id） |
| `catalog show` | **`catalog show <catalog>`** | rename 操作数 | 是 | `GET /catalogs/{id}` | **这一间**的文档：成员源 + 知识集。唯一可见时可省略 id |
| `catalog audit` | `catalog audit [<catalog>]` | keep（可加位置 id） | 是 | `GET …/audit` | 登记表 git |
| `catalog archive` | `catalog archive <catalog>` | keep | 是 | `POST …/archive` | 整间只读历史 |

`list`/`show` 现在成对：都作用在 Catalog 上。show 不再像「另一种 list」。

### 4. Catalog 成员仓 `kc catalog repo`

子命令组成立：同一成员关系上有 list/register/archive。日常少用，允许 4 token。CLI 用 `repo`（flag 已是 `--repo`）；正式文档仍写 Repository。

| 现行 | 目标 | 动作 | 子命令 | HTTP | 语义 |
|---|---|---|---|---|---|
| `catalog repository list` | **`catalog repo list`** | 缩写 | 是 | `GET …/repositories` | 承认哪些源（含源说明） |
| `catalog repository register` | **`catalog repo register`** | 缩写 | 是 | `POST …/repositories` | 承认该仓可进配方。不挂存储 |
| `catalog repository archive` | **`catalog repo archive`** | 缩写 | 是 | `POST …/repositories/{id}/archive` | 成员生命周期结束 |

### 5. 知识集 `kc workspace`（从 catalog 下提升）

子命令组成立：list/show/define/retire/pin/check ≥2。  
提升理由：知识集是消费主对象；`kc catalog workspace pin` 违反 N7，且像 Catalog 的内部实现细节。

| 现行 | 目标 | 动作 | 子命令 | HTTP | 语义 |
|---|---|---|---|---|---|
| `catalog workspace list` | **`workspace list`** | lift | 是 | `GET …/workspaces` | 命名知识集名单 |
| `catalog workspace show` | **`workspace show <id>`** | lift | 是 | `GET …/workspaces/{id}` | 这一条配方（现有公开形状） |
| `catalog workspace define` | **`workspace define`** | lift | 是 | `POST …/workspaces` | 发布/改 revision。动词跟 DEFINE，不用 create |
| `catalog workspace retire` | **`workspace retire`** | lift | 是 | `POST …/retire` | 这条不能再被消费 |
| `catalog workspace resolve` | **`workspace pin`** | lift+rename | 是 | `POST …/resolve` 或 `…/workspaces/resolve` | 解成 `{仓→commit}`，不落盘。临时组合：`workspace pin --source` |
| `catalog workspace check` | **`workspace check`** | lift | 是 | `POST …/check` | 该 pin 是否仍站得住（可先解再查） |

`--catalog` 在多 Catalog 时仍要。`pin` 命令 vs `--pin` flag：命令是「做一次 pin」，flag 是「带上已有 pin」。

### 6. 打包（Client，不是 Writer）

| 现行 | 目标 | 动作 | 子命令 | HTTP | 语义 |
|---|---|---|---|---|---|
| `writer ingest` | **`kc pack`** | flatten+rename | **否** | 无写面；最多 `GET …/head` 当 base | 目录 → ChangeSet 文件。`--out` 时 stdout 只有 files/diagnostics |

不要 `kc writer pack`：Writer 是提交权威。  
不要 `kc changeset create`：只有一个动作，再套名词+create 是空壳。  
`pack` 对 `commit`，类似暂存对提交，不假装已经摄入。

### 7. Writer 面 `kc writer`（子命令组成立：都打 `/writer/v1`）

| 现行 | 目标 | 动作 | 子命令 | HTTP | 语义 |
|---|---|---|---|---|---|
| `writer put` | 同左 | keep | 是 | `POST …/commits` | 单条 PUT 并提交 |
| `writer remove` | 同左 | keep | 是 | `POST …/commits` | 单条 REMOVE 并提交 |
| `writer commit` | 同左 | keep | 是 | `POST …/commits` | 提交已有 ChangeSet |
| `writer head` | 同左 | keep | 是 | `GET …/head` | 该仓 ref 的当前 commit |
| `writer receipt` | 同左 | keep | 是 | `GET …/receipts/{id}` | 按 command-id 查回执 |
| （无 CLI） | 仍无 | keep | — | `POST …/proposals` | HTTP 有；产品 CLI 继续走 governance |

put/remove 是 commit 糖，保留（单对象不必先 pack）。

### 8. Knowledge 面 `kc knowledge`

子命令组成立：同一知识对象上有多种问法。双靶 `--workspace`（+ `--pin`）或 `--repo`（+ `--ref`/`--commit`）保留，help 画两条车道。

| 现行 | 目标 | 动作 | 子命令 | HTTP | 语义 |
|---|---|---|---|---|---|
| `knowledge schema browse` | **`knowledge schema list`** | rename | 是（schema 上有 list/describe） | `POST /schemas:page` | **Schema 目录分页**，不是对象 LIST |
| `knowledge schema describe` | 同左 | keep | 是 | `POST /schemas:get` | 字段 text/filter/sort |
| `knowledge search` | 同左 | keep | 是 | `POST /search` | 定位候选 |
| `knowledge read` | 同左 | keep | 是 | `POST /objects:read`（地址另有 `/addresses:read`） | Canonical 正文 |
| `knowledge resolve` | 同左 | keep | 是 | `POST /objects:resolve` | 此 basis 上对象在不在 |
| `knowledge relations` | 同左 | keep | 是 | `POST /relations:query` | 关系边 |
| `knowledge provenance` | 同左 | keep | 是 | `POST /provenance:get` | 来源信封 |
| `knowledge log` | 同左 | keep | 是 | `POST /log:get` | 对象修订史 |
| `knowledge binding resolve` | **`knowledge binding show`** | rename | 是 | `POST /bindings:resolve` | 取出 Binding 声明，不调 live |
| `resource access` | **`knowledge access`** | lift+rename | 是 | `POST /resources:access` | 真去打墙外（`--aspect` 或 `--operation`） |
| （无 CLI） | 仍无 | keep | — | `/search:rerank` `/rerank` | HTTP 独有 |

`binding` 虽常只 show，仍留动词：和 `workspace pin`、`knowledge resolve` 三个「解析」必须能从路径上分开。  
`knowledge access` 不再叫 resource：调用方已经在 knowledge 面；`--aspect` / `--operation` 区分两种句柄。

### 9. Admin `kc admin grant`

子命令组成立：add/list/remove。保留 `admin`：对上 `/admin/v1`，并和 `local grant bootstrap` 分开。

| 现行 | 目标 | 动作 | 子命令 | HTTP | 语义 |
|---|---|---|---|---|---|
| `admin grant add` | 同左 | keep | 是 | `POST /grants` | 发稳定动作，不是 CLI 名 |
| `admin grant list` | 同左 | keep | 是 | `GET /grants` | 当前规则 |
| `admin grant remove` | 同左 | keep | 是 | `POST /grants/{id}/remove` | 撤规则 |

### 10. Governance `kc governance`

子命令组成立：proposal / preview / validation 是三类对象。前缀要：和 Writer commit、和 `kc pack` 的「预览」区分。Preview 是协议对象名（N6），不改。

| 现行 | 目标 | 动作 | 子命令 | HTTP | 语义 |
|---|---|---|---|---|---|
| `governance proposal create` | 同左 | keep | 是 | `POST /governance/v1/proposals` | 登记候选 |
| `governance proposal merge` | 同左 | keep | 是 | `POST …/proposals:merge` | 快进 published ref |
| `governance preview create` | 同左 | keep | 是 | `POST /previews` | 把 proposal overlay 到 Workspace pin |
| `governance preview validate` | 同左 | keep | 是 | `POST /previews:validate` | 结构检查 |
| `governance validation record` | 同左 | keep | 是 | `POST /validations` | 绑定外来 PASSED/FAILED |

### 11. Operations `kc operations`

子命令组成立：投影 / hook / gate / 证据 不是一类资源，用 `operations` 做平面前缀（对 `/operations/v1`）。下面再按名词拆。

| 现行 | 目标 | 动作 | 子命令 | HTTP | 语义 |
|---|---|---|---|---|---|
| `operations projection describe` | 同左 | keep | 是 | `POST /projections:describe` | 该仓该 commit 投影是否就绪 |
| `operations projection sync` | 同左 | keep | 是 | `POST /projections:sync` | 强制 Snapshot（及可选 State）重建 |
| `operations projection notify` | **`operations projection notice`** | rename | 是 | `POST /projections:notify` | 入站 ChangeNotice；控制器按 Binding 拉 |
| `operations access describe` | **`operations access-spec describe`** | rename | 是 | `POST /access-specs:describe` | 每成员 AccessSpec。避免和 `knowledge access` 撞 |
| `operations hook add\|list\|remove` | 同左 | keep | 是 | `/hooks` | 动词前后出站。不是 Gate |
| `operations gate add\|list\|remove` | 同左 | keep | 是 | `/gates` | merge 证据项。不是 Hook |
| `operations audit access` | 同左 | keep | 是 | `/access-log:query` | 访问账 |
| `operations audit trace` | 同左 | keep | 是 | `/traces:get` | 一条 trace |
| `operations audit hitmap` | 同左 | keep | 是 | `/hitmap:query` | 派生命中统计 |
| `operations feedback record` | 同左 | keep | 是 | `POST /feedback` | 按 trace 记反馈 |
| （无 CLI） | 仍无 | keep | — | retrieval/refine 四条查询 | HTTP 独有 |

`projection` 三动词成组，保留子命令。`notice` 替换 notify（N5）。

---

## 目标 argv 一览（改完后 help 按此排）

```text
Host
  kc local init|status
  kc local catalog attach
  kc local repository attach
  kc local store show|set
  kc local grant bootstrap
  kc local system publish
  kc local workspace overlay
  kc serve

Identity
  kc login / logout / whoami

Catalog
  kc catalog list
  kc catalog show <catalog>
  kc catalog audit [<catalog>]
  kc catalog archive <catalog>
  kc catalog repo list|register|archive

Workspace
  kc workspace list
  kc workspace show <id>
  kc workspace define
  kc workspace retire
  kc workspace pin [<id>|--source]
  kc workspace check

Pack（Client）
  kc pack --repo --dir --out <changeset.json>

Writer
  kc writer put|remove|commit|head|receipt

Knowledge（--workspace + --pin，或 --repo）
  kc knowledge schema list|describe
  kc knowledge search|read|resolve|relations|provenance|log
  kc knowledge binding show
  kc knowledge access

Admin
  kc admin grant add|list|remove

Governance
  kc governance proposal create|merge
  kc governance preview create|validate
  kc governance validation record

Operations
  kc operations projection describe|sync|notice
  kc operations access-spec describe
  kc operations hook add|list|remove
  kc operations gate add|list|remove
  kc operations audit access|trace|hitmap
  kc operations feedback record
```

最短路径：

```text
kc help consume   catalog 读 + workspace pin + knowledge + kcfs
kc help write     pack → writer commit/put → knowledge read --repo
kc help compose   catalog repo register + workspace define + admin grant
                  （不含 projection / governance / catalog audit）
```

删除 `kc help governor`。

---

## 为什么 `writer ingest` 不要子命令

它同时犯了 N3 和 N5：挂在 Writer 下，名字还像已经写入。

可选方案与否决：

| 方案 | 结果 |
|---|---|
| `kc writer ingest` | 现状。否决 |
| `kc writer pack` | 仍像 Writer API。否决 |
| `kc changeset create --dir` | 单动作空壳。否决 |
| `kc writer commit --dir` | 打包和发布并成一步，破坏「确认再提交」。否决 |
| **`kc pack`** | 选定。顶层、单动作、与 commit 并列 |

日常写：`kc pack … --out cs.json` → 看 diagnostics → `kc writer commit --changeset cs.json`。单对象继续 `kc writer put`。

---

## 分阶段

A. 只改 `help.go` 分组与 list/show 文案；删 governor 主题（可先把第三条写成 compose 但仍跑旧 argv）。  
B. 新路径 + 旧路径别名：`pack`、`workspace *`、`catalog repo *`、`whoami`、`knowledge schema list`、`binding show`、`knowledge access`、`projection notice`、`access-spec describe`、`catalog show <id>`。  
C. `local repository attach` 停止自动 register；夹具补 `catalog repo register`。  
D. 删别名；scenes / dsh-plugin / RoleHelp 测试切新名；`SURFACE.md` 按新面重写或删审查段。

## 接口

| 改 | 不改 |
|---|---|
| `cli/surface.go`、`help.go`、远程 dispatch 路径字符串 | HTTP path、`ResolveWorkspace`、`writer.Ingest`（函数可仍叫 Ingest）、`ChangeNotice` |
| parse 位置参数（catalog/workspace id） | semantic action 名（`writer.preview` 仍罩 pack 与 head） |
| attach 的 register 副作用 | 授权矩阵本身 |

## 风险

- `workspace pin` 与 `--pin` 同词，help 必须各写一句。
- `knowledge schema list` 会被理解成对象 LIST：help 第一句钉死「只列 schema/*」。
- attach 不再登记会破旧夹具，这是分层修正。
- `kc pack` 太短、易撞用户脚本：可接受；比挂错面更重要。
