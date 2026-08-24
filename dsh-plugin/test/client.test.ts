import { describe, expect, it, vi } from 'vitest';
import { LoomVfs, LoomError } from '../src/client.js';

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

describe('LoomVfs HTTP client', () => {
  it('posts workspace (not view) on vfs-list / vfs-read / vfs-write', async () => {
    const seen: { url: string; body: Record<string, unknown> }[] = [];
    const fetchImpl = vi.fn(async (url: string, init?: RequestInit) => {
      seen.push({ url: String(url), body: JSON.parse(String(init?.body)) });
      if (String(url).endsWith('/resolve')) {
        return jsonResponse(200, {
          workspaceId: 'notes', revision: 1, repositories: { 'kr://x': 'aaa' }, pinId: 'pin-a',
        });
      }
      if (String(url).endsWith('/vfs-list')) {
        return jsonResponse(200, {
          entries: [{ path: 'a.md', repository: 'kr://x', commit: 'aaa' }],
          mounts: [{ path: '', repository: 'kr://x', selector: 'refs/heads/main', commit: 'aaa' }],
        });
      }
      if (String(url).endsWith('/vfs-read')) {
        return jsonResponse(200, {
          path: 'a.md',
          repository: 'kr://x',
          commit: 'aaa',
          content: Buffer.from('hi\n').toString('base64'),
        });
      }
      return jsonResponse(200, { result: { repositoryId: 'kr://x', targetRef: 'refs/heads/main', oldCommit: 'aaa', newCommit: 'bbb' } });
    });

    const vfs = new LoomVfs({
      baseURL: 'http://127.0.0.1:7380/',
      workspace: 'notes',
      catalog: 'kr://acme/catalog',
      as: 'bot',
      fetchImpl: fetchImpl as unknown as typeof fetch,
    });

    expect(await vfs.listing()).toEqual({
      entries: [{ path: 'a.md', repository: 'kr://x', commit: 'aaa' }],
      mounts: [{ path: '', repository: 'kr://x', selector: 'refs/heads/main', commit: 'aaa' }],
    });
    const file = await vfs.read('a.md');
    expect(new TextDecoder().decode(file.content)).toBe('hi\n');
    await vfs.write('a.md', new TextEncoder().encode('next\n'), { base: 'aaa', commandId: 'cmd-1' });
    await vfs.read('a.md');

    expect(seen.map((s) => s.url)).toEqual([
      'http://127.0.0.1:7380/v1/resolve',
      'http://127.0.0.1:7380/v1/vfs-list',
      'http://127.0.0.1:7380/v1/vfs-read',
      'http://127.0.0.1:7380/v1/vfs-write',
      'http://127.0.0.1:7380/v1/vfs-read',
    ]);
    for (const call of seen) {
      expect(call.body.workspace).toBe('notes');
      expect(call.body.view).toBeUndefined();
      expect(call.body.catalog).toBe('kr://acme/catalog');
    }
    for (const call of seen.slice(1)) {
      expect((call.body.pin as { repositories: Record<string, string> }).repositories['kr://x']).toBeDefined();
    }
    expect(seen[3].body.base).toBe('aaa');
    expect(seen[3].body['command-id']).toBe('cmd-1');
    expect((seen[3].body.pin as { repositories: Record<string, string> }).repositories['kr://x']).toBe('aaa');
    expect((seen[4].body.pin as { repositories: Record<string, string> }).repositories['kr://x']).toBe('bbb');
  });

  it('requires Workspace and has no legacy composition fallback', () => {
    expect(() => new LoomVfs({ baseURL: 'http://127.0.0.1:7380', workspace: '' })).toThrow(
      'workspace is required',
    );
  });

  it('uses a verified-service token instead of a claimed principal', async () => {
    const fetchImpl = vi.fn(async (_url: string, init?: RequestInit) => {
      const headers = init?.headers as Record<string, string>;
      expect(headers.Authorization).toBe('Bearer secret-pat');
      expect(headers['X-Kc-As']).toBeUndefined();
      return jsonResponse(200, { workspaceId: 'notes', repositories: { 'kr://x': 'aaa' } });
    });
    const vfs = new LoomVfs({
      baseURL: 'http://127.0.0.1:7380', workspace: 'notes', authToken: 'secret-pat',
      fetchImpl: fetchImpl as unknown as typeof fetch,
    });
    await vfs.list();
  });

  it('rejects simultaneous claimed and authenticated identities', () => {
    expect(() => new LoomVfs({
      baseURL: 'http://127.0.0.1:7380', workspace: 'notes', as: 'bot', authToken: 'pat',
    })).toThrow('mutually exclusive');
  });

  it('maps kc error envelopes onto LoomError codes', async () => {
    const fetchImpl = vi.fn(async (url: string) => {
      if (String(url).endsWith('/resolve')) {
        return jsonResponse(200, { workspaceId: 'notes', revision: 1, repositories: { 'kr://x': 'aaa' } });
      }
      return jsonResponse(409, { error: { code: 'NON_FAST_FORWARD', message: 'ref moved' } });
    });
    const vfs = new LoomVfs({
      baseURL: 'http://127.0.0.1:7380',
      workspace: 'notes',
      fetchImpl: fetchImpl as unknown as typeof fetch,
    });
    await expect(vfs.read('a.md')).rejects.toMatchObject({ name: 'LoomError', code: 'NON_FAST_FORWARD' });
    expect(LoomError).toBeDefined();
  });

  it('refreshes only the conflicted repository pin after NON_FAST_FORWARD', async () => {
    let resolves = 0;
    let writes = 0;
    const seen: Record<string, unknown>[] = [];
    const fetchImpl = vi.fn(async (url: string, init?: RequestInit) => {
      const body = JSON.parse(String(init?.body)) as Record<string, unknown>;
      seen.push(body);
      if (String(url).endsWith('/resolve')) {
        resolves += 1;
        return jsonResponse(200, {
          workspaceId: 'notes',
          revision: 1,
          repositories: { 'kr://root': 'root-a', 'kr://semantic': resolves === 1 ? 'sem-a' : 'sem-b' },
        });
      }
      if (String(url).endsWith('/vfs-read')) {
        const pin = body.pin as { repositories: Record<string, string> };
        return jsonResponse(200, {
          path: 'refs/semantic/a.md', repository: 'kr://semantic',
          commit: pin.repositories['kr://semantic'], content: Buffer.from('value\n').toString('base64'),
        });
      }
      writes += 1;
      if (writes === 1) {
        return jsonResponse(409, { error: { code: 'NON_FAST_FORWARD', message: 'ref moved' } });
      }
      return jsonResponse(200, {
        result: { repositoryId: 'kr://semantic', oldCommit: 'sem-b', newCommit: 'sem-c', targetRef: 'refs/heads/main' },
      });
    });
    const vfs = new LoomVfs({
      baseURL: 'http://127.0.0.1:7380', workspace: 'notes',
      fetchImpl: fetchImpl as unknown as typeof fetch,
    });
    await vfs.read('refs/semantic/a.md');
    await expect(vfs.write('refs/semantic/a.md', new TextEncoder().encode('next\n'), { base: 'sem-a' }))
      .rejects.toMatchObject({ code: 'NON_FAST_FORWARD' });
    await vfs.read('refs/semantic/a.md');
    const lastPin = seen.at(-1)?.pin as { repositories: Record<string, string> };
    expect(lastPin.repositories).toEqual({ 'kr://root': 'root-a', 'kr://semantic': 'sem-b' });
    expect(resolves).toBe(2);
  });
});
