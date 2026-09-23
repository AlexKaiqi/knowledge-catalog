# Provider 抽象重构执行指南

> 状态：Dolt adapter 已按 [`STORE_ADAPTERS.md`](STORE_ADAPTERS.md) 的裁定退役删除；本文保留历史选型与实测记录，其中 Dolt 相关入口、命令与合同不再存在于代码中。

日期：2026-09-10
定位：**执行指南**，不是设计 owner。本文只回答四件事：从哪里开始读、按什么顺序做、每一步的完成定义是什么、怎么验证。
设计与被否决方案见 [`PROVIDER_ABSTRACTION_CONTRACT.md`](PROVIDER_ABSTRACTION_CONTRACT.md)；
门槛、负载与证据格式见 [`PROVIDER_CONTRACT_VALIDATION.md`](PROVIDER_CONTRACT_VALIDATION.md)；
验证方法与保留规则见 [`TEST_CATALOG.md`](TEST_CATALOG.md) §0；
不变量索引与固化规则见 [`ARCHITECTURE_INVARIANTS.md`](ARCHITECTURE_INVARIANTS.md)。
本文不复制字段、错误码、状态机与门槛定义。

---

## 0. 一句话目标

把「更换 Snapshot 底座」从重写降级为新增 adapter：**先让能力合同可证伪，再谈 substrate 选型。**

当前可认领条目：[`TASK.md`](../TASK.md) 的 `DOC-14`（能力隔离）与 `DOC-16`（合同接入与等价性）。

---

## 1. 阅读顺序

按序读，不要跳：

1. 本文。
2. [`LAYERS.md`](LAYERS.md) —— ⓪–③ 的层所有权与 import 方向；决定「哪些改动算越界」。
3. [`PROVIDER_ABSTRACTION_CONTRACT.md`](PROVIDER_ABSTRACTION_CONTRACT.md) —— 能力合同、`PAC-01`…`PAC-08`、被否决方案、替换改动集判定。
4. [`PROVIDER_CONTRACT_VALIDATION.md`](PROVIDER_CONTRACT_VALIDATION.md) —— 维度 `A`–`E`、`PV-01`…`PV-12` 门槛、执行分档、证据格式。
5. [`TEST_CATALOG.md`](TEST_CATALOG.md) §0 —— 测试保留规则、判读规则、复核入口。
6. [`ARCHITECTURE_INVARIANTS.md`](ARCHITECTURE_INVARIANTS.md) —— 不变量索引与「必须同时有反例测试」的固化规则。
7. 具体形状（接口、错误码、状态机）才读包 README、公开类型与 Conformance：
   [`internal/testkit`](../internal/testkit/README.md)、[`snapshot`](../snapshot/README.md)、
   [`knowledge/writer`](../knowledge/writer/README.md)、[`knowledge/reader`](../knowledge/reader/README.md)。

---

## 2. 前提：基线闸门状态（2026-09-10 实测）

| 闸门 | 状态 | 说明 |
|---|---|---|
| `make check-docs` | **PASS** | 31 documents / 93 relations |
| `make validation-inventory` | **PASS** | 60 CLI 命令、83 HTTP 路由、898 Go 声明 |
| `make check-validation` | **PASS** | 具名 Test 引用全部解析；原 3 处未解析引用的处置见 §6.1 |
| `make check-surface` | **PASS** | 环境已提供 `rg`，见 §6.2 |
| `make test` | **PASS** | 1441 pass / 0 fail / 24 skip；42 包通过、0 包失败；公开命令 60/60 成功且 60/60 满足风险分级边界要求 |
| 共享合同套件（provider 实跑） | 内存 tree PASS / Gitea PASS / native Dolt PASS | native 为临时探针，已删除；14 子测试全绿、零跳过、409.6s |

`make test` 的 24 条跳过全部是外部适配器与 live 服务门控（live Gitea、live Dolt、live Taihu、
live OpenSearch/runtime 容器、live 重排服务），属 `make test-all` 范围；本地档内没有未登记跳过。
**这些外部适配器在本次运行中未被验证，不得据此宣称其可用。** 同一次运行还暴露一个与本入口
直接相关的事实：`knowledge/maintenance` 包没有任何测试文件，与 `DOC-16` 记录的静默全量回退
缺口一致。

