---
entity: kc/mechanism/object-interface-dispatch
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: mechanisms/object-interface-dispatch/definition.md
---
# 对象能力分派

在明确的对象版本和消费范围内解析对象实现，选择支持该合同与实现形式的处理器，再执行消费者请求。

系统固定对象实现、Interface 与 Schema 依据；注册关系固定能力合同和实现结构，字段查询声明变化不改变处理器的结构支持；完整 Schema 与对象依据仍记录，而不是按同名字符串猜测兼容。存在多个实现时由消费者明确选择。缺依赖、缺处理器或不兼容时明确拒绝，不扫描仓或暗换版本。

运行请求保留可信调用主体和关联上下文。读取声明不授予操作权限；具有写效果的操作需显式授权。访问目标来自固定声明，消费者输入不能覆盖目标或身份。结果按各能力合同保留文件版本、观察依据或相应执行依据。

内置 LiveAccess 承接当前状态与已声明操作。其他能力通过对象合同、实现 Schema、处理器和一致性证据独立接入。发布合同不等于处理器已启用或当前主体获权，系统仓目录也不是动态插件安装入口。

相关说明：[对象 Interface](../../concepts/object-interface/definition.md)、[LiveAccess 合同](../../object-interfaces/live-access/definition.yaml)、[动态访问](../dynamic-access/definition.md)、[身份认证与授权](../authorization/definition.md)。
