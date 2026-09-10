# 产品 CLI 操作语义

公开路径闭集以 [`surface.go`](surface.go) 为权威；HTTP 闭集以
[`httpsurface`](../httpsurface/README.md) 为权威。本文解释产品操作，不复制协议字段。

## 上下文与操作数

- Server 入口来自 `--server`、`KC_SERVER_URL` 或 `client.json`；日常命令不重复填写。
- `catalog use` 按 Server 保存当前 Catalog；只有一间可见时 `catalog list` 自动选择。
- 单 Repository 操作用 `--repo`，多 Repository 任务先 `workspace pin`，后续用 `--pin`。
- 知识与 Writer 命令拒绝 `--catalog`、`--workspace`、`--source`。
- 对象用 `--object`；Writer 提交必须由用户提供 `--command-id`。
- 默认 Snapshot ref 是 `snapshot.DefaultRef`；历史复核才显式给 `--commit` 或 `--ref`。

## 身份

| 操作 | 语义 |
|---|---|
| `login` | 用已知 Server 发现认证模式，验证身份并保存本机会话；回执不重复 Server 地址 |
| `logout` | 清除当前 Server 的本机会话 |
| `whoami` | 显示当前认证主体 |
| `admission show` | 显示本人当前 grants，以及外部申请 URL 和当前 grant 管理者 |

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
| `grant add\|list\|remove` | 管理授权；`add` 的 `--repo` 与 `--catalog` 二选一 |

`catalog audit` / `catalog archive` 是进阶 Catalog 操作。HTTP 仍可保留资源分层的集合读取；
产品 argv 不镜像每条集合 GET。

## Workspace

Workspace 只用于同一任务需要多个 Repository：

```text
kc workspace pin --source <repo>[=<selector>] … [--out pin.json]
kc workspace pin --workspace <id> [--out pin.json]
kc workspace check --pin pin.json
```

`workspace define` / `retire` 管理可复用配方；`overlay` 是本机操作。单 Repository 消费不先
pin。`kcfs plan|mount` 必须使用含 Catalog 与 Workspace 坐标的 `--pin`。

## Knowledge

主路径：

```text
kc knowledge schema list --repo <id> [--limit] [--continuation]
kc knowledge search (--repo <id> | --pin <file>) --query …
kc knowledge read   (--repo <id> | --pin <file>) --object …
```

`schema list` 只接受 `--repo`，返回顶层 `repository`、`commit`、`schemas` 和可选
`continuation`；不返回 `coverage`、`exhausted`、`total`，每条 Schema 不重复 Repository 和
commit。`knowledge log` 与审计分页同样以“有 continuation 表示还有下一页”为准。

进阶对象操作是 `resolve`、`relations`、`provenance`、`log`、`schema describe`、
`binding show`、`access`、`invoke`。`access` 读取 Binding 观察，`invoke` 调用
ResourceDescriptor，二者不合并。

## 发布

```text
kc pack --repo <id> --dir <drafts> [--base] [--out changeset.json]
kc writer commit --command-id <id> --changeset <file>
kc writer put --command-id <id> --repo <id> --object <id> (--value | --file)
kc writer remove --command-id <id> --repo <id> --object <id>
kc writer head --repo <id>
kc writer receipt --command-id <id>
```

`pack` 是本机预处理，不连接 Server。`commit` 的 Repository 在 ChangeSet 内；同时给
`--repo` 时必须一致。

## 治理、运维与部署

- 治理：Proposal create/merge、Preview create/validate、Validation record。Preview
  create 必须用 `--pin`，不能重新解析命名 Workspace。
- 运维：Projection、AccessSpec、Hook、Gate、访问审计和反馈；根 help 不铺开进阶叶子。
- 部署：`deployment init|status|system publish`、`deployment identity migrate` 和
  `serve --config`。部署操作不属于普通业务旅程。

## Help

- `kc help`：七个产品分组与一句话，不铺命令和 flag。
- `kc help <组>`：该组每条命令一行，主路径在前。
- `kc help <组> <动词>`：用法、互斥操作数与必填 flag。
- `kc help consume|write|compose`：最短真人旅程，不是身份或授权角色。

旧 argv 没有兼容别名；拒绝分母由
`TestRemovedCommandsAreRejected` 固定。
