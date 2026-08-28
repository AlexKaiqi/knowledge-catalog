/** Human-only bridge over task mounts already present on the host filesystem. */

import type { Context } from '@deepseek-ai/cordis';
import type { IncomingMessage, ServerResponse } from 'node:http';
import { lstat, opendir, readFile, realpath, writeFile, mkdir } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import path from 'node:path';

export const name = 'loom-web';
export const inject = ['webServer'];

const ROUTE = '/api/loom/vfs';
const MAX_PREVIEW_BYTES = 512 * 1024;
const MAX_BROWSER_ENTRIES = 5_000;

interface TaskMount {
  path: string;
  mountpoint: string;
  repository: string;
  commit: string;
}

interface TaskContext {
  version: 1;
  catalog?: string;
  workspace: string;
  pinId: string;
  pin?: { workspaceId: string; pinId?: string; repositories: Record<string, string> };
  root: string;
  readOnly: true;
  mounts: TaskMount[];
}

interface LoomEntry {
  path: string;
  repository: string;
  commit: string;
  kind: 'file' | 'directory';
}

export interface LoomBrowserConfig { home?: string }

export interface LoomBrowserList {
  workspace: string;
  catalog?: string;
  state: 'ready' | 'unbound' | 'unavailable';
  pin?: { workspaceId: string; pinId: string; repositories: Record<string, string> };
  vfs: {
    enabled: boolean;
    state: 'disabled' | 'collapsed' | 'ready' | 'unavailable';
    error?: { code: string; message: string };
    entries: LoomEntry[];
    mounts: TaskMount[];
    continuation?: string;
  };
}

export interface LoomBrowserRead {
  path: string;
  repository: string;
  commit: string;
  size: number;
  binary: boolean;
  truncated: boolean;
  content?: string;
}

function resolveHome(configured?: string): string {
  return path.resolve(configured?.trim() || process.env.KC_HOME?.trim() || path.join(process.cwd(), '.kc-home'));
}

function contains(root: string, candidate: string): boolean {
  const relative = path.relative(root, candidate);
  return relative === '' || (!relative.startsWith('..') && !path.isAbsolute(relative));
}

async function taskContexts(home: string): Promise<TaskContext[]> {
  const tasks = path.join(home, 'tasks');
  let directory;
  try { directory = await opendir(tasks); } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') return [];
    throw error;
  }
  const contexts: TaskContext[] = [];
  for await (const entry of directory) {
    if (!entry.isDirectory()) continue;
    try {
      const parsed = JSON.parse(await readFile(path.join(tasks, entry.name, 'context.json'), 'utf8')) as TaskContext;
      if (parsed.version === 1 && parsed.readOnly === true && path.isAbsolute(parsed.root) && Array.isArray(parsed.mounts)) contexts.push(parsed);
    } catch { /* a half-removed task is not an active mount */ }
  }
  return contexts;
}

async function contextFor(home: string, cwd: string | undefined): Promise<TaskContext | undefined> {
  if (!cwd || !path.isAbsolute(cwd)) return undefined;
  const resolved = path.resolve(cwd);
  return (await taskContexts(home))
    .filter((context) => contains(context.root, resolved))
    .sort((left, right) => right.root.length - left.root.length)[0];
}

function preferencePath(home: string, root: string): string {
  return path.join(home, 'ui', `${Buffer.from(root).toString('base64url')}.json`);
}

async function enabled(home: string, root: string): Promise<boolean> {
  try { return JSON.parse(await readFile(preferencePath(home, root), 'utf8')).enabled === true; } catch { return false; }
}

async function setEnabled(home: string, root: string, value: boolean): Promise<void> {
  const file = preferencePath(home, root);
  await mkdir(path.dirname(file), { recursive: true, mode: 0o700 });
  await writeFile(file, `${JSON.stringify({ enabled: value })}\n`, { encoding: 'utf8', mode: 0o600 });
}

interface DirectoryCursor { version: 1; pinId: string; directory: string; after: string; check: string }

function cursorCheck(cursor: Omit<DirectoryCursor, 'check'>): string {
  return createHash('sha256').update(JSON.stringify(cursor)).digest('hex');
}

function encodeCursor(pinId: string, directory: string, after: string): string {
  const unsigned = { version: 1 as const, pinId, directory, after };
  return Buffer.from(JSON.stringify({ ...unsigned, check: cursorCheck(unsigned) })).toString('base64url');
}

