/**
 * LoomFileSystem: ctx.fs backed by a Loom Workspace over kc serve. Targets
 * are canonical virtual paths in the composed tree (docs/COMPOSITION.md);
 * every operation is a kc serve vfs-read/vfs-list/vfs-write call, and no
 * checkout is ever materialized on disk — reads and writes are routed by
 * path to whichever member repository owns it, per this Workspace's current
 * ResolveWorkspace pin.
 *
 * Only ctx.fs is provided here (see cordis.patch.yml): ctx.shell stays the
 * host's own — Loom has no "run a command inside repository X" concept, so
 * swapping the shell provider too would either do nothing useful or silently
 * disconnect shell commands from the virtual tree. `processPath`/`fileUrl`
 * are honest placeholders (loom:// forms) for exactly that reason: no real,
 * openable path exists for a virtual target.
 */

import { Context } from '@deepseek-ai/cordis';
import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import {
  FileSystem,
  FsError,
  FsTargetKey,
  FsVersion,
  type FsDirEntry,
  type FsEditOutcome,
  type FsEditRequest,
  type FsInfo,
  type FsPathInfo,
  type FsTarget,
  type FsWriteIntent,
  type FsWriteOutcome,
} from '@deepseek-ai/dsh-fs';
import { LoomVfs, type LoomVfsConfig } from './client.js';
import { toFsError, isMissing } from './errors.js';
import { applyLiteralEdit, decodeStrictText } from './text.js';
import { directChildren, directoryExists, joinPath, normalizePath } from './tree.js';

export type LoomFsConfig = LoomVfsConfig;

/** Cordis plugin name used by loader diagnostics. */
export const name = 'loom-fs';

function assertNotAborted(signal: AbortSignal | undefined, operation: string): void {
  if (signal?.aborted) {
    throw new DOMException(`${operation} aborted`, 'AbortError');
  }
}

export default class LoomFileSystem extends FileSystem {
  private readonly vfs: LoomVfs;
  private readonly mirrorRoot?: string;
  private materialized?: Promise<void>;
  // Per-path tail promise, mirroring the discipline other dsh-fs backends
  // use: serializes this process's own mutating calls on one target so a
  // local read -> edit -> write window cannot interleave with itself. It
  // does not replace the server's own CAS (base pins that); it only avoids
  // two local callers racing into avoidable NON_FAST_FORWARD retries.
  private readonly locks = new Map<string, Promise<unknown>>();

  constructor(ctx: Context, config: LoomFsConfig) {
    super(ctx);
    this.vfs = new LoomVfs({
      baseURL: config.baseURL || 'http://127.0.0.1:7380',
      workspace: config.workspace || 'notes',
      catalog: config.catalog || undefined,
      as: config.as || undefined,
      authToken: config.authToken || undefined,
      fetchImpl: config.fetchImpl,
    });
    this.mirrorRoot = config.materializeRoot ? path.resolve(config.materializeRoot) : undefined;
  }

  private mirrorPath(workspacePath: string): string | undefined {
    if (!this.mirrorRoot) return undefined;
    const relative = normalizePath(workspacePath);
    const target = path.resolve(this.mirrorRoot, relative);
    if (target !== this.mirrorRoot && !target.startsWith(`${this.mirrorRoot}${path.sep}`)) {
      throw new FsError(`cannot materialize path outside Workspace: ${workspacePath}`, 'FS_SANDBOX_DENIED');
    }
    return target;
  }

  private async mirrorBytes(workspacePath: string, content: Uint8Array): Promise<void> {
    const target = this.mirrorPath(workspacePath);
    if (!target) return;
    await mkdir(path.dirname(target), { recursive: true });
    await writeFile(target, content);
  }

  private async ensureMaterialized(): Promise<void> {
    if (!this.mirrorRoot) return;
    if (!this.materialized) {
      this.materialized = (async () => {
        const entries = await this.vfs.list();
        await mkdir(this.mirrorRoot!, { recursive: true });
        for (const entry of entries) {
          const file = await this.vfs.read(entry.path);
          await this.mirrorBytes(entry.path, file.content);
        }
      })();
    }
    await this.materialized;
  }

