# snapshot/console/

Snapshot Store 的完整 Web 控制台和静态 HTTP 适配：保留 lakeFS 原页面，在其上提供
Dataset 与对应权限管理入口。

## 边界

拥有 `webui/` 的前端源码、许可与锁定依赖、`webapp/` 的嵌入资产、页面深链接与静态资源交付。
Dataset 组织版本化文件，不要求解析对象、Schema 或检索索引。
原 Repositories、对象浏览、提交、分支、标签、比较、PR、Actions、Settings、Administration、
登录、Setup 和插件能力全部保留。

本包不执行数据 API，不验证身份或发放权限，不实现 lakeFS 后端。调用方在同一 origin 装配
原 lakeFS API 与 Snapshot Store 应用 API；身份与授权仍由对应 API 检查。
KC Server 是一个挂载方，原 KC 观察台仍由 `server/web/` 提供。

## 最小用法

```go
import (
    "net/http"
    "kc/snapshot/console"
)

mux := http.NewServeMux()
mux.Handle("GET /ui/", console.Handler())
```

先用已有依赖执行 `npm --prefix snapshot/console/webui run build`，再构建使用者二进制。
Vite 输出到本包 `webapp/`，只跟踪 `.gitkeep`。未构建时 handler 显示构建提示；
已声明页面支持深链接，缺失静态资产返回 404。

## 依赖方向

Go handler 仅依赖标准库，可独立挂载。禁止依赖 `server`、`cli`、`catalog`、`object`、
`retrieval` 或应用执行器。静态控制台不是 `snapshot.Store` 接口的一部分。
前端沿用 pinned lakeFS v1.58.0 的 Apache-2.0 源码和依赖，不单独建立 Dataset 登录。
完整代理、凭证边界、版本说明与检查入口见 [接线说明](webui/KC-VENDOR.md)。

个人权限与授权管理共用规则详情、创建/修改表单和条件修改窗口，字段采用左侧 key、
右侧 value 的列布局。个人权限只展示当前调用方的显式规则，只有服务端给出可管理提示
的规则才显示修改入口，实际修改仍由服务端校验当前权限和旧规则快照。
原 My Credentials、Users、Groups 导航保留；外部认证模式显示已核验主体并链接实际认证
系统，该系统管理账号、组与用户凭据。私网 lakeFS 的账号管理 API 不向用户转发。
遗留显式 lakeFS 认证模式继续显示原管理页并遵守原后端能力限制。

## 验证

`TestDatasetUIRoutingIndependentOfBuildState` 与 `TestDatasetConsoleHandlerCanBeMountedWithoutKCServer`
验证独立静态 handler；Server、CLI 保留挂载、脚本、安全和缺失资源的集成合同。
`make test-webui` 执行完整前端类型、lint、行为、构建及受影响 Go 回归。
原控制台路由由 `webui/src/extendable/completeConsole.test.tsx` 守卫；实际后端证据与版本边界
见 [控制台验证记录](webui/CONSOLE-VALIDATION.md)。

原 Repo API 也须经 `snapshot/service`：现有认证确认调用者，当前产品授权通过后用服务私有存储凭据转发未修改的私网 lakeFS。原账号与认证复用现有提供方；Dataset 没有第二次认证。接线与网络边界见 `webui/KC-VENDOR.md`。
