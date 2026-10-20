# Knowledge Catalog 文档

这套文档从愿景出发重新梳理 Knowledge Catalog：它要做成什么，由哪些概念组成，架构怎样
对应代码，每块能力走到了哪一步。重写完成后删除旧的 `docs/`。

## 阅读顺序

1. [产品书](product.html)：面向接入方与使用方的单文件 HTML，以清河茶铺的订单服务告警串起价值、架构、Agent 入门、消费、供给、更新、扩展和边界。
2. [愿景](vision.md)：多团队协作与生产两个前提，Agent 的 Spring Boot，discover、access、wield。
3. [核心概念](concepts.md)：读懂其余文档需要的全部名词。
4. [架构](architecture.md)：五层结构、代码目录对应、三条运行路径。
5. 设计：按需要读对应的一篇。
6. [路线](roadmap.md)：能力总览，已有什么，接下来改什么，哪些还没走到。

## 设计

| 篇 | 回答什么 |
|---|---|
| [Snapshot](design/snapshot.md) | 仓、版本、条件提交、登记与恢复 |
| [Dataset](design/dataset.md) | 组合、发布、消费与交付方式 |
| [权限](design/permissions.md) | 身份、三种资源、为什么不做对象级权限 |
| [对象](design/object.md) | 对象模型、对象引用、Schema、Writer、读取、系统仓 |
| [对象版本与读写（待重构）](design/object-versioning.md) | 对象状态的比较范围、条件写与无变化写入、固定坐标读取、历史及 Dataset 的版本关系 |
| [插件](design/plugins.md) | 插件与 AOP 的对应、作用域、建议的目标 |
| [发现](design/discovery.md) | 搜索、完整性、翻页、声明式索引维护 |
| [索引物理布局（待重构）](design/index-physical.md) | 构建值与查询结构的复用条件、成员增量、发布保护与回收 |
| [LiveAccess](design/liveaccess.md) | 外部取值、逐值失败、身份传递、证据 |
| [Affordance](design/affordance.md) | 对象与系统的说明、命令行规则 |
| [服务](design/service.md) | 请求入口、跨模块任务、恢复、证据、部署 |
| [治理](design/governance.md) | 反馈、工作分支、条件合并、Hook |

## 工程

| 篇 | 回答什么 |
|---|---|
| [测试与验证](engineering/testing.md) | 验证入口、用例位置、产品价值验证 |
| [工程质量闭环](engineering/quality.md) | 六层质量控制、三个出口、耦合度量 |
| [测试目录](engineering/test-catalog.md) | 旅程用例、编号规则、验证入口登记 |
| [架构不变量](engineering/architecture-invariants.md) | 架构不变量、禁止观察与守卫映射 |

## 进度标记

| 标记 | 含义 |
|---|---|
| 已有 | 参考实现已经具备，有测试保护。是否适合某个部署规模另说 |
| 近期 | 接下来就要改进或正在改，属于当前重构与收尾 |
| 未到 | 愿景里有，但我们还没走到，暂不投入 |

每篇设计末尾有一张进度表，[路线](roadmap.md)汇总全部。

## 写作约定

- 先讲目标，再讲进度。目标写成“应当是什么”，进度只放在进度表里。
- 产品书按“价值与主案例 → 架构 → Agent 入门 → 消费 → 供给 → 更新 → 扩展 → 边界”展开；机制取舍归设计篇，进度链接[路线](roadmap.md)，不复制进度表。
- 一个概念只在[核心概念](concepts.md)里定义一次，其它地方直接使用。
- 例子以“订单服务告警”为主线。产品书用清河茶铺贯穿这一场景，数据分析、素材交付等只作扩展示例。
- 不写字段全集、错误码和命令全集。需要时链接代码或 `kc help`。
- 待决定的设计明确标“待确认”，不写成既定目标。
