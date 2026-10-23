package cli

type helpEntry struct{ Description, Usage string }

func leafHelpEntries() map[string]helpEntry {
	return map[string]helpEntry{
		"plugin": {
			Description: "读取插件说明、实例能力清单或只读试运行声明",
			Usage: `kc plugin <name> help|describe|trial

用途：
  help 读取登记说明和编译清单；describe 给出实际安装实例的带版本清单。
  trial 用 Writer 相同结构规则校验候选声明，再按声明 catalog 核对实例能力。
用法与参数：
  kc plugin declarative-index describe
  kc plugin declarative-index trial --repo <id> --file <声明文件> [--commit <id> | --ref <ref>]
  help 与 describe 不接受目标；trial 只读本地文件后发送内容，不让服务打开本机路径。
条件：
  通过显式身份的认证 Server；help/describe 需 inventory.read。
  trial 需目标及所读 Schema 仓的完整 object.read；裸引用的远端回退无法证明读取权限时，改用显式 kc://<repo>/<schema> 定位符。
结果：
  清单列构建方式、可选值、修改是否重建、上限与摘要；不包含实例凭据、引擎地址或原始 mapping。
  trial 返回逐成员 supported/unsupported/invalid、原因、固定基准和重建判断；新成员需要构建。
后果：
  不写入对象、命令账本或索引，不激活后台维护；试运行不预留之后提交的 CAS。
风险与成本：
  静态编译清单不能替代实际实例清单；试运行只检查写法与能力，不评估检索效果或证明就绪。
失败恢复：
  按 invalid 的 Writer 定位修文件；unsupported 对照实例清单，不偷偷替换声明版本。
  目标分支变化后重新试运行，再用正常 Writer 发布；安装管理与实例选择属于部署。
示例：
  kc plugin declarative-index help
  kc plugin declarative-index trial --repo kr://scene/object --file index/name.yaml
  本地查看此任务用法：kc help plugin`,
		},
		"search": {
			Description: "在 Dataset 的固定修订内搜索并取得同版本命中坐标",
			Usage: `kc search --dataset <id> [--revision <N>] --query <text> [--eq <field=value>] [--sort <field>] [--limit <n>]
用途：
  先从 kc show 的消费库存找到 Dataset，再在该 Dataset 选入的对象与声明范围内检索。
用法与参数：
  --dataset 必填；--revision 固定已接受修订，省略采用当前版。所有后续读取和翻页沿用结果中的修订。
  用 kc schema describe --dataset <id> [--revision <N>] --object <schema> 查看声明支持的字段用途；过滤与排序按 Schema 字段定位符写，不传声明成员键。
  路径在这个 Dataset 里唯一时可写 --eq name=orders。同一路径出现在多个 Schema 或方面时，改写 schema::<aspect>::<path>=value。
  --query 是关键词文本，不是文件 glob；多个条件共同约束结果。--limit 默认 100，范围 1-1000；--continuation 用同查询、同修订续页。
  --recall lexical|semantic；semantic 需要查询文本和已配置向量能力，返回近似候选窗口，不支持续页。
条件：
  已认证并持有该 Dataset 完整选择范围的 file.read。仓读权不能代替 Dataset 消费权。
  只选入 Dataset 的声明生效；所用成员与固定修订投影必须支持、有效并就绪。
结果：
  JSON hits 提供 repository/objectId/commit，dataset 提供 id/revision；命中含来源版本、完整性及续页依据。
  命中的 basis 可交给 kc read <命中引用> --basis <basis> 原范围回读；state 描述完整所选单元的本地状态。
  固定回读依据独立于查询索引保留，不能用它延长检索或续页期限；外部观察另按证据合同复核。
  CLI 列出坐标；kc read --dataset <id> --revision <N> <命中引用> 取得范围内完整正文。
后果：
  查询并留证，不提交对象、不发布 Dataset、不隐式构建或同步投影。
风险与成本：
  成本随声明成员、候选和回读增长；semantic 可能调用模型。无命中不说明范围外内容不存在。
  不带 Dataset 或使用 --repo/--commit/--ref 会被拒绝。范围先于候选查询，不能先查范围外对象再遮正文。
失败恢复：
  未声明、不支持、失效、未就绪与零命中分别处理；按错误定位修声明或向维护者查 kc index describe --dataset <id>。
  续页须保持查询与修订，撤权后下一页拒绝；游标失效时在同一修订重新开始。
示例：
  kc search --dataset scene-set --revision 1 --query merchandise
  kc search --dataset scene-set --revision 1 --eq schema/sr.table.properties.v2::properties::name=orders`,
		},
		"relations": {
			Description: "在 Dataset 固定修订内列出连接指定对象的关系",
			Usage: `kc relations --dataset <id> [--revision <N>] --object <object-id> [--direction DIRECTED|UNDIRECTED] [--limit <n>] [--continuation <cursor>]
用途：
  找到以对象为端点的关系对象，按关系坐标读取或继续有界遍历。
用法与参数：
  --object 是对象身份；按 --relation-type、--role、--direction 筛选，direction 表示关系方向类型。
  --revision 固定发布版；--limit 默认 100、最大 1000，续页保持端点、条件与修订。
条件：
  已认证并持有 Dataset 完整选择范围的 file.read，固定修订的关系投影就绪。
结果：
  hits 给关系对象的 repository/objectId/commit、类型与匹配角色，不是邻居正文。
后果：
  查询已有关系，不创建关系、不加入范围外引用或赋予来源仓权限。
风险与成本：
  高连接度会产生多页；固定范围的结果不代表全部来源关系图。
失败恢复：
  索引故障查 kc index describe --dataset <id>；游标失效在同一修订重新查询。
示例：
  kc relations --dataset scene-set --revision 1 --object trade/order-service`,
		},
		"traverse": {
			Description: "在 Dataset 固定修订内沿关系做有界邻域遍历",
			Usage: `kc traverse --dataset <id> [--revision <N>] --object <object-id> --max-hops <1-8> [--min-hops <n>] [--relation-type <t>] [--role <r>] [--direction DIRECTED|UNDIRECTED] [--limit <n>] [--continuation <cursor>]
用途：
  从对象出发取得限定跳数的邻域闭包，边界处停止。
用法与参数：
  --max-hops 为 1-8；--min-hops 默认 0 且不大于上限。类型、角色和方向限制关系。
  --limit 默认 100、最大 1000；续页复用过滤、Dataset 与修订。
条件：
  已认证并持有 Dataset 完整范围的 file.read；范围内持有起点，关系投影可用。
结果：
  nodes 按身份去重并给最小跳深；edges 给关系坐标和端点，boundary 列出范围外端点。
后果：
  不追读范围外对象，不创建对象、关系或写权限。
风险与成本：
  扇出增加成本，整个续页序列最多选择 5000 条边；检查完整性与限制，不能把一页当完整图。
失败恢复：
  核对固定修订和投影状态；预算不足时缩小跳数、类型或角色重新执行。
示例：
  kc traverse --dataset scene-set --revision 1 --object trade/order-service --max-hops 2`,
		},

		"operations access-spec describe": {
			Description: "查看 Dataset 选入声明的逻辑访问概览",
			Usage: `kc operations access-spec describe --dataset <id> [--revision <N>]
用途：
  查看选入声明提供的字段用途、成员状态和解释实例，与该 Dataset 查询使用同一范围。
用法与参数：
  --dataset 必填，--revision 固定发布版，省略采用当前版；不会从范围外补入声明。
条件：
  已认证并持有该 Dataset 完整范围的 file.read；Schema 引用可解析。
结果：
  保留固定版本及 AccessSpec，并列出采用实例、能力清单版本和 kc plugin <名称> help 入口。
后果：
  只读描述，不查询、不取外部值、不构建索引或发布修订。
风险与成本：
  成本随来源和声明数增长；描述不是物理映射或就绪证明。
失败恢复：
  核对 Dataset 发布与 Schema、声明选择；就绪另查 kc index describe --dataset <id>。
示例：
  kc operations access-spec describe --dataset scene-set --revision 1`,
		},
	}
}
