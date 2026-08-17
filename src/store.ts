/**
 * MemoryStore — in-memory holder for repositories and append streams.
 * Shared by Ingress (write boundary) and Access (read boundary).
 */

import type { RepositoryIdentity } from "./contracts/index.ts";
import { MemoryRepository } from "./adapters/memory/repository.ts";
import { MemoryAppendStream } from "./adapters/memory/append.ts";

export class MemoryStore {
  readonly repos = new Map<RepositoryIdentity, MemoryRepository>();
  readonly streams = new Map<string, MemoryAppendStream>();

  addRepository(repo: MemoryRepository): void {
    this.repos.set(repo.repositoryId, repo);
  }

  stream(repositoryId: RepositoryIdentity, streamRef: string): MemoryAppendStream {
    const key = `${repositoryId}::${streamRef}`;
    let s = this.streams.get(key);
    if (!s) {
      s = new MemoryAppendStream(streamRef);
      this.streams.set(key, s);
    }
    return s;
  }
}
