package cli

type helpEntry struct {
	Description string
	Usage       string
}

var leafHelp = map[string]helpEntry{
	"read": {
		Description: "按固定声明依据读取对象或 URN 指定的数据",
		Usage: `kc read <object-urn[#/json/pointer]> [--shape '<graphql>'] [--repo <id> | --dataset <id> [--revision <N>]] [--commit <id> | --ref <name>]

用途：
  读取来源对象的完整内容或指定数据；同一个引用可随后交给 writer put/remove 维护完整单元。
用法与参数：
  位置参数接受对象 URN；# 后是 JSON Pointer，空 locator 选整个对象。
  /columns/order_id 选 Member；/columns/order_id/constraints/nullable 选其中的字段；/tags/0 选数组首项。
  键中的 / 写 ~1，~ 写 ~0；不支持通配、过滤或把点当层级。深层读取不会新增写粒度。
  --shape 用 GraphQL selection set 写出要的结果形状：顶层是方面，方面上 keys:["k"] 选成员，下层是字段；
  不写子选择返回该处完整值。只做投影，别名、fragment、directive、变量一律拒绝；URN 带 # 时不能再加 --shape。
  每个所选方面或成员先按维护坐标规划、只读所选单元并只为它们取外部值，字段在单元读出后即裁剪。
  --dataset 限制统一引用的交付范围；此时 --commit 须匹配发布版中的来源 commit，不接受 --ref。
  --revision <N> 固定 Dataset 的已发布修订，与 search/relations/traverse/schema 沿用同一个 N 时各步看到同一组来源 commit；省略时用当前修订。
  省略 --repo 与 --dataset 时，在你可读的全部仓里按 ID 解析：多个仓各持有不同方面时组合返回，同一方面来自两个仓时报 OBJECT_ID_CONFLICT，无权的仓列在 withheld。
  --repo 只读该仓，可加 --commit 或 --ref；省略时读取 refs/heads/main（published）的当前 commit。
  对象身份只由 URN 指定，内容只由 locator 选择；sources 记录每个方面来自哪个仓、哪个 commit。
条件：
  已认证；单仓需 object.read，命名 Dataset 按 file.read 解释发布版清单内材料。
 Dataset 须通过对象与选择范围的完整读取检查，不能越过已获准的范围读取源仓。
 Dataset 选中 Bound State 时，沿既有 logical READ 通路，还需相应 Runtime 可用并允许实际源访问。
结果：
  统一引用的 JSON 保留 reference/locator/repository/objectId/commit/value、sources/withheld、维护 Address/digest 和逐单元 declarations/provenance。
  --shape 返回 data 与 errors：缺失、无权、冲突或范围外的位置为 null，并按 path 给出错误码；合法 null 没有错误条目。
  采用封闭 Schema 时选了未声明字段整批拒绝；declarations 里每个所读单元的 digest 仍是完整维护摘要。
  适用的 digest 属于已保存的完整维护单元，可作写入条件；叶值为 null 与定位不存在分别处理。
 Dataset 的动态值另保留 observations：commit 固定声明依据，源观察及其版本不能当作该 commit 的固定正文。
后果：
  直接仓读取固定正文与声明；Dataset 选中的 Bound State 会沿既有通路实际访问源。
  此读取不发布或更新对象；来源专属解释与真实执行归已装配能力。
风险与成本：
  完整对象可能较大；选择器在读取计划中选择必需单元，深字段仍可能需要读取其完整维护单元。
  published 会推进；固定正文复现须显式复用返回 commit，来源声明不证明外部事实已经验证。
  Bound State 增加源延迟、限流或成本，重读可能观察新值；固定声明缓存不冻结实时观察。
失败恢复：
  核对 URN 的来源仓与 object_id、Pointer 转义、固定版本及获准范围；拒绝或缺失不能当作空值。
  若准备修订，保存返回 Address/digest，并提交完整维护单元；读出深字段不能直接局部 PATCH。
示例：
  例：kc read 'urn:kc:object:table/orders#/columns/order_id/constraints/nullable'
  例：kc read 'urn:kc:object:table/orders#/columns/order_id' --dataset scene-set
  例：kc read 'urn:kc:object:table/orders' --dataset scene-set --revision 2
  例：kc read 'urn:kc:object:table/orders' --shape '{ definition columns(keys: ["order_id"]) { type } }'
  例：kc read 'urn:kc:object:kc/system#/readme'
  例：kc read 'urn:kc:object:kc/concept/object#/definition'
  系统对象示例仅在该服务已出版相应对象且当前主体有读取权时可用；理解本地 kc help 不需要这个前提。
`,
	},
	"resolve": {
		Description: "检查对象是否存在并给出解析到的 commit，不打开正文",
		Usage: `kc resolve (--repo <id> | --dataset <id>) --object <object-id>

用途：
  在获准的固定范围内核对对象坐标，为读取或比较保存解析依据。
用法与参数：
  --object 同 read；可指定 --aspect <name>，--member <key> 必须同时指定 aspect。
  单仓可用 --commit 或 --ref；省略按 refs/heads/main（published）解析，Dataset 按本次解析的选择范围查找。
  Dataset 可加 --revision <N> 固定已发布修订，不能与 --pin 同用。
条件：
  已认证；单仓需 object.read，命名 Dataset 以其 file.read 准入，不另要求来源仓 object.read。
 Dataset 须通过所需对象与选择范围的完整读取检查；另行解析 pin 需 dataset.resolve，不能探测未获准的来源。
结果：
  JSON 给出 repository/objectId/commit/status 和适用的 digest/切面坐标，不返回正文。
  缺对象是 status=UNRESOLVED（单仓）或空名单（Dataset）；Dataset 中相同 id 的多个来源不会合并为一个对象。
后果：
  仅检查固定声明是否可解析，不调用 Bound State 源，不发布、缓存或补建对象。
风险与成本：
  成本随查找来源和切面数量增长；解析成功不证明清单能组成完整正文、检索就绪或实时源在线。
  省略 commit 时下一条命令可能采用更新后的 published，保存坐标不能替代随后显式选择同一版本。
失败恢复：
  UNRESOLVED 时核对对象 id、切面、成员和版本；不要把拒绝访问解释为对象不存在。
  需读正文时使用 read，并在单仓复用已返回的 commit；临时错误可重试，重新解析可能得到新依据。
示例：
  例：kc resolve --repo kr://scene/object --object example/report
  例：kc resolve --dataset scene-set --object example/report`,
	},
	"provenance": {
		Description: "读取对象来源信封",
		Usage: `kc provenance (--repo <id> | --dataset <id>) --object <object-id>

用途：
  查看对象各对象单元随固定版本保存的来源声明，帮助核对其产生依据。
用法与参数：
  --object 是对象 id；这是对象级命令，不接受 --aspect 或 --member。
  单仓可用 --commit 或 --ref；Dataset 在本次解析的合法选择范围内返回来源结果，可加 --revision <N> 固定已发布修订。
条件：
  已认证；单仓需 object.provenance，命名 Dataset 按其 file.read 解释清单内材料，不另要求来源仓对象动作。
 Dataset 须通过所需对象与选择范围的完整读取检查；另行解析 pin 需 dataset.resolve，不由此扩大可读来源。
结果：
  JSON 返回对象来源轨迹及其 repository/objectId/commit；多个来源分别保留身份。
  这是已保存的来源信封，不是 Git 作者历史，也不是实时验证过的外部业务事实。
后果：
  读取来源声明，不访问声明指向的外部系统，不递归追读 sourceRefs，不补写来源链。
风险与成本：
  成本随对象单元和来源数量增长；来源链可能含敏感坐标，交付结果仍需遵守相应权限。
  缺少来源信封不能推出“没有来源”；有来源声明也不能推出所有引用仍可访问或声明内容正确。
失败恢复：
  未解析时先 resolve 核对对象及固定版本；无权时按单仓 provenance 或 Dataset file.read 通路申请相应范围。
  需要核实某个来源对象时按其坐标另行 read，并接受该来源自己的权限和版本检查。
示例：
  例：kc provenance --repo kr://scene/object --object example/report
  例：kc provenance --dataset scene-set --object example/report`,
	},
	"log": {
		Description: "分页读取对象修订历史",
		Usage: `kc log (--repo <id> | --dataset <id>) --object <object-id> [--limit <n>] [--continuation <cursor>]

用途：
  查看对象的固定内容在哪些仓版本发生修订，为回读旧版本或核对发布提供依据。
用法与参数：
  --object 是对象 id，不接受 --aspect 或 --member；--limit 默认 50、最大 200。
  单仓可用 --commit 或 --ref 指定历史起点；Dataset 可加 --revision <N>；--continuation 续页时保持对象和来源依据。
条件：
  已认证；每个来源仓均需 object.history.read，Dataset 另需其 file.read；消费权或正文读权不替代来源历史权。
 Dataset 须通过所需对象与选择范围的完整读取检查，文件切片不提供来源历史；每次续页重新检查来源历史权。另行解析 pin 需 dataset.resolve。
  来源须支持修订历史读取。
结果：
  JSON logs 按来源给出对象修订和 commit，并返回适用的续页游标。
  limit 分别限制每个仓的修订数，Dataset 总条数可能超过 limit；不是跨仓全局时间排序。
后果：
  读取版本历史，不回滚对象、不恢复已删除内容，也不生成新的仓版本。
风险与成本：
  后端需要查版本历史，成本随历史深度和来源数量增长；页大小只限制返回修订，不保证固定查询成本。
  这是对象内容修订史，不是每条 Git 提交、库存登记审计或 Bound State 的观察历史；空结果不能证明对象从未存在。
失败恢复：
  历史能力或授权缺失时请相应维护者处理，不以当前正文替代历史结果。
  游标与当前依据不符时重新查询；单仓保存并显式复用起点 commit，避免 published 推进后混页。
示例：
  例：kc log --repo kr://scene/object --object example/report
  例：kc log --dataset scene-set --object example/report`,
	},
	"schema list": {
		Description: "列出一个 Repository 或 Dataset 已发布的对象类型，不是对象实例目录或合同正文",
		Usage: `kc schema list (--repo <id> | --dataset <id> [--revision <N>]) [--limit <n>] [--continuation <cursor>]

用途：
  发现单仓或 Dataset 范围内已发布的 Schema 身份，再用 schema describe 查看结构、可检索字段与就绪状态。
用法与参数：
  --repo 可用 --commit 或 --ref，省略固定读取 refs/heads/main（published）的当前 commit。
  --dataset 列出选入范围内各成员的 Schema；--revision <N> 固定已发布修订，省略时用当前修订。
  --limit 默认 50、最大 200；--continuation 续页须保持同仓同版本，Dataset 续页须带首页的同一 --revision。
条件：
  已认证；单仓需目标仓的 object.schema.read，命名 Dataset 以其 file.read 准入，不另要求来源仓 object.schema.read。
 Dataset 须通过整份选择范围的完整读取检查；来源须提供 Schema 名单读取能力，不依赖检索投影。
结果：
  单仓 JSON 返回 repository/commit 与 schemas 的类型名称 entity、aspect（有则给）、objectId 和适用的描述，以及后续页游标。
 Dataset JSON 返回 dataset/revision，schemas 每项带 repository/commit/objectId/entity/aspect/description、字段数 fields 与已声明的检索用途 uses。
  同一实体的多个 aspect 是多份合同、各占一行；先按 aspect 选定所需合同，再对其 objectId 做 schema describe。
  名单不是对象实例目录，也不是 Schema 合同正文；无续页游标表示该名单已到末页。
后果：
  读取已发布 Schema 名单，不安装 Schema、不扫描全部业务对象、不建立索引。
风险与成本：
  返回量受页大小约束，但后端仍可能先列出 Schema 身份并读取合同，Dataset 成本随成员和 Schema 数量增长。
  Schema 在名单中不证明实例存在、投影就绪或 Bound State 源可访问；uses 只说明声明了什么用途。
失败恢复：
  来源不支持名单时不能用全对象扫描补偿；请来源维护者提供相应能力或明确已知 Schema id。
  游标属于另一修订时用首页的 --revision 重查；单仓游标失效时显式使用回执 commit 重查；获取字段与就绪继续 schema describe。
示例：
  例：kc schema list --dataset scene-set --revision 2
  例：kc schema list --repo kr://scene/object --limit 50
  例：kc schema list --repo kr://kc/system`,
	},
	"schema describe": {
		Description: "查看 Schema 结构与约束的规范化展示；Dataset 下同时说明可检索字段与就绪状态",
		Usage: `kc schema describe (--repo <id> | --dataset <id> [--revision <N>]) --object <schema-id>

用途：
  核对 Schema 的结构与约束，判断已有类型是否适合自己的资料；在 Dataset 下确认哪些字段能检索、怎样检索。
用法与参数：
  --object 是 schema/* 合同 id，来自 schema list 的 objectId，不是业务对象 id 或文件路径。
  单仓可用 --commit 或 --ref 指定版本；Dataset 可用 --dataset 代替 --repo，但仅描述已选入范围，--revision <N> 固定已发布修订。
条件：
  已认证；单仓需 object.schema.read，命名 Dataset 以其 file.read 准入，不另要求来源仓 object.schema.read。
 Dataset 须通过所需对象与选择范围的完整读取检查；合同及必要依据须在清单内，另行解析 pin 需 dataset.resolve。
结果：
  JSON 给出 repository/commit、Schema objectId、规范化 definition 和字段 path/type；fields 是数组。
 Dataset 下按成员仓各给一份报告，另有 dataset/revision；fields[].access 是选入 Dataset 的索引声明给该字段的用途：text 供 --query，filter 供 --eq 等条件，sort 供 --sort。同一 path 跨 Schema 或方面时，条件写成 schema::<aspect>::<path>=value。
  search.state 为 ready 或 unavailable（附 reason，如投影未就绪或无索引插件）；search.declarations 逐条给出声明的 status（ready/unsupported/invalid/inapplicable，后者表示输入 Schema 不在本范围、范围内没有适用对象，查询它得到完整零命中）、reason、inputs 与 uses。
  这是供理解的规范化展示，不能把 definition 或整个响应直接当作 Schema 发布输入；原始 fields 必须是对象映射。
  要复用原始 Schema，用 kc read 读取相同 objectId 与 commit，取返回的 value，而非整个读取信封。
  也可经 kc files list / kc files read 从 Dataset 取得含文件头的 Schema 原文件，列目录与读取固定同一 revision。
  Schema 只描述结构；查询能力见独立索引声明及 kc plugin declarative-index help，外部取值见取值描述。
后果：
  读取字段、结构约束和已选入的索引声明，不查询业务对象，不调用外部取值，不更新 Schema 或投影。
风险与成本：
  成本随合同、字段和声明数量增长；结构可解释不证明外部源授权成立，就绪只反映本次固定修订。
  commit 固定声明版本，不证明外部当前值或后续写入已经通过校验。
失败恢复：
  id 未解析时，在 Dataset 内用 schema list --dataset 或 files list 查找已交付的 Schema；有来源仓读取权时也可用 schema list --repo。
  结构约束不符时核对实例与 Schema，需要变更须经 Writer 发布。
  字段没有所需 uses 时搜索会被拒绝：检索能力由独立索引声明决定，请维护者发布并把声明选入 Dataset，不通过修改 Schema 增加查询能力。
  声明 unsupported/invalid 按原因修声明；就绪问题另查 operations projection describe；原始合同 read 在单仓需 object.read，Dataset 仍按 file.read 清单解释。
示例：
  例：kc schema describe --dataset scene-set --revision 2 --object schema/metric.definition
  例：kc schema describe --repo kr://kc/system --object schema/meta/schema-definition/v1`,
	},
	"writer put": {
		Description: "按对象 URN 与 locator 创建或替换完整内容",
		Usage: `kc writer put <object-urn[#/json/pointer]> --repo <repository> --command-id <id> (--value <json> | --file <path>)
  用途：
    按统一引用创建或更新完整维护单元。PUT 替换完整单元，不合并部分字段。
  用法与参数：
    --value 是该完整单元允许的 JSON 值，用单引号包住。--file 从本机文件读取同样的 JSON 值，二选一。
    --file 不接受 frontmatter 对象文件，也不接受 read 响应信封；从读取结果取 value，多个对象文件见 kc help writer commit。
    位置参数是对象 URN 与 locator；服务端解析维护范围，不按 Pointer 深度猜写粒度。
    写入只支持完整 blob、Aspect 或 Member；深字段、集合整体不产生新的写粒度。
    首次建立 Member 需 --kind Member；已有单元采用其维护模型，不把路径深度自动当成 Member。
    可给 --schema-ref、--path-hint；省略路径时保留既有路径，显式路径用于迁移。
    --ref 默认 refs/heads/main；--base/--expected 约束仓基点，--if-absent 或 --if-digest 约束单元，二者互斥。
    --command-id 由你提供，--message 与来源参数可记录提交说明；此入口不接受 --bulk。
  条件：
    一个可写对象 Repository，当前主体有 writer.commit；正文、Schema、来源和并发条件通过校验。
    系统仓与登记权威不能作为普通 Writer 目标，Dataset 不能作为写入目标。
  结果：
    返回耐久 Writer 回执和 oldCommit/newCommit；首次为 APPLIED，同命令同请求重放为 REPLAYED。
  后果：
    发布该单元并推进目标 ref；省略 --schema-ref 时继承并校验已有声明。
    回执与来源提交已接受后，次级证据或派生维护失败不撤销发布；检索可能尚未服务新版本。
  风险与成本：
    完整替换可能丢掉未提供的字段，需要网络写入与校验；不要把 --command-id 改成自动随机重试。
    网络中断可能已经提交，客户端只查询原回执，默认等待最多十分钟，不自动再次提交。
    服务端明确证明未应用时立即返回原错误，不查询回执。
  失败恢复：
    先 kc writer receipt 查询原 --command-id；同 id 异请求为 IDEMPOTENCY_CONFLICT。
    NON_FAST_FORWARD 要重新核对基点并构造新请求；Schema/权限拒绝先修正原因。
    PENDING 或查询不到最终回执时保留原请求并请运营者对账，不能以换 id 推断安全重写。
  示例：
    例：kc writer put 'urn:kc:object:example/report' --repo kr://scene/object --command-id put-1 --value '{"name":"GMV"}'
    例：kc writer receipt --command-id put-1`,
	},
	"writer remove": {
		Description: "按对象 URN 与 locator 删除对象或完整维护单元",
		Usage: `kc writer remove <object-urn[#/json/pointer]> --repo <repository> --command-id <id>
  用途：
    从新的发布版本中显式删除整个对象或选定的完整维护内容。
  用法与参数：
    位置参数同 read；空 locator 删除整个对象的全部存储单元。--command-id 由你本次提供。
    非空 locator 只可选择完整可维护 Aspect 或 Member；深字段、数组索引及集合整体不新增删除粒度。
    /description 选择完整 Aspect，/columns/order_id 选择 Member；实际范围按对象维护模型确认。
    --ref 默认 refs/heads/main；可给 --base/--expected、--if-digest 和 --message 约束或说明这次删除。
  条件：
    目标是一个可写对象 Repository，当前主体有 writer.commit；维护范围和并发条件通过校验。
    仍被引用的 Schema 不能移除；系统仓、登记权威与 Dataset 不能作为普通写入目标。
  结果：
    返回 Writer 回执及目标 ref 的 oldCommit/newCommit；同命令同请求返回原回执。
  后果：
    空 locator 删除该对象全部存储单元；Aspect/成员 locator 只删选中单元，不递归删除其它对象。
    删除推进新的发布版本，不擦除历史版本。
    已接受发布不会因后续投影或证据失败被撤销；用户分多条 remove 时，每条是独立提交。
  风险与成本：
    新的读者会失去该单元，历史读取仍需当前权限；多条命令中前条成功不会被后条失败回滚。
    网络中断不表示删除未发生，客户端默认只轮询原回执最多十分钟。
    服务端明确证明未应用时立即返回原错误，不轮询、不自动重发。
  失败恢复：
    用原 command-id 查 writer receipt；同 id 换内容会冲突，结果未明先对账。
    并发冲突先重新读取，Schema 仍被引用先处理依赖；恢复内容需要一次新的明确 PUT，不倒退历史。
  示例：
    例：kc writer remove 'urn:kc:object:example/report' --repo kr://scene/object --command-id rm-1
    例：kc writer remove 'urn:kc:object:example/report#/description' --repo kr://scene/object --command-id rm-description
    例：kc writer receipt --command-id rm-1`,
	},
}
