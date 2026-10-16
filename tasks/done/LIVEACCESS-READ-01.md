# LIVEACCESS-READ-01 · 外部取值读取：逐值失败、请求时限、精确可信范围

## 目标

读取时外部值在原位置出现，观察依据与失败按地址列在值旁。一次读取的外部取值共用请求剩余时限，
用尽后其余单元逐个报超时。凭证只发往 scheme、host、port 都精确匹配的可信 origin，重定向不扩大范围。

## 依据

- [LiveAccess](../../doc/design/liveaccess.md)：规则、U21、U23。
- [权限](../../doc/design/permissions.md)：外部状态有两次授权。

## 验收

- [accept-03-live-read](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-03-live-read.yaml)、[accept-04-live-trust](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-04-live-trust.yaml) 全绿。
- [accept-13-cross-cutting](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-13-cross-cutting.yaml) 中 `accept-cross-request-deadline`、`accept-cross-origin-exact-match`、
  `accept-cross-per-caller-not-searchable` 的读取部分变绿。
- `make check-docs`、`make quality`、`make test` 通过。

## 前置

`LIVEACCESS-DECLARATION-01`。

## 不做

不保存动态状态的全部历史。

## 交付（2026-10-15）

前置 `ACCEPT-HARNESS-01` 按用户指示暂缓，`accept-*` 规格没有执行器；可在 Go 里证伪的条目已落成测试，其余等执行器（见下）。

实现范围：

- 可信范围精确匹配：`liveaccess/http_policy.go` 新增 `canonicalOrigin`，按 scheme、host、port 归一（http 补 80、https 补 443，host 大小写归一），`credentialOrigin` 与重定向 `validTarget` 都走它。精确匹配由 `TestHTTPPolicyTrustMatchesSchemeHostAndPortExactly` 证伪（缺省端口、大小写、alt-port、异 scheme、子域名、父域、尾点）。
- 范围外无凭证外呼：`perform` 删除「有凭证且目标不可信就报 `ErrForbidden`」分支，不可信时跳过 `applyCallerCredentials` 照常外呼，只带主体与关联。`TestHTTPPolicyUsesApplicableTargetAndOriginChangeCannotCarryProof`、`liveaccess/plugin/server_auth_boundary_test.go`（`assertProofFree`，`otherCalls` 被真实调用且无凭证）证伪。
- 请求共用时限：新增 `liveaccess/read_limits.go`——`ReadLimits{Lookup, Read}`、`ContextWithReadLimits`、`UnitLookupContext`（取单元时限与剩余时限较小者，预算用尽报超时）、`LookupTimedOut`。`service/hydrate.go` 在每单元 LookupState 前取时限并在超时时报 `ErrTemporaryUnavailable`。证伪：`service/read_limits_test.go`（3 单元共享 300ms、单元限只拖垮自己、无限额时传输取消仍致命、非正时限被配置拒绝）、`plugin/server_read_timeout_test.go`（部署 lookupTimeout 5s/readTimeout 250ms，共享超时）。
- 源错误逐单元保留：`state_lookup.go` 合并为 `runtimeHTTPError`，401/403→Forbidden、源可识别码保留、其余 4xx→CapabilityUnsatisfied（429/408 除外）、其他→TemporaryUnavailable。`TestHTTPStateLookupKeepsSourceFailureCode`（404 带/不带码、429、502、403）、`TestHTTPStateLookupUnreachableOriginIsTemporaryFailure`（不可达为失败不是 null）证伪。
- 配置：`config.go` 加 `LookupTimeout`/`ReadTimeout`（json/yaml `lookupTimeout`/`readTimeout`）与 `Limits()`，`Validate` 调用它；`server/serve_auth.go`/`serve_facade.go`/`service_invoke.go` 把 `ReadLimits` 从部署配置传入请求 ctx。

裁决（设计优先，依 `liveaccess.md` U7/U21/U23 与授权）：

- 不可信目标由「报 `ErrForbidden` 且不外呼」改为「无凭证照常外呼」。原 `server_auth_boundary_test.go` 的 Forbidden 断言与设计「取值描述写了未信任 origin 也照常不带凭证外呼」冲突，按设计改写为更严断言（确实被调用、不带任何调用者证明、值出现在原地址）。
- 共享时限用 context value（`readBudget`）记录，不设成请求 ctx 的 deadline，避免截断快照读取与证据留存。
- 两处 runtime 错误映射合并为一个 `runtimeHTTPError(subject,...)`，行为等价且集中。

文档：`liveaccess/README.md`（精确匹配、无凭证外呼、源错误码映射、lookupTimeout/readTimeout 语义）、`home/README.md` 与 `server/README.md`（新配置键与 `LiveAccessReadLimits`）、`doc/design/liveaccess.md` 进度表（共用请求时限、origin 精确匹配、404/不可达逐值失败移入「已有」）、`test-catalog` B-25 更新并新增 B-28（U21/U23）。

已验证范围：`make check-docs`、`make quality` 通过（gofmt、耦合基线 15.14% 不变）；`liveaccess/...`、`server`、`home`、`invocation`、`internal/arch`、`cli` 中不依赖 OpenSearch 的用例通过。

未验证 / 边界：

- `accept-03`、`accept-04`、`accept-13` 规格仍无执行器（`ACCEPT-HARNESS-01` 暂缓），上述 Go 测试覆盖其可在进程内证伪的部分，`accept-cross-hit-recheck` 等需索引回读的留待执行器。
- 本机无可用 OpenSearch，`liveaccess/discovery`、`server`/`home`/`cli` 中依赖 `KC_TEST_OPENSEARCH_URL` 的用例未验证。
- `internal/arch` 的 `TestObjectIsTheOnlyNameForCatalogContent` 失败来自并行任务 `ZK-TRIAL-01` 的 `zero-knowledge` 夹具命名，不属本条；未碰 `FILE-ACCESS-01`、`ZK-TRIAL-01` 的文件。
- 未提交（按约定）。