function decodeCursor(value: string, pinId: string, directory: string): string {
  const cursor = JSON.parse(Buffer.from(value, 'base64url').toString('utf8')) as DirectoryCursor;
  const unsigned = { version: cursor.version, pinId: cursor.pinId, directory: cursor.directory, after: cursor.after };
  if (cursor.version !== 1 || cursor.pinId !== pinId || cursor.directory !== directory || cursor.check !== cursorCheck(unsigned)) {
    throw new Error('directory continuation does not match the active pin and directory');
  }
  return cursor.after;
}

async function listMounted(context: TaskContext, requested: string, limit: number, continuation: string): Promise<{ entries: LoomEntry[]; continuation?: string }> {
  const directory = requested.trim().replace(/^\/+|\/+$/g, '');
  if (directory.includes('\0') || directory.split('/').includes('..')) throw new Error('directory is invalid');
  const after = continuation ? decodeCursor(continuation, context.pinId, directory) : '';
  const entries: LoomEntry[] = [];
  if (!directory) {
    for (const mount of context.mounts) entries.push({
      path: mount.path.replace(/^\/+|\/+$/g, ''), repository: mount.repository, commit: mount.commit, kind: 'directory',
    });
  } else {
    const virtual = new Map<string, LoomEntry>();
    for (const mount of context.mounts) {
      const root = mount.path.replace(/^\/+|\/+$/g, '');
      if (!root.startsWith(`${directory}/`)) continue;
      const next = root.slice(directory.length + 1).split('/')[0];
      const virtualPath = `${directory}/${next}`;
      virtual.set(virtualPath, { path: virtualPath, repository: mount.repository, commit: mount.commit, kind: 'directory' });
    }
    if (virtual.size > 0) {
      entries.push(...virtual.values());
    } else {
    const mount = [...context.mounts].sort((a, b) => b.path.length - a.path.length).find((candidate) => {
      const root = candidate.path.replace(/^\/+|\/+$/g, '');
      return directory === root || directory.startsWith(`${root}/`);
    });
    if (!mount) throw new Error('directory is outside the active mount manifest');
    const mountRoot = await realpath(mount.mountpoint);
    const relative = path.posix.relative(mount.path.replace(/^\/+|\/+$/g, ''), directory);
    const candidate = await realpath(path.join(mountRoot, ...relative.split('/').filter(Boolean)));
    if (!contains(mountRoot, candidate)) throw new Error('directory escaped its active mount');
    const opened = await opendir(candidate);
    for await (const entry of opened) {
      if (!entry.isDirectory() && !entry.isFile()) continue;
      entries.push({
        path: `${directory}/${entry.name}`, repository: mount.repository, commit: mount.commit,
        kind: entry.isDirectory() ? 'directory' : 'file',
      });
    }
    }
  }
  entries.sort((left, right) => left.path.localeCompare(right.path));
  const start = after ? entries.findIndex((entry) => entry.path === after) + 1 : 0;
  const page = entries.slice(start, start + limit);
  const next = start + page.length < entries.length && page.length > 0 ? encodeCursor(context.pinId, directory, page[page.length - 1].path) : undefined;
  return { entries: page, ...(next ? { continuation: next } : {}) };
}

function pinOf(context: TaskContext) {
  if (context.pin?.pinId) return context.pin as { workspaceId: string; pinId: string; repositories: Record<string, string> };
  const repositories: Record<string, string> = {};
  for (const mount of context.mounts) repositories[mount.repository] = mount.commit;
  return { workspaceId: context.workspace, pinId: context.pinId, repositories };
}

function looksBinary(bytes: Uint8Array): boolean {
  return bytes.subarray(0, Math.min(bytes.byteLength, 8192)).some((byte) => byte === 0);
}

