package cli

const guideSnapshot = `Snapshot 提供确定版本的文件保存、组合和交付能力；本服务使用 lakeFS 保存来源版本。

先分清三个资源/版本关系：
  Repository 是来源仓，拥有自己的文件和提交历史。一个 commit 标识该仓的一份固定文件版本。
  ref 是指向 commit 的引用，可随来源提交移动；选择分支是取得版本的办法，分支名本身不是固定版本。
 Dataset 是独立命名、版本化的交付范围，保存来源 commit 的引用、所选文件和交付路径，不是新的可写 Repository。
 Dataset revision 是这一份组合的发布版号；多个来源各有自己的 commit，revision 不等于其中任一仓的 commit。

例如：docs 仓某个 commit 的 handbook/ → delivery 第 1 版的 reference/。
来源仓后来提交，不改变 delivery 第 1 版；要采用更新，维护者显式发布更大的 revision。
Dataset 不复制来源整仓、不拼接文件字节，也不自动发权。固定内容仍受当前授权、成员登记、来源版本保留与退役状态约束。
发布需要目标的 dataset.manage 和每个来源的 repository.dataset.publish；普通读取与 Dataset 管理不能代替来源分享许可。许可可限制目标 Dataset 与来源 ref。SDK 显式固定 commit 时按该 commit 求值，CLI 配方则选择 ref 后由服务冻结。

从任务开始：
  已有仓、固定 commit 和文件路径，要下载单文件：kc help files read
  已有 Dataset，要读文件：kc help files
  已有来源仓，要发布一组文件：kc help composition，然后 kc help compose
  还没有来源仓：kc help repository
  要选定版本或理解 pin：kc help versions / kc help pin
  要写配方或自定义属性：kc help recipe / kc help metadata

文件交付不要求 Schema 或检索索引。要按对象查询时另看 kc help object；原始文件路径用 files list/read。`

const guideRepository = `Repository 是保存来源文件、独立提交历史和写入边界的仓。
来源文件路径相对于原仓根目录；发布 Dataset 时，可以选择子目录或单文件，再映射到新的交付位置。

接入和交付分开：
  平台供给空仓或连接已有仓：kc help create
  把服务已能连接的仓登记成可选来源：kc help attach
  核对已登记身份：kc help show
  选择文件并发布交付范围：kc help dataset define
create 不自动 attach，attach 不把整仓加入任何 Dataset；两步也不隐含授予消费权。

来源准备：
  由来源管理者提供 Repository 身份、原仓相对路径、可用分支和完整 commit；查看原 lakeFS 的分支、提交历史与对象浏览器。
  KC 当前没有列举原仓分支/提交历史/原始目录树的业务命令。files list 只浏览已发布 Dataset 的交付树。
  已知固定 commit 和文件路径时，可按仓下载单文件，无需发布 Dataset：kc help files read。
  普通文件仓可由原生来源工具维护；符合对象发布合同的仓才沿 KC Writer 发布对象，见 kc help write。

Repository 自定义属性由原仓原生元数据管理，使用路径和权限见 kc help metadata。
选源版本见 kc help versions；Dataset 与原仓的关系见 kc help snapshot。`

const guideVersions = `区分可移动来源引用与固定交付版本：
  commit：一个 Repository 的完整文件版本标识，从来源提交历史取得。
  ref：可指向另一 commit 的引用。当前 lakeFS 来源选择支持 HEAD、refs/heads/main 和 refs/heads/<分支名>。
  refs/heads/main 与 HEAD 在这里指向已连接原仓的默认分支；显式其它分支写 refs/heads/release 这样的形式。
 Dataset revision：同名 Dataset 递增的发布版号；发布时解析各来源引用，冻结实际 commit 与文件范围。

选源输入：
  kc dataset define delivery --revision 1 --source 'docs=refs/heads/release@reference@handbook'
  这里从 docs 的 release 分支选择 handbook/，交付到 reference/；实际接受的来源 commit 见命令回执和 dataset show。
  argv 省略 selector 使用 refs/heads/main；配方和 overlay 的每个条目必须显式写 selector。
  发布时，当前 lakeFS 不能把裸分支名、published、tag 引用或 commit ID 直接当作 selector。
  配方可写 baseRev：要求所选引用在发布时仍指向这个完整 commit；它是预期版本核对，不会选择任意旧 commit。
  本机 overlay 不解析远端引用，有改动时只检查 selector 非空及同仓一致等结构；空补丁只返回底稿。
  它可能保留 selector: main 这样的未支持引用。
  离线成功不证明来源可解析，发布前仍要按上述形式选择并核对。

怎样取得想选的版本：
  在原 lakeFS 的分支/提交历史/对象浏览器确认分支、完整 commit 和相对路径，或向来源管理者取得它们。
  要交付一个未被现有分支指向的旧 commit，先由来源管理者建立并保留指向它的专用分支，再选择该分支；KC 没有代建来源分支入口。
  KC 当前不提供来源分支/历史/原始文件浏览。已经发布的固定来源可用 kc dataset show delivery --revision 1 回读核对。

消费时：
  按仓读取单文件必须提供 --repo 与 --commit；采用文件地址所指的资源版本，不自动采用对象说明的 commit 或分支当前值。
  files list/read 可显式选已接受的 Dataset revision；第一次默认当前版后，续页和后续读取沿用返回版号。
  dataset clone 只消费本次解析的当前版，重试可能采用更晚发布；需要历史材料用 files list/read。
  固定版本不冻结权限。pin 的含义见 kc help pin；完整配方写法见 kc help recipe。
按词查说明也可使用 kc help selector / kc help ref / kc help commit；三者沿用这里的同一份版本关系与选源规则。`

