import { afterEach, expect, test, vi } from 'vitest';
import { setup } from './index';
import {userProof} from './sessionProof';

afterEach(() => { userProof.clear(); vi.unstubAllGlobals(); });

test('original setup forms preserve the fixed console marker with explicit headers', async () => {
    const fetcher = vi.fn(async () => new Response('{}', {status: 200, headers: {'Content-Type': 'application/json'}}));
    vi.stubGlobal('fetch', fetcher);
    await setup.lakeFS('fixture-user');
    await setup.commPrefs('fixture@example.test', false, false);
    expect(fetcher).toHaveBeenCalledTimes(2);
    for (const [, options] of fetcher.mock.calls as unknown as [string, RequestInit][]) {
        const headers = new Headers(options.headers);
        expect(headers.get('X-Kc-Console')).toBe('1');
        expect(headers.get('Content-Type')).toBe('application/json');
        expect(options.credentials).toBe('same-origin');
        expect(headers.has('Authorization')).toBe(false);
        expect(headers.has('Cookie')).toBe(false);
        expect(headers.has('X-Kc-As')).toBe(false);
    }
});

test('public login configuration survives an expired Dataset identity without keeping caller proof', async () => {
    userProof.set({mode: 'gitea', accessToken: 'previous-user-proof'});
    let complete!: (response: Response) => void;
    const fetcher = vi.fn(() => new Promise<Response>(resolve => {complete = resolve;}));
    vi.stubGlobal('fetch', fetcher);
    const startup = setup.getState();
    userProof.clear();
    complete(new Response('{"state":"initialized","login_config":{"snapshot_identity":true}}', {headers: {'Content-Type': 'application/json'}}));
    await expect(startup).resolves.toMatchObject({state: 'initialized', login_config: {snapshot_identity: true}});
    const headers = new Headers((fetcher.mock.calls as unknown as [string, RequestInit][])[0][1].headers);
    expect(headers.has('Authorization')).toBe(false);
    expect(headers.get('X-Kc-Console')).toBe('1');
});
