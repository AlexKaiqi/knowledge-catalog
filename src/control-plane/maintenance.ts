/**
 * ControlPlane orchestrates candidate writes and merge gates. Catalog owns
 * complete PreviewGenerations and serving Releases; Repository owns refs.
 */

import type {
  CommitChangeSet,
  CommitIdentity,
  Operation,
  RepositoryIdentity,
  ViewGeneration,
} from "../contracts/index.ts";
import { IngressError } from "../contracts/errors.ts";
import type { Writer } from "../api/writer.ts";
import type { Catalog } from "../catalog/catalog.ts";
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
  readonly baseGenerationId: string;
  readonly generation: ViewGeneration;
  readonly candidate: { readonly repositoryId: RepositoryIdentity; readonly commitId: CommitIdentity };
}

export interface ValidationReport {
  readonly reportId: string;
  readonly previewGenerationId: string;
  readonly suiteRevision: string;
  readonly outcome: "PASSED" | "FAILED";
}

export class ControlPlane {
  constructor(
    private readonly store: Store,
    private readonly writer: Writer,
    private readonly catalog: Catalog,
  ) {}

  /** PROPOSAL: commit to a candidate ref without moving the target ref. */
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
    const receipt = this.writer.commitIntent(`proposal:${input.proposalId}`, cs);
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

  /** CREATE_PREVIEW: replace one member and preserve the rest of the base generation. */
  createPreview(baseGenerationId: string, proposal: Proposal): PreviewGeneration {
    const base = this.catalog.generation(baseGenerationId);
    if (base.repositories[proposal.targetRepository] !== proposal.baseCommit) {
      throw new IngressError("VALIDATION_BASIS_MISMATCH", "proposal base is not the base generation member");
    }
    const generation = this.catalog.createPreview(baseGenerationId, {
      [proposal.targetRepository]: proposal.candidateCommit,
    });
    return {
      previewId: `preview-${generation.generationId}`,
      baseGenerationId,
      generation,
      candidate: { repositoryId: proposal.targetRepository, commitId: proposal.candidateCommit },
    };
  }

  /**
   * Run the protocol's structural checks on the preview generation, then record
   * the outcome. This is a real gate (repos mounted, commits exist). It is not
   * an external test suite — use recordValidation for those.
   */
  validateStructure(preview: PreviewGeneration): ValidationReport & { readonly check: ReturnType<Catalog["checkGeneration"]> } {
    const check = this.catalog.checkGeneration(preview.generation.generationId);
    const repo = this.store.repos.get(preview.candidate.repositoryId);
    const issues = [...check.issues];
    if (!repo || !repo.hasCommit(preview.candidate.commitId)) {
      issues.push({
        repository: preview.candidate.repositoryId,
        code: "VERSION_UNRESOLVED",
        message: `candidate ${preview.candidate.commitId} is unresolved`,
      });
    }
    const outcome = issues.length === 0 ? "PASSED" : "FAILED";
    return {
      ...this.recordValidation(preview, "structure", outcome),
      check: { ...check, outcome, issues },
    };
  }

  /** RECORD_VALIDATION: bind a caller-supplied outcome to this preview. Does not run a suite. */
  recordValidation(preview: PreviewGeneration, suiteRevision: string, outcome: "PASSED" | "FAILED"): ValidationReport {
    this.catalog.generation(preview.generation.generationId);
    return {
      reportId: `val-${preview.generation.generationId}-${suiteRevision}`,
      previewGenerationId: preview.generation.generationId,
      suiteRevision,
      outcome,
    };
  }

  /** MERGE: require a passing report for the exact complete preview, then ref CAS. */
  merge(proposal: Proposal, preview: PreviewGeneration, validation: ValidationReport): CommitIdentity {
    const repo = this.store.repos.get(proposal.targetRepository);
    if (!repo) throw new IngressError("TARGET_REPOSITORY_DENIED", `unknown repository ${proposal.targetRepository}`);

    const registeredPreview = this.catalog.generation(preview.generation.generationId);
    const registeredBase = this.catalog.generation(preview.baseGenerationId);
    if (
      preview.candidate.repositoryId !== proposal.targetRepository
      || preview.candidate.commitId !== proposal.candidateCommit
      || registeredPreview.repositories[proposal.targetRepository] !== proposal.candidateCommit
      || registeredBase.repositories[proposal.targetRepository] !== proposal.baseCommit
      || validation.previewGenerationId !== registeredPreview.generationId
    ) {
      throw new IngressError("VALIDATION_BASIS_MISMATCH", "validation is not bound to the exact preview generation");
    }
    if (validation.outcome !== "PASSED") {
      throw new IngressError("VALIDATION_BASIS_MISMATCH", "validation did not pass");
    }
    if (repo.getRef(proposal.candidateRef) !== proposal.candidateCommit) {
      throw new IngressError("CANDIDATE_MOVED", "candidate advanced after validation");
    }
    return repo.merge(proposal.targetRef, proposal.candidateCommit, proposal.baseCommit);
  }
}
