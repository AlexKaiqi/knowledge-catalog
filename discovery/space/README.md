# discovery/space

③ Discovery 的 Dataset 修订命名空间接缝。`Member` 以固定的 `DatasetPin` 和宿主的
`MemberLookup` 创建只读对象视图：对象正文保持真实来源仓与提交，Schema 和 `index`
成员从选入同一 Dataset 的来源汇总。未选入的声明不会进入编译，依赖仓不成为检索来源。

这个视图供声明式索引的显式构建及固定依据查询使用，不是新的 Snapshot Store，不打开
服务或引擎。维护的有界对象页在既有源维护端口上过滤；查询只读 Schema 命名空间和
投影保留的精确声明地址，不枚举源对象。`DatasetScope` 为插件已有窄维护口传递同一
固定输入；`StoreDigest` 把完整范围和来源版本加入派生身份，防止不同 Dataset 的说明
或范围共享检索依据。

`ReadMany` 与单点 `Read` 使用相同的命名空间和选入单位语义：Schema 按固定真实来源
分组批读，部分选入的对象沿既有按路径读取能力回读，未选入对象不出现在批次中。
批次只处理调用者列出的身份，不补扫来源或扩展文件范围。

一个 Dataset 的物理 corpus、打分、候选回读和观察空间由
[`declarativeindex/projection`](../../declarativeindex/projection/README.md) 持有；本包不持
索引或观察状态。
