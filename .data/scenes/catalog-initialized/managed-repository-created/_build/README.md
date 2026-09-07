# 已授权接入方创建平台仓

父状态表示已有部署；本节点通过 `runner: go-test` 的正式配置旅程运行，不能用 embedded construct 代替。Oracle：`cli/managed_repository_journey_test.go` 的 `TestManagedRepositoryProviderCreatesPublishesAndResumes`（真实 Dolt）。`cli/managed_repository_gitea_test.go` 的 `TestManagedRepositoryProviderOnLiveGitea` 经正式配置与真实远端 Gitea 验证 create、发布、删缓存恢复和幂等重放，归入标准 gitea 套件。

进入条件：平台已配置独立托管存储和明确 creatorActions；普通 `user:provider` 只有 Catalog 创建准入，目标仓既不存在，也不在静态 repositories 配置中。部署初始化和预授准入由测试框架建立，用户任务不执行它们。

同一主体通过公开 Run → typed HTTP 创建平台仓，立即 PUT 并按回执 commit READ/PROVENANCE；再 pack/commit 批量发布。pack 后回读确认 published HEAD 未动。客户端不提交存储地址、凭证或自选授权。

替换实例是任务间的外部事件：仅删 cache 并重新打开同一配置与耐久状态。原主体重放 create 和 Writer 命令得到原结果，读取历史版本，再按当前 commit/digest 更新。过程中不追加静态 Repository binding、不重新初始化，也不补发权限。

边界探针同属该 Go Oracle：无准入主体创建被拒且库存不变；另一 command-id 不能重复分配同仓，原 command-id 改目标必须冲突，两者都不改变库存和源 HEAD；旁观者不能读写；真实目标仓 grant 撤销后，重启与 create 重放不能重新发权。管理者仅参与已声明的前态与撤权事件，不替接入方完成任务。

该节点没有伪 construct.feature，也不声称被目录 DFS 执行；具名 Go 证据和全套命令执行报告共同验证它。现有 repository-attached 分支继续验证外部既有仓只读接入。

标准验证：`KC_E2E_RUN='^TestManagedRepositoryProviderCreatesPublishesAndResumes$' GO=go ./scripts/testsuite.sh e2e` 复用真实 Dolt 容器；`GO=go ./scripts/testsuite.sh gitea` 包含远端创建旅程。独立 go test 没有原生 Dolt 或复用 wrapper 时会按命令启动容器，不能以放宽 HTTP 超时替代标准依赖装配。两条正式旅程已通过，详细结果以本次测试输出为准。
