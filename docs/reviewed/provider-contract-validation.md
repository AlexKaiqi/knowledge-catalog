# Provider 合同与跨 Provider 等价性验证设计

> 状态：Dolt adapter 已按 [`STORE_ADAPTERS.md`](../STORE_ADAPTERS.md) 的裁定退役删除；本文保留历史选型与实测记录，其中 Dolt 相关入口、命令与合同不再存在于代码中。

日期：2026-09-09
状态：验证设计；维度 A 与 Nightly-fast 的核心语义对拍已接入，能力/恢复/发布档门槛按本页逐项报告

本页拥有 provider 合同覆盖与**跨 provider 等价性**的负载模型、执行方法和验收门槛，不表示这些档位已执行或通过。
通用验证方法、新增用例规范与运行报告入口见 [`test-catalog.md`](test-catalog.md) §0.2；
实现完成度与缺口台账见 [`mvp-acceptance.md`](mvp-acceptance.md) / [`TASK.md`](../../TASK.md)。
能力合同、被否决方案与替换改动集判定见 [`PROVIDER_ABSTRACTION_CONTRACT.md`](../PROVIDER_ABSTRACTION_CONTRACT.md)。

---

## 1. 本设计要回答的问题

1. 代表规模档的 native provider 是否满足与文件/tree provider 相同的 Repository/Writer 合同？
2. 两个 provider 对同一条 Operation 序列是否产生等价的解析、值、差分、来源与失败结论？
3. 变化识别能力缺失时，系统是显式失败还是静默退化为全量？
4. 规模权威 provider 是否关闭了绕过知识不变量的原始路径写入？
5. 只提供基础权威口的 provider 是否被明确报告为不可用？
6. 替换一个底座时，改动集是否只落在 adapter、装配与守卫，而不触及 ①/②/③ 公开语义？

不以"命令成功""测试存在""历史通过"代替结论，也不通过 `-short`、环境变量或降级配置获取通过。

---

## 2. 现状基线（只读，非门槛）

- **合同套件已存在且与 provider 无关。** `RepositoryContract`（[`internal/testkit/contract.go`](../../internal/testkit/contract.go)，T1–T12）与 `WriterContract`（[`internal/testkit/writer_contract.go`](../../internal/testkit/writer_contract.go)）都接受 `func(t, id) snapshot.Store` 工厂，内部自行用 Writer 与 Reader 组装，由 Reader 决定走原生解释还是 tree 解释。
- **已有四个调用点**，覆盖内存 tree、层 ⓪ Dolt（`snapshot/dolt`）、Gitea和 `knowledge/dolt` 原生行实现。native 调用点是
  `TestNativeKnowledgeDoltRepositoryAndWriterContracts`，不是层 ⓪ 字面路径适配器。
- **已有跨 provider 对拍**：`internal/testkit.ProviderParityContract` 接受两个
  provider-neutral 工厂；`TestNativeKnowledgeDoltMatchesTreeProviderByOperationStep` 以步骤
  坐标比较内存 tree 解释器和 native Dolt 的解析、值、声明、来源、历史、变化、维护分页与失败码。
- **条件跳过机制已存在且被记录**：`requireDoltRuntime`（`snapshot/dolt`）在缺少 dolt/docker 时跳过，并可用 `KC_REQUIRE_LIVE_ADAPTERS=1` 把跳过变成失败。本设计沿用该机制，不另造。
- **读取路径的归属已明确**：Reader 对 `knowledge.NativeRepository` 直接返回，否则要求字面路径能力并包一层解释器（[`knowledge/reader`](../../knowledge/reader/repository_service.go)）。因此"文件 provider"必须经解释器，"原生 provider"直用自身实现。
- **已执行的接入证据（2026-09-10）**：`KC_REQUIRE_LIVE_ADAPTERS=1` 下，native
  `RepositoryContract` + `WriterContract` 的 14 个子测试全部实际执行并通过，零跳过；
  同一进程复用 `dolt sql --continue` 会话后约 164s。Nightly-fast 确定性对拍 5 个成功步骤
  加失败步骤全部通过，约 20s。
- **对拍确实发现并关闭分歧**：首次运行发现 tree/native 的 Relation RESOLVE Address 和
  REMOVE 历史不一致；修复后同一脚本差异为空。排序只用于本页 §5.3 明定为集合的地址与声明，
  不掩盖值、状态、digest 或错误码差异。
- **仍未通过的档位保持未通过**：Gitea 配对、发布档恢复矩阵和证据 manifest 尚未在同一轮
  执行；不能由 Nightly-fast 结果外推。

---

## 3. 两个正交的验证维度

