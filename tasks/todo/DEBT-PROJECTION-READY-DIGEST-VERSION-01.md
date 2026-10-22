# DEBT-PROJECTION-READY-DIGEST-VERSION-01 · 就绪快速路径信任既有访问摘要，摘要算法变更时不会重建

## 债

- 位置：`declarativeindex/projection/spec.go` 的 `readyDigestAt`——READY 判定改为只读元数据后，
  它在 `meta.Basis == commit` 且 `physicalMatches` 成立时直接返回 `meta.AccessDigest`，不再重算
  `AccessSpec`。`physicalMatches` 覆盖 `PhysicalDigest`（含 manifest 摘要）与 `ProviderRevision`，
  但不覆盖 `discovery` 里 contract 摘要本身的构造方式（`finalizeAdoptedAccessSpec` 的
  `kernel.CanonicalDigest` 输入形状）。
- 信号：若将来改变 `AccessSpec.AccessDigest` 的组成（例如有选择地引入 `adoptions` 之外的字段、改变
  `declarationContracts` 的裁剪方式、或调整 `schemaForms` 的键），既有 READY 投影的旧摘要仍会与
  `physicalMatches` 相符，`readyDigestAt` 便把它当成本实例当前合同，从而不触发重建。变更前后同一
  commit 的投影会给出不同的检索面，而系统认为两者等价。当前 `Meta`（`discovery/projection/engine.go:62`）
  没有任何合同版本字段可用于区分。
- 判定：这是就绪快速路径换来的确定代价，不是实现疏漏。原路径每次重算摘要，天生对摘要算法变更免疫；
  但它为回答「是否已 READY」付出每条引用一次地址解析（`base-mini` 上 87056 次），使发布与定义被
  全局锁串死。两者不可同时成立：要么每次付全扫，要么信任既有摘要。选定信任，并把版本守卫登记为债。
  注意此债不动摇 READY 的含义——`ProtectDatasetResource` 仍独立核验完整固定声明见证
  （`dataset_resource.go:34` → `validateDatasetInterpretation`），本债只影响「是否需要重建」的判断。
- 处置：登记为债，改动摘要构造时按触发条件处理。可行方向二选一——引入随投影持久化的合同版本，
  在 `readyDigestAt` 额外比对；或把摘要构造方式纳入 `manifestDigest`/`PhysicalDigest` 的组成，
  使算法变更自动表现为物理配置变更而触发重建。后者复用既有 `physicalMatches`，无需新字段，优先考虑。
- 触发条件：任何改变 `finalizeAdoptedAccessSpec` 摘要输入或 `declarationContracts` 形状的改动，
  同一次提交内必须一并处理本债；不得只改摘要算法而留下无法重建的旧投影。

## 目标

就绪快速路径在合同摘要构造方式变更时不会把旧投影误判为当前合同：要么自动触发重建，要么在读取时
明确拒绝，而不是静默返回按旧算法得出的检索面。

## 依据

- [声明与访问合同](../../discovery/adoption.go)（`finalizeAdoptedAccessSpec`、`AdoptionWitnesses`）
- [投影就绪语义](../../declarativeindex/projection/README.md)（`CheckSearchProjectionAt` 的 READY/basis/digest 前置条件）
- [工程质量闭环](../../doc/engineering/quality.md)

## 验收

- 构造一个 READY 投影后改变摘要构造输入，再调用 `EnsureAt`，断言其进入重建而不是回答 READY。
- 不改变现有 READY 判定在「物理配置一致」时的零地址解析性质：`TestEnsureAtOnReadyBasisDoesNotWalkTheReferrerIndex`
  与 `TestDatasetPrepareReusesReadyRevisionWithoutReferrerWalk` 必须保持通过。

## 前置

无。

## 不做

- 不回退到每次重算摘要来规避本债；那会重新引入已修复的全扫占锁问题。
- 不把 READY 当作声明见证：`ProtectDatasetResource` 的完整见证核验不得因本债被削弱。
- 不为此新增 skip 或放宽既有 conformance 断言。
