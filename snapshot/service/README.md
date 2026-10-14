# snapshot/service/

Snapshot Store 的独立 HTTP 适配：文件 Dataset 的定义、固定来源解析、发布历史和原始
字节交付，以及同一显式授权账的自查与管理。完整控制台由相邻 `snapshot/console` 提供。

## 边界

核心调用 `snapshot/dataset`、`snapshot/access` 和 `snapshot.Store` 的文件能力。
一套服务只装配一份 Dataset authority，不接受 Catalog 选择或多空间 provider。
来源坐标是 Repository、commit 与文件路径/目录前缀；文件内容始终是不解释的字节，
不以 Object `object_id` 定位来源。Dataset 的历史 Dataset DTO 名称仍保留。

调用方通过 `Config.Acquire` 借出一次请求的 `Resources`，并负责释放租约、关闭顺序和
授权账的原子持久化。`Resources.Datasets` 直接持有服务唯一的 Dataset 登记权威，`Sources` 提供已打开的文件仓，
`access.RuleStore` 对授权检查与更新提供同一串行边界；不得以客户端断言代替已验证身份。
服务不提供第二份 Dataset、授权或身份账，不要求 KC Home、Invocation、Object 或索引。

`IdentityOptions` 使用显式配置的验证器；未配置不会自动接受 local 身份。
原 lakeFS 登录证明由 `NewLakeFSAuthenticator` 向固定的 native origin 校验。使用者先显式
初始化通用 `identity` 绑定权威，再以 `identity.WrapAuthenticator` 包装验证器。
lakeFS native 管理员身份不产生本模块的管理授权；退出也不删除耐久身份绑定或恢复 grant。
native 用户身份的秒级代际及兼容用户名限制与原接线相同，见控制台 API review 和 TASK 登记。

## 最小用法

```go
import (
    "net/http"
    "kc/snapshot/service"
)

// config 提供已配置的验证器、Dataset/文件仓及持久授权账。
func mountSnapshot(mux *http.ServeMux, config service.Config) {
    service.NewHandler(config).Register(mux)
}
```

`Register` 挂载已有身份发现/自查、授权规则、登记库存与审计、来源关联/解绑、Dataset 和文件交付路由，
以及 `/ui/` 静态控制台。路由和 DTO 由公开 Go 类型、实现与回归拥有。
控制台仍使用原 lakeFS API；同源网关需要将 native API、登录/退出和 S3 路径交给真实 lakeFS，
将本模块 API 交给此 handler。完整可审阅配置见
[控制台接线](../console/webui/KC-VENDOR.md)，模块不复刻 lakeFS 后端。

## 外部扩展与依赖方向

KC Server 通过窄回调提供既有默认政策、便携 recipe 准备、Object 准入、语义投影及观察
证据。独立文件服务不需要这些回调。发布仍由 Dataset 核心执行，授权先于领域操作；
`Around` 只可拒绝前置执行或观察真实结果，不能重复执行、伪造发布或覆盖领域错误。
来源库存的外部描述不能更改登记身份。请求元数据只供诊断，不能用于发权。

允许依赖 Snapshot 子包、通用 `identity` 与 `kernel`；禁止直接或传递依赖 `catalog`、
`home`、`invocation`、`invoke`、`server`、`cli`、`client`、Object、Index、Retrieval。
语义文件视图的解释和应用生命周期留在挂载方，原始文件算法只在本模块拥有。

## 验证

`snapshot/service` 回归覆盖身份/CSRF、完整 HTTP 形状、原子授权管理、固定文件读取、
真实历史、范围拒绝、分页及 hooks 至多一次执行；Server 的并发关闭测试通过本模块真实
HTTP 路由借用资源。架构守卫
`TestSnapshotStoreFileModuleHasIndependentOwners` 与
`TestLegacyCatalogDoesNotRetainDatasetAuthority` 检查所有权及依赖方向。
包内夹具回归不等于真实 lakeFS 验收；独立部署与浏览器证据单独记录。

`GET /snapshot/v1/repositories` 是控制台发布页的来源选择入口。
通过 `inventory.read` 后，只返回服务登记权威已登记的仓身份，维持已有
`repositories: [{id, ...}]` 响应；不枚举全部已连接 Store，不读取来源字节。
库存可见不产生来源消费或 Dataset 发布权。`Resources.DescribeRepository` 可为一个
已选定来源添加应用拥有的可选元数据（KC 沿用既有 schema count）；该扩展不能修改仓 ID，
默认模块只返回 `{id}`，库存与文件发布均不要求 Object 或索引实现。