现有合同套件回答"每个 provider 各自是否满足合同"。它**不能**回答"两个 provider 是否给出相同答案"。
本设计因此有两个维度，必须分别报告，不得互相代替：

```text
维度 A：合同符合性   一个 provider × 全部断言
维度 B：跨 provider 等价性   同一序列 × 两个 provider × 逐步骤对比
```

维度 B 是 [`scale-architecture.md`](scale-architecture.md) §5.3 所述"迁移核心差分测试"的落实；
维度 A 是它的前提。

---

## 4. 维度 A：合同覆盖

### 4.1 测试项

| 项 | 内容 | 说明 |
|---|---|---|
| `A-1` | native 工厂接入 `RepositoryContract` | 工厂形态与层 ⓪ Dolt 调用点保持一致：每个 id 一个隔离的临时 Repository |
| `A-2` | native 工厂接入 `WriterContract` | 覆盖命令幂等/摘要冲突、同 ChangeSet 的 schema_ref、PROPOSAL 候选 ref |
| `A-3` | 断言真实执行而非跳过 | `KC_REQUIRE_LIVE_ADAPTERS=1` 时必须失败而不是跳过 |
| `A-4` | 覆盖写入路径归属 | 必须证明用例走的是原生写能力，而不是退回字面路径 codec |

`A-4` 不能靠读代码断言。可核验的观察方式是**从 provider 移除原生写能力后同一用例必须显式失败**
（即 §6 的毒化项），而不是"仍能成功"。

### 4.2 跳过登记

短套件中的条件跳过沿用仓库既有做法：跳过必须登记在运行记录里，**不计为通过**。
报告必须分别给出"实际执行/通过/失败/跳过"的计数，不得只给合并值。

---

## 5. 维度 B：跨 provider 等价性

### 5.1 配对选择（按实测成本定档）

F 侧只需是一个**经 Reader 解释的文件 provider**；N 侧当前只能是 `knowledge/dolt`（唯一原生实现），
因此 N 侧成本固定，F 侧选型只影响增量。同一合同在三种 F 下的实测（2026-09-09，同机，dolt CLI 缺失故走容器模式）：

| F 侧 | 实测 | 形态 | 适用 |
|---|---:|---|---|
| 内存 tree（[`internal/testkit`](../../internal/testkit/README.md)） | 0.46s | 进程内；仍经真实 Reader 解释器与文件 codec | 最快的语义等价；不代表真实 ⓪ adapter |
| Gitea（容器每测试二进制复用） | 39.7s | 每次操作 HTTP | 真实 adapter 语义等价 |
| `snapshot/dolt` ⓪（容器 per call） | ~410s | 每次 dolt 调用新建容器 | 与 N 侧同底座的编码差分 |

配对矩阵：

| 档 | F | N | 用途 |
|---|---|---|---|
| Nightly-fast | 内存 tree | native Dolt | 知识代数等价；无需 Gitea 服务 |
| Nightly-live | Gitea | native Dolt | 真实 adapter 等价 |
| Release | Gitea + `snapshot/dolt` ⓪ | native Dolt | 完整迁移差分 |

两条结论：

- **Gitea 代替 `snapshot/dolt` ⓪ 作 F 侧成立，并把 F 侧成本降约一个数量级**；而且这就是
  [`scale-benchmark.md`](scale-benchmark.md) §7 P0 已经指定的配对（Gitea 与 native Dolt 跑同一随机 ChangeSet 序列）。
  本节是该配对在 conformance 规模上的落点；规模负载与资格门槛仍由 `scale-benchmark.md` 拥有。
- **换 F 不是成本大头**：N 侧固定约 410s，瓶颈是"每次 dolt 调用新建容器"。这正是
  [`scale-architecture.md`](scale-architecture.md) §3.1 列为不可接受成本反例的 transport 形态；
  降低该档成本的正确方向是长连接 / `dolt sql-server`（同文 §5.4 已选定），而不是换 F。

### 5.2 序列与坐标对齐

- 序列是一段**确定性、可复现**的 Operation 脚本，由固定 seed 生成，逐步施加；
- 每步产生一个 commit；**不同 provider 的 commit 标识必然不同**，因此比较按**步骤索引**对齐，不按 commit 标识；
- 观察函数在给定步骤索引上取值，输出规范化记录；等价 = 两侧在**所有步骤、所有观察目标**上记录相等。

禁止把 commit 标识、物理字节、表/文件布局、时间戳纳入比较；这些属于实现，不属于公开语义。

### 5.3 观察维度

