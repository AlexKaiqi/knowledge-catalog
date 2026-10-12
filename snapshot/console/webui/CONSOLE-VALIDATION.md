# 本轮控制台验证记录

2026-10-12 当前合同已收敛为一套服务一个登记空间：Dataset URL 为 `/datasets` 与 `/datasets/:dataset`，API 为 `/snapshot/v1`，不再选择 Catalog；授权范围为全服务、Repository、Dataset。下列浏览器与真实部署记录中的 Catalog 是当时版本的历史证据，不证明本轮重构已通过真实部署验收。本轮代码验证由当前前端与 Go 测试报告。

2026-10-12 目录收口：本前端整体归入 `snapshot/console/webui/`，静态 handler 与嵌入资产归入
`snapshot/console/`。下文已有运行 ID、源码指纹与真实后端记录保留其当时范围，不能作为此次
完整核心/HTTP 迁移后的验收；迁移后的整体验证另以当前源码指纹记录。

此次目录收口从已提交 `fc2d589207d5566bed850c62ed4b8e660e753278` 开始，分支为
`codex/snapshot-store-module`。Dataset 核心、通用显式授权、原始文件 HTTP 和完整静态控制台
分别由 `snapshot/dataset`、`snapshot/access`、`snapshot/service`、`snapshot/console` 拥有。
旧 Catalog 为兼容薄壳；KC Server 只装配共享身份、既有默认政策、Knowledge 扩展和租约。
不升级依赖、不改变现有部署，原 lakeFS 页面及 API 直接复用。

此次独立真实环境只使用缓存 lakeFS 1.86.0 与 MinIO，并直接装配 Snapshot 模块：真实
Catalog Snapshot 权威、文件 Store、持久规则端口和共享耐久身份，没有 KC Home、Knowledge
或索引。浏览器经同源 preview 一次原登录完成 Repository 目录/CSV（DuckDB/S3）、创建
分支、原提交、标签和原生 PR 创建/合并，以及 Dataset 的显式 Catalog 选择、目录/单文件映射、预览发布 v1/v2、
Form/JSON 切换、未知对象标识字段拒绝、定义变化后的预览失效、History/Compare、历史
文件预览、小文件下载和退役。原 Setup 的无效测试邮箱被 native 后端拒绝；合法测试通信偏好
通过原 API 完成，不将 Setup UI 完成记为通过。

新模块实际 API 的 29 项证据包括独立来源库存、固定文件坐标、内容含同一 `object_id` 的
不同文件不互换、伪文件清单拒绝、来源提交推进后旧 pin 保持字节、范围授权 CRUD、
全局/其它 Catalog/Repository 越界拒绝和撤权后旧 pin 拒绝；native 管理员在模块撤权后
仍只能继续原仓访问，不能取得模块权限。浏览器授权表单也对真实规则完成添加及确认撤销。
这些是独立模块验收，KC 外部适配的回归、关闭/并发借用和完整契约仍单独执行。

本地新证据目录为 `.validation/console/20261012/`，包括
`store-module-api-proof.json`、`store-module-dataset.png`、`store-module-compare.png`；
最终命令证据由 `scripts/validation.py` 写入 `.validation/runs/`，以各报告源码指纹与
`sourceChanged` 判断，不能将下文 2026-10-10 的计数或 mock 当作此次结果。
既有 live skip、身份代际/用户名边缘、锁定库声明检查和 API review 中的剩余差距仍按
TASK 登记，`DATASET-04` 不因目录收口或局部实测而标为完成。

本次最终实现指纹为 `07d3c41a9dff04eb6737a371f530642d4f7c99af541215d64b6229eee117eba3`。
前端运行 `20261012T041316Z-b60bb23a77cd` 通过整树类型、lint、16 文件 144 测试、构建及
受影响 Go 回归；普通测试 `20261012T041316Z-c0f006202927` 为 21 通过/0 失败/0 skip；
完整契约 `20261012T041316Z-b6a2eb950444` 为 1951 通过/0 失败/20 个既有外部/live skip；
质量运行 `20261012T041316Z-e07aa6881f73` 通过，包括文档、格式、静态检查和耦合棘轮。
四次运行均 `sourceChanged=false`，同一实现指纹；早先旧 owner 路由扫描造成的失败不计通过。
88 条真实路由的声明方法、错误方法拒绝和协议证据守卫保留。Root 文件分派、nil 外部视图、
范围授权及生命周期回归的整个 `snapshot/service` 51 项通过；受影响四包 race 为 182 通过、
0 失败、1 个既有 Taihu live skip，其后仅检查工具/CLI 守卫测试改动，未改变被测生产代码。

