/** Human-only bridge over task mounts already present on the host filesystem. */

import type { Context } from '@deepseek-ai/cordis';
import type { IncomingMessage, ServerResponse } from 'node:http';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { lstat, opendir, readFile, realpath, writeFile, mkdir, rm } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import path from 'node:path';

export const name = 'loom-web';
export const inject = ['webServer'];

const ROUTE = '/api/loom/vfs';
const MAX_PREVIEW_BYTES = 512 * 1024;
const MAX_BROWSER_ENTRIES = 5_000;
const execFileAsync = promisify(execFile);

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
  pid?: number;
  managedBy?: 'task-config' | 'project-ui';
  sourceFile?: string;
}

interface LoomEntry {
  path: string;
  repository: string;
  commit: string;
  kind: 'file' | 'directory';
}

interface SchemaSummary {
  objectId: string;
  entity?: string;
  aspect?: string;
  pattern?: string;
}

interface RepositorySummary {
  id: string;
  system: boolean;
  commit?: string;
  schemas: SchemaSummary[];
  schemaCoverage?: { enumerated: number; total: number; complete: boolean };
  error?: { code: string; message: string };
}

interface KnowledgeSetSummary {
  catalog: string;
  id: string;
  revision: number;
  repositories: string[];
}

export interface KnowledgeInventory {
  server: string;
  catalogs: Array<{ id: string; repositories: RepositorySummary[]; knowledgeSets: KnowledgeSetSummary[] }>;
  elapsedMs: number;
}

export interface LoomBrowserConfig {
  home?: string;
  bin?: string;
  server?: string;
  catalog?: string;
  principal?: string;
  view?: 'repository' | 'semantic';
}

export interface LoomBrowserList {
  workspace: string;
  catalog?: string;
  state: 'ready' | 'unbound' | 'unavailable';
  pin?: { workspaceId: string; pinId: string; repositories: Record<string, string> };
  managedBy?: 'task-config' | 'project-ui';
  inventory?: KnowledgeInventory;
  inventoryError?: { code: string; message: string };
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

async function contextsIn(directoryPath: string): Promise<TaskContext[]> {
  let directory;
  try { directory = await opendir(directoryPath); } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') return [];
    throw error;
  }
  const contexts: TaskContext[] = [];
  for await (const entry of directory) {
    if (!entry.isDirectory()) continue;
    try {
      const sourceFile = path.join(directoryPath, entry.name, 'context.json');
      const parsed = JSON.parse(await readFile(sourceFile, 'utf8')) as TaskContext;
      if (parsed.version === 1 && parsed.readOnly === true && path.isAbsolute(parsed.root) && Array.isArray(parsed.mounts)) {
        contexts.push({ ...parsed, sourceFile });
      }
    } catch { /* a half-removed task is not an active mount */ }
  }
  return contexts;
}

async function taskContexts(home: string): Promise<TaskContext[]> {
  const [tasks, projects] = await Promise.all([
    contextsIn(path.join(home, 'tasks')),
    contextsIn(path.join(home, 'projects')),
  ]);
  return [...tasks, ...projects];
}

const contextCache = new Map<string, { expires: number; contexts: TaskContext[] }>();

async function cachedTaskContexts(home: string, force = false): Promise<TaskContext[]> {
  const current = contextCache.get(home);
  if (!force && current && current.expires > Date.now()) return current.contexts;
  const contexts = await taskContexts(home);
  contextCache.set(home, { expires: Date.now() + 5_000, contexts });
  return contexts;
}

async function contextFor(home: string, cwd: string | undefined): Promise<TaskContext | undefined> {
  if (!cwd || !path.isAbsolute(cwd)) return undefined;
  const resolved = path.resolve(cwd);
  const select = (contexts: TaskContext[]): TaskContext | undefined => contexts
    .filter((context) => contains(context.root, resolved))
    .sort((left, right) => right.root.length - left.root.length || right.mounts.length - left.mounts.length)[0];
  return select(await cachedTaskContexts(home)) ?? select(await cachedTaskContexts(home, true));
}

function invalidateContexts(home: string): void { contextCache.delete(home); }

function preferencePath(home: string, root: string): string {
  return path.join(home, 'ui', `${Buffer.from(root).toString('base64url')}.json`);
}

