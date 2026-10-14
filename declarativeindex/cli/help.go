package cli

type helpEntry struct{ Description, Usage string }

func leafHelpEntries() map[string]helpEntry {
	return map[string]helpEntry{
		"operations projection describe": {
			Description: "查看投影状态",
			Usage: `kc operations projection describe --repo <id> [--commit <id>]
用途：
  核对指定知识版本的检索投影依据、对象数量和就绪信息，定位来源提交与派生状态的差距。
用法与参数：
  --repo 是已有来源仓；--commit 固定历史依据，省略时解析本次默认发布引用。也可用 --ref 明确引用。
条件：
  已认证并有该仓范围的 projection.read；来源版本可解析，服务已配置对应投影提供方。
结果：
  JSON 返回 basisRepository、basisCommit、headCommit、objectCount、lagBehindHead 及可用的 state/digest/字段声明。
后果：
  读取投影描述，不提交知识、不重新发布 Dataset，也不执行 projection sync。
风险与成本：
  读取元信息并向提供方统计对象，可能涉及索引 count；频繁轮询会增加后端负载。
  描述是本次读取的状态，不证明以后请求仍获授权，也不能把缺失投影当成已就绪的零对象仓。
失败恢复：
  版本未解析先核对仓与 commit；CAPABILITY_UNSATISFIED 或提供方故障由管理者恢复配置/后端。
  需要历史或排障构建时另行使用 projection sync；查询帮助和 describe 不会隐式替你建索引。
示例：
  例：kc operations projection describe --repo kr://scene/knowledge
  例：kc operations projection describe --repo kr://scene/knowledge --commit c1`,
		},
		"operations projection sync": {
			Description: "同步历史或排障投影",
			Usage: `kc operations projection sync --repo <id> [--commit <id>]
用途：
  显式构建或追赶指定来源版本的派生投影，用于历史准备和维护排障。
用法与参数：
  --repo 是已有仓；--commit 指定固定版本，省略时解析本次默认发布引用；可用 --ref 选择引用。
条件：
  已认证并有该仓范围的 projection.manage；来源版本与投影后端可用，State 刷新还依赖已配置的来源读取能力。
结果：
  JSON 返回投影的依据、state/mode/cause 及计数；已配置 State 读取时可返回 snapshot 与 state 两份同步结果。
后果：
  可构建固定版本投影、推进维护目标并追赶工作，更新派生索引；装配了 State 读取时还会保存观察并更新 State 投影。
  不推进知识仓 HEAD、不重新发布 Dataset、不发放消费权限；这是一条有派生写入的管理命令。
风险与成本：
  声明变化、索引缺失或依据不兼容时可能重建，涉及扫描来源、编译及批量索引写入，成本随内容和字段增长。
  同步某一步失败不承诺回滚此前已完成的派生工作；旧版本投影不代表当前请求自动可用。
失败恢复：
  核对 describe、来源版本和后端故障，恢复原因后按原 commit 重试，再核对实际结果。
  不为派生失败撤销已接受源提交；不要把一次同步成功当作所有 Dataset 已采用更新或全部来源永久就绪。
示例：
  例：kc operations projection sync --repo kr://scene/knowledge
  例：kc operations projection sync --repo kr://scene/knowledge --commit c1`,
		},
		"operations projection notice": {
			Description: "通知 Bound State 观察变化，不推进仓 HEAD",
			Usage: `kc operations projection notice --repo <id>
用途：
  将 Bound State 的变化位置提示给维护控制器，让服务重新取值并更新派生观察。
用法与参数：
  指定句柄时再加 --object 与 --aspect；--ref 选择声明依据，--source-revision 只提供来源修订提示。
  通知只定位变化，不携带 value/body/payload；不带句柄时通知该仓对应范围。
条件：
  已认证并有该仓范围的 projection.manage；控制器、声明及受信任的 State 读取来源已配置且可用。
结果：
  JSON 返回 repository、basisCommit 和观察 revision；不是新的知识仓提交回执。
后果：
  通知 Bound State 观察变化，不推进仓 HEAD；请求会登记待维护工作并执行追赶，可能访问来源、保存观察和写派生投影。
  不发布 Dataset、不写 Canonical，也不把通知者提供的修订提示当作已验证业务值。
风险与成本：
  可触发网络取值、观察保存和索引维护，成本随受影响句柄及待追赶工作增长；成功不是所有来源永远同步的保证。
  来源或提供方失败时可能已有待恢复工作或已完成观察，不承诺把这些阶段全部回滚。
失败恢复：
  核对声明、来源授权和提供方可用性，恢复后按同一变化位置重发提示并核对结果。
  缺少 State 投影会明确拒绝；不能通过附带正文把通知变成写入，也不以修改仓 HEAD 代替恢复观察。
示例：
  例：kc operations projection notice --repo kr://scene/knowledge
  例：kc operations projection notice --repo kr://scene/knowledge --object note/hello --aspect status --source-revision rev-2`,
		},
	}
}
