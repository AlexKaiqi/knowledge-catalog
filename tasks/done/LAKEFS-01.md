# LAKEFS-01 · LakeFS + 对象存储规模 Snapshot

- [x] 本轮完成：接入 LakeFS Snapshot authority，并以共享能力合同证明 provider 可替换；同一投影控制器分别处理静态 Snapshot Aspect 增量和动态 Recipe/Binding observation。LakeFS RepositoryContract、tree-provider 对拍、并发发布锁、预签名数据面和双 lane 控制器用例均已执行通过；`make check-docs`、`make check-validation` 与 `make test` 全绿。真实 LakeFS/COS 容量及长期故障资格仍归 SCALE-01，不由本项宣称。

**Goal：** LakeFS 以 Graveler/Refs 作为版本控制面、S3 兼容对象存储作为直传数据面，实现与 Dolt/Gitea 相同的固定版本、expected-old CAS、历史和增量语义；更换 binding 不修改 Catalog、Reader、Writer、Index 公开语义。

**Non-Goals：** 不向 Client/Collector/CI 暴露预签名地址或物理对象地址；不以 Actions/Webhook 作为正确性来源；不把动态 observation 写入 Snapshot；不为适配 LakeFS 放宽 CAS；不配置会删除已发布历史对象的 retention；不引入未经批准的 SDK。

**不变量：** `A-01`、`V-01`、`W-01`、`W-02`、`P-01`、`PC-01`、`E-01`、`IX-01`、`IX-04`。缺失规模必需能力必须在装配期失败关闭；静态变化只解释固定 `{from,to}`，动态变化只经 notice-and-pull 取得 observation basis。

**接口与验收：** 公开形状由 `snapshot.Store`、provider 能力合同和 `index.Controller` 拥有。LakeFS adapter 只使用 release-pinned REST 形状与标准库 HTTP；运行 Snapshot/Writer 共享合同、跨 provider 对拍、静态 Aspect 与动态 Recipe 控制器用例、架构守卫及文档检查。未配置真实 LakeFS/COS 的环境集成不得冒充生产容量资格。