**结论：§6 的两个前提已解除，可以开始重构。** 每次改动前后都应在同一 revision 上复算，否则"改动前后都红"的结论无法归因。

---

## 3. 不可违背项

- 不删除或削弱契约、conformance、架构守卫断言；不用 `skip`、降级配置或无价值复制用例换绿。
- 不把字段、错误码、状态机写进设计 Markdown；形状归公开类型与 Conformance，设计只写理由与可证伪要求。
- 不为同一事实建立第二个 owner；改事实先改 owner，再改派生入口。
- Oracle（`ARCHITECTURE_INVARIANTS.md`、`TEST_CATALOG.md`、`.data/scenes/`、`internal/arch`、`internal/testkit` 合同）由人类 owner 决定增删；Agent 只产出候选与理由。
- 换 substrate 之前不得先改 ①/②/③ 公开语义；若必须改，按能力合同判定为抽象破损，先修合同。
- 复核时复用既有评审可以，但 `.validation/` 不入库，**不能**作为证据被跟踪文档引用；结论若有效必须回写被跟踪文档。

---

## 4. 工作分解与完成定义

顺序是**依赖**，不是排期。每一步的完成定义都必须可证伪。

| 步骤 | 做什么 | 完成定义 | 依赖 |
|---|---|---|---|
| `W-1` | 把 native provider 接入共享合同套件 | adapter 包内存在 contract 调用点；`A-1`/`A-2` 真实执行、零未登记跳过；`KC_REQUIRE_LIVE_ADAPTERS=1` 下缺环境为失败而非跳过 | 无 |
| `W-2` | 建跨 provider 等价性 harness（按**步骤索引**对齐） | 同一 Operation 序列在文件 provider 与 native provider 上逐步骤、逐观察维度相等；差异报告为空；`PV-03`/`PV-04` 通过 | `W-1` |
| `W-3` | 能力拒绝与毒化项 | 只实现基础权威口的 fixture 在读写上失败关闭；缺能力不静默全量回退（`PV-06`/`PV-07`） | `W-2` |
| `W-4` | 静默全量回退显式化 | 变化识别能力缺失时显式失败或显式 partial；`knowledge/maintenance` 有行为测试 | `W-3` |
| `W-5` | 权威 provider 原始路径写口收口 | 原始路径写被拒绝的反例通过；必要文件能力下沉到明确适配层（`PV-08`，`DOC-14`） | `W-3` |
| `W-6` | 代数与编码分离 | unit 代数不再要求路径/扩展名；native 写入不再执行文件序列化副作用 | `W-1` |
| `W-7` | 不变量行固化 | `PAC-01`…`PAC-08` 对应行进入 `ARCHITECTURE_INVARIANTS.md`，每行证据列指向**已存在**的 Go 测试 | `W-1`…`W-6` |
| `W-8` | substrate 选型 | 候选在同一资格档上比较，输出单代安全线与 rollover 建议 | `W-1`…`W-7` |

`W-7` 必须是最后一步：不变量索引被机器校验，见 §5。

---

## 5. 验证闭环

```bash
export PATH="$HOME/.local/go/bin:$PATH"

# 1) 文档与库存（不需要外部服务）
make check-docs
make validation-inventory && make check-validation     # 当前 RED，见 §6.1

# 2) 结构守卫
make test-boundary                                     # 仅 internal/arch

# 3) 合同与等价性（需要运行时；容器模式实测 409.6s/全程）
KC_DOLT_FORCE_DOCKER=1 KC_DOLT_DOCKER_IMAGE=dolthub/dolt:2.3.1 \
  go test ./knowledge/dolt -count=1 -v
make test-adapters
```

三条机器约束，重构时必须知道：

1. **不变量行的证据列只能写已存在的测试名。** `internal/arch/test_catalog_test.go` 会拒绝引用缺失测试的不变量行，`make check-validation` 同样会失败。因此不变量索引是工作的**产物**，不是输入。
2. **文档具名 Test 的解析只覆盖三份文档**：`docs/TEST_CATALOG.md`、`docs/ARCHITECTURE_INVARIANTS.md`、`docs/MVP_ACCEPTANCE.md`（`scripts/validation-inventory`）。本指南与验证文档不在其内，因此可以在其中写计划中的测试名。
3. **容器模式不适合 PR 档**：成本来自每次 dolt 调用新建容器，而非被测语义；配对与分档见 [`PROVIDER_CONTRACT_VALIDATION.md`](PROVIDER_CONTRACT_VALIDATION.md) §5.1 / §9。

