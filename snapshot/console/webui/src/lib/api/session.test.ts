import { afterEach, expect, test, vi } from 'vitest';
import { Discovery, session } from './session';
import { kc } from './kc';
import { auth, AuthenticationError } from './index';

const discovery: Discovery = { mode: 'lakefs', localAssertion: false, accepts: ['Cookie'] };
const response = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status });
const key = () => `kc.ui.session.v1:${window.location.origin}`;
afterEach(() => { session.logout(); window.localStorage.clear(); window.sessionStorage.clear(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

test('entering Dataset verifies the original cookie without storing or forwarding credentials', async () => {
    const fetcher = vi.fn(async (path: string, options?: RequestInit) => response({principal: path === '/identity/v1/whoami' && options ? 'alice' : ''}));
    vi.stubGlobal('fetch', fetcher);
    window.localStorage.setItem('user', JSON.stringify({id: 'previous-user'}));
    await session.restore(discovery);
    expect(session.get()).toEqual({mode: 'lakefs', principal: 'alice'});
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(fetcher.mock.calls[0][0]).toBe('/identity/v1/whoami');
    expect(fetcher.mock.calls[0][1]).toMatchObject({credentials: 'same-origin', redirect: 'error', cache: 'no-store'});
    const headers = new Headers(fetcher.mock.calls[0][1]?.headers);
    expect(headers.get('X-Kc-Console')).toBe('1');
    expect(headers.has('Authorization')).toBe(false);
    expect(headers.has('X-Kc-As')).toBe(false);
    expect(headers.has('Cookie')).toBe(false);
    expect(window.sessionStorage.length).toBe(0);
    expect(window.localStorage.length).toBe(0);
});

test('legacy local or bearer state cannot become the console identity', async () => {
    window.sessionStorage.setItem(key(), JSON.stringify({mode: 'local', principal: 'forged', accessToken: 'old', refreshToken: 'renewal'}));
    window.localStorage.setItem('kc.ui.token', 'old'); window.localStorage.setItem('kc.ui.as', 'forged');
    vi.stubGlobal('fetch', vi.fn(async () => response({principal: 'alice'})));
    await session.restore(discovery);
    expect(session.get()?.principal).toBe('alice');
    expect(window.sessionStorage.length).toBe(0);
    expect(window.localStorage.length).toBe(0);
});

test('logout invalidates an in-flight cookie proof', async () => {
    let complete!: (value: Response) => void;
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(resolve => {complete = resolve;})));
    const restore = session.restore(discovery);
    const rejected = expect(restore).rejects.toMatchObject({code: 'UNAUTHENTICATED'});
    session.logout(); complete(response({principal: 'alice'}));
    await rejected;
    expect(session.get()).toBe(null);
});

test('leaving Dataset invalidates its in-flight identity proof', async () => {
    let complete!: (value: Response) => void;
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(resolve => {complete = resolve;})));
    const controller = new AbortController();
    const restore = session.restore(discovery, controller.signal);
    const rejected = expect(restore).rejects.toMatchObject({code: 'UNAUTHENTICATED'});
    controller.abort(); complete(response({principal: 'alice'}));
    await rejected;
    expect(session.get()).toBe(null);
});

test('a late previous identity proof cannot overwrite a newly verified native login', async () => {
    let complete!: (value: Response) => void;
    const fetcher = vi.fn().mockImplementationOnce(() => new Promise<Response>(resolve => {complete = resolve;})).mockResolvedValue(response({principal: 'bob'}));
    vi.stubGlobal('fetch', fetcher);
    const first = session.restore(discovery);
    const rejected = expect(first).rejects.toMatchObject({code: 'UNAUTHENTICATED'});
    await session.restore(discovery); complete(response({principal: 'alice'}));
    await rejected;
    expect(session.get()?.principal).toBe('bob');
});

test('a late successful business response cannot survive native logout', async () => {
    let complete!: (value: Response) => void;
    vi.stubGlobal('fetch', vi.fn(async (path: string) => path.endsWith('/whoami') ? response({principal: 'alice'}) : new Promise<Response>(resolve => {complete = resolve;})));
    await session.restore(discovery);
    const request = kc.datasets();
    const rejected = expect(request).rejects.toMatchObject({code: 'UNAUTHENTICATED'});
    await vi.waitFor(() => expect(complete).toBeDefined());
    session.logout(); complete(response({catalogs: [{id: 'private-alice'}]}));
    await rejected;
    expect(session.get()).toBe(null);
});

test('a late 401 from the previous login cannot clear the new native identity', async () => {
    let principal = 'alice'; let complete!: (value: Response) => void;
    vi.stubGlobal('fetch', vi.fn(async (path: string) => path.endsWith('/whoami') ? response({principal}) : new Promise<Response>(resolve => {complete = resolve;})));
    await session.restore(discovery);
    const request = kc.datasets();
    const rejected = expect(request).rejects.toMatchObject({code: 'UNAUTHENTICATED'});
    await vi.waitFor(() => expect(complete).toBeDefined());
    principal = 'bob'; await session.restore(discovery);
    complete(response({error: {code: 'UNAUTHENTICATED', message: 'Expired previous login'}}, 401));
    await rejected;
    expect(session.get()?.principal).toBe('bob');
});

