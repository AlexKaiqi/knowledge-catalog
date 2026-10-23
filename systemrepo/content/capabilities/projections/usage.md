---
entity: kc/capability/projections
aspect: usage
schema_ref: schema/system/capability-usage/v1
path_hint: capabilities/projections/usage.md
---
# 维护索引：使用与核对

先在明确的 Dataset 修订上诊断来源依据、声明覆盖与就绪状态，再选维护动作。`kc index describe --dataset <id> --revision <N>` 读取状态与编译依据，`kc index prepare --dataset <id> --revision <N>` 在维护授权下准备该修订；准备不重写对象，不推进来源仓 HEAD，也不撤销已接受版本。当前 Dataset 索引丢失后由维护者显式准备恢复，自动恢复尚未就绪；消费搜索本身不构建索引。

Dataset 发布先接受版本，索引在后台追：接受即对文件读取生效，检索就绪是同一版本上的第二个状态。`kc index --dataset <id> --revision <N>` 只回答能不能搜，是正常答案而非故障。

选择 [调用入口](entrypoints.yaml) 后，先运行对应叶子 help 查看当前版本的操作数、参数和输入形状。系统仓保留任务语义，完整命令合同由 help 提供。
