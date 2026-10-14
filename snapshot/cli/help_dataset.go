package cli

const helpDatasetDefine = `kc dataset define <id> --revision <n> (--source <repo>[=<selector>][@path[@subPath]]... | --file <recipe.yaml>)

用途：
  从单个或多个 Repository 选择文件或子目录，以固定版本发布一份命名 Dataset。
用法与参数：
  kc dataset define <id> --revision <n> (--source <repo>[=<selector>][@path[@subPath]]... | --file <recipe.yaml>)
  <id> 也可写作 --dataset；--revision 为正整数、必须递增，已接受的同版发布不可覆盖。
  --source 可重复，selector 省略或等号后为空时使用默认引用 refs/heads/main；省略 @ 不声明目录交付，@ 后为空表示根映射。
  selector 选择来源引用，发布时由连接的 lakeFS 解析：可选 HEAD、refs/heads/main（均指连接仓的默认分支）和 refs/heads/<分支名>。
  裸 main、published、refs/tags/<tag> 和 commit ID 无法由当前 adapter 在发布时解析；本机输入解析可以保留这些非空字符串。
  第一个 @ 后是交付目录 path，第二个 @ 后是来源子目录 subPath。
  任一目录来源声明 @path 时，其余目录来源也须声明；非根交付目录不得嵌套重叠，根映射作为其它映射之外的回退。
  --file-source <repo>[=<selector>]@<来源文件>@<交付路径> 可重复并与 --source 同用。
  --file 读取普通 YAML 配方：name 是便携名称，mounts 选目录，files 选单文件，metadata 是可选字符串属性。
  每条 YAML 来源须显式填写 repository/selector；mount 的 path 是交付目录（空字符串为根），subPath 是来源子目录。
  file 的 file 是来源仓相对文件路径，target 是交付相对文件路径；发布前编译文件清单时检查它们不能逃出所属树根，不要填知识对象 id。
  可选 baseRev 写管理者核对的完整 commit ID，发布时才检查 selector 当前 tip 与它相等；不是直接选旧 commit 的字段。
  同一仓的条目须共享 selector/baseRev；目录片段不能重叠。YAML 不提供 commit 输入，发布后才返回冻结 commit。
  发布 id 仍由 <id>/--dataset 指定；使用 --file 时不能再同用 --file-source，本机输入文件不会自动改写。
  --metadata '<JSON object>' 可省略，键和值均为字符串；显式参数替换输入配方的整份元数据，{} 清空。
  省略 --metadata 时采用输入配方的属性；直接指定来源时不会自动继承旧发布的元数据。
  修改属性须在新配方/--metadata 中给出本次整份 map 并发布新版；metadata 不改变文件选择，不从文件内容推导属性。
条件：
  已认证并有该 Dataset 的 dataset.manage；每个来源还需独立 repository.dataset.publish，范围覆盖来源仓、所选 ref 和目标 Dataset。
  普通仓读取或 dataset.* 不授予来源发布许可；来源已登记、selector 可解析且源版本可用。
  同一 Repository 的片段共享一致版本，选定文件必须存在，目标文件及文件/目录不能冲突；已退役 Dataset 不能再发布。
  文件组合不要求知识索引；宿主若另外承诺知识查询，可在接受前要求其准备成功。
结果：
  成功 JSON 返回 setId、revision、已冻结 sources 和可选 metadata；出现 recipeFile/recipeRepository/recipeCommit/recipeLocation/recipeSkipped 时核对实际配方去向。
后果：
  先检查候选范围，再接受不可变发布记录并推进当前服务版；之后来源提交不改变这一版。
  服务可在发布前独立保存便携配方：写入可写根挂载仓，或保存到服务端本地；缺权限或能力可跳过写回。
  发布不自动发权，不把 Dataset 变成可写仓。
风险与成本：
  多仓版本冻结、交付冲突检查和宿主能力准备会产生读取与等待成本；跨仓版本向量不是全局事务。
  配方写回可能新增来源提交或改写本地配方文件；后续准备、权限复查或最终接受失败不回滚已完成的配方写回。
  本机配方解析或 overlay 成功不是完整发布校验；引用支持与逐文件路径还须通过服务端来源解析和文件清单编译。
  响应丢失不能当作未发布；不要盲目换 revision 重提，也不能从定义命令推断配方一定写回。
失败恢复：
  NON_FAST_FORWARD 或响应丢失时，先 kc dataset show <id> --revision <n> 核对已接受结果；只有确认需要新发布后才选择更大 revision。
  来源、布局或准备失败时修复对应原因，再核对当前服务版与可能已写回的配方；已接受旧版继续保留。
  原始版本和路径先请来源管理者在原 lakeFS 仓的分支、提交历史与文件浏览器核对；本 CLI 不提供来源历史或原始树浏览。
  要采用某个旧 commit，请管理者保留指向它的专用分支，再选 refs/heads/<分支名> 并用 baseRev 核对当前 tip。
  dataset show 只核对已接受交付，files list 只浏览交付树；Writer 是知识发布入口，不是普通文件版本浏览器。
  无权时分别申请 Dataset 管理与来源发布许可；只有目标 Dataset 的授权管理者不能自授来源权。来源连接问题由服务管理者处理，不能以 metadata 或另起 Dataset 名称绕过授权。
示例：
  kc dataset define scene-set --revision 1 --source kr://scene/knowledge@
  kc dataset define delivery --revision 2 --source docs@reference@handbook --file-source docs@handbook/start.md@reference/index.md --metadata '{"description":"项目材料"}'
  以下完整文件示例在专用新目录执行，recipe.yaml 应尚不存在；cat 会覆盖同名文件。
  docs/archive/metrics 是已登记来源示例，请替换为你的仓与已核对路径；所有版本选择均为默认分支。
` + helpDatasetRecipeExample + `kc dataset define delivery --revision 1 --file recipe.yaml
  配方 name 不填写 revision；本次发布版由命令指定。此 define 会联系 Server 并发布，先按 overlay 示例离线核对也可以。`

