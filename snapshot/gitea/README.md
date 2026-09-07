# snapshot/gitea/

Gitea 是⓪ Snapshot 的远程实现：`snapshot.Store` / `TreeStore` / `HistoryStore`。按 commit 取 tree/blob（Git 对象 API），用 `PUT /branches` 做 ref CAS。不解释 frontmatter，不 clone、没有工作区、没有 `RootDir`。

需要 **Gitea 1.26+**（`UpdateBranch` / `old_commit_id`）。本 Adapter 只承担 Snapshot；State/Stream 通过墙外 Binding 访问。Token 只走 `KC_GITEA_TOKEN`。

接入和进程恢复使用 `OpenExisting(id, dsn, token)`，只读取已有 Repository、published ref 与 commit；源缺失、为空或已发布版本不可访问均失败，不创建仓、不补分支或 `.kc/root`。普通 Git 文件不要求知识格式。已有合法 published commit 的空树也有效；“空源”是没有可解析的 published snapshot。`Open` 保留为显式创建/初始化路径，供创建流程与测试夹具使用，不能用于接入/恢复。

平台托管供给使用 `CreateManaged`：应用先持久预留随机 allocation，再在同一次 Gitea 建仓请求的 description 中写入归属标识，记录返回的后端仓 ID。重试只认同一 allocation；同名外部仓无论是否为空都不能被收养。创建响应丢失时只查询原位置，不换名重复创建。`OpenManaged` 同时校验 allocation 与后端 ID，缺失或替换的源不自动重建。托管句柄在后续后端调用前继续校验归属；普通外部源不增加这些检查。知识上传仍经 Writer，连接和供给记录不进入 Catalog 登记表。

DSN 是仓库网页/clone URL（`{origin}/{owner}/{name}`），密码不得写进 URL。生产部署从声明绑定或耐久托管账恢复连接，不依赖目录发现。`remote.yaml` 指针仅用于组件夹具，adapter 没有本机 `.git`。

当前批量文件提交先在短生命周期 `kc-wip/*` branch 生成 commit，再用
`old_commit_id` 对目标 ref 做 CAS 并删除临时 branch。Gitea 1.26 的异步 action notifier
可能晚于删除动作读取该 branch，从而留下 “object does not exist” 日志；目标 commit/ref
与 KC Receipt 不受影响。若部署方把此类日志作为告警，应在 adapter 改用底层
blob/tree/commit API、完全移除临时 branch 后再把该告警视为零容忍。
