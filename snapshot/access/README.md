# snapshot/access

Explicit grants for Snapshot Store services: principal, action and resource
scope. The package has no dependency on Home, Invocation, Object, Server or
an index. Actions are opaque semantic names; the matcher does not translate
object actions or infer authority from native lakeFS roles.

`Rule` and `GrantList` use one service authority. Repository and Dataset are
exact selectors; a combined Repository + Dataset scope is their intersection.
An empty resource scope addresses service-wide actions. Ref and Aspect are exact optional
qualifiers; Object retains its existing `path.Match` pattern behavior as an
opaque application qualifier. Object is **not** the source file path of a
Dataset and is not interpreted as an object identity. Dataset file
coordinates and the accepted publication determine file selection independently.
Legacy command-to-action mappings remain in the KC adapter. Rule decoding
preserves the established retired-field rejection and on-read
`dataset.consume` to `file.read` migration. The retired JSON `catalog` selector
and `catalog.*` action namespace are explicitly rejected; they are never silently
dropped to widen an existing grant. Inventory, source management and registry
audit use `inventory.read`, `repositories.manage` / `repositories.create`, and
`registry.audit.read`.

`Match` is the shared explicit-rule evaluator. It does not evaluate public
service inventory discovery, authenticated repository defaults, local-owner bypass,
temporary object definitions or application delivery policy. Those policies
remain in the caller and use the same evaluator.

`Service` provides authenticated inventory, add, update and revoke operations through a
`RuleStore`. The caller supplies a verified principal. Scoped administration
uses explicit coordinates; revoke derives its target from the durable rule ID.
Unknown and ambiguous IDs remain unscoped before authorization. Ref/Object/
Aspect constrained management rules fail closed; this package does not invent
glob-containment or permission delegation semantics. Read and manage are
separate actions, and native administrator status implies neither.

`RuleStore.UpdateRules` serializes authorization and mutation together, preserves
state on a callback error and acknowledges only a successful durable update.
The KC adapter updates the Rules portion of the original `allow.json`, preserving
provisioning, admission and identity migration receipts. There is no second
policy ledger, initialization-on-read or rule restoration on restart.
`AddRule`/`RemoveRule` expose the same domain operations for the existing trusted
CLI operation boundary; they do not authorize a caller themselves. Grant IDs
remain random and opaque, and ambiguous legacy IDs cannot be revoked.

`Update` replaces one unique rule atomically, preserving its ID and provenance.
The caller supplies the expected previous `Rule` and the requested replacement.
Both the durable old scope and the requested new scope require current manage
authority inside the same `UpdateRules` boundary. A changed previous rule yields
`PRECONDITION_FAILED`; no failed update removes or replaces the original rule.
Repository and Dataset scopes can be combined and remain an intersection;
changing scope still requires current authority over both old and new scopes. Rules owned by a share cannot be
edited here; their lifecycle remains with sharing. This is a full replacement,
not a revoke followed by an add, and does not grant self-service elevation.

Inventory lists explicit rules only. It does not claim to describe effective
permissions and does not expose private deployment ledger records. Pins and
cached content never cache an authorization decision: services evaluate the
current rules for every new request.

## 原 Repo 操作

`repository_actions.go` 拥有闭集原 Repo 操作动作与求值选择。原仓 metadata、history、文件读取、文件写入、commit、引用管理、merge、Settings、Actions、PR 和 import 分别授权。`MatchRepository` 沿用同一规则求值，排除 Dataset、Object、Aspect 限定规则；这些约束不充当文件 ACL。账号管理和认证不由此包重建。

原仓自定义元数据读取使用 `repository.metadata.read`，增量设置与按键删除使用
`repository.metadata.write`。元数据写权不隐含文件、commit、Settings 或授权管理权；
这些动作也不隐含元数据写权。元数据键和值不进入规则求值，不因 `owner`、`source`
等自定义键产生授权或来源身份。

## Dataset 来源发布许可

`repository.dataset.publish` 允许将选定源版本发布到 Dataset，独立于目标的 `dataset.manage`。
仓 `file.read`、Dataset 管理与 `dataset.*` 不隐含这项源许可。`MatchSourcePublication` 要求
请求同时给出来源 Repo、目标 Dataset 与选定 Ref；规则可限定来源 Repo，也可将 Repo 与
Dataset 限定为交集，服务全局显式规则继续成立。Dataset-only、Object 或 Aspect 限定规则
不能成为私有来源的发布许可。Ref 在调用者显式固定 Commit 时表示该 Commit，否则表示
选定 selector（空值使用 `snapshot.DefaultRef`）；不能用 `main` 标签借用另一个 Commit 的权。

管理包含此动作的 grant（也包括覆盖它的 `repository.*` 等 wildcard）需要来源 Repo 或
服务全局的 `admin.grants.manage`。仅有 Dataset 的 grant 管理权不能授予、替换或撤销这项
来源许可。Repo×Dataset 的明确管理交集仅作用于该交集；不从 Dataset 管理推导来源管理。

该动作可由部署的 `CreatorActions` 显式选择，普通读取与 `ShareActions` 闭集不包含它。
创建重放与重启沿用既有耐久授权/撤销事实，不补授这项许可。已发布 Dataset 的消费继续按
Dataset 当前授权与固定文件范围执行，撤销源发布许可阻止后续发布，不追加逐读来源许可。
