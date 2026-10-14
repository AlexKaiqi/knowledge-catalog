package cli

import "strings"

var commandEntries = []Command{
	{Path: "read", Handler: "object-read", Action: "object.read", Flags: words("dataset commit ref"), Operands: []string{"reference"}, Groups: []string{"object"}},
	{Path: "resolve", Handler: "object-resolve", Action: "object.read", Flags: words("repo dataset commit ref object aspect member"), Groups: []string{"object"}},
	{Path: "log", Handler: "object-log", Action: "object.history.read", Flags: words("repo dataset commit ref object limit continuation"), Groups: []string{"object"}},
	{Path: "provenance", Handler: "object-provenance", Action: "object.provenance", Flags: words("repo dataset commit ref object"), Groups: []string{"object"}},
	{Path: "schema list", Handler: "object-schema-list", Action: "object.schema.read", Flags: words("repo commit ref limit continuation"), Groups: []string{"object"}},
	{Path: "schema describe", Handler: "object-schema-describe", Action: "object.schema.read", Flags: words("repo dataset commit ref object"), Groups: []string{"object"}},
	{Path: "writer put", Handler: "writer-put", Action: "writer.commit", Flags: writeFlags("value file value-source schema-ref path-hint"), Operands: []string{"reference"}, Groups: []string{"writer", "publishing"}},
	{Path: "writer remove", Handler: "writer-remove", Action: "writer.commit", Flags: writeFlags(""), Operands: []string{"reference"}, Groups: []string{"writer", "publishing"}},
}

func words(raw string) []string { return strings.Fields(raw) }
func writeFlags(extra string) []string {
	return words("command-id kind ref base expected if-absent if-digest message origin-kind source-ref evidence-ref actor-ref activity-ref input-dataset-version algorithm-spec algorithm-model algorithm-hash produced-at " + extra)
}

// Commands returns independent declarations so a host cannot mutate the module.
func Commands() []Command {
	entries := make([]Command, len(commandEntries))
	for i, entry := range commandEntries {
		help := leafHelp[entry.Path]
		entry.Description = help.Description
		entry.Usage = help.Usage
		entry.Flags = append([]string(nil), entry.Flags...)
		entry.Operands = append([]string(nil), entry.Operands...)
		entry.Groups = append([]string(nil), entry.Groups...)
		entries[i] = entry
	}
	return entries
}
func command(path string) (Command, bool) {
	for _, entry := range commandEntries {
		if entry.Path == path {
			return entry, true
		}
	}
	return Command{}, false
}

// ModelHelp is a local root-help contribution; it does not open a repository.
func ModelHelp() string {
	return `对象模型：
  object_id 是来源 Repository 内的对象身份；同名对象在不同仓仍是不同对象。
  Aspect 是对象的具名完整切面；集合 Aspect 的 Member 用稳定 key 标识。
  Address 指定可独立维护的整个对象、Aspect 或 Member；Relation 是有独立身份、角色端点并引用对象的关系声明。
  对象 URN 编码来源仓与 object_id；# 后的 JSON Pointer 定位内容，版本由读取依据另行固定。
  例：订单对象的 /columns/order_id 是 Member，/columns/order_id/constraints/nullable 是其中的数据。
  kc read 'urn:kc:object:kr%3A%2F%2Fscene%2Fobject:table%2Forders#/columns/order_id/constraints/nullable'
  read 读取正文或指定数据；writer put/remove 替换或删除完整维护单元；log/provenance 核对历史与来源。
  Schema 可选，采用后约束对应对象；schema list/describe 查看其结构与约束，读取深字段不产生新的写粒度。
  深入版本化模型定义见 kc help read 的系统对象示例；是否可读由实际服务出版与当前权限决定。
  具体参数与恢复：kc help read | kc help writer put | kc help writer remove | kc help schema

`
}
