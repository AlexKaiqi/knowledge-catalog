/**
 * Reader — the read API. RESOLVE / READ / LIST / SEARCH / GET_PROVENANCE
 * and READ_STREAM on a pinned commit (or an append stream). Every object
 * result carries repository/commit/object coordinates.
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
  ObjectDiff,
  ObjectIdentity,
  ObjectRevision,
  ProvenanceTrace,
  Resolution,
  Repository,
  StreamSlice,
} from "../contracts/index.ts";
import { selectAspects } from "../contracts/access.ts";
import { IngressError } from "../contracts/errors.ts";
import type { Store } from "../store.ts";

export class Reader {
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

  /** GET_PROVENANCE: envelopes on this object's units. Does not walk refs or git log. */
  getProvenance(ref: KnowledgeRef, commitId: CommitIdentity): ProvenanceTrace {
    return this.repo(ref).getProvenance(ref.object, commitId);
  }

  readStream(repositoryId: string, streamRef: string): StreamSlice {
    return this.repoById(repositoryId).readStream(streamRef);
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

  log(repositoryId: string, objectId: ObjectIdentity, commitId: CommitIdentity, limit?: number): readonly ObjectRevision[] {
    return this.repoById(repositoryId).log(objectId, commitId, limit);
  }

  diff(
    repositoryId: string,
    objectId: ObjectIdentity,
    fromCommit: CommitIdentity,
    toCommit: CommitIdentity,
  ): ObjectDiff {
    return this.repoById(repositoryId).diff(objectId, fromCommit, toCommit);
  }
}
