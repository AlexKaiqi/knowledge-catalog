/**
 * Access — the read boundary. RESOLVE / READ / LIST / SEARCH / ORIGIN on a
 * pinned commit. Every result carries repository/commit/object provenance.
 */

import type {
  CommitIdentity,
  KnowledgeRef,
  KnowledgeValue,
  ProvenanceTrace,
  Resolution,
} from "../contracts/index.ts";
import { IngressError } from "../contracts/errors.ts";
import type { MemoryStore } from "../store.ts";

export class Access {
  constructor(private readonly store: MemoryStore) {}

  private repo(ref: KnowledgeRef) {
    const repo = this.store.repos.get(ref.repository);
    if (!repo) throw new IngressError("KNOWLEDGE_REF_UNRESOLVED", `unknown repository ${ref.repository}`);
    return repo;
  }

  resolve(ref: KnowledgeRef, commitId: CommitIdentity): Resolution {
    return this.repo(ref).resolve(ref.object, commitId);
  }

  read(ref: KnowledgeRef, commitId: CommitIdentity): KnowledgeValue {
    return this.repo(ref).read(ref.object, commitId);
  }

  origin(ref: KnowledgeRef, commitId: CommitIdentity): ProvenanceTrace {
    return this.repo(ref).origin(ref.object, commitId);
  }

  search(query: string, repositoryId: string, commitId: CommitIdentity): KnowledgeValue[] {
    const repo = this.store.repos.get(repositoryId);
    if (!repo) throw new IngressError("KNOWLEDGE_REF_UNRESOLVED", `unknown repository ${repositoryId}`);
    return repo.search(query, commitId);
  }

  list(repositoryId: string, commitId: CommitIdentity): KnowledgeValue[] {
    const repo = this.store.repos.get(repositoryId);
    if (!repo) throw new IngressError("KNOWLEDGE_REF_UNRESOLVED", `unknown repository ${repositoryId}`);
    return repo.list(commitId);
  }
}
