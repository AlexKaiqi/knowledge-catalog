import {afterEach, expect, test, vi} from 'vitest';
import {Discovery, session} from './session';
import {userHeaders, userProof} from './sessionProof';
import {jsonRequest} from './transport';

const pairing: Discovery = {mode: 'organization-validator', localAssertion: false, accepts: ['Authorization']};
const response = (value: unknown, status = 200) => new Response(JSON.stringify(value), {status});
afterEach(() => {session.logout(); window.sessionStorage.clear(); vi.restoreAllMocks(); vi.unstubAllGlobals();});

test('a verifying pairing can authenticate and share proof regardless of its mode name', async () => {
    const fetcher = vi.fn(async (path: string, options?: RequestInit) => {
        expect(new Headers(options?.headers).get('Authorization')).toBe('Bearer user-proof');
        return response(path.endsWith('/whoami') ? {principal: 'reader'} : {datasets: []});
    });
    vi.stubGlobal('fetch', fetcher);
    await session.authenticate(pairing, 'user-proof');
    expect(session.get()).toEqual({mode: pairing.mode, principal: 'reader'});
    await jsonRequest('/snapshot/v1/datasets');
    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(window.localStorage.getItem('kc.ui.token')).toBeNull();
});

test('browserLogin capability enables same-subject token renewal for any verifying pairing', async () => {
    const configured = {...pairing, browserLogin: {oauth2Base: 'https://identity.example/oauth2', clientId: 'console'}};
    const fetcher = vi.fn(async (path: string, options?: RequestInit) => {
        if (path.endsWith('/token')) {
            expect(JSON.parse(String(options?.body))).toEqual({grantType: 'refresh_token', refreshToken: 'refresh'});
            return response({accessToken: 'renewed-proof', expiresIn: 60});
        }
        return response({principal: 'reader'});
    });
    vi.stubGlobal('fetch', fetcher);
    await session.authenticate(configured, 'user-proof', 'refresh', 60);
    const future = Date.now() + 61_000;
    vi.spyOn(Date, 'now').mockReturnValue(future);
    expect(await userHeaders()).toEqual({Authorization: 'Bearer renewed-proof'});
    expect(fetcher.mock.calls.map(([path]) => path)).toEqual(['/identity/v1/whoami', '/identity/v1/token', '/identity/v1/whoami']);
    expect(userProof.get()?.principal).toBe('reader');
});

test('a pairing without browserLogin cannot exchange an expired proof even when it contains a refresh token', async () => {
    const fetcher = vi.fn(async () => response({principal: 'reader'}));
    vi.stubGlobal('fetch', fetcher);
    await session.authenticate(pairing, 'user-proof', 'unadvertised-refresh', 60);
    const future = Date.now() + 61_000;
    vi.spyOn(Date, 'now').mockReturnValue(future);
    await expect(userHeaders()).rejects.toMatchObject({code: 'UNAUTHENTICATED'});
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(userProof.get()).toBeNull();
});

test.each([
    {...pairing, localAssertion: true},
    {...pairing, accepts: ['Cookie']},
    {...pairing, accepts: ['X-Kc-As']},
])('unsupported pairing shape refuses before accepting user proof: %j', async discovery => {
    const fetcher = vi.fn(); vi.stubGlobal('fetch', fetcher);
    await expect(session.authenticate(discovery, 'user-proof')).rejects.toMatchObject({code: 'PRECONDITION_FAILED'});
    expect(fetcher).not.toHaveBeenCalled();
    expect(userProof.get()).toBeNull();
});

test('stored proof cannot supply its own pairing or token exchange capability', async () => {
    window.sessionStorage.setItem(`kc.ui.proof.v2:${window.location.origin}`, JSON.stringify({mode: pairing.mode, accessToken: 'old-proof', principal: 'reader', refreshToken: 'old-refresh', expiresAt: Date.now() - 1,
        localAssertion: false, accepts: ['Authorization'], browserLogin: {oauth2Base: 'https://injected.example/oauth2', clientId: 'untrusted'}}));
    const fetcher = vi.fn(); vi.stubGlobal('fetch', fetcher);
    expect(userProof.get()).toBeNull();
    expect(await userHeaders()).toEqual({});
    userProof.bind(pairing);
    await expect(userHeaders()).rejects.toMatchObject({code: 'UNAUTHENTICATED'});
    expect(fetcher).not.toHaveBeenCalled();
});

