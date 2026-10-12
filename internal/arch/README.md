# internal/arch/

把 `docs/reviewed/core-architecture.md` 变成可执行断言，只在测试中存在。

| 文件 | 负责 |
|---|---|
| `layers_test.go` | 生产包 import DAG、`home`/`httpsurface`/`client` 边界、`knowledge/serving` 的 provider 隔离与已移除包守卫 |
| `provider_boundary_test.go` | Snapshot adapters 与 Retrieval providers 的物理边界 |
| `semantic_test.go` | 类型声明所有权；阻止 Knowledge 类型回流 kernel、Projection 回流 `knowledge/reader` |
| `single_registry_test.go` | 单服务唯一登记权威；禁止多空间公开类型、选择字段、wire tag、容器与归档生命周期回潮 |
| `snapshot_contract_test.go` | Snapshot 公开包闭集、含测试与嵌套模块的深层 import 禁止、退役路径、公开签名与具体实现隐藏 |

Snapshot 对外只允许 `kc/snapshot` 与具名的 `dataset`、`access`、`service`、`console` 一级包。
守卫同时检查参数、结果、字段、嵌入类型和别名；本包私有类型也不能借公开签名泄漏。
数据结构、数据结构的纯方法及装配回调有明确清单；权威和 handler 的工厂返回选定接口，
不允许导出的函数变量直通，也不允许 `Raw()` / `RootDir()` 后门。根包共享的六种私有纯 DTO
别名保留类型身份，并额外守卫其无私有状态、无实现方法。不可变常量不作为实现泄漏。
这些检查保留 `TreeReader`、`DirectoryReader` 等合法能力接口发现，不禁止接口断言。

单登记权威守卫检查部署与HTTP公开形状，也检查私有传输DTO中的退休选择字段。
Home和Resources只能持有单个Dataset服务与Registry，不能改成map、slice或interface指针；
登记工厂只接Snapshot authority与ref。正负夹具验证守卫不误拦来源仓自身的归档能力。