服务已登记来源的 inventory 可由 `Resources.DescribeRepository` 提供
`consoleRepository`（可选原 lakeFS 仓名），用于进入完整控制台的固定提交比较。
宿主只能在该来源的真实存储实例与浏览器已认证实例一致时提供此值；不得猜测逻辑
RepositoryID 对应的原仓名，不得返回凭据。它只是导航元数据，不赋予原仓读权；
原 Repository Compare 使用原 API 与调用方自己的会话。独立文件服务可以不提供此值。

`GET /snapshot/v1/inventory` 返回同一登记 authority 的 Repository 描述和 Dataset 摘要，
不枚举全部已连接 Store，也不读取来源字节。`Resources.DiscoveryWorkspaceID` 是宿主可选的
导航说明，不建立另一个登记空间或授权决定。`POST /snapshot/v1/repositories` 只登记已经
由 `Sources` 提供且未归档的来源；可选 `BeforeAttach` 先核对宿主耐久连接/绑定，不能代替
模块接受登记或隐式发权。`DELETE /snapshot/v1/repositories/{repository}` 只移除登记，
保留来源文件和已接受 Dataset 历史，之后旧 pin 同样受当前成员准入约束。两项变更都需
该 Repository 范围的 `repositories.manage`，并在前置 Hook 后重新核对当前权限。
KC 的供给、归档与保留 System Repository 的政策仍由宿主装配，文件模块不解释对象身份。

`GET /snapshot/v1/audit` 需独立的 `registry.audit.read`，返回原 `source`、`authorityId` 和
`entries` 形状。`limit` 默认 50、最大 200，0 采用默认；不提供续页游标，非空
`continuation` 明确拒绝。历史能力缺失、来源历史读取或提交元数据回读错误直接返回失败，
不能伪装成成功的空历史。它读取登记提交，不读取来源 Repository 的正文或提交历史。

`PUT /admin/v1/grants/{grant}` 原子修改一条规则。请求中的 `principal`、`actions`、
`repository` 或 `dataset` 以及可选限定字段表达新规则；`expected` 包含旧规则的同名字段、
`id`、`sharedBy` 和 `shareId`。URL、旧快照与当前耐久规则必须指向同一唯一 ID，旧范围与
新范围分别以当前授权校验。冲突返回 HTTP 409，拒绝或持久化失败保留旧规则。
规则 ID 和来源信息不能由修改请求重写，分享产生的规则继续由分享管理维护。
授权库存和个人准入响应附带可选 `canManage` 提示供控制台显示修改入口；提示只反映
读取时的权限，不能替代修改时的服务端校验，也不代表有效权限全集。

Dataset 定义请求可以携带可选 `metadata: map[string]string`。省略或 `{}` 表示本次发布
没有自定义元数据；新发布提交完整对象，不继承旧版或按键合并。请求不接受非字符串值、
null 或重复键。候选准备、可选配方出版与最终接受均保留这些属性；当前库存和按 revision
选择的历史详情回读同一已接受记录。修改和清空需发布新 revision；不读取或识别说明文件，
不扩大文件范围，也不改变 Dataset 授权。

文件组合 API 使用 `/snapshot/v1/datasets`、`/snapshot/v1/datasets/{dataset}` 及对应
`resolve` / `check` / `retire` 动作；原始文件交付使用
`/snapshot/v1/files/mounts:list`、`tree:list`、`file:read`。请求只给 Dataset 与固定 pin，
旧 `/catalog/v1/catalogs/...` 路由和 `catalog` DTO 字段明确拒绝，不保留别名。
`inventoryDiscovery` 是上层库存发现政策的显式标记，不改变文件清单、准入或授权。

## 原 Repo API 的强制检查

`Config.NativeOrigin` 是固定可信的原 lakeFS 地址，不能包含凭据、查询或片段。
`Resources.NativeRepositories` 来自实际登记，将同实例原仓名映射到逻辑 RepositoryID；
歧义、未登记和不同实例均拒绝，不能从 kr:// 名字推导。
`GET/POST/PUT/DELETE /api/v1/` 和原 PR 的 PATCH 保留原传输形状；
`nativeRepositoryRoutes` 为每个 Repo 操作选择 `snapshot/access` 的明确动作，未知操作拒绝。
GET 本身不代表只读，例如 staging/backing 需要单独的导入能力。

原仓可选自定义元数据直接使用原生 `/api/v1/repositories/{repository}/metadata`：
`GET` 返回字符串键值表；`POST` 接受 `{metadata: {...}}`，只新增或更新给定键；
`DELETE` 接受 `{keys: [...]}`，只删除指定键。两种修改返回原生 `204`。
读取要求 `repository.metadata.read`，修改要求 `repository.metadata.write`，
均按实际原仓名绑定的逻辑 RepositoryID 逐请求求值。唯一权威是 lakeFS 原仓元数据，
网关保留原请求与响应，不写入登记账、不冻结到来源 commit、不从文件生成值；
所有键都作为不解释的字符串处理，不用于身份、来源选择或授权。