| 维度 | 公开语义来源 | 比较方式 |
|---|---|---|
| 对象存在状态 | Resolve 的状态与路径提示 | 相等 |
| 对象值 | Read 的完整值 | 规范化摘要相等 |
| 单元值 | 按 Address 的精确读取 | 规范化摘要相等 |
| 组成地址集合 | Read 的单元集合 | 排序后相等 |
| 声明 | 单元声明及其摘要 | 摘要相等 |
| 来源 | 来源追溯 | 相等 |
| 对象历史 | 按坐标的历史序列 | 映射为步骤索引后相等 |
| 两版本差分 | 单对象差分 | 值摘要相等 |
| 变化识别 | 两坐标间的受影响对象集合 | 集合相等 |
| 维护分页 | 有界扫描页的身份集合 | 集合相等 |
| 失败语义 | 错误码 | 相等 |

### 5.4 脚本必须覆盖的形状

只跑"简单 PUT 几个对象"无法暴露分歧。脚本至少覆盖下表；每条都要在两个 provider 上产生**同一步骤、同一观察**。

| 形状 | 期望 |
|---|---|
| 单元级独立演进（同一对象多个 Aspect 分别提交） | 组成与单元读一致，互不覆盖 |
| 路径迁移（身份不变、路径提示改变） | 身份稳定，路径提示随版本推进 |
| Entity/Relation 混用同一对象标识 | 两侧同样拒绝，不出现一侧接受 |
| 非规范对象形状（多份裸正文、裸正文与 Aspect 混用） | 两侧同样拒绝 |
| 同 ChangeSet 内声明与引用（schema_ref 前向引用、缺失声明） | 两侧同样解析或同样拒绝 |
| 值来源与 Binding 声明校验（快照来源不得含动态绑定、内联与引用互斥） | 两侧同样拒绝 |
| 非法关系形状（端点少于两个、重复端点、未限定仓身份） | 两侧同样拒绝 |
| 合法跨仓端点引用（端点仓无需接入） | 两侧保留完整引用；只写关系存储仓，不访问或修改端点仓 |
| 三种前置条件（不存在/对象相等/摘要相等） | 成功与失败步骤逐一对应 |
| Entity 删除 | 状态为已删除，且前一版本仍可读 |
| 归档后写入 | 两侧同样拒绝 |
| 成员/记录粒度 | 同一对象的成员组成一致 |

---

## 6. 维度 C：能力拒绝与毒化

| 项 | 构造 | 期望禁止观察 |
|---|---|---|
| `C-1` | 只实现基础权威口的 fixture | 读或写被报告为成功；空结果冒充可用 |
| `C-2` | 有字面路径能力但无原生写能力 | 静默回退全量扫描后成功 |
| `C-3` | 有原生读能力但无变化识别能力 | 投影增量退化为两次全量遍历 |
| `C-4` | 规模权威 provider 的原始路径写入口 | 原始路径提交成功并改写权威内容 |

`C-4` 是 [`TASK.md`](../../TASK.md) DOC-14 完成标准的一部分。

---

## 7. 维度 D：恢复等价

| 项 | 构造 | 期望 |
|---|---|---|
| `D-1` | 命令预留后、提交前中断 | 两侧都不得留下部分可见内容 |
| `D-2` | 提交后、账本完成前中断，随后重开 | 两侧对同一命令身份给出**等价结论**：都能重建原结果，或都失败关闭并等待人工核对 |
| `D-3` | 同一命令身份、不同摘要重放 | 两侧都拒绝复用该身份 |

`D-2` 是能力合同 `PAC-07` 的证据；若某一 provider 无法把命令身份与提交关联，必须表现为显式失败而不是静默接受。

---

## 8. 维度 E：装配面

| 项 | 内容 |
|---|---|
| `E-1` | substrate 名称的归一化、校验与默认值只出现在枚举位置，不在装配根之外散落硬编码 |
| `E-2` | 新增一个 provider 时，①/②/③ 公开语义的签名与行为不变 |

`E-2` 通过"改动集只落在 adapter、装配、守卫、文档图"来证伪；若必须改 Reader/Writer 公开语义，
按 [`PROVIDER_ABSTRACTION_CONTRACT.md`](../PROVIDER_ABSTRACTION_CONTRACT.md) 视为抽象破损，先修合同。

---

## 9. 执行分级

| 级别 | 内容 | 触发 |
|---|---|---|
| PR | `C-1`、`E-1`；本机具备 dolt CLI 或长连接服务时可加 `A-1` `A-2` | 每次 provider、Writer、Reader 相关改动 |
| Nightly-fast | 维度 A + 维度 B（F=内存 tree） | 具备 dolt 运行时的专用 runner |
| Nightly-live | 维度 A + 维度 B（F=Gitea） | Gitea 与 dolt 均可用 |
| Release candidate | 维度 A + B + C + D + E，F 覆盖内存 tree / Gitea / `snapshot/dolt` ⓪ | 发布候选、provider 新增、编码或布局变更 |

