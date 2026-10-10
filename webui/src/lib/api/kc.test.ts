import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { decodeContent, kc } from './kc';
import config from '../../../vite.config.mts';
import { session } from './session';

beforeEach(() => { vi.stubGlobal('localStorage', window.localStorage); });
afterEach(() => { session.logout(); window.localStorage.clear(); vi.unstubAllGlobals(); });

test('file preview decodes the Go []byte JSON contract', async () => {
    vi.stubGlobal('fetch', vi.fn(async (path: string) => new Response(JSON.stringify(path === '/identity/v1/whoami' ? { principal: 'alice' } : {
        content: btoa(String.fromCharCode(...new TextEncoder().encode('茶铺\n'))),
        eof: true, totalBytes: 7,
    }), { status: 200 })));
    await session.restore({mode: 'lakefs', localAssertion: false, accepts: ['Cookie']});
    const result = await kc.file('beta', 'shared', {setId: 'shared', revision: 1, repositories: {'source': 'c1'}}, '说明.txt');
    expect(new TextDecoder().decode(decodeContent(result.content))).toBe('茶铺\n');
});

test('historical detail requests carry bounded pagination without altering the revision', async () => {
    const fetcher = vi.fn(async (path: string) => new Response(JSON.stringify(path === '/identity/v1/whoami' ? {principal: 'alice'} : {}), {status: 200}));
    vi.stubGlobal('fetch', fetcher);
    await session.restore({mode: 'lakefs', localAssertion: false, accepts: ['Cookie']});
    await kc.dataset('beta', 'shared', 1, undefined, {limit: 50, continuation: 'basis+/='});
    expect(fetcher.mock.calls[1][0]).toBe('/catalog/v1/catalogs/beta/datasets/shared?revision=1&limit=50&continuation=basis%2B%2F%3D');
});

test('build output belongs to the server that embeds the console', () => {
    expect(config({ command: 'build', mode: 'production' }).build.outDir).toBe('../server/webapp');
});

test('development uses the same UI base and closed KC API paths', () => {
    const dev = config({ command: 'serve', mode: 'development' });
    expect(dev.base).toBe('/ui/');
    if (!('server' in dev)) throw new Error('Development must configure the KC proxy');
    expect(Object.keys(dev.server.proxy)).toEqual(['/identity/v1', '/catalog/v1', '/dataset-files/v1', '/knowledge/v1', '/admin/v1', '/api/v1', '/logout', '/oidc', '^/(?!ui(?:/|$)|identity/v1|catalog/v1|dataset-files/v1|knowledge/v1|admin/v1|api/v1|logout|oidc)']);
    for (const [path, proxy] of Object.entries(dev.server.proxy)) expect(proxy.target).toBe(path.startsWith('^/') ? process.env.LAKEFS_S3_SERVER || process.env.LAKEFS_UI_SERVER || 'http://127.0.0.1:18000' : path === '/api/v1' || path === '/logout' || path === '/oidc' ? process.env.LAKEFS_UI_SERVER || 'http://127.0.0.1:18000' : process.env.KC_UI_SERVER || 'http://127.0.0.1:7380');
    expect(dev.preview.proxy).toEqual(dev.server.proxy);
});