认证复用现有验证器与耐久身份绑定。Taihu/Gitea 等外部认证模式及显式本地测试身份下，
`Config.NativeCredential` 只在服务端提供私有存储证明；每个请求先验证真实主体，再检查
当前产品授权。改写后删除用户 Authorization、Cookie、X-Kc、X-Tai 与转发身份头，
仅对固定 native origin 设置存储证明，绝不把该证明返回浏览器。
缺少私有证明失败关闭，不回退用户证明。`GET /api/v1/user` 只显示已验证主体；
native 账号、凭据、策略、组、初始化接口拒绝转发。账号入口由已有认证系统承接。
遗留 `auth: lakefs` 显式模式才按原调用者证明转发，并保留其原认证器限制。
认证管理员、存储管理员或组成员资格均不产生本模块 grant。
KC 只在明确 `auth: local` 且可信本地身份成立时接入同一私网证明路径；空模式或未配置
身份不借用私网凭据。既有 lakeFS 身份模式不扩大；服务身份的 setup/config 显示既有
提供方，不要求使用者初始化私有存储账号或填写 vendor 用户配置。
Repo 查询不使用库存默认政策、Dataset、Object 或 Aspect 规则；Ref 是精确范围，
文件路径不进入 Object 字段。库存按当前元数据权限过滤，游标仅来自可见仓。

下载、表格预览和 Range 读取走同一受控原 API。`pre_sign_support_ui` 关闭，避免上传/
下载绕过检查，原文件流保持字节与响应头。同源浏览器文件 GET/HEAD 可使用 Fetch Metadata，
修改仍要求同源标记；multipart 例外仅限原上传，本模块 JSON API 的检查不因此放宽。

可选 `Resources.CreateNativeRepository` 复用宿主耐久分配、登记和初始授权，返回真实仓名与
逻辑身份。KC 使用已配置单个 managed pool、默认分支及原创建政策；表单明确展示此模式，
不忽略自定义 namespace/分支/样例数据。未配置生命周期时明确拒绝创建。
creatorActions 显式选择初始权限，重试不恢复撤权；创建、导入、物理删除和发权各自授权。

生产 native 数据地址必须仅在服务内部可达，外部 `/api/v1` 全部走本模块。
另一个用户可直达的 native 数据地址会绕过检查，不构成已验收部署。
仅按部署声明分流已有认证网关的退出/回调，不把这些路径默认交给私网存储服务；
不保留任意路径的 native fallback。

导入与物理地址关联检查后端真实 namespace 中每个相交来源仓的 `file.read`，同时检查
目标 `repository.import`；未登记来源、未知 namespace、路径上跳或无版本坐标的 ref
限权读取都拒绝。宽前缀不能越过子 namespace。任意外部介质导入尚无来源授权合同，
明确拒绝，不能把目标导入权解释为存储凭据能读取的全部位置。
后端失败记录为失败，私有存储认证错误返回服务不可用，不使有效业务用户被错误登出；
后端会话 Cookie 与受保护数据重定向不能进入客户端。

当前物理删除入口先核对 `repository.delete`，随后拒绝删除仍在宿主绑定中的仓：
尚无解绑部署连接、托管恢复记录与 Dataset 来源的耐久生命周期。直接转发 DELETE 会
使恢复失败，因此不能将此入口写成已完成。原控制台保留删除入口及明确错误，后续需
由宿主提供完整解绑流程，不能删除账或假造成功。已有分支、标签及文件删除照常执行。

原生 GC 设置读取仍按 `repository.settings.read`。PUT / DELETE `settings/gc_rules`
先核对 `repository.settings.manage`，再以 HTTP 409 拒绝：当前没有证明已提交历史及
Dataset 固定依据仍可回读的策略验证能力，不能直接转发时间保留或默认 GC 策略。
拒绝发生在私网请求之前，不执行真实 GC。私网直接管理须遵循部署保留合同，此代理
拒绝不能保护绕过产品入口的存储管理员操作。


宿主 Authorize 适配还需保留实际 Store 的只读/归档边界。KC 原仓写操作检查字面文件写入能力，原 API 不得绕过只读 system 或只读连接；这不改变读取授权。

## Dataset 发布的来源许可

发布分别检查目标 `dataset.manage` 与每个来源的 `repository.dataset.publish`。来源许可
以 Repo、目标 Dataset 与选定 Ref 求值；显式 Commit 使用该 Commit，未固定来源使用
selector（空值为 `snapshot.DefaultRef`）。普通仓读取、Dataset 管理和 `dataset.*` 不隐含
分享来源权。独立服务与 KC 宿主的自定义授权都使用 `access.MatchSourcePublication`。

