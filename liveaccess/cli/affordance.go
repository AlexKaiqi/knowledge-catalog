// Package cli owns the LiveAccess plugin's static user explanation.
package cli

import "kc/discovery/adoption"

type PluginAffordance = adoption.PluginAffordance

// Affordance reads no configuration and performs no business or source I/O.
func Affordance() PluginAffordance {
	return PluginAffordance{
		Name: "liveaccess", Summary: "按固定声明取得外部实际值，传递本次适用认证，并保留可独立复核的访问证据。",
		Tasks: []string{"read", "access", "binding show", "invoke", "operations audit access", "operations audit trace"},
		Usage: `LiveAccess

说明入口：
  kc plugin liveaccess help
取值协议：
  LookupState(ctx, StateLookupRequest) → (StateObservation, error)。
  请求含 Binding、SchemaRef、Origin、Identity、Trace、RequestID；成功返回 Value、Basis，失败返回原因。
  Basis 保留来源代际、一致性、观察时间及可选的源版本、水位，不能用声明版本冒充外部观察。
适用条件：
  插件由部署装配，不绑定仓或 Dataset；取值来源写在方面或成员上，在读到相应单元时生效。
  取值来源以 descriptorRef 引用固定取值描述，部署按其 runtime + protocol 匹配插件实例。
  访问按本次固定范围和对象、Schema、Binding 或实现声明核验；没有匹配能力明确失败，不回退全局实现。
结果影响：
  对象读取无感取得实际值；合法 null 保持 null 及成功依据。
  外部值失败只在原位置给出说明，其他可读值保留；不以旧值、空值或句柄掩盖失败。
  仅 identity 为 shared 的取值描述所对应的外部值可按索引声明采用，并保留原观察依据。
  动态命中回读取得新观察后按查询条件补判，不再满足的命中淘汰；per-caller 外部值只在读取时按调用者取值。
认证承诺：
  自动传递本次适用认证，仅交给可信适用目标；地址变化、重定向不扩大凭证范围。
  凭证不进入对象、日志或审计。透明取值只读，源修改必须显式调用并授权。
独立审计承诺：
  记录访问主体、时间、实际位置及 requestId；保存当次采用值或失败、固定声明和原观察依据。
  证据独立授权查询，源变化后仍能复核；留证可靠后交付，丢失与到期分别说明。
相关入口：
  kc read；kc access；kc binding show；kc invoke；kc operations audit access；kc operations audit trace。
  参数见 kc help <任务>。本说明只读，不访问来源、不展示凭证，也不证明当前配置或就绪。`,
	}
}
