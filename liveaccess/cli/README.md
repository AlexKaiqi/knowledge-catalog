# LiveAccess CLI 说明

`Affordance()` 拥有本插件的取值协议、适用条件、结果影响、认证及独立审计承诺。
它只返回静态数据，不依赖执行服务、仓配置、来源 adapter 或后台状态。

本包还拥有既有 `binding show`、`access`、`invoke` 的 `Commands()`、帮助、用法校验和
`Shape()`，不更名业务任务。`New(Client, Context)` 采用宿主注入的已认证 typed transport；
请求复用已有 `KnowledgeScope` 与 JSON 字段，宿主提供已经处理的 Dataset 固定依据，
本包不重做身份解析、pin 文件解析或路由。`read` 和独立审计入口的说明只链接已有任务。

统一 `kc plugin liveaccess help` 使用现有插件说明任务读取已编译说明。实际访问及审计
经 Server 认证、授权后执行；静态说明不证明当前 Repo 已配置本插件，亦不证明来源
和证据服务已经就绪。
