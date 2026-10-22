# DEBT-SHARED-SEARCH-UNOWNED-GROUPS-01 · 非 lakefs 组复用共享搜索实例时没有所有权前缀与清理

## 债

- 位置：`scripts/testsuite.sh` 的 `start_local_opensearch` 与 `run_lakefs`；`scripts/opensearch_fixture.py` 的 `run_owned`。
- 信号：local 部署实例 `http://127.0.0.1:19200` 上有 411 个 `kc-proj-<12 hex>-g-<hex>` 单分片索引，创建时间集中在
  2026-10-21T12:33Z–12:59Z（本地 20:33–20:59），即 `make test-contracts`（`20261021T124518Z-fa8653d28286`）的窗口。
  每个索引都有一条 `kc-projection-control-v1` 控制文档指向它，仓库名为测试夹具形状
  （`dataset-index/agent`、`kr://conformance/*`、`kr://acme/history/source` 等）；该部署 `kc show` 只登记
  `kr://kc/system`。411 个索引全部是 1 主分片 0 副本——这正是 `home/config/stores.go` 在看到
  `KC_TEST_OPENSEARCH_URL` 时给测试夹具设的形状（`stores.OpenSearch.PrimaryShards = 1`），产品默认是 8 主分片。
  411 个索引占用 411 个分片，占实例 1000 分片上限的 41%；指向这些索引的控制文档共 1057 条
  （同一索引上残留了多条代记录）。
- 判定：这不是意外。`doc/engineering/testing.md` 明确写着“显式 endpoint 保留既有绑定行为，不拿自动实例租约”，
  即配置了 `KC_TEST_OPENSEARCH_URL` 时各测试入口直接绑定该实例，不做所有权登记。缺口在于这条合同没有配套
  的清理责任：`lakefs` 组额外经过 `opensearch_fixture.py run`，会生成 `KC_TEST_OPENSEARCH_RUN_PREFIX` 并在
  退出时精确清理本次前缀；`component`、`boundary`、`e2e`、`contracts` 组只经 `start_local_opensearch`
  拿到端点，既没有前缀也没有运行后回收。于是“绑定别人的实例”这件事成立，但“绑定后留下什么”没人负责。
  验收执行器不属于这个问题：它每次都用本次前缀精确清理并回到运行前容量（本轮 26 条子集运行前后均为
  426/1000）。
- 处置：需要定一件事——显式 endpoint 指向长期实例时，是要求这些组自己带所有权前缀并清理，还是拒绝执行
  并提示改用专属实例。取舍受既有限制约束：`opensearch_fixture.py` 的 `require_scene_command` 已明确拒绝
  普通 Go 夹具采用场景 namespace transport，因此这些组更可能应绑定专属实例，而不是假装拥有共享实例里的
  资源。判定落定前不改文档合同、不动 `lakefs` 已成立的前缀语义。
- 触发条件：任何一次在 `KC_TEST_OPENSEARCH_URL` 指向长期实例的环境里运行 `make test-contracts` 或
  `make test-all`。本轮已在 `ACCEPT-HARNESS-01` 的真实验收里作为环境容量上限出现：全量 51 条 probe 需要
  960 个分片，实例只有 574 空闲。

## 目标

复用共享搜索实例的测试组，要么只写本次拥有的资源并在结束时清除，要么在入口明确拒绝，不留下无法确权的
索引与分片占用。

## 依据

- [测试与验证](../../doc/engineering/testing.md)
- [工程质量闭环](../../doc/engineering/quality.md)
- 既有清理合同：`DEBT-SHARED-SEARCH-CLEANUP-01`（并入 `ACCEPT-HARNESS-01`）与
  `DEBT-TEST-OPENSEARCH-LEASE-01`（`done/`）。

## 验收

- 在 `KC_TEST_OPENSEARCH_URL` 指向一个已运行实例的情况下运行 `component`/`boundary`/`e2e`/`contracts`，
  结束后该实例的索引集合与分片计数和运行前完全一致；无法做到时入口以非零退出并说明该组需要专属实例。
- 离线回归覆盖入口的拒绝与清理两条路径，不依赖 Docker 与真实搜索实例。
- 不删除他人索引，不提高 `cluster.max_shards_per_node`，不把容量不足记成测试通过。

## 前置

无。与 `ACCEPT-HARNESS-01` 共用同一个共享搜索实例，实际执行时两者不能同时占用。

## 不做

- 不清空共享实例，不删除本轮之前已存在的未确权残留；那 411 个索引需要单独的归属裁决。
- 不改 `lakefs` 组已经成立的前缀所有权与清理语义。
- 不为让门禁变绿而降低断言或增加 skip。
## 交付（2026-10-21）

处置落定：显式 endpoint 指向长期实例时，这几组不假装拥有共享实例里的资源，而是由入口负责“只回收
本次新增”。实现三处：

