# SCENE-01 · 走查只用协议场景树

- [x] 本轮完成：走查与 `TestProductScenes` 都只用 `.data/scenes`；`Given existing repository` 经 lakeFS adapter 打开既有 authority。父 home 副本分叉独立物理仓（拷目录不够）。Gherkin 仍 store-agnostic。不接 Datasets API，不出现 `warehouse-agent`。

**Goal：** `Given existing repository` 经 `snapshot/lakefs` 打开既有 authority；`TestProductScenes` 父 home 副本必须隔离物理仓（拷目录不够）。owner 为 `doc/architecture.md`（`snapshot-authority-and-derived-media`）与 `.data/scenes/README.md`。

**Non-Goals：** 不改 Gherkin 写 lakefs；不把 Catalog 组件夹具（`NewRegistry` gitdir）改成生产 OpenSnapshotRegistry；不在 `cli` 里 import adapter；不把 docker Graveler 编进协议包；不接 lakeFS Datasets API。

**不变量：** `A-01`、`V-01`；更换 Snapshot driver 不得改变 Reader/Writer/Catalog 命令语义。

**选定与否决：** 选定进程内协议忠实 lakeFS HTTP 假服务作为 hermetic `make test` 夹具，经 `home` 的 lakefs `OpenExisting` 打开；父副本 `ForkStamps` 改写 `remote.yaml` DSN。否决场景执行器继续默认 Dolt；否决把 docker Graveler 编进协议包；否决 Gherkin 写介质名。

**接口与验收：** 夹具在 `internal/testkit`，装配仍走 `cli.AddRepository(..., "lakefs", dsn)`。锚点 `TestSceneExistingRepositoryFixtureUsesLakeFS`（stamp `driver=lakefs` + 两份副本 DSN 互异）与 `TestProductScenes`（全绿、无 skip）。Catalog 仍是 gitdir `NewRegistry`。
