package cli

type helpEntry struct{ Description, Usage string }

func leafHelpEntries() map[string]helpEntry {
	return map[string]helpEntry{
		"operations projection describe": {Description: "查看 Dataset 修订的投影状态", Usage: `kc operations projection describe --dataset <id> [--revision <N>]
用途：
  核对该修订选择范围内每个来源的索引依据与就绪信息。
用法与参数：
  --dataset 必填；--revision 固定已接受版本，省略采用当前修订。
条件：
  已认证并持有 Dataset 的完整 file.read；服务已配置索引实例。
结果：
  JSON dataset/revision 固定本次范围，members 给真实来源 repository/commit；单份 snapshot 报该 Dataset 投影的状态、摘要与计数。
后果：
  读取描述，不构建索引、不发布 Dataset、不推进来源 HEAD。
风险与成本：
  状态观察可能访问后端统计；缺失或失败不能当已就绪的零对象。
失败恢复：
  核对声明选择与实例故障；由维护者用同一修订 sync，再读 describe 核验。
示例：
  kc operations projection describe --dataset scene-set --revision 1`},
		"operations projection sync": {Description: "显式准备 Dataset 固定修订的索引", Usage: `kc operations projection sync --dataset <id> [--revision <N>]
用途：
  为已接受修订的选入声明与范围恢复派生投影。
用法与参数：
  --dataset 必填；--revision 固定发布版，省略采用当前修订。
条件：
  已认证并有该 Dataset 的 dataset.manage；file.read 消费权不能触发构建。
结果：
  JSON dataset/revision 与 members 的真实来源 repository/commit 固定范围；snapshot 与可用 state 保留整份 Dataset 的真实准备结果。
后果：
  编译与写入派生索引，不推进来源 HEAD、不发布新修订、不改变授权。
风险与成本：
  构建可能扫描所选内容、调用共享外部值来源，失败时不回滚已完成派生工作。
失败恢复：
  修复具体成员或后端故障后按同一修订重试，再用 describe 观察状态。
示例：
  kc operations projection sync --dataset scene-set --revision 1`},
	}
}
