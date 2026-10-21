# CONSOLE-RETIRE-01 · 退役 kc serve 观察台 /console 与 /repositories/{repository} 跳转页

## 目标
`kc serve` 不再提供自有浏览器前端：`GET /console`、`GET /assets/console.js`、`GET /repositories/{repository}`、
`GET /assets/repository.js` 不再挂载，`server/web/` 及其 node 测试、`make test-server-web` 与 CI `server-web`
作业一并删除。浏览器入口只剩 `/ui/`（`snapshot/console`）；投影状态、对象检索/关系/来源等运维与观察只走 `kc` CLI。
帮助、启动横幅、部署访问卡与冒烟把控制台入口改为 `/ui/`，Repository 自定义元数据指向
`/ui/` → Repositories → 目标仓 → Settings → General。平台仓 `managementURL` 由 `{publicURL}/repositories/{name}`
改为 `{publicURL}/ui/repositories/{name}`，直接落到 `/ui/` 原仓页。

## 依据
- [服务](../../doc/design/service.md)：HTTP 面只保留 typed API 与 `/ui/` 挂载。
- [Snapshot](../../doc/design/snapshot.md)：`snapshot/console` 是唯一浏览器控制台。
- 用户决定（2026-10-19）：`/console` 无独立必要，`/repositories/{repository}` 跳转页一起删，观察只走 CLI；开发阶段不保留旧入口。

## 验收
- `server/routes_golden.txt`、`server/routes.go` 与 HTTP 合同库存/覆盖测试不再含上述四条路由，路由金样双向对账通过。
- `kc serve --help`、`kc show/create --help`、`kc help metadata` 指向 `/ui/`，对应帮助测试断言新入口。
- `make check-docs`、`make quality` 通过；`go test ./server ./cli ./snapshot/... ./internal/arch` 通过，无 skip。

## 前置
无。

## 不做
- 不改 `snapshot/console`（`/ui/`）前端与 handler。
- 不把 `/console` 的投影/对象观察功能移植到 `/ui/`。

## 交付
实现范围：删除 `server/console_page.go`、`server/repository_page.go`、`server/static_pages.go`、`server/web/` 全部资产与
node 测试、`cli/console_page_internal_test.go`、`server/repository_page_internal_test.go`；`server/service_repositories.go`
去掉四条挂载，`server/routes.go`、`routes_golden.txt`、`serve_telemetry.go` 同步；Makefile `test-server-web` 与 CI
`server-web` 作业删除；`home/authority_drivers.go` 管理地址改为 `/ui/repositories/{name}`；帮助、`kc serve` 横幅、
`scripts/deploy/{deploy.sh,smoke.sh,README.md}`、`scripts/kc-snapshot-plane.sh`、根 README、
`doc/engineering/test-catalog.md`、`snapshot/console/README.md` 指向 `/ui/`。路由计数断言随下线面更新：
公开路由 91→87、直连证据 53→49、validation-inventory server 62→58/总 87；断言语义不变。

已验证范围：主工作区存在其它进行中任务的未提交改动（其 `snapshot/internal` 使耦合门禁漂移），故在 HEAD 干净副本
仅叠加本任务改动后执行：`make check-docs`、`make quality` 通过；`make test-contracts` 0 fail，14 个 skip 均为
环境门控的 live/规模用例（如 `TestLiveTaihuAuthentication` 需 `KC_LIVE_TAIHU=1`，`TestSceneAccessorRefreshUpdatesSearchIndex`
需 accessor compose），与本改动无关、不由本任务引入；`make test` 0 fail 0 skip。日志：
`.validation/console-retire/20261019/`。未做真实部署验收（`make deploy-local-scenes`），部署访问卡与冒烟改动未在真实栈上执行。
提交：未提交。

收口复验（2026-10-19，主工作区全部在途改动的冻结副本，源码稳定后执行）：`make check-docs` PASS；`make quality` 退出 0
（耦合棘轮 held，propagation 14.31% 与基线持平）；`make test` 退出 0，无 FAIL、无 SKIP；`make test-contracts` 退出 0，
`kc/server`、`kc/cli`、`kc/snapshot/...`、`kc/internal/arch` 全部 ok。合同组合里 14 个 SKIP 都是缺环境变量的 live/规模用例
（`KC_SCENE_LIVE_LAKEFS_URL`、`KC_LIVE_TAIHU`、`KC_LIVE_LLM_RERANK`、`make test-state-runtime-e2e` 等），与本任务无关。
日志：`.validation/remaining-20261019/{check-docs,quality,test,test-contracts}.log`。真实部署验收仍未做。
