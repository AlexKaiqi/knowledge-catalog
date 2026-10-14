package cli

const helpGrantAdd = `kc grant add --principal <id> --action <action[,action...]> [--repo <id>] [--dataset <id>]

用途：
  为指定主体新增一条服务、Repository 或 Dataset 范围的显式授权。
用法与参数：
  kc grant add --principal <id> --action <action[,action...]> [--repo <id>] [--dataset <id>]
  可附加 --ref、--object、--aspect 限定；Repository 与 Dataset 同时指定时是交集，均省略为全服务范围。
  action 使用 object.read 等带点语义名，逗号分隔；不能发 '*'，通配值须加单引号避免 shell 展开。
 Dataset 文件消费需 dataset.resolve,file.read；object 是规则限定，不能当作 Dataset 文件路径 ACL。
  repository.dataset.publish 是来源发布许可：指定来源 --repo，可用 --dataset 限制交付目标、--ref 限制所选引用或显式 commit；Object/Aspect 或仅 Dataset 范围不能开放来源。
条件：
  已认证并有请求范围的 admin.grants.manage；管理者只能在当前获准范围内发权，原生存储管理员身份不自动获得此权。
  发放或维护来源发布许可还需来源仓或全局的授权管理权；只有目标 Dataset 的管理权不能自授私有来源。
结果：
  JSON 返回新规则的 id、principal、actions 与范围；保存 id，随后 grant list 可核对规则。
后果：
  持久新增规则，后续服务请求按当前政策求值；不创建来源、发布 Dataset 或替目标主体登录。
风险与成本：
  每次调用都会生成新规则，重复提交不是幂等；省略资源范围会扩大到全服务，确认动作和目标后再提交。
  更新规则账需要持久化，成本随授权规则数量增长；新增规则不证明外部源访问或对象读取也已获权。
失败恢复：
  无权时申请该范围的授权管理能力；用法错误时核对主体、动作和资源范围。
  响应丢失先由有 grant read 权的管理者按主体/范围核对 grant list，不能盲目重发；误发时按回执 id 执行 grant remove。
示例：
  kc grant add --repo docs --principal healzhou --action object.read,object.search
  kc grant add --dataset delivery --principal healzhou --action dataset.resolve,file.read
  kc grant add --repo docs --dataset delivery --ref refs/heads/main --principal publisher --action repository.dataset.publish`

const helpGrantList = `kc grant list [--principal <id>] [--action <action>] [--repo <id>] [--dataset <id>]

用途：
  查看显式授权规则，或询问一项动作是否命中显式规则。
用法与参数：
  kc grant list [--principal <id>] [--action <action>] [--repo <id>] [--dataset <id>]
  还可按 --ref、--object、--aspect 筛选；不带 action 时按给定坐标过滤规则。
  带 action 时求值一项动作；省略 principal 使用宿主提供的当前主体。
  询问 repository.dataset.publish 时同时给出来源 --repo、目标 --dataset 和所选 --ref；固定 commit 以该 commit 作为 ref。
条件：
  已认证；名单查看需对应范围的 admin.grants.read，带 action 的显式问题需要全局 admin.grants.read。
结果：
  名单模式返回 rules 中的单条规则；问题模式命中时返回 allow:true 与 ruleId，不命中时返回 FORBIDDEN。
后果：
  只读取或求值显式规则，不发权、不撤权、不执行被询问的动作。
风险与成本：
  服务读取规则账并筛选，成本随规则数量增长；列表不是私有部署账，也不是完整有效权限集合。
  不包含部署默认消费声明；显式求值成功也不能替代实际操作时的来源、范围和当前授权检查。
失败恢复：
  FORBIDDEN 时区分无法读取规则与问题未命中；普通主体可先 kc admission show 查本人授权与管理者。
  需要确认能否读取材料时使用实际 files read；不要把名单为空或问题拒绝解释成资源不存在。
示例：
  kc grant list --dataset delivery
  kc grant list --repo docs --principal healzhou --action object.read`

const helpGrantRemove = `kc grant remove --id <rule-id>

用途：
  撤销一条已存授权规则，停止它对后续服务请求的放行。
用法与参数：
  kc grant remove --id <rule-id>
  id 来自 grant add 回执或 grant list 的单条规则，不是主体、资源名称或占位符。
条件：
  已认证并有该已存规则范围的 admin.grants.manage；规则 id 必须唯一。
结果：
  JSON 返回 revoked 的规则 id；用 grant list 复核该规则已移除。
后果：
  持久删除这一条显式规则，不删除 Repository、Dataset 或文件；撤权约束后续服务访问。
风险与成本：
  同一主体仍可能经其它规则或部署默认政策获权，不能从删一条规则推出有效权限全部消失。
  已交付字节不能追回；持久策略更新的成本随规则数量增长，误删管理规则可能使自己失去后续管理权。
失败恢复：
  未知 id 或响应丢失时先由有读取权的管理者核对 grant list；重复删除不会被当成新撤销成功。
  id 重复歧义需管理方显式修复授权账；误撤销时由仍有管理权的主体重新 grant add，得到新的规则 id。
示例：
  kc grant remove --id alw_123
  kc grant list --dataset delivery`

const helpGrantUpdate = `kc grant update --id <rule-id> --file <replacement-rule.json> --expected-file <observed-rule.json>

用途：
  按此前观察的旧规则原子修改授权，保持规则身份和来源，避免覆盖其它编辑者的修改。
用法与参数：
  kc grant update --id <rule-id> --file <replacement-rule.json> --expected-file <observed-rule.json>
  两份文件各是 grant list 中的单条规则，不是整份 {rules:[...]} 库存；expected-file 保存完整旧规则。
  replacement 需 principal 和 actions 数组；repo/dataset/ref/object/aspect 表达完整新范围，省略的限定会清空。
  文件使用 repo 键；id、sharedBy、shareId 若写出须与旧值一致，不能伪改；未知键、重复键、null 或多份 JSON 被拒绝。
条件：
  已认证，对旧规则和新范围均有当前 admin.grants.manage；id 唯一，旧规则与此前观察一致。
  带 shareId 的分享规则由分享管理维护，本入口不能修改。
结果：
  JSON 返回完整替换后的规则，保留原 id 和来源；用 grant list 回读，再核对受影响主体的消费。
后果：
  在同一原子边界检查旧、新范围并保存完整替换；不是先撤销再新增，拒绝或持久化失败保留旧规则。
风险与成本：
  省略范围限定可扩大授权，改变动作也可能收紧消费或撤销自己的管理权；提交前核对整个替换对象。
  客户端读取两份本机文件，服务读取并更新规则账；expected-file 不会自动刷新，过期观察会发生冲突。
失败恢复：
  PRECONDITION_FAILED 时重新 grant list 读取现状，人工确认变更后保存新的完整观察，再决定是否提交；不能自动刷新后盲目覆盖。
  FORBIDDEN 时核对旧、新两个范围的管理权；响应丢失先回读规则，确认实际结果后再重试。
  分享来源或歧义 id 的拒绝交由相应管理方处理，不改 id 或删除来源字段绕过检查。
示例：
  kc grant list --dataset delivery
  kc grant update --id alw_123 --file replacement.json --expected-file observed.json`
