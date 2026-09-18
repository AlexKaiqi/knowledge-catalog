# 产品 CLI 操作语义

产品 argv、help 与操作数的应然设计见 [`docs/CLI.md`](../docs/CLI.md)。
公开路径闭集以 [`surface.go`](surface.go) 为权威；HTTP 闭集以
[`httpsurface`](../httpsurface/README.md) 为权威。本文解释产品操作，不复制协议字段。

## 上下文与操作数

- Server 入口来自 `--server`、`KC_SERVER_URL` 或 `client.json`；日常命令不重复填写。
- `catalog use` 按 Server 保存当前 Catalog；只有一间可见时 `catalog list` 自动选择。
- 单 Repository 操作用 `--repo`，已定义 Dataset 用 `--dataset`。回执带 `commit`；精确历史重放抄 `--repo --commit`。
- 知识命令拒绝 `--catalog`、`--source`、`--pin`。Writer、`diff` 与 `schema list` 拒绝 `--dataset`。
- 对象用 `--object`；Writer 提交必须由用户提供 `--command-id`。
- 默认 Snapshot ref 是 `snapshot.DefaultRef`；历史复核才显式给 `--commit` 或 `--ref`。

## 身份

| 操作 | 语义 |
|---|---|
| `login` | 用已知 Server 发现认证模式，验证身份并保存本机会话；回执不重复 Server 地址 |
| `logout` | 清除当前 Server 的本机会话 |
| `whoami` | 显示当前认证主体 |
| `admission show` | 显示本人当前 grants，以及外部申请 URL 和当前 grant 管理者（含 bootstrap `*`） |

KC 不托管申请/审批队列；没有 `admission request`。部署可选
`admission.requestURL` 仅提供外部入口。

## 组合

| 操作 | 语义 |
|---|---|
| `catalog list` | 列出可见 Catalog，只返回身份 |
| `catalog use <id>` | 选择当前 Catalog |
| `show` | 显示当前 Catalog 已 attach 的 Repository 与命名知识集；不是对象目录 |
| `create --name [--store]` | 供给一个 KC 可打开的托管 Repository；不 attach |
| `create --url --credential-file` | 连接批准来源上的自有 Repository；不 attach |
| `attach --repo` | 将已连接 Repository 登记进当前 Catalog；不要求已发布，不发读权 |
| `detach --repo` | 从当前 Catalog 移除成员；不归档或删除 Snapshot |
| `grant add\|list\|remove` | 管理授权；`add` 的 `--repo` 与 `--catalog` 二选一，Dataset 消费权是 `--catalog` 加 `--dataset`。`list` 打印 `{rules}`，可按仓或 Catalog 过滤；不是整份 allow 文件 |

`catalog audit` / `catalog archive` 是进阶 Catalog 操作。HTTP 仍可保留资源分层的集合读取；
产品 argv 不镜像每条集合 GET。

## 知识集

知识集只用于同一任务需要多个 Repository：

```text
kc dataset define <id> --revision <n> --source <repo>[=<selector>] …
kc dataset retire <id>
kc dataset overlay --file <recipe.yaml> --overlay <overlay.yaml>
kc search --dataset <id> --query …
kc read   --dataset <id> --object …
kcfs plan  --dataset <id> --root <project>
kcfs mount --dataset <id> --root <project>
```

`dataset define` / `retire` 管理可复用配方，位置参数 `<id>` 与 `--dataset` 相同。`overlay` 是本机 `--file` 底稿加 `--overlay` 补丁，不发表；用 overlay 名去 `search` / `read --dataset` 是 `KNOWLEDGE_SET_INVALID`。单 Repository 消费直接 `--repo`。`kcfs plan|mount` 在挂载时内部冻结本次 ResolvedKnowledgeSet，产品 argv 不出现 pin 文件。

## Knowledge

主路径：

```text
kc schema list --repo <id> [--limit] [--continuation]
kc search (--repo <id> | --dataset <id>) --query …
kc read   (--repo <id> | --dataset <id>) --object …
```

`schema list` 只接受 `--repo`。它只说明该仓已发布 basis 上有哪些实体（`entity`、可选
`description`、打开合同用的 `objectId`）。合同正文走 `read --object schema/…`；检索字段走 `schema describe`。
顶层有 `repository`、`commit` 和可选 `continuation`；不返回 `coverage`、`exhausted`、
`total`，每行不重复 Repository 和 commit。
`log` 与审计分页同样以“有 continuation 表示还有下一页”为准。

产品 CLI 的 `search` 列出匹配身份（`hits[]` 为 `repository` / `objectId` / `commit`），不嵌套正文。
产品 CLI 的 `read` 打开正文（`repository` / `objectId` / `commit` / `value`）。`--dataset` 为同一形状的数组。HTTP SEARCH/READ 仍是协议信封。
`grant list` 列出 `{rules}`；带 `--repo` / `--catalog` 时只列出该范围。
`schema describe` 打印字段 AccessHints 与可选 `origin`（Schema frontmatter 访问路径）；`resolve` 只打印存在性与 commit；`relations` 只打印一跳邻居身份。

进阶对象操作是 `resolve`、`relations`、`provenance`、`log`、`schema describe`、
`binding show`、`access`、`invoke`。`schema describe` 回答字段 AccessHints 与可选 origin，不替代 list。
`access` 按 origin + 实体 ID 读取该 Aspect 观察，`invoke` 调用 ResourceDescriptor，二者不合并。

## 发布

```text
kc diff --repo <id> --dir <drafts>
kc writer commit --command-id <id> --repo <id> --dir <drafts>
kc writer put --command-id <id> --repo <id> --object <id> (--value | --file)
kc writer remove --command-id <id> --repo <id> --object <id>
kc writer head --repo <id>
kc writer receipt --command-id <id>
```

`diff` 对照当前发布版本，列出这个目录会改哪些对象，不写仓。`writer commit --dir` 用同一对照后提交。`writer put` 直接发布一个 Address，不经过草稿目录。HTTP Writer 仍收 ChangeSet。`--changeset` 是进阶操作数。

## 治理、运维与部署

- 治理：Proposal create 把变更写到 candidate，不推进 target；Preview create/validate、Validation record、merge。Preview
  create 用 `--dataset`，本次命令解析一次并冻进 Preview；stdout 是 Preview 坐标，不是合入后正文。
- 运维：Projection、AccessSpec、Hook、Gate、访问审计和反馈；根 help 不铺开进阶叶子。
  `operations audit hitmap` 的 `source` 是 `hitmap`，与访问账的 `source:access` 分开。
  `operations access-spec describe` 接受 `--repo` 或 `--dataset`。
- 部署：`deployment init|status|system publish`、`deployment identity migrate` 和
  `serve --config`。部署操作不属于普通业务旅程。

## Help

- `kc help`：七个产品分组与一句话，不铺命令和 flag。
- `kc help <组>`：该组每条命令一行，主路径在前。
- `kc help <组> <动词>`：先说这条命令干什么，再给 argv 骨架；值有写法的叶子再加上怎么写和能抄的例子。
- `kc help consume|write|compose`：最短真人旅程，不是身份或授权角色。
- 未写完的家族前缀（`kc grant`、`kc catalog`、`kc writer`）输出同一份组索引并失败关闭；不是叶子命令。退役 argv 仍是 `USAGE_INVALID`。
- 每条公开叶子都有语义 + 用法。`USAGE_INVALID` 短路打出失败原因和同一份叶子 help，不是单独一份 error JSON。HTTP 仍是 FaultJSON。

旧 argv 没有兼容别名；拒绝分母由
`TestRemovedCommandsAreRejected` 固定。