独立真实验收为 27 个浏览器步骤、29 项 API 检查，追加根映射、真实源文件改名后历史寻址、
退役与自撤权后的重启检查；证据按各自二进制 hash 和源码范围区分，不把早期 binary 当最终
修复的证据。当前前端 bundle index hash 与迁移前一致。测试结束后的进程、容器、卷、网络、
凭据与下载夹具均已清理，现有部署及原共享目录未改动。本地最终汇总为
`store-module-final-checks.json`，清理为 `store-module-cleanup-proof.json`。
此段为实现验证完成后的交付记录；随后的发布指纹差异仅包含本段记录，另跑文档与质量闸。

日期：2026-10-10。工作树：`codex/dataset-console-ui`；提交基线：
`2aa827f8edd2c45c6014b1b17e62a46275ff3207`。用户已授权聚合提交并快进合并至 GitHub 默认 `main`；原共享目录未提交改动不纳入交付。
范围是完整原 lakeFS 控制台组合与 Dataset 增量，见
[TASK 的 DATASET-04](../../../TASK.md)、[接线与许可](KC-VENDOR.md) 和
[Dataset API review](DATASET-API-REVIEW.md)。

## Access Control 增量与验证入口

用户随后扩展 Access Control 范围：原 Administration 页面仍复用 lakeFS API，
增加 KC“我的权限”与“授权管理”，共用原登录。自查展示显式规则的全部已有约束和
外部申请地址；全量或明确 Catalog/Repository/Dataset 范围的读取、新增与撤销由
KC Server 授权。读取和修改是两种权限，列表拒绝不能被解释成修改也失败。
Scope 读取在服务端裁剪，页面内筛选只作用于已经取得的规则。

本轮补口包括：稳定规则列表不泄露私有初始化账、管理授权不退回默认 Catalog、
按耐久目标规则核对撤销范围、拒绝无法证明包含的复杂管理约束、新授权 ID 不复用、
旧重复 ID 不能批量误撤、自查保留 Dataset/ref/object/aspect。CLI 旧显式规则问题
仍保留其语义，但须全局规则读取权；它不是全部有效权限自查。
官方 API 对照、原 native 版本限制和剩余差距见
[Access Control review](ACCESS-CONTROL-API-REVIEW.md)。

定向失败证据分别在 `/tmp/kc-access-control-before.log`、
`/tmp/kc-console-access-before.log` 和 `/tmp/kc-access-proxy-before.log`；
其中 proxy 回归确认 KC `/admin/v1` 原来被发往湖的 S3 兜底路径。
dev/preview 和生产分流示例已纳入这个命名空间。mock 只为行为回归提供隔离，
Access Control 产品验收必须连接本轮独立真实 lakeFS/MinIO/KC 服务。
最终检查、实际浏览器和服务清理结果按本节后续记录判读，不引用下方旧运行充数。

Access Control 的本地运行记录归入 `.validation/console/20261010/`：
`access-final-checks.json` 绑定最终源码指纹、测试/闸层运行 ID 与 skip 边界；
`access-browser-proof.json` 记录原登录、真实授权表单与确认撤销、范围加载和原仓页面；
`access-scoped-api-proof.json` 核对实际作用域内操作、跨 Catalog 拒绝、Dataset 文件
真实字节及 KC 撤权不影响原湖权限；`access-cleanup-proof.json` 确认仅本轮进程、
容器、卷与一次性凭据已经清理。实际结果以这些记录的状态和源码匹配为准。
缓存 Community 后端拒绝新增原用户，原企业 RBAC / SSO 没有本轮真实验收证据；
前端仍固定 v1.58.0，测试后端使用已缓存 v1.86.0，不混淆两个版本。

## Access Control 最终结果与交付整理

最终实现指纹 `1b5b8d34280d2900317ee24c109c194ccaab5255cb90c2213465dccbfc71a991`：
`make test-webui`（`20261010T112247Z-f843b155b1da`）通过类型、lint、16 文件 144 测试、
生产构建和受影响 Go 回归；`make quality`（`20261010T112247Z-326606961031`，含 docs）通过；
`make test`（`20261010T112243Z-87d79c5c3de8`）21 通过、0 失败、0 skip；
`make test-contracts`（`20261010T112417Z-575927b3fcfe`）1856 通过、0 失败、20 个既有外部/live skip。
四次运行均 `sourceChanged=false`；没有新增 skip，跳过项不能算作真实验收。

