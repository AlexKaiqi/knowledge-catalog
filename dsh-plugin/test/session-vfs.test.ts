import { describe, expect, it, vi } from 'vitest';
import { vfsForTask } from '../src/session-vfs.js';

function response(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'content-type': 'application/json' } });
}

describe('Agent task Workspace pin', () => {
  it('shares one client and one resolve across fs/search in a task, then refreshes for a new task', async () => {
    let resolves = 0;
    const fetchImpl = vi.fn(async (url: string, init?: RequestInit) => {
      const body = JSON.parse(String(init?.body)) as Record<string, unknown>;
      if (String(url).endsWith('/resolve')) {
        resolves += 1;
        return response({ workspaceId: 'notes', revision: 1, repositories: { 'kr://knowledge': `commit-${resolves}` } });
      }
      const pin = body.pin as { repositories: Record<string, string> };
      return response({
        path: String(body.path), repository: 'kr://knowledge',
        commit: pin.repositories['kr://knowledge'], content: Buffer.from(pin.repositories['kr://knowledge']).toString('base64'),
      });
    });
    const config = {
      baseURL: 'http://kc', workspace: 'notes', fetchImpl: fetchImpl as unknown as typeof fetch,
    };
    const firstTask = new AbortController().signal;
    const fsClient = vfsForTask(config, { workspace: 'notes' }, firstTask);
    const searchClient = vfsForTask(config, { workspace: 'notes' }, firstTask);
    expect(searchClient).toBe(fsClient);
    expect(new TextDecoder().decode((await fsClient.read('a.md')).content)).toBe('commit-1');
    expect(new TextDecoder().decode((await searchClient.read('b.md')).content)).toBe('commit-1');
    expect(resolves).toBe(1);

    const secondTask = new AbortController().signal;
    const freshClient = vfsForTask(config, { workspace: 'notes' }, secondTask);
    expect(freshClient).not.toBe(fsClient);
    expect(new TextDecoder().decode((await freshClient.read('a.md')).content)).toBe('commit-2');
    expect(resolves).toBe(2);
  });
});
