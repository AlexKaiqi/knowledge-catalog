# 产品 CLI 操作语义

产品 argv、help 与操作数的应然设计见 [CLI 交互](../docs/reviewed/cli.md)。
公开路径闭集以 [`surface.go`](surface.go) 为权威；HTTP 闭集以
[`httpsurface`](../httpsurface/README.md) 为权威。本文解释产品操作，不复制协议字段。

## 上下文与操作数

- Server 入口来自 `--server`、`KC_SERVER_URL` 或 `client.json`；日常命令不重复填写。
- 一套 Server 对应一个登记空间；没有 Catalog 对象、当前选择或切换。
- 基本对象读写由对象 URN 指定来源仓和身份；范围操作用 `--repo`，已定义 Dataset 用 `--dataset`。回执带 `commit`；对象历史读取使用原 URN 加 `--commit`。
- 知识命令拒绝 `--catalog`、`--source`、`--pin`。Writer、`diff` 与 `schema list` 拒绝 `--dataset`。
- Object `read`、`writer put/remove` 只接受位置参数中的完整对象 URN＋locator，拒绝 `--reference` 别名及 `--repo/--object/--aspect/--member/--include/--exclude`，即使与引用内容相同也拒绝。Writer 提交由用户提供 `--command-id`。
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
| `show` | 显示服务登记空间 已 attach 的 Repository 与命名知识集；不是对象目录 |
| `create --name [--store]` | 供给一个 KC 可打开的托管 Repository；不 attach |
| `create --url --credential-file` | 连接批准来源上的自有 Repository；不 attach |
| `attach --repo` | 将已连接 Repository 登记进服务登记空间；不要求已发布，不发读权 |
| `detach --repo` | 从服务登记空间 移除成员；不归档或删除 Snapshot |
| `grant add\|list\|remove` | 管理授权；`add` 可限定 `--repo` 或 `--dataset`，两者省略表示全服务范围。`list` 打印 `{rules}`，可按仓或 Dataset 过滤；不是整份 allow 文件 |

`registry audit` 是登记历史入口；没有整个登记空间的归档操作。HTTP 保留资源分层的集合读取；
产品 argv 不镜像每条集合 GET。

## 知识集

Dataset 按消费任务选择一个或多个 Repository 的确定版本中的文件，并组织交付布局。
单仓文件选择同样成立；配方是输入形式，接受的 Dataset 发布才是服务交付范围。

```text
kc dataset define <id> --revision <n> --source <repo>[=<selector>] …
kc dataset retire <id>
kc dataset overlay --file <recipe.yaml> --overlay <overlay.yaml>
kc search --dataset <id> --query …
kc read   <object-urn[#/locator]> --dataset <id>
kcfs plan  --dataset <id> --root <project>
kcfs mount --dataset <id> --root <project>
```

`dataset define` 发布新版本，固定来源与文件范围并推进当前服务版；`retire` 停止后续消费，保留已接受发布记录。
位置参数 `<id>` 与 `--dataset` 相同。源仓后来提交不改变既有 Dataset；采用更新需要再次发布。
目录映射中，一旦任一目录来源声明交付路径，其余目录来源也须声明；逐文件条目使用自己的来源与交付路径。
`overlay` 是本机 `--file` 底稿加 `--overlay` 补丁，不发表；stdout 交付合成定义，`--out` 写便携配方。
`clone` 物化当前服务版到空目录或不存在的目录，不覆盖已有内容。解析与文件消费分别需要相应 Dataset 权限。
`kcfs plan|mount` 在调用时内部冻结本次 ResolvedKnowledgeSet，产品 argv 不出现 pin 文件；当前宿主投影不支持逐文件条目，改用 clone 或文件网关。

## Knowledge

主路径：

```text
kc schema list --repo <id> [--limit] [--continuation]
kc search (--repo <id> | --dataset <id>) --query …
kc read <object-urn[#/json/pointer]> [--commit <id> | --ref <name>]
```

`schema list` 只接受 `--repo`。它只说明该仓已发布 basis 上有哪些实体（`entity`、可选
`description`、打开合同用的 `objectId`）。`schema describe` 保留完整 definition（结构与约束）及附加字段访问声明；原始正文也可走 `read`。
顶层有 `repository`、`commit` 和可选 `continuation`；不返回 `coverage`、`exhausted`、
`total`，每行不重复 Repository 和 commit。
`log` 与审计分页同样以“有 continuation 表示还有下一页”为准。

产品 CLI 的 `search` 列出匹配身份（`hits[]` 为 `repository` / `objectId` / `commit`），不嵌套正文。
`--recall lexical|semantic` 选择召回策略（省略即 `lexical`）；`semantic` 依赖向量投影与 embedding 能力，
缺任一即失败关闭，不降级为 lexical。
产品 CLI 的 `read` 打开一个来源对象的完整正文或指定数据，保留引用、locator、来源身份、固定 commit、维护 Address/digest、实际 declarations/provenance 和 value。Dataset 读取也返回该引用的单个结果。逻辑 State 结果保留 observations，不能把源观察当固定对象正文。HTTP SEARCH/READ 仍是协议信封。
`grant list` 列出 `{rules}`；带 `--repo` / `--dataset` 时只列出该范围。
`schema describe` 打印完整 definition、附加字段 AccessHints 与可选 `origin`（Schema frontmatter 访问路径）；`resolve` 只打印存在性与 commit；`relations` 返回命中的关系对象坐标，邻居身份位于关系端点，不是邻居正文。