const helpDatasetRecipeExample = `cat > recipe.yaml <<'YAML'
name: delivery
metadata:
  description: "项目材料"
  audience: "分析团队"
mounts:
  - repository: docs
    selector: refs/heads/main
    path: reference
    subPath: handbook
    # 可选：baseRev: "来源管理者确认的完整 commit ID"
  - repository: archive
    selector: refs/heads/main
    path: legacy
    subPath: handouts
files:
  - repository: metrics
    selector: refs/heads/main
    file: exports/kpi.csv
    target: data/kpi.csv
YAML
`

const helpDatasetShow = `kc dataset show <id> [--revision <n>] [--limit <n>] [--continuation <cursor>]

用途：
  核对 Dataset 已接受发布的定义、固定来源、元数据与一页版本历史。
用法与参数：
  kc dataset show <id> [--revision <n>] [--limit <n>] [--continuation <cursor>]
  <id> 也可写作 --dataset；省略 revision 选择当前服务版，显式值必须是已接受的正整数发布版。
  --limit 只控制历史名单，默认 50、最多 200，0 使用默认；版本可能跳号。
  --continuation 采用上页返回值，保持同一 Dataset 与查询选择。
条件：
  已认证并有 inventory.read；Dataset 在该服务的已接受登记中可见。
结果：
  JSON 含所选 revision、当前 servingRevision、sources/items、可选 metadata/retired，以及 versions 和可能的 continuation。
  历史版使用当时的元数据和布局，退役状态按当前政策显示；详情可见不证明文件可读。
后果：
  只回读已接受定义；退役后仍可查看发布记录，不因此恢复消费或发权。
风险与成本：
  一次读取定义和有界版本名单，不扫描来源正文；单页不是全部历史。
  新发布会使旧历史游标失效；管理详情中的坐标不能自行充当消费授权。
失败恢复：
  VERSION_UNRESOLVED 时核对 versions 或取消不存在的 revision；Dataset 不可见时先 kc show 核对身份。
  游标失效时从不带 continuation 的历史查询重新开始；无权时申请库存查看权，读取文件还需另行消费授权。
示例：
  kc dataset show delivery --revision 2 --limit 10
  kc dataset show delivery --revision 2 --limit 10 --continuation '<上页游标>'`