独立真实环境 `2d72bbf34c` 的浏览器完成原登录、完整约束自查、授权新增、取消及确认撤销、
精确范围加载、撤自身列表读取权后的成功回执与 403、仅有管理权时继续创建授权、原仓对象读取、
Dataset 真实文件读取和原退出。跨 Catalog、跨 Repository 授权被拒绝；浏览器 error/warn 为 0。
本轮 KC/preview、容器、网络、卷与一次性凭据均清理，未修改现有部署。
浏览器先于最终仅测试/场景 DSL 兼容修复执行；最终构建与实测版本的 487 项非测试生产源码和
嵌入输入逐项哈希相同。完整绑定、具名 skip 与清理记录保存在忽略目录，不随源码提交。

提交整理更新交付授权、最终结果与生产分流示例的查询保留，不改变上述运行载荷；
文档与质量检查再次执行。原生 Setup 对无效邮箱只显示 Unknown 的既有错误呈现登记为债，触发条件是
上游 Setup 错误处理 review 或明确初始化 UX 范围。本轮不把企业 SSO/RBAC、生产入口或
`make deploy-local-scenes` 记为已验收；`DATASET-04` 保持未完成。

## 实现与清理

恢复原 LakeFSApp、IndexPage、PluginManager、Layout、原导航和全部原路由；
原 API 路径与对象浏览/渲染实现、依赖和锁文件保留；统一登录纠偏只给原用户
profile 缓存补充失效代次，防旧请求迟到后把旧账号重新写回。
清除 Dataset 专用外壳、手工凭证提示、重复测试 helper 和无关 renderer 重构。
修复 `/ui` 导航、静态嵌入及缺失资产；Dataset 增加认证、Catalog、定义/发布、
固定来源预览、历史/定义比较与文件交付。KC 没有重新实现原 lakeFS API。
原认证保留浏览器重定向；OIDC 与 logout 接线由原 lakeFS 后端处理。
真实新构建登录暴露的 Vite 裸 `/ui` 刷新问题由 dev/preview 的 canonical 入口修复；
仅把 GET/HEAD 的该入口转到 `/ui/` 并保留查询，不改变原登录或深链接。

## 统一登录纠偏

用户确认 Datasets 不应单独登录。后续实现移除第二登录表单，复用原 lakeFS 会话，
KC 服务端验证并独立评权。下方两次登录的记录是纠偏前的历史证据，
不证明新的单次登录行为；新实现的真实后端与检查结果另行记录。统一登录部署、权限分责和账号映射边界见
接线文档与 Server README；本轮不会应用配置到现有实例。

## 统一登录后的真实验证

独立环境 `56008dfa48` 使用缓存的真实 lakeFS v1.86.0、MinIO 与本轮构建的 KC
Server，前端继续固定 v1.58.0。原用户 `console-validation` 通过原 lakeFS 登录页
获得会话，Dataset 直接复用同源 Cookie；KC 采用显式 `auth: lakefs` 与固定
`authURL`，部署初始化显式授权该测试用户，没有继承原平台管理员权限。

浏览器实际完成一次原登录、显式 Catalog 选择、来源校验、发布 `shared-login v1`、
交付目录与正文预览。来源 `reference` 映射到 `docs`，固定提交为
`52f3cfde7894475a4da7ea5b301a13e67057d8b3e31cb9548f680690dad06988`；
公开 KC API 回读发布来源与 pin，解码文件 45 字节与 Writer 来源逐字一致。
同一用户能进入原 Repositories/Objects 并读取相同正文。Dataset 使用原用户菜单
退出后，深链接回到唯一原登录；再次原登录保留 `?revision=1#history` 完整目标。

随后通过公开 API 撤销这一个独立测试用户的 KC grant，Dataset 刷新显示真实
`FORBIDDEN / catalog.read`，没有第二登录表单；同一原会话仍能列出三个原仓库。
同源 Cookie 的 whoami 与 Catalog 读取通过；缺少控制台标记或跨站请求明确拒绝，
KC 未提供 CORS 响应头。浏览器无 error/warn。此次未配置外部 IdP，原 SSO 路径
保留与回归通过不等于外部 SSO 实测通过。

无凭据证据与截图保存在 `.validation/console/20261010/single-login-*-proof.json`
及 `shared-login-*.png`。独立 KC/preview、两个带本轮标签的容器、网络、私有临时
目录与凭据均已清理，自建标签页已关闭；现有部署与原共享工作区未改动。

## 统一登录后的检查与源码边界

首轮源码指纹 `63ad2c26629354e7c30721ebd80ee8b7e4684df8d9b26b178dad308470e9509e`
前后一致：`make test-webui`（`20261010T100242Z-e8c56509cefc`）通过类型、lint、
13 文件 105 测试、生产构建与受影响 Go 回归；`make test`
（`20261010T100440Z-1ec4bf51c35e`）通过 21 个测试事件，无失败或跳过；
`make test-contracts`（`20261010T100639Z-67312cd0a3e8`）通过 1836 个测试事件，
无失败、20 个既有 live 条件跳过，结果为 passed-with-skips。