const guidePin = `pin 是服务将一次 Dataset 选择解析为固定来源版本与文件范围后的结果，pinId 是它的识别依据。
它把一次消费采用的 Dataset revision、各仓 commit 和文件布局连在一起，不是新的可写仓或新的发布版。

使用者不需要先创建 pin 文件，产品命令也不接收 --pin。
files list/read 会解析并核对发布依据；续页沿用返回的 revision、目录和 continuation，后续读取使用同一 revision。
dataset clone 解析当前发布一次，整个复制期间沿用这份依据；一次新的 clone 会重新选择当前版。
pin 不授予权限，旧 pin 也不能绕过退役、成员移除或当前撤权。

下一步：kc help files / kc help versions / kc help dataset show。`

const guideRecipe = `配方是 Dataset 文件选择和交付布局的便携输入，保存为 YAML；它本身不是已接受的 Dataset 发布。
其中 name 是 Dataset 名称，mounts 描述来源目录到交付目录，files 描述来源文件到交付文件，metadata 是可选字符串键值属性。
每条来源必须显式写 selector；baseRev 只核对所选分支当前 commit，不能直接选旧版本。

完整可复制例子：
  kc help dataset define：给出 recipe.yaml 的完整内容和发布调用。
  kc help dataset overlay：给出底稿、overlay.yaml、合成调用与预期布局变化。
先把例子中的仓身份、来源路径和引用换成来源管理者已确认的坐标。

overlay 只在本机调整候选，不发布、不开连接、不改变登记状态。
不带 --out 会在 stdout 展示候选 JSON；带 --out 可覆盖目标文件，应使用新的输出路径。
合成后需要共享时，再按已接受历史选择新的 revision 执行 dataset define。
有改动的本机合成只做部分结构检查；空补丁只返回底稿，不补做组合校验。离线成功不证明所有输入可发布。
当前单文件 file/target 的越界检查在服务接受发布前完成。
例如 target: ../outside.csv 可以残留在未发布候选中，但不能形成合法交付；请按合法相对路径填写并核对。
配方保存位置和发布时可能已发生的独立写回，见 kc help dataset define 的后果与恢复。`

const guideMetadata = `Repository 和 Dataset 都可承载可选自定义资源元数据，形式是字符串键值，例如 description、owner 或你的自定义键。
属性不改变资源身份、文件范围或授权；没有强制说明文件，也不会从来源正文自动推导或继承属性。

Repository：
  使用 lakeFS 原仓的原生元数据，表示当前原仓属性，不随 commit 冻结。
  本服务 /ui/ → Repositories → 目标仓 → Settings → General → Repository 自定义元数据。
  读取需 repository.metadata.read，增改或删键需 repository.metadata.write；进入页面另受原仓浏览权限检查，无法进入时联系来源管理者。
  KC CLI 的 show 当前不展示原仓属性，create 不接收 --metadata，attach 不读写或复制它们。
  保存可能包含多次增改/删除操作；失败先回读当前值，不把部分失败当作全部未写入。

Dataset：
  kc dataset define delivery --revision 1 --source 'docs@reference@handbook' --metadata '{"description":"项目材料","owner":"team-a"}'
  也可在配方 metadata 节填写，或完全省略。显式 --metadata 整份替换输入属性，{} 清空。
  来源范围、布局和属性一起随 revision 固定；改属性同样需要更大的 revision，并明确保留希望沿用的来源和其它属性。
  不会自动继承原仓属性；直接指定来源并省略 metadata 也不会自动继承旧版属性。
  kc show 查看当前 Dataset 摘要；kc dataset show delivery --revision 1 查看历史版属性。

定义/清空完整规则见 kc help dataset define；历史回读见 kc help dataset show。`