const helpDatasetClone = `kc dataset clone <dataset> <dir>

用途：
  把 Dataset 当前服务版的交付文件物化到一个普通本地目录，供现有文件工具使用。
用法与参数：
  kc dataset clone <dataset> <dir>
  Dataset 与目标目录必须明确；本入口采用当前版，不提供选择历史版的 revision 或 pin 参数。
条件：
  已认证，持有该 Dataset 的 dataset.resolve 和 file.read；来源版本仍保留，Dataset 未退役且成员未移除。
  <dir> 必须不存在或为空，父路径可写，本机有足够磁盘容量。
结果：
  成功 JSON 含 dataset、revision、pinId、dir、files、bytes、mounts 与 fileEntries，目录来源列出 Repository 与 commit。
  不写 manifest 文件；本机保留原始文件字节，包括零字节文件，空目录不独立物化。
后果：
  解析一次当前发布，逐目录分页、逐文件分块写入目标；不挂载、不持续同步，也不改来源或共享发布。
风险与成本：
  读取整棵交付树，网络请求、传输量与本机占用随目录和文件规模增长；超过可支持的目录深度会停止。
  开始时拒绝非空目标，但不提供整树原子完成或回滚；中途失败可留下部分目录、完整文件或部分文件。
  运行时不要并发修改目标；已下载字节无法由撤权追回，后续服务访问仍按当前权限检查。
失败恢复：
  目标非空时换专用空目录；网络、权限或来源失败后先检查残留，再自行清理本次专用目标或换新空目录重试。
  每次重试重新选择当时的当前版；需要精确历史材料时用 files list/read 显式沿用 revision，不能把两次 clone 当作同一版。
  需要独立空目录时自行建立，不把文件已复制的回执当作全部目录占位已创建。
示例：
  kc dataset clone delivery ./delivery-copy
  kc files list --dataset delivery --revision 2 --path reference`

const helpDatasetRetire = `kc dataset retire <id>

用途：
  停止消费一个 Dataset，保留它的已接受发布记录用于核对。
用法与参数：
  kc dataset retire <id>
  <id> 也可写作 --dataset，取自 kc show 或 kc dataset show。
条件：
  已认证并有该 Dataset 的 dataset.manage；目标已存在。
结果：
  JSON 返回 dataset 和 retired:true；用 kc dataset show 核对当前退役状态。
后果：
  停止消费，旧 pin 与历史版也受当前退役约束；保留发布记录、来源文件和来源提交历史。
风险与成本：
  这是持久生命周期变更；当前入口没有取消退役，同名 Dataset 也不能通过重新 define 复活。
  影响该 Dataset 的后续消费，不能删除消费者已拿到的副本；保存状态可能遇到并发或持久化失败。
失败恢复：
  响应丢失或冲突时先 dataset show 查明是否已退役，不能把旧文件仍在本机视为退役失败。
  误退役时经管理方确认后另建 Dataset 重新发布并显式授权；本命令不删除来源以代替停止交付。
示例：
  kc dataset retire delivery
  kc dataset show delivery`

