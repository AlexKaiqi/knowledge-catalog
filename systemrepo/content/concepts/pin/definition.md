---
entity: kc/concept/pin
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/pin/definition.md
---
# 固定消费依据

解析 Dataset 得到的固定文件范围与来源版本向量。公开类型为 ResolvedDataset，用户侧简称 pin。

pin 固定读什么，新的请求仍按当前权限判断谁能读。已经打开的范围不自动跟随源仓 HEAD 或新服务版，采用更新需要重新解析。

相关说明：[Dataset](../dataset/definition.md)、[版本](../version/definition.md)、[身份认证与授权](../../mechanisms/authorization/definition.md)。
