import { createServer, type Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ensureWorkspaceAnchor } from '../src/binding.js';
import { createLoomWorkspaceHandler, LoomBrowserApi } from '../src/web.js';

function listen(server: Server): Promise<string> {
  return new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      server.off('error', reject);
      const address = server.address() as AddressInfo;
      resolve(`http://127.0.0.1:${address.port}`);
    });
  });
}

function close(server: Server): Promise<void> {
  return new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
}

function sendJson(res: import('node:http').ServerResponse, value: unknown): void {
  const body = JSON.stringify(value);
  res.writeHead(200, { 'content-type': 'application/json', 'content-length': Buffer.byteLength(body) });
  res.end(body);
}

describe('read-only VFS browser API', () => {
  const roots: string[] = [];
  afterEach(async () => {
    vi.unstubAllEnvs();
    await Promise.all(roots.splice(0).map((root) => rm(root, { recursive: true, force: true })));
  });

  it('uses a fresh Workspace resolution for every refresh/read', async () => {
    let clients = 0;
    const api = new LoomBrowserApi({
      baseURL: 'http://127.0.0.1:7380',
      workspace: 'warehouse',
      catalog: 'kr://acme/catalog',
    }, () => {
      clients += 1;
      return {
        listing: vi.fn(async () => ({
          entries: [{ path: 'public/a.md', repository: 'kr://public', commit: `c${clients}` }],
          mounts: [{ path: '', repository: 'kr://public', selector: 'refs/heads/main', commit: `c${clients}` }],
        })),
        read: vi.fn(async (path: string) => ({
          path,
          repository: 'kr://public',
          commit: `c${clients}`,
          content: new TextEncoder().encode('hello\n'),
        })),
      };
    });

    expect(await api.list()).toMatchObject({
      workspace: 'warehouse',
      catalog: 'kr://acme/catalog',
      state: 'ready',
      entries: [{ commit: 'c1' }],
      mounts: [{ path: '', repository: 'kr://public', commit: 'c1' }],
    });
    expect(await api.read('public/a.md')).toMatchObject({
      path: 'public/a.md',
      repository: 'kr://public',
      commit: 'c2',
      size: 6,
      binary: false,
      content: 'hello\n',
    });
    expect(clients).toBe(2);
  });

  it('projects a missing kc home as a first-run state instead of a raw error', async () => {
    const api = new LoomBrowserApi({
      baseURL: 'http://127.0.0.1:7380', workspace: 'warehouse',
    }, () => ({
      listing: vi.fn(async () => {
        throw new (await import('../src/client.js')).LoomError(
          'no kc home at /tmp/empty; run: kc init --home /tmp/empty',
          'USAGE_INVALID',
        );
      }),
      read: vi.fn(),
    }));

    await expect(api.list()).resolves.toEqual({
      workspace: 'warehouse',
      state: 'uninitialized',
      entries: [],
      mounts: [],
    });
  });

  it('does not serialize binary content into the browser response', async () => {
    const api = new LoomBrowserApi({
      baseURL: 'http://127.0.0.1:7380', workspace: 'warehouse',
    }, () => ({
      listing: vi.fn(async () => ({ entries: [], mounts: [] })),
      read: vi.fn(async (path: string) => ({
        path,
        repository: 'kr://private',
        commit: 'deadbeef',
        content: Uint8Array.from([1, 0, 2, 3]),
      })),
    }));

    expect(await api.read('image.bin')).toEqual({
      path: 'image.bin',
      repository: 'kr://private',
      commit: 'deadbeef',
      size: 4,
      binary: true,
      truncated: false,
    });
  });

  it('rejects an empty path before contacting kc', async () => {
    const create = vi.fn();
    const api = new LoomBrowserApi({
      baseURL: 'http://127.0.0.1:7380', workspace: 'warehouse',
    }, create);
    await expect(api.read(' / ')).rejects.toMatchObject({ code: 'USAGE_INVALID' });
    expect(create).not.toHaveBeenCalled();
  });

  it('uses the host KC_AUTH_TOKEN without exposing it to the browser', async () => {
    vi.stubEnv('KC_AUTH_TOKEN', 'secret-pat');
    const requests: Array<{ url: string; authorization: string }> = [];
    const kc = createServer((req, res) => {
      requests.push({ url: String(req.url), authorization: String(req.headers.authorization ?? '') });
      if (req.url === '/v1/status') {
        const body = JSON.stringify({ error: { code: 'FORBIDDEN', message: 'consumer cannot administer kc' } });
        res.writeHead(403, { 'content-type': 'application/json', 'content-length': Buffer.byteLength(body) });
        res.end(body);
        return;
      }
      sendJson(res, {
        workspaceId: 'warehouse', revision: 1,
        repositories: { 'kr://local/workbench': 'abc123' },
      });
    });
    const kcURL = await listen(kc);
    const bridge = createServer(createLoomWorkspaceHandler({
      baseURL: kcURL,
      workspace: '',
      catalog: 'kr://acme/catalog',
      suggestedWorkspace: 'warehouse',
    }));
    const bridgeURL = await listen(bridge);
    try {
      const response = await fetch(`${bridgeURL}/api/loom/vfs`);
      expect(response.ok).toBe(true);
      const body = await response.text();
      expect(requests).toEqual([
        { url: '/v1/status', authorization: 'Bearer secret-pat' },
        { url: '/v1/resolve', authorization: 'Bearer secret-pat' },
      ]);
      expect(body).not.toContain('secret-pat');
      expect(JSON.parse(body)).toMatchObject({
        state: 'unbound',
        available: [{ catalog: 'kr://acme/catalog', workspace: 'warehouse', revision: 1 }],
      });
    } finally {
      await close(bridge);
      await close(kc);
    }
  });

  it('normalizes an empty home before creating a Workspace anchor', async () => {
    const home = await mkdtemp(path.join(os.tmpdir(), 'dsh-loom-web-home-'));
    roots.push(home);
    vi.stubEnv('KC_HOME', home);
    const kc = createServer((req, res) => {
      if (req.url === '/v1/resolve') {
        sendJson(res, {
          workspaceId: 'warehouse', revision: 1,
          repositories: { 'kr://acme/public': 'abc123' },
        });
        return;
      }
      res.writeHead(404).end();
    });
    const kcURL = await listen(kc);
    const bridge = createServer(createLoomWorkspaceHandler({
      baseURL: kcURL,
      home: '',
      workspace: '',
    }));
    const bridgeURL = await listen(bridge);
    try {
      const response = await fetch(`${bridgeURL}/api/loom/vfs`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          action: 'activate', catalog: 'kr://acme/catalog', workspace: 'warehouse',
        }),
      });
      expect(response.ok).toBe(true);
      const body = await response.json() as { anchor: string };
      expect(body.anchor.startsWith(`${path.join(home, 'agent-workspaces')}${path.sep}`)).toBe(true);
      await expect(readFile(path.join(body.anchor, '.dsh-loom-workspace.json'), 'utf8')).resolves.toContain('warehouse');
    } finally {
      await close(bridge);
      await close(kc);
    }
  });

  it('keeps every Catalog Workspace available after the current Session is bound', async () => {
    const home = await mkdtemp(path.join(os.tmpdir(), 'dsh-loom-web-switch-'));
    roots.push(home);
    const anchor = await ensureWorkspaceAnchor(home, {
      catalog: 'kr://acme/catalog',
      workspace: 'warehouse',
    });
    let bindingUnavailable = false;
    const kc = createServer(async (req, res) => {
      const chunks: Buffer[] = [];
      for await (const chunk of req) chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
      const request = JSON.parse(Buffer.concat(chunks).toString('utf8') || '{}') as Record<string, unknown>;
      if (req.url === '/v1/status') {
        if (request.catalog === 'kr://other/catalog') {
          sendJson(res, {
            catalog: { repositoryId: 'kr://other/catalog' },
            workspaces: [{ workspaceId: 'research', revision: 4 }],
          });
        } else {
          sendJson(res, {
            catalog: { repositoryId: 'kr://acme/catalog' },
            catalogs: [{ id: 'kr://acme/catalog' }, { id: 'kr://other/catalog' }],
            workspaces: [
              { workspaceId: 'warehouse', revision: 2 },
              { workspaceId: 'analytics', revision: 3 },
            ],
          });
        }
        return;
      }
      if (req.url === '/v1/resolve') {
        if (bindingUnavailable) {
          const body = JSON.stringify({ error: { code: 'USAGE_INVALID', message: 'unknown catalog kr://acme/catalog' } });
          res.writeHead(400, { 'content-type': 'application/json', 'content-length': Buffer.byteLength(body) });
          res.end(body);
          return;
        }
        sendJson(res, {
          workspaceId: 'warehouse', revision: 2,
          repositories: { 'kr://acme/public': 'abc123' },
        });
        return;
      }
      if (req.url === '/v1/vfs-list') {
        sendJson(res, { entries: [], mounts: [] });
        return;
      }
      res.writeHead(404).end();
    });
    const kcURL = await listen(kc);
    const bridge = createServer(createLoomWorkspaceHandler({
      baseURL: kcURL,
      workspace: '',
      home,
    }));
    const bridgeURL = await listen(bridge);
    try {
      const response = await fetch(`${bridgeURL}/api/loom/vfs?cwd=${encodeURIComponent(anchor)}`);
      expect(response.ok).toBe(true);
      await expect(response.json()).resolves.toMatchObject({
        state: 'ready',
        catalog: 'kr://acme/catalog',
        workspace: 'warehouse',
        available: [
          { catalog: 'kr://acme/catalog', workspace: 'warehouse', revision: 2 },
          { catalog: 'kr://acme/catalog', workspace: 'analytics', revision: 3 },
          { catalog: 'kr://other/catalog', workspace: 'research', revision: 4 },
        ],
      });

      bindingUnavailable = true;
      const stale = await fetch(`${bridgeURL}/api/loom/vfs?cwd=${encodeURIComponent(anchor)}`);
      expect(stale.ok).toBe(true);
      await expect(stale.json()).resolves.toMatchObject({
        state: 'unavailable',
        catalog: 'kr://acme/catalog',
        workspace: 'warehouse',
        bindingError: { code: 'USAGE_INVALID', message: 'unknown catalog kr://acme/catalog' },
        available: [
          { catalog: 'kr://acme/catalog', workspace: 'warehouse', revision: 2 },
          { catalog: 'kr://acme/catalog', workspace: 'analytics', revision: 3 },
          { catalog: 'kr://other/catalog', workspace: 'research', revision: 4 },
        ],
      });
    } finally {
      await close(bridge);
      await close(kc);
    }
  });
});
