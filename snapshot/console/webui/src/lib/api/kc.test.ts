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
    await session.restore({mode: 'taihu', localAssertion: false, accepts: ['Authorization']});
    const result = await kc.file('shared', {setId: 'shared', revision: 1, repositories: {'source': 'c1'}}, '说明.txt');
    expect(new TextDecoder().decode(decodeContent(result.content))).toBe('茶铺\n');
});

test('historical detail requests carry bounded pagination without altering the revision', async () => {
    const fetcher = vi.fn(async (path: string) => new Response(JSON.stringify(path === '/identity/v1/whoami' ? {principal: 'alice'} : {}), {status: 200}));
    vi.stubGlobal('fetch', fetcher);
    await session.restore({mode: 'taihu', localAssertion: false, accepts: ['Authorization']});
    await kc.dataset('shared', 1, undefined, {limit: 50, continuation: 'basis+/='});
    expect(fetcher.mock.calls[1][0]).toBe('/snapshot/v1/datasets/shared?revision=1&limit=50&continuation=basis%2B%2F%3D');
});

test.each([undefined, 1])('named Dataset consumption resolves revision %s without management fields', async revision => {
    const pin = {setId: 'shared/name', revision: revision ?? 3, repositories: {source: 'fixed'}, pinId: 'accepted'};
    const fetcher = vi.fn(async (path: string) => new Response(JSON.stringify(path === '/identity/v1/whoami' ? {principal: 'alice'} : pin), {status: 200}));
    vi.stubGlobal('fetch', fetcher);
    await session.restore({mode: 'taihu', localAssertion: false, accepts: ['Authorization']});
    const controller = new AbortController();
    expect(await kc.resolve('shared/name', revision, controller.signal)).toEqual(pin);
    expect(fetcher.mock.calls[1]).toEqual(['/snapshot/v1/datasets/shared%2Fname/resolve', expect.objectContaining({
        method: 'POST', body: JSON.stringify(revision === undefined ? {} : {revision}), signal: controller.signal,
    })]);
});

test('build output belongs to the Snapshot Store console module', () => {
    expect(config({ command: 'build', mode: 'production' }).build.outDir).toBe('../webapp');
});

test('a grant update cannot acknowledge another rule as a successful replacement', async () => {
    vi.stubGlobal('fetch', vi.fn(async (path: string) => new Response(JSON.stringify(path === '/identity/v1/whoami' ? {principal: 'alice'} : {
        id: 'another-rule', principal: 'alice', actions: ['file.read'], }), {status: 200})));
    await session.restore({mode: 'taihu', localAssertion: false, accepts: ['Authorization']});
    await expect(kc.updateGrant({id: 'original-rule', principal: 'alice', actions: ['file.read'], }, {principal: 'alice', actions: ['file.read'], })).rejects.toMatchObject({status: 502});
});

test('development uses the same UI base and closed KC API paths', () => {
    const dev = config({ command: 'serve', mode: 'development' });
    expect(dev.base).toBe('/ui/');
    if (!('server' in dev)) throw new Error('Development must configure the KC proxy');
    expect(Object.keys(dev.server.proxy)).toEqual(['/identity/v1', '/snapshot/v1', '/object/v1', '/admin/v1', '/api/v1', '^/(?!ui(?:/|$)|identity/v1|snapshot/v1|object/v1|admin/v1|api/v1|logout|oidc)']);
    for (const proxy of Object.values(dev.server.proxy)) expect(proxy.target).toBe(process.env.KC_UI_SERVER || 'http://127.0.0.1:7380');
    expect(dev.preview.proxy).toEqual(dev.server.proxy);
});