async function enabled(home: string, root: string): Promise<boolean> {
  try {
    return JSON.parse(await readFile(preferencePath(home, root), 'utf8')).enabled === true;
  } catch (error) {
    // Visibility is an explicit human preference. It does not create or tear
    // down the mount, alter the pin, or change Agent file access.
    return false;
  }
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

interface RuntimeConfig {
  home: string;
  bin: string;
  server: string;
  catalog?: string;
  principal: string;
  authorization?: string;
  view: 'repository' | 'semantic';
}

interface MountManifest {
  workspaceId: string;
  pinId: string;
  pin?: { workspaceId: string; pinId?: string; repositories: Record<string, string> };
  root: string;
  readOnly: true;
  pid: number;
  mounts: TaskMount[];
}

class KCRequestError extends Error {
  constructor(readonly code: string, message: string) { super(message); }
}

function runtimeConfig(input: LoomBrowserConfig): RuntimeConfig {
  return {
    home: resolveHome(input.home),
    bin: input.bin?.trim() || process.env.KCFS_BIN?.trim() || 'kcfs',
    server: (input.server?.trim() || process.env.KC_SERVER_URL?.trim() || '').replace(/\/+$/, ''),
    catalog: input.catalog?.trim() || process.env.KC_CATALOG?.trim() || undefined,
    principal: input.principal?.trim() || process.env.KC_AS?.trim() || '',
    authorization: process.env.KC_AUTH_TOKEN?.trim() || undefined,
    view: input.view ?? 'semantic',
  };
}

function requestHeaders(config: RuntimeConfig, json = false): Record<string, string> {
  const headers: Record<string, string> = {};
  if (json) headers['content-type'] = 'application/json';
  if (config.authorization) headers.authorization = config.authorization;
  else if (config.principal) headers['x-kc-as'] = config.principal;
  return headers;
}

async function kcJSON<T>(config: RuntimeConfig, route: string, body?: unknown): Promise<T> {
  if (!config.server) throw new KCRequestError('NOT_CONFIGURED', '未配置 KC_SERVER_URL，无法发现可用知识。');
  if (!config.authorization && !config.principal) throw new KCRequestError('UNAUTHENTICATED', '未配置 KC_AS 或 KC_AUTH_TOKEN，无法读取知识目录。');
  const response = await fetch(`${config.server}${route}`, {
    method: body === undefined ? 'GET' : 'POST',
    headers: requestHeaders(config, body !== undefined),
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  const decoded = await response.json() as T & { error?: { code?: string; message?: string } };
  if (!response.ok) throw new KCRequestError(decoded.error?.code ?? 'TEMPORARY_UNAVAILABLE', decoded.error?.message ?? `KC request failed (${response.status})`);
  return decoded;
}

const inventoryCache = new Map<string, { expires: number; value: KnowledgeInventory }>();

async function discoverKnowledge(config: RuntimeConfig, force = false): Promise<KnowledgeInventory> {
  const cacheKey = [config.server, config.catalog ?? '', config.principal, createHash('sha256').update(config.authorization ?? '').digest('hex')].join('|');
  const cached = inventoryCache.get(cacheKey);
  if (!force && cached && cached.expires > Date.now()) return cached.value;
  const started = performance.now();
  const catalogResponse = await kcJSON<{ catalogs: Array<{ id: string }> }>(config, '/catalog/v1/catalogs');
  const catalogIDs = catalogResponse.catalogs.map((item) => item.id)
    .filter((id) => !config.catalog || id === config.catalog)
    .slice(0, 20);
  if (config.catalog && catalogIDs.length === 0) throw new KCRequestError('CATALOG_NOT_FOUND', `Catalog ${config.catalog} 不可见。`);
  const catalogs = await Promise.all(catalogIDs.map(async (catalogID) => {
    const encoded = encodeURIComponent(catalogID);
    const [repositoryResponse, workspaceResponse] = await Promise.all([
      kcJSON<{ repositories: Array<string | { id: string }> }>(config, `/catalog/v1/catalogs/${encoded}/repositories`),
      kcJSON<{ workspaces: Array<{ workspaceId: string; revision: number; retired?: boolean; sources: Array<{ repository: string }> }> }>(config, `/catalog/v1/catalogs/${encoded}/workspaces`),
    ]);
    const repositories = await Promise.all(repositoryResponse.repositories.slice(0, 100).map(async (item): Promise<RepositorySummary> => {
      const repository = typeof item === 'string' ? item : item.id;
      try {
        const page = await kcJSON<{
          repository: string;
          commit: string;
          schemas: SchemaSummary[];
          coverage: { enumerated: number; total: number; complete: boolean };
        }>(config, '/knowledge/v1/schemas:page', { repository, limit: 50 });
        return {
          id: repository,
          system: repository === 'kr://kc/system',
          commit: page.commit,
          schemas: page.schemas,
          schemaCoverage: page.coverage,
        };
      } catch (error) {
        return {
          id: repository,
          system: repository === 'kr://kc/system',
          schemas: [],
          error: {
            code: error instanceof KCRequestError ? error.code : 'TEMPORARY_UNAVAILABLE',
            message: error instanceof Error ? error.message : String(error),
          },
        };
      }
    }));
    const knowledgeSets = workspaceResponse.workspaces.filter((workspace) => !workspace.retired).slice(0, 100).map((workspace) => ({
      catalog: catalogID,
      id: workspace.workspaceId,
      revision: workspace.revision,
      repositories: [...new Set(workspace.sources.map((source) => source.repository))],
    }));
    return { id: catalogID, repositories, knowledgeSets };
  }));
  const value = { server: config.server, catalogs, elapsedMs: Math.round((performance.now() - started) * 10) / 10 };
  inventoryCache.set(cacheKey, { expires: Date.now() + 10_000, value });
  return value;
}

function projectContextDir(home: string, root: string): string {
  return path.join(home, 'projects', createHash('sha256').update(path.resolve(root)).digest('hex'));
}

async function projectConnection(home: string, root: string): Promise<TaskContext | undefined> {
  return (await contextsIn(path.join(home, 'projects'))).find((context) => path.resolve(context.root) === path.resolve(root));
}

async function stopProjectConnection(config: RuntimeConfig, context: TaskContext): Promise<void> {
  if (context.managedBy !== 'project-ui' || !Number.isSafeInteger(context.pid) || Number(context.pid) <= 1) {
    throw new KCRequestError('PRECONDITION_FAILED', '当前知识挂载不是由项目界面创建，不能从这里卸载。');
  }
  await execFileAsync(config.bin, ['stop', '--pid', String(context.pid)], { timeout: 30_000 });
  await rm(projectContextDir(config.home, context.root), { recursive: true, force: true });
  invalidateContexts(config.home);
}

async function connectProject(config: RuntimeConfig, cwd: string, catalog: string, workspace: string): Promise<MountManifest> {
  if (!config.server) throw new KCRequestError('NOT_CONFIGURED', '未配置 KC_SERVER_URL，不能添加知识。');
  if (!config.principal) throw new KCRequestError('UNAUTHENTICATED', '未配置 KC_AS，不能建立 Agent 只读挂载。');
  const hostContext = await contextFor(config.home, cwd);
  if (!hostContext) throw new KCRequestError('PRECONDITION_FAILED', '当前目录不是一个活动项目。');
  const inventory = await discoverKnowledge(config);
  const selectable = inventory.catalogs.some((item) => item.id === catalog && item.knowledgeSets.some((knowledgeSet) => knowledgeSet.id === workspace));
  if (!selectable) throw new KCRequestError('WORKSPACE_INVALID', `知识集 ${workspace} 在 Catalog ${catalog} 中不可见或已退役。`);
  const root = path.resolve(hostContext.root);
  const existing = await projectConnection(config.home, root);
  if (existing) await stopProjectConnection(config, existing);
  if (hostContext.workspace && hostContext.managedBy !== 'project-ui') {
    throw new KCRequestError('PRECONDITION_FAILED', '当前项目由宿主配置了默认知识集，需先移除 KC_WORKSPACE 配置后再切换。');
  }
  const args = ['daemon-mount', '--server', config.server, '--view', config.view, '--workspace', workspace, '--root', root, '--as', config.principal];
  if (catalog) args.push('--catalog', catalog);
  let manifest: MountManifest;
  try {
    const { stdout } = await execFileAsync(config.bin, args, { timeout: 60_000, maxBuffer: 4 << 20 });
    manifest = JSON.parse(stdout) as MountManifest;
  } catch (error) {
    const failure = error as { stderr?: string | Buffer };
    const detail = String(failure.stderr ?? (error instanceof Error ? error.message : error)).trim();
    throw new KCRequestError('TEMPORARY_UNAVAILABLE', `知识挂载失败：${detail}`);
  }
  if (!manifest.pinId || manifest.workspaceId !== workspace || path.resolve(manifest.root) !== root || !Number.isSafeInteger(manifest.pid) || !Array.isArray(manifest.mounts)) {
    throw new KCRequestError('PRECONDITION_FAILED', 'kcfs 返回了无效的固定版本挂载清单。');
  }
  const directory = projectContextDir(config.home, root);
  await mkdir(directory, { recursive: true, mode: 0o700 });
  await writeFile(path.join(directory, 'context.json'), `${JSON.stringify({
    version: 1,
    catalog,
    workspace,
    pinId: manifest.pinId,
    pin: manifest.pin,
    root,
    readOnly: true,
    pid: manifest.pid,
    managedBy: 'project-ui',
    mounts: manifest.mounts,
  }, null, 2)}\n`, { encoding: 'utf8', mode: 0o600 });
  invalidateContexts(config.home);
  return manifest;
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
  const config = runtimeConfig(input);
  const home = config.home;
  return async (req: IncomingMessage, res: ServerResponse): Promise<void> => {
    try {
      const url = new URL(req.url ?? ROUTE, 'http://dsh.local');
      if (req.method === 'GET') {
        const cwd = url.searchParams.get('cwd') ?? undefined;
        let inventory: KnowledgeInventory | undefined;
        let inventoryError: { code: string; message: string } | undefined;
        if (url.searchParams.get('discover') === '1') {
          try {
            inventory = await discoverKnowledge(config, url.searchParams.get('refresh') === '1');
          } catch (error) {
            inventoryError = {
              code: error instanceof KCRequestError ? error.code : 'TEMPORARY_UNAVAILABLE',
              message: error instanceof Error ? error.message : String(error),
            };
          }
        }
        const discovery = { ...(inventory ? { inventory } : {}), ...(inventoryError ? { inventoryError } : {}) };
        const context = await contextFor(home, cwd);
        if (!context) {
          send(res, 200, { workspace: '', state: 'unbound', ...discovery, vfs: { enabled: false, state: 'disabled', entries: [], mounts: [] } });
          return;
        }
        const ready = context.workspace !== '';
        const isEnabled = await enabled(home, context.root);
        const base = {
          workspace: context.workspace,
          ...(context.catalog ? { catalog: context.catalog } : {}),
          state: ready ? 'ready' as const : 'unbound' as const,
          ...(ready ? { pin: pinOf(context) } : {}),
          ...(context.managedBy ? { managedBy: context.managedBy } : {}),
          ...discovery,
        };
        if (!ready || !isEnabled || url.searchParams.get('load') !== '1') {
          send(res, 200, { ...base, vfs: { enabled: ready && isEnabled, state: ready && isEnabled ? 'collapsed' : 'disabled', entries: [], mounts: [] } });
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
        if (body.action === 'set-vfs-enabled') {
          if (typeof body.cwd !== 'string' || typeof body.enabled !== 'boolean') throw new Error('set-vfs-enabled requires cwd and enabled');
          const context = await contextFor(home, body.cwd);
          if (!context || !context.workspace) throw new Error('cwd has no active knowledge mount');
          await setEnabled(home, context.root, body.enabled);
          send(res, 200, { preferences: { vfsEnabled: body.enabled } });
          return;
        }
        if (body.action === 'connect-workspace') {
          if (typeof body.cwd !== 'string' || typeof body.catalog !== 'string' || typeof body.workspace !== 'string' || !body.workspace.trim()) {
            throw new Error('connect-workspace requires cwd, catalog and workspace');
          }
          const manifest = await connectProject(config, body.cwd, body.catalog, body.workspace);
          send(res, 200, { connection: { workspace: manifest.workspaceId, pinId: manifest.pinId, mounts: manifest.mounts } });
          return;
        }
        if (body.action === 'disconnect-workspace') {
          if (typeof body.cwd !== 'string') throw new Error('disconnect-workspace requires cwd');
          const context = await contextFor(home, body.cwd);
          if (!context) throw new Error('cwd has no active project');
          const project = await projectConnection(home, context.root);
          if (!project) throw new KCRequestError('PRECONDITION_FAILED', '当前项目没有由界面添加的知识集。');
          await stopProjectConnection(config, project);
          send(res, 200, { connection: { workspace: '', mounts: [] } });
          return;
        }
        throw new Error('unknown knowledge action');
      }
      send(res, 405, { error: { code: 'USAGE_INVALID', message: 'GET or POST required' } });
    } catch (error) {
      send(res, 400, { error: { code: error instanceof KCRequestError ? error.code : 'USAGE_INVALID', message: error instanceof Error ? error.message : String(error) } });
    }
  };
}

export function apply(ctx: Context, config: LoomBrowserConfig): void {
  const webServer = (ctx as unknown as { webServer: { register(route: { kind: 'exact'; path: string; handler: ReturnType<typeof createLoomWorkspaceHandler> }): () => void } }).webServer;
  ctx.effect(() => webServer.register({ kind: 'exact', path: ROUTE, handler: createLoomWorkspaceHandler(config) }), 'dsh-loom: mounted-files host bridge');
}