配对与历史实测成本见 §5.1。当前 adapter 在同一 Repository 生命周期复用
`dolt sql --continue` 会话，变更前释放写租约；它消除了逐查询容器启动，但完整 native 合同
仍属于 live 档。PR 档要么使用本机 dolt/可复用引擎，要么只跑不依赖外部运行时的毒化项与结构守卫。
普通 `make test` 不得启动 live dolt；沿用 `testing.Short` 与 `KC_REQUIRE_LIVE_ADAPTERS` 的既有约定。

---

## 10. 验收门槛

以下为首版 gate。每条给出可证伪观察与所需证据；**未执行、被跳过或环境缺失的档位必须标为未通过**。

| ID | 门槛 | 对应 | 证据 |
|---|---|---|---|
| `PV-01` | native provider 完整通过 `RepositoryContract`，真实执行 | `PAC-01` `PAC-03` | 运行输出，零未登记跳过 |
| `PV-02` | native provider 完整通过 `WriterContract`，真实执行 | `PAC-03` | 同上 |
| `PV-03` | 脚本每一步的每个观察维度逐条相等 | `PAC-03` `PAC-04` | 差异清单为空 |
| `PV-04` | 失败步骤的错误码逐一相等 | `PAC-03` | 步骤索引对照表 |
| `PV-05` | 变化识别结果集合与脚本推导的期望集合相等 | `PAC-01` | 集合差为空 |
| `PV-06` | 只提供基础权威口的 provider 在读写上失败关闭 | `PAC-02` | 毒化用例输出 |
| `PV-07` | 能力缺失时不得出现静默全量回退 | `PAC-01` `PAC-08` | 扫描/遍历计数为 0 或显式失败 |
| `PV-08` | 规模权威 provider 的原始路径写被拒绝 | `PAC-06` | 毒化用例输出（DOC-14） |
| `PV-09` | 中断后两 provider 的恢复结论等价 | `PAC-07` | 恢复用例输出 |
| `PV-10` | 证据包含 Kernel revision、Go 版本、dolt 版本或镜像摘要、OS、脚本 seed | — | manifest |
| `PV-11` | 报告分别给出执行/通过/失败/跳过计数，未执行档位不得计为通过 | — | report 一致性检查 |
| `PV-12` | 装配改动集不触及 ①/②/③ 公开语义 | `PAC-02` | 改动文件清单 + 分层守卫 |

硬门槛（出现即判失败，不得以其它档位通过抵消）：`PV-03` `PV-04` `PV-06` `PV-09` `PV-11`。

---

## 11. 停止条件

出现以下任一情况立即停止该轮，保存证据并先修正确性：

- 两个 provider 在同一 Operation 上产生不同值、不同存在状态或不同失败码；
- 出现未声明的静默全量回退；
- 任一条件跳过被计入通过；
- 规模权威 provider 的原始路径写入成功；
- 中断后出现部分可见的权威内容，或命令身份无法核对且未被显式报告；
- 用例通过修改模型、关闭历史、放宽断言或删除毒化项而变绿。

---

## 12. 证据格式

```text
manifest.json     KC revision、Go、dolt 版本/摘要、OS、脚本 seed、配对定义
contract.json     维度 A：逐用例执行/通过/失败/跳过
parity.json       维度 B：逐步骤、逐维度的规范化摘要与首个差异
capability.json   维度 C：毒化构造与期望拒绝
recovery.json     维度 D：中断点与恢复结论
gates.json        PV-01…PV-12 的通过/失败/未执行
```

原始日志不提交到仓库；结论必须可由 manifest + evidence 复算。差异报告必须给出最小复现：
配对、步骤索引、观察维度、两侧取值。

---

## 13. 测试落点

| 落点 | 内容 |
|---|---|
| [`internal/testkit`](../../internal/testkit/README.md) | 差分 harness 与脚本生成器；**不得 import 具体 adapter**，配对由调用方注入 |
| `knowledge/dolt/` | native 合同接入、native × tree 差分 wiring、恢复用例 |
| `snapshot/dolt` | 复用既有 dolt 运行时门控；不重复实现门控 |
| [`internal/arch`](../../internal/arch/layers_test.go) | 维度 E 的结构守卫 |

具体用例命名、库存与覆盖分母由公开注册表与测试代码生成，不在本文手工维护
（[`test-catalog.md`](test-catalog.md)、[`.data/scenes/README.md`](../../.data/scenes/README.md)）。
