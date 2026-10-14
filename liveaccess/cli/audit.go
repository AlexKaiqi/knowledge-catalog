package cli

// AuditRequestID is the module's operand contribution to the existing audit
// task. --request-id still correlates the new query request itself.
func AuditRequestID(flags map[string]any) string { return flagString(flags, "filter-request-id") }

func AuditUsageExtension() string {
	return `
LiveAccess 复核：
  --filter-request-id <原请求标识> 筛选原读取；--request-id 仅标识本次查询。
  results 保留当次采用的值（包括 null）、逐值失败、固定声明和原观察依据。
  Snapshot 用记录的固定版本和位置回读；审计不向外部来源重新取值。
  resultFailure 区分保留期内证据丢失（TEMPORARY_UNAVAILABLE）与正常到期（CAPABILITY_UNSATISFIED）。
  例：kc operations audit access --filter-request-id read-1`
}
