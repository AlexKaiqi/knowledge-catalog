# catalog/ 兼容入口

Dataset 文件组合、仓登记、固定版本、发布历史与生命周期的唯一实现已移到
[`snapshot/dataset`](../snapshot/dataset/README.md)。上层 KC 与其它文件服务使用该模块；
本目录只通过类型别名和函数转发保留历史 Go import，不拥有登记表或第二条发布路径。

公开的 `Catalog`、`Registry`、`KnowledgeSet` 和 pin 类型保持兼容。JSON、YAML、持久化
文件名、命令和错误码不因目录迁移改变。`KnowledgeSet` 是历史类型名，来源使用仓、
固定提交和文件路径，不能用它推导知识对象标识或 Schema 解释职责。

[`catalog/worktree`](worktree/worktree.go) 是宿主检出和同步消费者；其内部实现直接调用
`Snapshot Store` 的 Dataset 模块，不参与登记表权威写入。

## 兼容限制

`EmptyCatalogState` 保留原空值，Go 无跨包可赋值变量别名。给旧路径变量重新赋值不会
修改 `snapshot/dataset.EmptyCatalogState` 或新模块的空登记表默认值；新模块不反向调用
兼容入口。全仓没有依赖这类赋值的调用方，因此本轮保留值读取兼容，不引入双向同步或
第二份登记状态。

迁移债：位置为 `compatibility.go` 的 `EmptyCatalogState`；信号是旧公开可变变量无法
成为跨包别名；判定为当前无调用方的源兼容限制；处置为保留原空值并显式记录；发现外部
消费者依赖给旧变量赋值时，需 review 该初始化合同，不能用未同步的变量假装两个模块共享
权威。
