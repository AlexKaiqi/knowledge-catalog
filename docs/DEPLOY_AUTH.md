# 部署：Taihu 身份认证接入

日期：2026-09-02

定位：共享部署上的 Taihu 认证器、登录旅程与密钥。Client↔Server 配对不变量、三种
principal 与 `onBehalfOf` 的授权含义由 [`PERMISSIONS.md`](PERMISSIONS.md) 拥有；
传输头与无会话请求由 [`SERVICE_ARCHITECTURE.md`](SERVICE_ARCHITECTURE.md) §8.1
拥有。本文不复制 allow 规则字段或 HTTP DTO 全集。

---

## Goal

规定共享部署上 Taihu 认证器、登录旅程与密钥注入：Server 声明 `--auth taihu|gitea`，Client 只发送 `Authorization`。

## Non-Goals

- 不拥有 allow 规则字段或 HTTP DTO 全集（`PERMISSIONS.md`、`SERVICE_ARCHITECTURE.md`）。
- `--auth local` 不是 Taihu 的降级模式。
- 密钥不得写入仓库、镜像、启动脚本或日志。

## 硬性约束 / Invariants

- `kc serve` 省略 `--auth` 不得静默变成 local 断言。
- 身份由认证器注入；taihu/gitea 拒绝自报 `X-Kc-As`。
- `principal` / `onBehalfOf` 含义不随认证器替换而改变（`PERMISSIONS.md`）。

## 选定方案 / 被否决方案

- 选定：部署环境注入 `KC_TAIHU_HMAC_SECRET` 等；本机夹具用 `--auth local`。
- 否决：Client 自报身份头；把凭证复制进 evidence、patch、报告或 trace。

## 接口契约 / 状态机

产品配对：Server 显式 `--auth`，Client 只发送 `Authorization`。密钥只从部署环境注入。传输头由 `SERVICE_ARCHITECTURE.md` §8.1 拥有。参考实现可在 `cli/` 装配 Taihu/Gitea/local，local 不是 Taihu 的降级。


## 1. 部署前提

产品配对是 `--auth taihu`（或 `--auth gitea`）的 Server，加上只发送
`Authorization` 的 Client。`kc serve` 必须显式声明 `--auth`；省略不得静默变成
local 断言。本机/夹具配对使用 `--auth local`，见 Permissions §7.3，不是 Taihu
的降级模式。

密钥只从部署环境注入，不得写入仓库、镜像、启动脚本或日志。

| 字段 | 值 |
|------|------|
| client_id | `knowledge-catalog`（公开标识，不是密钥） |
| client_secret | `KC_SERVICE_CLIENT_SECRET`：KC 作为资源方向 Taihu introspection 的应用密钥，不是调用方身份 |
| 网关 HMAC | `KC_TAIHU_HMAC_SECRET`：校验 `x-tai-identity` 的 hex 密钥 |
| 服务域名 | `test.dw-knowledge-base.tianqiong.woa.com` |
| OAuth2 授权服务器 | `http://iam.it.woa.com`（当前为 HTTP） |

`--auth-hmac-secret` / `--service-client-secret` 只是覆盖环境变量的 flag，不要把字面量写进 git 或进程 argv。如果真实凭据曾进入仓库或构建日志，删除文本并不能使它失效；必须在身份系统中撤销并轮换。

---

## 2. Server 启动

### 方案 A：太湖网关后（推荐）

网关校验 Bearer 并注入 `x-tai-identity`。生产必须配置 HMAC 密钥；空密钥只允许
受控开发拓扑。

```bash
export KC_TAIHU_HMAC_SECRET   # hex，从 Secret Manager 注入
export KC_SERVICE_CLIENT_SECRET
kc serve --auth taihu --listen :7380 --service-client-id knowledge-catalog
```

### 方案 B：直连 introspection（不经网关）

Client 把用户或服务账号的 Bearer 直接打到 KC。KC 用资源方凭证向 Taihu
introspection，再映射 `principal` / `onBehalfOf`。

```bash
export KC_SERVICE_CLIENT_SECRET
kc serve --auth taihu --auth-url http://iam.it.woa.com --listen :7380 \
  --service-client-id knowledge-catalog
```

两种方案都拒绝 `X-Kc-As` 和客户端自报的 `X-Kc-On-Behalf-Of`。未配置
introspection 时，裸 Bearer **不得**被接受为开发身份；缺少网关头或 introspection
结果时返回 `UNAUTHENTICATED`。

