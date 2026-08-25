/**
 * LoomFileSystem composes the Agent's normal local project with a remote Loom
 * Workspace mounted at one synthetic directory. Local targets delegate to
 * dsh-fs-local; remote targets use kc serve vfs-* and are never materialized.
 * Legacy standalone mode (empty mountPath) still exposes only the Workspace
 * tree for the existing write-path acceptance suite.
 *
 * Only ctx.fs is provided here (see cordis.patch.yml): ctx.shell stays the
 * host's own. It can open local targets, but not the synthetic remote mount;
 * remote `processPath`/`fileUrl` values therefore remain honest loom://
 * placeholders rather than fake host paths.
 */

import { Context } from '@deepseek-ai/cordis';
import { LocalFileSystem } from '@deepseek-ai/dsh-fs-local';
import { writableRoots, type SandboxExecutionPolicy, type SandboxMode } from '@deepseek-ai/dsh-sandbox';
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
import { readWorkspaceBinding } from './binding.js';
import { toFsError, isMissing } from './errors.js';
import { vfsForTask } from './session-vfs.js';
import { applyLiteralEdit, decodeStrictText } from './text.js';
import { directChildren, directoryExists, joinPath, normalizePath } from './tree.js';

export type LoomFsConfig = LoomVfsConfig & {
  home?: string;
  /**
   * Mount the remote Workspace below this single project-directory entry.
   * Empty keeps the historical standalone virtual Workspace mode.
   */
  mountPath?: string;
};

/** Cordis plugin name used by loader diagnostics. */
export const name = 'loom-fs';

interface CompositeTarget extends FsTarget {
  loomBackend: 'local' | 'remote';
  logicalPath: string;
  localTarget?: FsTarget;
  loomPath?: string;
  loomVfs?: LoomVfs;
}

function normalizeMountPath(raw: string | undefined): string | undefined {
  if (raw === undefined || raw === '') return undefined;
  const normalized = raw.replace(/^\/+|\/+$/g, '');
  if (!normalized || normalized === '.' || normalized === '..' || normalized.includes('/') || normalized.includes('\\')) {
    throw new Error('dsh-loom: mountPath must be one project-relative directory name');
  }
  return normalized;
}

function logicalPath(requested: string, cwd?: string): string {
  const root = cwd && path.isAbsolute(cwd) ? path.resolve(cwd) : undefined;
  if (root && path.isAbsolute(requested)) {
    const absolute = path.resolve(requested);
    if (absolute === root) return '';
    const prefix = `${root}${path.sep}`;
    if (absolute.startsWith(prefix)) return normalizePath(absolute.slice(prefix.length));
  }
  // In overlay mode, leading slash means the Agent project's virtual root,
  // not the host machine's filesystem root.
  return normalizePath(requested.replace(/^[/\\]+/, ''));
}

function assertNotAborted(signal: AbortSignal | undefined, operation: string): void {
  if (signal?.aborted) {
    throw new DOMException(`${operation} aborted`, 'AbortError');
  }
}