- `scripts/opensearch_fixture.py` 新增 `inventory(endpoint)`、`cleanup_added(endpoint, before)`、
  `run_guarded(endpoint, command)`，argparse 的 action 扩为 `('run','guard','capacity','inventory')`。
  `cleanup_added` 只删快照之外的新增索引，逐个精确 DELETE，从不用通配；返回 `added`/`failed`/`leaked`，
  `failed` 或 `leaked` 非空即让该组以非零退出。`run_guarded` 复用 `run_owned` 的信号转发、
  `grace_seconds` 强杀、`start_new_session=True` 与 `os.killpg` 结构，只把清理改为差集回收。
- `scripts/testsuite.sh` 新增 `run_group_on_shared_search`：设了 `KC_TEST_OPENSEARCH_URL` 且未持
  `KC_GROUP_GUARD` 哨兵时，以 `guard` 重入本组；`contracts`/`component`/`boundary`/`e2e` 四个入口接上。
  重入而非在内层加前缀，是因为 `require_scene_command` 明确拒绝普通 Go 夹具采用场景 namespace，
  这些组不可能在自己的进程里确权前缀。
- `doc/engineering/testing.md` 在“显式 endpoint 保留既有绑定行为”之后补充清理责任：有前缀的组由
  `run` 领取前缀并只删前缀，无前缀的组经 `guard` 取快照并只删新增；清理失败或新增存活都算该组失败；
  留不下清理能力时应要求专属实例。

本轮修掉的一个真实缺陷（本债实现过程中的自伤）：初版接线写成
`if guard_shared_search contracts; then return; fi`，两处错误同时存在——脚本顶层 `return` 在 bash 里
是语法错误（`can only return from a function or sourced script`，实测退出 1），而失败状态在 `if`
条件上下文里被 `set -e` 放过，于是“守卫跑完”与“组失败”都被读成成功。改为重入本组后，组的退出码
就是脚本退出码，无顶层 `return`。

第二个缺陷更要紧，靠真实运行才暴露：`run_group_on_shared_search contracts run_component run_boundary run_e2e`
在 bash 里只把 `run_component` 当作命令执行，`run_boundary`/`run_e2e` 成了它的位置参数被安静丢弃。
表现是 `contracts` 组退出 0、72 个包 `ok`，而 `internal/arch` 与 e2e 覆盖行完全没有出现——三分之二
没跑却显示全绿。修复方式是把三段收进 `run_contracts_steps` 单个函数后再传入，并把步骤数当成可观察
资产：回归 `test_contracts_group_runs_every_step_not_only_the_first` 用计数型伪 `go` 记录每次调用，
断言恰好三次、且分别出现 `internal/arch` 与 `./cli`。

离线回归 13 条全绿（`python3 -m unittest scripts.opensearch_fixture_test`），其中新增两条覆盖本债入口：
`test_shared_instance_group_is_guarded_and_keeps_its_failure_status`（子组退出 73 必须原样透传，且
不出现顶层 return 错误、新增索引被逐个删除）、`test_shared_instance_group_reports_success_when_the_group_passes`。

真实实例已验证全部四组，运行前后容量均为 `{"openShards":426,"maximumShards":1000,"freeShards":574}`：

- `boundary`：退出 0，31s，不创建索引。
- `component`：退出 0，全部包通过。
- `e2e`：退出 0，`end-to-end public command coverage: 53/53 successful`。
- `contracts`：三段全部执行（75 个包 `ok`、`internal/arch` 通过、e2e 覆盖行 53/53），报告 `executionStatus: passed`、
  `exitCode: 0`。

其中 `contracts` 那次的外层退出码是 1，原因是 `sourceChanged`——我在它运行期间编辑了任务文件，
`scripts/validation.py` 会把“运行期源码变化”判为 `source-changed` 并置 1。这是操作噪音，不是测试失败；
报告里的 `exitCode: 0` 与 `executionStatus: passed` 是该次运行的真实结果。

离线回归 14 条全绿（`python3 -m unittest scripts.opensearch_fixture_test`），覆盖本债入口的是三条：
子组失败时退出码原样透传、子组通过时新增索引被逐个删除、`contracts` 必须执行全部三段。

另需单独裁决的既有残留：实例上 20:33–20:59 那批 412 个 `kc-proj-<12hex>-g-<hex>` 索引仍无归属结论，
本债“不做”条款已明确不碰它们。

本轮补到一条关键事实，它把归属问题从推测变成观测：在独占的自动实例
（`kc-local-opensearch-8866dea7622f`）上运行 `go test -short -count=3 ./cli`，实例内出现 327 个
`kc-proj-<hex>-g-<hex>` 索引，形状与共享实例上的残留完全一致。也就是说，`./cli` 整包里的普通测试
自己就会批量创建投影索引，且在包里没有对应的整包级清理——不是某一次契约运行写坏，而是每次整包
运行都如此。这解释了残留为何集中在一次 `make test-contracts` 的窗口内（`contracts` 含 e2e 段，
e2e 就是 `./cli` 整包），也说明组入口的守卫是当前唯一有效的拦截点：包内不清理，只能由组边界回收。
按本债“不做”条款，残留本身仍留给单独的归属裁决；这条观测只用于说明拦截点选在入口是对的。
