/**
 * LoomVfs is the framework-free core: a thin client over kc serve's
 * vfs-read/vfs-list/vfs-write verbs (docs/COMPOSITION.md's RawFileStore,
 * lifted to path routing over HTTP). It knows nothing about Cordis or
 * @deepseek-ai/dsh-fs — that adaptation lives in fs.ts, which is a thin
 * wrapper around this class so the HTTP/routing logic itself is unit
 * testable against a real `kc serve` process without any dsh dependency.
 */

export interface LoomVfsConfig {
  /** kc serve base URL, e.g. http://127.0.0.1:7380. No trailing slash. */
  baseURL: string;
  /** The Workspace this filesystem is a virtual tree of. */
  workspace: string;
  /** Optional --catalog when the home has more than one. */
  catalog?: string;
  /** Principal for X-Kc-As; omit to act as the home's owner. */
  as?: string;
  /** Gitea PAT for an authenticated kc serve. Mutually exclusive with as. */
  authToken?: string;
  /** Optional disposable host mirror used by shell-based tests and compilers. */
  materializeRoot?: string;
  /** Injectable for tests; defaults to the global fetch. */
  fetchImpl?: typeof fetch;
}

export interface LoomFileEntry {
  path: string;
  repository: string;
  commit: string;
}

export interface LoomMount {
  /** Workspace-relative mount point; empty string is the root mount. */
  path: string;
  repository: string;
  selector: string;
  subPath?: string;
  /** Commit chosen for this Workspace pin. */
  commit: string;
}

export interface LoomVfsListing {
  entries: LoomFileEntry[];
  mounts: LoomMount[];
}

export interface LoomFileRead {
  path: string;
  repository: string;
  commit: string;
  content: Uint8Array;
}

export interface LoomWriteResult {
  repositoryId: string;
  targetRef: string;
  oldCommit: string;
  newCommit: string;
}

export interface ResolvedWorkspace {
  workspaceId: string;
  revision: number;
  repositories: Record<string, string>;
  pinId?: string;
}

/** kc's {error:{code,message}} envelope, surfaced with the protocol's own
 * stable code (see kernel.ErrorCode) rather than an HTTP status alone. */
export class LoomError extends Error {
  readonly code: string;
  constructor(message: string, code: string) {
    super(message);
    this.name = 'LoomError';
    this.code = code;
  }
}

function randomId(): string {
  return `dsh-${cryptoRandomUUID()}`;
}