test.each([403, 503])('business HTTP %i retains the verified identity', async status => {
    vi.stubGlobal('fetch', vi.fn(async (path: string) => path.endsWith('/whoami') ? response({principal: 'alice'}) : response({error: {code: status === 403 ? 'FORBIDDEN' : 'TEMPORARY_UNAVAILABLE', message: 'Unavailable'}}, status)));
    await session.restore(discovery);
    await expect(kc.datasets()).rejects.toMatchObject({status});
    expect(session.get()?.principal).toBe('alice');
});

test('an active authentication refusal clears the console identity', async () => {
    const fetcher = vi.fn(async () => response({principal: 'alice'})); vi.stubGlobal('fetch', fetcher);
    await session.restore(discovery);
    fetcher.mockResolvedValue(response({error: {code: 'UNAUTHENTICATED', message: 'Expired login'}}, 401));
    await expect(kc.datasets()).rejects.toMatchObject({code: 'UNAUTHENTICATED'});
    expect(session.get()).toBe(null);
});

test.each([
    {mode: 'local', localAssertion: true, accepts: ['X-Kc-As']},
    {mode: 'taihu', localAssertion: false, accepts: ['Authorization']},
    {mode: 'gitea', localAssertion: false, accepts: ['Authorization']},
    {mode: 'lakefs', localAssertion: false, accepts: ['Authorization']},
])('a $mode deployment without the native cookie bridge reports configuration failure', async mismatch => {
    const fetcher = vi.fn(); vi.stubGlobal('fetch', fetcher);
    await expect(session.restore(mismatch)).rejects.toMatchObject({code: 'PRECONDITION_FAILED'});
    expect(fetcher).not.toHaveBeenCalled();
    expect(session.get()).toBe(null);
});

test.each([{}, {principal: 123}, null])('an invalid identity response %j cannot unlock Dataset', async invalid => {
    vi.stubGlobal('fetch', vi.fn(async () => response(invalid)));
    await expect(session.restore(discovery)).rejects.toMatchObject({code: 'TRANSPORT'});
    expect(session.get()).toBe(null);
});

test('a late native profile response returns the current cached identity instead of restoring the previous login', async () => {
    auth.clearCurrentUser();
    let complete!: (value: Response) => void;
    const fetcher = vi.fn().mockImplementationOnce(() => new Promise<Response>(resolve => {complete = resolve;})).mockResolvedValue(response({user: {id: 'bob'}}));
    vi.stubGlobal('fetch', fetcher);
    const previous = auth.getCurrentUserWithCache();
    auth.clearCurrentUser();
    expect(await auth.getCurrentUserWithCache()).toEqual({id: 'bob'});
    complete(response({user: {id: 'alice'}}));
    expect(await previous).toEqual({id: 'bob'});
    expect(await auth.getCurrentUserWithCache()).toEqual({id: 'bob'});
    expect(fetcher).toHaveBeenCalledTimes(2);
});

test('a native authentication refusal invalidates every in-flight profile cache write', async () => {
    auth.clearCurrentUser();
    let complete!: (value: Response) => void;
    const fetcher = vi.fn()
        .mockImplementationOnce(() => new Promise<Response>(resolve => {complete = resolve;}))
        .mockResolvedValueOnce(new Response(JSON.stringify({message: 'error authenticating request'}), {status: 401, headers: {'Content-Type': 'application/json'}}))
        .mockResolvedValue(response({user: {id: 'bob'}}));
    vi.stubGlobal('fetch', fetcher);
    const previous = auth.getCurrentUserWithCache();
    await expect(auth.listUsers()).rejects.toBeInstanceOf(AuthenticationError);
    complete(response({user: {id: 'alice'}}));
    expect(await previous).toEqual({id: 'bob'});
    expect(await auth.getCurrentUserWithCache()).toEqual({id: 'bob'});
    expect(fetcher).toHaveBeenCalledTimes(3);
});

test('native login cannot cache a profile that arrives after the shared identity changes', async () => {
    auth.clearCurrentUser();
    let complete!: (value: Response) => void;
    const fetcher = vi.fn()
        .mockResolvedValueOnce(response({}))
        .mockImplementationOnce(() => new Promise<Response>(resolve => {complete = resolve;}))
        .mockResolvedValue(response({user: {id: 'bob'}}));
    vi.stubGlobal('fetch', fetcher);
    const login = auth.login('native-key', 'native-secret');
    await vi.waitFor(() => expect(complete).toBeDefined());
    auth.clearCurrentUser();
    expect(await auth.getCurrentUserWithCache()).toEqual({id: 'bob'});
    complete(response({user: {id: 'alice'}}));
    expect(await login).toEqual({id: 'bob'});
    expect(await auth.getCurrentUserWithCache()).toEqual({id: 'bob'});
    expect(fetcher).toHaveBeenCalledTimes(3);
});

test('a native profile arriving after logout cannot restore the cleared identity', async () => {
    auth.clearCurrentUser();
    let complete!: (value: Response) => void;
    const fetcher = vi.fn()
        .mockImplementationOnce(() => new Promise<Response>(resolve => {complete = resolve;}))
        .mockResolvedValue(new Response(JSON.stringify({message: 'error authenticating request'}), {status: 401, headers: {'Content-Type': 'application/json'}}));
    vi.stubGlobal('fetch', fetcher);
    const previous = auth.getCurrentUserWithCache();
    const rejected = expect(previous).rejects.toBeInstanceOf(AuthenticationError);
    auth.clearCurrentUser();
    complete(response({user: {id: 'alice'}}));
    await rejected;
    expect(localStorage.getItem('user')).toBe(null);
    expect(fetcher).toHaveBeenCalledTimes(2);
});
