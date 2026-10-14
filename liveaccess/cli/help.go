package cli

type helpEntry struct{ Description, Usage string }

func leafHelpEntries() map[string]helpEntry {
	return map[string]helpEntry{
		"binding show": {
			Description: "查看 Binding 声明",
			Usage: `kc binding show (--repo <id> | --dataset <id>) --object <object-id> --aspect <name>

用途：
  核对对象 Aspect 的 Bound State 声明，确定其固定 Schema origin 与绑定依据。
用法与参数：
  --object 与 --aspect 必填；集合成员可加 --member <key>。
  单仓可用 --commit 或 --ref；Dataset 从本次固定选择范围解析绑定，不接受任意源 URL。
条件：
  已认证；单仓需 object.binding.resolve，命名 Dataset 以其 file.read 准入，不另要求来源仓 object.binding.resolve。
 Dataset 须通过所需对象与选择范围的完整读取检查，声明及 Schema 须在合法清单内；另行解析 pin 需 dataset.resolve。
  所选 Aspect 或成员须有可解析的 Bound State origin。
结果：
  JSON 返回解析的 Binding，包含对象 Address、schemaRef、origin 和声明的固定版本依据；Dataset 可返回多个绑定。
后果：
  仅读取绑定声明，不读取墙外值、不调用源、不安装运行时，也不改写 origin。
风险与成本：
  需要回读对象与 Schema，成本随来源数量增长；绑定解析成功不证明源在线、调用已获源授权或运行时已部署。
  相同对象 id 在多个仓仍是不同绑定，不能任取一个当作其它来源的替身。
失败恢复：
  无绑定时用 schema describe 核对 origin 与 Aspect 类型；缺对象时先 resolve/read 核对声明版本。
  需当前值时另行 access；运行时问题应由源维护者处理，不能靠改客户端目标绕过绑定。
示例：
  例：kc binding show --repo kr://scene/object --object table/orders --aspect stats
  例：kc binding show --dataset scene-set --object table/orders --aspect stats`,
		},
		"access": {
			Description: "按 Schema origin 与对象 ID 读取该 Aspect 的当前观察",
			Usage: `kc access (--repo <id> | --dataset <id>) --object <object-id> --aspect <name>

用途：
  按固定 Binding 的 origin 和对象坐标实际访问源，读取指定 Aspect 的当前值。
用法与参数：
  --object 与 --aspect 必填；可用 --member <key> 选择成员，单仓可用 --commit 或 --ref 固定绑定声明。
  不接受 --operation、--input 或 --implementation；调用对象能力请用 invoke。
条件：
  已认证并有目标范围的 resource.access；Dataset file.read 不蕴含此访问权，单仓不另要求 object.binding.resolve。
  命名 Dataset 还须通过选择范围的完整读取检查，整范围 file.read 可满足，不另要求来源仓 object.read。
  另行解析 Dataset pin 需 dataset.resolve；绑定声明须可解析，源运行时须可用并按其授权规则允许访问。
  origin 来自所选 Domain Schema，服务将请求身份和追踪上下文传给运行时，不由此命令另选访问地址。
结果：
  JSON 展示当前 value、basis 及适用的 schemaRef/对象/切面坐标；basis 用于核对观察时间与源版本等依据。
  当前 CLI 多绑定结果只展示首条观察；要完整核对多来源需使用 typed API，或明确指定单仓。
后果：
  实际调用外部源读取状态；不发布绑定声明，不把观察值写回 Repository，也不执行指定的源操作。
风险与成本：
  每个绑定都可能产生网络、源计算、限流或计费；声明 commit 不固定当前观察，重复读取可得到不同值。
  源失败会使本次命令失败，先前源读取可能已发生；不能把错误当作合法 null，缺观察依据也不能据此证明业务事实。
失败恢复：
  先 binding show 核对固定 origin；授权失败分别核对服务访问权与源侧授权，源不可用请其维护者处理。
  修复后可重新读取，但会产生新的观察；复核已交付观察用 operations audit access/trace，须有独立 audit.read。
  声明 commit 只能复核声明，不能代替当次外部值及其观察证据。
示例：
  例：kc access --repo kr://scene/object --object table/orders --aspect stats
  例：kc access --dataset scene-set --object table/orders --aspect stats`,
		},
		"invoke": {
			Description: "调用对象声明的插件操作（LiveAccess）或兼容的 ResourceDescriptor 操作",
			Usage: `kc invoke (--repo <id> | --dataset <id>) --object <object-id> --implementation <member-key> --operation <name> --input <json>

用途：
  根据对象已声明的插件实现和固定合同，调用已安装插件的处理器或源操作。
用法与参数：
  --implementation 选择 interfaces 切面中的实现；--operation 必填，--input 必须是一个完整 JSON 值，用单引号包住。
  不接受 --aspect/--member。省略 implementation 时走兼容的 ResourceDescriptor access 操作声明。
  单仓可用 --commit 或 --ref 固定声明；Dataset 的所选实现须能唯一解析，不能任取同名来源。
条件：
  已认证并有目标范围的 resource.access；Dataset file.read 不蕴含此调用权，单仓不另要求 object.read。
  命名 Dataset 还须通过选择范围的完整读取检查，整范围 file.read 可满足，不另要求来源仓对象动作。
  另行解析 Dataset pin 需 dataset.resolve；操作已声明，相关对象/合同可解析，运行时已装配且源允许调用。
  插件合同、实现 Schema 与处理器须按固定版本匹配；声明存在不等于插件已安装或外部操作已获授权。
结果：
  JSON 返回处理器或源操作输出；插件结果附固定声明与合同依据，LiveAccess read 的当前值另带观察依据。
  兼容 ResourceDescriptor 路径按其声明调用源并返回源结果，不等于读取对象固定正文。
后果：
  立即执行选定能力；read 可访问实时源，其它操作可能修改外部状态、排队任务或产生外部费用。
  固定对象声明不能冻结外部执行结果；此命令不提供通用事务、撤销或幂等承诺。
风险与成本：
  代价和可逆性由具体操作合同决定，执行前需读清输入、输出和副作用；请求 id 只是关联依据，不是通用幂等键。
  超时、响应丢失或执行后输出校验失败都不能推出“源未执行”，盲目重试可能重复写入或重复扣费。
失败恢复：
  用法/合同不匹配时先核对对象实现、操作和输入；能力未满足时请部署或源维护者安装匹配插件。
  执行结果不明时用请求追踪及源侧状态确认是否已完成，再按操作自己的恢复约定处理；不要默认重发安全。
示例：
  例：kc invoke --repo kr://scene/object --object table/orders --implementation live --operation read --input '{}'
  例：kc invoke --repo kr://scene/object --object handle/orders --operation preview --input '{}'
  例：kc invoke --dataset scene-set --object handle/orders --operation preview --input '{}'`,
		},
	}
}
