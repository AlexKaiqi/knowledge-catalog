# integrationruntime/

墙外 Collector 的构建与运行宿主。它复用 KC 登录，通过 typed Writer API 获取基点、提交变更；源协议和 Address 映射仍在接入方自己的 integration repo。所有权见 [外部资源访问](../docs/reviewed/resource-access.md) §4；纯对账合同见 [`connector/README.md`](../connector/README.md)。

## 接入与运行

先用 `kc login --server <KC地址>` 登录，选定自己有 Writer 权限的 Repository。服务端仍在每次请求时检查当前权限。运行宿主与 KC Server 分开启动：

```sh
go build -o /tmp/kc-integration ./cmd/kc-integration
/tmp/kc-integration build --file /absolute/provider/artifact.json
/tmp/kc-integration activate --file /absolute/provider/integration.json
/tmp/kc-integration run --id notes-sync
/tmp/kc-integration status --id notes-sync
/tmp/kc-integration daemon
```

`build` 的配置属于接入方 integration repo，例如：

```json
{
  "id": "notes-provider",
  "directory": "/absolute/provider",
  "build": ["go", "build", "-o", "collect", "./cmd/collect"],
  "executable": "collect",
  "credentialEnv": ["SOURCE_TOKEN"]
}
```

`directory` 必须是绝对路径。`build` 可省略以登记已经构建的可执行文件；指定时是 argv，运行宿主不代加 shell。文件按内容 digest 原子保存，激活时冻结具体 artifact。可执行文件应自包含；运行位置是保存的 artifact 目录，不复制 integration repo 的其他文件。更新代码后再次 `build`、`activate` 才切换运行版本。构建环境不继承源凭证或 KC token，使用运行目录内独立的 HOME 和 Go cache。

`activate` 的 manifest 固定目标、Scope 与调度：

```json
{
  "id": "notes-sync",
  "artifactId": "notes-provider",
  "repository": "kr://kaiqidong/notes",
  "mode": "patch",
  "scope": {"aspects": ["content"], "objectPrefix": "note/"},
  "intervalSeconds": 300,
  "timeoutSeconds": 30
}
```

省略 `server` 时使用 `--server`、`KC_SERVER_URL` 或登录后保存的 `client.json` 入口。激活后把 Server 保存到该 integration；以后更改默认登录入口不会让已有任务切到另一个服务。`ref` 省略时用 `snapshot.DefaultRef`。调度间隔为 1–86400 秒，采集超时默认 30 秒、最多 600 秒。

`run` 立即执行一次。`daemon` 持续执行到期的任务；每次完成输出一条 JSON 状态，某个任务失败不阻止其他任务。运行方可交给自己的进程管理器保持启动与收集 stdout。控制命令：

```sh
/tmp/kc-integration pause --id notes-sync
/tmp/kc-integration resume --id notes-sync
/tmp/kc-integration status --id notes-sync
```

暂停阻止后续采集；已经发出的 Writer 请求仍按原 command 恢复。相同 integration ID 不能换 owner、Server、Repository 或 ref。存在未决写入时不能换 artifact 或 Scope，先恢复原请求。

## Collector 输入与输出

进程 stdin 是一个 [`CollectInput`](types.go)，包含激活 manifest、上次成功的 `checkpoint` 和本次 Writer Head 返回的固定 `baseCommit`。stdout 必须只输出一个 [`Collection`](types.go) JSON，最多 8 MiB；stderr 不写入运行账。示例形状：

```json
{
  "desired": [{
    "address": {"kind": "Aspect", "objectId": "note/one", "aspectName": "content"},
    "value": {"text": "source value"},
    "schemaRef": "schema/note-content"
  }],
  "observed": [],
  "sourceRefs": ["source://batch/42"],
  "cursor": "42"
}
```

