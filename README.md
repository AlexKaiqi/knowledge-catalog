# Knowledge Catalog — 单一语义协议 + Git Store（完整骨架）

面向 AI 知识底座的一套统一 Catalog 协议（TypeScript）。

**Catalog 语义只有一套**：身份、版本、来源、写边界、ViewGeneration、维护闭环、联邦读取。不同的只是 store adapter；当前实现使用真实 git，未来可按数据规模替换为 Dolt/PostgreSQL 等，而协议层不变（K-23）。

## 核心理念

> 别把 git 已经会的东西重新发明成协议；只定义 git 不提供的三样——**身份、来源、写边界**——其余通过 store adapter 映射到成熟底层。

- **身份**（RESOLVE）：`ObjectIdentity ≠ path`，身份内嵌文件内容（frontmatter），address-map 是可重建 projection。
- **来源**（ORIGIN）：最小 provenance 链，git commit 元数据 + frontmatter + DERIVATION 强制 input/algorithm。
- **写边界**（Ingress）：COMMIT / PROPOSAL / APPEND 是同一协议的写入语义。
- **当前 store**：Snapshot 用真实 git；Append 用 gitignore 的 JSONL side file（保持非 Git 演化语义）。

## 结构

```text
src/
├── contracts/
│   └── repository.ts   # 统一 Repository 接口（协议层唯一依赖）
├── adapters/
│   ├── file-git/       # 当前 store：真实文件 + git + JSONL append
│   └── embedded/       # SQLite FTS5 投影（可重建、非权威、basis/lag）
├── api/
│   ├── ingress.ts      # 写边界：幂等 + 路由 + COMMIT/APPEND
│   ├── access.ts       # RESOLVE/READ/LIST/SEARCH/ORIGIN
│   ├── ingestion.ts    # INGEST / RECONCILE / groundingCitation
│   └── refine.ts       # SEM_FILTER / SEM_RERANK（Ref-preserving）
├── control-plane/
│   └── maintenance.ts  # PROPOSAL → Preview → Validate → Merge → Promote
├── catalog/
│   └── catalog.ts      # ViewDefinition→ViewGeneration、联邦读、Promote
├── digest.ts
├── store.ts            # Map<RepositoryIdentity, Repository>
└── index.ts
scripts/
└── assemble-doc.sh     # WorkSurface checkout → 单文件文档
```

## 运行

```bash
npm install
npm run typecheck
npm test            # conformance T1–T11，全部跑真实 git
```

## Conformance

| 测试 | 不变量 |
|---|---|
| T1 Path Move | 文件移动后 ObjectIdentity / KnowledgeRef 不变 |
| T2 Commit CAS | 过期 expected target commit 被拒绝 |
| T3 Atomicity | 任一操作失败无部分提交 |
| T4 Command Idempotency | 精确重试返回原 Receipt；异内容冲突 |
| T5 Append Idempotency | JSONL stream：同 event id 同内容重放；异内容冲突 |
| T6 FileGit Store | frontmatter 内嵌 object_id、移动、CAS、ORIGIN |
| T7 Ingestion/Grounding | ingest 扫描、reconcile 对账、groundingCitation |
| T8 Embedded Access | SQLite FTS5 定位 + Canonical 读值；可重建、非权威、basis/lag |
| T9 Maintenance Loop | 真实 git candidate branch、Merge CAS、Promote 分离 |
| T10 Refine | SEM_FILTER 三值 + Ref-preserving；SEM_RERANK RankGroup |
| T11 Catalog | 多 Repo 联合：确定性 generation、来源保留不覆盖、Promote CAS |

## 文档

- `WHITEPAPER_v5.1.md`：正式设计文档（结论叙述）
- `WALKTHROUGH_v5.1.md`：用统一协议做端到端推演
- `KNOWLEDGE_CATALOG_DESIGN.md`：WorkSurface 模板组装快照
- WorkSurface：权威决策与证据留痕

## Store 扩展

新增 Dolt/PostgreSQL 等实现时，只需实现 `Repository` 接口；Ingress、Access、ControlPlane、Catalog 与所有协议对象不变。store 的选择由数据规模、查询形态、部署约束决定，而不是由“单人/多人”决定。
