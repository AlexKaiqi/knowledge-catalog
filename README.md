# Knowledge Catalog — Phase 0

面向「团队/组织共用」的 AI 知识底座的最小语义层骨架（TypeScript）。

本目录是设计文档（白皮书 v5.0 + 全流程推演 v4.0）在 **WorkSurface** 上收敛出的最小语义层的可执行验证。

## 核心理念

> 别把 git 已经会的东西重新发明成协议；只发明 git 不会的那三样——**身份、来源、写边界**——其余全部坍缩成 git 原生 + 一个薄 CLI。

- **身份**（RESOLVE）：`ObjectIdentity ≠ path`，身份内嵌文件内容，address-map 是可重建 projection。
- **来源**（ORIGIN）：最小 provenance 链，git commit 元数据 + frontmatter + DERIVATION 强制 input/algorithm。
- **写边界**（Ingress）：COMMIT（= git commit + CAS ref）+ APPEND（append-only 流）。

## 结构

```
src/
├── contracts/          # 纯契约类型（不依赖运行时）
│   ├── identity.ts     #   KnowledgeRef / PinnedKnowledgeRef / FileRef
│   ├── address.ts      #   KnowledgeAddress
│   ├── surface.ts      #   COMMIT/PROPOSAL/APPEND、PUT/REMOVE、Precondition
│   ├── provenance.ts   #   ProvenanceEnvelope
│   ├── receipt.ts      #   CommitReceipt / AppendReceipt
│   ├── access.ts       #   Resolution / KnowledgeValue / GroundingCitation
│   ├── view.ts         #   ViewGeneration/ViewReadVersion（占位，多人展开）
│   └── errors.ts       #   结构化错误码
├── adapters/memory/    # Phase 0 Memory Adapter
│   ├── repository.ts   #   git-like：immutable object/tree/commit + ref CAS
│   └── append.ts       #   append-only stream + event-id 幂等
├── api/
│   ├── ingress.ts      #   写边界：幂等 + 目标路由 + COMMIT/APPEND
│   └── access.ts       #   读边界：RESOLVE/READ/LIST/SEARCH/ORIGIN
└── store.ts            #   内存持有 repos + streams
```

## 运行

```bash
npm install
npm run typecheck   # tsc --noEmit
npm test            # vitest run（conformance T1–T5）
```

## Conformance（当前通过）

| 测试 | 不变量 |
|---|---|
| T1 Path Move | 文件移动后 ObjectIdentity / KnowledgeRef 不变 |
| T2 Commit CAS | 过期 expected target commit 被拒绝 |
| T3 Atomicity | 任一操作失败无部分提交 |
| T4 Command Idempotency | 精确重试返回原 Receipt；异内容冲突 |
| T5 Append Idempotency | 同 event id 同内容重放；异内容冲突 |

## 薄度验收标准

单人 Profile 下，只懂 git + 文件 + grep 的 Coding Agent，零新协议即可完成
`RESOLVE → READ → ORIGIN → COMMIT`，并追问「这条来自哪、哪个版本」。
