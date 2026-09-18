# 部署：Taihu 身份认证接入

日期：2026-09-02

定位：共享部署上的 Taihu 认证器、登录旅程与密钥边界。Client↔Server 配对不变量、三种
principal 与 `onBehalfOf` 的授权含义由 [`PERMISSIONS.md`](PERMISSIONS.md) 拥有；
传输头与无会话请求由 [`SERVICE_ARCHITECTURE.md`](SERVICE_ARCHITECTURE.md) §8.1
拥有。本文不复制 allow 规则字段或 HTTP DTO 全集。

---

## Goal

规定共享部署上 Taihu 认证器、登录旅程与密钥注入：服务端验证身份，客户端使用用户或已授权机器主体的凭证；普通用户不持有部署应用秘密。

## Non-Goals

- 不拥有 allow 规则字段或 HTTP DTO 全集（`PERMISSIONS.md`、`SERVICE_ARCHITECTURE.md`）。
- `--auth local` 不是 Taihu 的降级模式。
- 密钥不得写入仓库、镜像、启动脚本或日志。

## 硬性约束 / Invariants

- 部署未声明认证模式时，不得静默变成 local 断言。
- 身份由认证器注入；taihu/gitea 拒绝自报 `X-Kc-As`。
- `principal` / `onBehalfOf` 含义不随认证器替换而改变（`PERMISSIONS.md`）。

## 选定方案 / 被否决方案

- 选定：部署环境注入 `KC_TAIHU_HMAC_SECRET` 等；本机夹具显式配置 `auth: local`。
- 否决：Client 自报身份头；把凭证复制进 evidence、patch、报告或 trace。

## 接口契约 / 状态机

产品配对：Server 配置显式声明认证模式，Client 只发送 `Authorization`。密钥只从部署环境注入。传输头由 `SERVICE_ARCHITECTURE.md` §8.1 拥有。参考实现可在 `cli/` 装配 Taihu/Gitea/local，local 不是 Taihu 的降级。


## 1. 部署前提

产品配对是配置 `auth: taihu`（或 `auth: gitea`）的 Server，加上只发送
`Authorization` 的 Client。部署必须显式声明认证模式；省略不得静默变成
local 断言。本机/夹具配对使用配置 `auth: local`，见 Permissions §7.3，不是 Taihu
的降级模式。

密钥只从部署环境注入，不得写入仓库、镜像、启动脚本或日志。

KC 的资源方应用标识可以公开；introspection 应用密钥和网关验签密钥只交给服务部署。
授权服务器、可信域名和应用配置由该部署在 Taihu 管理，不能把某个测试环境地址写成产品默认。
配置名与认证 adapter 映射见 [`cli/README.md`](../cli/README.md)。若真实凭据曾进入仓库或构建日志，
必须在身份系统撤销并轮换；删除文本不能使已泄漏凭据失效。

---

## 2. Server 启动

先持久保存部署配置和服务状态，首次运行 `kc deployment init --config deployment.yaml`；后续启动只恢复。配置显式声明 Taihu 认证与监听地址；直连方案另声明部署使用的授权服务器。Catalog Snapshot 权威、配置和授权状态均不依赖实例工作目录。密钥由 Secret Manager 注入，不进入配置 Git。

### 方案 A：太湖网关后（推荐）

网关校验 Bearer 并注入 `x-tai-identity`。生产必须配置 HMAC 密钥；空密钥只允许
受控开发拓扑。

网关验签配置只存在服务端；公开配置与启动参数由 [`cli/README.md`](../cli/README.md) 和 `kc serve` 帮助维护。

### 方案 B：直连 introspection（不经网关）

Client 把用户或服务账号的 Bearer 直接打到 KC。KC 用资源方凭证向 Taihu
introspection，再映射 `principal` / `onBehalfOf`。

两种方案都拒绝客户端自报 principal 或委托关系。未配置可用认证器时，裸 Bearer 不得被
接受为开发身份；认证模式不匹配与授权不足必须可区分。无凭证的配对发现只报告登录方式，
不暴露秘密，也不发权。配对发现和身份查询的路径、响应及错误码由 CLI/HTTP 公开合同拥有。

---

## 3. 三种身份怎样从 Taihu 进入 KC

用户本人、代理用户的 Agent、服务账号三种主体遵守 [`PERMISSIONS.md`](PERMISSIONS.md) §7.3。
Taihu adapter 必须从已验证声明中取得稳定主体；代理关系只能来自经过验证的委托声明，不能
靠客户端请求头或本地身份选项冒充。外部 claim 到 KC 身份的确切映射由
[`cli/README.md`](../cli/README.md) 与 `auth_taihu.go` 维护，不在两个设计 owner 重复。

---

## 4. 客户端登录

交付给接入方和消费方的客户端应封装唯一服务地址，先发现服务的认证方式，再引导用户
证明本人身份。后续业务请求使用登录凭证；配方与 pin 不保存凭证，刷新凭证不改变固定版本。

普通登录不得要求用户取得 `KC_SERVICE_CLIENT_SECRET` 或网关 HMAC 密钥。Taihu 接入沿用
PAR/PKCE，选定由 Server 在固定部署上游完成授权码交换与续期，应用秘密只在 Server。
浏览器所需的授权发起和状态查询也可经该固定上游转送；不允许调用者替换上游或应用身份。
这些操作只验证授权证明，不建立 Workspace session、不发权。身份查询和本机持久化均成功
后客户端才报告登录成功；客户端按 Server 隔离凭证，续期不改变任务 pin。

当前客户端登录行为与限制见 [`cli/README.md`](../cli/README.md)；产品缺口由
[`MVP_ACCEPTANCE.md`](MVP_ACCEPTANCE.md) 记录。客户端保存的是本机登录态，不是服务端
Workspace session；具体存储与身份模式配对由公开 Client/CLI 合同维护。

---

## 5. 太湖平台配置

1. 登录 [tai.it.woa.com](https://tai.it.woa.com)
2. 为当前部署登记资源方应用
3. 在应用详情页配置当前部署的登录可信域名
4. 创建站点并关联该部署域名
5. 确认网关已注入 `x-tai-identity` header

---

## 6. 验证

本机不要占用数仓 Compose 的 `--auth local`（默认 `127.0.0.1:7380`）。直连
introspection 用另一端口：

```bash
export KC_SERVICE_CLIENT_SECRET
./scripts/live-taihu-auth.sh
```

脚本的验证目标是以 Taihu 认证配置启动 Server，打印授权
URL，等你在浏览器完成 Taihu 登录，再跑 `whoami`。不要把 token 写进
仓库。拿到 Bearer 之后可重复：

```bash
export KC_LIVE_TAIHU=1 KC_AUTH_TOKEN
make test-taihu-live
```

配对失败必须能从错误分辨：缺凭证或发了 `X-Kc-As` 到 Taihu Server 是认证/配对
错误，不是“尚未登录”的含糊提示。证据见
[`TEST_CATALOG.md`](TEST_CATALOG.md) P-12..P-20。
