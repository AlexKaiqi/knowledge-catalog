package cli

const helpFilesList = `kc files list --dataset <id> [--revision <n>] [--path <dir>] [--limit <n>] [--continuation <cursor>]

用途：
  逐页查看 Dataset 一个交付目录的直接子项，选择后续要读取的材料。
用法与参数：
  kc files list --dataset <id> [--revision <n>] [--path <dir>] [--limit <n>] [--continuation <cursor>]
  省略 path 从交付树根目录开始；使用相对交付路径，不能用原仓路径、对象 ID 或父目录跳转。
  首次省略 revision 取当前版一次；续页必须显式沿用返回的 revision、同一 path 与 continuation。
  --limit 默认 50、最多 200，0 使用默认。
条件：
  已认证并有该 Dataset 的 dataset.resolve 和 file.read；所选版已接受、当前未退役，成员与来源版本仍可用。
结果：
  JSON 含 dataset、实际 revision、pinId、repositories、path、entries、exhausted 和可能的 continuation。
  条目的 path 可传给 files read；空页带 continuation 时仍未结束，不能把空页当作目录已遍历完。
后果：
  只枚举直接子项，不递归物化目录，不修改文件、发布或权限。
风险与成本：
  每次先核对发布依据再取一页，成本随来源扇出与分页增长；limit 限制返回条数，不承诺来源端只检查同等数量。
  游标绑定 Dataset、发布版和目录；多条命令若各自省略 revision 可能读到不同服务版，旧版也检查当前消费权。
失败恢复：
  游标或 revision 不匹配时重新核对首个成功页，不自动改读新版；未发布版用 dataset show 查明。
  路径无效时从根目录逐层选择返回的 path；FORBIDDEN 时向授权管理者申请消费权，不能改用来源路径绕过。
示例：
  kc files list --dataset delivery --path reference --limit 20
  kc files list --dataset delivery --revision 2 --path reference --continuation '<上页游标>'`

const helpFilesRead = `kc files read --dataset <id> [--revision <n>] --path <file> [--offset <n>] [--length <n>]

用途：
  按仓和固定 commit，或按已发布 Dataset，读取文件字节块或下载完整单文件。
用法与参数：
  kc files read --repo <id> --commit <commit> --path <file> [--offset <n>] [--length <n>]
  kc files read --repo <id> --commit <commit> --path <file> --out <local-file>
  kc files read --dataset <id> [--revision <n>] --path <file> [--offset <n>] [--length <n>]
  kc files read --dataset <id> [--revision <n>] --path <file> --out <local-file>
  --repo 与 --dataset 互斥。repo 使用服务已连接的逻辑 Repository ID；按仓必须提供固定 commit，path 相对于原仓根目录。
  commit 仅用于按仓读取，不接受 HEAD 或分支名；revision 仅用于 Dataset，不默认读取分支最新版。
  按 Dataset 时 path 是相对交付文件路径，沿用 files list 返回值；revision 省略时独立选择当前版。
  对象 definition 中的文件地址是普通值，按其业务说明取得仓、资源自己的 commit 和路径；资源可在别仓，不能套用对象说明的版本。
  offset 是非负字节偏移，默认 0；length 默认 512 KiB、最多 4 MiB，0 使用默认。
  --out 完整下载，与 offset/length 互斥，不能指向已有文件或符号链接。
条件：
  已认证；按仓需该仓的 file.read，按 Dataset 需该 Dataset 的 dataset.resolve 和 file.read。
  仓和固定版本必须可用；按 Dataset 时所选发布、成员及来源版本当前仍可消费。object.read 不包含 file.read。
  下载时目标父目录可写，支持同目录临时文件和硬链接发布，并有足够磁盘空间。
结果：
  字节块 JSON 的 content 使用 base64，附 offset、totalBytes、eof 及来源 Repository/commit；Dataset 另有 revision、pinId，不假定 UTF-8。
  --out 成功返回完整文件的本机路径、bytes 和固定读取依据；一个字节块不代表整个文件。
后果：
  字节块模式不写本机文件；--out 在完整读取并核对后发布新目标，不覆盖已有目标。
  读取对象不会自动下载其正文提及的文件，文件读取也不解释图片类型、不发权。
风险与成本：
  每个块检查当前权限和固定依据；范围只限制返回字节数，不承诺来源端 Range IO，来源实现仍可能读取完整文件。
  下载的网络与磁盘成本随完整文件增长；先写同目录临时文件，失败时尝试清理临时文件且不留下成功目标。
  进程被强制终止或清理受阻可能留下临时文件；已经交付的字节不能因撤权远程收回。
失败恢复：
  目标存在时换新路径；磁盘或硬链接失败时核对目录能力，进程异常终止后可人工清理该目录中的本次 .kc-file-* 临时文件。
  权限撤销、源版本丢失或响应依据变化时停止使用失败下载，处理原因后按原 revision 重试；不要无提示改读当前版。
  按仓失败则核对资源坐标和 file.read，处理原因后按原 commit 重试。
示例：
  kc files read --repo kr://acme/assets --commit <资源的完整commit> --path media/logo.png --out ./logo.png
  kc files read --dataset delivery --revision 2 --path reference/index.md --length 4096
  kc files read --dataset delivery --revision 2 --path data/export.bin --out ./export.bin`
