# AUTH-LAYOUT-01 · 服务端认证收进 identity/

## 目标

服务端认证是一个独立能力，全部在 `identity/` 下维护，依赖单向：`server/` 与 `snapshot/service/` 只消费它，
它不依赖二者，也不依赖 `invocation/`、`invoke/`、`home/`、`client/`、`cli/`。

做完后的形状：

| 位置 | 职责 |
|---|---|
| `identity/` | 已认证身份与认证器接口、认证器注册表、唯一的请求认证判定、身份绑定（`Bind`、`WrapAuthenticator`）、校验响应辅助 |
| `identity/local/` | local 认证器：只认 `X-Kc-As`，显式选择才生效，是一个普通认证器而不是“没有认证器”的特例 |
| `identity/taihu/` | Taihu 认证器（网关签名与 introspection）、服务应用身份、换 token 与浏览器授权中转的业务逻辑 |
| `identity/identitytest/` | 测试共用的认证器替身，各包测试不再各写一份 |

`server/` 只保留装配和 HTTP 路由（`/identity/v1/*` 的处理器调用 `identity/taihu`），不再有 `auth_*.go` 转调文件；
`snapshot/service/` 调用同一个判定入口。按认证方式名字分支（`mode == "taihu"` 一类）只允许出现在
`identity/` 内；控制台请求附加检查由调用方作为选项传入，不再写成按名字的特例。`invocation/` 不再保留
`HTTPIdentity`、`HTTPAuthenticator` 和注册表的别名。

认证可替换：新增或替换一种认证方式只需在 `identity/` 下加一个子包并注册，`server/`、`snapshot/service/`
和授权代码不改。认证器只回答“这个请求是谁”，不决定 KC 怎样访问存储。

lakeFS 只作存储，不作身份来源：`--auth lakefs`（拿用户自己的 lakeFS 会话或密钥问 lakeFS `/api/v1/user`）
退役删除。KC 访问 lakeFS（Snapshot adapter 与控制台原生 API 代理）一律用部署的服务私有凭证，在 KC 完成认证与
授权之后发出；用户凭证不转发给 lakeFS，lakeFS 账号、凭证、策略接口继续拒绝转发。原生代理里按
`mode == "lakefs"`、`Provider == "lakefs"` 转发调用者证明的分支随之删除。浏览器 Cookie 请求的同源检查
（`X-Kc-Console`、Fetch Metadata、修改要求 JSON）与认证方式无关，保留为服务对浏览器请求的通用检查。

## 依据

- [权限](../../doc/design/permissions.md)：身份由 Server 认证；“实现位置”一段需同步改成 `identity/`。
- [服务](../../doc/design/service.md)：认证方式只有 local 与 Taihu，替换认证器不改授权语义；模块表同步。
- [架构](../../doc/architecture.md)：服务层目录职责同步。
- [架构不变量](../../doc/engineering/architecture-invariants.md)：新增守卫登记。

## 验收

- 分层守卫：`internal/arch/layers_test.go` 增加 `identity`（含子包）禁止依赖上述包的规则，故意违反时变红。
- 新守卫：生产代码中实现认证器接口的类型只在 `identity/` 下；`identity/` 外没有按认证方式名字的比较。故意违反时变红。
- 退役路径守卫列入 `server/auth.go`、`server/auth_taihu.go`、`server/auth_service.go`、`server/auth_verifier.go`、`server/auth_binding.go`、`invocation/auth_registry.go`。
- 现有认证与配对用例（local 配对错配、Taihu 网关/introspection、身份绑定、控制台请求检查）随代码搬到 `identity/` 后仍全绿，断言不削弱；`TestLiveTaihuAuthentication` 搬家后 `make test-taihu-live` 入口和 `test-catalog.md` 的 P-20 登记同步。
- 可替换性证据：用 `identity/identitytest` 注册一个测试认证器，不改 `server/`、`snapshot/service/` 即可跑通认证、授权与控制台原生代理；守卫禁止 `identity/` 外出现具体认证器包的 import（装配根除外）。
- lakeFS 认证退役：`--auth lakefs` 返回未知认证方式；`snapshot/service/auth_lakefs.go` 的认证器部分、`server/auth_lakefs.go`、`cli` 的 lakefs 登录提示删除并列入退役路径守卫；原生代理只用服务凭证，反例“用户自带 lakeFS 证明被转发给 lakeFS”变红即失败。相关 lakefs 模式用例直接删除，不改成 skip，`test-catalog.md` 同步；`server/README.md`、`home/README.md`、`snapshot/service/README.md`、控制台 `KC-VENDOR.md`、`CONSOLE-VALIDATION.md` 里“遗留 `auth: lakefs`”的说法改掉。
- 其余 `serve --auth` 可选值、`GET /identity/v1/auth` 的配对返回和错误文案不变。
- 三篇设计与 `identity/README.md` 同步；`make check-docs`、`make quality`、`make test`、`make test-contracts` 通过。

## 前置