进阶对象操作是 `resolve`、`relations`、`traverse`、`provenance`、`log`、`schema describe`、
`binding show`、`access`、`invoke`。`schema describe` 回答结构、约束、附加字段 AccessHints 与可选 origin，不替代 list。
`traverse` 在请求开始固定的范围内（Dataset pin 或单仓 commit）做有界邻域闭包：`nodes` 按对象去重并带最小
跳深，`edges` 是入选关系对象（保留关系坐标与完整端点），引用到范围外的 frontier 列入 `boundary`；
不枚举全部路径、不承诺最短路径，游标续传闭包增量。
`access` 按 origin + 实体 ID 读取该 Aspect 观察，`invoke` 调用 ResourceDescriptor，二者不合并。

## 发布

```text
kc diff --repo <id> --dir <drafts>
kc writer commit --command-id <id> --repo <id> --dir <drafts>
kc writer put <object-urn[#/json/pointer]> --command-id <id> (--value | --file)
kc writer remove <object-urn[#/json/pointer]> --command-id <id>
kc writer head --repo <id>
kc writer receipt --command-id <id>
```

`diff` 对照当前发布版本，列出这个目录会改哪些对象，不写仓。`writer commit --dir` 用同一对照后提交。`writer put/remove` 只将完整对象引用发送 Object 服务，由服务确认可维护单元；PUT 替换完整 blob/Aspect/Member，空 locator REMOVE 删除整个对象。首次建立 Member 显式 `--kind Member`，不从深度推断模型；深字段、数组索引和集合整体不新增写粒度。基本 `read/put/remove` 拒绝分列寻址；内部 Address 与批量 ChangeSet 是执行合同，不形成第二套基本入口。目录出版仍是应用流程，`--changeset` 是进阶操作数。

## 治理、运维与部署

- 治理：Proposal create 把变更写到 candidate，不推进 target；Preview create/validate、Validation record、merge。Preview
  create 用 `--dataset`，本次命令解析一次并冻进 Preview；stdout 是 Preview 坐标，不是合入后正文。
- 运维：Projection、AccessSpec、Hook、Gate、访问审计和反馈；根 help 不铺开进阶叶子。
  `operations audit hitmap` 的 `source` 是 `hitmap`，与访问账的 `source:access` 分开。
  `operations access-spec describe` 接受 `--repo` 或 `--dataset`。
- 部署：`deployment init|status|system publish`、`deployment identity migrate` 和
  `serve --config`。部署操作不属于普通业务旅程。

## Help

- `kc help`：直接解释 Object 核心概念、联系、能力与一个定位示例，同时保留产品分组/topic；不倾倒叶参数。模型说明本地可用，深入参数走叶 help；Snapshot 文件消费可继续查看 `kcfs help`。
- `kc help <组>`：该组每条命令一行，主路径在前。
- `kc help <组> <动词>`：先说这条命令干什么，再给 argv 骨架；值有写法的叶子再加上怎么写和能抄的例子。
- `kc help consume|write|compose`：最短真人旅程，不是身份或授权角色。
- 未写完的家族前缀（`kc grant`、`kc registry`、`kc writer`）输出同一份组索引并失败关闭；不是叶子命令。退役 argv 仍是 `USAGE_INVALID`。
- 每条公开叶子都有语义 + 用法。`USAGE_INVALID` 短路打出失败原因和同一份叶子 help，不是单独一份 error JSON。HTTP 仍是 FaultJSON。

旧 argv 没有兼容别名；拒绝分母由
`TestRemovedCommandsAreRejected` 固定。

## 按需文件与条件授权更新

`dataset show` 查看所选发布的完整定义、元数据和来源，以及一页真实历史和当前服务版；
历史游标在新发布后失效，退役后仍可核对定义。该入口沿库存查看权，不检查来源字节可用性。

```text
kc files list --dataset <id> [--revision <n>] [--path <dir>] [--limit <n>] [--continuation <cursor>]
kc files read --dataset <id> [--revision <n>] --path <file> [--offset <n>] [--length <n>]
kc files read --dataset <id> [--revision <n>] --path <file> --out <local-file>
kc grant update --id <id> --file <replacement-rule.json> --expected-file <observed-rule.json>
```

文件路径属于交付树，支持目录映射和重命名单文件混排；目录页只含直接子项并返回可继续
读取的路径。首次省略发布版只选当前一次，续页须显式沿用返回版号；后续读取沿用同版才能
延续依据，独立命令默认当前版不构成同一基点。解析、每页和每块均检查当前权限。
默认读取是 JSON 中的 base64 字节块，`--out` 则完整下载单文件且不覆盖既有目标。
块大小有界不表示源端 Range I/O 或内存有界。这些入口复用 Snapshot 网关；独立 `kcfs`
作为外部宿主客户端，不归 Snapshot 挂载实现。

授权更新使用此前观察的单条规则和完整新规则，通过已有 Admin PUT 原子替换；保持 ID
与来源，省略范围表示清空。同一边界检查当前主体对旧、新范围的管理权；过期观察或失败
保留旧规则，不通过自动刷新 expected 或 remove/add 掩盖并发修改。
