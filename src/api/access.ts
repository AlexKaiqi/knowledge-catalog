/**
 * Access — the read boundary. RESOLVE / READ / LIST / SEARCH / ORIGIN on a
 * pinned commit. Every result carries repository/commit/object provenance.
 * Depends only on the Repository contract (store-agnostic).
 *
 * Target may be a KnowledgeRef (assembled entity) or a KnowledgeAddress
 * (one aspect/member). AspectSelector is a read/index strategy, not a write.
 */

import type {
  AspectSelector,
  CommitIdentity,
  KnowledgeAddress,
  KnowledgeRef,
  KnowledgeValue,
  ProvenanceTrace,
  Resolution,
  Repository,
} from "../contracts/index.ts";
import { selectAspects } from "../contracts/access.ts";
import { IngressError } from "../contracts/errors.ts";
import type { Store } from "../store.ts";

export class Access {
  constructor(private readonly store: Store) {}

  private repo(ref: KnowledgeRef): Repository {
    return this.repoById(ref.repository);
  }

  private repoById(repositoryId: string): Repository {
    const repo = this.store.repos.get(repositoryId);
    if (!repo) throw new IngressError("KNOWLEDGE_REF_UNRESOLVED", `unknown repository ${repositoryId}`);
    return repo;
  }

  resolve(ref: KnowledgeRef, commitId: CommitIdentity): Resolution {
    return this.repo(ref).resolve(ref.object, commitId);
  }

  read(ref: KnowledgeRef, commitId: CommitIdentity, selector?: AspectSelector): KnowledgeValue {
    const value = this.repo(ref).read(ref.object, commitId);
    if (!selector) return value;
    return { ...value, value: selectAspects(value.value, value.units, selector) };
  }

  resolveAddress(repositoryId: string, address: KnowledgeAddress, commitId: CommitIdentity): Resolution {
    return this.repoById(repositoryId).resolveAddress(address, commitId);
  }

  readAddress(repositoryId: string, address: KnowledgeAddress, commitId: CommitIdentity): KnowledgeValue {
    return this.repoById(repositoryId).readAddress(address, commitId);
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
