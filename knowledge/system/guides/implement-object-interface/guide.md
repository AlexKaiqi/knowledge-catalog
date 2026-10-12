---
entity: kc/guide/implement-object-interface
aspect: guide
schema_ref: schema/system/usage-guide/v1
path_hint: guides/implement-object-interface/guide.md
---
# 为对象实现共同能力

1. 明确对象边界、能力用途和结果依据，选用已发布的 Interface 合同。
2. 将所需实现 Schema 通过正常发布或固定来源采用到对象所在仓，保持普通 Schema 引用的同仓解析。
3. 根据合同准备对象实现信息，推荐放在 interfaces 集合切面的稳定成员中；材料组成留在领域定义，内部文件与对象复用已有引用。
4. 通过 Writer 发布并精确读回对象身份、实现和来源。结构校验与能力实现的语义检查分别完成。
5. 部署方装配支持精确能力合同与实现 Schema 结构的处理器；以实际调用及一致性证据确认能力，不把知识发布当成服务就绪。

独立扩展合同发布到普通仓；内置 LiveAccess 合同随平台出版。运行时不能追加系统仓。多个相同能力实现需要稳定标识和明确选择，后续更新遵守 Writer 并发检查。

## LiveAccess 实现示例

`services/payment/interfaces/live.yaml` 保存支付服务的一个实现成员。先将系统出版物中的 `schema/implements.live-access/v1` 同内容采用到业务仓；`interfaceRef` 中的发布 commit 换成已固定且允许读取的系统版本。

```yaml
---
object_id: service/payment
aspect_name: interfaces
member_key: live
kind: Member
schema_ref: schema/implements.live-access/v1
path_hint: services/payment/interfaces/live.yaml
---
interfaceRef: kc://kc/system@<发布commit>/interface/live-access/v1
runtime: payment-runtime
protocol: resource-access/v1
origin: https://payment-runtime.example.org
operations:
  read:
    call: payment.currentState
  invoke:
    call: payment.restart
```

该成员提供声明，公共处理逻辑由 LiveAccess 处理器完成。读取与操作分别授权，资源凭证不写进知识。具体调用合同查看同版本 help。

成功证据：对象实现能按固定合同解析，实际处理返回符合合同的结果及依据，权限拒绝和依赖缺失明确可辨。

相关说明：[对象 Interface](../../concepts/object-interface/definition.md)、[首次发布](../first-publication/guide.md)、[使用对象能力](../use-object-interface/guide.md)。