`GITEA-LOGIN-RETIRE-01`：先删 Gitea 认证，避免搬完再删。`DEBT-EMPTY-PRINCIPAL-01` 的触发条件正是触碰认证装配，
建议紧接其后做，或在本任务的统一判定入口里一并收口。

## 不做

- 不改授权（`snapshot/access/`、`allow.json`）与身份绑定语义，不新增登录方式。
- 客户端登录与会话（`cli/remote_login.go`、`cli/remote_session.go` 的 PKCE、浏览器流程、会话文件）归
  `ARCH-LAYOUT-01` 收进 `client/`：会话属于调用方，不进服务端认证目录。
- 不做 lakeFS 用户与 KC 用户的映射或同步，不按 KC 用户给 lakeFS 签发凭证；lakeFS 侧只有 KC 的服务账号。

## 交付记录（2026-10-15）

实现范围：

- `identity/` 根包：认证器注册表（`Register`/`Lookup`/`Names`/`Resolve`）、唯一请求判定 `Authenticate`、
  `BrowserRequestCheck`、`LoginBroker` 端口；`identity/local`、`identity/taihu`（认证器、服务应用身份、换 token 与
  浏览器授权中转）、`identity/builtin`（装配链接）、`identity/identitytest`（共享替身）。
- `server/` 删除 `auth.go`、`auth_taihu.go`、`auth_service.go`、`auth_binding.go`、`auth_lakefs.go`；`/identity/v1/token|authorize`
  处理器只解码并调用 `LoginBroker`。`invocation/auth_registry.go` 与别名删除。`snapshot/service/` 改调同一判定入口。
- lakeFS 认证退役：`--auth lakefs` / deployment `auth: lakefs` 报未知方式；原生代理只用服务凭证，Authorization 与
  lakeFS Cookie 均不转发，账号类请求本地应答或 403；控制台删除 lakefs Cookie 分支。
- 守卫：`identity/builtin`、`identity/identitytest` 分层规则（共用 `authenticatorDenied`）；`TestAuthenticatorsAreConfinedToIdentity`
  （`identity/` 外不实现认证器、`server/` 与 `snapshot/service/` 不比较认证方式名字、装配根外不 import 具体认证器）及其反例
  `TestAuthenticatorGuardRejectsLeaks`；`TestAuthenticatorDoublesComeFromIdentitytest`（测试不再自写替身，原 11 份已迁移）；退役路径。
- 可替换性证据：`TestRegisteredAuthenticatorReplacesProviderWithoutServerChange` 用 `identitytest.Register` 注册新名字，
  经 `--auth` 选用，跑通配对发现、whoami、授权与原生代理，未改 `server/`、`snapshot/service/`。
- 文档：permissions、service、architecture、architecture-invariants（新增 AUTHN-01）与 identity/server/home/cli/snapshot service/console
  README、控制台 KC-VENDOR、CONSOLE-VALIDATION、ACCESS-CONTROL-API-REVIEW 同步。`scripts/coupling/baseline.json` 为 4 个新单元收紧
  （传播率 15.14% → 14.26%）。

偏差与裁决：

- `TestLiveTaihuAuthentication` 与用户名用例依赖 Server handler，`identity/taihu` 不能 import `server/`，故留在 `server/`；
  `make test-taihu-live` 与 test-catalog P-20 无需改动。
- 守卫的名字比较集合不含 `lakefs`：它仍是存储 driver 名（`Registry.Driver == "lakefs"`），按设计不是认证分支；
  `--auth lakefs` 的复活由“具体认证器只在 `identity/` 子包”约束。`server/serve_auth.go` 作为 `--auth local` 默认测试接缝列入装配根。
- 前端仍按 `'taihu'` 名字判断共享登录，登记为 `DEBT-CONSOLE-AUTH-NAME-01`。`DEBT-EMPTY-PRINCIPAL-01` 未在本任务处理：
  统一判定已拒绝空 principal，求值函数自身的防线仍按原触发条件保留在 `todo/`。

已验证范围：

- `go build ./...`；`go test ./...` 失败集与基线 2f0993d 完全一致（141 条，均为基线既有：`discovery/datasets`、`home` 测试编译错误，
  `internal/arch` 术语守卫等），本任务无新增失败；`home` 在临时修补基线编译错误后与基线失败集一致，新用例通过。
- `make check-docs` 通过；限定范围质量门（gofmt/vet/staticcheck/复杂度/体积/克隆/耦合棘轮）通过，新子包另行 vet/staticcheck。
- 控制台 `vitest`：191/193，2 条失败与基线相同（`nativeBaseProxy.test.ts`）。
- 未达成：`make test`、`make test-contracts` 因基线本身红（编译错误）无法全绿，本任务留在 `doing/`，待基线修复后复跑再移 `done/`。

合并回主工作区（2026-10-15，与并发改动三方合并，未提交）：

- 冲突裁决：`cli/service_roles_live_test.go` 按 GITEA-LOGIN-RETIRE-01 保持删除；architecture-invariants 取主工作区 D-01 行并在其前插入 AUTHN-01。
  `liveaccess/plugin/server_read_timeout_test.go`（并发新增）改用 `acceptanceAuthenticator` 变量。
