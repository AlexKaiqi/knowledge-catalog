---
entity: kc/guide/connect-source
aspect: guide
schema_ref: schema/system/usage-guide/v1
path_hint: guides/connect-source/guide.md
---
# 接入一个来源

适用于需要把一个来源交给当前 KC 服务管理、尚未完成连接或登记的情形。先确认服务地址和已认证身份；供给或连接需要部署准入，登记需要该登记范围的管理权，读回还需来源读权。

## 选择来源并取得身份

先运行 `kc show`。如果目标仓已登记且身份正确，跳过创建和登记，直接核对读取；库存可见本身不证明有读权。

尚无仓、希望由平台分配存储时，申请托管仓：

```sh
kc create --name <name>
```

已经有自己的 lakeFS 仓和历史时，连接自有仓：

```sh
kc create --url <lakefs-repository-url> --credential-file <credential-file>
```

两种方式只选一种，替换占位符后执行。自有来源须处于部署允许的地址范围，凭证能验证已有非空仓。保留回执中的仓身份与创建或连接结果；此时还未自动登记。凭证交给服务私有保管，不能放进仓内说明、对象或共享脚本。以后凭证失效，由有权限的连接所有者执行凭证轮换，不以新的 create 请求替换既有绑定。

失败时先区分准入、凭证、来源不可用与身份冲突。修正可恢复条件后，以原名称和 Store 或相同连接请求重试；供给可能已完成一部分，不能把失败当成远端什么也没发生，更不能换名称创建第二个仓。绑定身份不符时停止并核对来源。

## 登记并读回

使用上一步回执中的身份；已经连接但未登记的来源也从这里开始：

```sh
kc attach --repo <repository-id>
kc show
```

核对返回的 `repositoryId` 与库存中的身份一致，再由获权主体读取仓自描述或一个已知对象。登记不创建仓、不改来源文件、不隐含发权，也不把整仓加入 Dataset。托管创建时的初始授权仅依部署策略应用一次，不保证之后仍有权限。

登记保存失败不会留下半个登记，也不会撤销已经完成的连接或远端供给。响应丢失先 `kc show` 核对；确需重试时沿用同一仓身份。来源未知或不可用时，先核对连接、在线状态与凭证，不重复建仓。来源在线但登记权威丢失或损坏时由管理者恢复原记录，不用重新初始化替代恢复。

## 解除与完成判断

只想停止把它作为本服务的来源时，先确认受影响的 Dataset 和消费者，再执行：

```sh
kc detach --repo <repository-id>
kc show
```

解除登记保留源文件、提交历史和 Dataset 已接受记录；后续读取会受当前成员准入限制，旧版坐标不能绕过解除。已经下载的字节不会被收回。回执丢失先查库存；误移除时确认原绑定仍可用，再 attach 同一身份。

接入成功的证据是正确来源身份已登记、获权主体可读取预期内容。创建成功、库存可见和索引就绪各自核对；需要分享时继续[向消费者交付 Dataset](../share-a-dataset/guide.md)，发布范围与消费授权仍是另外的动作。当前版本的参数和凭证文件形状见[接入对象来源](../../capabilities/repositories/usage.md)的调用入口。

所需能力：[进入和查看 登记空间](../../capabilities/inventory/definition.md)、[接入对象来源](../../capabilities/repositories/definition.md)、[理解对象类型](../../capabilities/schemas/definition.md)、[查询与管理授权](../../capabilities/permissions/definition.md)。
