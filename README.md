# Knowledge Catalog — 单一语义协议 + Git Store（参考骨架）

面向 AI 知识底座的一套统一 Catalog 协议（TypeScript）。

**Catalog 语义只有一套**：身份、版本、来源、写边界、ViewGeneration、维护闭环、联邦读取。不同的只是 store adapter；当前实现使用真实 git，未来可按数据规模替换为 Dolt/PostgreSQL 等，而协议层不变（K-23）。

## 核心理念

> 别把 git 已经会的东西重新发明成协议；只定义 git 不提供的三样——**身份、来源、写边界**——其余通过 store adapter 映射到成熟底层。

- **身份**（RESOLVE）：`ObjectIdentity ≠ path`，身份内嵌文件内容（frontmatter），address-map 是可重建 projection。维护单元唯一键是 Address（`object_id` + aspect + member），不是裸 `object_id`。
- **来源**（ORIGIN）：精确 commit 坐标 + frontmatter provenance；DERIVATION 强制固定 input/algorithm。
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
│   └── maintenance.ts  # PROPOSAL → 完整 Preview → Validate → Merge
├── catalog/
│   └── catalog.ts      # Generation Registry、联邦读、Preview、Promote
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
npm test            # conformance T1–T12，47 case；Repository 相关用例跑真实 git
```

## Conformance

| 测试 | 不变量 |
|---|---|
| T1 Path Move | 文件移动后 ObjectIdentity / KnowledgeRef 不变 |
| T2 Commit CAS | 过期 expected target commit 被拒绝 |
| T3 Atomicity | 任一操作失败无部分提交 |
| T4 Command Idempotency | 精确重试返回原 Receipt；异内容冲突 |
| T5 Append Idempotency | Event ID 幂等、异内容冲突、expected cursor CAS |
| T6 FileGit Store | object_id、移动、CAS、ORIGIN、pinned tree read、DERIVATION 约束、Aspect 独立单元 |
| T7 Ingestion/Grounding | ingest 扫描、reconcile 对账、groundingCitation |
| T8 Embedded Access | SQLite FTS5 定位 + Canonical 读值；可重建、非权威、basis/lag；AspectSelector 不编 ACL |
| T9 Maintenance Loop | 完整多 Repo Preview、Validation basis、Merge/Promote 分离 |
| T10 Refine | SEM_FILTER 三值 + Ref-preserving；SEM_RERANK RankGroup |
| T11 Catalog | Generation Registry、故障传播、来源不覆盖、有效 Promote CAS |
| T12 Repository Contract | Adapter Factory 可复用的身份、版本、CAS、Append 接口断言 |

## 文档

- `KNOWLEDGE_CATALOG_DESIGN.md`：整合后的正式设计文档与决策留痕
- `ASPECT_ACCESS.md`：Aspect 写单元 vs 读/检索形态（业界对照与决策）
- `WALKTHROUGH_v5.1.md`：用统一协议做端到端推演
- 旧版白皮书与 v4.0 推演已归并到上述文档，不再单独维护

## Store 扩展

新增 Dolt/PostgreSQL 等实现时，实现 `Repository` 接口并通过 T12 的共享 Contract Test Kit；Ingress、Access、ControlPlane、Catalog 与协议对象保持不变。生产 Adapter 还必须补足持久化、进程间并发、授权与恢复保证。
