# snapshot/dolt/

规模化 Snapshot authority：

| 层 | 实现 | 状态 |
|---|---|---|
| ⓪ Snapshot | Dolt | `DoltRepository` 实现 `snapshot.Store` / `TreeStore` / `HistoryStore`；commit、branch、AS OF 与通用 native SQL transaction 由 Dolt 提供，不解释知识格式 |

`kc_files` 只服务 `RAW_WRITE` 与 TreeStore conformance，不是规模化知识 Canonical。`knowledge/dolt/` 在②层拥有 `kc_units`、`kc_objects`；Relation endpoint/type/role 只进入 layer ③ 投影，CLI 的 dolt driver 打开该② wrapper。

接入和进程恢复使用 `OpenExisting`：只读检查已有 `.dolt`、已存在的身份 stamp 与 published ref；不创建数据库、不补 stamp、不安装表。没有 Knowledge 表不影响 Snapshot 身份。只要已有可解析的 published commit，零表或零知识对象也有效；“空源”是缺少 published snapshot，不以业务数据量判断。`OpenDolt` 保留为显式创建或初始化路径，供创建流程与测试夹具使用，不能用于接入/恢复。

平台托管供给使用 `CreateManaged`，只在服务分配的目录创建持久 allocation marker，再初始化 Snapshot substrate；已有目录必须持有相同 marker，否则不收养、不补 stamp。持有归属证据的部分初始化可在同一请求重试中完成；若进程恰在建目录后、marker 持久化前中断，缺失归属证据仍须恢复或核对，不能自动接管该目录。`OpenManaged` 只恢复既有源，并在所有后续引擎调用（含 NativeQuery 与写入）前验证 marker。②层的托管创建组合原生 Knowledge 表初始化，恢复则只验证兼容性；业务知识始终经 Writer。

Dolt 优先使用 `KC_DOLT_BIN`，其次是 PATH 中的 `dolt`，最后可用 Docker fallback；`KC_DOLT_DOCKER_IMAGE` 固定镜像，`KC_DOLT_FORCE_DOCKER=1` 强制 Docker。密码只走相应环境变量，不写 stores.yaml。

一次 `dolt` 调用启动整个引擎，因此不可变与元数据查询共用该数据库目录的常驻 `dolt sql` 会话：有界读取的成本不再随调用次数增长。每条语句后跟一条确认查询来框定回复，因为 provider SQL 跨行、诊断也会回显失败语句。会话持有数据库写租约，所以每个变更类命令先关闭会话再执行，Dolt 仍然只有一个活动写者；会话随打开该 Repository 的 Home 由 `Close()` 释放（CLI 是一条命令，Server 是被服务的 Home）。Docker fallback 给会话容器固定名字，`Close()` 会 `docker rm -f` 该容器：只杀掉 docker 客户端会留下 bind-mount 占用 `.dolt/noms`。会话不可用的环境自动退回单次进程，结果相同。

OpenSearch 位于 `retrieval/opensearch/`，本包不依赖 Index/Reader。动态 state/stream 属于 Aspect Binding 指向的上层运行时，不是 Snapshot authority 或 cache。

| 文件 | 负责 |
|---|---|
| `dolt.go` | adapter 实体与 Snapshot capability 断言 |
| `open.go` | 数据库初始化、stamp 与 archive 初始状态 |
| `command.go` | 本机 binary / Docker 选择、命令执行和 SQL JSON transport |
| `session.go` | 按数据库目录复用的查询会话、逐语句回复框定与写租约释放 |
| `refs.go` | ref、commit、CAS merge 与 archive 生命周期 |
| `tree.go` | `kc_files` 的字面 TreeStore 读写 |
| `native.go` | provider-neutral SQL transaction、CAS commit 与 schema bootstrap substrate |
| `knowledge_history.go` | ⓪ commit history；不解释 object_id |
