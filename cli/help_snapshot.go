package cli

// Snapshot help shares the existing CLI surface, identity context and renderer.
func snapshotHelpEntries() map[string]helpEntry {
	return map[string]helpEntry{
		"show": {
			Description: "显示服务的 Repository 与 Dataset 库存",
			Usage: `kc show
  库存可见不表示来源文件可读、Dataset 可消费或资源可管理。`,
		},
		"create": {
			Description: "创建托管 Repository 或连接自有 Repository",
			Usage: `kc create (--name <名称> [--store <store>] | --url <url> --credential-file <path>)
  --name 与 --url 互斥。--name 是平台供给空仓；--url 是连接已有仓。
  创建或连接不自动登记到服务库存；登记使用 kc attach。
  LakeFS 上 --name 就是仓 ID，与 lakeFS 仓库名相同。
  例：kc create --name table-meta`,
		},
		"attach": {
			Description: "把已连接 Repository 挂入服务登记空间",
			Usage: `kc attach --repo <id>
  验证并登记已有来源，不创建仓，也不隐含发权。`,
		},
		"detach": {
			Description: "从服务登记空间移除 Repository 登记",
			Usage: `kc detach --repo <id>
  保留来源仓的文件和提交历史。`,
		},
		"grant add": {
			Description: "发放一条服务、仓或 Dataset 范围的授权规则",
			Usage: `kc grant add --principal <id> --action <action[,action...]> [--repo <id>] [--dataset <id>]
  --action 是 knowledge.read 这种带点的名字，逗号分隔。不能发 '*'（那是 deployment init 的 bootstrap）。
  Dataset 消费权用 --dataset，解析与读取需要 dataset.resolve,file.read。
  值里有空格或 * 必须加单引号，否则 shell 会把 * 展开成当前目录的文件名。
  重复发放会新增一条规则，不是幂等。撤销抄本条回执或 grant list 的 id。
  例：kc grant add --repo kr://scene/knowledge --principal healzhou --action knowledge.read,knowledge.search
  例：kc grant add --dataset scene-set --principal healzhou --action dataset.resolve,file.read`,
		},
		"grant list": {
			Description: "列出授权规则",
			Usage: `kc grant list [--principal <id>] [--action <action>] [--repo <id>] [--dataset <id>]
  不带 action 时列出显式规则，可按 --principal、--repo 或 --dataset 过滤，不是整份 allow 文件。
  带 action 时是显式规则求值，省略 principal 则问当前主体；需要全局 admin.grants.read，不包含默认消费声明。
  例：kc grant list --repo kr://scene/knowledge
  例：kc grant list --repo kr://scene/knowledge --principal healzhou --action knowledge.read`,
		},
		"grant remove": {
			Description: "撤销一条授权规则",
			Usage: `kc grant remove --id <rule-id>
  --id 来自 grant add 回执或 grant list，不是场景执行器占位符。`,
		},
		"registry audit": {
			Description: "查看服务登记历史",
			Usage:       `kc registry audit [--limit <n>] [--continuation <cursor>]`,
		},
		"dataset define": {
			Description: "发布 Dataset 的固定版本文件交付范围（目录映射与逐文件交付）",
			Usage: `kc dataset define <id> --revision <n> (--source <repo>[=<selector>][@path[@subPath]]... | --file <recipe.yaml>)
  Dataset 可以选择单个或多个 Repository；接受时固定来源版本，源仓后续提交不改变这一版。
  --revision 必须递增；已接受的同版发布不可覆盖。
  --source 可重复；等号后面是 selector，省略则用默认 published。<id> 也可用 --dataset 写出。
  --metadata '<JSON object>' 可省略；键和值均为字符串，随发布版本保存。显式参数替换输入配方的整份元数据，{} 清空。
  目录映射：第一个 @ 后写交付目录（path），第二个 @ 后写来源仓内的子目录（subPath）。
  一旦任一目录来源声明 @path，所有目录来源都须声明（根用 @ 写空串）。
  非根交付目录不得嵌套重叠；根映射是其他映射之外文件的回退。
  逐文件交付：--file-source <repo>[=<selector>]@<来源文件>@<交付路径>，可与 --source 同用；
  同一交付目标文件只能有一个来源，冲突会被拒绝；同一仓的目录与文件共用一个固定 commit。
  服务可在发布前另行保存便携配方，可能写入可写根挂载仓或服务端本地；缺权限或能力时跳过写回。
  核对回执 recipeLocation / recipeCommit / recipeSkipped，不能从定义命令推断配方已写回来源仓。
  例：kc dataset define scene-set --revision 1 --source kr://scene/knowledge@
  例：kc dataset define --dataset scene-set --revision 1 --source kr://scene/knowledge@
  例：kc dataset define delivery --revision 1 --source docs@reference/policies@handbook/policies --source data@data/metrics@semantic/metrics --file-source docs@handbook/README.md@reference/README.md
  例：kc dataset define scene-set --revision 1 --file .kc-dataset.yaml`,
		},
		"dataset clone": {
			Description: "把已发布 Dataset 的交付目录树物化成普通本地目录",
			Usage: `kc dataset clone <dataset> <dir>
  把已发布 Dataset 的当前服务版交付目录树物化成普通本地目录，不需要 kcfs。
  <dir> 必须是空目录或不存在；clone 不覆盖已有文件。
  回执列出版本、pinId 与每个来源仓的 commit；不写 manifest 文件，也不收 --pin。
  例：kc dataset clone scene-set ./scene-copy`,
		},
		"dataset retire": {
			Description: "退役 Dataset，停止消费并保留发布记录",
			Usage: `kc dataset retire <id>
  <id> 也可用 --dataset 写出。停止后续消费，保留已接受发布记录，不删除来源文件。
  例：kc dataset retire scene-set`,
		},
		"dataset overlay": {
			Description: "只在本机合成一份不发表的配方",
			Usage: `kc dataset overlay --file <recipe.yaml> --overlay <overlay.yaml> [--out <path>]
  --file 读取底稿配方，--overlay 读取本机补丁；不联系 Server，也不发布 Dataset。
  未指定 --out 时，stdout 返回合成定义的 JSON；指定 --out 时将便携配方文件写到该路径并返回回执。
  例：kc dataset overlay --file recipe.yaml --overlay overlay.yaml
  例：kc dataset overlay --file recipe.yaml --overlay overlay.yaml --out merged.yaml`,
		},
	}
}