const helpDatasetOverlay = `kc dataset overlay --file <recipe.yaml> --overlay <overlay.yaml> [--out <path>]

用途：
  在本机合成一份临时配方，调整文件选择或交付位置后供人工核对或另行发布。
用法与参数：
  kc dataset overlay --file <recipe.yaml> --overlay <overlay.yaml> [--out <path>]
  --file 是底稿配方；--overlay 是 YAML 补丁：remove 为仓 id 数组，删除该仓全部目录和文件条目。
  mounts 按交付 path 替换目录条目或新增目录；files 按交付 target 替换单文件条目或新增文件，不改写文件正文。
  有改动时可选 name 须匹配底稿；每条 mounts/files 必须填 repository/selector，YAML 不会为省略 selector 补默认值。
  selector 是来源引用，发布时由连接的 lakeFS 解析：支持 HEAD、refs/heads/main（默认分支）与 refs/heads/<分支名>，不支持裸分支名、tag 或 commit ID。
  mount 的 path 是交付目录、subPath 是来源子目录；file 的 file 是来源文件、target 是交付文件，发布前编译文件清单时检查后两者不能逃出树根。
  baseRev 在发布时只核对该引用当前 tip，不选任意旧 commit；同一仓须保持相同 selector/baseRev，不能在本地合成时冻结远端版本。
  离线解析检查来源字段非空；有改动的补丁再检查同仓坐标一致和目录布局，空补丁只返回底稿、不补做组合校验。
  不解析来源引用，也未完成单文件 file/target 的清单编译检查。
  补丁不改 metadata，合成结果沿用底稿；要更新属性，编辑整份新配方 metadata 后另发更大 revision。
条件：
  两份本机文件可读并通过解析与上述组合检查；指定 --out 时目标父目录存在且可写。无需登录或连接 Server。
结果：
  不带 --out 时 stdout 返回合成定义的 JSON；带 --out 时写出便携配方文件并返回 setId/out 回执。
  合成定义中的 revision=1 是本机候选，不能解释成已接受发布版。
  selector: main 或 target: ../outside.csv 可留在离线候选中；前者在来源解析时被拒绝，后者在发布清单编译时被拒绝，不能成为合法交付。
后果：
  不联系 Server、不发布 Dataset、不改变登记状态；只有显式 --out 会写本机目标。
风险与成本：
  读取并解析两份文件，成本随配方条目增长；本机成功不证明输入全合法、来源存在、授权满足或服务端发布能接受。
  --out 会覆盖已有文件并跟随目标符号链接，写入不提供原子替换保证；若与输入同路径可覆盖底稿。
失败恢复：
  名称、删除目标、同仓坐标或目录布局冲突时修改补丁后重试；发布另会检查逐文件路径和交付冲突，勿将离线成功当作已通过。
  写入失败先检查目标内容，必要时从备份恢复。
  selector、旧版本和来源路径请来源管理者在原 lakeFS 仓核对；本 CLI 不提供原始树/来源历史浏览，旧版本需先保留可用专用分支。
  优先输出到新文件并核对；需要共享时另用 dataset define 显式选择新 revision，合成不授予发布权。
示例：
  以下完整离线示例在专用新目录执行，recipe.yaml、overlay.yaml、merged.yaml 都应尚不存在，避免覆盖已有材料。
  此例用假定来源名和路径即可本机合成，不联系来源；发布前再由管理者核对这些来源、引用与路径。
` + helpDatasetRecipeExample + helpDatasetOverlayExample + `kc dataset overlay --file recipe.yaml --overlay overlay.yaml
kc dataset overlay --file recipe.yaml --overlay overlay.yaml --out merged.yaml
  结果保留底稿 metadata，移除 archive，reference 改交付 docs/guides，media 交付 assets/images，data/kpi.csv 改来自 metrics/exports/latest.csv。
  单文件 notes/release.txt 来自 docs/notes/release.txt；只选择上述目录/文件，不补入邻近目录。
  merged.yaml 是未发布候选；核对后才可另行 kc dataset define delivery --revision 2 --file merged.yaml。`

const helpDatasetOverlayExample = `cat > overlay.yaml <<'YAML'
name: delivery
remove:
  - archive
mounts:
  - repository: docs
    selector: refs/heads/main
    path: reference
    subPath: guides
  - repository: assets
    selector: refs/heads/main
    path: media
    subPath: images
files:
  - repository: metrics
    selector: refs/heads/main
    file: exports/latest.csv
    target: data/kpi.csv
  - repository: docs
    selector: refs/heads/main
    file: notes/release.txt
    target: notes/release.txt
YAML
`
