# internal/arch/

把 `docs/LAYERS.md` 变成可执行断言，只在测试中存在。

| 文件 | 负责 |
|---|---|
| `layers_test.go` | 生产包 import DAG、`knowledge/serving` 的 provider 隔离与已移除包守卫 |
| `provider_boundary_test.go` | Snapshot adapters 与 Retrieval providers 的物理边界 |
| `semantic_test.go` | 类型声明所有权；阻止 Knowledge 类型回流 kernel、Projection 回流 `knowledge/reader` |