- `go build ./...`、`go vet ./...` 干净；`identity/...`、`snapshot/service`、`snapshot/client`、`liveaccess/plugin` 通过；
  `internal/arch` 认证、分层、能力、退役守卫通过（仅余术语守卫，属并发零知识试跑）；控制台 `vitest` 193/193。
- 合并后的其余失败经逐项核对与认证无关：Schema access 退役（`MATCH needs text access`、`SCHEMA_UNSUPPORTED`）、
  缺 `KC_TEST_OPENSEARCH_URL`、本机端口耗尽（复测通过）；`make check-docs` 红于 `tasks/doing/ZK-TRIAL-01.md` 断链，
  耦合棘轮红于 `declarativeindex/cli` 漂移，均来自并发改动，不在本任务范围。

## 接管

判定时间：2026-10-15 20:33:29（Asia/Shanghai）。最后一次观察到进展：20:13:01，任务交付记录更新。
至判定连续20分28秒未见新信号：任务文件mtime未再次更新；明确认证范围内没有非本轮的新改动；
HEAD仍为 `94383e50d1c5f8e99075acd3d6b02fabeef8819c`、无新提交；`ps`没有非本轮的相关测试或构建进程。
观察和排除清单在 `.validation/runs/20261015T120241Z-task-orchestration/`。接管前任务全文、在途diff与工作区状态
保存于 `.validation/runs/20261015T123329Z-auth-takeover/`；已读包含合并回主工作区的全部交付记录，从当前实现继续，未回退、未重写。
目标与实际依据：按权限、服务及架构篇核对认证收归identity和lakeFS只作存储的边界；设计未改，先跑现有认证相关回归作为接管基线。
GITEA-LOGIN-RETIRE-01仍待统一门禁，本任务先核对既有交付，不提前扩展认证行为。未提交。

本轮稳定候选门禁：`make check-docs`通过；`make quality`在 `declarativeindex/plugin/plugin_control_recovery_fixture_test.go` 的gofmt检查失败，归仍在途的 `INDEX-DECLARATION-01`，未替其它线格式化。证据 `.validation/runs/20261015T123258Z-42a42700fe5d/report.json`，`sourceChanged=false`。本任务继续doing；尚未达到统一门禁验收。

接管后认证基线：显式选择53个顶层用例，121个通过事件，5包全绿、无fail/skip且源码指纹稳定；证据 `.validation/runs/20261015T123517Z-6d1f892df022/report.json`，具体选择在 `.validation/runs/20261015T123329Z-auth-takeover/selection.json`。实现未改；真实Taihu环境用例未选择，不以此替代真实登录或完整合同。

## 交付

实现范围：具体认证器收进 `identity/`（`local`、`taihu`、`builtin`、测试用 `identitytest`），`server/`、`snapshot/service/` 只经注册表按名字取用；`--auth lakefs` 退役，lakeFS 只作存储，原生代理只用服务私有凭证；11 份测试替身统一到 `identitytest`；新增 AUTHN-01 守卫及反例；设计篇、包 README、控制台会话判断同步。

已验证范围（2026-10-15 21:2x，主工作区、`-count=1`）：

- `go build ./...`、`go vet ./...` 干净；本任务触碰的 Go 文件 gofmt 干净；`identity/...`、`server`、`snapshot/service`、`snapshot/client`、`liveaccess/plugin`、`home`、`cli`、`internal/arch` 的 staticcheck v0.6.1 无告警。
- `identity/...`、`snapshot/client`、`liveaccess/plugin` 全绿；`internal/arch` 认证、退役路径守卫全绿；`server`、`snapshot/service`、`home`、`cli` 的认证/登录/会话/原生代理/部署定向用例全绿，可替换性用例通过；控制台 `vitest` 193/193；`make check-docs` 通过。
- 无 skip，未削弱断言。

未全绿的统一门禁及归属（均不在本任务改动内，按用户 2026-10-15 明确指示移入 done）：

- `make quality`：`home/index_trial.go` 新文件空白检查，归在途索引试跑。
- `go test ./...`：Inventory/Dataset 消费（`DEBT-INVENTORY-CONSUMPTION-01`）、Dataset 根配方（`DEBT-DATASET-ROOT-RECIPE-01`）、Schema access 退役（`INDEX-DECLARATION-01`）、HTTP 路由数 89→91、零知识术语（`ZK-TRIAL-01`、`DEBT-TERMINOLOGY-TRIAL-01`）、缺 `KC_TEST_OPENSEARCH_URL`。
- `make test-contracts`、`make deploy-local-scenes` 未在本任务运行。

遗留：`DEBT-CONSOLE-AUTH-NAME-01`（前端按 `'taihu'` 名字判断）；`DEBT-EMPTY-PRINCIPAL-01` 保持原触发条件。`DEBT-DATASET-ROOT-RECIPE-01` 等待的 `server/snapshot_service.go` 归属随本任务完成解除。

提交：未提交。
