# ARCH-CLIENT-SESSION-01 · 客户端登录与会话归位

## 目标

`client/session/` 唯一拥有服务地址选择、登录流程、会话存储、逐请求重读、刷新与注销。
CLI 与 kcfs 使用同一客户端能力，不为会话加载服务端应用执行和部署资源。

## 依据

- [服务](../../doc/design/service.md)、[权限](../../doc/design/permissions.md)。
- [架构](../../doc/architecture.md)、[工程质量闭环](../../doc/engineering/quality.md)。
- 实施时核对 `client/README.md`、`cli/README.md`、公开会话入口与现行登录回归。

## 范围

从 `cli/remote_login.go`、`cli/remote_session.go`、`cli/public_session.go`、
`invocation/session_helpers.go` 及实际引用链归位会话职责。CLI 保留参数校验、显示和打开浏览器等宿主交互；
会话包使用明确参数或窄交互端口，不接收整个 Invocation 或 CLI flags。
会话包消费 `client/auth` 的认证运输与身份 SDK；根 `client` 组合模块 SDK 并保留原公开别名与入口，通过已有认证/会话存储端口接受注入，
不反向 import 会话实现；模块 SDK 也不依赖会话。
既有公开 CLI 会话入口如需保留，只委派唯一实现；不保留私有迁移转发。

## 验收

- 服务地址优先级、按服务隔离、旧会话的既有兼容读取、原子保存和权限模式均有明确预期；
  保存失败不得返回已成功持久登录。
- local 与 token 配对保持现行身份合同；空身份、混合证明、凭据重定向拒绝，切换服务不发送旧服务凭据。
- kc 与长运行 kcfs 在逐请求重读、过期刷新、刷新失败和注销后行为一致，更新会话不移动已固定的文件视图。
  现有并发刷新合同保留，并执行对应 race 回归。
- 架构守卫证明会话包及依赖闭包不进入 `cli`、`invocation`、`home`、`server` 或具体 provider；
  CLI 的剩余服务端引用须来自部署或宿主职责，不来自会话辅助函数。
- 会话算法测试随实现迁移，真实登录 argv、broker 与 kcfs 集成仍在实际入口验证；
  既有公开符号保持，旧历史材料不冒充当前登录路径覆盖。
- 执行 `ARCH-LAYOUT-01` 的共同实施与验收约定；同步包说明、服务篇的代码位置及登录测试登记。

## 前置

可提前做，无整项任务前置；不等待 Object、索引、Dataset 或 Home 重构，也不成为这些行为重构的前置。
开工前核对登录与会话文件无重叠在途修改，尤其 `invocation/session_helpers.go`：
同文件中的消费范围与授权正由主线调整时，先让该批行为改动稳定，再接入会话迁移，不同时编辑。
只有一条执行线时主线优先，可提前不等于必须先做。
本任务交付后，仓管理仍须达到自身的 Object/Index 后置条件，再接手共享辅助文件中剩余的供给职责。

## 不做

不改认证协议、身份规范、发权规则、会话保存格式或已选定的服务地址优先级；不更换凭据存储产品，
不把浏览器/终端交互写入模块 SDK，不借迁移实现服务端会话。

## 迁移清单

现有会话实现位于 CLI 的 remote_login/remote_session、服务地址与落盘辅助位于 invocation；
目标 owner 为 `client/session`。生产消费者是 kc 的远程调用、kcfs 的远程文件客户端，以及两个既有
公开 CLI 会话入口。公开 Invocation 配置/路径入口只保留兼容委派。通用私有 JSON 原子落盘归
`internal/jsonfile`，供会话及既有服务端调用复用，不使服务端持有会话规则。
登录配对、浏览器 PKCE/PAR、token 验证、刷新、注销与按服务隔离保持原合同；CLI 保留显示及打开浏览器。
纯存储/PKCE算法测试归会话包，真实 argv、Server broker 与挂载集成留在入口。当前无并行执行者；
`invocation/session_helpers.go` 的在途改动属于已验证的固定读取与来源上下文，本次只移动其配置/路径辅助区，
不改该文件中的读取、授权、供给与来源逻辑。耦合改前口径为 depth=2、81 units、14.31%。


## 认证运输归位

