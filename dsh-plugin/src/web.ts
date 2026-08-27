/**
 * Read-only host bridge for the human-facing VFS browser.
 *
 * The browser calls this same-origin route instead of kc serve directly. That
 * keeps KC_AUTH_TOKEN in the DSH host process while reusing kc serve's
 * resolve + vfs-list/vfs-read observation protocol. Agent file I/O uses the
 * Linux host mounts instead.
 */

import type { Context } from '@deepseek-ai/cordis';
import type { IncomingMessage, ServerResponse } from 'node:http';
import {
  ensureWorkspaceAnchor,
  readWorkspaceBinding,
  resolveKcHome,
  type LoomWorkspaceBinding,
} from './binding.js';
import {
  LoomError,
  LoomVfs,
  type LoomFileEntry,
  type LoomMount,
  type LoomVfsConfig,
  type LoomVfsListing,
} from './client.js';

export const name = 'loom-web';
export const inject = ['webServer'];

const ROUTE = '/api/loom/vfs';
const MAX_PREVIEW_BYTES = 512 * 1024;

export type LoomBrowserConfig = Omit<LoomVfsConfig, 'fetchImpl' | 'materializeRoot'>;

export interface LoomBrowserList {
  workspace: string;
  catalog?: string;
  state: 'ready' | 'uninitialized' | 'unbound' | 'unavailable';
  bindingError?: { code: string; message: string };
  available?: Array<{ catalog: string; workspace: string; revision: number }>;
  entries: LoomFileEntry[];
  mounts: LoomMount[];
}

interface LoomWebConfig extends LoomBrowserConfig {
  home?: string;
  suggestedWorkspace?: string;
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

interface BrowserVfs {
  listing(prefix?: string): Promise<LoomVfsListing>;
  read(path: string): Promise<{
    path: string;
    repository: string;
    commit: string;
    content: Uint8Array;
  }>;
}

type VfsFactory = () => BrowserVfs;

function looksBinary(bytes: Uint8Array): boolean {
  const sample = bytes.subarray(0, Math.min(bytes.byteLength, 8192));
  for (const byte of sample) {
    if (byte === 0) return true;
  }
  return false;
}

export class LoomBrowserApi {
  private readonly workspace: string;
  private readonly catalog?: string;
  private readonly createVfs: VfsFactory;

  constructor(config: LoomBrowserConfig, createVfs?: VfsFactory) {
    this.workspace = config.workspace;
    this.catalog = config.catalog?.trim() || undefined;
    // A fresh client per request deliberately resolves a fresh Workspace pin.
    // External governed writes therefore appear after Refresh.
    this.createVfs = createVfs ?? (() => new LoomVfs(config));
  }

  async list(prefix?: string): Promise<LoomBrowserList> {
    try {
      const listing = await this.createVfs().listing(prefix?.trim() || undefined);
      return {
        workspace: this.workspace,
        ...(this.catalog ? { catalog: this.catalog } : {}),
        state: 'ready',
        ...listing,
      };
    } catch (error) {
      // A missing kc home is the expected first-run state. Keep the protocol
      // error away from the human surface while preserving every other error.
      if (error instanceof LoomError && error.code === 'USAGE_INVALID' && /no kc home\b/i.test(error.message)) {
        return {
          workspace: this.workspace,
          ...(this.catalog ? { catalog: this.catalog } : {}),
          state: 'uninitialized',
          entries: [],
          mounts: [],
        };
      }
      throw error;
    }
  }

