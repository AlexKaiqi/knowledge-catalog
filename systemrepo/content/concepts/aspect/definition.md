---
entity: kc/concept/aspect
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/aspect/definition.md
---
# 切面

同一个对象某一方面的对象，是独立维护的单元。结构、说明、质量可以属于同一对象的不同切面。

修改一个切面不改其它切面。消费者可以读取单个切面或完整对象。切面划分不产生独立正文读 ACL。

插件实现声明、人工补充与衍生说明都可以作为普通切面或成员维护。衍生性质由来源和真实输入依据表达，不增加特殊切面种类。

## 切面与成员的切分

切面按"谁维护、多久变"划分：独立修改、独立归因、可能由不同团队维护的部分做成切面，系统结构与说明、质量分开正是这个理由。切面划分不产生独立正文读 ACL，不要用切面代替授权边界。

集合里的元素是否需要单独寻址和更新，决定它做成成员还是普通字段。需要按稳定键单独读写、单独标失效或单独统计的，做成成员；只是一起读写的普通字段留在切面内即可，不必提升为成员。

相关说明：[对象](../object/definition.md)、[成员](../member/definition.md)、[对象地址](../address/definition.md)、[身份认证与授权](../../mechanisms/authorization/definition.md)。
