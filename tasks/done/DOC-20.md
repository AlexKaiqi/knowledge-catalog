# DOC-20 · 观察记录与 M 面服务合同

- [x] DOC-20 认领（2026-09-29 复核批 R5）：两个反例测试可执行并全绿——`TestLatestOnlyRecordLossAndExpiryFailDistinguishably`（latest-only 平台保留记录：保留期内重读返回原观察原 basis；丢失=恢复故障 `TEMPORARY_UNAVAILABLE`、到期=能力耗尽 `CAPABILITY_UNSATISFIED`，可区分且都明确失败，不返回当前值或空历史；B-22 gap 收口）+ `TestBusinessNullContrastsPlaceholderAndNeverFallsBackToScan`（业务空值带合法 basis=成功观察；占位空缺 basis 失败关闭；取值失败只传播错误、不降级声明扫描）。合同写清：liveaccess/README.md 观察保留与失败关闭段。不改运行实现与公开形状 ✓（Non-Goals 遵守）。

**问题与影响：** 当前有 `StateLookup`、动态投影与同 basis hydrate，但它们不能共同证明新鲜度、
历史保留和当前源授权。现有 State catch-up 会跳过已有观察，活动记录替换不等于历史保留，
旧 Dataset 声明的动态维护也不能由 HEAD 对账覆盖。单 key refresh 只收窄 lookup，不能据此
宣称观察存取和索引写入满足增量成本。

**Goal：** 按[动态物化](../../doc/design/liveaccess.md) §2.6/§6/§9 定义可信 State 消费，并将
方向性任务关联到已有断言或明确缺口。新观察按声明取值，同依据重读使用可靠保留的观察；
控制恢复归[投影控制](../../doc/design/discovery.md)，授权归[权限设计](../../doc/design/permissions.md)。

**Non-Goals：** 不改运行实现或公开形状，不执行验证；不新增 Snapshot/live Store、Catalog cut、
对象 ACL、通用消息物化器或 Stream 查询入口。介质边界依[介质角色](../../doc/architecture.md)，
统一访问职责依[服务架构](../../doc/design/service.md) §4.7。

**不变量与方案：** 延续 `D-01`、`V-01`、`P-01`、`IX-04`、`KS-02` 及 `AUTH-01`–`AUTH-03`。
选定分别证明覆盖/时效/重读/授权，State 阶段提供有界观察保留；否决用 complete 代替实时保证、
用最新值补旧依据、用后台身份代替消费授权、只追 HEAD，以及单 key 变化的隐式全库重建。
默认时效、容量/保留值和共享授权装配仍待选定；形状由公开类型、包 README 与 Conformance
承接，不写入设计正文。

**设计与用例产物：** 动态物化 U1–U9；验证体系 B-19–B-27；场景 `product/materialization/*`
从 owner 提取方向任务，以具名 evidence 关联实际断言，缺口写入 `_views.yaml`。未创建假定
能力已实现的场景状态；未新增执行结果或完成结论。

**完成标准：** 两个反例可执行——(1) 来源声明 `latest-only` 时，观察记录丢失后历史读取必须明确失败，不得静默返回当前值或空值；(2) 缺少取值入口时 Bound State 读取必须失败关闭，不得把信号载荷当值、不得降级为扫描、不得返回占位空值。运行时的重读能力与保留期上限按声明校验。

依据：[动态物化](../../doc/design/liveaccess.md) §6、[介质角色](../../doc/architecture.md) §2、[服务架构](../../doc/design/service.md) §4.7、[观察类型](../../object/observation.go)、[Serving 端口](../../liveaccess)。
