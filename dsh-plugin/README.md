# dsh-loom

DSH 的 Knowledge Catalog 宿主集成。用户可以直接用自然语言询问当前知识
Workspace，也可以在侧栏“知识”中浏览同一固定版本的只读文件。插件不注册模型
工具；Agent 使用分组后的 `kc` CLI，文件访问使用 DSH 原有 shell 与文件系统工具。

## 第一次提问（约 30 秒）

如果任务已正常打开，身份、Catalog、Workspace 和本次固定知识版本都已由宿主准备
完成。普通用户不需要配置它们，也不需要先学 KC 命令。直接描述：

- 主题或对象，例如“支付告警”或某个 runbook；
- 必要范围，例如生产环境、某个团队或时间段；
- 希望的输出，例如排障步骤、对比、摘要或来源说明。

例如直接问：“搜索支付告警的处理方法，读取最相关的 runbook，并给出可执行步骤和
来源。”Agent 会先发现候选，再读取固定版本上的正式内容；搜索标题本身不会被当作
最终答案。

侧栏“知识”用于浏览已挂载的只读文件，适合已知目录或文件名的场景；它不是完整的
结构化知识目录。若侧栏显示“未连接”，请把下方宿主配置交给管理员处理，不要让普通
用户猜 Workspace 或身份。

## 安装与宿主配置（管理员，约 5 分钟）

开始前需要一个已经定义好的 Catalog Workspace，以及 Linux 主机上的 `kcfs`、
`/dev/fuse` 和 `fusermount3`。插件不会替你创建 Repository、发权或猜测身份。

1. 配置当前任务使用的知识坐标：

   ```bash
   export KC_HOME=/absolute/path/to/private-task-state
   export KC_SERVER_URL=http://127.0.0.1:7380
   export KC_CATALOG=kr://acme/catalog       # 多 Catalog 时必须明确
   export KC_WORKSPACE=agent
   export KC_AS=agent:dsh
   export KCFS_BIN=/absolute/path/to/kcfs
   ```

2. 在项目根先做无挂载预检：

   ```bash
   kcfs plan --server "$KC_SERVER_URL" --as "$KC_AS" --catalog "$KC_CATALOG" \
     --workspace "$KC_WORKSPACE" --root "$PWD"
   ```

   本机与共享部署都使用同一 typed Workspace File Gateway；`KC_HOME`
   只保存任务私有上下文，不是供 `kcfs` 直开的 Repository Home。预检应返回固定 pin 和 mount 计划。

3. 由任务宿主在本次任务的临时 DSH home 中安装并启动插件：

   ```bash
   export DSH_LOOM_PLUGIN=/absolute/path/to/knowledge-catalog/dsh-plugin
   task_dsh_home="$(mktemp -d /tmp/dsh-loom-task.XXXXXX)"
   DSH_HOME="$task_dsh_home" dsh plugin --profile task add "file:$DSH_LOOM_PLUGIN"
   DSH_HOME="$task_dsh_home" dsh --profile task
   ```

   `task_dsh_home` 属于任务生命周期，任务结束后由宿主回收。不要把这个仓库内插件
   固定安装到用户的 `~/.dsh/profiles` 或通用 `web`/`headless` profile。
   当前包是仓库内私有插件，尚未以公开包名发布。不要执行
   `dsh plugin ... add dsh-loom`：公开 registry 中的同名包属于无关项目。
   正式发布前必须选择受控的 npm scope/name，并同步这里的安装命令。

   上述命令假设 `dsh` 已在 `PATH`。若使用 DSH 源码 checkout，可执行
   `pnpm --dir /absolute/path/to/deepseek-harness dsh ...`；自动化脚本也接受
   `DSH_EXECUTABLE=/absolute/path/to/apps/cli/lib/bin.js`。

任务只有在固定 pin 和所有只读目录挂载完成后才会打开。首次进入时，侧栏“知识”
默认已启用但保持折叠，不会提前扫描文件。可以直接：

