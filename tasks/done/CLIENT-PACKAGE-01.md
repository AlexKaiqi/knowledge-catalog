# CLIENT-PACKAGE-01 · 本地构建与客户端安装包

## 目标

从当前源码构建现有 `kc`，生成 macOS 与 Linux 的 arm64/amd64 可安装包。使用者安装后无需
Go 工具链即可连接 KC Server；构建不发布、不启动服务。

## 依据

- [服务](../../doc/design/service.md)：业务 CLI 经 Server，部署与客户端状态各自维护。
- [CLI 交互](../../doc/design/affordance.md)：命令入口与副作用清楚。
- [测试与验证](../../doc/engineering/testing.md)、[工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- `make build` 生成单目标二进制，`make package` 生成单目标安装包，`make package-all` 覆盖四个目标。
- macOS `.pkg` 的安装路径、固定包身份与版本正确；Linux 压缩包提供安装、升级和卸载，保留本机配置与会话。
- 目标错误、非法版本、构建或打包失败明确退出；各目标构建与打包全部完成前不替换已有产物与校验清单。
- 包内记录源码 revision、工作区修改状态、目标与 Go 工具链；包外 SHA-256 可核对产物。
- 安装回归与打包回归、实际四目标构建及本机包内容检查通过；`make check-validation`、
  `make check-docs`、`make quality` 通过。交叉编译与非本机运行分别记录。

## 前置

无。

## 不做

不新增客户端协议或业务命令，不拆分客户端与服务端二进制，不包含 `kcfs`；不签名、公证或发布，
不安装新依赖，不向宿主机系统目录安装验收包。

## 交付

- 本地入口：`make build`、`make package`、`make package-all`；目标、数字版本及输出目录可显式选择。
  实现位于 `scripts/package/`，安装说明链接自根 README 与 `cmd/kc/README.md`。
- 使用已有 Go 1.23.6 构建开发版本 `0.0.0` 的四个安装包，产物与校验表在忽略目录 `dist/`。
  未新增依赖、未启动部署、未发布、未提交。
- `make test-package` 18 项回归全过，无 skip：目标与版本拒绝、跨平台输出、包内容与 SHA-256、
  后续目标失败保留已有包、安装与升级、卸载保留配置、平台拒绝、Rosetta 默认架构及严格 umask 下的包权限。
- 最终 `make check-docs`、`make quality` 通过；`make check-validation` 通过，其打包目标在新增最后一项
  权限回归后单独重跑到绿。证据：验证接线（`.validation/client-package-gates.log`）、
  最终专项与工程门禁（`.validation/client-package-final-gates.log`）。
- 实际构建（`.validation/client-package-build.log`）与最终产物检查（`.validation/client-package-smoke.json`）：
  四包校验及二进制摘要一致；macOS 两包的 payload、固定包身份/版本及 Installer 读取通过，arm64 包内
  二进制本机 `help` 通过；Linux arm64 在已有缓存 Alpine 容器内完成安装、运行、覆盖安装和卸载，保留会话，
  Linux amd64 的 `help` 在已有模拟环境运行通过。未向宿主机系统目录安装 `.pkg`，未作 Intel 原生安装验收。
- 自检修复了 Rosetta 默认目标与安装前架构判定不一致、严格 umask 影响 macOS 包目录权限两项增量问题，
  均已有回归；无新增协议形状或旁路客户端入口。

## 完整合同的未闭合项

- `make test` 已通过，使用已有显式 OpenSearch endpoint；未启动或改造部署。
- 首次 `make test-contracts` 在自动搜索夹具离线生命周期测试的就绪/清理等待上超时；同源孤立复测及
  第二次完整合同中的该测试均通过，但不据此宣布根因已修复。新债 `DEBT-TEST-OPENSEARCH-TIMING-01`
  已登记并立即并入现有 `ACCEPT-HARNESS-01`，保留首次日志（`.validation/client-package-contracts.log`）。
- 第二次完整合同在 `TestDatasetUILoadsBuildHintWithoutExposingAuthority` 失败：Go 1.23.6 的默认
  `/ui` 补斜杠跳转为 301，断言期待 307。该已知差异由 `ROUTE-MANIFEST-01` 并入的
  `DEBT-UI-REDIRECT-STATUS-01` 覆盖；本轮不改路由或削弱断言。
  完整复跑（`.validation/client-package-contracts-retry.log`）与
  当前 Go 工具链专项（`.validation/client-package-ui-go126.log`）分别保留。
  当前 Go 1.26 的该专项通过；最低 Go 失败确认后，停止本轮补充全量运行中剩余的 CLI 测试及其子进程，
  不把这次部分执行计为完整合同通过。
- 通用合同中按独立入口显式启用的外部 live 测试不计为本轮打包专项验收；没有新增 skip。
  由于完整合同尚未全绿，任务保持 doing；本地安装包已交付不等于完整协议或部署验收通过。

## 进展（2026-10-16）

- `DEBT-UI-REDIRECT-STATUS-01` 已由 `ROUTE-MANIFEST-01` 收口（提交 5e280349）：`GET /ui` 显式注册并固定 307，Go 1.23.6 与当前工具链各验证通过，断言未放宽。本任务的该已知阻塞解除，见 `done/ROUTE-MANIFEST-01.md`。
- 剩余闭环：一次全绿的 `make test-contracts`（含 `DEBT-TEST-OPENSEARCH-TIMING-01`，已并入 `ACCEPT-HARNESS-01`）。当前多条并行线在途改动工作区源码且共享 OpenSearch 需独占，按并行编排规则（源码稳定时跑全量门禁），待并行线收束后单独执行，不与在途改动叠加。

## 收口（2026-10-19）

- 主工作区并行线在途改动未收束，按既有冻结候选先例在独立工作树（`/tmp/kc-closeout-20261019`，detached
  at `4f12b4b6`）执行全量合同：`passed-with-skips`、exitCode 0、`sourceChanged=false`，
  4964 项测试通过、0 失败、14 项测试级跳过均为既有外部 live 用例（Luna / Taihu / live lakeFS /
  scale），公开命令覆盖 54/54 成功且满足风险边界；未复现 `DEBT-TEST-OPENSEARCH-TIMING-01` 的
  就绪/清理等待超时。证据已复制至主工作区 `.validation/runs/20261019T061931Z-e755e8e4fd69/`。
- `TestShapeCLIProductHostCommands` 等形状读取用例在合同中通过；`kc/cli` 包 644s 全绿。
- 打包实现与产物证据不变；本任务全部验收项闭合，移入 `done/`。
