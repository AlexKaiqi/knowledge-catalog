package cli

import accesscli "kc/liveaccess/cli"

func hostOperationsHelpEntries() map[string]helpEntry {
	return map[string]helpEntry{
		"operations hook add": {
			Description: "登记 Hook",
			Usage: `kc operations hook add --on <verb> --phase <phase> [--repo <id>] [--url <url>] [--run <command>]
用途：
  登记已授权应用动作前后的出站扩展，让外部脚本检查或接收动作通知。
用法与参数：
  --on 是会改状态的语义动作（如 writer.commit）；--phase 只接受 pre 或 post，--run 与 --url 二选一，pre 必须 --run。
  --repo 限定一个仓，省略则匹配该服务中同一动作；--run 是 Server 部署的绝对 stateDir 内的可执行文件路径，不是 Client 当前目录里的命令。
  脚本工作目录也是该 stateDir；相对脚本路径由服务端解析，不解析 shell 参数串。
条件：
  已认证并有对应范围的 operations.hooks.manage；脚本需部署到服务端，HTTP 接收地址用于 post。
  登记只保存配置，不试运行脚本，也不证明接收方可达或动作已经获授权。
结果：
  JSON 返回 Binding 的 id、on、phase、repo 及 run/url；后续删除使用这个 id。
后果：
  保存服务配置；匹配动作通过授权后才执行 Hook，pre 可额外否决，失败或 15 秒脚本超时阻止后续动作。
  post 发生在动作成功之后，投递失败进入 outbox，后续 post 尝试重投，不回滚已成功动作。
  同一 command-id 的已接受 Writer 命令重放不再制造一次动作 Hook；这不承诺所有挂接动作都幂等。
风险与成本：
  脚本在 Server 上运行，外部效果没有跨系统回滚；pre 放行不代表领域动作必然成功。
  HTTP 投递超时为 10 秒；post 可能重投，接收方需按事件依据防重。重复 add 新增配置，可能放大出站调用。
失败恢复：
  登记失败先核对动作、阶段和配置；响应不确定时先 hook list 核对，避免重复登记。
  执行失败修复服务端脚本或接收方；修复通知不要重复提交对象，外部批准也不授予动作权限。
示例：
  例：kc operations hook add --on writer.commit --phase pre --repo kr://scene/object --run check.sh
  例：kc operations hook add --on writer.merge --phase post --repo kr://scene/object --url https://ci.example/events`,
		},
		"operations hook list": {
			Description: "列出 Hook",
			Usage: `kc operations hook list [--on <verb>] [--repo <id>]
用途：
  核对当前服务已登记的出站动作配置，取得移除时所需的 Binding 身份。
用法与参数：
  当前公共 typed CLI 读取全服务名单，--on/--repo 未传作查询过滤；需在返回 bindings 中核对 on、phase 和 repo。
条件：
  已认证并有服务范围的 operations.hooks.read，服务配置可读；查看配置不代表调用者能执行其中动作。
结果：
  JSON 返回 bindings，包含 id、on、phase、可选 repo 和 run/url；无登记时是空名单。
后果：
  不运行脚本、不投递事件，也不修改配置、授权或 outbox。
风险与成本：
  一次读取配置名单，未分页；条目数影响响应大小，run/url 是服务配置资料，应按当前读权保护。
  配置存在不证明脚本存在、接收方在线或旧通知已经送达。
失败恢复：
  身份或读权失败先处理对应原因；配置损坏交由部署管理者恢复，不能把读取失败当空名单。
  删除前核对完整条目的 id 与范围；不要依赖当前未执行的 --repo 过滤推断其它绑定不存在。
示例：
  例：kc operations hook list
  例：kc operations hook remove --id hk_1`,
		},
		"operations hook remove": {
			Description: "移除 Hook",
			Usage: `kc operations hook remove --id <id>
用途：
  删除一条已登记出站绑定，停止后续动作按这条配置触发新调用。
用法与参数：
  --id 来自 hook add 回执或 hook list，先核对它的动作、阶段和仓范围。
条件：
  已认证并有服务范围的 operations.hooks.manage，服务配置可写且该 id 当前存在。
结果：
  JSON 返回 revoked，值是已删除的绑定 id；不存在的 id 会失败，不是幂等空成功。
后果：
  保存删去该绑定的配置；不撤销对象提交或授权，不收回外部效果，也不取消已经排队的 outbox 事件。
风险与成本：
  影响后续出站行为；已开始的脚本/投递仍可能完成，待投递事件保存了原绑定信息。
  配置写入与并发管理不提供调用者 CAS 合同，删除前后的名单都需按实际服务核对。
失败恢复：
  未知 id 时重新 hook list 核对是否已被删除；保存或服务故障恢复后先读名单，不盲目假定删除成功。
  误删时按原 on/phase/repo/run/url 重新登记，不能借重放业务提交恢复外部事件。
示例：
  例：kc operations hook remove --id hk_1
  例：kc operations hook list`,
		},
		"operations audit access": {
			Description: "查看访问账",
			Usage: `kc operations audit access [--repo <id>] [--object <id>] [--limit <n>] [--continuation <cursor>]
用途：
  查询已保存的访问证据，核对谁在什么依据上进行了哪些访问及其授权/执行结果。
用法与参数：
  可用 --filter-principal、--filter-on-behalf-of、--action、--trace-id、--evidence-id 及 --since/--until 限定；时间使用 RFC3339。
  --limit 默认 50、最多 200，0 使用默认；有 continuation 时沿原过滤条件继续更旧记录，一页不代表全部历史。
条件：
  已认证并有查询范围的 audit.read，服务证据存储可读；库存或对象读权不能代替审计权。
结果：
  JSON 返回 source:access、entries、可选 continuation 和 completeThrough；记录带主体、trace、版本引用与访问结果。
后果：
  不改对象、授权或合并回执；存储按自身保留政策清理过期闭日分区，查询不能恢复已删除事件。
风险与成本：
  默认查询热窗，本机 adapter 默认保留 30 天，可由部署调整；不是无限审计历史，空结果不证明从未发生访问。
  查询会读取并过滤所选时间窗的记录；limit 限制响应条数，不保证底层只读取这些条数。
失败恢复：
  非法时间或游标先核对输入；证据存储不可用交由管理者恢复，不能把失败当成功空账。
  丢失游标时重新限定时间窗查询并自行核对重复；completeThrough 是本存储已记录的水位，不证明外部全部访问都被观察。
示例：
  例：kc operations audit access --repo kr://scene/object --limit 20
  例：kc operations audit access --repo kr://scene/object --limit 20 --continuation '<cursor>'` + accesscli.AuditUsageExtension(),
		},
		"operations audit trace": {
			Description: "查看请求追踪",
			Usage: `kc operations audit trace --trace-id <id>
用途：
  将一个 trace 已保存的访问、检索、精炼与反馈按时间关联，核对这次请求的证据链。
用法与参数：
  --trace-id 来自已有请求/证据的关联身份；此入口没有分页参数或 continuation，不能当作全服务事件导出。
条件：
  已认证并有 audit.read，服务证据账可读；需要的是已保存 trace 身份，不是对象 ID。
结果：
  JSON 返回 traceId 和 entries，kind 可为 access、retrieval、refine、feedback；不存在可见保留记录时 entries 为空。
后果：
  只关联已有证据，不重放访问、不调用模型、不改反馈或来源，也不保存模型思维链。
风险与成本：
  要读取多个证据流并按 trace 汇总排序；事件多时响应增长，此入口没有逐页续传保证。
  结果受证据保留窗口约束；源外直读、未记录动作或已清理分区不能凭 trace ID 找回。
失败恢复：
  缺少身份时核对原请求回执；读权或存储失败分别处理，不把读取故障解释为不存在 trace。
  大 trace 的访问记录可另用 audit access 限定时间窗分页；空链不能作为系统内外所有动作都未发生的证明。
示例：
  例：kc operations audit trace --trace-id tr-1
  例：kc operations audit access --trace-id tr-1 --limit 20`,
		},
		"operations audit hitmap": {
			Description: "按对象汇总检索命中，不是访问事件账",
			Usage: `kc operations audit hitmap [--limit <n>] [--continuation <cursor>]
用途：
  按已解析的对象及固定版本汇总访问使用次数，核对哪些对象被谁使用。
用法与参数：
  支持 audit access 的时间、主体、动作、trace、仓与对象过滤；--limit 默认 50、最多 200，0 使用默认。
  当前聚合结果不返回下一页游标，--continuation 不会推进聚合排名；不要将此入口当作可穷尽的对象分页。
条件：
  已认证并有查询范围的 audit.read，服务访问证据可读；不是对象检索或对象列举权限。
结果：
  按对象汇总命中。source 是 hitmap，不是 access；hits 含版本身份、计数、访问时间与主体统计。
  统计取 ALLOW/RESOLVED 访问证据，同一事件内同一目标去重，按仓、commit、对象及 Address 区分。
后果：
  派生已有证据的汇总，不访问对象正文、不改变排名规则、授权或对象内容。
风险与成本：
  需汇总所选热窗中的访问，再按次数排序和截断；小 limit 不会避免窗口内的聚合成本。
  计数受保留窗口与已记录访问限制，不是全历史使用量、业务正确性或训练标签；截断排名不是全部对象。
失败恢复：
  先核对时间窗、过滤与保留政策；存储故障恢复后重查，不因空汇总自动重建对象索引。
  需要逐条证据或真实续页时使用 audit access；不能不断重传 continuation 假装取完聚合结果。
示例：
  例：kc operations audit hitmap --limit 20
  例：kc operations audit hitmap --repo kr://scene/object --since 2026-10-01T00:00:00Z --until 2026-10-02T00:00:00Z`,
		},
		"operations feedback record": {
			Description: "记录检索反馈",
			Usage: `kc operations feedback record --trace-id <id> --dataset <id> --outcome <answered|accepted|rejected|corrected|helpful|unhelpful> [--message <text>]
用途：
  给已有对象访问 trace 保存一条使用反馈，为维护者核对问题和后续修订提供依据。
用法与参数：
  --outcome 只能是 enumerated 那六个词，不是 PASSED/FAILED；--trace-id 取自已有访问，--dataset 明确反馈范围。
  --message 可省略，去除首尾空白后最多 4096 字节；不要把新的请求关联身份当作原访问 trace。
条件：
  已认证并有 feedback.write 和相应 Dataset 范围的 file.read；当前服务保留账内必须已有该 trace 的 access 记录。
结果：
  JSON 返回 traceId、outcome、recorded:true 及可用的关联证据身份；记录成功不表示修订已批准或训练已完成。
后果：
  追加带主体、Dataset、原 trace 和结果的反馈证据；不写仓、不改 Dataset、不训练模型、不自动授权。
风险与成本：
  需要核对既有访问并耐久保存反馈；重复提交会追加记录，没有调用者反馈幂等键。
  反馈是使用者的判断，不自动成为对象真相或可训练标签；证据过期后不能通过补造 trace 恢复关联。
失败恢复：
  PRECONDITION_FAILED 时核对原访问 trace 是否存在于当前服务/保留窗口；无权时取得相应授权，不能改 Dataset 范围绕过。
  响应不确定时请有 audit.read 的管理者用 audit trace 核对后再决定重试；存储失败不能当作 recorded:true。
示例：
  例：kc operations feedback record --trace-id tr-1 --dataset scene-set --outcome helpful
  例：kc operations audit trace --trace-id tr-1`,
		},
	}
}
