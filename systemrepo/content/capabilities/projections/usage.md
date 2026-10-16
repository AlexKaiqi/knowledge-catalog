---
entity: kc/capability/projections
aspect: usage
schema_ref: schema/system/capability-usage/v1
path_hint: capabilities/projections/usage.md
---
# 维护查询投影：使用与核对

先在明确的 Dataset 修订上诊断来源依据、声明覆盖与就绪状态，再选维护动作。`kc operations projection describe --dataset <id> --revision <N>` 读取状态，`kc operations projection sync --dataset <id> --revision <N>` 在维护授权下准备该修订；同步不重写对象，不推进来源仓 HEAD。当前 Dataset 索引丢失后由维护者显式同步恢复，自动恢复尚未就绪；消费搜索本身不构建索引。

选择 [调用入口](entrypoints.yaml) 后，先运行对应叶子 help 查看当前版本的操作数、参数和输入形状。系统仓保留任务语义，完整命令合同由 help 提供。
