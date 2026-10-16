# OBJECT-CASES-PREP-01 · 重构前固定必要且充分的对象用例（2026-10-13）

- [ ] 本轮认领：按 Object Guideline 已选定能力建立稳定用例与前态／动作／Oracle，核对已有精确断言及局部缺口，补维护单元生命周期与拒绝后态回归，关联实际场景宿主和验证 owner。
- 范围：写用例、测试及其文档／图／场景关联，不开始生产重构，不新增临时 URN API、解析器、不可执行 feature 或 skip；未选定的集合／字段删除不进入分母。目录发布、Discovery、LiveAccess、治理仅按受影响消费兼容分别验收。
- 完成条件：每项承诺有正向控制、独立失败风险及移除后漏检说明；新入口尚未实现的风险有可证伪规格和公开接入位置，保持语义用例在既有入口执行。独立充分性／必要性复核，受影响回归、场景产品／视图检查、`make check-docs` 与 `make quality` 全绿后只记用例准备完成，不记 Object 重构或新入口验收通过。
- 用例产物：Guideline 固定 U1～U12，验证 owner §2.15 写明前态、动作、准确 Oracle、已有局部断言、接入缺口与删除风险；补身份编码／Pointer 独立向量、读取维护摘要和失败后态，场景关联只指向实际断言。新增四项回归并加强仓 CAS，不复制既有 Schema／Dataset／Writer 语义套件；根帮助、新 URN／locator 及对应 CLI→HTTP 旅程仍明确为待实现规格。
- 实际验证：新跨仓同名隔离、Member 完整替换与旧摘要拒绝、Aspect／Member 删除隔离及增强 CAS 通过；Writer 整包有根删除一例和既有表示冲突四子例失败，Reader 整包有实际只读四分支失败。`object/service`、`internal/arch`、所选 CLI 帮助／Schema／真实服务消费／范围／独立采用回归和场景覆盖合同通过。产品／视图检查、`make check-docs`（33 文档／174 关系）、最终 `make quality` 通过；不记受影响运行合同全绿，本项保留 `[ ]`，不开始生产重构。
- 证据：Writer 原始输出在 `.validation/object-cases-prep-20261013/writer-package.jsonl` 与 `writer-lifecycle-targeted-final.jsonl`；Reader／服务／架构输出 `/tmp/kc-object-cases-prep-read-boundaries-20261013.log`；CLI 输出 `/tmp/kc-object-cases-prep-cli-evidence-20261013.log`、场景合同 `/tmp/kc-object-cases-prep-scene-contracts-20261013.log`；最终质量 `/tmp/kc-object-cases-prep-quality-final-20261013.log`。未跑真实部署，不将既有证据映射算作本次执行。
- 基线缺陷登记（R1 处置，保留精确失败守卫）：
  - U5：位置 `object/format/apply.go:104`、`object/format/tree.go:60`；信号为已接受根删除后 `columns/status` 仍可读。源码显示遍历 ObjectUnits 时 Remove 原地改同一切片；判定为完整删除不成立。处置为沿唯一单元删除路径修复并重跑全单元／旧版／兄弟 Oracle；触发条件为 R1 写入或格式迁移之前，未绿不得切换。
  - U6：位置 `object/format/layout.go:145`、`object/units.go:214`；四种完整 Aspect／Members 最终冲突被接受，Ref 与 APPLIED 回执推进，原对象不可读。判定为最终表示校验缺口。处置为唯一表示规则补齐并保留显式转换正向控制；触发条件为 R1 单元代数或 Writer 迁移之前，不能用 Schema 或输入顺序遮住缺口。
  - U12：位置 `object/reader/repository_service.go:23`、`:34`；tree／native × Require／Wrap 的实际返回仍可取得 CreateRef、Merge、Archive。判定为接口收窄未隔离实际能力。处置为落实真实只读视图并保持四分支守卫；触发条件为 R1 Reader 公共边界切换之前，不能以返回类型名称或目录守卫代替。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。早于 2026-10-14 对象模型重构的认领，相关代码与设计已被后续重构改写（见 `0759640` 之后的提交），原范围按旧结构写成，不再继续。整体验收并入 `FINAL-ACCEPTANCE-01`，插件相关用例由验收规格 accept-01～13 取代。
