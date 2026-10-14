# internal/arch/

把 `docs/reviewed/core-architecture.md` 变成可执行断言，只在测试中存在。

| 文件 | 负责 |
|---|---|
| `layers_test.go` | 生产包 import DAG、`home`/`httpsurface`/`client` 边界、`liveaccess` 的 provider 隔离与已移除包守卫 |
| `provider_boundary_test.go` | Snapshot adapters 与 Retrieval providers 的物理边界 |
| `semantic_test.go` | 类型声明所有权；阻止 Knowledge 类型回流 kernel、Projection 回流 `object/reader` |
| `single_registry_test.go` | 单服务唯一登记权威；禁止多空间公开类型、选择字段、wire tag、容器与归档生命周期回潮 |
| `snapshot_contract_test.go` | Snapshot 公开包闭集、含测试与嵌套模块的深层 import 禁止、退役路径、公开签名与具体实现隐藏 |
| `snapshot_cli_ownership_test.go` | 从模块声明派生叶集合；禁止中央重复登记叶帮助、操作数/分派分支、产品输出特例和纯模块导航名单 |
| `cli_root_boundary_test.go` | 根 CLI 的生产/测试边界：禁止无符号壳、恒 nil registry、私有跨 owner 原样转发与嵌入执行；保护已有公开类型，模块单元断言归 owner，允许真实宿主装配与集成 |
| `liveaccess_module_test.go` | LiveAccess 实现、CLI 叶、明确分层与消费者依赖方向；正反夹具保留 Object core 无反向依赖守卫 |
| `plugin_adoption_test.go` | 共同采用事实仅在低层定义一次，旧插件入口与模块说明只作别名；阻止共同合同和 LiveAccess 回流索引登记运行时 |

Snapshot 对外只允许 `kc/snapshot` 与具名的 `dataset`、`access`、`service`、`console`、`client`、`cli` 一级包。
守卫同时检查参数、结果、字段、嵌入类型和别名；本包私有类型也不能借公开签名泄漏。
数据结构、数据结构的纯方法及装配回调有明确清单；权威和 handler 的工厂返回选定接口，
不允许导出的函数变量直通，也不允许 `Raw()` / `RootDir()` 后门。根包共享的六种私有纯 DTO
别名保留类型身份，并额外守卫其无私有状态、无实现方法。不可变常量不作为实现泄漏。
这些检查保留 `TreeReader`、`DirectoryReader` 等合法能力接口发现，不禁止接口断言。

SDK 与任务 CLI 归 Snapshot 模块，但层表分别按 app 客户端适配分类；仅向所属公开合同
依赖，根、Dataset、access、service、console 均不得反向依赖两包，SDK 不依赖 CLI。
`snapshot_module_test.go` 用真实图和正负间接依赖夹具冻结该方向；新增入口的 DTO、工厂
返回与签名依赖仍逐项登记，不豁免既有私有类型和具体实现隐藏断言。
叶归属守卫检查真实装配边界与 host 帮助映射，正反夹具保留身份注入、跨模块旅程、
私有任务和集成测试；装配边界重命名须同步守卫，不能静默绕过检查。
`internal/httpjson` 按 infra 分类，只承接通用运输，不能反向依赖领域合同或 KC 会话。

根 CLI 生产只装配任务声明、身份运输与宿主流程，嵌入操作表和执行链只留在测试宿主。
私有包级函数必须有其它生产声明中的真实引用；函数值和泛型装配算引用，声明名、自递归、
成员选择、局部同名与字符串不算。`init` 是运行入口，方法可能由接口隐式调用，均不作此检查。
根测试直接验证模块算法时，必须有实际 CLI、HTTP 或宿主装配边界；守卫跟随被调用的
helper 与子测试回调，未调用的 helper/closure 不算集成证据。共享执行器守卫仍扫描测试
宿主的真实 `Run` 绑定与 Server 的独立路由，迁移不删除同一 executor 的断言。

单登记权威守卫检查部署与HTTP公开形状，也检查私有传输DTO中的退休选择字段。
Home和Resources只能持有单个Dataset服务与Registry，不能改成map、slice或interface指针；
登记工厂只接Snapshot authority与ref。正负夹具验证守卫不误拦来源仓自身的归档能力。
