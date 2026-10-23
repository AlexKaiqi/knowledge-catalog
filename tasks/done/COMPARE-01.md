# COMPARE-01 · 两版对象比较

## 目标

同一对象的两版可以比较：两侧各自固定单仓提交、Dataset 发布版或完整读取依据，分别经原对象授权打开，
只比较本地 Unit，不调用外部取值，不读取区间历史。

同范围且双方完整时才能判断整体是否相等；范围不同则报告范围变化及共同部分的差异，不给全对象相等结论。
保留双侧坐标，区分值变化、声明变化、Unit 加入或移除，以及选择范围带来的进入或离开。

## 依据

设计篇：[比较的状态与范围](../../doc/design/object-versioning.md#比较的状态与范围)、
[两版对象比较](../../doc/design/object-versioning.md#两版对象比较)。

已实现的语义边界：

- 本地状态摘要只回答同一逻辑范围内的本地状态是否相同。覆盖完整 Unit 的地址、保存值和本地声明；
  Unit 构成参与比较，因为拆分变化改变寻址与维护边界。路径、来源仓、commit 与 provenance 不参与。
- 字段投影不缩小所属 Unit 的比较范围：同一 Unit 中未返回字段的持久变化仍改变摘要。
  部分读取只聚合所选 Unit，不为计算摘要额外读取未选单元。
- 既有维护摘要继续服务原条件写：Snapshot 单元比较值，Binding 单元比较本地声明，不能静默更换相等关系。
- 本地状态摘要不充当索引成员的构建身份，成员复用按构建身份判断。
- 范围标识不能由返回的 Unit 集合生成，否则新增单元会被误当作范围改变。
- 本地状态完整不要求解释依赖或外部观察成功，三者各自报告。已证明不存在对象是完整的空 Unit 状态；
  无权或未知不能当空集合。

## 已交付

### 领域与 Reader

`object.CompareLocalReads` 实现完整本地 Unit 比较，保留双侧固定依据与独立状态，分别报告值、声明、
种类和来源变化。范围改变时不返回整体相等判定；同范围内 Unit 新增仍是状态变化。整仓视图可证明
新增或删除，文件切片中的消失只报告离开交付范围，不读取范围外内容推断原因。

Reader 的 `ReadComparisonBasis` 只规划并读取所选完整 Unit，不调用解释依赖、Schema 或外部取值；
空对象与失败分开，正文读取必须兑现元数据规划，冲突和部分结果明确失败。

复核发现 `AddressKey` 不含种类，原本地摘要及历史折叠可能漏掉同地址、同值、同声明的种类变化。
本地状态算法升级为 `kc-object-local-state/v2`，包含完整地址；原维护摘要和公开历史值/声明摘要不变。

### 公开入口

`objects:diff`、Object SDK `Compare` 与 `kc diff <urn>` 连接同一比较执行器。每侧必须显式给出一种
固定视图：耐久依据、仓提交或命名 Dataset 发布修订。双方各经原对象读取授权与 Hook，不合并范围借用
另一侧发权；Dataset 消费比较不额外要求来源历史权。保留原目录 `kc diff --repo ... --dir ...`，
混入比较参数而未给 URN 也拒绝，不静默改走目录模式。

形状读取在整次调用中固定主仓版本及描述符候选版本，不让不同选项分别采用新 head。

## 并入 `DEBT-OBJECT-VERSION-SEMANTICS-01`

- 位置：[核心概念](../../doc/concepts.md)的 Object Version；[对象](../../doc/design/object.md)的读取；
  [对象版本与读写](../../doc/design/object-versioning.md)的比较状态与范围。
- 信号：三处曾把彼此独立的依据压缩成同一个东西：范围限定的本地状态摘要、固定来源及依赖的读取坐标、
  辨认变化的修订历史。
- 已交付：新增[版本语义守卫](../../scripts/check-docs/version_semantics.go) 随 `make check-docs`
  扫三类混用措辞，回归四条；新增[证据引用守卫](../../scripts/check-docs/evidence_citation.go)
  禁止设计文档引用运行标识，回归四条。
- 剩余验收：同范围状态比较、不同 Dataset 范围、部分读取和 `A→B→A` 的例子能够同时成立。
  守卫只保证表述一致，不能证明行为，故仍需本任务的执行断言。

## 验收

| 场景 | 判定 |
|---|---|
| 其他对象提交、仅声明变化、Unit 拆分变化、字段投影未返回的字段变化 | 比较粒度符合设计，既有维护摘要兼容；未选 Unit 不被为算摘要而读取 |
| D1 选 A 仓、D2 选 A+B；部分无权、失败或冲突；同一选择规则下新增或删除 Unit | 范围差异与未决项可见，不误报完整对象比较；Unit 集合变化仍可比较同范围状态，本地状态不混入描述符版本或外部观察 |
| 正文相同但路径、provenance 或 Binding 声明变化 | 报告为变化，不被正文摘要掩盖 |
| Dataset 两版同来源 commit 但范围不同 | 各自保留文件范围与独立消费授权，比较不互相补齐 |
| `A → B → A`、删除后原样重建、合并提交 | 历史侧保留 first-parent 实际变化；比较侧不据端点 diff 为空跳过真实修订 |
| 任一侧无权、读取失败或组成不完整 | 比较明确失败，不给部分相等结论 |

HTTP 与 CLI 分别执行"读取取得 basis → 同范围回读 → 两版比较"完整路径，覆盖 basis 与其它坐标混用、
改变选择范围、跨重启回读，以及完整依赖缺失、无权或不可解析时不回退最新。

## 剩余

1. 同一冻结候选上的完整混合契约。此前 `.validation/runs/20261020T174955Z-b43781c01295`
   （1397 通过、2 skip）与 `.validation/runs/20261020T174956Z-cc7442aeb3c1`（138 通过、无 skip）
   都是批次证据，不是冻结候选的完整验收。
2. 真实部署验收。
3. `A → B → A` 行为断言与守卫表述的对应关系在验收表中逐条对齐。

## 前置

依赖 `READ-BASIS-01` 的固定依据作为双侧输入。

## 不做

不做跨仓原子条件；不把状态摘要升级为全局版本或仓级 CAS 的替代。比较不调用外部取值、不读区间历史。
本轮不做性能优化，也不为优化设成本目标。

## 交付

实现与公开入口已达成，见本节；整体验收未完成，任务保持 `doing/`。

- 本地比较与历史构成修复：`object.CompareLocalReads`、`ReadComparisonBasis`、
  `kc-object-local-state/v2`。冻结整包证据 `.validation/runs/20261020T172343Z-05eb547ac866`：
  1875 通过、无失败、1 条真实 skip（真实 lakeFS 吞吐环境项），源码未变化。
- 双侧比较入口：`objects:diff`、SDK `Compare`、`kc diff <urn>`。
  `.validation/runs/20261020T174955Z-b43781c01295` 修正路由与公开形状登记后 1397 通过、无失败、2 skip；
  `.validation/runs/20261020T174956Z-cc7442aeb3c1` 138 通过、无失败或 skip，覆盖双侧消费与撤权、
  完整 Unit 未投影字段、来源推进、同 commit 不同切片、切片依据重开、真实空状态及隐式最新拒绝。
- 冻结候选 `.validation/runs/20261022T154324Z-93b18dd4deb1`（默认场景 18 通过、无 skip）、
  `.validation/runs/20261022T154436Z-5d9104e40a73`（5682 通过、0 失败、18 条既有环境 skip）、
  `.validation/runs/20261022T155925Z-48b3b9703db5`（真实部署专项 2 通过、无 skip），源码指纹
  `c967676f46a0a8c53b3dd3ceff862af8c7628a36f8679ce800899db629c70a0c`，三次 `sourceChanged=false`。
- 五条路径里的「比较两版对象」由 `object-published` 节点 `probe-compare-two-versions` 在候选上执行：
  动作级 `object.read` 授权后可比较工作区范围依据；两侧同仓固定 commit 判 `scopeChanged=false`、
  `equal=false`，逐字段看到 `presence absent`、`valueChanged true`、`sourceChanged absent`；
  依据对固定提交判 `scopeChanged=true` 且 `equal` 省略；混用 `--dir` 或省略 commit 报 `USAGE_INVALID`。
- A → B → A 的对象侧落点 `TestReturnToEarlierValueKeepsFirstBasisNewRequestAndBothRevisions`：第一个 A
  的依据仍在其自身 commit 可读，新请求绑定第三次提交而非复用旧坐标，历史保留三次修订不合并。
- 未验证：索引侧复用与物理回收判定，归 `INDEX-PHYSICAL-01`。
