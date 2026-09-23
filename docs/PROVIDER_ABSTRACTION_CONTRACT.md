# Provider 能力合同与底座替换边界

日期：2026-09-10
定位：演进决策。本文只定义"更换 Snapshot 底座"必须满足的**能力合同与抽象接缝要求**，
使替换从重写降级为新增 adapter。实现完成度与缺口台账只在 `MVP_ACCEPTANCE.md` / `TASK.md` 维护；
分层不由本文定义（[`LAYERS.md`](LAYERS.md)），介质角色不由本文定义（[`STORE_ADAPTERS.md`](STORE_ADAPTERS.md)），
规模档位与门槛不由本文定义（[`SCALE_BENCHMARK.md`](reviewed/scale-benchmark.md)）。
本合同的验证负载、执行方法与验收门槛见 [`PROVIDER_CONTRACT_VALIDATION.md`](reviewed/provider-contract-validation.md)。

---

## Goal

定义更换 Snapshot 底座时必须显式声明的 provider 能力集合、能力缺失时的失败语义，以及知识代数与文件编码的分离要求；使不同 provider 能在同一套 conformance 下被差分验证，从而让规模 provider 与文件 provider 的替换不再依赖未声明的隐含假设。

## Non-Goals

- 不改变公开知识语义与写读语义；`catalog/`、`knowledge/reader`、`knowledge/writer` 的公开合同不因换底座而变（[`LAYERS.md`](LAYERS.md)）。
- 不重定义权威/派生介质划分与介质角色（[`STORE_ADAPTERS.md`](STORE_ADAPTERS.md)）。
- 不选型任何具体 substrate，也不把某一后端提升为协议本体（[`STORE_ADAPTERS.md`](STORE_ADAPTERS.md) §1）。
- 不维护实现完成度、阶段流水账或缺口清单（`MVP_ACCEPTANCE.md` / `TASK.md`）。
- 不在此登记规模档位、负载模型与验收门槛（[`SCALE_BENCHMARK.md`](reviewed/scale-benchmark.md)）。
- 不新增第四套不变量编号空间；本文 `PAC-*` 是按 [§2.1 交叉索引](reviewed/architecture-invariants.md) 规则提交的**候选**，固化前缀由该文 owner 决定。

## 硬性约束 / Invariants

候选不变量见 §3；每条都带禁止观察与预期证据入口。摘要：

- `PAC-01` provider 能力必须显式声明，能力缺失必须失败关闭，不得静默降级为全量扫描。
- `PAC-02` 可替换边界是**能力集**，不是 `snapshot.Store` 这单个接口。
- `PAC-03` 不同 provider 对同一 Operation 序列必须产生等价知识结果与等价恢复结论。
- `PAC-04` 对象组装在公开语义上只有一条实现路径。
- `PAC-05` unit 代数不得要求调用方提供路径、扩展名或文件序列化副作用。
- `PAC-06` 规模权威 provider 不得暴露绕过知识不变量的原始路径写入。
- `PAC-07` 任何 provider 的提交都必须可关联命令身份，供崩溃恢复判定。
- `PAC-08` 历史与 diff 的有界性由请求决定，不得引入与请求无关的固定全量上界。

## 选定方案 / 被否决方案

选定：

- 能力以**显式声明 + conformance** 表达，而不是靠调用方不检查结果的 `type assert` 猜；缺能力 fail-closed。
- 能力声明在**装配期一次解析并冻结**，请求期只读判定结果；缺能力在装配期即失败，而不是在请求路径上逐次断言、更不是 panic。
- 变化识别（增量）是 ⓪ 的**一等能力**：每个 authority 必须给出实现；缺失时在建投影时就显式失败，不得静默退化为全量对账。
- unit 代数产出**对象级变更**；路径、扩展名与序列化只属于文件 provider 自己的编码层。
- 原生 provider 接入统一 provider 合同套件，并建立跨 provider 语义对拍。
- 规模 provider 关闭 raw path 写口；仍需要的文件能力由**明确命名的适配层**承担，而不是留在权威 provider 上。
- 「可替换」按**能力集 + 装配改动集**定义，并把装配改动集本身作为合同的一部分验证。

