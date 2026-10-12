import { vi } from 'vitest';

export const cookieDiscovery = {mode: 'lakefs', localAssertion: false, accepts: ['Cookie']};
export function mockKC() {
    let revision = 1, retired = false;
    let sources = [{ repository: 'kr://example/source', selector: 'refs/heads/main', commit: 'c1', path: 'docs', subPath: 'handbook' }];
    let rejected = '';
    let identity: string | undefined = 'alice';
    const fetcher = vi.fn(async (input: string, options?: RequestInit) => {
        const path = new URL(input, window.location.origin);
        const body = options?.body ? JSON.parse(String(options.body)) : undefined;
        const headers = options?.headers as Record<string, string> | undefined;
        const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), {status, headers: {'Content-Type': 'application/json'}});
        const fault = (code: string, message: string, status: number) => json({ error: { code, message } }, status);
        if (path.pathname === '/snapshot/v1/datasets:resolve') return json({setId: body.dataset, revision: body.revision, repositories: {'kr://example/source': 'c1'}, items: []});
        if (path.pathname === '/api/v1/setup_lakefs') return json({state: 'initialized', login_config: {login_cookie_names: [], logout_url: '/logout'}});
        if (path.pathname === '/api/v1/user') return identity ? json({user: {id: identity, friendly_name: identity}}) : json({message: 'error authenticating request'}, 401);
        if (path.pathname === '/api/v1/auth/login') { identity = 'alice'; return json({}); }
        if (path.pathname === '/api/v1/config/version') return json({version: 'v1.58.0', version_context: 'lakeFS', upgrade_recommended: false});
        if (path.pathname === '/api/v1/config') return json({storage_config: {blockstore_type: 'local', import_support: true}, version_config: {version: 'v1.58.0', version_context: 'lakeFS', upgrade_recommended: false}});
        if (path.pathname.startsWith('/api/v1/repositories')) return json({results: [], pagination: {has_more: false}});
        if (path.pathname === '/identity/v1/auth') return json(cookieDiscovery);
        if (path.pathname === '/identity/v1/whoami') {
            return identity && headers?.['X-Kc-Console'] === '1' && options?.credentials === 'same-origin'
                ? json({principal: identity}) : fault('UNAUTHENTICATED', '需要原控制台登录', 401);
        }
        if (rejected && path.pathname.includes(rejected)) return fault('FORBIDDEN', '当前账号无权发布', 403);
        if (path.pathname.endsWith('/repositories')) return json({ repositories: [{ id: 'kr://example/source' }] });
        const dataset = (version = revision) => ({
            id: 'shared', revision: version, servingRevision: revision, versions: revision === 1 ? [1] : [1, revision],
            repositories: ['kr://example/source'], itemCount: 1, retired,
            items: [{ target: 'docs', repository: 'kr://example/source', commit: 'c1', kind: 'prefix', prefix: 'handbook' }], sources,
        });
        if (path.pathname.endsWith('/datasets') && options?.method === 'POST') { revision = body.revision; sources = body.sources; return json({ dataset: body.dataset }); }
        if (path.pathname.endsWith('/datasets')) return json({ datasets: [dataset()] });
        if (path.pathname.endsWith('/retire')) { retired = true; return json({ retired: true }); }
        if (path.pathname.endsWith('/datasets/shared')) return json(dataset(Number(path.searchParams.get('revision')) || undefined));
        const pin = { setId: 'shared', revision: body?.pin?.revision ?? revision, ref: `v${body?.pin?.revision ?? revision}`, repositories: { 'kr://example/source': 'c1' }, pinId: 'accepted' };
        if (path.pathname === '/snapshot/v1/files/mounts:list') return json({ pin, mounts: [] });
        if (path.pathname === '/snapshot/v1/files/tree:list') return json({ pin,
            entries: [{ name: body.continuation ? 'second.txt' : 'first.txt', kind: 'file' }],
            continuation: body.continuation ? undefined : 'next-page', exhausted: !!body.continuation,
        });
        if (path.pathname === '/snapshot/v1/files/file:read') return json({ pin, content: btoa('published bytes'), totalBytes: 15, offset: 0, eof: true });
        throw new Error(`Unexpected KC route: ${path.pathname}`);
    });
    return {fetcher, reject: (path: string) => {rejected = path;}, setIdentity: (principal?: string) => {identity = principal;}};
}
