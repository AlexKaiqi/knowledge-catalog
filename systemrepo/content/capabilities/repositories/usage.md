---
entity: kc/capability/repositories
aspect: usage
schema_ref: schema/system/capability-usage/v1
path_hint: capabilities/repositories/usage.md
---
# 接入对象来源：使用与核对

先确认目标服务、当前身份和接入权限，核对 `kc show` 中是否已有目标仓。已有正确连接和登记时直接使用，不重复创建。

| 要完成的事 | 入口与结果 | 失败后核对 |
|---|---|---|
| 获得新托管仓 | `kc create --name <name>`；保留返回的仓身份和供给结果 | 按原名称和 Store 恢复，不换名再建；失败可能发生在远端已创建之后 |
| 连接自有 lakeFS 仓 | `kc create --url <url> --credential-file <path>`；验证现有非空仓并保存服务私有连接 | 先修正准入、来源可用性或凭证，再以相同来源请求恢复；绑定身份冲突时停止，不能收养同名仓 |
| 登记已连接仓 | `kc attach --repo <id>`；用 `kc show` 核对登记身份 | 响应丢失或并发冲突时先查库存，再以同一身份重试，不重复供给或发权 |
| 解除登记 | `kc detach --repo <id>`；用 `kc show` 核对已移除 | 先查实际登记结果；误移除时确认原绑定可用，再 attach 原身份 |

`--repo` 使用回执中的仓身份，不是 URL 或本机目录。以上占位符换成实际值，完整参数和凭证文件形状见[调用入口](entrypoints.yaml)中的叶子 help；完整操作顺序见[接入一个来源](../../guides/connect-source/guide.md)。

凭证文件用于首次授权服务连接，服务随后私有持有凭证，不向消费者下发。失效时由有仓管理权的连接所有者走连接管理 API：`GET /snapshot/v1/connections/{repository}/connection` 核对绑定，`POST /snapshot/v1/connections/{repository}/connection:rotate` 轮换凭证；路径中的仓身份须作为一个路径段编码。轮换先验证原绑定，再替换秘密，失败保留原绑定与凭证。当前这项管理不在 `create`、`attach`、`detach` 三个 CLI 入口中；不要用 create 重试替代轮换，也不要把新秘密写进对象或公开回执。

登记仍在但来源不可读时，核对来源在线状态、连接凭证和当前读权；登记权威缺失或损坏时，恢复原登记记录。两种情况都不能靠新建同名仓恢复。初始化是显式管理动作，重启服务只恢复原状态。

解除登记保留源仓文件、历史和已接受的 Dataset 发布记录，但引用它的后续访问会受当前登记准入限制，旧版本也不能绕过。它不删除源数据，也不能收回消费者已经下载的字节。接入完成后，以目标身份可见、获权主体能读取为核对结果；向其他人交付仍需另外发布范围和发权。