  async read(path: string): Promise<LoomBrowserRead> {
    const normalized = path.trim().replace(/^\/+/, '');
    if (!normalized || normalized.includes('\0')) {
      throw new LoomError('path is required', 'USAGE_INVALID');
    }
    const file = await this.createVfs().read(normalized);
    const binary = looksBinary(file.content);
    const preview = file.content.subarray(0, MAX_PREVIEW_BYTES);
    return {
      path: file.path,
      repository: file.repository,
      commit: file.commit,
      size: file.content.byteLength,
      binary,
      truncated: file.content.byteLength > preview.byteLength,
      ...(binary ? {} : { content: new TextDecoder().decode(preview) }),
    };
  }
}

function send(res: ServerResponse, status: number, body: unknown): void {
  const encoded = JSON.stringify(body);
  res.writeHead(status, {
    'content-type': 'application/json; charset=utf-8',
    'cache-control': 'no-store',
    'content-length': Buffer.byteLength(encoded),
  });
  res.end(encoded);
}

function errorEnvelope(error: unknown): { error: { code: string; message: string } } {
  if (error instanceof LoomError) {
    return { error: { code: error.code, message: error.message } };
  }
  return {
    error: {
      code: 'UNKNOWN',
      message: error instanceof Error ? error.message : String(error),
    },
  };
}

export function createLoomBrowserHandler(api: LoomBrowserApi) {
  return async (req: IncomingMessage, res: ServerResponse): Promise<void> => {
    if (req.method !== 'GET') {
      send(res, 405, { error: { code: 'USAGE_INVALID', message: 'GET required' } });
      return;
    }
    try {
      const url = new URL(req.url ?? ROUTE, 'http://dsh.local');
      const path = url.searchParams.get('path');
      if (path !== null) {
        send(res, 200, await api.read(path));
      } else {
        send(res, 200, await api.list(url.searchParams.get('prefix') ?? undefined));
      }
    } catch (error) {
      const envelope = errorEnvelope(error);
      const status = envelope.error.code === 'UNAUTHENTICATED' ? 401
        : envelope.error.code === 'FORBIDDEN' ? 403
          : envelope.error.code === 'KNOWLEDGE_REF_UNRESOLVED' ? 404
            : 400;
      send(res, status, envelope);
    }
  };
}

function headers(config: LoomWebConfig): Record<string, string> {
  const out: Record<string, string> = { 'content-type': 'application/json' };
  const authToken = config.authToken?.trim() || process.env.KC_AUTH_TOKEN?.trim() || undefined;
  if (authToken) out.Authorization = `Bearer ${authToken}`;
  else if (config.as) out['X-Kc-As'] = config.as;
  return out;
}

async function kcCall(config: LoomWebConfig, verb: string, flags: Record<string, unknown>): Promise<any> {
  const response = await fetch(`${config.baseURL.replace(/\/$/, '')}/v1/${verb}`, {
    method: 'POST', headers: headers(config), body: JSON.stringify(flags),
  });
  const value = await response.json().catch(() => ({})) as any;
  if (!response.ok) {
    throw new LoomError(String(value?.error?.message ?? `${verb} failed`), String(value?.error?.code ?? 'UNKNOWN'));
  }
  return value;
}

async function availableWorkspaces(config: LoomWebConfig): Promise<LoomBrowserList['available']> {
  let root: any;
  try {
    root = await kcCall(config, 'status', {});
  } catch (error) {
    if (error instanceof LoomError && error.code === 'USAGE_INVALID' && /no kc home\b/i.test(error.message)) return undefined;
    // Authenticated consumers cannot call the administrative status surface.
    // A configured Workspace is already a stable capability: resolve it through
    // the consumer surface instead of widening the user's privileges merely so
    // the workbench can populate its selector.
    const catalog = config.catalog?.trim();
    const workspace = config.suggestedWorkspace?.trim();
    if (error instanceof LoomError && error.code === 'FORBIDDEN' && catalog && workspace) {
      const resolved = await kcCall(config, 'resolve', { catalog, workspace });
      return [{
        catalog,
        workspace: String(resolved.workspaceId ?? workspace),
        revision: Number(resolved.revision ?? 0),
      }];
    }
    throw error;
  }
  const catalogs = Array.isArray(root.catalogs) ? root.catalogs : [];
  const rows: NonNullable<LoomBrowserList['available']> = [];
  for (const item of catalogs) {
    const catalog = String(item.id ?? '');
    if (!catalog) continue;
    const state = root.catalog?.repositoryId === catalog ? root : await kcCall(config, 'status', { catalog });
    for (const workspace of Array.isArray(state.workspaces) ? state.workspaces : []) {
      if (workspace?.retired) continue;
      rows.push({ catalog, workspace: String(workspace.workspaceId), revision: Number(workspace.revision ?? 0) });
    }
  }
  return rows;
}

async function readBody(req: IncomingMessage): Promise<Record<string, unknown>> {
  const chunks: Buffer[] = [];
  for await (const chunk of req) chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
  if (chunks.reduce((n, chunk) => n + chunk.length, 0) > 64 * 1024) throw new LoomError('request too large', 'USAGE_INVALID');
  const value = JSON.parse(Buffer.concat(chunks).toString('utf8') || '{}') as unknown;
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new LoomError('JSON object required', 'USAGE_INVALID');
  return value as Record<string, unknown>;
}

function required(body: Record<string, unknown>, key: string): string {
  const value = body[key];
  if (typeof value !== 'string' || !value.trim()) throw new LoomError(`${key} is required`, 'USAGE_INVALID');
  return value.trim();
}

function workspaceApi(config: LoomWebConfig, binding: LoomWorkspaceBinding): LoomBrowserApi {
  return new LoomBrowserApi({ ...config, workspace: binding.workspace, catalog: binding.catalog });
}

export function createLoomWorkspaceHandler(input: LoomWebConfig) {
  const config: LoomWebConfig = { ...input, home: resolveKcHome(input.home) };
  return async (req: IncomingMessage, res: ServerResponse): Promise<void> => {
    try {
      const url = new URL(req.url ?? ROUTE, 'http://dsh.local');
      if (req.method === 'GET') {
        const cwd = url.searchParams.get('cwd') ?? undefined;
        const binding = await readWorkspaceBinding(cwd);
        if (!binding) {
          const available = await availableWorkspaces(config);
          send(res, 200, {
            workspace: config.suggestedWorkspace || '',
            state: available === undefined ? 'uninitialized' : 'unbound',
            entries: [], mounts: [], ...(available ? { available } : {}),
          });
          return;
        }
        const api = workspaceApi(config, binding);
        const filePath = url.searchParams.get('path');
        if (filePath !== null) {
          send(res, 200, await api.read(filePath));
          return;
        }
        // A bound DSH host Session stays pinned to its current Catalog Workspace, but
        // the human surface must still know the other launch targets. Choosing
        // one creates/opens that target's independent DSH Workspace/Session;
        // it never rewrites this host Session's binding. A stale binding is also a
        // launch state: returning its error alongside the available targets is
        // the only way the operator can recover without deleting host state.
        const available = await availableWorkspaces(config) ?? [];
        try {
          send(res, 200, { ...await api.list(), available });
        } catch (error) {
          send(res, 200, {
            workspace: binding.workspace,
            ...(binding.catalog ? { catalog: binding.catalog } : {}),
            state: 'unavailable',
            bindingError: errorEnvelope(error).error,
            available,
            entries: [],
            mounts: [],
          });
        }
        return;
      }
      if (req.method === 'POST') {
        const body = await readBody(req);
        const action = required(body, 'action');
        const binding: LoomWorkspaceBinding = {
          catalog: required(body, 'catalog'), workspace: required(body, 'workspace'),
        };
        if (action === 'create') {
          const repo = required(body, 'repository');
          const available = await availableWorkspaces(config);
          const homeState = available === undefined ? undefined : await kcCall(config, 'status', {});
          if (available === undefined) await kcCall(config, 'init', { catalog: binding.catalog });
          else if (!available.some((row) => row.catalog === binding.catalog)) await kcCall(config, 'catalog-add', { catalog: binding.catalog });
          if (!Array.isArray(homeState?.repos) || !homeState.repos.some((item: any) => item?.id === repo)) {
            await kcCall(config, 'repo-add', { repo });
          }
          const targetState = await kcCall(config, 'status', { catalog: binding.catalog });
          if (!Array.isArray(targetState.repositories) || !targetState.repositories.includes(repo)) {
            await kcCall(config, 'register', { catalog: binding.catalog, repo });
          }
          await kcCall(config, 'define-workspace', {
            catalog: binding.catalog, workspace: binding.workspace, revision: 1,
            source: [`${repo}=refs/heads/main@/`],
          });
        } else if (action !== 'activate') {
          throw new LoomError(`unknown action ${action}`, 'USAGE_INVALID');
        }
        // Resolve is both validation and a stable guarantee that an Agent will
        // never be launched against a misspelled Workspace.
        await kcCall(config, 'resolve', { catalog: binding.catalog, workspace: binding.workspace });
        const anchor = await ensureWorkspaceAnchor(config.home ?? '', binding);
        send(res, 200, { binding, anchor });
        return;
      }
      send(res, 405, { error: { code: 'USAGE_INVALID', message: 'GET or POST required' } });
    } catch (error) {
      const envelope = errorEnvelope(error);
      send(res, envelope.error.code === 'FORBIDDEN' ? 403 : 400, envelope);
    }
  };
}

export function apply(ctx: Context, config: LoomWebConfig): void {
  const webServer = (ctx as unknown as {
    webServer: { register(route: {
      kind: 'exact';
      path: string;
      handler: (req: IncomingMessage, res: ServerResponse) => void | Promise<void>;
    }): () => void };
  }).webServer;
  const resolved: LoomWebConfig = {
    baseURL: config.baseURL || 'http://127.0.0.1:7380',
    workspace: config.workspace || '',
    catalog: config.catalog || undefined,
    as: config.as || undefined,
    authToken: config.authToken || undefined,
    home: resolveKcHome(config.home),
    suggestedWorkspace: config.suggestedWorkspace || config.workspace,
  };
  ctx.effect(() => webServer.register({
    kind: 'exact',
    path: ROUTE,
    handler: createLoomWorkspaceHandler(resolved),
  }), 'loom-web: Workspace launch and VFS browser route');
}