---

## 6. 原阻塞与处置（已解除）

### 6.1 `docs/MVP_ACCEPTANCE.md` 曾引用三个不存在的具名 Test

原 `make check-validation` 输出：

```text
docs/MVP_ACCEPTANCE.md:214: unresolved TestAdmissionRequiresExplicitHumanRequestAndNeverRegrantsAfterRestart
docs/MVP_ACCEPTANCE.md:214: unresolved TestAdmissionConcurrentRequestsIssueOneDurablePolicy
docs/MVP_ACCEPTANCE.md:215: unresolved TestCatalogDiscoveryClientResolvesConfiguredWorkspaceBeforeSearch
3 exact document Test references do not resolve
```

已核实：全仓不存在这三个函数名，且它们**不是重命名**。进一步核对表明，它们描述的能力
（admission 请求/审批队列、CLI 的 Catalog 范围 SEARCH）是 `CLI-REFACTOR` 明确退役的入口，
`ARCHITECTURE_INVARIANTS.md` §4 也记着"Catalog 范围 SEARCH 未选定，也不是待实现入口"。
因此**补测试等于复活已退役能力**，处置取第二种：把这两段验收声明改写成与保留行为一致的措辞，
并引用既有测试。没有删除或削弱任何断言。

### 6.2 `make check-surface` 依赖 `rg`

原因是本机缺少 ripgrep，脚本报 `rg: command not found`，随后输出**假**的「missing route namespace」。
环境现已提供 `rg`（15.1.0），未新增仓库依赖。**该脚本在缺少 `rg` 时把工具缺失报成 surface 违规，
这一误报形态本身仍未修**；在没有 `rg` 的环境里重跑时要先确认这一点，再判读输出。

---

## 7. 已知陷阱

- **commit 标识跨 provider 必然不同**，等价性比较必须按步骤索引对齐，不能按 commit 标识。
- **不要把"接口存在"当成"能力可用"**：本仓已有实例包括零生产实现的增量端口、无调用方的证据端口、跳过 `scripts/` 的守卫。判据见测试价值审计的分类学。
- **`knowledge/dolt` 目前仍实现 `snapshot.TreeStore`**，不要把它当作能力面已收口。
- **`.validation/` 不入库**：那里的评审可以作为线索，但不能作为证据，也不能被跟踪文档引用。

---

## 8. 范围边界：本入口不覆盖什么

**本入口只覆盖一件事：provider 能力合同与跨 provider 等价性。** 它不覆盖同日并行复核中提出的其它类别，
包括（按类别，不按条目）：高层设计取舍与产品裁决、仓库身份与物理代际、检索可达性与交付可见性政策、
有界读写与定位结构形态、端口收窄与包重组、实现细节收敛。

处置规则：

1. 这些类别的问题在**各自建立 `TASK.md` 条目与 owner 文档之后**才是可执行的；本入口不为它们背书。
   当前已完成回写：有界读写与增量识别见 `DOC-17`，守卫与能力面收口见 `DOC-18`，
   可达性政策、字段级可见性、证据分级、源侧授权、撤回语义、可迁移导出与目标形态采纳见 `REVIEW-05`…`REVIEW-11`。
2. 范围重叠处（**能力协商**、**变化识别/增量**）已归并到[能力合同](PROVIDER_ABSTRACTION_CONTRACT.md)的选定方案，
   不要在别处再写一份应当被遵守的合同。
3. 并行复核的产物目录不入库，因此**只能作为线索**：结论若有效，必须回写被跟踪的 owner 文档或 `TASK.md` 条目，
   否则下一个人（或下一个 Agent）会重做一遍。
4. 与有界化重构的顺序交互：若先做写/读路径的有界化（`DOC-17`），**`W-1` 应作为它的回归网**——
   native provider 进入共享合同是低成本且立刻生效的护栏，先做不冲突。
