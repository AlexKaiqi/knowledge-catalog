# CLI 重构方案

审查结论的落地稿，不是已生效合同。改完之前路径权威仍是 `surface.go`。
批时在节下写 `批：`。

与 [`HTTP_REFACTOR.md`](HTTP_REFACTOR.md) 配套。本文只拥有 **CLI argv / help**；typed HTTP 的 path/method 由那一篇拥有。两面独立注册，对齐操作与 action。

本文是**完整目标面**：含不改名的命令。只列 delta 无法判断「到底要不要这层子命令」。

---

## Goal

人看见 `kc <名词> <动词> [哪一个]` 就能预期：在哪一类资源上、做什么、是不是已经进权威。Client 分组跟 Server 面对齐，但不把 URL 层级复制成命令层级。不发明第三种进程或角色。

## Non-Goals

- 不改协议动词、semantic action。HTTP path/method 的改动见 [`HTTP_REFACTOR.md`](HTTP_REFACTOR.md)，不在本篇偷偷改。
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
B. **权威路径换成目标 argv**；旧路径进 `cliAlias`（不进 `CLICommandsForTest` 分母）。  
C. `local repository attach` 停止自动 register；夹具补 `catalog repo register`。  
D. 删别名；scenes / 数仓 feature / 文档示例 / dsh-plugin / RoleHelp 切新名；`SURFACE.md` 按新面重写或删审查段。

## 接口

| 改 | 不改 |
|---|---|
| `cli/surface.go`、`help.go`、远程 dispatch 的 **CLI 路径字符串** | HTTP 权威 path（见 HTTP 篇）；`ResolveWorkspace`、`writer.Ingest` 函数名、`ChangeNotice` |
| parse 位置参数（catalog/workspace id） | semantic action 名（`writer.preview` 仍罩 pack 与 head） |
| attach 的 register 副作用 | 授权矩阵本身 |

## 风险