test('a new service pairing never receives the previous pairing proof', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => response({principal: 'reader'})));
    await session.authenticate(pairing, 'old-proof');
    const fetcher = vi.fn(async (_path: string, options?: RequestInit) => {
        expect(new Headers(options?.headers).has('Authorization')).toBe(false);
        return response({error: {code: 'UNAUTHENTICATED'}}, 401);
    });
    vi.stubGlobal('fetch', fetcher);
    await expect(session.restore({...pairing, mode: 'another-validator'})).rejects.toMatchObject({code: 'UNAUTHENTICATED'});
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(userProof.get()).toBeNull();
});

test.each(['', ' '])('an unusable renewed token %j cannot fall back to cookie identity', async accessToken => {
    const configured = {...pairing, browserLogin: {oauth2Base: 'https://identity.example/oauth2', clientId: 'console'}};
    const fetcher = vi.fn(async (path: string) => response(path.endsWith('/token') ? {accessToken, expiresIn: 60} : {principal: 'reader'}));
    vi.stubGlobal('fetch', fetcher);
    await session.authenticate(configured, 'old-proof', 'refresh', 60);
    const future = Date.now() + 61_000;
    vi.spyOn(Date, 'now').mockReturnValue(future);
    await expect(userHeaders()).rejects.toMatchObject({code: 'UNAUTHENTICATED'});
    expect(fetcher.mock.calls.map(([path]) => path)).toEqual(['/identity/v1/whoami', '/identity/v1/token']);
    expect(userProof.get()).toBeNull();
});

test('a previous pairing renewal cannot become the renewal of the current login', async () => {
    const configured = {...pairing, browserLogin: {oauth2Base: 'https://identity.example/oauth2', clientId: 'console'}};
    let finishOld!: (response: Response) => void;
    const fetcher = vi.fn(async (path: string, options?: RequestInit) => {
        if (path.endsWith('/token')) {
            const body = JSON.parse(String(options?.body));
            return body.refreshToken === 'old-refresh' ? new Promise<Response>(resolve => {finishOld = resolve;}) : response({accessToken: 'new-renewed', expiresIn: 60});
        }
        return response({principal: new Headers(options?.headers).get('Authorization')?.includes('new-') ? 'new-reader' : 'reader'});
    });
    vi.stubGlobal('fetch', fetcher);
    await session.authenticate(configured, 'old-proof', 'old-refresh', 60);
    const later = Date.now() + 61_000;
    const now = vi.spyOn(Date, 'now').mockReturnValue(later);
    const previous = expect(userHeaders()).rejects.toMatchObject({code: 'UNAUTHENTICATED'});
    await session.authenticate({...configured, mode: 'next-validator'}, 'new-proof', 'new-refresh', 60);
    now.mockReturnValue(later + 61_000);
    const current = expect(userHeaders()).resolves.toEqual({Authorization: 'Bearer new-renewed'});
    finishOld(response({error: {code: 'UNAUTHENTICATED'}}, 401));
    await Promise.all([previous, current]);
    expect(userProof.get()?.principal).toBe('new-reader');
});


test.each(['unsupported', 'different-mode'])('a late renewal sends no proof after current pairing becomes %s', async change => {
    const prior = {...pairing, browserLogin: {oauth2Base: 'https://identity.example/oauth2', clientId: 'console'}};
    userProof.bind(prior);
    userProof.set({mode: prior.mode, accessToken: 'old-expired', refreshToken: 'old-refresh', principal: 'old-reader', expiresAt: Date.now() - 1});
    let finish!: (value: Response) => void;
    const sent: Array<{path: string; authorization: string | null}> = [];
    vi.stubGlobal('fetch', vi.fn((path: string, options?: RequestInit) => {
        sent.push({path, authorization: new Headers(options?.headers).get('Authorization')});
        return path.endsWith('/token') ? new Promise<Response>(resolve => {finish = resolve;}) : Promise.resolve(response({principal: 'old-reader'}));
    }));
    const pending = expect(userHeaders()).rejects.toMatchObject({code: 'UNAUTHENTICATED'});
    if (change === 'unsupported') userProof.bind({mode: 'local-validator', localAssertion: true, accepts: ['X-Kc-As']});
    else {
        userProof.bind({...prior, mode: 'validator-b'});
        userProof.set({mode: 'validator-b', accessToken: 'new-user-proof', principal: 'new-reader'});
    }
    finish(response({accessToken: 'old-renewed-proof', expiresIn: 60}));
    await pending;
    if (change === 'different-mode') {
        expect(await userHeaders()).toEqual({Authorization: 'Bearer new-user-proof'});
        expect(userProof.get()?.principal).toBe('new-reader');
    } else {
        expect(await userHeaders()).toEqual({});
        expect(userProof.get()).toBeNull();
    }
    expect(sent).toEqual([{path: '/identity/v1/token', authorization: null}]);
});
