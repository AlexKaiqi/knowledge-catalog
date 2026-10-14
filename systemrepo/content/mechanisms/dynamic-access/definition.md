---
entity: kc/mechanism/dynamic-access
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: mechanisms/dynamic-access/definition.md
---
# 动态资源访问

LiveAccess 是访问当前状态与已声明操作的对象 Interface。版本化实现声明说明怎样访问外部资源，每次取值另有观察依据。

## 动态访问声明（Binding）

声明某个对象单元的动态值如何从外部 runtime 取得。它说明取值模式、访问入口与操作，随对象声明版本化；LiveAccess 实现提供访问原点和操作映射；历史 Schema origin 与 ResourceDescriptor 继续兼容解释。

固定版本只固定声明，不冻结外部值。凭证、游标与运行进度留在外部运行面；动态取值经资源访问与 Serving，保留声明和观察两种依据。当前普通读取支持 State 取值，事件窗口与持续订阅尚未选定，不能由声明中的 Stream 模式推导已有流处理能力。

## 观察（Observation）

外部动态资源在一次取得时得到的值及其观察依据。

观察依据与仓提交分别记录，声明版本不会冻结动态值。来源可能只提供最新值，同一观察能否重读还取决于来源能力、保留记录与当前源授权。缓存不授予访问权；将观察保存为对象，需要显式选择内容并经 Writer 发布。

## 资源访问（Resource Access）

通过声明的 origin 和对象身份向独立 runtime 请求动态值或执行资源操作。

对象读权不授予业务数据或执行权限。调用前确认声明、操作形状、源侧准入与委托条件；资源凭证不写入正文。

相关说明：[对象能力分派](../object-interface-dispatch/definition.md)、[LiveAccess 合同](../../object-interfaces/live-access/definition.yaml)、[Schema](../../concepts/schema/definition.md)、[版本](../../concepts/version/definition.md)、[身份认证与授权](../authorization/definition.md)。
