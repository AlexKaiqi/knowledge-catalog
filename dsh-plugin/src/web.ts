/** Human-only bridge over task mounts already present on the host filesystem. */

import type { Context } from '@deepseek-ai/cordis';
import type { IncomingMessage, ServerResponse } from 'node:http';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { lstat, opendir, readFile, realpath, writeFile, mkdir, rm, rename } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import path from 'node:path';
import { resolveBrowserIdentity, BrowserIdentityError } from './auth.js';

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

interface WorkspaceDefinition { workspaceId: string; revision: number; sources: Array<{ repository: string; selector: string; path?: string }> }
interface TaskPin { workspaceId: string; revision: number; pinId: string; repositories: Record<string, string>; catalog?: string; definition?: WorkspaceDefinition }

interface TaskContext {
  server?: string;
  principal?: string;
  authMode?: 'token' | 'local';
  version: 1;
  catalog?: string;
  dataset?: string;
  workspace: string;
  pinId: string;
  pin?: TaskPin;
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
  profile?: string;
  title?: string;
  summary?: string;
  schemaCount?: number;
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

interface Coverage { enumerated: number; total: number; complete: boolean }
export interface KnowledgeInventory {
  coverage: Coverage;
  server: string;
  catalogs: Array<{ id: string; repositories: RepositorySummary[]; knowledgeSets: KnowledgeSetSummary[]; repositoryCoverage: Coverage; knowledgeSetCoverage: Coverage }>;
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
  filesMounted?: boolean;
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
    .sort((left, right) => right.root.length - left.root.length || Number(right.managedBy === 'project-ui') - Number(left.managedBy === 'project-ui') || right.mounts.length - left.mounts.length)[0];
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
  setId?: string;
  workspaceId?: string;
  pinId: string;
  pin?: TaskPin;
  root: string;
  readOnly: true;
  pid: number;
  mounts: TaskMount[];
}

class KCRequestError extends Error {
  constructor(readonly code: string, message: string) { super(message); }
}

async function runtimeConfig(input: LoomBrowserConfig): Promise<RuntimeConfig> {
  const identity = await resolveBrowserIdentity(input);
  return {
    home: resolveHome(input.home),
    bin: input.bin?.trim() || process.env.KCFS_BIN?.trim() || 'kcfs',
    server: identity.server,
    catalog: input.catalog?.trim() || process.env.KC_CATALOG?.trim() || undefined,
    principal: identity.principal,
    authorization: identity.authorization,
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
    redirect: 'error',
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
  const visibleCatalogIDs = catalogResponse.catalogs.map((item) => item.id).filter((id) => !config.catalog || id === config.catalog);
  const catalogIDs = visibleCatalogIDs.slice(0, 20);
  if (config.catalog && catalogIDs.length === 0) throw new KCRequestError('CATALOG_NOT_FOUND', `Catalog ${config.catalog} 不可见。`);
  const catalogs = await Promise.all(catalogIDs.map(async (catalogID) => {
    const encoded = encodeURIComponent(catalogID);
    const [repositoryResponse, workspaceResponse] = await Promise.all([
      kcJSON<{ repositories: Array<string | { id: string; profile?: string; title?: string; summary?: string; schemaCount?: number }> }>(config, `/catalog/v1/catalogs/${encoded}/repositories`),
      kcJSON<{ workspaces: Array<{ workspaceId: string; revision: number; retired?: boolean; repositories: string[] }> }>(config, `/catalog/v1/catalogs/${encoded}/workspaces`),
    ]);
    const repositories = await Promise.all(repositoryResponse.repositories.slice(0, 100).map(async (item): Promise<RepositorySummary> => {
      const repository = typeof item === 'string' ? item : item.id;
      const profile = typeof item === 'string' ? {} : { profile: item.profile, title: item.title, summary: item.summary, schemaCount: item.schemaCount };
      try {
        const page = await kcJSON<{
          repository: string;
          commit: string;
          schemas: SchemaSummary[];
          coverage: { enumerated: number; total: number; complete: boolean };
        }>(config, '/knowledge/v1/schemas:list', { repository, limit: 50 });
        return {
          id: repository,
          ...profile,
          system: repository === 'kr://kc/system',
          commit: page.commit,
          schemas: page.schemas,
          schemaCoverage: page.coverage,
        };
      } catch (error) {
        return {
          id: repository,
          ...profile,
          system: repository === 'kr://kc/system',
          schemas: [],
          error: {
            code: error instanceof KCRequestError || error instanceof BrowserIdentityError ? error.code : 'TEMPORARY_UNAVAILABLE',
            message: error instanceof Error ? error.message : String(error),
          },
        };
      }
    }));
    const knowledgeSets = workspaceResponse.workspaces.filter((workspace) => !workspace.retired).slice(0, 100).map((workspace) => ({
      catalog: catalogID,
      id: workspace.workspaceId,
      revision: workspace.revision,
      repositories: [...new Set(workspace.repositories)],
    }));
    return { id: catalogID, repositories, knowledgeSets,
      repositoryCoverage: coverage(repositories.length, repositoryResponse.repositories.length),
      knowledgeSetCoverage: coverage(knowledgeSets.length, workspaceResponse.workspaces.filter((item) => !item.retired).length),
    };
  }));
  const value = { server: config.server, catalogs, coverage: coverage(catalogs.length, visibleCatalogIDs.length), elapsedMs: Math.round((performance.now() - started) * 10) / 10 };
  inventoryCache.set(cacheKey, { expires: Date.now() + 10_000, value });
  return value;
}

function projectContextDir(home: string, root: string): string {
  return path.join(home, 'projects', createHash('sha256').update(path.resolve(root)).digest('hex'));
}

async function projectConnection(home: string, root: string): Promise<TaskContext | undefined> {
  return (await contextsIn(path.join(home, 'projects'))).find((context) => path.resolve(context.root) === path.resolve(root));
}

function coverage(enumerated: number, total: number): Coverage { return { enumerated, total, complete: enumerated === total }; }

async function saveProjectContext(config: RuntimeConfig, context: TaskContext): Promise<void> {
  const directory = projectContextDir(config.home, context.root);
  await mkdir(directory, { recursive: true, mode: 0o700 });
  const temporary = path.join(directory, `context-${process.pid}-${Date.now()}.tmp`);
  try {
    await writeFile(temporary, `${JSON.stringify(context, null, 2)}\n`, { encoding: 'utf8', mode: 0o600 });
    await rename(temporary, path.join(directory, 'context.json'));
  } finally { await rm(temporary, { force: true }); }
  invalidateContexts(config.home);
}

async function stopProjectConnection(config: RuntimeConfig, context: TaskContext): Promise<void> {
  if (context.managedBy !== 'project-ui') throw new KCRequestError('PRECONDITION_FAILED', '当前知识由宿主配置，不能从这里移除。');
  if (context.pid) await execFileAsync(config.bin, ['stop', '--pid', String(context.pid)], { timeout: 30_000 });
  await rm(projectContextDir(config.home, context.root), { recursive: true, force: true });
  invalidateContexts(config.home);
}

async function resolveProjectPin(config: RuntimeConfig, catalog: string, workspace: string, definition?: WorkspaceDefinition): Promise<TaskPin> {
  const prefix = `/catalog/v1/catalogs/${encodeURIComponent(catalog)}`;
  const pin = await kcJSON<TaskPin>(config, definition
    ? `${prefix}/workspaces:resolve`
    : `${prefix}/workspaces/${encodeURIComponent(workspace)}/resolve`, definition
      ? { workspace: '', revision: definition.revision, sources: definition.sources } : {});
  if (!pin.pinId || pin.workspaceId !== workspace || !pin.repositories || typeof pin.repositories !== 'object' || Array.isArray(pin.repositories)) {
    throw new KCRequestError('PRECONDITION_FAILED', 'KC 返回了无效的固定版本。');
  }
  return { ...pin, catalog, ...(definition ? { definition } : {}) };
}

async function connectProject(config: RuntimeConfig, cwd: string, catalog: string, workspace: string, repositories?: string[]): Promise<TaskContext> {
  const hostContext = await contextFor(config.home, cwd);
  if (!hostContext) throw new KCRequestError('PRECONDITION_FAILED', '当前目录不是一个活动项目。');
  if (hostContext.pid && hostContext.managedBy !== 'project-ui') {
    throw new KCRequestError('PRECONDITION_FAILED', '当前项目由宿主配置了默认知识集，请先移除宿主默认配置再切换。');
  }
  const inventory = await discoverKnowledge(config);
  const catalogInventory = inventory.catalogs.find((item) => item.id === catalog);
  let definition: WorkspaceDefinition | undefined;
  if (repositories) {
    const selected = [...new Set(repositories)];
    if (!selected.length || selected.length > 100 || selected.some((id) => !catalogInventory?.repositories.some((item) => item.id === id))) {
      throw new KCRequestError('WORKSPACE_INVALID', '请选择同一 Catalog 中可见的知识源。');
    }
    // The public default selector is verified against snapshot.DefaultRef by the Server transport contract test.
    definition = { workspaceId: '', revision: 1, sources: selected.map((repository) => ({ repository, selector: 'refs/heads/main' })) };
  } else if (!catalogInventory?.knowledgeSets.some((item) => item.id === workspace)) {
    throw new KCRequestError('WORKSPACE_INVALID', `知识集 ${workspace} 不可见或已退役。`);
  }
  const pin = await resolveProjectPin(config, catalog, workspace, definition);
  if (hostContext.pid) throw new KCRequestError('PRECONDITION_FAILED', '请先移除当前文件挂载，再切换知识源；当前固定版本已保留。');
  const context: TaskContext = {
    version: 1, server: config.server, principal: config.authorization ? undefined : config.principal,
    authMode: config.authorization ? 'token' : 'local', catalog, dataset: workspace, workspace, pinId: pin.pinId, pin,
    root: path.resolve(hostContext.root), readOnly: true, managedBy: 'project-ui', mounts: [],
  };
  await saveProjectContext(config, context);
  return context;
}

async function mountProjectFiles(config: RuntimeConfig, context: TaskContext): Promise<void> {
  if (context.server && context.server !== config.server) throw new KCRequestError('PRECONDITION_FAILED', '当前知识连接属于另一服务，请切回该服务。');
  if (context.pid) return;
  if (!context.workspace) throw new KCRequestError('CAPABILITY_UNSATISFIED', '自主选择的知识源可直接搜索和读取；文件挂载需要带目录布局的命名知识集。');
  const args = ['daemon-mount', '--server', config.server, '--view', config.view, '--dataset', context.workspace, '--root', context.root];
  if (!config.authorization && config.principal) args.push('--as', config.principal);
  if (context.catalog) args.push('--catalog', context.catalog);
  let manifest: MountManifest;
  try {
    const { stdout } = await execFileAsync(config.bin, args, { timeout: 60_000, maxBuffer: 4 << 20 });
    manifest = JSON.parse(stdout) as MountManifest;
  } catch (error) {
    const failure = error as { stderr?: string | Buffer };
    throw new KCRequestError('CAPABILITY_UNSATISFIED', `文件挂载未建立，结构化知识仍可使用：${String(failure.stderr ?? error).trim()}`);
  }
  const setId = manifest.setId || manifest.workspaceId;
  if (manifest.pinId !== context.pinId || setId !== context.workspace || path.resolve(manifest.root) !== context.root || !Number.isSafeInteger(manifest.pid) || manifest.pid <= 1 || !Array.isArray(manifest.mounts)) {
    if (Number.isSafeInteger(manifest.pid) && manifest.pid > 1) await execFileAsync(config.bin, ['stop', '--pid', String(manifest.pid)], { timeout: 30_000 });
    throw new KCRequestError('PRECONDITION_FAILED', 'kcfs 返回了与当前固定版本不一致的挂载清单。');
  }
  try { await saveProjectContext(config, { ...context, pid: manifest.pid, mounts: manifest.mounts }); }
  catch (error) { await execFileAsync(config.bin, ['stop', '--pid', String(manifest.pid)], { timeout: 30_000 }); throw error; }
}

// Pending updates stay host-side. Adoption reuses exactly the inspected pin and
// refuses a stale current basis instead of resolving latest a second time.
const pendingUpdates = new Map<string, { from: string; pin: TaskPin }>();
async function projectUpdate(config: RuntimeConfig, cwd: string, adopt: boolean): Promise<unknown> {
  const context = await contextFor(config.home, cwd);
  if (!context?.pin || context.managedBy !== 'project-ui') throw new KCRequestError('PRECONDITION_FAILED', '当前项目没有可更新的自主知识连接。');
  if (context.server && context.server !== config.server) throw new KCRequestError('PRECONDITION_FAILED', '当前知识连接属于另一服务，请切回该服务。');
  const key = `${config.home}|${context.root}`;
  if (!adopt) {
    const pin = await resolveProjectPin(config, context.catalog ?? '', context.workspace, context.pin.definition);
    pendingUpdates.set(key, { from: context.pinId, pin });
    return { changed: pin.pinId !== context.pinId, current: context.pin, available: pin };
  }
  const pending = pendingUpdates.get(key);
  if (!pending || pending.from !== context.pinId) throw new KCRequestError('PRECONDITION_FAILED', '请先检查更新，再采用所显示的固定版本。');
  if (context.pid) throw new KCRequestError('PRECONDITION_FAILED', '请先移除文件挂载，再采用更新；当前固定版本已保留。');
  await saveProjectContext(config, { ...context, pinId: pending.pin.pinId, pin: pending.pin });
  pendingUpdates.delete(key);
  return { pinId: pending.pin.pinId };
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
  return async (req: IncomingMessage, res: ServerResponse): Promise<void> => {
    try {
      const config = await runtimeConfig(input);
      const home = config.home;
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
              code: error instanceof KCRequestError || error instanceof BrowserIdentityError ? error.code : 'TEMPORARY_UNAVAILABLE',
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
        const ready = !!context.pinId;
        const isEnabled = await enabled(home, context.root);
        const base = {
          workspace: context.workspace,
          ...(context.catalog ? { catalog: context.catalog } : {}),
          state: ready ? 'ready' as const : 'unbound' as const,
          ...(ready ? { pin: pinOf(context) } : {}),
          ...(context.managedBy ? { managedBy: context.managedBy } : {}),
          filesMounted: context.mounts.length > 0,
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
          if (!context || !context.pinId) throw new Error('cwd has no active knowledge connection');
          if (body.enabled && !context.pid && context.mounts.length === 0) await mountProjectFiles(config, { ...context, managedBy: 'project-ui' });
          await setEnabled(home, context.root, body.enabled);
          send(res, 200, { preferences: { vfsEnabled: body.enabled } });
          return;
        }
        if (body.action === 'connect-workspace' || body.action === 'connect-sources') {
          if (typeof body.cwd !== 'string' || typeof body.catalog !== 'string' || (body.action === 'connect-workspace' && (typeof body.workspace !== 'string' || !body.workspace.trim())) || (body.action === 'connect-sources' && (!Array.isArray(body.repositories) || body.repositories.some((item) => typeof item !== 'string')))) {
            throw new Error('connect-workspace requires cwd, catalog and workspace');
          }
          const context = await connectProject(config, body.cwd, body.catalog, typeof body.workspace === 'string' ? body.workspace : '', body.action === 'connect-sources' ? body.repositories as string[] : undefined);
          send(res, 200, { connection: { workspace: context.workspace, pinId: context.pinId, mounts: context.mounts } });
          return;
        }
        if (body.action === 'check-updates' || body.action === 'adopt-update') {
          if (typeof body.cwd !== 'string') throw new Error('update requires cwd');
          send(res, 200, { update: await projectUpdate(config, body.cwd, body.action === 'adopt-update') });
          return;
        }
        if (body.action === 'unmount-files') {
          if (typeof body.cwd !== 'string') throw new Error('unmount-files requires cwd');
          const context = await contextFor(home, body.cwd);
          if (!context || context.managedBy !== 'project-ui') throw new KCRequestError('PRECONDITION_FAILED', '当前文件由宿主管理。');
          if (context.pid) await execFileAsync(config.bin, ['stop', '--pid', String(context.pid)], { timeout: 30_000 });
          await saveProjectContext(config, { ...context, pid: undefined, mounts: [] });
          await setEnabled(home, context.root, false);
          send(res, 200, { filesMounted: false });
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
      send(res, 400, { error: { code: error instanceof KCRequestError || error instanceof BrowserIdentityError ? error.code : 'USAGE_INVALID', message: error instanceof Error ? error.message : String(error) } });
    }
  };
}

export function apply(ctx: Context, config: LoomBrowserConfig): void {
  const webServer = (ctx as unknown as { webServer: { register(route: { kind: 'exact'; path: string; handler: ReturnType<typeof createLoomWorkspaceHandler> }): () => void } }).webServer;
  ctx.effect(() => webServer.register({ kind: 'exact', path: ROUTE, handler: createLoomWorkspaceHandler(config) }), 'dsh-loom: mounted-files host bridge');
}
