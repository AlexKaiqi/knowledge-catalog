# DOLT-01 · 原生 Dolt 每条查询付一次引擎冷启动

> 状态：已随 Dolt adapter 退役关闭；以下保留为历史选型与实测记录。

- [x] 本轮完成：让同一 Repository 在一条命令内复用常驻 Dolt 查询会话，取消每条 SQL 一个 `dolt` 进程的执行方式。

**问题与实测：** `snapshot/dolt/command.go` 的 `query` 每次都执行一个新的 `dolt sql -r json -q`。本机无宿主 `dolt` 时走 Docker fallback，单条查询实测约 750 ms（`docker run --rm`）或约 135 ms（常驻容器 `docker exec`）；开销是 `dolt` 进程启动，与数据量无关。一条只写 2 个对象的 CLI 用例发出 113 次 `dolt` 进程调用，其中 39 次 `DOLT_HASHOF`、13 次 `dolt_branches`、12 次 `SHOW TABLES LIKE` 共 64 次（57%）是每次 `home.Open` 重复的开仓元数据探测。结果是 `go test ./cli` 单条用例耗时 150～220 秒并触发整包超时。这是执行通路缺陷，不是数据规模结论。

**Goal：** 把 `query` 改为向该 rootDir 的常驻 `dolt sql -r json --continue` 会话逐条发送语句并读回结果，会话由既有 `Home.Close()` → `snapshot.Registry.Close()` 释放，即 CLI 是一条命令、Server 是被服务的 Home；变更类命令先释放会话，Dolt 仍只有一个活动写者。owner 为 `doc/architecture.md`（`snapshot-authority-and-derived-media`）与 `snapshot/dolt/README.md`。

**Non-Goals：** 不引入新依赖（含 `database/sql` 与任何 MySQL wire driver）；不改 `snapshot.Store` / `TreeStore` / `HistoryStore` 公开形状；不改 ref、CAS、archive、幂等与错误码语义；不改 `kc_files` / `kc_units` / `kc_objects` 表形状；不把 `dolt sql-server` 作为运行时依赖；不改 Gitea adapter；不把测试夹具的容器编排搬进协议代码（`doc/architecture.md`、`SCALE_ARCHITECTURE.md`、`doc/architecture.md`）。

**不变量：** `A-01`、`W-02`、`V-01`；provider 执行通路改变不得改变 Reader/Writer/Catalog 与命令语义，CAS 与命令幂等保持，AS OF 固定 basis 回读结果逐字节不变。

**选定与否决：** 选定按 rootDir 复用的常驻 `dolt sql --continue` 会话（纯 `os/exec`，实测 0.4 ms/语句），结果走 stdout、错误走 stderr 逐条归属，写语句无结果集故以哨兵语句框定响应；任何变更类命令（`init`/`add`/`commit`/`checkout`/`reset`/`branch`/`sql -q` DDL 与 `runSQLScript`）先关闭该 rootDir 的会话再执行，因为实测会话持有独占写锁会使外部进程写入变为 `database is read only`。否决 `dolt sql-server` + `go-sql-driver/mysql`（未批准依赖，且 server 通路把 `COUNT(*)` 等整数结果变成字符串，改变既有 JSON 解码）；否决仅启动 `sql-server` 而客户端仍是 `dolt sql -q` 进程（实测约 205 ms/查询，比现状更慢）；否决把 Gitea 换成 CLI 测试默认 store（会替换掉 native Dolt 默认路径覆盖，与 DOC-14 / provider 合同缺口冲突）；否决只在测试夹具复用容器（仅省约四成，不解决调用次数）。

**接口与验收：** 会话实现私有于 `snapshot/dolt`，不出现在包外公开 API；先跑会失败的进程计数证据（同一流程的 `dolt` 进程启动数上界），再实现，再以 Dolt `RepositoryContract` / `WriterContract`、`snapshot/dolt`、`object/dolt` 与 `make check-docs` 验收，不以 skip 或删除断言换绿。
