package cli

func hostGovernanceHelpEntries() map[string]helpEntry {
	return map[string]helpEntry{
		"governance proposal create": {
			Description: "把变更写到 candidate 并打开 Proposal，不推进 target",
			Usage: `kc governance proposal create --repo <id> --proposal-id <id> --candidate <ref> (--value <json> | --file <path> | --changeset <file>) [--object <id>] [--target <ref>] [--base <commit>] [--message <text>]
用途：
  把待评审的对象变更写到候选引用，保存可供 Preview 和合并使用的 Proposal。
用法与参数：
  --repo 是目标仓；--proposal-id 是本次提案身份，--candidate 是候选引用；--target 默认 refs/heads/main。
  --object 指定本次候选变更的对象身份，--value 提供正文；--file 读取本机内容，--changeset 读取操作集合，两种输入不要混用。
  --base 指定目标基点；省略时读取目标当前提交。--message 保存提案说明。
条件：
  已认证并有目标范围的 governance.proposal.create；来源仓可用，输入满足 Writer 的结构、来源与候选写入约束。
结果：
  JSON 返回 proposalId、targetRepository、targetRef、candidateRef、baseCommit、candidateCommit 及可选 rationale。
后果：
  把这次变更写到 --candidate，不推进 --target（默认 refs/heads/main）；同时保存治理记录，后续评审引用这个候选 commit。
  不自动创建 Preview、不运行检查、不合入目标，也不发布新版 Dataset。
风险与成本：
  要读取输入并提交候选，成本随变更内容增长；候选写入与治理记录保存跨两个阶段，失败不能一概解释为候选未写入。
  目标或候选后来变化会影响后续合并；同一 proposal-id 不能用于另一份变更。
失败恢复：
  输入或来源错误先修正原因；响应丢失或治理保存失败时按原 proposal-id 和原输入核对 Writer 回执，再决定重试。
  不因提案失败而重建来源仓、覆盖别人的候选或回滚已接受提交。
示例：
  例：kc governance proposal create --repo kr://scene/object --proposal-id PR-1 --candidate refs/heads/change --object note/hello --value '{"text":"proposed"}'
  例：kc writer receipt --command-id proposal:PR-1`,
		},
		"governance proposal merge": {
			Description: "合入 Proposal",
			Usage: `kc governance proposal merge --proposal <id> --preview <id> [--validation <id>]
用途：
  将已经评审和验证的候选合入 Proposal 记载的目标引用，取得新的发布提交。
用法与参数：
  --proposal 与 --preview 使用已保存的身份；目标 Repository/Ref 从 Proposal 推导，不另传 --repo 替换目标。
  有匹配 Gate 时核对该 Preview 的全部已保存证据；没有匹配 Gate 时仍须 --validation 指定一份 PASSED 结果。
条件：
  已认证并有 Proposal 目标范围的 governance.merge；Proposal、Preview 和证据存在，候选与目标仍符合固定基点及并发检查。
结果：
  JSON 返回 commitId、proposalId、previewId、repository、targetRef 和 gate 的 status/basis/required。
后果：
  通过精确 Preview、证据和候选检查后条件推进目标引用，并通知投影维护；不自动重新发布已冻结的 Dataset。
  后置 Hook 可通知外部系统，投递失败不回滚已经成功的合并。
风险与成本：
  合并持久改变目标仓，Gate 通过不能关闭目标 CAS；候选已推进或目标已变化时不能沿用旧评审结果。
  涉及证据读取、存储合并和派生维护；源合并成功不等于搜索已就绪或消费者已采用新版。
失败恢复：
  GATE_UNSATISFIED 时补齐同一 Preview 的必需证据；依据不匹配或 CANDIDATE_MOVED 时重新形成有效提案/Preview 并检查。
  响应丢失时先用 writer head 核对目标提交，不盲目重复发布，也不为修复通知撤销目标提交。
示例：
  例：kc governance proposal merge --proposal PR-1 --preview PV-1
  例：kc governance proposal merge --proposal PR-1 --preview PV-1 --validation val-PV-1-structure`,
		},
		"governance preview create": {
			Description: "把 proposal 叠到本次解析的 Dataset 上，给出 candidate 坐标，不改 main",
			Usage: `kc governance preview create --proposal <id> --dataset <id>
用途：
  固定本次 Dataset 的成员提交，并在这份依据上替换提案目标成员，形成可评审的候选组合。
用法与参数：
  --proposal 来自 proposal create；--dataset 是已发布 Dataset，不是本机目录或可移动分支名。
  本次命令解析 Dataset 一次，冻进 Preview；提案基点必须等于本次解析中对应成员的提交。
条件：
  已认证并有该 Dataset 范围的 governance.preview.create；Dataset 已发布，提案已保存，目标是成员，候选提交存在。
结果：
  stdout 是 Preview 坐标（previewId、candidate），还含 setId、repositories 和 baseCommit，不是合入后正文。
后果：
  保存固定 Preview 和治理记录；不推进目标/main，不改 Dataset 服务版，不在这里运行验证。
风险与成本：
  需要解析组合、核对来源版本并保存 Preview；固定内容依据不保留被撤销的权限。
  旧 Dataset 未采用提案基点时会拒绝；重新解析可能得到另一份依据，不能把它当成旧评审的延续。
失败恢复：
  VALIDATION_BASIS_MISMATCH 时核对 Proposal 的 base 与 Dataset 成员，按实际目标形成匹配的发布依据或新提案。
  来源或保存失败先恢复对应服务；不能用跳过成员、换版本或伪造 pin 让预览成功。
示例：
  例：kc governance preview create --proposal PR-1 --dataset scene-notes
  例：kc governance preview validate --preview PV-1`,
		},
		"governance preview validate": {
			Description: "校验 Preview 的协议结构",
			Usage: `kc governance preview validate --preview <id>
用途：
  检查固定 Preview 的成员登记和提交是否仍可解释，保存一份 structure 验证证据。
用法与参数：
  --preview 使用 preview create 返回并已保存的 previewId；检查不会重新解析同名 Dataset 替换它。
条件：
  已认证并有该 Preview 所属 Dataset 范围的 governance.validate；服务能读取预览及来源登记。
结果：
  JSON 返回 reportId、previewId、suiteRevision:structure、outcome 和 check 中的检查问题。
  outcome 可以是 FAILED；命令返回检查报告不表示验证通过，应核对 outcome 与 issues。
后果：
  保存绑定该 Preview 的 PASSED/FAILED 证据及治理记录；不合并候选、不改 Dataset、不运行外部套件。
风险与成本：
  检查成员登记、提交及候选存在性，成本随成员数量增长；内建结构通过不能证明业务正确或外部 CI 通过。
  同一 Preview 的 structure 使用同一报告身份，再次检查会替换该身份当前保存的结果。
失败恢复：
  根据 issues 修复成员或来源可用性；预览依据已不成立时重新创建有效 Preview 后重新检查。
  保存证据失败不能当作通过；不得把外部场景或验收套件塞进此入口代跑。
示例：
  例：kc governance preview validate --preview PV-1
  例：kc governance validation record --preview PV-1 --outcome PASSED --suite lint`,
		},
		"governance validation record": {
			Description: "记录外部套件结果",
			Usage: `kc governance validation record --preview <id> --outcome <PASSED|FAILED> --suite <id> [--message <text>]
用途：
  将调用方在平台外得到的检查结果绑定到一份固定 Preview，供合并时核对证据。
用法与参数：
  --preview 是已保存预览；--suite 标识实际检查套件/版本。--outcome 只接受 PASSED 或 FAILED，是调用方断言，不在这里跑套件。
  当前 typed CLI 请求不保存 --message；需要保留详细报告时在外部检查系统按 Preview 与 suite 关联。
条件：
  已认证并有该 Preview 所属范围的 governance.validation.record；Preview 存在，调用者应先完成对应外部检查。
结果：
  JSON 返回 reportId、previewId、suiteRevision 和 outcome；记录成功表示结果已保存，不表示平台执行或复核了外部套件。
后果：
  保存治理证据，不推进候选/目标、不合并提案，也不授予记录者其它套件或合并权限。
风险与成本：
  同一 Preview/suite 使用同一 reportId，重复记录会替换当前保存结果；检查身份要足以区分实际套件版本。
  错录 PASSED 可能满足对应 Gate，记录权与外部检查责任须明确；数据保存失败不等于外部检查未执行。
失败恢复：
  非法 outcome 或未知 Preview 先核对输入；依据变化后须针对新 Preview 重新运行外部检查和记录。
  响应不确定时核对已有证据再重试，不借更换 suite 名称绕过必需检查，也不把 FAILED 写成 PASSED 来消除拒绝。
示例：
  例：kc governance validation record --preview PV-1 --outcome PASSED --suite structure
  例：kc governance validation record --preview PV-1 --outcome FAILED --suite lint`,
		},
	}
}
