# APP-CORE-01 · Typed Application Core

- [x] 把应用用例从 CLI flags/HTTP transport 中抽离为 typed executor。所属阶段：M5 的结构纠偏；
  方向由 `doc/design/service.md` 的 `API-01` 与 §1.2 选定，待 `DOC-14/16/17/18/19`
  闭环后认领。

**问题与影响：** CLI 与 HTTP 已要求调用同一 executor，但参考实现仍让大量应用编排依赖
`map[string]FlagValue`、CLI dispatcher 和 transport 邻近状态。Repository、Workspace、Pin 等
标识在应用边界继续以裸字符串流通，授权、固定 basis、Reader/Writer、交付链和错误映射容易随
入口分叉。现有 `API-01` 证明两个入口复用执行路径，不足以证明应用核心独立于 transport。

**处理范围：** 先抽一条 READ/SEARCH 纵切，再迁 Writer、Governance 和 Operations；每个用例以
typed request/result 表达，标识在 transport 边界解析为 owner 包命名类型。CLI 只解析 argv 和
渲染输出，HTTP 只转换 wire DTO；应用核心只依赖协议服务与注入端口。为新应用包补分层归属和
import 守卫。不改变公开 CLI/HTTP surface、授权、固定 basis、错误码和结果语义。

**完成标准：** CLI 与 HTTP 的同一请求进入同一个 typed executor 并逐观察等价；应用核心不
import CLI parser、HTTP registry、具体 authority/retrieval adapter 或部署配置解析器；生产应用
代码不接收通用 verb/flags map；命名标识不可在编译期互换。迁移期间每条纵切先有失败反例，再
以公开 surface、应用合同和 `make test-boundary` 验收。

依据：[服务架构](../../doc/design/service.md) `API-01`/§1.2、[分层设计](../../doc/architecture.md)、
[CLI 交互](../../doc/design/affordance.md)。
