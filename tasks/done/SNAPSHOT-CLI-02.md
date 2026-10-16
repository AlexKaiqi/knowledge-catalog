# SNAPSHOT-CLI-02 · Snapshot 缺失入口的行为合同选择（2026-10-13）

- [x] 本轮完成合同选择：按此前第二批约定，选择 Snapshot 缺失 CLI 入口的任务、输入输出、权限、固定版本与失败边界；交付可实施合同及前置缺口，四个新命令尚未实现。
- Goal / owner：让消费者核对 Dataset 已接受定义与历史、按需枚举交付目录和读取单文件，让管理者原子修改既有授权规则。交互归 `doc/design/affordance.md`，文件选择/版本归 `dataset.md`，网关与客户端边界归 `service.md`，当前授权归 `permissions.md`；owner 与直接关系按 `docs/graph/` 核对。
- Non-Goals：本批不新增执行命令、不改业务代码或公开类型、不迁移对象层、不包装全部原生 Store API、不增加 pin 文件/消费会话、不纳入独立 kcfs / datasetfs 的挂载与生命周期重构、不安装依赖、不启动部署服务、不提交；保留第一批及其它会话在途改动。
- 不变量：V-01、KS-01/KS-02、AUTH-01～03、API-01、A-01/L-01；详情可见不发消费权，交付布局不扩大来源文件范围，显式旧版不会自动漂到当前版，固定依据不绕过当前权限，规则更新不拆成撤销再新增。
- 选定方向：优先 Dataset 详情/历史与两个按需文件入口，复用同一 Server、当前身份及文件网关；授权原子修改复用既有服务更新边界。文件浏览不限于项目附着，但始终是交付文件访问；设计区分它与知识发现，并保留挂载/整体本机采用的原边界。
- 选定 CLI：`dataset show` 查看指定发布及一页真实历史；`files list` 返回一页交付目录直接子项；`files read` 默认返回有界字节块，指定本机目标时完整下载一个文件；`grant update` 用完整新规则和此前观察到的旧规则做条件原子替换。文件路径都属于 Dataset 交付树；公开命令不暴露内部 pin，文件续页须显式选版，游标绑定 Dataset、版本与目录。新入口尚未实现。
- 输入与本机交付：详情可按发布版和历史游标读取；文件首次省略选版时只取当前一次，后续采用返回版号，JSON 字节编码沿现有 Client 合同。完整单文件下载与字节范围参数互斥，不提供原始字节 stdout，保留现有错误/回执流；目标不覆盖既有文件或符号链接，使用同目录临时文件并在完整成功后发布。授权更新的旧、新规则分别从文件输入，复用单条 Rule 的库存表示；typed 适配显式转换为已有 PUT 合同，身份和来源不得伪改，发送前不拉新规则覆盖旧观察。
- 精确调用方向：`kc dataset show <id> [--revision <n>] [--limit <n>] [--continuation <cursor>]`（也可 `--dataset`）；`kc files list --dataset <id> [--revision <n>] [--path <dir>] [--limit <n>] [--continuation <cursor>]`；`kc files read --dataset <id> [--revision <n>] --path <file> ([--offset <n>] [--length <n>] | --out <file>)`；`kc grant update --id <id> --file <replacement-rule.json> --expected-file <observed-rule.json>`。根目录仅用于 list；详情的 limit 只控制版本名单。四个入口的权限依次沿库存读、Dataset 解析与文件读、旧/新范围管理权，不额外发权或要求消费者取得库存管理权限。
- 否决：用管理详情拼消费 pin、要求消费者手抄 pin 或创建服务端消费会话、只在 CLI 截短非有界目录结果、将字节范围响应承诺成源端 Range IO、将原生仓名猜成逻辑 Repository ID、提交前自动刷新 expected，以及把 remove/add 当原子修改。
- 接口指向：Dataset 管理详情沿 `snapshot/dataset.ManagementView` 和现有 Snapshot GET；文件沿 `client.WorkspaceFilesService` / `snapshot/service.FileView`，消费版本由服务接受记录解析；授权沿 `snapshot/access.Service.Update` 和既有 Admin PUT。后续实施补 typed Client 与 CLI 接线，所需公开形状只在代码/包 README/Conformance 落地。
- 代码落点：详情加到第一批的 `cli/remote_dataset.go`，文件请求及单文件交付收拢为同包 `cli/remote_files.go` / `dataset_file_read.go`，规则更新沿 `cli/remote_access.go`；叶子说明沿 `help_snapshot.go`，root / composition / dataset / files / grant 逐层可达，同源系统入口由现有生成器发布。`surface.go`、分派、CLI 应用登记与 HTTP 登记继续各自维护，不引入新的标签或注册框架。
- 边界纠偏（2026-10-13）：用户明确排除 kcfs。它消费 Snapshot 文件网关，但宿主挂载计划、FUSE、卸载和进程生命周期独立；`cmd/kcfs`、`cli/datasetfs*.go`、根 `datasetfs/` 均保持在 Snapshot 目录之外，不纳入本批分组或迁移。`files list/read` 仍是 Snapshot 文件交付的产品 CLI 适配，不是挂载实现；跨工具 help 链接不表示模块归属。
- 前置缺口：现有命名 Dataset resolve 缺发布版本选择，交付目录的派生/混排分支尚未严格分页；两项须随文件入口实施并补真实回归。仓属性的 SDK 接收原生仓名，可信逻辑 ID 到原仓/实例的定位合同未闭合，metadata 读/设置/删键入口独立后续选择，不靠可选控制台导航补足。
- 质量出口：`snapshot/service/file_view.go` 的范围读取先整文件读取再切片，判定为既有来源 IO/内存能力差距；本批明确只选返回字节块和单文件交付，不声称源端资源有界。触发大文件源端按需 IO 保证时，须由来源 Range 能力与服务合同单独实现、验证；本批不静默扩大承诺。
- 验证计划：基于公开代码与现有 Conformance 做独立审查；改动 owner 与图登记重读后运行 `make check-docs`、`make quality`。行为尚未实现，不用第一批或既有 HTTP 测试宣称新 CLI 已验收。
- 后续实施验收：真实 argv 从库存选 Dataset、查看定义、分页获得交付路径、按同版读取/下载；目标放到第二页以后并覆盖纯文件、目录映射及混排。分页不得超限、重复、漏项或接受异版/异目录游标；发布后旧版连续，退役/撤权/来源失效明确失败。二进制字节不改写，已有目标及失败后的目标保持原状；规则更新覆盖正确字段转换、旧/新范围鉴权、过期观察冲突与失败保留旧值。缺权、空目录、未完成分页和不可用分别表达，不使用 skip 或削弱既有断言换绿。
- 最终复核与验证：以本批开始的工作树为基线，只新增任务记录并修改 CLI / Dataset / Service owner 的任务和保证；公开字段、错误代数和状态机仍由 Go / 包 README / Conformance 拥有，图的 owner 与直接关系不变。独立复核确认固定版本、分页、当前权限、条件更新和成本边界闭合，`make check-docs`（32 文档 / 149 关系）、`make quality` 与 `git diff --check` 通过；质量工具复用本机缓存。未改业务代码、未新增入口、未做新入口行为验收、未提交。