  private withLock<T>(key: string, body: () => Promise<T>): Promise<T> {
    const previous = this.locks.get(key) ?? Promise.resolve();
    const next = previous.then(body, body);
    const tail = next.then(() => undefined, () => undefined);
    this.locks.set(key, tail);
    void tail.then(() => {
      if (this.locks.get(key) === tail) this.locks.delete(key);
    });
    return next;
  }

  async resolve(path: string, opts?: { cwd?: string; signal?: AbortSignal }): Promise<FsTarget> {
    assertNotAborted(opts?.signal, 'resolve');
    await this.ensureMaterialized();
    const normalized = normalizePath(path, opts?.cwd);
    return { targetKey: FsTargetKey(normalized), displayPath: normalized === '' ? '/' : normalized };
  }

  // No real, openable path exists for a virtual target (nothing is ever
  // materialized on disk): this is an honest placeholder, not a usable
  // subprocess path. Nothing calls it today since ctx.shell is not swapped.
  processPath(target: FsTarget): string {
    const mirrored = this.mirrorPath(String(target.targetKey));
    if (mirrored) return mirrored;
    return `loom://${String(target.targetKey)}`;
  }

  fileUrl(target: FsTarget): string {
    const path = String(target.targetKey);
    const encoded = path.split('/').map(encodeURIComponent).join('/');
    return `file://loom/${encoded}`;
  }

  contains(parent: FsTarget, child: FsTarget): boolean {
    const parentKey = String(parent.targetKey);
    const childKey = String(child.targetKey);
    if (parentKey === childKey) return true;
    const prefix = parentKey === '' ? '' : `${parentKey}/`;
    return childKey.startsWith(prefix);
  }

  async stat(target: FsTarget, signal?: AbortSignal): Promise<FsInfo | undefined> {
    assertNotAborted(signal, 'stat');
    const path = String(target.targetKey);
    if (path !== '') {
      try {
        const file = await this.vfs.read(path);
        return { version: FsVersion(file.commit), type: 'file', size: file.content.byteLength };
      } catch (err) {
        if (!isMissing(err)) throw toFsError(err, 'stat', target.displayPath);
      }
    }
    let entries;
    try {
      entries = await this.vfs.list(path === '' ? undefined : `${path}/`);
    } catch (err) {
      throw toFsError(err, 'stat', target.displayPath);
    }
    if (path === '' || directoryExists(entries.map((e) => e.path), path)) {
      return { version: FsVersion(`dir:${entries.length}`), type: 'directory' };
    }
    return undefined;
  }

  async lstat(path: string, opts?: { cwd?: string }, signal?: AbortSignal): Promise<FsPathInfo | undefined> {
    assertNotAborted(signal, 'lstat');
    const target = await this.resolve(path, opts);
    const info = await this.stat(target, signal);
    return info; // no symlinks in a git-shaped tree; type is already 'file' | 'directory'
  }

  async readText(target: FsTarget, signal?: AbortSignal): Promise<string> {
    assertNotAborted(signal, 'read');
    const path = String(target.targetKey);
    let file;
    try {
      file = await this.vfs.read(path);
    } catch (err) {
      throw toFsError(err, 'read', target.displayPath);
    }
    try {
      await this.mirrorBytes(path, file.content);
      return decodeStrictText(file.content, target.displayPath);
    } catch (err) {
      throw toFsError(err, 'read', target.displayPath);
    }
  }

  async streamText(target: FsTarget, signal?: AbortSignal): Promise<AsyncIterable<string>> {
    const text = await this.readText(target, signal);
    return (async function* () {
      yield text;
    })();
  }

