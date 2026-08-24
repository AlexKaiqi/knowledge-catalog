import { describe, expect, it, vi } from 'vitest';
import { LoomBrowserApi } from '../src/web.js';

describe('read-only VFS browser API', () => {
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
});
