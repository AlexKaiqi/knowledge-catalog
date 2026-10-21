package cli

const helpShow = `kc show

用途：
  发现这套服务已登记的 Repository 和已发布 Dataset，取得后续查看或消费的身份。
用法与参数：
  kc show
  使用 KC 已保存的服务与身份；可由宿主提供 --server 和本次认证上下文。
条件：
  已连接可用服务、通过认证，并持有 inventory.read。
结果：
  JSON 库存含 repositories 与 datasets；Dataset 摘要含身份、发布版、成员及可选元数据/退役状态。
  Repository 可附带宿主提供的 schemaCount 等描述；库存可见不表示来源文件可读、Dataset 可消费或资源可管理。
  当前 kc show 不读取 Repository 自定义元数据表；它是 lakeFS 原仓维护的可选字符串键值属性，不随 commit 冻结。
  查看或编辑原仓属性：打开本服务 /ui/ → Repositories → 目标仓 → Settings → General 的“Repository 自定义元数据”。
  读取需 repository.metadata.read，新增/更新键或删除键需 repository.metadata.write；进入页面仍受原仓浏览权限检查。
 Dataset metadata 由 dataset define 的 --metadata 或输入配方随 revision 完整发布；dataset show --revision 回读历史属性。
后果：
  读取当前登记状态，不登记来源、不发表新版本、不授予权限。
  Repo 属性不复制到登记账，也不自动继承到 Dataset；当前原仓属性变化不会替换 Dataset 已接受版本的属性。
风险与成本：
  一次库存请求返回当前名单，结果规模随登记成员与 Dataset 数量增长；宿主可能另行读取来源描述。
  名单反映本次读取时的状态，不证明来源在线、索引就绪或后续请求仍获授权。
失败恢复：
  UNAUTHENTICATED 时检查该服务的登录；FORBIDDEN 时用 kc admission show 查看本人授权与管理者。
  已知 Dataset 身份后用 kc dataset show 核对定义；服务或来源异常交由相应管理者处理，空名单不触发自动创建。
  原仓元数据页面无法进入或保存时联系来源管理者核对浏览及元数据权限；保存可能分多个原生操作，失败先重新读取当前属性。
示例：
  kc show
  kc dataset show delivery
  kc dataset show delivery --revision 2`

const helpAttach = `kc attach --repo <id>

用途：
  把服务已能连接的 Repository 登记为 Dataset 可选来源。
用法与参数：
  kc attach --repo <id>
  --repo 使用已有 Repository 的身份，不是本机目录或连接 URL。
  此入口不接受 --metadata；要维护 Repository 属性，按 kc help show 指引进入本服务的原仓管理页。
条件：
  已认证并有该登记范围的 repositories.manage；服务已有对应来源连接或托管仓绑定，来源可打开且未归档。
结果：
  JSON 回执返回 repositoryId；用 kc show 核对它已进入库存。
后果：
  验证并持久登记已有来源，不创建仓、不改来源文件，也不隐含发权；同一来源不会重复成为两个成员。
  不读取、复制或编辑原仓自定义元数据；Dataset 的可选属性另由 dataset define 随新 revision 发布，不自动继承来源属性。
风险与成本：
  需要核对来源绑定及保存登记状态；来源不可用或登记权威并发变化时可能失败。
  登记不把整仓加入任何 Dataset，消费者仍需单独发布范围与获得消费权。
失败恢复：
  来源未知或不可用时先核对身份与服务连接；需要供给或连接时另用 kc create，随后再 attach。
  响应丢失或版本冲突时先 kc show 核对，再按同一 --repo 重试；不要据此重复建仓或发权。
示例：
  kc attach --repo docs
  kc show`

const helpDetach = `kc detach --repo <id>

用途：
  停止把一个 Repository 作为这套服务的已登记来源。
用法与参数：
  kc detach --repo <id>
  --repo 使用 kc show 中的 Repository 身份。
条件：
  已认证并有该范围的 repositories.manage；来源当前已登记，保留的 System Repository 不能移除。
结果：
  JSON 回执含 repositoryId 和 detached；用 kc show 复核成员已移除。
后果：
  只移除登记，保留来源文件、提交历史与 Dataset 已接受发布记录。
  引用该来源的 Dataset 后续解析或读取会受当前成员准入限制，旧版本坐标不能绕过移除。
风险与成本：
  影响所有引用该成员的消费任务；已经下载的字节不能远程收回。
  保存登记状态可能遇到并发冲突；此动作不归档或物理删除源仓。
失败恢复：
  未登记时先 kc show 核对身份；响应丢失时先查库存，避免把已完成的移除当成新失败。
  误移除可在确认来源绑定仍可用后 kc attach 同一身份；消费还需当前授权和保留的来源版本。
示例：
  kc detach --repo docs
  kc show`

const helpRegistryAudit = `kc registry audit [--limit <n>]

用途：
  核对来源登记、Dataset 发布与退役在登记权威中的变更记录。
用法与参数：
  kc registry audit [--limit <n>]
  --limit 默认 50、最多 200，0 使用默认；负数或超限值被拒绝。
条件：
  已认证、服务提供登记历史入口，并持有 registry.audit.read。
结果：
  JSON 返回 source、authorityId 和最新一段 entries，条目含提交身份、消息及可用的审计信息。
  当前入口不返回续页游标；非空 --continuation 明确拒绝，本段不是完整历史。
后果：
  只查看登记历史，不回退发布、不重放写入，也不读取来源仓正文历史。
风险与成本：
  回读最多指定数量的登记提交，成本随历史回读规模增长；空 entries 不足以证明从未发生变更。
  关键发布应同时用 dataset show 核对已接受版本，历史读取异常需由管理者检查登记权威。
失败恢复：
  无权时向授权管理者申请 registry.audit.read；服务不可用时先恢复服务或登记历史读取能力。
  需要具体 Dataset 的已接受版本时用 kc dataset show 的历史分页，不用增大此 limit 假装无限续页。
示例：
  kc registry audit --limit 20
  kc dataset show delivery --limit 20`
