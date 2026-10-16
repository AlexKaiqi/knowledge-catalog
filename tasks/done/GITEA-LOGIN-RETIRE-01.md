# GITEA-LOGIN-RETIRE-01 · 移除 Gitea 登录与 /api/v1/ 代理

## 目标

身份只走 KC 自身：Gitea 登录认证与 `/api/v1/` 代理路由全部删除，部署和文档里不再出现 Gitea。

## 依据

- [权限](../../doc/design/permissions.md)、[服务](../../doc/design/service.md)。
- 原 `REFACTOR-OBJECT-MODEL-01` 阶段 2b（2026-10-15 用户确认不再需要）。

## 验收

- 代码、配置、部署清单中没有 Gitea 登录与代理路由；退役路径守卫把 `gitea` 列入并在故意违反时变红。
- 根 `README.md` 共享服务示例里的 `gitea:<user-id>` 主体、`object/README.md` 里“空 LakeFS/Gitea Snapshot”的说法同步改掉（原 `CLI-OWNERSHIP-CLEANUP-01` 登记的资料漂移）。
- 删掉的 Gitea 场景用例不改成 skip，直接删除并在 `test-catalog.md` 同步。
- `make check-docs`、`make quality`、`make test` 通过。

## 前置

无。与插件重构互不依赖，可以穿插做。

## 不做

不新增登录方式。

## 交付记录（2026-10-15）

- 范围裁决：标题中的 `/api/v1/` 代理指 Gitea `/api/v1/user` 校验链，已随 `server/auth_gitea.go` 删除；`snapshot/service` 的 lakeFS 原生控制台代理 `/api/v1/*` 由服务私有凭证驱动，属于现行设计，保留（其 lakeFS 用户证明转发归 AUTH-LAYOUT-01 退役）。
- 删除：Gitea 认证器、Gitea testkit、Gitea 双角色 live 旅程（`TestLiveServiceProviderConsumerJourney`、`scripts/e2e-service-roles.sh`、`make test-service-e2e`/`service-e2e` 组）；`server/auth_verifier.go` 薄壳因唯一调用方删除而成为死代码，一并删除。
- 守卫：`retiredPaths` 列入 `server/auth_gitea.go`、`server/auth_verifier.go`、`internal/testkit/gitea.go`、`scripts/e2e-service-roles.sh`，放回探针文件时 `TestRetiredPathsStayAbsent` 变红；`maintenance_test.py` 断言 `gitea`、`service-e2e` 组退出码 2 且无副作用；webui 新增"gitea 证明被拒且不恢复"反例。
- 文档：根/cli/client/home/identity/object/server/snapshot/service/testkit README、KC-VENDOR、ACCESS-CONTROL-API-REVIEW、test-catalog（删 P-13，F-01 去掉 live 旅程）、architecture-invariants A-01 已同步。`doc/concepts.md`、`snapshot/README.md` 中的 Gitea 只作退役记录保留。
- 已验证：`make check-docs` 绿；质量门除 vet/staticcheck 跳过 `kc/home`、`kc/discovery/datasets` 外全绿；`go test ./...` 失败集合是基线 141 条的子集，无新增；webui `src/lib/api` 除基线同样失败的 2 条预览路由用例外全绿；`scripts/maintenance_test.py` 绿。
- 未完成验收：`make quality`、`make test` 被基线编译错误阻断（`home/connected_plugin_fixture_test.go:152`、`discovery/datasets/*_test.go`，属于主工作区并发进行的 Dataset 准备改动，不在本任务范围），待其恢复后复跑再移到 `done/`。

## 接管

判定时间：2026-10-15 20:25:06（Asia/Shanghai）。最后一次观察到进展：20:04:48，外部 `go test ./...` 仍在运行。
至判定连续 20 分 18 秒未出现本任务新信号：

- 任务文件 mtime 保持 19:56:31，未更新。
- 本任务列明的文件没有非本轮的新改动；排除我方路径，并将索引夹具 `internal/testkit/index_declaration.go` 的变化明确归 `INDEX-DECLARATION-01`。
- HEAD 仍为 `94383e50d1c5f8e99075acd3d6b02fabeef8819c`，无新提交。
- `ps` 未发现非本轮的相关测试或构建进程。

基线与逐次观察：`.validation/runs/20261015T120241Z-task-orchestration/`；接管前任务全文、在途diff与工作区状态：
`.validation/runs/20261015T122506Z-gitea-takeover/`。已读原交付记录，从当前删除/认证改动继续，只复核与补验收；未回退或重写既有实现。
目标与实际依据：按权限、服务设计核对 Gitea 认证退役，保留现行 lakeFS 服务私有凭据代理；设计未改。
此时先跑受影响范围作为接管测试基线，后续仍须统一门禁全绿才移 done。未提交。

接管测试基线：

- 身份现有测试 25 个事件全绿，3 个无测试文件子包只作编译（报告有 package skip，test skip 为0）；`.validation/runs/20261015T122631Z-b317b57469fd/report.json`。
- 退役路径、认证配对/可替换性、私有存储凭据代理定向8项全绿、无skip；`.validation/runs/20261015T122631Z-3cd9cacb70bf/report.json`。
- 维护入口7项、控制台会话/证明/代理42项全绿；`.validation/runs/20261015T122631Z-d0b1ffba7d0d/report.json`、`.validation/runs/20261015T122631Z-3246a840538e/report.json`。
- 上述4次运行源码指纹均未变化。没有发现本任务新增实现缺陷，也未改认证实现。原文件中的认领/分支行按任务格式规则移除，原文已在接管快照保留。
- 尚不移done：INDEX-DECLARATION-01仍持续改动，完整架构、make quality、make test须在稳定候选复跑；术语守卫的精确补丁另待既有脏文件修改边界澄清，未以定向通过冒充全仓验收。

本轮稳定候选门禁：`make check-docs`通过；`make quality`在 `declarativeindex/plugin/plugin_control_recovery_fixture_test.go` 的gofmt检查失败，归仍在途的 `INDEX-DECLARATION-01`，未替其它线格式化。证据 `.validation/runs/20261015T123258Z-42a42700fe5d/report.json`，`sourceChanged=false`。本任务继续doing；尚未达到统一门禁验收。

## 交付

实现范围见上方交付记录：Gitea 认证器、testkit、双角色 live 旅程删除，退役路径守卫覆盖；作为 `AUTH-LAYOUT-01` 的前置，与其一起合回主工作区。

已验证范围：与 `AUTH-LAYOUT-01` 同一轮（2026-10-15）——`TestRetiredPathsStayAbsent` 等退役守卫、认证定向用例、控制台 `vitest` 193/193、`make check-docs` 全绿，无 skip。统一 `make quality`、`go test ./...` 仍红于其它在途任务（明细见 `AUTH-LAYOUT-01` 交付），按用户 2026-10-15 明确指示移入 done。

提交：未提交。
