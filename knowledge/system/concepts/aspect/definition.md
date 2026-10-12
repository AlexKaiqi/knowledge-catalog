---
entity: kc/concept/aspect
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/aspect/definition.md
---
# 切面

同一个对象某一方面的知识，是独立维护的单元。结构、说明、质量可以属于同一对象的不同切面。

修改一个切面不改其它切面。消费者可以读取单个切面或完整对象。切面划分不产生独立正文读 ACL。

Interface 实现声明、人工补充与衍生说明都可以作为普通切面或成员维护。衍生性质由来源和真实输入依据表达，不增加特殊切面种类。

相关说明：[对象](../object/definition.md)、[成员](../member/definition.md)、[知识地址](../address/definition.md)、[身份认证与授权](../../mechanisms/authorization/definition.md)。
