---
entity: kc/capability/search-object
aspect: usage
schema_ref: schema/system/capability-usage/v1
path_hint: capabilities/search-object/usage.md
---
# 搜索对象：使用与核对

先用 `kc show` 按名称和用途找到自己能消费的 Dataset，查看 `kc dataset show <id>`。在选定的发布修订中用 `kc schema list --dataset <id> --revision <N>` 与 `kc schema describe --dataset <id> --revision <N> --object <schema-id>` 核对结构、字段用途、声明支持与就绪，再构造查询。全文只给查询文本，过滤和排序按 Schema 字段定位符表达，不直接指定索引成员键。

`kc search --dataset <id> --revision <N> --query <文本>` 的命中保留来源仓、commit 与 Dataset 修订；后续 `kc read <对象URN> --dataset <id> --revision <N>` 沿同一修订回读，续页也沿首页依据。消费 Dataset 不需要源仓读取授权。缺声明、不支持、失效、未就绪与确实零命中分别判断，不能把失败或有限窗口写成不存在。

选择 [调用入口](entrypoints.yaml) 后，先运行对应叶子 help 查看当前版本的操作数、参数和输入形状。系统仓保留任务语义，完整命令合同由 help 提供。
