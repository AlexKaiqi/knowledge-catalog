# repository/

**⓪ 操作语义的口**，外加当前 FileGit / Dolt 仍承担的 **② 知识解释**。

```text
SnapshotStore   git 形权威：commit / ref / CAS。Catalog 成员、用户可挂的仓。
Stream          有序段：cursor / eventId。不是 git，不是 Catalog 成员。
Knowledge       解释某次 commit 上的文件：object_id / Aspect / 来源。pin 不经过这里。
Repository      SnapshotStore + Knowledge（FileGit / Dolt / Gitea）。APPEND 不在此口。
Snapshot        COMMIT/merge 的 from→to 事件（hook），不是 SnapshotStore
Store           成员仓 map + 按仓 id 绑定的 Stream map（ACL/同居，流 ≠ 仓）
```

分层总表见 [`docs/LAYERS.md`](../docs/LAYERS.md)。不要 `repo-add --driver stream`。

实现：

| 口 | local | scale |
|---|---|---|
| SnapshotStore | `FileGitRepository` | `DoltRepository`（git 形知识文件；原生 Dolt SQL 未装配） |
| SnapshotStore（远程） | — | `gitea.Repository`（Git 对象 API + 分支 CAS，无工作区；Gitea 1.26+） |
| Stream | `JSONLStream` | `OpenStream` stub（Append → `CAPABILITY_UNSATISFIED`） |

Catalog `checkGeneration` 只用 SnapshotStore（仓已挂、commit 存在）。消费读走 `Catalog.Serving`（`OpenRelease`）；`FederatedRead` 是内部 union。

Writer：`COMMIT`/`PROPOSAL` → SnapshotStore；`APPEND` → Stream；ChangeSet 的 PUT/REMOVE 是 ②。
