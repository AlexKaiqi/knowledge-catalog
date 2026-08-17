/**
 * ControlPlane — the active-maintenance loop (Phase 3). It orchestrates
 * PROPOSAL → PreviewGeneration → Validation → Merge → Promote WITHOUT bypassing
 * the domain protocols (K-21): all state changes go through Ingress/Repository,
 * reads go through Access.
 *
 * Invariants enforced here:
 *  - PROPOSAL writes a Candidate Branch/Commit, never main (K-07).
 *  - ValidationReport binds a full PreviewGeneration, not just a candidate (K-09).
 *  - A candidate that advances invalidates prior validation (ADR-013).
 *  - Merge is a CAS fast-forward; Promote is a SEPARATE CAS on the channel.
 */

import type {
  CommitChangeSet,
  CommitIdentity,
  Operation,
  RepositoryIdentity,
} from "../contracts/index.ts";
import { IngressError } from "../contracts/errors.ts";
import type { Ingress } from "../api/ingress.ts";
import type { Store } from "../store.ts";

export interface Proposal {
  readonly proposalId: string;
  readonly targetRepository: RepositoryIdentity;
  readonly targetRef: string;
  readonly candidateRef: string;
  readonly baseCommit: CommitIdentity;
  readonly candidateCommit: CommitIdentity;
  readonly rationale?: string;
}

export interface PreviewGeneration {
  readonly previewId: string;
  readonly repositoryCommits: Readonly<Record<RepositoryIdentity, CommitIdentity>>;
  readonly candidate: { readonly repositoryId: RepositoryIdentity; readonly commitId: CommitIdentity };
}

export interface ValidationReport {
  readonly reportId: string;
  readonly previewId: string;
  readonly suiteRevision: string;
  readonly outcome: "PASSED" | "FAILED";
}

export class ControlPlane {
  private readonly channels = new Map<string, CommitIdentity>();

  constructor(
    private readonly store: Store,
    private readonly ingress: Ingress,
  ) {}

  /** PROPOSAL: commit to a candidate ref (never main). */
  propose(input: {
    proposalId: string;
    repositoryId: RepositoryIdentity;
    targetRef: string;
    candidateRef: string;
    baseCommit: CommitIdentity;
    operations: readonly Operation[];
    rationale?: string;
  }): Proposal {
    const repo = this.store.repos.get(input.repositoryId);
    if (!repo) throw new IngressError("TARGET_REPOSITORY_DENIED", `unknown repository ${input.repositoryId}`);

    // baseCommit = main's head at proposal open (the fast-forward origin at merge time).
    // candidateParent = where the candidate commit forks from (candidate head, or base on first open).
    const candidateParent = repo.getRef(input.candidateRef) ?? input.baseCommit;
    if (repo.getRef(input.candidateRef) === undefined) {
      repo.createRef(input.candidateRef, candidateParent);
    }
    const cs: CommitChangeSet = {
      targetRepository: input.repositoryId,
      targetRef: input.candidateRef,
      baseCommit: candidateParent,
      expectedTargetCommit: candidateParent,
      operations: input.operations,
      message: input.rationale,
    };
    const receipt = this.ingress.commit(`proposal:${input.proposalId}`, cs);
    return {
      proposalId: input.proposalId,
      targetRepository: input.repositoryId,
      targetRef: input.targetRef,
      candidateRef: input.candidateRef,
      baseCommit: input.baseCommit,
      candidateCommit: receipt.result.commitId,
      rationale: input.rationale,
    };
  }

  /** CREATE_PREVIEW: substitute the repo's commit with the candidate commit. */
  createPreview(proposal: Proposal): PreviewGeneration {
    const previewId = `preview-${proposal.candidateCommit}`;
    return {
      previewId,
      repositoryCommits: { [proposal.targetRepository]: proposal.candidateCommit },
      candidate: { repositoryId: proposal.targetRepository, commitId: proposal.candidateCommit },
    };
  }

  /** VALIDATE_GENERATION: bind a report to a full preview generation + suite. */
  validate(preview: PreviewGeneration, suiteRevision: string, outcome: "PASSED" | "FAILED"): ValidationReport {
    return {
      reportId: `val-${preview.previewId}-${suiteRevision}`,
      previewId: preview.previewId,
      suiteRevision,
      outcome,
    };
  }

  /** MERGE: gate then CAS fast-forward main to the candidate commit. */
  merge(proposal: Proposal, validation: ValidationReport): CommitIdentity {
    const repo = this.store.repos.get(proposal.targetRepository);
    if (!repo) throw new IngressError("TARGET_REPOSITORY_DENIED", `unknown repository ${proposal.targetRepository}`);

    // Gate: validation must be bound to the current candidate and PASSED.
    if (validation.previewId !== `preview-${proposal.candidateCommit}`) {
      throw new IngressError("VALIDATION_BASIS_MISMATCH", "validation not bound to current candidate commit");
    }
    if (validation.outcome !== "PASSED") {
      throw new IngressError("VALIDATION_BASIS_MISMATCH", "validation did not pass");
    }
    if (repo.getRef(proposal.candidateRef) !== proposal.candidateCommit) {
      throw new IngressError("CANDIDATE_MOVED", "candidate advanced after validation");
    }
    return repo.merge(proposal.targetRef, proposal.candidateCommit, proposal.baseCommit);
  }

  /** PROMOTE: CAS move a channel to a new serving commit (Catalog pointer only). */
  promote(channel: string, expectedCommit: CommitIdentity | undefined, newCommit: CommitIdentity): void {
    const current = this.channels.get(channel);
    if (current !== expectedCommit) {
      throw new IngressError("PROMOTION_CAS_FAILED", `expected ${expectedCommit} but channel is ${current}`);
    }
    this.channels.set(channel, newCommit);
  }

  channel(channel: string): CommitIdentity | undefined {
    return this.channels.get(channel);
  }
}
