---
entity: kc/capability/datasets
aspect: definition
schema_ref: schema/system/capability-definition/v1
path_hint: capabilities/datasets/definition.md
---
# 读取文件与组合 Dataset

已知仓、固定版本和文件路径时，可以直接下载单文件。需要组合交付时，选择确定版本的来源文件、定义交付目录并发布 Dataset；消费方读取已接受范围，明确采用更新。

相关说明：[Dataset](../../concepts/dataset/definition.md)、[固定消费依据](../../concepts/pin/definition.md)、[登记空间](../../concepts/registration/definition.md)。

继续阅读：[用法与恢复](usage.md)、[当前版本调用入口](entrypoints.yaml)。实际启用、就绪与授权由当前服务判断。
