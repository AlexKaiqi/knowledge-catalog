import type { Context } from '@deepseek-ai/cordis';
import { execFileSync } from 'node:child_process';
import { mkdirSync, rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';

export interface MountControllerConfig {
  home?: string;
  bin?: string;
  server?: string;
  catalog?: string;
  workspace?: string;
  principal?: string;
}

interface SessionLike {
  id: string;
  header: { cwd?: string; parentSession?: string };
}

interface MountManifest {
  workspaceId: string;
  pinId: string;
  pin: unknown;
  root: string;
  readOnly: true;
  pid: number;
  mounts: Array<{ path: string; mountpoint: string; repository: string; commit: string }>;
}

interface ActiveMount {
  manifest: MountManifest;
  sessions: Set<string>;
}

function requiredAbsolute(value: string | undefined, name: string, hint?: string): string {
  const resolved = value?.trim();
  const recovery = hint ? ` ${hint}` : '';
  if (!resolved) throw new Error(`dsh-loom: ${name} is required.${recovery}`);
  if (!path.isAbsolute(resolved)) throw new Error(`dsh-loom: ${name} must be absolute.${recovery}`);
  return path.resolve(resolved);
}

function mountFailure(error: unknown, bin: string): Error {
  const failure = error as { code?: string; stderr?: Buffer | string };
  if (failure.code === 'ENOENT') {
    return new Error(`dsh-loom: cannot start ${bin}. Install kcfs or set KCFS_BIN to its absolute path, then reopen the task.`);
  }
  const stderr = failure.stderr ? String(failure.stderr).trim() : '';
  const detail = stderr || (error instanceof Error ? error.message : String(error));
  return new Error(`dsh-loom: knowledge mount failed: ${detail}. Run "kcfs plan" with the same Workspace and project root to inspect the configuration.`);
}

export class MountController {
  private readonly home: string;
  private readonly bin: string;
  private readonly server: string;
  private readonly catalog?: string;
  private readonly workspace: string;
  private readonly principal: string;
  private readonly byRoot = new Map<string, ActiveMount>();
  private readonly bySession = new Map<string, ActiveMount>();

  constructor(config: MountControllerConfig) {
    this.home = requiredAbsolute(
      config.home?.trim() || process.env.KC_HOME,
      'KC_HOME',
      'Set it to an absolute private state directory, for example /var/lib/kc.',
    );
    this.bin = config.bin?.trim() || process.env.KCFS_BIN?.trim() || 'kcfs';
    this.server = config.server?.trim() || process.env.KC_SERVER_URL?.trim() || '';
    this.catalog = config.catalog?.trim() || process.env.KC_CATALOG?.trim() || undefined;
    this.workspace = config.workspace?.trim() || process.env.KC_WORKSPACE?.trim() || '';
    this.principal = config.principal?.trim() || process.env.KC_AS?.trim() || '';
    if (!this.server) throw new Error('dsh-loom: KC_SERVER_URL is required. Local deployments must start kc serve and use the same typed Workspace File Gateway.');
    if (!this.workspace) throw new Error('dsh-loom: KC_WORKSPACE is required. Set it to an existing Workspace id; inspect available Workspaces with "kc catalog workspace list".');
    if (!this.principal) throw new Error('dsh-loom: KC_AS is required for an Agent mount. Set an explicit Agent principal, for example agent:dsh.');
  }

  created(session: SessionLike): void {
    const root = requiredAbsolute(session.header.cwd, 'session cwd');
    const parent = session.header.parentSession ? this.bySession.get(String(session.header.parentSession)) : undefined;
    if (parent) {
      if (parent.manifest.root !== root) throw new Error('dsh-loom: child task cwd conflicts with its parent mount root');
      parent.sessions.add(String(session.id));
      this.bySession.set(String(session.id), parent);
      this.writeContext(session, parent.manifest);
      return;
    }
    const existing = this.byRoot.get(root);
    if (existing) {
      if (existing.manifest.workspaceId !== this.workspace) throw new Error('dsh-loom: another Workspace is already mounted on this task root');
      existing.sessions.add(String(session.id));
      this.bySession.set(String(session.id), existing);
      this.writeContext(session, existing.manifest);
      return;
    }

    const args = ['daemon-mount', '--server', this.server];
    args.push('--workspace', this.workspace, '--root', root, '--as', this.principal);
    if (this.catalog) args.push('--catalog', this.catalog);
    let manifest: MountManifest;
    try {
      manifest = JSON.parse(execFileSync(this.bin, args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] })) as MountManifest;
    } catch (error) {
      throw mountFailure(error, this.bin);
    }
    if (!manifest.pinId || manifest.workspaceId !== this.workspace || manifest.root !== root || !Number.isSafeInteger(manifest.pid)) {
      throw new Error('dsh-loom: kcfs returned an invalid ready manifest');
    }
    const active: ActiveMount = { manifest, sessions: new Set([String(session.id)]) };
    this.byRoot.set(root, active);
    this.bySession.set(String(session.id), active);
    this.writeContext(session, manifest);
  }

  disposed(session: SessionLike): void {
    const id = String(session.id);
    const active = this.bySession.get(id);
    if (!active) return;
    this.bySession.delete(id);
    active.sessions.delete(id);
    rmSync(this.contextDir(id), { recursive: true, force: true });
    if (active.sessions.size > 0) return;
    this.byRoot.delete(active.manifest.root);
    execFileSync(this.bin, ['stop', '--pid', String(active.manifest.pid)], { stdio: ['ignore', 'ignore', 'pipe'] });
  }

  private contextDir(sessionId: string): string {
    return path.join(this.home, 'tasks', Buffer.from(sessionId).toString('base64url'));
  }

  private writeContext(session: SessionLike, manifest: MountManifest): void {
    const dir = this.contextDir(String(session.id));
    mkdirSync(dir, { recursive: true, mode: 0o700 });
    writeFileSync(path.join(dir, 'context.json'), `${JSON.stringify({
      version: 1,
      sessionId: String(session.id),
      principal: this.principal,
      catalog: this.catalog,
      workspace: this.workspace,
      pinId: manifest.pinId,
      pin: manifest.pin,
      root: manifest.root,
      readOnly: true,
      mounts: manifest.mounts,
    }, null, 2)}\n`, { encoding: 'utf8', mode: 0o600 });
  }
}

export function applyMountController(ctx: Context, config: MountControllerConfig): void {
  const controller = new MountController(config);
  const lifecycle = ctx as unknown as {
    on(event: 'session/created' | 'session/disposed', listener: (session: SessionLike) => void, options?: { global?: boolean }): () => void;
  };
  ctx.effect(() => {
    const created = lifecycle.on('session/created', (session) => controller.created(session), { global: true });
    const disposed = lifecycle.on('session/disposed', (session) => controller.disposed(session), { global: true });
    return () => { disposed(); created(); };
  }, 'dsh-loom: task-scoped read-only Knowledge mounts');
}
