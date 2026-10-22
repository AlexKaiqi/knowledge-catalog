# DEBT-PROTOCOL-SCHEMA-COPY-DRIFT-01 · 协议 Schema 副本只有索引声明被锁

## 债

- 位置：`object/trusted_schema.go:47-64` 的 `AssertProtocolSchemaPublication`；覆盖断言见 `object/readme_test.go:58-90`；存量说明见 `tasks/done/SYSTEM-META.md` 的"存量债登记"。
- 信号：`AssertProtocolSchemaPublication` 首行 `if objectID != IndexDeclarationSchemaV1 { return nil }` 只检查索引声明，
  而 `TrustedSchemaOperations()` 还声明了不可演进的 Meta Schema。其余系统出版的 readme、relation 等是普通 Schema，
  不在该内置保护集合；原登记将它们视为“漏锁协议 Schema”的推断不成立。
- 判定：真实实现缺口是 Meta Schema 的一致副本约束未生效。同时系统仓多处把约束泛称为“所有系统 Schema 副本一致”，
  与普通 Schema 的既有兼容演进合同不符，须同时修正实现与过宽文案。
- 处置：一致性检查覆盖 `TrustedSchemaOperations()` 的完整集合；保留普通 readme、relation Schema 的演进断言，
  为每个受保护 Schema 补发布与拒绝漂移回归。不改变仅索引声明可从内置定义直接解析的另一项合同。
- 触发条件：下一次修改 `AssertProtocolSchemaPublication`、`TrustedSchemaOperations` 或协议 Schema 发布路径时。

## 目标

所有被声明为"副本必须与系统出版物一致"的协议 Schema，在 Writer 侧都真的被拒绝漂移；允许各仓演进的类型有明确清单和理由，而不是靠一个 ID 白名单隐式决定。

## 依据

- [对象](../../doc/design/object.md)：Schema 与实例、兼容修改的解释义务。
- [工程质量闭环](../../doc/engineering/quality.md)：宣称与守卫不一致属于必须有三出口的问题。

## 验收

- 一致性检查覆盖范围与选定的清单一致，边界有文字依据而非仅代码判断。
- 对清单内每个协议 Schema，各有"发布一致副本通过"与"发布漂移副本被拒"两个方向的回归。
- `go test ./object/... ./systemrepo/...` 全绿，无 skip；`make quality`、`make test-contracts` 通过。

## 前置

- 无（可与 `SYSTEM-GUIDE-EXECUTABLE-01` 并行，改动的文件不重叠）。

## 不做

不改变业务仓对 Schema 的正常发布与演进路径；不新增原地平台升级合同；不改既有协议 Schema 的字段。

## 边界复核

当前集合仅为 `schema/meta/schema-definition/v1` 与 `schema/core/index-declaration/v1`。
对象设计明确 Meta Schema 由服务内置校验，`TrustedSchemaOperations` 的公开合同声明这两份数据不可演进；
`object/README.md`、既有 readme 回归及普通 Schema 兼容检查则保留普通内容的演进责任。
`ProtocolTrustRootSchema` 处理无需仓内副本的实例解析，当前只包含索引声明，不能将它与副本保护集合混为一谈。

## 本轮交付（2026-10-20）

复核工作区已有实现：保护集合取自 `TrustedSchemaOperations()`，Meta Schema 与索引声明 Schema
均拒绝描述字段这样的兼容漂移；普通 readme、relation Schema 仍允许兼容演进。
仅索引声明可通过内置信任根直接解析的合同未改变；系统仓过宽文案已对应收窄。

`go test ./object/... ./systemrepo/...` 与 `make quality`（含 check-docs）通过，
记录在 `.validation/runs/20261020-task-closeout/`。Writer 回归涵盖一致副本首发、首发漂移拒绝、
更新漂移拒绝后值与版本保留，以及普通 Schema 演进与旧版可读。
完整 `make test-contracts` 首轮暴露对象版本提案的文档命令不匹配，随 `OBJECT-VERSIONING-01`
修正文档表达；另有未配置外部 live 用例显式 skip，不能按本任务无 skip 验收归档。
本轮未提交，未修改协议 Schema 字段；后续完整门禁结果另记。

最终定向对象/Writer/系统仓回归、`make quality` 与冻结后的 `make test` 通过。
完整契约首轮 5111 pass / 4 fail / 14 skip；失败的两组文档/场景声明守卫已修复并定向通过，
其中 Dataset 新设计缺口登记为 `DEBT-DATASET-VERSION-CONTRACT-01`。
完整运行源码有变化，尚未取得同候选无 skip 的完整契约，仍保持 doing。
详细命令日志见 `.validation/runs/20261020-task-closeout/`。

## 交付（2026-10-21）

实现已在工作区落地并经当前源码复核（`object/trusted_schema.go` 的一致性检查覆盖
`TrustedSchemaOperations()` 完整集合；历史授权每个来源读取前单独检查 `object.history.read`；
未应用写入证明经统一错误信封传递）。

当前源码统一门禁：`make check-docs` PASS、`make quality` 通过
（`units=86 propagation=13.69%`，ratchet held）、`make test` 0 FAIL / 0 SKIP、
`make test-contracts` 0 FAIL / 18 SKIP，e2e `53/53 successful`。

18 项 SKIP 经逐条核对全部为既有环境分级项（lakeFS 独立部署、太湖认证、Luna 重排、
HTTP state runtime 独立栈、规模与吞吐），不是本项遗留的断言缺口。
按既定判断，环境分级跳过不计入本项阻塞。
