# repository/

这里定义 ⓪ Snapshot 口和当前参考适配器承担的 ② Knowledge 解释：

```text
SnapshotStore   commit / ref / CAS；Catalog 成员
Knowledge       在固定 commit 解释 object_id / Aspect / provenance
RawFileStore    可选的字面路径读写能力，不认识 object_id
Repository      SnapshotStore + Knowledge
Snapshot        COMMIT/merge 的 from→to 通知事件
Store           只保存成员 SnapshotStore；Knowledge 在需要 ② 的接缝上按能力取得
```

`Operation` 的变化代数只有 PUT/REMOVE。PUT 可在 Address 上声明 `ValueSource`：默认 Snapshot；Binding 则声明 state/stream observation 的逻辑 runtime、protocol 和 operation，或引用同一 commit 的 `ResourceDescriptor`。Binding 声明有独立 `DeclarationDigest`，不会和 value digest 混为一个版本。

| Snapshot 实现 | 介质 |
|---|---|
| `local.FileGitRepository` | 本机 git |
| `scale.DoltRepository` | Dolt |
| `gitea.Repository` | Gitea Git 对象 API + 分支 CAS |

Catalog pin 只检查成员及 commit；动态 observation cut 由上层 Materialization/Retrieval 请求持有。