function cryptoRandomUUID(): string {
  const g = globalThis as { crypto?: { randomUUID?: () => string } };
  if (g.crypto?.randomUUID) return g.crypto.randomUUID();
  // Fallback for a runtime without globalThis.crypto (older Node without
  // --experimental-global-webcrypto); good enough for a client-chosen
  // idempotency key, not a security token.
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

export class LoomVfs {
  private readonly workspace: string;
  private readonly catalog?: string;
  private readonly as?: string;
  private readonly authToken?: string;
  private readonly baseURL: string;
  private readonly fetchImpl: typeof fetch;
  private pin?: ResolvedWorkspace;
  private pinPromise?: Promise<ResolvedWorkspace>;
  private readonly pathRepositories = new Map<string, string>();

  constructor(config: LoomVfsConfig) {
    this.workspace = config.workspace || '';
    if (!this.workspace) {
      throw new Error('dsh-loom: workspace is required');
    }
    this.catalog = config.catalog;
    this.as = config.as;
    this.authToken = config.authToken?.trim() || process.env.KC_AUTH_TOKEN?.trim() || undefined;
    if (this.as && this.authToken) {
      throw new Error('dsh-loom: as and authToken are mutually exclusive');
    }
    this.baseURL = config.baseURL.replace(/\/$/, '');
    this.fetchImpl = config.fetchImpl ?? fetch;
  }

  private async request(verb: string, body: Record<string, unknown>, pin?: ResolvedWorkspace): Promise<any> {
    const headers: Record<string, string> = { 'content-type': 'application/json' };
    if (this.authToken) headers.Authorization = `Bearer ${this.authToken}`;
    else if (this.as) headers['X-Kc-As'] = this.as;
    const payload: Record<string, unknown> = {
      workspace: this.workspace,
      ...body,
    };
    if (pin) payload.pin = pin;
    if (this.catalog) payload.catalog = this.catalog;
    const res = await this.fetchImpl(`${this.baseURL}/v1/${verb}`, {
      method: 'POST',
      headers,
      body: JSON.stringify(payload),
    });
    const json = await res.json().catch(() => ({}));
    if (!res.ok) {
      const err = (json as { error?: { code?: string; message?: string } })?.error ?? {};
      throw new LoomError(String(err.message ?? `${verb} failed with status ${res.status}`), String(err.code ?? 'UNKNOWN'));
    }
    return json;
  }

  private async ensurePin(): Promise<ResolvedWorkspace> {
    if (!this.pinPromise) {
      this.pinPromise = this.request('resolve', {}).then((json) => {
        const pin = json as ResolvedWorkspace;
        if (!pin || typeof pin !== 'object' || !pin.repositories) {
          throw new LoomError('resolve returned no Workspace pin', 'INVALID_RESPONSE');
        }
        this.pin = pin;
        return pin;
      });
    }
    return this.pinPromise;
  }

  private async call(verb: string, body: Record<string, unknown>): Promise<any> {
    const pin = await this.ensurePin();
    return this.request(verb, body, pin);
  }

  private async refreshRepository(repository: string): Promise<void> {
    const fresh = (await this.request('resolve', {})) as ResolvedWorkspace;
    const commit = fresh.repositories?.[repository];
    if (!commit || !this.pin) return;
    this.pin.repositories[repository] = commit;
    this.pin.pinId = undefined;
  }

  /** Every path across every mount, optionally filtered by prefix. A member
   * without RawFileStore is already left out server-side (docs/COMPOSITION.md). */
  async list(prefix?: string): Promise<LoomFileEntry[]> {
    return (await this.listing(prefix)).entries;
  }

  /** Files plus the declared mount boundaries that explain their routing.
   * Mounts are permission-filtered by kc serve and include empty mounts. */
  async listing(prefix?: string): Promise<LoomVfsListing> {
    const json = await this.call('vfs-list', prefix ? { prefix } : {});
    const entries = (json.entries ?? []) as LoomFileEntry[];
    for (const entry of entries) this.pathRepositories.set(entry.path, entry.repository);
    return { entries, mounts: (json.mounts ?? []) as LoomMount[] };
  }

  /** Raw bytes at a virtual path, routed to its owning mount. Throws
   * LoomError('KNOWLEDGE_REF_UNRESOLVED' | 'USAGE_INVALID' | ...) on failure. */
  async read(path: string): Promise<LoomFileRead> {
    const json = await this.call('vfs-read', { path });
    const file = {
      path: String(json.path),
      repository: String(json.repository),
      commit: String(json.commit),
      content: base64ToBytes(String(json.content)),
    };
    this.pathRepositories.set(file.path, file.repository);
    return file;
  }

  /** Writes content at path. base pins the CAS precondition to a commit the
   * caller already observed (read() or list() return one); omit only for a
   * first write nobody else can be racing. commandId defaults to a fresh
   * random one per call — this client does not attempt cross-call replay,
   * only the CAS (base) protects against a lost update. */
  async write(
    path: string,
    content: Uint8Array,
    opts: { base?: string; commandId?: string; message?: string; ref?: string } = {},
  ): Promise<LoomWriteResult> {
    try {
      const json = await this.call('vfs-write', {
        path,
        content: bytesToBase64(content),
        'command-id': opts.commandId ?? randomId(),
        base: opts.base,
        message: opts.message,
        ref: opts.ref,
      });
      const result = json.result as LoomWriteResult;
      if (this.pin) {
        this.pin.repositories[result.repositoryId] = result.newCommit;
        this.pin.pinId = undefined;
      }
      this.pathRepositories.set(path, result.repositoryId);
      return result;
    } catch (error) {
      if (error instanceof LoomError && error.code === 'NON_FAST_FORWARD') {
        const repository = this.pathRepositories.get(path);
        if (repository) await this.refreshRepository(repository);
      }
      throw error;
    }
  }

  /** Removes path. Same CAS/commandId contract as write(). */
  async remove(path: string, opts: { base?: string; commandId?: string; message?: string; ref?: string } = {}): Promise<LoomWriteResult> {
    try {
      const json = await this.call('vfs-write', {
        path,
        remove: true,
        'command-id': opts.commandId ?? randomId(),
        base: opts.base,
        message: opts.message,
        ref: opts.ref,
      });
      const result = json.result as LoomWriteResult;
      if (this.pin) {
        this.pin.repositories[result.repositoryId] = result.newCommit;
        this.pin.pinId = undefined;
      }
      this.pathRepositories.delete(path);
      return result;
    } catch (error) {
      if (error instanceof LoomError && error.code === 'NON_FAST_FORWARD') {
        const repository = this.pathRepositories.get(path);
        if (repository) await this.refreshRepository(repository);
      }
      throw error;
    }
  }
}

export function base64ToBytes(encoded: string): Uint8Array {
  return Uint8Array.from(Buffer.from(encoded, 'base64'));
}

export function bytesToBase64(bytes: Uint8Array): string {
  return Buffer.from(bytes).toString('base64');
}
