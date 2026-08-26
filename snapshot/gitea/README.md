# snapshot/gitea/

Gitea 是⓪ Snapshot 的远程实现：`snapshot.Store` / `TreeStore` / `HistoryStore`。按 commit 取 tree/blob（Git 对象 API），用 `PUT /branches` 做 ref CAS。不解释 frontmatter，不 clone、没有工作区、没有 `RootDir`。

需要 **Gitea 1.26+**（`UpdateBranch` / `old_commit_id`）。本 Adapter 只承担 Snapshot；State/Stream 通过墙外 Binding 访问。Token 只走 `KC_GITEA_TOKEN`。

```bash
export KC_GITEA_TOKEN=...
kc repo-add --repo kr://acme/public/core --driver gitea --dsn http://127.0.0.1:3001/kc/public-core
```

DSN 是仓库网页/clone URL（`{origin}/{owner}/{name}`），密码不得写进 URL。本机只留 `repos/<encoded-id>/remote.yaml` 指针（无 `.git`）。

当前批量文件提交先在短生命周期 `kc-wip/*` branch 生成 commit，再用
`old_commit_id` 对目标 ref 做 CAS 并删除临时 branch。Gitea 1.26 的异步 action notifier
可能晚于删除动作读取该 branch，从而留下 “object does not exist” 日志；目标 commit/ref
与 KC Receipt 不受影响。若部署方把此类日志作为告警，应在 adapter 改用底层
blob/tree/commit API、完全移除临时 branch 后再把该告警视为零容忍。