其中 Schema 必须已按普通 Writer 流程存在且能解析。例中 `observed: []` 仅适用于本次没有既存观察的情形；不要以空数组代替读取失败。接入方负责 source key → Address 映射，并从 `baseCommit` 读取对账所需 `Observed` 的 value digest 与 declaration digest。需要从 KC 读取时可在 Collector 中调用普通 `kc` 读取命令；运行宿主提供非秘密的 `HOME`、`KC_CONFIG_DIR` 路径和固定的 `KC_SERVER_URL`，复用同一份按 Server 隔离的登录与刷新逻辑。读取必须显式使用 stdin 的 `baseCommit`，不能重新解析浮动 head。`checkpoint.commit` 是上次成功发布的位置，不代替本次基点。

运行宿主固定 `targetRepository`、`targetRef`、`baseCommit`、Actor、Mode 与 Scope，再调用 `connector.Preview`。输出不能覆盖这些值。`patch` 只 PUT Desired；`reconcile` 只在 `Observed ∩ Scope` 中推断 REMOVE。Desired 越界整批失败。采集源访问失败应以非零状态退出，避免把失败翻译为空集合造成删除。Schema、provenance、CAS、当前权限与幂等仍由 Server Writer 执行。

## 凭证与状态

`credentialEnv` 只保存环境变量名。运行进程从自己的环境取得值，每次采集重新读取；未提供值则失败并保留原 checkpoint。`KC_*`、HOME、PATH 等平台或进程环境名不能作为源凭证引用。运行宿主不把用户 token 或部署应用秘密放进 manifest、stdin、artifact 元数据或状态输出。使用 `kc login` 保存会话后，长时间运行可复用刷新；换用户后旧 integration 会拒绝以另一个 owner 运行。

这是同一 OS 用户显式构建并授权运行的程序，不是代码沙箱。源程序自行读取保存的 KC 会话时具有该用户的现有权限；应由接入方控制构建来源。源凭证通过外部进程配置供给，不写入 JSON 配置或知识声明。

默认本地运行目录为 `~/.local/state/kc/integrations`，可用 `--state-dir <绝对路径>` 覆盖。私有 Bolt 账保存 artifact、激活配置、执行计数、最后错误码、下一次调度时间、checkpoint 和原始待提交 ChangeSet；后者可能含采集内容，但不用于保存凭证。目录、账和 artifact 一起备份。已经初始化的账丢失或损坏时拒绝重新创建，必须恢复原状态。

非空 Preview 在调用 Writer 前先保存原 `commandId` 和完整 ChangeSet。成功回执后才推进 cursor；丢响应或进程重启先重试同一 command，不重新采集。确定的 `NON_FAST_FORWARD`、`SCHEMA_UNSUPPORTED`、`SCHEMA_REVISION_UNRESOLVED`、`SCHEMA_INSTANCE_INVALID`、`SCHEMA_INCOMPATIBLE` 拒绝释放原 pending，保留旧 cursor；可以修正并重新构建、激活 artifact，下一次重新取基点。只有这些已证明未发布的 Writer 拒绝允许重采；通用 HTTP 4xx、`USAGE_INVALID`、`PRECONDITION_FAILED`、认证/权限/归档错误均保留原 pending，因为它们不能排除之前已经发布但回执未到达。空 Preview 跳过 Writer 并推进到已完成的源 cursor。进程间使用耐久 lease，任务超时在 lease 内结束，重启后到期 lease 可恢复。

`status` 无需在线登录或源服务，返回保存的 owner、目标、active、phase、checkpoint、pending command ID、最后错误码、运行计数及调度时间。它不返回待提交内容或凭证。`FAILED` 且有 pending 表示原 Writer 结果仍待恢复；`PUBLISHED` 表示已取得发布回执，`UNCHANGED` 表示对账为空。

## 验证

`runtime_test.go` 覆盖失去提交响应后的重启与数字精度、Scope、空预览、调度暂停、artifact 完整性与中断重建、凭证隔离及状态丢失。`cmd/kc-integration/main_test.go` 覆盖真实命令链和保存登录的复用；`cli/integration_runtime_journey_test.go` 经实际 HTTPHandler/Writer 发布并从固定版本回读。
