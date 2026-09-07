# 已有部署替换实例

前态是已初始化且已有业务操作的部署。正式配置与 Server 旅程由 `TestDeploymentSurvivesInstanceReplacement` 维护：保留远端 Catalog Git 与独立耐久状态，删除实例缓存后重建 Server，观察同一 Catalog、成员/Workspace、授权/Gate、Writer receipt、ControlState 与原始证据。`TestDeploymentMissingDurableStateFailsClosed` 验证丢失必需状态不会变成空的新部署。

这里不以 embedded Home 副本替代正式恢复证据，也不重复创建业务 Snapshot。用户后续接着处理原任务。
