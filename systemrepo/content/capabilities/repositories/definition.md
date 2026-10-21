---
entity: kc/capability/repositories
aspect: definition
schema_ref: schema/system/capability-definition/v1
path_hint: capabilities/repositories/definition.md
---
# 接入对象来源

把来源接入当前服务，供后续发布对象或选择 Dataset 来源。尚无存储仓时申请托管仓；已经有自己的 lakeFS 仓和历史时连接自有仓，保留原来的仓身份与内容。

供给或连接、登记、授权是不同的决定。`kc create` 创建托管仓或验证并连接自有仓；`kc attach` 把已连接的仓登记到当前登记空间；`kc detach` 只解除登记。创建或连接不自动登记，登记也不把整仓交付给消费者。需要交付时再发布 Dataset，并由授权管理者授予消费权。

连接凭证由服务私有保管，不进入对象、登记说明或消费者的文件地址。凭证可以轮换，轮换须验证仍是原来源；不能通过换凭证重新连接另一个同名仓。托管供给可能分阶段完成，重试恢复原结果，不重复建仓或补回已撤销的权限。

登记本身完整保存或失败，不留下半个登记；这不表示先前已完成的远端供给也已回滚。来源暂时离线与登记记录丢失分别恢复，不能以空库存或读失败为由自动重建来源。

相关说明：[仓](../../concepts/repository/definition.md)、[登记空间](../../concepts/registration/definition.md)、[身份认证与授权](../../mechanisms/authorization/definition.md)。

继续阅读：[用法与恢复](usage.md)、[当前版本调用入口](entrypoints.yaml)。实际启用、就绪与授权由当前服务判断。