首轮 `make quality` 的文档检查通过，但新增认证模式使 `DeploymentConfig.Validate`
复杂度从 50 变成 51，触发既有上限。本轮把同一允许模式集合改为等价 switch，
没有调整阈值；同时把修改过的原 API 用户缓存文件加入永久前端 lint 入口。
这两处与本验证记录使源码指纹变化，因此首轮不能冒充最终源码验收。
最终源码的前端、宽 Go、质量检查结果与完整具名跳过项记录在本地
`.validation/console/20261010/single-login-final-checks.json` 和
`single-login-go-summary.json`；这些运行证据不随源码提交。

## 纠偏前独立真实后端与浏览器证据

使用已缓存的真实 lakeFS **v1.86.0**、独立 MinIO/S3 与 KC Server；Catalog 和来源
均使用 lakeFS driver，源文件更新经 Writer。前端仍固定 **v1.58.0**；未升级或安装依赖。
这次证据没有使用 mock 后端替代原 API。早期 mock 浏览器 fixture 已关闭、其临时文件
已清理；前端单元测试中的 mock 只证明隔离回归。

浏览器通过同源 Vite preview 连接两套真实后端，使用一次性原 lakeFS 用户身份和
独立 KC principal；没有把 KC 管理员凭证注入原控制台。没有操作现有部署或原共享工作区。

| 实际操作 | 观察与后端回读 |
|---|---|
| 原登录、Setup、仓列表/对象 | 原登录成功；未完成 setup 时显示原 setup 页，初始化后正常列仓；对象正文来自真实存储。 |
| 原仓库、分支、标签 | UI 创建 `console-browser-repo`、`browser-created`、`browser-created-tag`；原 API 回读真实对象与提交。 |
| 原 Changes/Commit | 75 字节文件通过原 API 上传到真实 S3；UI 显示 Changes、提交成功、变化清空，commit/message/bytes 回读一致。**文件选择器 UI 未验收**。 |
| 原 PR/Compare | UI 合并真实 PR，状态 Merged、合并 commit 一致；原比较页显示 baseline 到 main 的真实文件正文差异。 |
| 原 CSV viewer/下载 | 原 DuckDB-WASM 查询 `data/counts.csv`，显示 `console,1`，浏览器无 error/warn；原 API 下载与原 S3 viewer Cookie 链读取准确字节，无有效身份拒绝。 |
| 原 Actions/Settings/Administration | Actions 列表与 General/GC/Branch Protection 读取真实配置。社区后端的 RBAC `none` 显示原产品升级入口；不把它算成 RBAC 功能验收。 |
| Dataset 登录与选择 | KC whoami 验证后进入；首次显示 Catalog 选择，选中 Catalog 保留在 URL。原身份和 KC 身份互不串用，分别退出/刷新验证。 |
| Dataset 定义/发布 | 目录与单文件映射，Form/JSON 保真切换；未知字段明确拒绝；未预览禁止发布，定义变化使预览失效。 |
| 来源漂移与固定版本 | 预览 v1 后通过真实 Writer 推进源头，再发布；v1 仍使用确认的旧 commit。实际预览/下载字节保持 v1，新版显式采用更新后读取 v2。 |
| 历史、比较、退役 | UI 查看真实 v1/v2、交付映射差异与来源定义；历史 URL 刷新保持 revision。API `limit=1` 得到 v2 后 v1，退役后旧 pin 的 mounts/tree/file 均拒绝继续消费。 |
| Server embed | 真实 KC Server 的原仓与 Dataset 深链接返回同一入口；已有资产 200，缺失 js/css/png 404。 |

本地证据保存在忽略目录 [`.validation/console/20261010/`](../../../.validation/console/20261010/)：
`kc-console-real-api-evidence.json` 包含状态码、版本、真实提交与字节校验，不含凭证；
`go-summary.json` 包含各运行源码指纹、结果与具名 skip；另保存原提交、比较、CSV、
Dataset 比较截图及实际下载的 v1 文件。这些是本地运行证据，不随源码提交。
最后原认证与挂载复验见 `native-auth-final-api.json`：真实 backend `/logout` 的 307
经 preview 映射到 `/ui/auth/login`；OIDC 路径回读与真实 API 后端字节一致，未落到
独立 S3 后端。当前真实 fixture 未配置外部 IdP，其 OIDC fallback 200 不能算 SSO 成功。
最后构建还实测了无 `next` 的原登录，经过裸 `/ui` 的规范跳转进入 Repositories；
原退出回到 `/ui/auth/login`，刷新仍为登录页，浏览器无 error/warn。