来源授权先于来源检查与可选 portable-recipe 副作用；外部 before-hook、`BeforePublish`
及 `PreparePublication` 之后再次求值。selector 被服务器冻结为 Commit 后，最终复查仍
使用调用方的原选定坐标；`BeforePublish` 返回的新选择要重新授权，最终准备不能原地
替换冻结的来源。撤权或替换失败不接受发布、不推进登记权威和当前版；之前独立源提交
仍成立。可信 programmatic Dataset 端口不携带 principal，强制检查由产品服务承担。

此许可仅保护发布接受，已发布版本的 resolve、pin 与文件消费沿用当前 Dataset 授权和
已接受文件范围，不逐读追加源发布许可。来源 grant 的管理边界与明确 CreatorActions
选择见 [`access/README.md`](../access/README.md#dataset-来源发布许可)。

KC HTTP 的 `EnsureDataset` 只确认已接受定义，非部署 Home 也不从消费请求自动采用
portable recipe。缺失定义返回原错误；首次采用须经显式 `dataset define` 完成目标管理与
来源发布授权。可信 embedded Invocation 的配方采用端口仍保留。其它宿主若通过可选
`Resources.EnsureDataset` 采用配方，也须在一次性接受前完成同样的发布授权，不能借
消费许可创建发布；正常消费不重复检查源发布权。

## 命名 Dataset 的发布版本选择

`POST /snapshot/v1/datasets/{dataset}/resolve` 可带正整数 `revision`，从已接受
发布记录重建固定来源与文件范围；省略仍选择当前版。`revision` 与 `pin` 互斥，
显式 null、零、负数或非整数拒绝，且输入校验先于可选配方采用。`check` 保留既有
pin 校验合同，不接受 `revision`。正文可用性检查先从已接受发布重建可信 pin，再逐个
固定来源按实际 Repository/commit 核对当前 `file.read`；零或部分消费许可均在任何正文
探测前拒绝，不借用管理或来源发布许可。解析仍需当前 `dataset.resolve`，历史选择不借用
管理详情、不要求额外 `inventory.read`，也不绕过当前退役、成员登记或来源可用性。
显式发布版不会触发配方首次采用或创建发布记录；省略版号解析当前已接受版。可选宿主
协调不改变上述首次发布许可，KC HTTP 消费始终不隐式创建发布。

现有详情 GET 的 `revision`、`limit`、`continuation` 仍属于管理记录读取：详情采用
选定发布的属性与来源，历史名单只列已接受版本，当前退役政策继续显示。消费解析
与管理详情分别授权，取得详情不授予文件读取权。

## 交付目录分页

`tree:list` 的交付寻址只枚举一个目录的直接子项。每页最多返回 `limit` 个子项，
零使用既有来源默认大小 500，负数拒绝；不新增 HTTP 页大小上限。固定顺序是先按名称
分页交付声明派生的子项，再沿来源 `DirectoryReader` 的原有分页顺序继续，不要求来源
提供全局字典序。来源段每次最多请求一页；不全量读取来源树，也不读取文件正文。

交付游标只保存位置，绑定 Dataset、已接受 revision、pin、目录、完整占用范围与本次可见文件范围。
跨范围游标拒绝；当前可见范围变化时要求重新开始，游标不保存或授权旧可见子项。
纯 prefix 的挂载相对寻址继续使用来源自己的游标。literal 布局在该目录包含逐文件选择
或显式挂载遮蔽时，挂载相对坐标先经 `RepositoryPath` 检查，再映射到相同的交付目录与
合成分页；混用交付 `path` 和挂载相对坐标明确拒绝。外部语义视图仍使用自己的字节和
目录端口，不转入 literal 路由。
完整清单的显式挂载和逐文件条目也占用其祖先目录；当前权限隐藏条目后，祖先不能
退回根来源的同名文件。文件读取在正文探测前拒绝隐藏声明目录，目录页根据已授权来源
返回的子项类型过滤被排除的祖先文件；实际目录内其它获准根来源文件仍可消费。

来源与派生项的同名目录只交付一次；文件同名或文件与目录冲突在遇到该来源页时拒绝。
未完成分页不证明整个目录没有冲突，直接文件读取仍独立拒绝冲突字节交付。来源返回
超过请求条数、不同 generation 或未完成却缺失/重复的续页位置时失败关闭；空页只有
来源位置实际推进时才可续。最终 `exhausted` 只在两个段都完成后成立。

文件读取的两种地址复用 Dataset 的固定来源编译与有界冲突检查。完整已接受 pin 决定
占用与排除范围，当前可见 mount/item 决定可以交付的内容；隐藏显式文件或挂载仍遮蔽
根 fallback，不因过滤权限改读另一来源。旧坏清单的同名、祖先文件和后代目录冲突
在读取时失败关闭；冲突依据来源不可读时拒绝，不探读隐藏来源。
resolve、mounts 与正常目录分页只复核已接受坐标，不以正文探测代替消费授权。
