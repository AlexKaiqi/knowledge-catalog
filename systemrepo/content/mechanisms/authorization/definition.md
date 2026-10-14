---
entity: kc/mechanism/authorization
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: mechanisms/authorization/definition.md
---
# 身份认证与授权

认证确定实际调用主体，授权判断该主体能否执行指定动作；登记空间、Repository 与 Dataset 各有自己的授权范围。

## 调用主体（Principal）

经 Server 认证的实际请求身份，可以是用户、Agent 或服务账号。

Agent 的代理关系不能自行合并权限。所有业务 kc 请求经 Server 并携带显式 principal，local 夹具身份模式只用于受控环境。

## 权限（Permission）

对当前主体、动作与资源范围的准入和交付判断。服务动作、Repository 与 Dataset 分别授权。

登录、库存发现、知道 pin 或后台可见都不授予其它能力。Dataset 交付限定发布范围，跨仓引用不扩权，外部资源仍需独立授权。

相关说明：[登记空间](../../concepts/registration/definition.md)、[仓](../../concepts/repository/definition.md)、[Dataset](../../concepts/dataset/definition.md)、[CLI](../../interfaces/cli/definition.md)。
