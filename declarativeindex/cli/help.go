package cli

type helpEntry struct{ Description, Usage string }

func leafHelpEntries() map[string]helpEntry {
	return map[string]helpEntry{
		"index": {Description: "查看某个 Dataset 修订能不能搜，或无参查看当前追赶进度", Usage: `kc index [--dataset <id> [--revision <N>]]
用途：
  不带 --dataset 时读服务当前这一趟索引构建的进度；带 --dataset 时回答该修订现在能不能搜。
用法与参数：
  --dataset 可选；--revision 固定已接受版本，省略采用当前修订。--revision 必须与 --dataset 同用。
条件：
  已认证并持有 Dataset 的完整 file.read；服务已配置索引实例。
结果：
  JSON dataset/revision/ready，已就绪时附 objectCount。未就绪是正常答案，不是故障。
  无参时给 repository/done/total，没有正在进行的构建时不输出内容。
后果：
  只读，不构建索引、不发布 Dataset、不推进来源 HEAD。
风险与成本：
  状态观察可能访问后端统计；缺失或失败不能当成"已就绪的零对象"。
失败恢复：
  未就绪时用 kc index describe 看卡在哪，或由维护者用 kc index prepare 显式准备。
示例：
  kc index --dataset scene-set --revision 1
  kc index`},
		"index prepare": {Description: "显式准备某个 Dataset 修订的索引", Usage: `kc index prepare --dataset <id> [--revision <N>]
用途：
  为已接受修订的选入声明与范围构建检索语料，用于重试失败的构建。
用法与参数：
  --dataset 必填；--revision 固定已接受版本，省略采用当前修订。
条件：
  已认证并有该 Dataset 的 dataset.manage；file.read 消费权不能触发构建。
结果：
  JSON dataset/revision/ready 与 objectCount，格式与 kc index 的就绪行一致。
后果：
  编译并写入派生索引，不推进来源 HEAD、不发布新修订、不改变授权。
风险与成本：
  构建可能扫描所选内容、调用共享外部值来源；失败时该修订停在未就绪，不回滚已接受事实。
失败恢复：
  修复具体成员或后端故障后按同一修订重试，再用 kc index describe 观察。
示例：
  kc index prepare --dataset scene-set --revision 1`},
		"index describe": {Description: "查看某个 Dataset 修订的索引诊断信息", Usage: `kc index describe --dataset <id> [--revision <N>]
用途：
  就绪失败时回答缺哪个成员、各自处于哪个阶段、basis 指到哪个提交、是否曾失败。
用法与参数：
  --dataset 必填；--revision 固定已接受版本，省略采用当前修订。
条件：
  已认证并持有 Dataset 的完整 file.read；服务已配置索引实例。
结果：
  JSON dataset/revision 固定范围，members 给真实来源 repository/commit，以及该修订的索引状态、摘要与计数。
后果：
  读取描述，不构建索引、不发布 Dataset、不推进来源 HEAD。
风险与成本：
  状态观察可能访问后端统计；比 kc index 贵，日常就绪判断不要用它。
失败恢复：
  核对声明选择与实例故障；修复后用同一修订 prepare，再读 describe 核验。
示例：
  kc index describe --dataset scene-set --revision 1`},
	}
}
