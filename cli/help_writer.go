package cli

func hostWriterHelpEntries() map[string]helpEntry {
	return map[string]helpEntry{
		"diff": {
			Description: "对照当前发布版本，列出这个目录会改哪些对象",
			Usage: `kc diff --repo <id> --dir <drafts>
  用途：
    在发布前查看目录中声明的 Address 相对仓内固定基点有哪些正文差异。
  用法与参数：
    --dir 是期望正文。对照当前发布版本列出 add / update，不写仓。
    --repo 是一个知识 Repository，不能用 Dataset；可用 --ref 选择目标引用、--base 固定对照 commit。
    默认 ref 为 refs/heads/main，未给 --base 时读取该引用的当前 HEAD。
  条件：
    目录可读且知识文件可解析；当前主体可读取基点和解析目录涉及的 Address。
  结果：
    返回 repository、commit 对照基点和 add/update 明细；正文相同的 Address 不列为变更。
  后果：
    不发布、不修改本机目录；隐藏文件/目录不参与，目录中没有声明的仓内对象保留。
  风险与成本：
    读取全部参与文件并逐 Address 查询基点，成本随目录内容增长。
    diff 只是正文对照，不替代 Writer 的 Schema、前置条件、幂等或当前授权校验；下一条命令可能采用新 HEAD。
  失败恢复：
    修复文件声明、读取权限或来源后重新 diff；准备发布时用固定 --base 防止静默换基点。
    想删除 Address 须显式 writer remove，不能依赖删掉本机文件。
  示例：
    例：kc diff --repo kr://scene/knowledge --dir drafts
    例：kc diff --repo kr://scene/knowledge --dir drafts --base commit-123`,
		},

		"writer commit": {
			Description: "把目录里的最终样子发布成仓的新版本",
			Usage: `kc writer commit --command-id <id> --repo <id> --dir <drafts> [--bulk]
  用途：
    把目录中声明的正文变更作为一次单仓发布；不把未出现在目录中的对象当成删除。
  用法与参数：
    --dir 是期望正文。Client 对照当前版本求差后提交。--command-id 由你本次提供，Client 不代填。
    --ref 默认 refs/heads/main；可用 --base 固定基点，省略时读取当前 HEAD；隐藏文件/目录不参与。
    也可用 --changeset <file> 提交明确 ChangeSet，与 --dir 互斥；目标、操作和前置条件来自该文件。
    --bulk 一次性接入大量文件时走介质批量通路；介质不支持时整条拒绝，不带时行为不变。
  条件：
    目录/ChangeSet 可读且解析合法，只有一个知识 Repository 目标，当前主体有相应读取与写入权限。
    所有操作须通过 Writer 的 Schema、来源与并发校验；不能写 Dataset、登记权威或 system 仓。
  结果：
    一次 Commit 的耐久回执及 oldCommit/newCommit；同 command-id 同摘要返回原回执。
    目录正文完全相同时，不创建空提交；若该 id 已有回执则返回该回执，否则报告已与当前版本一致。
  后果：
    仅发布目录里实际不同的单元，目录外已有对象保留；删除须显式 writer remove 或 ChangeSet REMOVE。
    一次请求不自动拆批，--bulk 仅改变介质传输方式，CAS 与单次 commit 语义不变。
    手工分成多次发布时各命令独立，前批成功不会因后批失败撤销；投影维护也不回滚已接受提交。
  风险与成本：
    目录全量读取、逐 Address 对照、校验和远端写入，时间与内存随参与内容增长。
    网络中断后客户端只轮询原回执，默认最多十分钟；同 id 异摘要为 IDEMPOTENCY_CONFLICT。
  失败恢复：
    先按原 command-id 查询回执；等待结束或 PENDING 不能解释为未提交，保留原请求供对账。
    并发冲突重新 diff 并选择新基点；内容变化后的新发布使用新 id，不能借旧 id 提交另一份目录。
    若自己拆批，先确认每批已完成的回执，再处理未完成批次，不能假定整批回滚。
  示例：
    例：kc writer commit --command-id commit-1 --repo kr://scene/knowledge --dir drafts
    例：kc writer commit --command-id commit-bulk-1 --repo kr://scene/knowledge --dir drafts --bulk
    例：kc writer receipt --command-id commit-1`,
		},
		"writer head": {
			Description: "查看 Repository 当前发布版本",
			Usage: `kc writer head --repo <id>
  用途：
    取得一个仓目标 ref 当前指向的发布 commit，供对照和并发条件使用。
  用法与参数：
    --repo 指定单仓；--ref 可选择引用，默认 refs/heads/main。
  条件：
    当前身份通过认证且有该仓/引用的 writer.preview 能力，来源可读且引用存在。
  结果：
    返回 repository 和 commit；不返回对象正文或目录明细。
  后果：
    不推进引用、不发布知识；这次查询也不为后续命令保留锁或冻结权限。
  风险与成本：
    一次 HEAD 查询；结果可能随后被其它发布推进，不能当成永久最新版本。
  失败恢复：
    核对仓/引用、当前授权与来源可用性；提交时将选定 commit 明确作为基点，冲突后重新核对。
  示例：
    例：kc writer head --repo kr://scene/knowledge
    例：kc writer head --repo kr://scene/knowledge --ref refs/heads/main`,
		},
		"writer receipt": {
			Description: "查询 Writer 命令回执",
			Usage: `kc writer receipt --command-id <id>
  用途：
    按原命令身份查询耐久结果，尤其在提交响应丢失后确认发生了什么。
  用法与参数：
    --command-id 必须是原 Writer 请求的 id；无需重新传 --repo 或 --ref，范围由账本确定。
  条件：
    连接到保存原命令账的 Server，当前主体拥有账本目标仓/引用的 writer.receipt.read 权限。
  结果：
    返回命令记录、状态和已有回执；APPLIED 回执携带采用的 targetRef、oldCommit/newCommit。
    PENDING 还没有最终回执，ABANDONED 表示已由维护流程明确放弃；未知 id 返回错误。
  后果：
    只读命令账，不重放写入、推进引用、恢复缺失账本或发权。
  风险与成本：
    一次耐久账查询；知道 id 不是读取授权。已完成记录受部署保留窗口约束，
    查不到或暂时无法查不证明原提交未发生，也不意味着可以安全换 id 再写。
  失败恢复：
    核对 Server、原 id 和当前权限；PENDING 或无最终结果时保留原请求交运营者对账。
    原提交已有成功回执时据 newCommit 精确读取，不因随后审计、索引或通知失败重复发布。
  示例：
    例：kc writer receipt --command-id commit-1`,
		},
	}
}
