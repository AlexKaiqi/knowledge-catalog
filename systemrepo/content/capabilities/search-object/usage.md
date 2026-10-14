---
entity: kc/capability/search-object
aspect: usage
schema_ref: schema/system/capability-usage/v1
path_hint: capabilities/search-object/usage.md
---
# 搜索对象：使用与核对

先了解 Schema 的字段结构及其 access 声明支持的访问方式，再构造查询。保留固定命中坐标与分页依据。无可用投影时明确诊断能力或就绪问题，不能把失败或有限窗口写成不存在。

选择 [调用入口](entrypoints.yaml) 后，先运行对应叶子 help 查看当前版本的操作数、参数和输入形状。系统仓保留任务语义，完整命令合同由 help 提供。