否决：

- 把 `snapshot.Store` 当作可替换边界。
- 在请求路径上逐个 `type assert` 可选能力，把「有 / 没有」变成每次调用的分支。
- 把增量变化识别当作可选加速：它缺失时的退化成本是 O(仓库总量)，不是性能取舍。
- 用"再加一个可选接口"隐式扩展能力，而不声明、不验证。
- provider 内静默回退全量扫描，以"总能成功"换不可预期的成本。
- 保留两份对象组装实现，靠人工同步维持一致。
- 以包重命名或别名门面冒充 provider-neutral。
- 为换 substrate 先改 ①/②/③ 的公开语义。
- 回退 `kc_files + 伴随表` 兜底（沿用 [`SCALE_ARCHITECTURE.md`](reviewed/scale-architecture.md) S-13）。

## 接口契约 / 状态机

能力与接缝的形状由既有公开类型与 conformance 拥有，本文不复制字段，也不新造协议：

- ⓪ 权威口与可选能力：[`snapshot.Store`](../snapshot/store.go)（极薄，无知识概念）及其可选能力接口；
- ② 解释与写命令：[`knowledge.Repository`](../knowledge/repository.go) 与 [`knowledge.ChangeSet`](../knowledge/changeset.go)；
- 共享代数：[`knowledge/unitcodec`](../knowledge/unitcodec/unitcodec.go)；
- ② → ⓪ 唯一 dispatch：[`knowledge/writer/treecodec.go`](../knowledge/writer/treecodec.go)；
- provider 合同套件：[`internal/testkit`](../internal/testkit/README.md)；
- 分层与能力隔离守卫：[`internal/arch`](../internal/arch/layers_test.go)；
- 装配根：[`home/authority_drivers.go`](../home/authority_drivers.go)。

本文只规定这些接缝**必须保持的性质**（§3）与**替换时需要改动的文件集**（§5）。字段、错误码与状态机若需变更，先改上述公开类型与其 conformance。

---

## 1. 为什么要单独定义这个合同

规模设计已经选定"每个 Repository 对应一个物理库、按能力选择原生或文件解释"，并把 substrate 替换列为
可评估路径（[`SCALE_ARCHITECTURE.md`](reviewed/scale-architecture.md) S-13）。但"能不能换"目前无法被证伪：

- 底层权威口 `snapshot.Store` 确实干净且被机器强制（[`LAYERS.md`](LAYERS.md) 的 `A-01`）；
- 但决定可行性的**规模能力**散落在一组可选接口上，只能靠类型断言发现；
- 而真正代表规模档的 native provider **并未接入统一 provider 合同套件**，跨 provider 语义等价没有自动化证据。

结果是：替换 substrate 时，缺能力不会在编译期或 conformance 期暴露，而是在 1 亿对象上以
全量扫描或 OOM 的形式暴露。本文把这件事变成可检查的合同。

## 2. 替换边界是能力集

只实现 `snapshot.Store` 的 provider **不可用**：READ 侧缺解释能力时失败关闭，WRITE 侧缺 Tree 或原生写能力时失败关闭。
因此"支撑替换"的最小单位是一张能力表，而不是某一个接口。

| 能力 | 角色 | 缺失后果 |
|---|---|---|
| `snapshot.Store` | 必需 | 无法挂载 |
| 知识解释（READ/Address/Resolve） | 必需 | 消费面不可用 |
| 批量 hydrate | 必需 | 逐对象远程调用，有界批量端口退化 |
| 原生增量写（ChangeSet 应用） | 规模必需 | 退回全树解释写路径 |
| 变化识别（两固定坐标求差） | 规模必需 | 退回双全量快照比较 |
| 声明定位（Schema / Binding / referrer） | 规模必需 | 退回普通对象枚举 |
| 字面路径读取（TreeReader / Directory） | 可选 | VFS 与文件解释路径不可用 |
| 原始路径写（TreeStore） | 仅文件型 authority 可选 | native Knowledge authority 暴露绕过 Writer 的写旁路 |
| 版本历史定位 | 可选 | LOG / 历史读退化 |
| 维护枚举（有界分页扫描） | 可选（仅维护） | 重建/导出不可用；不得进入消费面 |

