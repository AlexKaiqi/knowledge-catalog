# LiveAccess 插件重构

## Goal

1. 代码与 CLI 扩展放在独立目录。
2. 插件有真实调用与结果采用。
3. 对象读取无感取值；外部值失败只在原位置说明，其他值保留。
4. 自动传递适用认证。
5. 独立审计复核当次值、失败及请求。

Repo 配置，Server 按仓执行；Dataset 复用来源配置。

## Affordance

- `kc plugin liveaccess help`：说明协议、适用条件、结果影响与认证、审计承诺。

## 契约与承诺

### 取值协议（Go）

- 调用：`LookupState(ctx, StateLookupRequest) → (StateObservation, error)`。
- 请求：`Binding`、`SchemaRef`、`Origin`、`Identity`、`Trace`、`RequestID`。
- 成功：`Value`、`Basis`。
- `Basis`：来源代际、一致性、观察时间及可选源版本/水位。
- 失败：`error` 及原因。

### 读取结果

- 成功：对象层在原位置呈现实际值。
- 空值：合法 null 保持 null。
- 索引可采用 LiveAccess Member 的值与原观察依据。
- 失败：仅该位置呈现说明，其他值保留；不以旧值或空值掩盖失败。

### 身份与审计

- 自动传递适用认证，限可信目标；地址/重定向不扩权，凭证不入对象、日志或审计。
- 访问记录：主体、时间、位置、requestId。
- Snapshot：固定版本/位置，承诺期内可复核。
- LiveAccess：保留当次采用值或失败、固定声明及原依据。
- 证据独立授权查询，源变后仍可复核。
- 留证可靠后交付；丢失与到期可区分。

## 方向性用例

验收 U13～U20；保留旧 U1～U12。规格见[用例库](../../.data/scenes/service-initialized/source-repositories-configured/repository-attached/_specs/liveaccess.yaml)，证据见[用例表](test-catalog.md#liveaccess-重构验收)。

### U1：查询当前失败的服务，包括零命中

### U2：区分业务空值、取值失败与没有取值能力

### U3：通知丢失、重复或乱序后仍能恢复

### U4：来源只能给最新值，仍能复核已交付的观察

### U5：HEAD 前进后，Dataset 仍按原声明服务

### U6：后台能看到，消费方未必能看到

### U7：访问地址变化不扩大凭证范围

### U8：持续更新不会掩盖维护成本和失败范围

### U9：先看当前态，再看事件窗口

### U10：同一 Dataset 聚合来源的索引和访问能力

### U11：更换访问实例不改来源声明

### U12：旧声明兼容与未匹配版本明确区分

### U13：独立目录

### U14：插件接入

### U15：混合值读取

### U16：身份传递

### U17：当次结果复核

### U18：证据可靠性

### U19：插件说明

### U20：Member 索引