async function readMounted(context: TaskContext, requested: string): Promise<LoomBrowserRead> {
  const normalized = requested.trim().replace(/^\/+/, '');
  if (!normalized || normalized.includes('\0') || normalized.split('/').includes('..')) throw new Error('path is invalid');
  const mount = [...context.mounts].sort((a, b) => b.path.length - a.path.length).find((candidate) => {
    const root = candidate.path.replace(/^\/+|\/+$/g, '');
    return normalized === root || normalized.startsWith(`${root}/`);
  });
  if (!mount) throw new Error('path is outside the active mount manifest');
  const mountRoot = await realpath(mount.mountpoint);
  const relative = path.posix.relative(mount.path.replace(/^\/+|\/+$/g, ''), normalized);
  const candidate = await realpath(path.join(mountRoot, ...relative.split('/')));
  if (!contains(mountRoot, candidate)) throw new Error('path escaped its active mount');
  const info = await lstat(candidate);
  if (!info.isFile()) throw new Error('path is not a file');
  const bytes = await readFile(candidate);
  const preview = bytes.subarray(0, MAX_PREVIEW_BYTES);
  const binary = looksBinary(preview);
  return {
    path: normalized, repository: mount.repository, commit: mount.commit,
    size: info.size, binary, truncated: info.size > preview.byteLength,
    ...(binary ? {} : { content: new TextDecoder().decode(preview) }),
  };
}

function send(res: ServerResponse, status: number, body: unknown): void {
  const encoded = JSON.stringify(body);
  res.writeHead(status, { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store', 'content-length': Buffer.byteLength(encoded) });
  res.end(encoded);
}

async function requestBody(req: IncomingMessage): Promise<Record<string, unknown>> {
  const chunks: Buffer[] = [];
  for await (const chunk of req) chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
  if (chunks.reduce((total, chunk) => total + chunk.length, 0) > 64 * 1024) throw new Error('request too large');
  const value = JSON.parse(Buffer.concat(chunks).toString('utf8') || '{}') as unknown;
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('JSON object required');
  return value as Record<string, unknown>;
}

export function createLoomWorkspaceHandler(input: LoomBrowserConfig) {
  const home = resolveHome(input.home);
  return async (req: IncomingMessage, res: ServerResponse): Promise<void> => {
    try {
      const url = new URL(req.url ?? ROUTE, 'http://dsh.local');
      if (req.method === 'GET') {
        const cwd = url.searchParams.get('cwd') ?? undefined;
        const context = await contextFor(home, cwd);
        if (!context) {
          send(res, 200, { workspace: '', state: 'unbound', vfs: { enabled: false, state: 'disabled', entries: [], mounts: [] } });
          return;
        }
        const isEnabled = await enabled(home, context.root);
        const base = { workspace: context.workspace, ...(context.catalog ? { catalog: context.catalog } : {}), state: 'ready' as const, pin: pinOf(context) };
        if (!isEnabled || url.searchParams.get('load') !== '1') {
          send(res, 200, { ...base, vfs: { enabled: isEnabled, state: isEnabled ? 'collapsed' : 'disabled', entries: [], mounts: [] } });
          return;
        }
        const file = url.searchParams.get('path');
        if (file !== null) {
          send(res, 200, await readMounted(context, file));
          return;
        }
        const directory = url.searchParams.get('directory') ?? '';
        const limit = Math.min(MAX_BROWSER_ENTRIES, Math.max(1, Number(url.searchParams.get('limit') ?? 500)));
        const page = await listMounted(context, directory, limit, url.searchParams.get('continuation') ?? '');
        send(res, 200, { ...base, vfs: { enabled: true, state: 'ready', entries: page.entries, mounts: context.mounts, ...(page.continuation ? { continuation: page.continuation } : {}) } } satisfies LoomBrowserList);
        return;
      }
      if (req.method === 'POST') {
        const body = await requestBody(req);
        if (body.action !== 'set-vfs-enabled' || typeof body.cwd !== 'string' || typeof body.enabled !== 'boolean') throw new Error('set-vfs-enabled requires cwd and enabled');
        const context = await contextFor(home, body.cwd);
        if (!context) throw new Error('cwd has no active task mount');
        await setEnabled(home, context.root, body.enabled);
        send(res, 200, { preferences: { vfsEnabled: body.enabled } });
        return;
      }
      send(res, 405, { error: { code: 'USAGE_INVALID', message: 'GET or POST required' } });
    } catch (error) {
      send(res, 400, { error: { code: 'USAGE_INVALID', message: error instanceof Error ? error.message : String(error) } });
    }
  };
}

export function apply(ctx: Context, config: LoomBrowserConfig): void {
  const webServer = (ctx as unknown as { webServer: { register(route: { kind: 'exact'; path: string; handler: ReturnType<typeof createLoomWorkspaceHandler> }): () => void } }).webServer;
  ctx.effect(() => webServer.register({ kind: 'exact', path: ROUTE, handler: createLoomWorkspaceHandler(config) }), 'dsh-loom: mounted-files host bridge');
}