`GET /identity/v1/auth` 无需凭证，报告 `mode=taihu`、`localAssertion=false`、
`accepts=["Authorization"]`。`GET /identity/v1/whoami` 仍要求配对凭证。

---

## 3. 三种身份怎样从 Taihu 进入 KC

授权仍是 `principal × action × repository`。`onBehalfOf` 只进访问证据。

| 产品登录 | 谁在浏览器/机器上证明 | 注入的身份 |
|---|---|---|
| 用户直接使用 | 用户在 Taihu 授权 | `principal=taihu:<username>`，无 `onBehalfOf`；工号留在 `subject` |
| Agent 代理用户 | 用户同意后由 Agent 持有携带 actor+subject 的 token | `principal=agent:<id>`，`onBehalfOf=taihu:<username>` |
| 服务账号 | Taihu `client_credentials`（测试夹具才用 `--auth local --as service:<id>`） | `principal=service:<client_id>`，无 `onBehalfOf` |

introspection 映射（claim 名可随 Taihu 对齐，语义不得改）：

- 有用户 subject、无 actor → 用户直接使用；`principal=taihu:<username>`；缺 `username` 失败关闭。
- 有用户 subject 且有 actor/agent client → Agent 代理用户；`onBehalfOf=taihu:<username>`；缺 `username` 失败关闭。
- 无用户、只有 client → 服务账号；`principal=service:<client_id>`。
- 工号只写入 `subject`，不进入 allow.json。

网关 `x-tai-identity` 用 `user_name`（或 `username`）作为用户 principal，`staff_id`
只进 `subject`；缺用户名失败关闭。`x-tai-user` 同样是用户名，不是工号。Agent 委托必须出现在 **已验证** token /
introspection 声明里，不能靠 `kc login --as <user>` 或请求头冒充。

---

## 4. 客户端登录

`kc login --server <url>` 先读 `GET /identity/v1/auth`，再按 Server 模式分支。默认
模式跟随 Server，不再在未探测时默认 Taihu。

- Server `taihu`：浏览器 PAR/PKCE；`kc login --wait` 用 `KC_SERVICE_CLIENT_SECRET`
  做 token 交换（Taihu confidential client），凭证写入
  `~/.config/kc/session-taihu.json`；随后 `whoami` 覆盖占位 principal。
- `--mode local` / `--as` 作为身份来源：对 Taihu Server 返回客户端
  `USAGE_INVALID`。
- `--mode token` 不是第三种配对：已签发 Bearer 只从 `KC_AUTH_TOKEN` 读取，交给
  `--auth taihu|gitea` 的 Server。不要把 token 写进 git 或命令行。

Taihu 会话文件与 local 断言文件互斥。后续业务请求只发 `Authorization`。

```bash
kc login --server http://localhost:7380
# 浏览器打开 → 授权 → 另一终端
kc login --wait --server http://localhost:7380
kc whoami
```

---

## 5. 太湖平台配置

1. 登录 [tai.it.woa.com](https://tai.it.woa.com)
2. 创建应用（已创建：`knowledge-catalog`）
3. 在应用详情页配置登录可信域名：`test.dw-knowledge-base.tianqiong.woa.com`
4. 创建站点，关联域名 `test.dw-knowledge-base.tianqiong.woa.com`
5. 确认网关已注入 `x-tai-identity` header

---

## 6. 验证

本机不要占用数仓 Compose 的 `--auth local`（默认 `127.0.0.1:7380`）。直连
introspection 用另一端口：

```bash
export KC_SERVICE_CLIENT_SECRET
./scripts/live-taihu-auth.sh
```

脚本会启动 `kc serve --auth taihu --auth-url http://iam.it.woa.com`，打印授权
URL，等你在浏览器完成 Taihu 登录，再跑 `whoami`。不要把 token 写进
仓库。拿到 Bearer 之后可重复：

```bash
export KC_LIVE_TAIHU=1 KC_AUTH_TOKEN
make test-taihu-live
```

配对失败必须能从错误分辨：缺凭证或发了 `X-Kc-As` 到 Taihu Server 是认证/配对
错误，不是“尚未登录”的含糊提示。证据见
[`TEST_CATALOG.md`](TEST_CATALOG.md) P-12..P-20。
