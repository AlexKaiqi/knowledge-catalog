package cli

type helpEntry struct{ Description, Usage string }

func leafHelpEntries() map[string]helpEntry {
	return map[string]helpEntry{
		"plugin": {
			Description: "读取插件的用途、适用声明和已有任务用法",
			Usage: `kc plugin <name> help

用途：
  读取服务登记插件的说明，了解它解释哪些声明及如何沿已有任务使用。
用法与参数：
  name 来自 operations access-spec describe 的来源插件帮助入口；第二个位置参数只能是 help。
  不接受安装、卸载、实例选择或目标配置参数。日常查询仍用 search、relations、traverse。
条件：
  通过已认证 Server 读取；仅需既有 inventory.read，名称须对应已登记说明。
结果：
  返回插件名称、用途、用法与任务入口；不返回实例配置、凭据或投影状态。
后果：
  只读说明，不执行插件能力、不访问来源、不建索引或启动维护。
风险与成本：
  说明读取不证明目标声明适用、提供方支持、版本就绪或本次请求已获授权。
失败恢复：
  名称不存在时回到目标访问概览核对帮助入口；身份或服务故障按实际原因恢复。
示例：
  例：kc plugin declarative-index help
  本地查看此任务用法：kc help plugin`,
		},
		"search": {
			Description: "在固定仓或 Dataset 上按 Schema 字段的查询声明检索对象，列出匹配对象",
			Usage: `kc search (--repo <id> | --dataset <id>) --query <text>

用途：
  根据已发布 Schema 的字段访问声明找对象，再按命中坐标读取正文。
用法与参数：
  --query 是 MATCH 关键字，不是文件 glob；精确条件可改用 --eq path=value，多个条件共同约束结果。
  字段路径有歧义时写 schema::aspect::path；可用 --sort、--limit <1-1000> 和 --continuation <cursor>。
  --recall lexical|semantic，省略为 lexical；semantic 需要文本查询，当前只返回一个近似候选窗口，不支持续页。
  单仓可用 --commit 或 --ref 固定依据；省略使用 published。含空格、* 或 $ 的值须加单引号。
条件：
  已认证；单仓需 object.search，命名 Dataset 按覆盖选定材料的 file.read 准入，不另要求 object.search。
  不带范围的库存发现需 inventory.read；另行解析 Dataset pin 才需 dataset.resolve，解析权不代替材料读取权。
  查询字段须经 Schema 声明，固定版本的检索投影须就绪；semantic 另需向量投影与 embedding 能力。
结果：
  JSON hits 给出 repository/objectId/commit，供后续 read；还返回范围、完整性、续页或检索证据信息。
  hits 不带正文；单仓搜索权不替代 object.read，Dataset 的对象消费范围由 file.read 和清单限定。
  无命中不代表范围外或无权读取的对象不存在。
后果：
  执行查询并留下服务配置的访问/检索证据，不发布对象、不自动建立或同步投影。
  不带范围时仅可使用服务配置的库存发现 Dataset，不会扫描所有连接的来源。
风险与成本：
  lexical 默认每页 100 条；成本随来源、候选及回读数量增长。semantic 还会调用 embedding，可能有额外网络与模型成本。
  semantic 窗口不保证穷尽或跨仓全局分数排序；'*' 不是全仓浏览，空条件不是扫描入口。
失败恢复：
  字段被拒绝时先 schema list/schema describe；投影未就绪时查 operations projection describe，请维护者修复，不能把失败当零命中。
  lexical 续页须保持查询、范围和版本；游标失效时核对依据后重新查询，单仓可显式复用已返回的 commit。
示例：
  例：kc search --repo kr://scene/object --query 'merchandise'
  例：kc search --dataset scene-set --query 'merchandise'
  例：kc search --repo kr://scene/object --eq unit=CNY
  例：kc search --repo kr://scene/object --eq 'name=Gross merchandise value'`,
		},

		"relations": {
			Description: "列出连着对象的关系对象命中，不是邻接对象正文",
			Usage: `kc relations (--repo <id> | --dataset <id>) --object <object-id> [--direction DIRECTED|UNDIRECTED] [--limit <n>] [--continuation <cursor>]

用途：
  找到把指定对象作为端点的关系对象，供读取关系或继续有界遍历。
用法与参数：
  单仓 --object 是对象 id；Dataset 必须是范围内完整 kc://<repository>/<object> 引用，不能只写同名对象 id。
  可按 --relation-type、--role 筛选；--direction 是关系的方向类型，不是 in/out。
  --limit 默认 100、最大 1000；用 --continuation 续页。单仓可用 --commit 或 --ref 固定依据。
条件：
  已认证；单仓需 object.relations，命名 Dataset 以其 file.read 准入，不另要求来源仓 object.relations。
 Dataset 须通过整份选择范围的完整读取检查，单个端点放行不能替代；另行解析 pin 需 dataset.resolve。
  固定版本的关系检索投影必须就绪。
结果：
  JSON hits 的 objectId 是关系对象身份，含 repository/commit/relationType/matchedRoles；不是邻居身份。
  当前 CLI 命中视图不展开关系端点正文；按关系坐标 read 可核对端点，邻域节点请用 traverse。
后果：
  查询已有关系投影，不建立关系、不读取邻居正文、不把范围外引用加入 Dataset。
风险与成本：
  高连接度对象会产生多页；结果受固定范围、权限和投影覆盖约束，不能当作所有来源的完整关系图。
  关系查询不等于有向入边/出边查询，也不证明关系声明反映了外部源的当前事实。
失败恢复：
  无投影时查 operations projection describe；Dataset 端点被拒绝时核对完整引用是否属于当前范围。
  续页保持端点、过滤和固定依据；游标失效时重新查询，单仓可复用返回的 commit，勿拼接不同版本的页。
示例：
  例：kc relations --repo kr://scene/object --object metric/gmv --direction DIRECTED
  例：kc relations --dataset scene-set --object kc://scene/object/metric/gmv --direction DIRECTED`,
		},
		"traverse": {
			Description: "在固定范围内沿关系做有界邻域遍历，不是图查询语言",
			Usage: `kc traverse (--repo <id> | --dataset <id>) --object <object-id> --max-hops <1-8> [--min-hops <n>] [--relation-type <t>] [--role <r>] [--direction DIRECTED|UNDIRECTED] [--limit <n>] [--continuation <cursor>]

用途：
  从一个对象出发，沿获准的关系得到限定跳数的邻域闭包。
用法与参数：
  单仓 --object 用对象 id；Dataset 用完整 kc://<repository>/<object> 引用。
  --max-hops 必填且为 1-8；--min-hops 默认 0，须不大于 max-hops。类型、角色和方向可限制关系。
  --limit 默认 100、最大 1000；续页须复用过滤与范围。单仓 --commit/--ref 固定来源版本。
条件：
  已认证；单仓需 object.traverse，命名 Dataset 以其 file.read 准入，不另要求来源仓 object.traverse。
 Dataset 须通过整份选择范围的完整读取检查，单个起点放行不能替代；另行解析 pin 需 dataset.resolve。
  起点在已解析范围内，所需固定版本的关系投影可用。
结果：
  nodes 按仓/对象去重并给最小跳深；edges 给关系坐标及完整端点，boundary 表示越界处停止。
  游标返回闭包增量；完整性与未完成原因须一起检查，不枚举全部路径，也不承诺最短路径查询。
后果：
  在请求固定的范围查询关系，不创建对象或关系，不追读范围外对象。
风险与成本：
  跳数和关系扇出会迅速增加查询量；整个续页序列最多选择 5000 条边，达到预算不代表图已穷尽。
  单页节点数不是闭包完整性指标；游标、预算或前沿未完成时不能把已返回片段当作全图。
失败恢复：
  投影不可用时请维护者核对就绪版本；预算或高扇出时缩小跳数、关系类型或角色后重新遍历。
  游标拒绝时核对固定依据，不混接不同版本的增量；临时失败可重试同一页，合并时按身份去重。
示例：
  例：kc traverse --dataset scene-set --object kc://acme/public/core/Table:orders --max-hops 3
  例：kc traverse --repo kr://scene/object --object Table:orders --max-hops 2 --relation-type owned-by`,
		},

		"operations access-spec describe": {
			Description: "查看 AccessSpec",
			Usage: `kc operations access-spec describe (--repo <id> | --dataset <id>)

用途：
  查看固定来源的逻辑字段访问合同，核对哪些字段参与 Snapshot 查询或 State 访问。
用法与参数：
  选择 --repo 或 --dataset；单仓当前采用 published，Dataset 使用本次解析的来源选择。
  此入口不传递 --commit/--ref，不能靠这两个 flag 指定其它单仓依据。
条件：
  已认证；单仓需 object.access.describe 及整仓 object.read，命名 Dataset 按其 file.read 准入，不另要求这两项来源仓动作。
  此报告须通过整份选择范围的完整读取检查，单个对象放行不能替代；另行解析 Dataset pin 需 dataset.resolve。
  来源已登记且可解析，所需 Schema 在合法范围内。
结果：
  JSON 保留固定来源依据与由 Schema 编译的逻辑 AccessSpec，并列出本次来源采用插件的用途和帮助入口。
  可沿 kc plugin <名称> help 阅读完整说明；说明不证明支持、就绪或本次执行授权。
  这不是引擎物理映射、就绪状态报告或一次查询的命中结果。
后果：
  读取并编译现有访问声明，不执行查询、不调用实时源、不建立或同步投影。
风险与成本：
  成本随固定来源和 Schema 数量增长；声明能力不证明投影已就绪、后端支持所有算子或源授权可用。
  published 或 Dataset 后续发布可改变下一次报告依据，比较报告时须同时核对返回版本。
失败恢复：
  来源或合同缺失时用 show/schema list/schema describe 核对登记与发布，请维护者修复合法选择及合同。
  就绪问题另查 operations projection describe；不要把描述失败替换为空合同或隐式触发投影重建。
示例：
  例：kc operations access-spec describe --repo kr://scene/object
  例：kc operations access-spec describe --dataset scene-set`,
		},
	}
}
