import {afterEach, beforeEach, expect, test, vi} from 'vitest';
import {userHeaders, userProof} from './sessionProof';
import {objects} from './index';
import {jsonRequest} from './transport';

beforeEach(() => {userProof.clear(); userProof.bind({mode: 'taihu', localAssertion: false, accepts: ['Authorization'], browserLogin: {oauth2Base: 'https://identity.example/oauth2', clientId: 'console'}});});
afterEach(() => {userProof.clear(); vi.unstubAllGlobals();});

test('native file bytes and Dataset requests use the same provider proof without storage credentials', async () => {
    userProof.set({mode: 'taihu', accessToken: 'real-user-proof', principal: 'reader'});
    const fetcher = vi.fn(async (url: string) => url.startsWith('/api/v1') ? new Response(new Uint8Array([0, 1, 255])) : new Response('{}'));
    vi.stubGlobal('fetch', fetcher);
    expect(new Uint8Array(await (await objects.blob('alpha', 'main', 'image.png')).arrayBuffer())).toEqual(new Uint8Array([0, 1, 255]));
    await jsonRequest('/snapshot/v1/datasets');
    for (const [, options] of fetcher.mock.calls as unknown as [string, RequestInit][]) {
        expect(new Headers(options.headers).get('Authorization')).toBe('Bearer real-user-proof');
        expect(options.redirect).toBe('error');
        expect(new Headers(options.headers).has('X-Kc-As')).toBe(false);
    }
    expect(window.localStorage.getItem('kc.ui.token')).toBeNull();
});

test('concurrent requests renew once and reject a refreshed different subject', async () => {
    userProof.set({mode: 'taihu', accessToken: 'expired', principal: 'reader', refreshToken: 'refresh', expiresAt: Date.now() - 1});
    const fetcher = vi.fn(async (url: string) => new Response(JSON.stringify(url.endsWith('/token') ? {accessToken: 'renewed', expiresIn: 60} : {principal: 'operator'})));
    vi.stubGlobal('fetch', fetcher);
    const results = await Promise.allSettled([userHeaders(), userHeaders()]);
    expect(results.every(result => result.status === 'rejected')).toBe(true);
    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(userProof.get()).toBeNull();
});

test('refresh arriving after logout cannot restore proof', async () => {
    userProof.set({mode: 'taihu', accessToken: 'expired', principal: 'reader', refreshToken: 'refresh', expiresAt: Date.now() - 1});
    let complete!: (response: Response) => void;
    const fetcher = vi.fn((url: string) => url.endsWith('/token') ? new Promise<Response>(resolve => {complete = resolve;}) : Promise.resolve(new Response('{"principal":"reader"}')));
    vi.stubGlobal('fetch', fetcher);
    const result = userHeaders();
    userProof.clear();
    complete(new Response('{"accessToken":"late","expiresIn":60}'));
    await expect(result).rejects.toThrow();
    expect(userProof.get()).toBeNull();
});

test('retired gitea login proof is rejected and never restored', () => {
    expect(() => userProof.set({mode: 'gitea', accessToken: 'retired-proof'})).toThrow();
    window.sessionStorage.setItem(`kc.ui.proof.v2:${window.location.origin}`, JSON.stringify({mode: 'gitea', accessToken: 'retired-proof'}));
    expect(userProof.get()).toBeNull();
});