这张表是**能力语义**的划分，不是具体接口清单；哪些接口承载哪一项由 §4 的公开类型确定。

## 3. 不变量候选与证据入口

沿用 [`ARCHITECTURE_INVARIANTS.md`](reviewed/architecture-invariants.md) 的验收模型：每条必须有稳定决策、禁止观察与自动化证据。
**下表是候选，尚未进入该文的不变量索引；"待新增"表示证据测试尚不存在，只有文字不算固化。**

| ID | 可证伪属性 | 禁止观察 | 证据入口 |
|---|---|---|---|
| `PAC-01` | provider 显式声明能力集合，能力缺失失败关闭 | 增量能力报错被吞掉后走两次全量比较；只提供基础权威口的 provider 被报告为可读 | 待新增：能力拒绝/毒化反例 + provider 合同 |
| `PAC-02` | 可替换边界按能力集判定 | 缺解释或写能力的 provider 返回空结果或"成功" | 待新增：能力矩阵 conformance |
| `PAC-03` | 同一 Operation 序列在不同 provider 上产生等价值、解析、差分、来源与恢复结论 | 两 provider 对同一序列产生不同 digest、不同值或不同恢复判定 | 待新增：跨 provider 对拍；并把 native provider 接入 [`internal/testkit`](../internal/testkit/README.md) 合同 |
| `PAC-04` | 对象组装只有一条公开语义路径 | 只修订其中一份组装实现后，两 provider 的 READ 结果分叉 | 待新增：组装唯一性结构守卫 |
| `PAC-05` | unit 代数与文件编码分离 | 原生写路径必须执行扩展名/路径规则才能完成 | 待新增：代数层不含路径的边界检查 |
| `PAC-06` | 规模权威 provider 不暴露 raw path 写旁路 | 权威 provider 上以原始路径提交成功并改写权威内容 | 待新增：能力拒绝/毒化反例（DOC-14 完成标准） |
| `PAC-07` | 提交可关联命令身份以判定恢复 | 某 provider 的提交无法在恢复时关联命令身份 | 待新增：跨 provider 崩溃恢复合同 |
| `PAC-08` | 历史与 diff 的有界性由请求决定 | limit 很小的历史查询拉取与请求无关的固定数量提交 | 待新增：有界性反例 |

与既有约束的关系：`PAC-01`/`PAC-08` 受 [`SCALE_ARCHITECTURE.md`](reviewed/scale-architecture.md) §3.1 与
[`SCALE_BENCHMARK.md`](reviewed/scale-benchmark.md) §10.5「稳态热路径全仓扫描计数为 0」约束；
`PAC-03` 是 [`SCALE_ARCHITECTURE.md`](reviewed/scale-architecture.md) §5.3 所述迁移差分测试的固化；
`PAC-07` 对应其 §8.3 的恢复判定；`PAC-06` 对应其 §5.2 的能力隔离要求。

## 4. 审计基线（只读快照，非合同）

以下是一次只读审计的结论，用于说明 §3 的必要性。它是**带日期的快照，会漂移**，不作为合同或台账。
行号仅供定位；`make check-docs` 不校验本节。

- 已做好的接缝：`snapshot.Store` 极薄且无知识概念；① 与 ②③ 由可达集断言隔离；知识→快照握手单向（Writer 造命令、权威口只消费）；缺能力的原生协商显式且失败关闭；具体 adapter 只出现在唯一装配根；SQL 引擎依赖不存在于任何 adapter 包；EAR 形状干净（N 元 typed Relation、身份/值/声明分离、身份与路径无关）。
- 已闭环一：native provider 已接入 `RepositoryContract` / `WriterContract`，并由
  `ProviderParityContract` 与 tree provider 按步骤对拍；实际门槛与未执行档见验证 owner。
