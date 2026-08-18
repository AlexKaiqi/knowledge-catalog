/**
 * Store — holds repositories (all implementing the ONE Repository contract).
 * Snapshot versioning is real git; append streams live inside each repo.
 */

import type { Repository, RepositoryIdentity } from "./contracts/index.ts";
import { IngressError } from "./contracts/errors.ts";

export class Store {
  readonly repos = new Map<RepositoryIdentity, Repository>();

  addRepository(repo: Repository): void {
    if (this.repos.has(repo.repositoryId)) {
      throw new IngressError("PRECONDITION_FAILED", `repository ${repo.repositoryId} is already registered`);
    }
    this.repos.set(repo.repositoryId, repo);
  }
}