export default class LoomFileSystem extends FileSystem {
  static inject = ['sandboxPolicy'];
  private readonly config: LoomFsConfig;
  private readonly staticVfs?: LoomVfs;
  private readonly localFs?: LocalFileSystem;
  private readonly mountPath?: string;
  private readonly defaultSandboxMode?: SandboxMode;
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
    this.config = config;
    this.mountPath = normalizeMountPath(config.mountPath);
    // LocalFileSystem is used as an internal mechanics delegate. Give it an
    // isolated Cordis context so it does not try to register a second public
    // `ctx.fs` service beside this composite provider.
    if (this.mountPath) this.localFs = new LocalFileSystem(new Context(), { diffBasisMaxBytes: 10 * 1024 * 1024 });
    this.defaultSandboxMode = (ctx as Context & { sandboxPolicy?: { defaultMode?: SandboxMode } }).sandboxPolicy?.defaultMode;
    if (config.workspace) this.staticVfs = new LoomVfs({
      baseURL: config.baseURL || 'http://127.0.0.1:7380',
      workspace: config.workspace || 'notes',
      catalog: config.catalog || undefined,
      as: config.as || undefined,
      authToken: config.authToken || undefined,
      fetchImpl: config.fetchImpl,
    });
    this.mirrorRoot = config.materializeRoot ? path.resolve(config.materializeRoot) : undefined;
  }

  get sandboxMode(): SandboxMode | undefined {
    return this.defaultSandboxMode;
  }

  private isRemote(logical: string): boolean {
    return this.mountPath !== undefined && (logical === this.mountPath || logical.startsWith(`${this.mountPath}/`));
  }

  private remotePath(logical: string): string {
    if (!this.mountPath) return logical;
    if (logical === this.mountPath) return '';
    return logical.slice(this.mountPath.length + 1);
  }

  private remoteDisplay(loomPath: string): string {
    return this.mountPath ? joinPath(this.mountPath, loomPath) : loomPath;
  }

  private asCompositeLocal(target: FsTarget, logical: string, vfs?: LoomVfs): CompositeTarget {
    return {
      targetKey: FsTargetKey(`local:${String(target.targetKey)}`),
      displayPath: logical || '.',
      loomBackend: 'local',
      logicalPath: logical,
      localTarget: target,
      loomVfs: vfs,
    };
  }

  private asCompositeRemote(loomPath: string, vfs: LoomVfs): CompositeTarget {
    const displayPath = this.remoteDisplay(loomPath);
    return {
      targetKey: FsTargetKey(`loom:${displayPath}`),
      displayPath: displayPath || '/',
      loomBackend: 'remote',
      logicalPath: displayPath,
      loomPath,
      loomVfs: vfs,
    };
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
    if (this.mountPath) return;
    if (!this.mirrorRoot) return;
    if (!this.staticVfs) throw new FsError('choose a Catalog Workspace before using files', 'FS_SANDBOX_DENIED');
    if (!this.materialized) {
      this.materialized = (async () => {
        const entries = await this.staticVfs!.list();
        await mkdir(this.mirrorRoot!, { recursive: true });
        for (const entry of entries) {
          const file = await this.staticVfs!.read(entry.path);
          await this.mirrorBytes(entry.path, file.content);
        }
      })();
    }
    await this.materialized;
  }

  private async vfsForCwd(cwd?: string, signal?: AbortSignal): Promise<LoomVfs> {
    const binding = await readWorkspaceBinding(cwd);
    if (binding) return vfsForTask(this.config, binding, signal);
    if (this.config.workspace) return signal
      ? vfsForTask(this.config, { workspace: this.config.workspace, catalog: this.config.catalog }, signal)
      : this.staticVfs!;
    throw new FsError('choose or create a Catalog Workspace before starting the Agent', 'FS_SANDBOX_DENIED');
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
    if (this.mountPath) {
      const logical = logicalPath(path, opts?.cwd);
      if (this.isRemote(logical)) {
        const collisionTarget = await this.localFs!.resolve(this.mountPath!, { cwd: opts?.cwd, signal: opts?.signal });
        if (await this.localFs!.stat(collisionTarget, opts?.signal)) {
          throw new FsError(
            `cannot mount remote knowledge at "${this.mountPath}": the project already contains that path`,
            'FS_SANDBOX_DENIED',
          );
        }
        return this.asCompositeRemote(
          this.remotePath(logical),
          await this.vfsForCwd(opts?.cwd, opts?.signal),
        );
      }
      const local = await this.localFs!.resolve(logical || '.', { cwd: opts?.cwd, signal: opts?.signal });
      return this.asCompositeLocal(local, logical, await this.vfsForCwd(opts?.cwd, opts?.signal));
    }
    const normalized = normalizePath(path, opts?.cwd);
    const target = { targetKey: FsTargetKey(normalized), displayPath: normalized === '' ? '/' : normalized } as FsTarget & { loomVfs?: LoomVfs };
    target.loomVfs = await this.vfsForCwd(opts?.cwd, opts?.signal);
    return target;
  }

  private vfs(target: FsTarget): LoomVfs {
    const vfs = (target as FsTarget & { loomVfs?: LoomVfs }).loomVfs;
    if (!vfs) throw new FsError('Catalog Workspace binding is missing', 'FS_SANDBOX_DENIED');
    return vfs;
  }

  private composite(target: FsTarget): CompositeTarget | undefined {
    const candidate = target as Partial<CompositeTarget>;
    return candidate.loomBackend === 'local' || candidate.loomBackend === 'remote'
      ? candidate as CompositeTarget
      : undefined;
  }

  private localTarget(target: FsTarget): FsTarget {
    const composite = this.composite(target);
    if (!composite?.localTarget) throw new FsError('local project target is missing', 'FS_NOT_FOUND');
    return composite.localTarget;
  }

  private targetPath(target: FsTarget): string {
    return this.composite(target)?.loomPath ?? String(target.targetKey);
  }

  // Remote targets have no openable host path. Local targets delegate to the
  // native provider; standalone acceptance may explicitly configure a mirror.
  processPath(target: FsTarget): string {
    const composite = this.composite(target);
    if (composite?.loomBackend === 'local') return this.localFs!.processPath(this.localTarget(target));
    if (composite?.loomBackend === 'remote') return `loom://${composite.loomPath ?? ''}`;
    const mirrored = this.mirrorPath(String(target.targetKey));
    if (mirrored) return mirrored;
    return `loom://${String(target.targetKey)}`;
  }

  fileUrl(target: FsTarget): string {
    const composite = this.composite(target);
    if (composite?.loomBackend === 'local') return this.localFs!.fileUrl(this.localTarget(target));
    const workspacePath = composite?.loomPath ?? String(target.targetKey);
    const encoded = workspacePath.split('/').map(encodeURIComponent).join('/');
    return `file://loom/${encoded}`;
  }

  contains(parent: FsTarget, child: FsTarget): boolean {
    const compositeParent = this.composite(parent);
    const compositeChild = this.composite(child);
    if (compositeParent && compositeChild) {
      if (compositeParent.logicalPath === '') return true;
      if (compositeParent.loomBackend !== compositeChild.loomBackend) return false;
      if (compositeParent.loomBackend === 'local') {
        return this.localFs!.contains(this.localTarget(parent), this.localTarget(child));
      }
      return compositeChild.logicalPath === compositeParent.logicalPath
        || compositeChild.logicalPath.startsWith(`${compositeParent.logicalPath}/`);
    }
    const parentKey = String(parent.targetKey);
    const childKey = String(child.targetKey);
    if (parentKey === childKey) return true;
    const prefix = parentKey === '' ? '' : `${parentKey}/`;
    return childKey.startsWith(prefix);
  }

  async stat(target: FsTarget, signal?: AbortSignal): Promise<FsInfo | undefined> {
    assertNotAborted(signal, 'stat');
    const composite = this.composite(target);
    if (composite?.loomBackend === 'local') {
      const info = await this.localFs!.stat(this.localTarget(target), signal);
      if (info) return info;
      if (composite.logicalPath === '') return { version: FsVersion('project-root'), type: 'directory' };
      return undefined;
    }
    const workspacePath = this.targetPath(target);
    if (workspacePath !== '') {
      try {
        const file = await this.vfs(target).read(workspacePath);
        return { version: FsVersion(file.commit), type: 'file', size: file.content.byteLength };
      } catch (err) {
        if (!isMissing(err)) throw toFsError(err, 'stat', target.displayPath);
      }
    }
    let entries;
    try {
      entries = await this.vfs(target).list(workspacePath === '' ? undefined : `${workspacePath}/`);
    } catch (err) {
      throw toFsError(err, 'stat', target.displayPath);
    }
    if (workspacePath === '' || directoryExists(entries.map((e) => e.path), workspacePath)) {
      return { version: FsVersion(`dir:${entries.length}`), type: 'directory' };
    }
    return undefined;
  }

  async lstat(path: string, opts?: { cwd?: string }, signal?: AbortSignal): Promise<FsPathInfo | undefined> {
    assertNotAborted(signal, 'lstat');
    const target = await this.resolve(path, { ...opts, signal });
    const composite = this.composite(target);
    if (composite?.loomBackend === 'local') {
      return this.localFs!.lstat(composite.logicalPath || '.', { cwd: opts?.cwd }, signal);
    }
    const info = await this.stat(target, signal);
    return info; // no symlinks in a git-shaped tree; type is already 'file' | 'directory'
  }

  async readText(target: FsTarget, signal?: AbortSignal): Promise<string> {
    assertNotAborted(signal, 'read');
    const composite = this.composite(target);
    if (composite?.loomBackend === 'local') return this.localFs!.readText(this.localTarget(target), signal);
    const workspacePath = this.targetPath(target);
    let file;
    try {
      file = await this.vfs(target).read(workspacePath);
    } catch (err) {
      throw toFsError(err, 'read', target.displayPath);
    }
    try {
      await this.mirrorBytes(workspacePath, file.content);
      return decodeStrictText(file.content, target.displayPath);
    } catch (err) {
      throw toFsError(err, 'read', target.displayPath);
    }
  }

  async streamText(target: FsTarget, signal?: AbortSignal): Promise<AsyncIterable<string>> {
    const composite = this.composite(target);
    if (composite?.loomBackend === 'local') return this.localFs!.streamText(this.localTarget(target), signal);
    const text = await this.readText(target, signal);
    return (async function* () {
      yield text;
    })();
  }

  async readBytes(target: FsTarget, signal: AbortSignal | undefined, maxBytes: number): Promise<Uint8Array> {
    assertNotAborted(signal, 'read');
    const composite = this.composite(target);
    if (composite?.loomBackend === 'local') return this.localFs!.readBytes(this.localTarget(target), signal, maxBytes);
    const workspacePath = this.targetPath(target);
    let file;
    try {
      file = await this.vfs(target).read(workspacePath);
    } catch (err) {
      throw toFsError(err, 'read', target.displayPath);
    }
    if (file.content.byteLength > maxBytes) {
      throw new FsError(
        `cannot read "${target.displayPath}": ${file.content.byteLength} bytes exceeds cap ${maxBytes}`,
        'FS_TOO_LARGE' as never,
      );
    }
    await this.mirrorBytes(workspacePath, file.content);
    return file.content;
  }

  async listDir(target: FsTarget, signal?: AbortSignal): Promise<FsDirEntry[]> {
    assertNotAborted(signal, 'list');
    const composite = this.composite(target);
    if (composite?.loomBackend === 'local') {
      const localEntries = await this.localFs!.listDir(this.localTarget(target), signal);
      const wrapped = localEntries.map((entry) => ({
        ...entry,
        target: this.asCompositeLocal(
          entry.target,
          joinPath(composite.logicalPath, entry.name),
          composite.loomVfs,
        ),
      }));
      if (composite.logicalPath !== '' || !this.mountPath) return wrapped;
      const collision = wrapped.find((entry) => entry.name === this.mountPath);
      if (collision) {
        throw new FsError(
          `cannot mount remote knowledge at "${this.mountPath}": the project already contains that path`,
          'FS_SANDBOX_DENIED',
        );
      }
      return [...wrapped, {
        name: this.mountPath,
        type: 'directory' as const,
        target: this.asCompositeRemote('', this.vfs(target)),
      }].sort((left, right) => left.name.localeCompare(right.name));
    }
    const workspacePath = this.targetPath(target);
    let entries;
    try {
      entries = await this.vfs(target).list(workspacePath === '' ? undefined : `${workspacePath}/`);
    } catch (err) {
      throw toFsError(err, 'list', target.displayPath);
    }
    const paths = entries.map((e) => e.path);
    if (workspacePath !== '' && !directoryExists(paths, workspacePath)) {
      throw new FsError(`cannot list "${target.displayPath}": no such directory`, 'FS_NOT_FOUND');
    }
    const children = directChildren(paths, workspacePath);
    return children.map((child) => {
      const childPath = joinPath(workspacePath, child.name);
      if (composite?.loomBackend === 'remote') {
        return {
          name: child.name,
          type: child.type,
          target: this.asCompositeRemote(childPath, this.vfs(target)),
        };
      }
      return {
        name: child.name,
        type: child.type,
        target: Object.assign({ targetKey: FsTargetKey(childPath), displayPath: childPath }, { loomVfs: this.vfs(target) }),
      };
    });
  }

  async writeText(
    target: FsTarget,
    content: string,
    expected?: FsWriteIntent,
    signal?: AbortSignal,
    sandboxPolicy?: SandboxExecutionPolicy,
  ): Promise<FsWriteOutcome> {
    const composite = this.composite(target);
    if (composite?.loomBackend === 'local') {
      const checked = await this.checkedLocalTarget(this.localTarget(target), sandboxPolicy);
      return this.localFs!.writeText(checked, content, expected, signal);
    }
    if (composite?.loomBackend === 'remote') {
      throw new FsError(
        `cannot write "${target.displayPath}": mounted remote knowledge is read-only`,
        'FS_SANDBOX_DENIED',
      );
    }
    return this.withLock(String(target.targetKey), async () => {
      assertNotAborted(signal, 'write');
      const workspacePath = this.targetPath(target);

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
        result = await this.vfs(target).write(workspacePath, new TextEncoder().encode(content), { base });
      } catch (err) {
        throw toFsError(err, 'write', target.displayPath);
      }
      await this.mirrorBytes(workspacePath, new TextEncoder().encode(content));
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
    sandboxPolicy?: SandboxExecutionPolicy,
  ): Promise<FsEditOutcome> {
    const composite = this.composite(target);
    if (composite?.loomBackend === 'local') {
      const checked = await this.checkedLocalTarget(this.localTarget(target), sandboxPolicy);
      return this.localFs!.editText(checked, edit, expected, signal);
    }
    if (composite?.loomBackend === 'remote') {
      throw new FsError(
        `cannot edit "${target.displayPath}": mounted remote knowledge is read-only`,
        'FS_SANDBOX_DENIED',
      );
    }
    return this.withLock(String(target.targetKey), async () => {
      assertNotAborted(signal, 'edit');
      const workspacePath = this.targetPath(target);

      let file;
      try {
        file = await this.vfs(target).read(workspacePath);
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
        result = await this.vfs(target).write(workspacePath, new TextEncoder().encode(edited), { base: file.commit });
      } catch (err) {
        throw toFsError(err, 'edit', target.displayPath);
      }
      await this.mirrorBytes(workspacePath, new TextEncoder().encode(edited));
      return { version: FsVersion(result.newCommit), before: original, after: edited };
    });
  }

  private async checkedLocalTarget(
    target: FsTarget,
    policy: SandboxExecutionPolicy | undefined,
  ): Promise<FsTarget> {
    if (!policy || policy.mode === 'danger-full-access') return target;
    if (policy.mode === 'read-only') {
      throw new FsError(`cannot write "${target.displayPath}": file access denied under read-only mode`, 'FS_SANDBOX_DENIED');
    }
    const fresh = await this.localFs!.resolve(target.displayPath);
    for (const root of writableRoots(policy)) {
      const rootTarget = await this.localFs!.resolve(root);
      if (this.localFs!.contains(rootTarget, fresh)) return fresh;
    }
    throw new FsError(`cannot write "${target.displayPath}": file access denied under workspace-write mode`, 'FS_SANDBOX_DENIED');
  }
}
