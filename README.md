# Knowledge Catalog — 最小语义层 + Catalog（完整）

面向「团队/组织共用」的 AI 知识底座的最小语义层（TypeScript）。

本目录是设计文档（WorkSurface）收敛出的最小语义层的可执行验证，以及 Phase 1 的 repo-native File+Git Profile。

## 核心理念

> 别把 git 已经会的东西重新发明成协议；只发明 git 不会的那三样——**身份、来源、写边界**——其余全部坍缩成 git 原生 + 一个薄 CLI。

- **身份**（RESOLVE）：`ObjectIdentity ≠ path`，身份内嵌文件内容（frontmatter），address-map 是可重建 projection。
- **来源**（ORIGIN）：最小 provenance 链，git commit 元数据 + frontmatter + DERIVATION 强制 input/algorithm。
- **写边界**（Ingress）：COMMIT（= git commit + CAS ref）+ APPEND（append-only 流）。

## 结构

```
src/
├── contracts/          # 纯契约类型（不依赖运行时）
├── adapters/
│   ├── memory/         # Phase 0：git-like immutable tree/commit + ref CAS；append stream
│   ├── file-git/       # Phase 1：真实文件 + git（frontmatter 内嵌 object_id）
│   └── embedded/       # Phase 2：SQLite FTS5 投影（可重建、非权威、记录 basis/lag）
├── api/
│   ├── ingress.ts      #   写边界：幂等 + 目标路由 + COMMIT/APPEND
│   ├── access.ts       #   读边界：RESOLVE/READ/LIST/SEARCH/ORIGIN
│   ├── ingestion.ts    #   INGEST / RECONCILE / groundingCitation（薄编排）
│   └── refine.ts       #   SEM_FILTER / SEM_RERANK（Ref-preserving，规则 judge）
├── control-plane/
│   └── maintenance.ts  #   PROPOSAL → Preview → Validate → Merge → Promote（维护闭环）
├── catalog/
│   └── catalog.ts      #   ViewDefinition→ViewGeneration、联合读（保留来源）、Promote（多 Repo）
├── store.ts
└── index.ts
scripts/
└── assemble-doc.sh     # 模板组装：surface.md + blocks → 单文件文档
```

## 运行

```bash
npm install
npm run typecheck   # tsc --noEmit
npm test            # vitest run（conformance T1–T7）
```

## Conformance

| 测试 | 不变量 |
|---|---|
| T1 Path Move | 文件移动后 ObjectIdentity / KnowledgeRef 不变 |
| T2 Commit CAS | 过期 expected target commit 被拒绝 |
| T3 Atomicity | 任一操作失败无部分提交 |
| T4 Command Idempotency | 精确重试返回原 Receipt；异内容冲突 |
| T5 Append Idempotency | 同 event id 同内容重放；异内容冲突 |
| T6 FileGit Profile | 真实文件 + git：frontmatter 内嵌 object_id、移动、CAS、ORIGIN |
| T7 Ingestion/Grounding | ingest 扫描、reconcile 对账、groundingCitation 投影 |
| T8 Embedded Access | SQLite FTS5 定位 + Canonical 读值；可重建、非权威、basis/lag |
| T9 Maintenance Loop | Proposal 隔离、candidate 前移失效、Merge CAS、Promote 分离 |
| T10 Refine | SEM_FILTER 三值 + Ref-preserving；SEM_RERANK RankGroup + unjudged |
| T11 Catalog | 多 Repo 联合：确定性 generation、K-10 去重、联合读保留来源不覆盖、Promote CAS |

## 组装文档

`KNOWLEDGE_CATALOG_DESIGN.md` 是由 WorkSurface（surface.md + 13 blocks）模板组装生成的可分发快照。权威文档是 WorkSurface 本身；脚本 `scripts/assemble-doc.sh` 负责在 `ws checkout` 目录上渲染。

## 薄度验收标准

单人 Profile 下，只懂 git + 文件 + grep 的 Coding Agent，零新协议即可完成
`RESOLVE → READ → ORIGIN → COMMIT`，并追问「这条来自哪、哪个版本」。