直接依赖 SDK 聚合根会使 invocation 的兼容地址入口连带加载所有模块 SDK；同口径度量实测
82 units、14.39%，超过改前 14.31%。因此将认证运输、认证/会话存储端口与身份 HTTP SDK 的实际
算法归 `client/auth`，原根 client 保留公开类型别名与方法兼容，继续组合模块 SDK。
会话通过 Configure 输出已建立登录态的运输配置，CLI 在宿主组合根 SDK；不导出 CLI 状态或私有文件格式。
83 units、14.06%，无 >=0.05 不稳定度漂移；未修改单位划分规则或门禁阈值。

本轮已显式写入收紧后的耦合基线（83 units、14.06%），新增单位持有实际算法与测试；阈值及度量规则不变。

完整架构守卫进一步发现 invocation 的兼容配置入口不能进入 client 子树；按原禁止依赖断言修复，
未追加豁免。服务地址与路径算法归无运输/凭据依赖的 internal/clientconfig，会话与旧公开配置入口
共同调用；登录、验证、凭据落盘与刷新仍唯一属于 client/session。身份别名直接使用既有 kernel owner，
避免经 observability 引入无关 LiveAccess 与 provider 闭包。最终 84 units、13.58%，较最初 14.31% 收紧；
client/session 是本批新增单位，其最终依赖归位后重新记录基线，既有单位和原门禁规则不放宽。

## 本批验证记录（2026-10-21）

源码冻结候选：完整架构 `20261021T011448Z-0b2873e6bf2f` 719 pass / 0 skip；
会话选定回归 `20261021T011449Z-2771308c946d` 35 pass / 0 skip；
稳定 race `20261021T011516Z-85afd876f82a` 37 pass / 0 skip；
make test `20261021T011508Z-b98740449ccf` 18 pass / 0 skip；上述 sourceChanged=false。
make quality 与 make check-docs 通过，最终耦合 84 units / 13.58%。
`20261021T011447Z-2043e59be8ae` 的 race 虽通过，但启动时基线生成尚未结束，sourceChanged=true，
不作有效候选证据；已由上述稳定 race 替代。

make test-contracts `20261021T011508Z-5c8e0c389c74`：3458 pass、1 fail、9 skip，sourceChanged=false，
未通过验收。失败是 OBJECT 新增 objects:diff 后库存回归仍固定 58/29/87；现按注册表和独立金样
更新为 59/29/88，保留精确数量、位置、归属及全金样闭包断言。9 个外部 live 测试的跳过仍待各自
真实环境入口验证，不降低断言、不删除测试、不把本次报告写成验收通过；本任务保持 doing。

修正路由库存后，`20261021T011917Z-bd37d74f8247` 完整混合契约仍未通过（5604 pass / 9 fail / 14 skip，
sourceChanged=false）。发布协调、投影丢失修复及语义解释的失败已在 INDEX-PHYSICAL-01 登记并推进；
无变化写入旧预期归 OBJECT-VERSIONING-01 修正。本任务不据组件/边界阶段通过而移动到 done。

## 交付

实现：`client/session` 持有服务地址选择、登录流程、会话存储、逐请求重读、刷新与注销；
`client/auth` 持有认证运输与身份 HTTP SDK；`internal/clientconfig` 持有无运输/凭据依赖的地址与路径
算法；根 `client` 只保留公开别名与兼容委派。`cli/remote_login.go`、`cli/public_session.go` 只剩参数
校验、显示与打开浏览器等宿主交互，`invocation/session_helpers.go` 的配置/路径辅助委派到
`internal/clientconfig` 与 `internal/jsonfile`，`cli/remote_session.go` 已消除。分层守卫
（`internal/arch/layers_test.go:236-237`、`internal/arch/client_session_test.go`）证明会话包及其依赖闭包
不进入 `cli`、`invocation`、`home`、`server` 或具体 provider。

已验证：完整架构 719 pass / 0 skip；会话选定回归 35 pass / 0 skip；稳定 race 37 pass / 0 skip；
`make test`、`make quality`（84 units / 13.58%，ratchet held）、`make check-docs`（21 篇）通过；
`make test-contracts` 0 FAIL，e2e 公开命令覆盖 54/54。

环境分级未验：14 项 live skip 属独立真实入口（付费 LLM 2 项、太湖鉴权 1 项、规模压测 4 项、
state runtime 独立栈 3 项、真实 lakeFS 2 项、FUSE 场景 1 项、访问者刷新 1 项），按各自合同单独进行，
不计为通过；本任务的会话合同由 argv、broker 与 kcfs 入口回归覆盖，不依赖这些外部环境。