- 已闭环二：已声明的变化能力报错直接返回，Index 不把它改成 rebuild；只有已证明旧 basis
  不属于当前 authority generation 时才以 `diverged` 明确重建。
- 已闭环三：对象组装与 declaration digest 由 [`knowledge/unitcodec`](../knowledge/unitcodec/unitcodec.go)
  唯一拥有，tree codec 只把文件 unit 转成 provider-neutral unit。
- 已闭环四：`unitcodec.Unit` 不含 path、扩展名、文件内容或序列化字段；文件规则只留在
  `internal/repofile`，native 行写不再要求生成 storage path。
- 已闭环五：`knowledge/dolt` 只实现 `TreeReader`，不实现 raw-write `TreeStore`。普通文件
  功能由 Home 装配根的具名 file-capability adapter 提供，不改变 native provider 合同。
- 缺口六：substrate 名称硬编码在装配根之外的多处部署/flag/profile 代码中，替换成本并非"一个 adapter + 一个装配根"。
- 附带发现：设计文中 `R-08` 在两处文档中指向不同内容，说明编号权威本身需要收敛，但**不得**与本文的能力合同混为一谈。

## 5. 替换一个底座的判定与改动集

判定顺序（先证伪，再选型）：

1. 该 provider 声明了 §2 中哪些能力；
2. 缺的能力是否只影响可选路径（VFS、历史、维护），还是影响规模必需能力；
3. 跨 provider 对拍与崩溃恢复合同是否通过；
4. `SCALE_BENCHMARK.md` 的资格档与"零全仓扫描"是否通过；
5. 装配改动集是否可枚举且不触及 ①/②/③ 公开语义。

改动集约定：

- **应当改**：`snapshot/<driver>/`、`knowledge/<driver>/`、装配根与其配置校验、provider 合同套件、分层/导入守卫、文档图。
- **应当不改**：`catalog/`、`retrieval/`、`index/`、`delivery/`，以及 Reader/Writer 的公开语义。
- 若替换必须改动 Reader/Writer 的公开语义，视为抽象破损，应先修合同而不是改协议。

LakeFS 是该改动集的第一个新增后端反例：`snapshot/lakefs`、Home driver registry、部署 binding
校验和分层守卫可以增加；Catalog、Reader、Writer、Index 的公开语义不改变。它必须运行
`RepositoryContract` 与 `ProviderParityContract`，而不能只靠“实现了 `snapshot.Store`”宣称
可替换。S3/COS/Ceph/MinIO 只承担该 provider 的对象数据平面；普通对象桶没有 ref、不可变 commit
图与 expected-old CAS，不能独立声明满足能力集。

## 6. 收敛顺序

前置依赖，不是实现进度计划（工作项记入 `TASK.md`）：

| 步骤 | 前置条件 | 理由 |
|---|---|---|
| 1. native provider 接入合同套件并做跨 provider 对拍 | 无 | 没有对拍，"抽象够好"不可证伪，后续判断都不可信 |
| 2. 能力显式声明 + 缺能力失败关闭 | 步骤 1 | 让缺口在编译期或 conformance 期暴露，而不是在亿级数据上 |
| 3. 消除静默全量回退 | 步骤 2 | 把不可预期成本变成显式失败或显式 partial |
| 4. 组装收敛为一条路径 | 步骤 1 | 两份实现是 provider 语义分叉的入口 |
| 5. 代数与编码分离 | 步骤 4 | 让新 provider 不必接受文件概念 |
| 6. 关闭权威 provider 的 raw path 写口 | 步骤 2、5 | 能力隔离；文件能力下沉到明确适配层（`TASK.md` DOC-14） |
| 7. substrate 选型与容量承诺 | 步骤 1–6 | 用同一套资格门槛比较候选，见 [`SCALE_BENCHMARK.md`](reviewed/scale-benchmark.md) |

**结论：** 抽象骨架合格，能力层不合格。在步骤 1–4 完成前更换 substrate，迁移的是同一批未验证的隐含假设，
而不是一套可替换的架构。
