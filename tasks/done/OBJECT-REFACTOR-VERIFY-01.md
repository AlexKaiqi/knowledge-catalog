# OBJECT-REFACTOR-VERIFY-01 · 按验证依据裁决重构路线（2026-10-12）

- [x] 本轮完成独立评估与计划修订：从目标验收、实际包依赖和用例树验证机制比较路线，撤回按层划分的 P0～P7；采用 R0 先补验收、R1 必要行为纠偏、R2 一次组织迁移、R3 最终验收，未启动实现。
- Goal / owner：object-refactor-plan 拥有路线，object-refactor-guideline 拥有目标与边界；验证按 test-catalog / 场景树合同，不把既有覆盖当目标限制。
- 依据：当前 `go list -json ./...` 枚举 60 个生产包，knowledge / reader / serving / retrieval / index 的直接生产消费方分别为 25 / 7 / 5 / 9 / 5；计数只说明影响面，结合共享类型、混合职责、无需兼容和不改持久/wire 语义选择统一组织切换。行为验收先在旧实现取基线，架构目标在迁移批次先红后绿；不强制经营每层的过渡 API。
- 更正：ResourceLookup 构造适配器本身不访问源，WithLiveAccess 优先选择已注册 Handler，已有独立 Echo 正式调用测试；这里先判为结构耦合，不直接判“普通 Interface 不能调用”的行为缺陷。必要行为风险仍由反例复核，不靠读到类型名推导故障。
- 本轮范围与证据：只改计划、guideline 执行原则、文档图标题及任务记录；views / product / states 三项树静态检查通过，未运行场景或业务回归。运行 `make check-docs` / `make quality` 并复核增量；不改 Snapshot 在途源码、不安装依赖、不提交。
- 验证结果：`make quality` 退出码 0，含 32 文档 / 143 关系的文档检查、工程门禁和耦合棘轮通过；`git diff --check` 通过。三项树检查只证明组织、声明及共享前态合同，`go list` 只证明静态枚举；未将其记作业务或部署验收，R0～R3 全部未实施。
