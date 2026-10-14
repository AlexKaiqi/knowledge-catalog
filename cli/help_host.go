package cli

func hostEnvironmentHelpEntries() map[string]helpEntry {
	return map[string]helpEntry{
		"login": {
			Description: "验证身份并保存当前 Server 的本机会话",
			Usage: `kc login [--mode taihu|token|local] [--wait]
  用途：
    为后续 CLI 请求准备当前 Server 的认证证明。
  用法与参数：
    Server 来自 --server、KC_SERVER_URL 或 client.json；只有 local 模式使用 --as。
    --mode token 使用 --token 或 KC_AUTH_TOKEN；local 使用 --as 或 KC_AS。
    Taihu 首次执行打开授权链接，浏览器完成后再执行 kc login --wait；模式须与 Server 配对。
  条件：
    Server 的认证发现与身份验证可用，本机配置目录可写。local 只适用于显式 local 部署。
    普通用户无需部署应用秘密；Gitea/lakeFS 登录使用认证系统已经签发的 Bearer token。
  结果：
    token/local 或 Taihu 等待阶段验证并保存成功后输出 authenticated 和 principal。
    Taihu 发起阶段输出授权链接与 next_step，尚未完成登录。
  后果：
    保存按 Server 隔离的本机凭证或 local 身份，并更新默认 Server；不登记业务范围或发权。
    Taihu 发起阶段还保存待完成的授权材料；后续请求仍按当前身份和权限检查。
  风险与成本：
    有认证网络请求；Taihu 等待最多五分钟。保存的 token/refresh token 是本机敏感材料。
    --token 命令行可能进入 shell 历史，优先使用 KC_AUTH_TOKEN；登录成功不保证业务动作获准。
  失败恢复：
    配对错误先核对 Server 与 mode；授权过期或待登录材料不匹配时重新发起。
    本机保存失败不算登录完成，修复目录权限后重试；无业务权限时用 kc admission show 查申请入口。
  示例：
    例：kc login --mode local --as kaiqidong
    例：kc login --server https://kc.example --mode token
    例：kc login --mode taihu
    例：kc login --wait`,
		},
		"logout": {
			Description: "清除当前 Server 的本机会话",
			Usage: `kc logout
  用途：
    移除本机保存的当前 Server 登录证明。
  用法与参数：
    用 --server 指定目标，或沿用 KC_SERVER_URL、client.json 中的 Server。
  条件：
    需要能确定 Server；本机相应会话文件须可删除，无需先通过业务授权。
  结果：
    清除成功输出 status=logged out。
  后果：
    删除这个 Server 已保存的 token/local 会话，保留其它 Server 的登录态和默认服务地址。
    不撤销认证系统中的 token、grant 或浏览器网关会话；环境变量提供的证明仍由调用方控制。
  风险与成本：
    仅操作本机凭证文件；正在执行的请求可能已取得证明，退出不能撤回已接受的操作。
  失败恢复：
    删除失败时修复本机文件权限后再次 logout；再次使用服务需登录或显式提供有效证明。
    若要让已签发 token 失效，应在原认证系统退出或撤销。
  示例：
    例：kc logout
    例：kc logout --server https://kc.example`,
		},
		"whoami": {
			Description: "显示当前认证主体",
			Usage: `kc whoami
  用途：
    确认当前请求在 Server 上被识别为谁。
  用法与参数：
    使用当前 Server 与本机登录证明；可用 --server 选择部署。
  条件：
    认证证明有效且与 Server 认证模式匹配，身份服务可用。
  结果：
    返回 Server 验证的 principal，以及适用的服务身份或代理关系。
  后果：
    不改资源或授权；读取已有登录态，必要时刷新凭证并保存续期结果。
  风险与成本：
    一次身份查询及可能的续期请求；身份确认不表示已拥有某仓或 Dataset 的权限。
  失败恢复：
    核对 Server，凭证失效时重新 kc login；查询成功但业务拒绝时查看 kc admission show。
  示例：
    例：kc whoami
    例：kc whoami --server https://kc.example`,
		},
		"admission show": {
			Description: "显示本人已有权限和外部申请入口",
			Usage: `kc admission show
  用途：
    查看本人现有规则，并找到部署声明的外部权限申请渠道。
  用法与参数：
    使用当前 Server 与登录证明；可用 --server 指定部署。
  条件：
    当前身份通过认证，Server 的耐久授权账可用。
  结果：
    返回本人的 grants 及其仓、Dataset、ref、对象或 Aspect 范围、可管理标记，
    同时给出已配置的 request URL 和当前 grant 管理者；未配置 URL 时不会生成申请链接。
  后果：
    只查询已有规则，不提交申请、审批或发权，也不补回已撤销的权限。
  风险与成本：
    一次授权账查询；列出的规则有各自作用范围，不能据此推断某个具体操作一定获准。
  失败恢复：
    认证失败先登录；授权账不可用联系部署运维。缺少所需规则时按外部入口或管理者申请。
  示例：
    例：kc admission show
    例：kc admission show --server https://kc.example`,
		},
		"deployment init": {
			Description: "初始化部署",
			Usage: `kc deployment init --config <deployment.yaml>
  用途：
    由部署运营者显式建立首次运行所需的耐久状态和系统知识出版物。
  用法与参数：
    --config 是部署文件路径，不是登记空间标识。
    配置声明登记权威、实际 system 仓、独立持久 stateDir、cacheDir、认证和 bootstrapPrincipal。
  条件：
    配置与部署凭证有效，目标权威可达，持久状态目录可写且未丢失已有部署的账本。
    配置必须绑定实际 system 仓；初始化发布系统知识，启动只核对。
  结果：
    成功输出 initialized=true，表示初始化或既有初始化核对完成，不表示服务已监听。
  后果：
    首次建立控制账、身份账、初始化回执和初始管理员规则，发布并登记系统知识。
    再次执行保留既有状态、撤销和回执；不启动 Server、lakeFS 或索引服务。
  风险与成本：
    会写登记 Snapshot、system Snapshot 和持久卷，需要远端读写与出版物校验。
    首次初始化跨权威和持久卷，不提供跨介质事务；中断后可能已有部分耐久结果。
  失败恢复：
    配置错误先修配置；既有卷、登记权威或系统出版物丢失/漂移时恢复原权威。
    权威已写而状态未安装时由运营者核对未完成阶段，不删除状态标记以重新发管理员权限。
    完成后用 deployment status 检查，再显式运行 serve。
  示例：
    例：kc deployment init --config deployment.yaml
    例：kc deployment status --config deployment.yaml`,
		},
		"deployment status": {
			Description: "查看部署恢复状态",
			Usage: `kc deployment status --config <deployment.yaml>
  用途：
    检查声明的部署能否打开既有状态，并查看登记权威与来源清单。
  用法与参数：
    --config 指向部署配置文件；本命令直接检查部署，不使用客户端登录选择范围。
  条件：
    已有初始化回执与耐久账完整，部署凭证和实际 system/登记权威可用。
  结果：
    返回 status=ready、registry.head 与 repositories；ready 表示本次恢复核对通过。
  后果：
    不初始化、补发布或修复权威；可在部署持久状态中追加本次管理动作的审计记录。
  风险与成本：
    打开和校验权威会产生网络读取；结果是本次观察，不保证每个来源持续在线、
    检索投影就绪或某个用户拥有业务权限。
  失败恢复：
    按错误恢复原持久卷、凭证或权威，缺失的已有状态不能用 init 重建为空。
    状态核对通过后仍须由 serve 启动服务，并针对实际业务验证读取或发布。
  示例：
    例：kc deployment status --config deployment.yaml`,
		},
		"deployment system publish": {
			Description: "把系统知识出版物写入绑定仓",
			Usage: `kc deployment system publish --config <deployment.yaml>
  用途：
    显式发布或核对当前二进制版本对应的完整系统知识出版物。
  用法与参数：
    --config 指向声明实际 kr://kc/system Snapshot 与登记权威的部署文件。
  条件：
    部署状态可恢复，配置明确绑定实际 system 仓，部署凭证有相应操作能力。
    目标为空才发布；已有完整且相同的出版物只核对，不产生新提交。
  结果：
    返回系统出版物发布/核对结果及所采用的版本依据。
  后果：
    空 system 仓经专用 Writer 写入出版物；本入口不改变仓成员登记，也不会启动服务。
    已有不同内容、缺失或额外内容文件时拒绝，不覆盖为当前版本；运行时 Writer 仍不能写系统仓。
  风险与成本：
    需要完整出版物校验和 Snapshot 写入；发布与部署控制账不构成跨介质事务。
    内容版本必须匹配部署二进制，不能把本命令当成任意覆盖或升级迁移入口。
  失败恢复：
    恢复正确的仓绑定、凭证和系统权威后重试核对；漂移先明确恢复/升级方案。
    出版或审计中断时核对已经完成的结果，不假定所有写入已回滚。
  示例：
    例：kc deployment system publish --config deployment.yaml`,
		},
		"deployment identity migrate": {
			Description: "迁移历史身份绑定",
			Usage: `kc deployment identity migrate --config <deployment.yaml> --file <request.json>
  用途：
    把核验过的历史 provider 主体绑定迁到规范用户名，保留现存规则与托管归属。
  用法与参数：
    --config 是部署文件；--file 是身份迁移请求 JSON。
    请求含 legacyPrincipal 与 user 的 provider、issuer、subject、username；仅支持 Gitea/Taihu。
  条件：
    运营者已独立验证可信 issuer 和不可重新指派的 subject；用户名相同不足以授权迁移。
    Gitea subject 必须是稳定正整数；停止服务，避免与在线 grant 修改并发。
  结果：
    返回 APPLIED 或 REPLAYED、规范 principal 和 migratedRules 数量。
  后果：
    写入身份别名、迁移仍存在的 grant 主体，并在策略账保存迁移回执；不重发创建者权限，
    不恢复已撤销规则，历史托管请求保留原身份与请求摘要。
  风险与成本：
    身份账与策略账分别耐久写入，没有跨文件事务；错误绑定会改变现有权利的归属。
  失败恢复：
    同一明确绑定可重试；同历史主体的不同请求返回 IDEMPOTENCY_CONFLICT。
    写入中断时先核对两份账，使用原请求恢复，不能用同名新账号替代原可信 subject。
  示例：
    例：kc deployment identity migrate --config deployment.yaml --file migrate.json`,
		},
		"create": {
			Description: "创建托管 Repository 或连接自有 Repository",
			Usage: `kc create (--name <名称> [--store <store>] | --url <url> --credential-file <path>)
  用途：
    获得平台供给的新仓，或把本人已有的 lakeFS 仓连接到当前服务。
  用法与参数：
    --name 与 --url 互斥。--name 是平台供给空仓；--url 是连接已有仓。
    多个托管 Store 时用 --store 选择；连接模式必须给 --credential-file，不能带 --store。
    LakeFS 上 --name 就是仓 ID，与 lakeFS 仓库名相同。
    kc create 不接受 --metadata，也不设置原仓属性；该参数只用于 dataset define 的可选 Dataset 属性。
  条件：
    已认证且当前主体有供给/连接准入；部署开放相应能力。
    仓名须为 3–63 位小写字母、数字或连字符，kc- 前缀保留给平台仓。
    连接 URL 须在部署批准的 origin 内，凭证能只读验证已存在的非空 Snapshot。
  结果：
    返回仓标识、管理信息与耐久创建/连接结果；创建完成不代表检索就绪或持续健康。
  后果：
    名称模式创建 authority 并记录供给进度；连接模式只读验证原仓、保存服务私有连接凭证。
    初始权限只按部署明确策略应用一次；相同请求恢复原结果，不补回已撤销的权限。
    创建或连接不自动登记到服务库存；登记使用 kc attach。
    Repository 自定义元数据由 lakeFS 原仓按字符串键值维护，不随 commit 冻结，KC 不复制到登记账；Dataset 不自动继承来源属性。
    创建/连接后需查看或设置属性时，打开本服务 /console → Repositories → 目标仓 → Settings → General 的“Repository 自定义元数据”。
    读取需 repository.metadata.read，新增/更新键或删除键需 repository.metadata.write；进入页面仍受原仓浏览权限检查。
    Dataset 属性另用 dataset define --metadata 或输入配方随 revision 发布，历史属性用 dataset show --revision 回读。
  风险与成本：
    创建消耗远端仓和对象存储；连接将用户授权的 credential 保存在服务私有耐久账。
    供给、连接记录与授权分阶段完成，不提供跨介质全部回滚；初始 grant 不保证后续仍有权。
  失败恢复：
    用原名称、Store 或相同连接请求重试恢复进度；不要换名称制造第二个仓。
    凭证、准入或已占用绑定错误须先修正；成功后按需 attach，业务仍受当前权限检查。
    原仓元数据页面无法进入或编辑时，请来源管理者核对浏览及元数据权限；多项属性保存失败先回读实际结果。
  示例：
    例：kc create --name table-meta
    例：kc create --name table-meta --store lakefs
    例：kc create --url https://lakefs.example/table-meta --credential-file credential.json`,
		},
		"serve": {
			Description: "启动 typed API Server；浏览器打开根路径会 404",
			Usage: `kc serve --config deployment.yaml [--listen <address>]
  用途：
    恢复部署状态并运行面向客户端的 typed API 服务。
  用法与参数：
    --config 为已初始化部署配置；--listen 覆盖配置监听地址，均未声明时为 127.0.0.1:7380。
    认证方式来自配置，可显式 --auth 与 --auth-url；密钥由部署环境注入。
    这是 typed API，浏览器打开根路径会 404，不是敲 kc 的页面。浏览器控制台入口为 /console。
  条件：
    已有部署状态、身份和授权账完整，实际 system 仓与当前出版物匹配，
    配置的权威、认证器及启用的运行能力可用；先由 deployment init 完成首次初始化。
  结果：
    进程打印地址与启用能力并保持运行；真正监听失败或恢复失败会返回错误。
  后果：
    启动 HTTP 监听和已配置的投影维护，处理请求与耐久审计；启动仅恢复和核对，
    不初始化权威、不补发布系统内容、不重发 bootstrap 权限。
  风险与成本：
    长期占用端口、连接和后台维护资源；监听到外部网络须配合部署认证与网络边界。
    服务可监听不代表每个来源、检索能力或用户操作均就绪；已接受的请求不因退出自动撤销。
  失败恢复：
    监听冲突修正 --listen；配置、凭证或权威失配时恢复原部署后再启动。
    Ctrl-C/SIGTERM 触发最多十秒的 HTTP 关闭等待；写入结果不确定时按原 command-id 查回执。
  示例：
    例：kc serve --config deployment.yaml --listen 127.0.0.1:7380`,
		},
	}
}
