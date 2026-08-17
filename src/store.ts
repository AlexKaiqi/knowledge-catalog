/**
 * Store — holds repositories (all implementing the ONE Repository contract).
 * Snapshot versioning is real git; append streams live inside each repo.
 */

import type { Repository, RepositoryIdentity } from "./contracts/index.ts";

export class Store {
  readonly repos = new Map<RepositoryIdentity, Repository>();

  addRepository(repo: Repository): void {
    this.repos.set(repo.repositoryId, repo);
  }
}
