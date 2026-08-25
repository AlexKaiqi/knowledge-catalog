# snapshot/dolt/

规模化 Snapshot authority：

| 层 | 实现 | 状态 |
|---|---|---|
| ⓪ Snapshot | Dolt | `DoltRepository` 用原生 Dolt `kc_files` 版本表实现 `snapshot.Store` / `TreeStore`，并提供可选② `knowledge.Repository` capability；commit、branch、AS OF 由 Dolt 提供 |

Dolt 优先使用 `KC_DOLT_BIN`，其次是 PATH 中的 `dolt`，最后可用 Docker fallback；`KC_DOLT_DOCKER_IMAGE` 固定镜像，`KC_DOLT_FORCE_DOCKER=1` 强制 Docker。密码只走相应环境变量，不写 stores.yaml。

Elasticsearch 与 StarRocks 已分别移到 `retrieval/elasticsearch/`、`retrieval/starrocks/`，本包不依赖 Index/Reader。动态 state/stream 属于 Aspect Binding 指向的上层运行时，不是 Snapshot authority 或 cache。

| 文件 | 负责 |
|---|---|
| `dolt.go` | adapter 实体与 Store/TreeStore/Knowledge capability 断言 |
| `open.go` | 数据库初始化、stamp 与 archive 初始状态 |
| `command.go` | 本机 binary / Docker 选择、命令执行和 SQL JSON transport |
| `refs.go` | ref、commit、CAS merge 与 archive 生命周期 |
| `tree.go` | `kc_files` 的字面 TreeStore 读写 |
| `knowledge_commit.go` | Knowledge ChangeSet → TreeChangeSet |
| `knowledge_read.go` | Resolve/Read/Address/List/Provenance |
| `knowledge_history.go` | Object LOG/DIFF 与历史遍历 |
