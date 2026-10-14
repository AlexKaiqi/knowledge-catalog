# Snapshot Store 控制台 — vendored lakeFS webui

本目录沿用 [treeverse/lakeFS](https://github.com/treeverse/lakeFS/tree/v1.58.0/webui)
的 Apache-2.0 前端，在完整原控制台上增加 Dataset 顶层入口与对应权限管理。
Repositories、对象浏览、提交、分支、标签、比较、PR、Actions、Settings、Administration、
原登录、Setup 和插件扩展继续由原 `LakeFSApp` / `IndexPage` / `Layout` 提供。
前端、嵌入资产与静态 HTTP handler 归属 `snapshot/console/`；组合入口复用未修改的私网 lakeFS 后端与现有认证系统。
Dataset 与文件授权由 Snapshot Store 的应用面提供，上层 KC 负责装配和调用，不在控制台建立对象模型。

## Pin 与修改范围

- 上游 tag：`v1.58.0`，抽取时间：2026-09-22；未升级或安装依赖。
- 上游根 `LICENSE`、`NOTICE` 原样保存在本目录；改造部分沿用 Apache-2.0。
- `src/main.tsx` / `src/extendable/kcApp.tsx` 保留原 App 与 PluginManager。
  `src/pages/index.jsx` 增加 Dataset routes，Router 的 basename 为 `/ui`。
- `src/lib/components/navbar.jsx` 增加 Datasets 导航；Dataset 页面复用 Layout，
  原仓页保留原导航、仓内 tabs 和能力入口。恢复可达页面时修正了源码类型问题。
- `src/lib/api/kc.ts` / `session.ts` / `transport.ts` 与原 `index.js`
  共用现有认证证明。Taihu/Gitea 模式使用认证系统已签发的用户令牌，组织认证沿用
  既有 PAR/PKCE 与续期；`whoami` 确认主体后才进入控制台。浏览器令牌仅保存在
  按 origin 隔离的 sessionStorage，不进入 URL、localStorage、pin 或资源定义。
  原 Repo、Dataset 与授权页没有第二次认证。退出、身份切换和迟到响应有代次保护，
  续期必须再次确认同一主体。遗留 lakeFS 模式仍可显式使用原会话。
  `sharedLogin.tsx` 复用 Bootstrap 登录布局，不创建账号、组或签发用户令牌。
- `src/pages/kc/` 在 `/datasets` 直接列出服务登记的 Dataset；名称在一套服务内唯一。
  新建、编辑与历史详情使用 `/datasets/actions/create`、`/datasets/:dataset/edit` 与 `/datasets/:dataset`，
  没有 Catalog 选择、query 或路径。未知 JSON 字段被拒绝；定义变化后需重新预览才能发布。
  旧来源未声明交付路径时保持原声明，不能因编辑自动扩成整仓交付。
- Dataset 发布定义与原仓内容比较共用 `repository/TextDiff.tsx`，即原来的
  `react-diff-viewer-continued` 组件。来源文件变化直接链接原 Repository Compare，
  传入两版实际固定 commit，复用原 lakeFS diff API 和文件 renderer；不创建 Git checkout。
  `consoleRepository` 是注册来源的可选导航元数据，由宿主验证它属于已认证的原仓实例后
  提供；来源必须绑定在配置的 native origin，没有坐标则明确显示未接入，不能从 kr:// 身份猜仓名。此链接不发权，不自动用
 Dataset 授权读取整仓；跨仓组合和目标路径变化在发布定义中展示。
- `vite.config.mts` 的 `base: '/ui/'`、输出 `../webapp` 和构建后 `.gitkeep`
  与 Snapshot Store console embed 对齐。缺失静态资源返回 404，已声明页面支持深链接。

官方 Dataset 能力、KC 可复用合同与剩余差距见 [API review](DATASET-API-REVIEW.md)。
较新 lakeFS 的 Dataset 文档不能被当成 v1.58.0 开源前端已具备的后端功能。

## 构建与本机开发

使用已有锁定依赖；依赖缺失时按项目批准流程准备，检查命令不安装依赖。

```bash
make test-webui
# 或分别执行
npm --prefix snapshot/console/webui run typecheck
npm --prefix snapshot/console/webui run lint:console
npm --prefix snapshot/console/webui test
npm --prefix snapshot/console/webui run build
```

先构建前端，再构建使用者二进制；`snapshot/console` 从本包的 `webapp/` 嵌入产物。
KC Server 只挂载 `console.Handler()`；其它 HTTP 服务也可在 `/ui/` 挂载该 handler。
生成物被 Git 忽略，仅跟踪 `.gitkeep`。未构建时 `/ui/` 显示构建提示。

开发和 preview 复用同一组代理规则：

```bash
KC_UI_SERVER=http://127.0.0.1:7380 \
KC_UI_AUTH_SERVER=http://127.0.0.1:18080 \
npm --prefix snapshot/console/webui run dev
# npm --prefix snapshot/console/webui run serve 使用同样的环境变量和分流
```

`KC_UI_SERVER` 默认 7380。`KC_UI_AUTH_SERVER` 是可选的已有认证网关地址，
没有默认值；只有显式配置时才代理 `/logout` 与 `/oidc/*`，不会误接私网存储认证。
例子只连接已准备后端，不启动服务。dev / preview 的裸 `/ui` 统一 308 到 `/ui/`。
原 DuckDB 引擎保留：网关 Cookie 模式使用同源 HTTP/Range；Bearer 模式通过受控
原文件 API 读取字节后注册 buffer，因为 worker 不继承浏览器脚本的令牌。
图片、PDF 和原文件下载也通过实际用户的受控请求，令牌不会写入链接。
预览仍受 vendor 的文件体积上限约束；原下载的 Blob 方式会占用文件大小的浏览器内存，
超大文件流式保存待浏览器文件写入能力接线，触发条件为大文件下载需求。

## 生产同源接线

现有认证系统验证人、Agent 或服务主体；Snapshot 统一授权 Repo 与 Dataset。
授权通过后，服务内部的 `KC_LAKEFS_CREDENTIAL` 调用未修改的 lakeFS 1.86。
该凭据既不是用户认证证明，也不进入前端。native origin 来自显式 Registry lakeFS
DSN；不同认证系统的 issuer 不会被误当成存储地址。

| 请求 | 路径与身份 |
|---|---|
| `/ui/` 与深链接 | `snapshot/console.Handler()`；先构建前端再构建服务 |
| `/identity/v1`、`/snapshot/v1`、`/object/v1`、`/admin/v1` | 现有认证验证主体，当前产品规则授权 |
| `/api/v1` | Snapshot 入口验证与授权后调用私网 lakeFS；未知操作拒绝 |
| native 账号、凭据、组、策略、初始化 | 外部认证模式拒绝转发私有存储账号；保留导航并链接真实认证系统 |
| 已有认证网关的退出／回调 | 部署显式配置；浏览器会话由原认证系统管理 |
| 其余路径 | KC 明确入口或 404；没有 native 数据 fallback |

配置示例（秘密由部署环境注入）：

```yaml
auth: taihu
# 或已有 Gitea 认证地址；不是 Gitea 存储 adapter
authURL: https://identity.example.test/oauth2
# 使用网关浏览器会话时必须声明其既有退出入口
authLogoutURL: https://identity.example.test/logout
registry:
  id: kr://deployment/registry
  driver: lakefs
  dsn: http://private-lakefs:8000/kc-registry
```

直连 Bearer 浏览器退出清除本地证明并返回认证页，不撤销认证系统已签发的全局令牌。
网关 Cookie 会话还须导航到 `authLogoutURL` 清除其会话；KC 不猜测 Taihu 的退出 URL。
CLI 保存与退出仍按既有 Client 合同。认证系统与存储管理员不会自动得到产品 grant。

下面是入口分流例子，放入 Nginx `http` 上下文。示例未应用到任何部署。
外部认证网关应清理客户端伪造的身份头并按原验签合同注入；这里不自行制造身份。

```nginx
upstream kc_console_backend { server 127.0.0.1:7380; }
server {
    listen 8080;
    server_name console.example.test;
    client_max_body_size 512m;
    proxy_http_version 1.1;
    proxy_set_header Host $http_host;
    proxy_set_header X-Forwarded-Host $http_host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_cache off;
    location = /ui { return 308 /ui/$is_args$args; }
    location / {
        proxy_request_buffering off;
        proxy_pass http://kc_console_backend;
    }
}
```

所有产品数据请求交给同一个入口；已有身份网关若位于同源外层，在其自身配置中承接
已登记的回调和退出路径。无需把 `/api/v1` 或任意根路径转给 lakeFS。
不带 URI 的 `proxy_pass` 保留原路径与查询，依据见
[Nginx 官方合同](https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_pass)。
Cookie/签名网关浏览器请求保留 Fetch Metadata 和 `X-Kc-Console: 1`，不能替跨源
请求补标记；修改仅接受 JSON，原 Repo multipart 上传是受控例外。
客户端自报 KC 身份头仍被拒绝。原后端 Cookie、数据重定向与预签名下载不向外提供。
导入与物理地址关联也检查每个真实来源仓，不允许用目标 Repo 的导入权读取其它仓。
未登记外部介质没有可用来源授权合同，当前明确拒绝并记录这一能力边界。

私网 lakeFS 与原始介质必须仅能被服务访问；只在网关挂权限却允许用户直连数据 origin
仍可绕过，不能算可用部署。本轮测试用随机 localhost 端口隔离容器，真实用户令牌直连
native 被拒绝；生产网络防火墙与具体 Taihu SSO/SLO 仍需独立部署验收。
遗留 `auth: lakefs` 接线保留其单用户与秒精度身份代际限制，不计多用户验收。

## 检查范围与已知限制

`tsconfig.console.json` 包含整个 `src`、回归和 Vite 配置，原页面重新纳入源码类型检查；
`allowJs: true` / `checkJs: false` 保持 vendor JS 的既有检查边界。当前 **`skipLibCheck: true`**
跳过锁定第三方声明之间的兼容问题，不能描述为“未关闭库检查”。恢复库检查的
触发条件是获批依赖升级或第三方声明兼容修复；不以此排除原页面源码。
`lint:console` 覆盖本轮改动与 KC 页面，零警告；锁定工具链的 AST 弃用提示待获批升级处理。

前端回归与 Go 合同覆盖原路由保留、统一会话验证/失效/凭据边界、唯一登记空间、固定来源发布、
JSON 拒绝与切换、预览失效、真实历史/比较、分页 pin、Base64 和迟到响应。
实际浏览器与最终总门禁的本次证据见 [控制台验证记录](CONSOLE-VALIDATION.md)
和 `TASK.md` DATASET-04。
Taihu 外部授权、生产代理配置和真实 lakeFS 部署均为独立验证入口。