- `workspace pin` 与 `--pin` 同词，help 必须各写一句。
- `knowledge schema list` 会被理解成对象 LIST：help 第一句钉死「只列 schema/*」。
- attach 不再登记会破旧夹具，这是分层修正。
- `kc pack` 太短、易撞用户脚本：可接受；比挂错面更重要。
- Agent 六角色测试人格（`dsh-plugin` 的 provider/governor/consumer/…）**不是** `kc help` 主题，本方案不改。

---

## 系统性交付

改的是**公开 CLI 路径与帮助**，以及一条宿主副作用（attach 不再 register）。协议动词、semantic action 不动。HTTP 路由见 [`HTTP_REFACTOR.md`](HTTP_REFACTOR.md)；本篇阶段 B 只要求「打到同一操作」，不把 67 条路由冻死。下面按「冻结 → 影响面 → 用例 → 阶段验收 → 完成定义」一次看完。

### 0. 范围冻结

| 动 | 不动 |
|---|---|
| `cli/surface.go` 公开路径、别名、位置参数 | HTTP 权威表与条数（HTTP 篇；现行 67，目标 65） |
| `cli/help.go` 总表与 consume/write/compose | `catalog.ResolveWorkspace`、`writer.Ingest` 函数名、`index.ChangeNotice` |
| `cli/parse.go` / remote dispatch 的路径字符串 | semantic action（`writer.preview`、`workspace.resolve`、`knowledge.schema.read`…） |
| `.data/scenes/**` 的 `When I run kc …` 与 `catalog.yaml` capabilities | PERMISSIONS 接口表；授权求值 |
| 根 README、Walkthrough、MVP 最短旅程里的 **CLI 示例** | 设计文档新造命令穷尽清单；`docs/graph/` 主题所有权 |
| `catalog/README.md`、包 README 里点名的 `kc …` | Agent e2e 人格名 `governor`（`agent-scenarios.json`） |
| dsh-plugin Skill 里的 kc 命令串 | `kcfs` 动词；`kc serve` 的 `--auth` |
| attach 的自动 register | Writer CAS / pin 不落盘 / SEARCH 不教 sync |

进阶段 B 之前按下面的落地默认视为已选定；批方案时只打「不同意哪一条」。

落地默认（可批掉再改）：

| 缺口 | 默认 |
|---|---|
| `workspace pin` 像写入 | 保留 `pin`；help 第一句「打印 ResolvedWorkspace JSON，不写入 Catalog」；可选 `--out` |
| `knowledge access` 像 READ | 保留；help 第一句「调用墙外 runtime，不是 knowledge read」 |
| 双靶 XOR | 不拆命令树；help Knowledge 节画死两条车道，混用 flag 继续 USAGE_INVALID |
| 位置参数 | 仅 catalog id、workspace id；`--object` / `--repo` / `--id` 仍 flag |
| pack 出口 | 无 `--out` 时 ChangeSet 在 stdout（与 pin 相同）；有 `--out` 时 stdout 仅 files/diagnostics（现状 ingest） |
| 全局 | help 增加 Global：`--server` / `KC_SERVER_URL`；顶层动词闭集：`help serve login logout whoami pack` |

### 1. 影响面（按层）

**别名不进公开分母。** 阶段 B 不得把旧路径再写进 `cliSurface`：`len(CLICommandsForTest())` 与改名前相同（只换名字、不增条目）。旧 argv 走单独 `cliAlias`（或等价表）。`catalog.yaml` `capabilities` 与 coverage 报告只认权威路径。阶段 B 起权威路径就是目标 argv；scenes 的 `When I run` 可暂留旧串，靠别名跑绿，到 D 再改 feature 文本。

```text
公开 CLI          surface.go  help.go  parse.go  remote_*.go
                  verbs_write.go（ingest handler 名可留）
应用操作名        ingest / resolve / index-notify / resource-access 可暂留
HTTP              不改 path；Client 换 CLI 字符串仍打同一 route
场景 Oracle       .data/scenes/**/_build/*.feature
                  .data/scenes/**/_probes/*.feature
                  .data/scenes/catalog.yaml capabilities
                  cli/scene_catalog_test.go 路径→状态映射
产品走通（make test）
                  cli/kc_test.go Help / RoleHelp / expandLegacy（ingest→pack）
                  command_coverage* / command_test.go（分母长度）
                  consume_flow / write_flow / server_client_only
                  remote_test / remote_dispatch_internal_test
                  projection_recovery_test / observability* / home_*
数仓黑盒（make test-data-warehouse / test-all）
                  .data/data-warehouse/features/{consumer,provider,change,agent}.feature
                  .data/data-warehouse/{README,connector/README}.md
                  docker/{cli-smoke,bootstrap,cli.profile}.sh
脚本              scripts/{e2e-kcfs-linux,system-gitea,live-taihu-auth}.sh
文档（示例，非穷尽表）
                  README.md
                  docs/MVP_ACCEPTANCE.md
                  docs/WALKTHROUGH_v5.1.md
                  docs/TEST_CATALOG.md（格子里的 kc 句）
                  docs/DEPLOY_AUTH.md（whoami）
                  docs/CONNECTORS.md / SERVICE_ARCHITECTURE.md（若点名 CLI）
                  .data/scenes/README.md
                  catalog/README.md  knowledge/README.md  knowledge/reader/README.md
                  index/README.md
插件              dsh-plugin/skills/knowledge-catalog/SKILL.md
                  dsh-plugin/test（若断言命令串）
审查底表          cli/SURFACE.md（D 阶段按新面重写或删审查段）
```

`make check-docs`：只在改了 `docs/*.md` 的示例句之后跑；不要为 CLI 新加 `docs/graph/` 节点。设计正文里的协议动词（DEFINE_WORKSPACE、`workspace.resolve` action）保留。

#### argv 替换表（B 起权威路径；D 删别名并改文档）

权威路径条数不变。内部 handler / semantic action 不变。

| 现行权威路径 | 目标权威路径 | action（不变） |
|---|---|---|
| `identity whoami` | `whoami` | `identity.read` |
| `catalog repository list\|register\|archive` | `catalog repo *` | 同左 |
| `catalog workspace list\|show\|define\|retire` | `workspace *` | 同左 |
| `catalog workspace resolve` | `workspace pin` | `workspace.resolve` |
| `catalog workspace check` | `workspace check` | `workspace.resolve` |
| `writer ingest` | `pack` | `writer.preview` |
| `knowledge schema browse` | `knowledge schema list` | `knowledge.schema.read` |
| `knowledge binding resolve` | `knowledge binding show` | `knowledge.binding.resolve` |
| `resource access` | `knowledge access` | `resource.access` |
| `operations projection notify` | `operations projection notice` | `projection.manage` |
| `operations access describe` | `operations access-spec describe` | `knowledge.access.describe` |

help 主题：`consumer`→`consume`，`provider`→`write`，`governor`→删除（可选 `compose`，不含 projection/merge）。

测试里的短名 `kc ingest`（`expandLegacy`）在 B 起映射到 `pack`，D 可删短名或保留为别名（不进分母）。

### 2. 用例对照（改完后必须仍能走）

语义不变，argv 变。

| 用例 | 现行 | 目标 | 验收线索 |
|---|---|---|---|
| 宿主起空 Catalog | `local init` + bootstrap + serve | 同左 | catalog-initialized |
| 打开空知识仓 | `local repository attach` **兼登记** | attach **只 ⓪**；再 `catalog repo register` | repository-attached 的 construct 必须两条 When；`catalog show` 在 register 之后才含该仓 |
| 接入：草稿未发表 | `writer ingest --out` | `kc pack --out` | drafts-ingested；stdout 无 changeSet |
| 接入：发表并读回 | `writer commit` + `knowledge read --repo` | 同左（commit/read 不改） | domain-schema-published |
| 发现 | `catalog list` → `show` → `schema browse` | `list` → `show <id>` → `knowledge schema list` | catalog-read-granted / schema-read-granted |
| 钉版本 | `catalog workspace resolve` | `workspace pin` | pin JSON 含 pinId 与 `{仓→commit}` |
| 消费 SEARCH/READ | `knowledge search/read --workspace --pin` | 同左 | 双靶仍 USAGE 互斥 |
| 组合+发权 | `workspace define` + `admin grant` | `workspace define` + `admin grant`（define 只是去掉 catalog 前缀） | 无 projection 出现在 compose help |
| 动态观察 | `operations projection notify` | `operations projection notice` | observation-refreshed construct |
| Agent 最短说明书 | `help consumer\|provider\|governor` | `help consume\|write\|compose` | `help governor` 非零退出 |

P1–P6 / C1–C6（`MVP_ACCEPTANCE.md`）继续成立，只换命令字符串。C2 的机器条件从 `kc catalog workspace resolve` 改为 `kc workspace pin`。

### 3. 分阶段与验收

每阶段：**先红（旧针失败或新针未写）→ 改 → 绿**。阶段出口全绿才能合入下一阶段。禁止 skip。

#### 阶段 A — 说明书（无 argv 行为）

改：`help.go` 按目标树分组；list/show 文案；删 governor 主题（或先让 `help governor` 指向 compose 文案但 **推荐直接失败**）；consumer/provider 文案改 consume/write，可暂时接受旧 topic 作别名。

验收：

- [ ] `go test ./cli -run 'TestRoleHelp|TestHelp'`：总表节名含 Catalog / Workspace / Writer / Knowledge / Admin / Governance / Operations / Pack 或等价；**不含**「Writing and governance」把 writer 与 governance 混在一节；`resource access` 不出现在 Operations 节
- [ ] `kc help governor` 退出非 0，stderr/stdout 含 `consume, write, or compose`（若 A 仍留别名，则本条放到 D，A 必须在 help 正文写「governor 已废弃」）
- [ ] consume help **不含** `projection sync`、`--home`、OpenSearch、Dolt、Gitea、`local repository attach`
- [ ] 行为测试（scenes、write_flow）仍用旧 argv 且全绿

#### 阶段 B — 新路径 + 旧路径别名（HTTP 不变）

`surface.go` **只保留目标路径为权威键**；旧路径进单独别名表，不进 `CLICommandsForTest`。位置参数：`catalog show <id>`、`workspace show/pin/check <id>`；唯一可见 Catalog 仍可省略。

验收：

- [ ] `len(CLICommandsForTest())` **等于**阶段 A 的条数（只换名）；`TestCLICommandsIsTheCompleteStableSurface` 绿
- [ ] `catalog.yaml` capabilities 已改成目标路径；每个 `CLICommandsForTest()` 条目仍挂一个 state
- [ ] 新路径走通：`pack`、`workspace list/show/define/retire/pin/check`、`catalog repo *`、`whoami`、`knowledge schema list`、`knowledge binding show`、`knowledge access`、`operations projection notice`、`operations access-spec describe`
- [ ] 旧路径在 B 仍成功（**别名，不进分母**）：`writer ingest`、`catalog workspace *`、`identity whoami`、`schema browse`、`binding resolve`、`resource access`、`projection notify`、`catalog repository *`、help `consumer|provider`
- [ ] `remote_dispatch_internal_test`：新 CLI 路径的 HTTP method/path **与当时的 HTTP 权威表相同**（例如 `workspace pin` 打 `:resolve` 或过渡期的 `/resolve`，不得新造第三种）
- [ ] 不因本阶段 CLI 改动新增 HTTP 路由；条数以 HTTP 权威表为准（HA 仍 67，HB 后 65）
- [ ] `pack` 无 `--out`：stdout 含 ChangeSet；有 `--out`：stdout 无 `changeSet` 键（与现行 ingest `--out` 合同一致）
- [ ] `workspace pin` stdout 含 `pinId` 与 `repositories`；不写登记表（随后 `workspace show` revision 不变）
- [ ] `knowledge schema list` 与原 browse 同 JSON 形状（schemas/coverage/exhausted）
- [ ] `make test` 中 cli 包 + `TestProductScenes` 绿（scenes 在 B 可仍写旧命令，靠别名）

#### 阶段 C — 分层行为（attach ≠ register）

验收：

- [ ] 仅 `local repository attach` 之后：`catalog show` 的 `repositories[].id` **不含**该业务仓（仍可含 `kr://kc/system`）
- [ ] 再 `catalog repo register --repo <id>` 之后：`catalog show` 含该仓
- [ ] `.data/scenes/.../repository-attached/_build/construct.feature` 含上述两步 When
- [ ] 该节点 README 不再写「attach 同时登记」
- [ ] `write_flow` / `ops_test` 里「repo-add 即出现在默认 Catalog」的断言改为 register 之后

#### 阶段 D — 删别名、切分母、改文档示例

验收：

- [ ] `surface.go` **没有**旧路径键；别名表为空；`kc writer ingest` 等为 unknown command（非零）
- [ ] `catalog.yaml` capabilities 与 `CLICommandsForTest()` 仍一一对应且无旧路径；`scene_catalog_test.go` 映射同步
- [ ] 全部协议 scenes 的 `When I run \`kc …\`` 使用新路径；`make test` 的 `TestProductScenes` / metric scenes 绿
- [ ] `.data/data-warehouse/features/*.feature` 与 docker/smoke 脚本切新路径（`make test-data-warehouse-check` 至少语法绿；live 套件不挡 D 合入，但不得留旧 argv）
- [ ] `scripts/e2e-kcfs-linux.sh`、`system-gitea.sh`、`live-taihu-auth.sh` 若调用旧路径则已换
- [ ] `TestRoleHelp` 针：consume/write/compose；**禁止** needle `governor`、`writer ingest`、`catalog workspace resolve`、`schema browse`、`help provider`、`help consumer`
- [ ] 根 `README.md` 最短闭环表不再出现 governor/provider/consumer help 名，改为 consume/write/compose；旅程命令为 pack / workspace pin / schema list
- [ ] `docs/MVP_ACCEPTANCE.md` P/C 表与示例命令已换新 argv；机器条件可判定
- [ ] Walkthrough / TEST_CATALOG / DEPLOY_AUTH / scenes README / catalog README / knowledge README / reader README / CONNECTORS / SERVICE_ARCHITECTURE 中作为 **调用示例** 的旧 CLI 已替换（协议动词 DEFINE_WORKSPACE、action 名等保留）
- [ ] dsh-plugin Skill 命令串已换；`make test-plugin` 绿
- [ ] `make check-docs` 绿
- [ ] `cli/SURFACE.md` 按新面重写或删除审查段，避免第三份清单
- [ ] `cli/kc_test.go` Help 金样与 `help.go` 一致
- [ ] 仓库内 `rg 'writer ingest|catalog workspace |schema browse|identity whoami|resource access|projection notify|help governor'` 在产品路径上为零命中（允许 `cli/REFACTOR.md` 对照表、git 历史、Agent 人格 JSON 的 `"governor"`）

### 4. 全局完成定义（DoD）

同时成立才算 CLI 重构完成：

1. **合同**：公开路径只有目标 argv；help 树 = 命令树；顶层动词闭集已印在总表。
2. **分层**：attach 不登记；register 才进入 `catalog show`。
3. **协议未收窄**：HTTP 66、action 名、pin 不落盘、pack/ingest 语义（不发表）、consume 不教 sync。
4. **Oracle**：`make test`（含 scenes）无 skip；RoleHelp/Help 金样绿。
5. **文档**：根 README + MVP 最短旅程与 CLI 一致；设计篇没有新的命令穷尽表。
6. **插件**：Skill 用新命令；Agent 人格 JSON **仍允许** 叫 governor（那是测试角色，不是 help topic）。

### 5. 建议提交切分（合入顺序）

不要一个 PR 打完。每个 PR 对应一阶段，标题用阶段字母：

1. `cli: regroup help; drop governor topic`（A）
2. `cli: canonical new paths; old argv as aliases`（B）
3. `cli: attach does not register repositories`（C，含 scene construct）
4. `cli: remove old paths; retarget scenes and docs`（D）

C 可与 B 对调，但不能在 D 之后：否则 scenes 已切新路径又要改 attach 语义，会搅在一起。

HTTP 阶段（HA/HB/HD）与 CLI 的合入顺序见文末 **联合交付**。

---

## 联合交付（CLI + HTTP）

两面独立注册，但 Client 远程路径是同一条线。不要一个 PR 里只改 argv 却让 `client/` 打到已删除的 URL，也不要 HTTP HD 早于 CLI B。

```text
1. CLI A     help 分组（argv 旧）
2. HTTP HA   权威路由表（URL 旧，分母改读表）
3. CLI B  + HTTP HB   两边权威名同时切；旧 argv / 旧 URL 都只是别名
4. CLI C     attach ≠ register（无 HTTP）
5. CLI D  + HTTP HD   删两面别名；scenes / 文档 / Skill / curl 一次切完
```

3 必须同批或 HTTP HB 略早于 CLI B（`client/` 已能打新 URL，旧 URL 别名仍在）。5 必须同批。

联合 DoD = CLI 篇 §4 **且** HTTP 篇 §4：

- CLI 公开路径 = CLI 目标 argv；HTTP 公开路径 = 65 条目标路由
- `workspace pin` → `POST …/workspaces/{id}:resolve`（或 HB 完成前的 `/resolve` 别名）
- `pack` 无 HTTP；`objects:read` 无 `addresses:read`；提案无 writer 第二扇门
- `make test` 无 skip；`make check-docs` 在文档示例更新后绿

对齐表以 HTTP 篇「CLI ↔ HTTP」为准，不在 `docs/*.md` 再抄一全套。

---

## 目标面符合性（用上面的尺子打本方案）

只打**本文目标 argv**，不打现行 `surface.go`（现行不合格项已是重构动机）。

| 准则 | 目标面 | 说明 |
|---|---|---|
| U1 / N4 list-show | **过** | `catalog list` / `show <id>` 都作用在 Catalog；`workspace list` / `show <id>` 成对 |
| U1 一般范式 | **部分** | 整体 noun-verb 像 gh；但顶层 `pack`/`whoami` 像 git。靠 M1 闭集收住，help 必须列出闭集 |
| U2 可类推 | **部分** | Catalog 下 list/show/audit/archive 可类推；创建词仍是 define/register/add/create 四套（N6 故意） |
| N1 用户对象 | **过** | workspace 提升；repo 留在 catalog 下（成员关系，不是第三套顶层名词） |
| N2 子命令 | **过** | `pack` 不再套 writer；`whoami` 不再套 identity |
| N3 面与预处理 | **过** | pack 顶层 |
| N5 反训 | **过** | ingest→pack，notify→notice |
| N6 协议词 | **过** | define/register/attach/preview 保留 |
| N7 深度 | **过** | 日常 3；`catalog repo *` 为 4 |
| N8 无角色 | **过** | consume/write/compose |
| M1 语法闭集 | **钉 help** | 总表首行列：`help serve login logout whoami pack` 是仅有的顶层动词 |
| M2 一条原语 | **部分** | 写权威的原语是 `writer commit`；`put`/`remove` 是糖。help 的 Writer 节第一句要写这句，否则仍像三种写 |
| M3 副作用 | **接受 + 钉 help** | `pin` / `knowledge access` 不改名；help 第一句写明「打印不落盘 / 墙外调用」。`catalog show` 拼源说明保持现状 |
| M4 互斥句法 | **接受（Non-Goal）** | `--workspace` XOR `--repo` 仍两 flag；混用 `USAGE_INVALID`；help 画死两条车道 |
| M5 id 位置 | **部分选定** | 仅 catalog / workspace id 用位置参数；`--object` / `--repo` / grant `--id` 仍 flag |
| M6 全局 flag | **钉 help** | 总表 Global：`--server` / `KC_SERVER_URL` |
| M7 输出 | **钉合同** | 无 `--out`：文档走 stdout；有 `--out`：stdout 无 `changeSet` |
| M8 一词一义 | **部分** | pack 不叫 preview，好。仍留三本账：`catalog audit` / `knowledge log` / `operations audit`——help 钉「登记表 / 对象 / 访问」，不合并动词。`remove` 靠名词消歧（grant/hook/writer） |
| M9 无隐含副作用 | **过（阶段 C）** | attach 不再 register |
| M10 可脚本 | **过** | JSON、无 confirm；Agent 第一用户，破坏靠授权 fail closed |
| M11 分页 flag | **过（未展示）** | help 骨架要让 `--limit` `--continuation` 出现在 log / schema list / audit |
| M12 无实现词 | **过** | RoleHelp 不得出现 OpenSearch/Dolt/`--home` |
| M13 消费不教运维 | **过** | compose 不含 projection |
| M14 HTTP 缺口 | **明示保留** | rerank、四条 retrieval 查询继续无 CLI；writer proposals **不再**作为 HTTP 第二扇门（见 HTTP 篇） |
| M15 command-id | **钉 help** | `writer put/commit` 帮助行必须带 `--command-id` |
| M16 help=树 | **过** | 节名与第一名词对齐 |

### 符合性缺口的落地选定（与 §0 落地默认同一套）

| 缺口 | 选定 | 验收落点 |
|---|---|---|
| `workspace pin`（M3） | 保留 `pin` | help 第一句含「打印」「不写入 Catalog」；随后 `workspace show` revision 不变 |
| `knowledge access`（M3） | 保留 | help 第一句含「墙外」且不含 READ 同义词 |
| 双靶（M4） | 不拆树 | Knowledge 节两车道；混用 flag → `USAGE_INVALID` |
| 位置参数（M5） | 仅 catalog / workspace id | `knowledge read` 仍 `--object` |
| 产物出口（M7） | 无 `--out` 走 stdout | 与 pin 相同；有 `--out` 时 stdout 无 `changeSet` |
| M1/M6 | 总表 Global + 顶层闭集 | Help 金样含 `--server` 与 `help serve login logout whoami pack` |

批方案时优先打「未过」项是否接受上述选定，而不是再扫一遍命令表。
