# DEBT-DATASET-CLONE-RETIRED-01 · 移除本地 clone 交付树物化

## 债

位置：`snapshot/cli/clone.go`（已删除）、`cli/dataset_clone_journey_test.go`（已删除）、
`snapshot/cli/clone_empty_test.go`（已删除）、`.data/scale/scenes/.../probe-clone-delivery-curve.yaml`（已删除），
以及 `snapshot/cli/{commands,executor,help_dataset,help_concepts,help_tasks}.go`、`cli/help.go`、
`cli/SURFACE.md`、`systemrepo/content/capabilities/datasets/entrypoints.yaml` 中的声明与文案。

信号：`kc dataset clone` 把整棵交付树急切拷贝到普通本地目录，与「拦截实际访问」的方向相反——
它先落盘全量字节再交给外部工具，既没有按需取数，也不随服务端版本演进；消费方拿到的是一份
脱离授权检查的静态副本，撤回权限无法追回。

判定：该实现方向错误，不是参数或细节问题。用户 2026-10-21 明确：正确做法是类似 Cloudflare
ArtifactFS、Meta EdenFS / Mononoke 那样在实际访问处拦截，本地物化组件没有价值，先行清理，后续重做。

处置：整条链路移除，包括命令声明、分发分支、叶帮助、概念页与任务页引用、公开面清单、能力清单
入口、专属测试与压测探针及其指标与档位。`dataset clone` 登记进 `TestRemovedCommandsAreRejected`
的退役分母，防止被静默重新纳入。`kcfs`（FUSE 惰性只读挂载，`datasetfs/` + `cmd/kcfs`）不属于
本次清理范围：它在访问时按需向文件网关取数、每次请求仍检查当前授权，方向与新方案一致，保留。

触发条件：重做本地消费能力时，新实现须在访问路径上拦截并按需取数，不得以「先全量拷贝到本地目录」
作为交付方式；`kc dataset clone` 这一 argv 形状不再复活，需要本地目录形态时用 `kcfs mount` 或
逐文件 `files read --out`。
