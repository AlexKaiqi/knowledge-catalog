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
    --repo 是一个对象 Repository，不能用 Dataset；可用 --ref 选择目标引用、--base 固定对照 commit。
    默认 ref 为 refs/heads/main，未给 --base 时读取该引用的当前 HEAD。
    目录文件采用 kc help writer commit 的同一合同；不是 writer put --file 的纯 JSON 值。
  条件：
    目录可读且对象文件可解析；当前主体可读取基点和解析目录涉及的 Address。
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
    例：kc diff --repo kr://scene/object --dir drafts
    例：kc diff --repo kr://scene/object --dir drafts --base commit-123`,
		},

		"writer commit": {
			Description: "把目录里的最终样子发布成仓的新版本",
			Usage: `kc writer commit --command-id <id> --repo <id> --dir <drafts> [--bulk]
  用途：
    把目录中声明的正文变更作为一次单仓发布；不把未出现在目录中的对象当成删除。
  用法与参数：
    --dir 是期望正文。Client 对照当前版本求差后提交。--command-id 由你本次提供，Client 不代填。
    --dir 也可指向 .tar / .tar.gz / .tgz 归档，用 archive.tar.gz//子目录 指定归档内充当仓根的目录；归档在本机流式读取并并发解析，拒绝越界、符号链接、重复条目和空子目录。
    --ref 默认 refs/heads/main；可用 --base 固定基点，省略时读取当前 HEAD；隐藏文件/目录不参与。
    首次上传到空仓时自动省去逐对象旧值查询；目录直接生成提交，无需手写 ChangeSet。
    也可用 --changeset <file> 提交明确 ChangeSet，与 --dir 互斥；目标、操作和前置条件来自该文件。
    --bulk 一次性接入大量文件时走介质批量通路；介质不支持时整条拒绝，不带时行为不变。
  目录文件格式：
    一个文件声明一个完整对象单元；文件首行和头部结尾各为 ---，头部给身份与 Schema，后面是正文。
    YAML/JSON 正文的文件用 object_id；方面加 aspect_name 与 kind: Aspect，成员再加 member_key 与 kind: Member。
    schema_ref 采用已发布或同批 Schema 的 object_id；Schema 文件本身的 object_id 使用 schema/ 命名空间。
    Schema 原始 fields 是按字段名组织的对象映射；schema describe 的 fields 数组是规范化展示，不能作为输入。
    优先从公开 Dataset 下载已有 Schema 与对象文件复用格式，入口见 kc help write。
    path_hint 是仓内相对路径，不是对象身份；普通单元省略时采用目录中的相对路径，目录名不替代文件头。
    Schema 固定放入 _schemas/ 规范路径；其它单元（包括任意多个 readme 方面）按各自 path_hint 存放。
    Markdown 说明方面用 entity / aspect，正文直接写 Markdown，不包 JSON；以下是 drafts/README.md 的完整内容，复制时去掉四格缩进。
    该示例复用系统 schema/core/readme/v1；准备自己的说明时换 entity，引用的 Schema 必须可解析。
    ---
    entity: example/notes
    aspect: readme
    schema_ref: schema/core/readme/v1
    path_hint: README.md
    ---
    # 资料说明
    这里记录资料的用途、来源和维护人。
  输入区别：
    writer put --file 只接收一个纯 JSON 值，不接收以上 frontmatter 文件；身份与 Schema 由 put 的参数提供。
    目录中的 JSON/YAML 正文必须符合所引用的 Schema；原始 SQL、CSV 等资料需按对象模型整理后再提交。
  条件：
    目录/ChangeSet 可读且解析合法，只有一个对象 Repository 目标，当前主体有相应读取与写入权限。
    所有操作须通过 Writer 的 Schema、来源与并发校验；不能写 Dataset、登记权威或 system 仓。
  结果：
    一次 Commit 的耐久回执及 oldCommit/newCommit；同 command-id 同摘要返回原回执。
    目录正文完全相同时，不创建空提交；已有 id 先按原基点重建请求核对摘要，一致才返回原回执。
  后果：
    仅发布目录里实际不同的单元，目录外已有对象保留；删除须显式 writer remove 或 ChangeSet REMOVE。
    一次请求不自动拆批，--bulk 仅改变介质传输方式，CAS 与单次 commit 语义不变。
    手工分成多次发布时各命令独立，前批成功不会因后批失败撤销；投影维护也不回滚已接受提交。
  风险与成本：
    目录全量读取、校验和远端写入，时间与内存随参与内容增长；非空基点仍逐 Address 对照。
    网络中断后客户端只轮询原回执，默认最多十分钟；同 id 异摘要为 IDEMPOTENCY_CONFLICT。
    服务端明确证明未应用时立即返回原错误，不轮询、不自动重发。
  失败恢复：
    先按原 command-id 查询回执；等待结束或 PENDING 不能解释为未提交，保留原请求供对账。
    并发冲突重新 diff 并选择新基点；内容变化后的新发布使用新 id，不能借旧 id 提交另一份目录。
    若自己拆批，先确认每批已完成的回执，再处理未完成批次，不能假定整批回滚。
  示例：
    例：kc writer commit --command-id commit-1 --repo kr://scene/object --dir drafts
    例：kc writer commit --command-id commit-bulk-1 --repo kr://scene/object --dir drafts --bulk
    例：kc writer receipt --command-id commit-1`,
		},
		"writer branch": {
			Description: "从当前发布版本开一条工作分支，写入不影响 main",
			Usage: `kc writer branch --repo <id> --name <branch> [--from <ref> | --commit <commit>]
  用途：
    为一组待审改动建立独立工作分支；之后用 writer commit --ref 或 writer put --ref 写到这条分支。
  用法与参数：
    --name 是新分支名，裸名自动补成 refs/heads/<name>，也可直接写完整 refs/...。
    --from 选择起点引用（默认 refs/heads/main），--commit 固定起点 commit，两者只能给一个。
  条件：
    已认证并有该仓新分支范围的 writer.branch；仓已登记、未归档且不是 system 仓；新分支尚不存在。
  结果：
    返回 repositoryId、ref 和起点 commitId。
  后果：
    只新建一个引用，不改 main、不发布对象、不触发索引或 Dataset 维护；分支上的后续写入同样不通知订阅者。
  风险与成本：
    一次引用创建；分支不会随 main 自动前进，审阅期间 main 推进后合并会被拒绝，需要在新基点上重新开分支。
  失败恢复：
    PRECONDITION_FAILED 表示分支已存在，换名或沿用该分支；VERSION_UNRESOLVED 表示起点不存在，核对 --from/--commit。
  示例：
    例：kc writer branch --repo kr://scene/object --name review/policy-a
    例：kc writer commit --command-id draft-1 --repo kr://scene/object --ref refs/heads/review/policy-a --dir drafts`,
		},
		"writer merge": {
			Description: "把审阅过的分支提交校验后快进发布到目标引用",
			Usage: `kc writer merge --command-id <id> --repo <id> --from <branch> [--commit <reviewed>] [--into <ref>] [--expected <commit>]
  用途：
    审批由业务完成后，把分支上审阅过的那个 commit 发布到目标引用（默认 refs/heads/main）。
  用法与参数：
    --from 是工作分支（裸名补 refs/heads/）；--commit 固定审阅时看到的分支 commit，--expected 固定审阅时的目标 commit。
    省略 --commit/--expected 时取当前值；--command-id 由你提供，同 id 重放返回原回执。
  条件：
    已认证并有目标引用范围的 writer.merge；分支是目标的后代（快进），分支改动全部是经 Writer 写入的对象单元。
    合并前按目标的 Schema 重新校验分支里每个改动对象；前置 writer.merge Hook 可否决，用于接入业务审批。
  结果：
    返回 Surface MERGE 的耐久回执，含 oldCommit/newCommit；重放时 disposition 为 REPLAYED。
  后果：
    只在目标仍等于 expected 时条件推进；推进 main 时才通知索引、Dataset 与维护。后置 Hook 投递失败不回滚合并。
  风险与成本：
    读取分支差异并逐对象校验，成本随改动规模增长；不做三方合并，不自动变基。
  失败恢复：
    CANDIDATE_MOVED：分支在审阅后又被写入，重新审阅新 commit；NON_FAST_FORWARD：目标已推进，从新 main 重开分支。
    SCHEMA_INSTANCE_INVALID 或 PRECONDITION_FAILED：分支里有不合规或绕过 Writer 的文件，在分支上修正后重新审阅。
    响应丢失时按原 command-id 查 writer receipt，不换 id 重发。
  示例：
    例：kc writer merge --command-id merge-1 --repo kr://scene/object --from review/policy-a
    例：kc writer merge --command-id merge-1 --repo kr://scene/object --from review/policy-a --commit c-reviewed --expected c-main`,
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
    不推进引用、不发布对象；这次查询也不为后续命令保留锁或冻结权限。
  风险与成本：
    一次 HEAD 查询；结果可能随后被其它发布推进，不能当成永久最新版本。
  失败恢复：
    核对仓/引用、当前授权与来源可用性；提交时将选定 commit 明确作为基点，冲突后重新核对。
  示例：
    例：kc writer head --repo kr://scene/object
    例：kc writer head --repo kr://scene/object --ref refs/heads/main`,
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