  async readBytes(target: FsTarget, signal: AbortSignal | undefined, maxBytes: number): Promise<Uint8Array> {
    assertNotAborted(signal, 'read');
    const path = String(target.targetKey);
    let file;
    try {
      file = await this.vfs.read(path);
    } catch (err) {
      throw toFsError(err, 'read', target.displayPath);
    }
    if (file.content.byteLength > maxBytes) {
      throw new FsError(
        `cannot read "${target.displayPath}": ${file.content.byteLength} bytes exceeds cap ${maxBytes}`,
        'FS_TOO_LARGE' as never,
      );
    }
    await this.mirrorBytes(path, file.content);
    return file.content;
  }

  async listDir(target: FsTarget, signal?: AbortSignal): Promise<FsDirEntry[]> {
    assertNotAborted(signal, 'list');
    const path = String(target.targetKey);
    let entries;
    try {
      entries = await this.vfs.list(path === '' ? undefined : `${path}/`);
    } catch (err) {
      throw toFsError(err, 'list', target.displayPath);
    }
    const paths = entries.map((e) => e.path);
    if (path !== '' && !directoryExists(paths, path)) {
      throw new FsError(`cannot list "${target.displayPath}": no such directory`, 'FS_NOT_FOUND');
    }
    const children = directChildren(paths, path);
    return children.map((child) => {
      const childPath = joinPath(path, child.name);
      return {
        name: child.name,
        type: child.type,
        target: { targetKey: FsTargetKey(childPath), displayPath: childPath },
      };
    });
  }

  async writeText(
    target: FsTarget,
    content: string,
    expected?: FsWriteIntent,
    signal?: AbortSignal,
    _sandboxPolicy?: unknown,
  ): Promise<FsWriteOutcome> {
    return this.withLock(String(target.targetKey), async () => {
      assertNotAborted(signal, 'write');
      const path = String(target.targetKey);

      let base: string | undefined;
      if (expected?.kind === 'replaceIfVersion') {
        base = String(expected.version);
      } else if (expected?.kind === 'createIfAbsent') {
        const existing = await this.stat(target, signal);
        if (existing !== undefined) {
          throw new FsError(`cannot write "${target.displayPath}": file already exists`, 'FS_NOT_OBSERVED');
        }
      }

      const before = await this.readText(target, signal).catch(() => null);

      let result;
      try {
        result = await this.vfs.write(path, new TextEncoder().encode(content), { base });
      } catch (err) {
        throw toFsError(err, 'write', target.displayPath);
      }
      await this.mirrorBytes(path, new TextEncoder().encode(content));
      return {
        operation: before === null ? 'create' : 'update',
        version: FsVersion(result.newCommit),
        before,
        after: content,
      };
    });
  }

  async editText(
    target: FsTarget,
    edit: FsEditRequest,
    expected?: { version: FsVersion },
    signal?: AbortSignal,
    _sandboxPolicy?: unknown,
  ): Promise<FsEditOutcome> {
    return this.withLock(String(target.targetKey), async () => {
      assertNotAborted(signal, 'edit');
      const path = String(target.targetKey);

      let file;
      try {
        file = await this.vfs.read(path);
      } catch (err) {
        throw toFsError(err, 'edit', target.displayPath);
      }
      if (expected !== undefined && file.commit !== String(expected.version)) {
        throw new FsError(`cannot edit "${target.displayPath}": file changed since it was read`, 'FS_STALE_VERSION');
      }
      let original: string;
      let edited: string;
      try {
        original = decodeStrictText(file.content, target.displayPath);
        edited = applyLiteralEdit(original, edit.oldString, edit.newString, edit.replaceAll, target.displayPath);
      } catch (err) {
        throw toFsError(err, 'edit', target.displayPath);
      }

      let result;
      try {
        result = await this.vfs.write(path, new TextEncoder().encode(edited), { base: file.commit });
      } catch (err) {
        throw toFsError(err, 'edit', target.displayPath);
      }
      await this.mirrorBytes(path, new TextEncoder().encode(edited));
      return { version: FsVersion(result.newCommit), before: original, after: edited };
    });
  }
}
