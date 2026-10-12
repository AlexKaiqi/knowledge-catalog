# Snapshot Store 控制台 — vendored lakeFS webui

本目录沿用 [treeverse/lakeFS](https://github.com/treeverse/lakeFS/tree/v1.58.0/webui)
的 Apache-2.0 前端，在完整原控制台上增加 Dataset 顶层入口与对应权限管理。
Repositories、对象浏览、提交、分支、标签、比较、PR、Actions、Settings、Administration、
原登录、Setup 和插件扩展继续由原 `LakeFSApp` / `IndexPage` / `Layout` 提供。
前端、嵌入资产与静态 HTTP handler 归属 `snapshot/console/`；组合入口直接复用已有 lakeFS 后端与用户登录。
Dataset 与文件授权由 Snapshot Store 的应用面提供，上层 KC 负责装配和调用，不在控制台建立知识对象模型。

## Pin 与修改范围

- 上游 tag：`v1.58.0`，抽取时间：2026-09-22；未升级或安装依赖。
- 上游根 `LICENSE`、`NOTICE` 原样保存在本目录；改造部分沿用 Apache-2.0。
- `src/main.tsx` / `src/extendable/kcApp.tsx` 保留原 App 与 PluginManager。
  `src/pages/index.jsx` 增加 Dataset routes，Router 的 basename 为 `/ui`。
- `src/lib/components/navbar.jsx` 增加 Datasets 导航；Dataset 页面复用 Layout，
  原仓页保留原导航、仓内 tabs 和能力入口。恢复可达页面时修正了源码类型问题。
- `src/lib/api/kc.ts` / `session.ts` / `transport.ts` 与 `src/lib/hooks/kcAuth.tsx`
  复用 KC 认证发现和 whoami，使用原 lakeFS 登录后的 HttpOnly Cookie。
  Datasets 没有独立登录表单，不保存用户名、token 或访问密钥；进入页面先复核身份，
  取消、退出和身份变化使在途结果失效。原用户 profile 缓存仅增失效代次守卫，
  防旧请求在账号切换后写回；原 API 路径和登录提交继续复用。
- `src/pages/kc/` 在 `/datasets` 直接列出服务登记的 Dataset；名称在一套服务内唯一。
  新建、编辑与历史详情使用 `/datasets/actions/create`、`/datasets/:dataset/edit` 与 `/datasets/:dataset`，
  没有 Catalog 选择、query 或路径。未知 JSON 字段被拒绝；定义变化后需重新预览才能发布。
  旧来源未声明交付路径时保持原声明，不能因编辑自动扩成整仓交付。
- Dataset 发布定义与原仓内容比较共用 `repository/TextDiff.tsx`，即原来的
  `react-diff-viewer-continued` 组件。来源文件变化直接链接原 Repository Compare，
  传入两版实际固定 commit，复用原 lakeFS diff API 和文件 renderer；不创建 Git checkout。
  `consoleRepository` 是注册来源的可选导航元数据，由宿主验证它属于已认证的原仓实例后
  提供；没有坐标则明确显示未接入，不能从 kr:// 身份猜仓名。此链接不发权，不自动用
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
LAKEFS_UI_SERVER=http://127.0.0.1:18080 \
LAKEFS_S3_SERVER=http://127.0.0.1:18081 \
npm --prefix snapshot/console/webui run dev
# npm --prefix snapshot/console/webui run serve 使用同样的环境变量和分流
```

`KC_UI_SERVER` 默认 7380；`LAKEFS_UI_SERVER` 的源码默认值为 18000；
`LAKEFS_S3_SERVER` 默认与 lakeFS API 地址相同。示例只连接已准备后端，不启动它们。
dev / preview 对 GET/HEAD 的裸 `/ui` 统一 308 到 `/ui/`，保留查询；与 Server 和
部署入口的挂载规则对齐，原登录的刷新行为不需要改写。
S3 使用 path-style URL，原 DuckDB viewer 向同源的 `/<repository>/...` 发请求。
上游预签名对象地址仍由 lakeFS 返回，跨源地址需对象存储允许浏览器 CORS。

## 生产同源分流

部署层在同一外部 origin 将原页面请求转给已有 lakeFS 后端，将 Dataset 和 KC 授权请求转给 KC。
下表及配置是组合接线，原 API 和原认证继续由 lakeFS 提供，使用用户自己的凭证。

| 请求 | 后端与凭证 |
|---|---|
| `/ui/` 与 `/ui/*` | `snapshot/console.Handler()` 的静态资产及页面深链接；KC Server 只挂载 |
| `/identity/v1`、`/snapshot/v1`、`/knowledge/v1`、`/admin/v1` | KC Server；复用原用户的 lakeFS Cookie，经固定 lakeFS 后端验证后由 KC 独立评权 |
| `/api/v1` 与其子路径 | lakeFS 原 API；该用户的 lakeFS Cookie/凭证 |
| `/logout`、`/oidc/*` | lakeFS 原登出与 OIDC 登录/回调；保留后端或外部 IdP 的浏览器重定向。全部页面共用原退出入口，同时清除 Dataset 内存状态 |
| 其余根路径 | lakeFS S3 网关；原 viewer 沿用湖用户 Cookie/token（DuckDB 的占位 S3 密钥不承担认证） |

以下为可审阅的 Nginx 分流示例，放入 `http` 上下文；替换域名、上游端口，
由部署方在外层配置 HTTPS。没有应用到任何现有部署。
不带 URI 的 `proxy_pass` 保留请求路径和查询，避免破坏编码的 Dataset ID 或 S3 签名；
行为依据见 [Nginx 官方文档](https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_pass)。

```nginx
upstream kc_console_backend { server 127.0.0.1:7380; }
upstream lakefs_console_backend { server 127.0.0.1:18080; }
upstream lakefs_s3_backend { server 127.0.0.1:18081; }

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
    location ^~ /ui/ { proxy_pass http://kc_console_backend; }
    location ~ ^/(identity|snapshot|knowledge|admin)/v1(?:/|$) {
        # proxy_set_header directives inherit only if none are set locally.
        proxy_set_header Host $http_host;
        proxy_set_header X-Forwarded-Host $http_host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_pass http://kc_console_backend;
    }
    location ~ ^/api/v1(?:/|$) {
        proxy_set_header Host $http_host;
        proxy_set_header X-Forwarded-Host $http_host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Kc-As "";
        proxy_pass http://lakefs_console_backend;
    }
    location = /logout {
        # Mount only the original relative login page under /ui; keep external IdP redirects.
        proxy_redirect ~^(/auth/login(?:[?#].*)?)$ /ui$1;
        proxy_pass http://lakefs_console_backend;
    }
    location ^~ /oidc/ {
        proxy_redirect ~^(/auth/login(?:[?#].*)?)$ /ui$1;
        proxy_pass http://lakefs_console_backend;
    }
    location / {
        proxy_request_buffering off;
        proxy_pass http://lakefs_s3_backend;
    }
}
```

登出保留原浏览器导航，不通过 `fetch` 截断认证器的跳转。仅把原后端相对
`/auth/login` 响应地址挂到 `/ui/auth/login`，外部与已带 `/ui` 的地址保持原样；
对应 Nginx 行为见 [proxy_redirect 官方文档](https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_redirect)。
保留外部 Host；S3 签名不能在代理中改路径或主机。原 lakeFS 的 SSO `login_url`、
fallback、logout_url、Cookie Domain/Path 和回调地址应以外部 origin 配置；认证器
若使用额外回调路径，部署方必须明确转发该路径到 lakeFS，不能落入 S3。
组合控制台部署显式配置 `auth: lakefs`、`authURL: http://127.0.0.1:18080`，
或 `kc serve --auth lakefs --auth-url http://127.0.0.1:18080`。KC 只向这个固定
原后端的 `/api/v1/user` 验证调用方证明；不转发部署的 Store 管理凭据。
原 Cookie 的 Domain/Path 必须覆盖控制台 origin 的 KC 路径。KC Cookie 请求
附带 `X-Kc-Console: 1`，KC 不开放跨源 CORS；网关应保留该标记和 Fetch Metadata，
不得替跨源请求自动补标记或把浏览器用户名转成身份头。客户端传来的 KC
自报身份头仍被拒绝。具体证明过滤与耐久身份绑定见 `server/README.md`。
统一登录不授予新权限；lakeFS 管理员身份不自动成为 KC 管理员。原 User API
仅有账号 id 与秒级创建时间，账户回收/迁移必须显式处置旧 KC 绑定和授权；
同名同秒重建的识别缺口与验证边界见 TASK、控制台验证记录。自动绑定限于
符合现有 KC 用户名合同的原账号 ID；邮箱等不兼容 ID 明确拒绝，需要显式身份关联，
不静默改名，也不让原用户重复登录。KC 的 Taihu/
Gitea/local API 与 CLI 模式保留，组合控制台没有这些模式的第二次登录入口；
不兼容配置明确报错，不回退本地身份。
Server 不固定增加会阻断上游预签名请求、远端图片或 PDF viewer 的 CSP；
部署如增加 CSP，应按实际对象存储、图片与 SSO 来源验证完整原控制台。

部署前验证原登录一次后直接进入 Dataset、统一登出与会话过期、权限不足、
原文件上传下载、S3 viewer、刷新深链接
和缺失资源 404。本轮夹具验证不代替目标网关、SSO 或真实湖部署验收。

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
