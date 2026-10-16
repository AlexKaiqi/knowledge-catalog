package cli

const guideComposition = `从来源仓选择文件，发布为可消费的 Dataset。
Repository 拥有来源文件和 commit 历史；Dataset 保存固定来源的引用及交付路径，另有自己的 revision，不是新的可写仓。

开始前先取得来源身份、相对路径和引用，见 kc help repository / kc help versions。
服务已连接来源后 attach 登记；define 选择文件并发布；授权管理者另行发消费权；消费者用 files list/read 或 clone 取文件。
创建/连接、登记、发布与发权是独立步骤，各自的成功不推导下一步已完成。

完整发布与消费旅程：kc help compose。
只消费已有 Dataset：kc help files。
配方和属性：kc help recipe / kc help metadata。
各操作的输入、后果与恢复见下列叶帮助。`

const guideDataset = `Dataset 是命名、版本化的文件交付范围，保存来源 commit 的引用、文件选择、交付路径和可选元数据。
它可以只选一个仓，也可以组合多个仓；不是新的可写 Repository，修改组合要发布新 revision。
旧版不可覆盖，来源随后提交不改变它；再次发布才采用新来源版本或布局。

维护者：
  show 找已登记身份 → 确认原仓版本和路径 → define 发布 → dataset show 核对 → grant 显式授权。
  本机试配方用 overlay；停止全部版本的后续消费用 retire，保留发布记录。
消费者：
  show 找 Dataset → dataset show 核对 revision/来源/属性 → files list 选路径 → files read 读字节。
  clone 可把整个当前交付树复制到空目录，失败可留部分产物；需要历史版用 files list/read 明确 revision。

资源与版本关系：kc help snapshot / kc help versions。
完整旅程：kc help compose / kc help files；可复制输入：kc help recipe。
定义查看权、管理权和文件消费权分开；固定版仍受当前权限、来源保留和退役状态约束。`

const guideFiles = `按仓和固定 commit 下载原始文件，或消费 Dataset 的交付文件；不需要 Schema、检索索引或图片处理器。

对象 definition 中读到文件地址后，按其业务说明取得资源自己的仓和 commit，以及相对于原仓根目录的路径：
  kc files read --repo <id> --commit <commit> --path <file> --out <local-file>
仓身份使用服务中的逻辑 Repository ID；资源可以属于另一个仓，也可以采用独立于对象说明的版本。
若正文只有相对路径，仅在业务说明明确资源与该方面同仓同版时采用该方面 sources 的 repository/commit；否则先取得完整资源坐标。
这些字段是普通值，由 Agent 选择并调用文件命令；KC 不自动识别图片、下载或授予权限。
按仓读取需要该仓的 file.read；object.read 不包含 file.read，不需要先创建 Dataset。

按 Dataset 时，--path 是交付树中的相对路径，不是原仓路径或对象 ID；只读已选入的材料。

从已有 Dataset 开始（示例 delivery，示例发布版 1）：
  kc show
  kc dataset show delivery
  kc files list --dataset delivery --path '' --limit 20
  # 首页返回 revision；假设为 1，entries 给出可继续读取的 path。
  kc files list --dataset delivery --revision 1 --path reference --limit 20
  kc files read --dataset delivery --revision 1 --path reference/index.md --out ./index.md
把名称和路径换成上一步真实返回值，后续命令沿用首次得到的 revision。
目录只列直接子项；有 continuation 则带同一 revision/path 继续，直至 exhausted=true。
库存/定义查看需要 inventory.read；文件消费需要该 Dataset 的 dataset.resolve 和 file.read。

成功获得什么：
  list 的 entries.path 是交付路径，revision 是采用的发布版；下钻目录继续 list，文件交给 read。
  read 不带 --out 返回含 base64 content 的字节块；--out 完整下载单文件，目标必须不存在。
  要整个当前树用 kc dataset clone delivery ./delivery-copy，目标须不存在或为空；clone 不提供历史 revision 选择。

下载后的普通文件不会持续同步。单文件读取失败不发布成功目标；clone 中途失败可留部分树，重试前核对残留。
旧版也检查当前权限，已拿到的字节无法通过撤权追回。
详细分页、下载与恢复：kc help files list / kc help files read / kc help dataset clone。
需要来源、交付位置与固定版本的概念：kc help snapshot / kc help versions。`

const guideGrant = `授权是主体、动作和资源范围的显式规则，不会因登记、发布或持有版本坐标而自动获得。
Dataset 文件消费需 dataset.resolve 与 file.read；读取库存/定义另需 inventory.read。
新增、撤销或修改规则改变后续请求的判断，固定 Dataset 或旧 pin 仍检查当前权限。

本人已有权限与申请入口：kc help admission show。
管理者可 add 新规则，list 核对，remove 按 id 撤销；update 按完整旧观察原子替换旧规则。
add 重试可能重复发权，update 冲突先回读，不自动刷新观察后覆盖；其它规则也可能继续放行。
规则说明和当前权限不是同一份保证，已下载字节不能追回。
具体动作和恢复见下列叶帮助；完整发布与发权旅程见 kc help compose。`

const guideRegistry = `登记空间是这套服务的来源成员与 Dataset 已接受发布状态；每套服务只有一个。
create 供给或连接来源，attach 才登记；define 发布固定交付范围，retire 停止其后续消费。
登记不会把来源整仓交付，也不发消费权；detach 保留来源文件/历史，但会影响引用它的 Dataset 后续访问。

show 看当前库存；registry audit 看最近一段登记、发布和退役记录。
登记历史不是来源正文提交历史，当前 audit 不提供历史续页；特定 Dataset 的版本回读用 dataset show。
下一步：kc help composition / kc help dataset / kc help registry audit。`
