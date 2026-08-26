# Knowledge Catalog 当前 MVP 验收

状态：首版 VFS 的权威验收边界。当前版本只承诺 Linux 宿主级只读投影；写入仍显式走
Writer / TreeWriter，不把编辑器写文件隐式升级为新的 Write Surface。

## 用户结果

用户在原有项目目录启动 `kcfs` 后，Workspace 的若干 Repository 子树分别出现在配方指定
的位置。项目其余文件不移动、不复制、不需要特殊目录结构。用户、IDE、shell 和 Agent
使用同一棵宿主文件树；DSH 使用自己的标准 filesystem/search 工具。

```text
现有项目根
├── 用户原有文件
├── vendor/policy/       <- repo A: policy/
├── docs/catalog/        <- repo A: docs/
└── schemas/shared/      <- repo B: schema/
```

每个命令开始时只解析一次 Workspace selector。所有 mount 共用这次得到的 pin，直到
`kcfs` 进程退出都不跟随 Repository HEAD。

## 必须满足

| ID | 结果 | 机器可判定条件 |
|---|---|---|
| V1 | 附着任意已有项目 | 项目根只要求是可访问目录；非 mount 内容的 inode/bytes 不变 |
| V2 | 多目录组合 | 每个 `WorkspaceSource.Path` 是独立 mountpoint；可来自不同 Repo |
| V3 | 同仓多子树 | 同一 Repo 可用相同 selector/baseRev 投影多个互不重叠的 `SubPath`；pin 中仍只有一个 commit |
| V4 | 一致视图 | `cat`、`rg`、IDE 和 Agent 读到相同 bytes；manifest 中所有 mount 使用同一 pin |
| V5 | 固定版本 | mount 期间上游 ref 推进不改变已挂载 bytes；重启 `kcfs` 后才解析新 pin |
| V6 | 只读 | create/write/truncate/rename/remove 均失败，Repository ref 和用户原文件不变 |
| V7 | 授权 | 先检查 `read-workspace`，再逐 Repo 检查 `read`；无权成员不出现在 plan 或 mount 中 |
| V8 | 安全路径 | 拒绝绝对路径、`..`、反斜杠、NUL、根挂载、重叠 mount 和项目根下的 symlink 穿越 |
| V9 | 宿主失败可解释 | 缺 `/dev/fuse`、`fusermount3`、TreeStore capability 或非空 mountpoint 时明确失败 |
| V10 | 无 Agent 专用 VFS | DSH patch 不替换 `fs-sandbox` / `tool-fs-search`，不再导出 `loom-fs` / `loom-search` |

## Linux 环境要求

- Linux 内核可访问 `/dev/fuse`；
- 安装 `fusermount3`（通常来自发行版 `fuse3` 包）；
- 容器运行时需要显式暴露 `/dev/fuse` 以及挂载所需 capability；
- 每个 mountpoint 必须不存在或为空。父目录和项目其他内容没有布局要求。

首版不支持把单个文件直接作为 FUSE mount，也不允许把 Workspace source 挂到项目根：
前者不是可移植的 FUSE 目录挂载原语，后者会遮住用户整个项目。

## 验收入口

```bash
go test ./workspacefs ./catalog ./cli ./internal/arch -count=1
npm --prefix dsh-plugin run typecheck
npm --prefix dsh-plugin test
./scripts/e2e-kcfs-linux.sh
./scripts/e2e-kcfs-docker.sh
```

`scripts/e2e-kcfs-linux.sh` 在 Linux 上创建两个 Repository、三个 mountpoint（其中两个来自
同一 Repo 的不同子树），验证 `cat`、`rg`、只读拒绝、pin 和卸载清理。非 Linux 环境明确
输出 `SKIP`；不能把该结果记作真实 FUSE PASS。

`scripts/e2e-kcfs-docker.sh` 用官方 Go Debian 镜像执行同一测试，只授予
`/dev/fuse`、`SYS_ADMIN` 和解除 AppArmor mount 限制，不要求 `--privileged`。

## 后续写回边界

若未来提供可写体验，应另建显式的 checkout/overlay + reconcile/commit 流程，保留
`expectedTargetCommit`、`command_id`、逐 Repo 结果和冲突恢复。不能把 FUSE 的普通 write
系统调用直接解释成协议 COMMIT，也不能伪装跨 Repo 原子事务。