## 纠偏前自动检查及源码边界

| 命令 | 已完成结果与范围 |
|---|---|
| `make test-webui` | 最后 `20261010T092854Z-7e8be1a1e452`：类型、lint、13 文件 81 测试、构建及受影响 cli/server/invocation 回归通过，`sourceChanged=false`。原页面全树纳入源码类型检查；库声明检查限制见下文。 |
| `make test` | `20261010T085444Z-769a2c5f3d51`：21 个测试事件通过，0 失败、0 skip。是该命令选择的产品/指标场景，不代表全部 Go 测试库存。 |
| `make test-contracts` | `20261010T085725Z-57e181c13cd7`：1815 个测试事件通过，0 失败、20 个既有外部/live skip。包事件 51 pass、10 no-tests/skip；结果是 **passed-with-skips**。 |
| `make quality`（含 `make check-docs`） | `20261010T090747Z-900dc13cc0eb`：文档图、gofmt/tidy/vet/staticcheck、复杂度/体积/克隆/耦合检查通过。 |

上述宽 Go 与质量检查都绑定源码指纹
`476874696dc78ca027adc5fc2a2f2fdfce4c29b6659ba7af05fd6f19f4bc359d`，
各运行前后相同，`sourceChanged=false`。其后只调整原认证导航/同源代理/裸 `/ui` 接线、
前端认证回归 fixture 和交付记录，生产 Go 实现未再变更。前端最后上述运行的指纹是
`61a8eaab5efe6750186a24880f0691d408aacf0c2c1c38bcc2128f39ecd8246a`；
其后仅改验证记录和认证 HTTP 测试的 Node 文件路径定位，定向复核同一代理场景。
最后接线回归、重新构建后的受影响 Go 以及最终质量复核分别记录在本地
`final-checks.json` 与对应日志；不能把早期共享目录的 source-changed 结果拼成当前验收。

## 剩余边界与债

- 原文件选择器采用 vendor 的 File System Access API，浏览器自动化未取得 chooser；
  没有改原上传实现或用 API 上传冒充 UI 上传。位置 `src/pages/repositories/repository/objects.jsx`；
  处置为保留真实 API/Commit 证据并登记 UI 验证缺口，触发为可操作该选择器的浏览器或人工验收。
- 外部 lakeFS IdP/SSO/SLO、Taihu PAR/PKCE、RBAC 管理、Actions hook 执行、生产 Nginx
  配置及 `make deploy-local-scenes` 未验收。原路由保留与接线回归不等于外部系统实测通过。
- 纠偏前辅助独立 SigV4 探针受夹具 21 字符测试 key 与 v1.86 parser 长度限制返回 400；
  原 Cookie/占位签名 viewer 已实际通过。未轮换原凭证或更改认证模式来使探针通过；
  独立 SigV4 客户端不计本控制台验收。
- `tsconfig.json` 当前 `skipLibCheck: true`，JS 为 `allowJs: true` / `checkJs: false`；
  锁定声明、工具链弃用提示、大 renderer chunk 和 Node/happy-dom worker stderr 均保留并登记于 TASK。
  真实浏览器 CSV 查询通过，不用修改生产 renderer 掩盖测试运行时问题。
- 原 `src/lib/hooks/api.jsx` 的登录返回只保存 pathname，丢失对象 URL 查询；统一登录
  接线已触发修复，改为保留 pathname/search/hash 并补回归，结果在新检查中记录。
- 官方 Dataset 元数据、定义审批、Iceberg、发布审计信息与消费协议差距为 API review 的 D-01–D-05。
- 新 lakefs 认证接线的账号代际仅能复用原 User API 的 id 与秒级创建时间，不能证明
  同名同秒重建不可接管。具体位置、判定、处置和触发见 TASK 的认证质量登记；
  单次登录走查不等于这个身份生命周期缺口已通过验收。原 lakeFS 账号 ID 与 KC
  用户名不兼容时明确拒绝，邮箱等账号的显式身份关联尚未实现，也不声称已验收。

本轮独立验证服务在证据保存后清理；仅删除带本轮标签的容器、独立网络、匹配临时
binary 的 KC 进程与一次性测试凭据/状态。preview 与自建标签页也关闭。
没有改动现有实例、共享工作区，或新增 skip。DATASET-04 仍为 `[ ]`，
遵守项目“全绿且无 skip 才可完成”的登记要求。
