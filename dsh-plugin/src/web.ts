/**
 * Read-only host bridge for the human-facing VFS browser.
 *
 * The browser calls this same-origin route instead of kc serve directly. That
 * keeps KC_AUTH_TOKEN in the DSH host process while reusing the exact same
 * resolve + vfs-list/vfs-read protocol as the agent-facing filesystem.
 */

import type { Context } from '@deepseek-ai/cordis';
import type { IncomingMessage, ServerResponse } from 'node:http';
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
  entries: LoomFileEntry[];
  mounts: LoomMount[];
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
    // Agent writes performed by loom-fs therefore appear after Refresh.
    this.createVfs = createVfs ?? (() => new LoomVfs(config));
  }

  async list(prefix?: string): Promise<LoomBrowserList> {
    const listing = await this.createVfs().listing(prefix?.trim() || undefined);
    return {
      workspace: this.workspace,
      ...(this.catalog ? { catalog: this.catalog } : {}),
      ...listing,
    };
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

export function apply(ctx: Context, config: LoomBrowserConfig): void {
  const webServer = (ctx as unknown as {
    webServer: { register(route: {
      kind: 'exact';
      path: string;
      handler: (req: IncomingMessage, res: ServerResponse) => void | Promise<void>;
    }): () => void };
  }).webServer;
  const api = new LoomBrowserApi({
    baseURL: config.baseURL || 'http://127.0.0.1:7380',
    workspace: config.workspace || 'notes',
    catalog: config.catalog || undefined,
    as: config.as || undefined,
    authToken: config.authToken || undefined,
  });
  ctx.effect(() => webServer.register({
    kind: 'exact',
    path: ROUTE,
    handler: createLoomBrowserHandler(api),
  }), 'loom-web: read-only VFS browser route');
}