- 问 Agent：“搜索支付告警的处理方法，并读取最相关的 runbook”；
- 问 Agent：“解释这个对象的来源”；
- 展开侧栏“知识”，点击文件预览；
- 在 shell 中运行 `rg '回滚' knowledge/` 搜索已挂载文件。

结构化知识发现使用 `kc knowledge search`；文件浏览不是 Knowledge LIST，也不保证
覆盖未挂载的知识。

## 运行合同

宿主配置：

- `KC_HOME`：必填绝对路径，保存私有任务上下文；
- `KC_SERVER_URL`：必填；本机部署也先启动 `kc serve`，`kcfs` 只通过 typed Workspace File Gateway 读取；
- `KC_WORKSPACE`：必填，任务要使用的组合；
- `KC_CATALOG`：多 Catalog 时应明确，避免依赖本机默认 Catalog；
- `KC_AS`：必填，明确的 Agent principal；
- `KCFS_BIN`：可选，默认 `kcfs`。

任务创建时，根插件同步调用 `kcfs daemon-mount`。该命令只在所有知识目录均已
只读挂载并产生固定 pin 后返回；失败会阻止任务进入未挂载状态。上下文和 mount
manifest 写入 `$KC_HOME/tasks/`，不会写进用户项目。子任务复用父任务的同一 mount
和 pin；最后一个引用释放时调用 `kcfs stop` 卸载。

同一项目根若已有不同 Workspace，会明确拒绝。没有 FUSE 的环境返回能力错误，
不会把知识静默复制到项目。用户项目的普通文件仍可写，只有 Workspace 配方指定
的知识目录是只读 mount。

## Agent 使用

典型路径：

```bash
kc knowledge search --query '支付告警'
kc knowledge read --object 'runbook/payment-alert'
kc knowledge provenance --object 'runbook/payment-alert'
rg '回滚' knowledge/
```

Catalog、Workspace、pin 和身份由当前 mount 上下文继承；与上下文冲突的显式参数
必须被 CLI 拒绝。系统没有公开 Knowledge 枚举，也不会在检索能力缺失时改做全仓扫描。

## “知识”侧栏

侧栏“知识”只读取 `$KC_HOME/tasks/*/context.json` 允许的已挂载目录，并通过
普通宿主文件 API 预览内容。它不调用 KC 文件协议，不接触 Repository 凭证，也不
向模型注册文件工具。首次默认启用但折叠；用户可关闭，开关偏好保存在 `$KC_HOME/ui/`。

## 常见问题

| 现象 | 如何恢复 |
|---|---|
| `KC_HOME must be absolute` | 改成绝对路径；它保存私有任务上下文，不应放进用户项目 |
| `KC_WORKSPACE is required` | 用 `kc catalog workspace list` 找到 Workspace id 后配置并重开任务 |
| `cannot start kcfs` | 安装/构建 `kcfs`，或把 `KCFS_BIN` 指向它的绝对路径 |
| mount/FUSE 能力错误 | 确认 Linux、`/dev/fuse` 和 `fusermount3`；先运行同坐标的 `kcfs plan` |
| `FORBIDDEN` | 由管理员给当前 `KC_AS` 发所需权限；不要换身份重试 |
| SEARCH 不可用 | 明确报告检索投影能力缺口；可浏览/`rg` 已挂载文件，但不要声称等价于结构化检索 |

## 开发验证

使用仓库 `.node-version` 声明的 Node 24 LTS；Node 23 不受当前测试工具链支持。

```bash
npm ci
npm run typecheck
npm test
npm run build
npm run pack:check
```

Linux 主机可直接运行仓库根 `scripts/e2e-kcfs-linux.sh`。在 macOS 上运行
`make test-kcfs-e2e`，会通过 Docker 显式传入 `/dev/fuse` 和挂载 capability，
验收真实 Linux mount 以及插件 `MountController → kcfs daemon-mount → stop`
生命周期；不要仅因宿主是 macOS 就把 mount 列为 SKIP。
