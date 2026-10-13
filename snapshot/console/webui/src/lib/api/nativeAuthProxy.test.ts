import { createServer as createHTTPServer, request, Server } from 'node:http';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { createServer, ViteDevServer } from 'vite';
import config from '../../../vite.config.mts';

let native: Server, s3: Server, ui: ViteDevServer;
let uiPort: number;
const calls: {path?: string; host?: string}[] = [];
const redirects = [
    {path: '/logout', location: '/auth/login', expected: '/ui/auth/login'},
    {path: '/oidc/login', location: 'https://login.example.test/authorize?client_id=lakefs', expected: 'https://login.example.test/authorize?client_id=lakefs'},
    {path: '/oidc/callback', location: '/auth/login?next=%2Frepositories#login', expected: '/ui/auth/login?next=%2Frepositories#login'},
    {path: '/oidc/signed-out', location: '//login.example.test/signed-out', expected: '//login.example.test/signed-out'},
    {path: '/oidc/other-page', location: '/repositories', expected: '/repositories'},
    {path: '/oidc/login-prefix', location: '/auth/login-other', expected: '/auth/login-other'},
];
async function listen(server: Server): Promise<number> {
    await new Promise<void>(resolve => {server.listen(0, '127.0.0.1', resolve);});
    const address = server.address();
    if (!address || typeof address === 'string') throw new Error('Expected a temporary HTTP port');
    return address.port;
}
beforeEach(async () => {
    calls.length = 0;
    native = createHTTPServer((req, res) => {
        calls.push({path: req.url, host: req.headers.host});
        res.writeHead(302, {Location: redirects.find(({path}) => path === req.url)?.location ?? '/'}); res.end();
    });
    s3 = createHTTPServer((_req, res) => {res.writeHead(418); res.end('Wrong S3 backend');});
    vi.stubEnv('KC_UI_AUTH_SERVER', `http://127.0.0.1:${await listen(native)}`);
    vi.stubEnv('LAKEFS_S3_SERVER', `http://127.0.0.1:${await listen(s3)}`);
    const dev = config({command: 'serve', mode: 'development'});
    if (!('server' in dev)) throw new Error('Expected development proxy configuration');
    ui = await createServer({configFile: false, root: resolve(dirname(fileURLToPath(import.meta.url)), '../../..'),
        base: dev.base, plugins: [], logLevel: 'silent', optimizeDeps: {noDiscovery: true, include: []},
        server: {host: '127.0.0.1', port: 0, proxy: dev.server.proxy, watch: {ignored: ['**']}}});
    await ui.listen();
    const address = ui.httpServer?.address();
    if (!address || typeof address === 'string') throw new Error('Expected a temporary Vite port');
    uiPort = address.port;
});
afterEach(async () => {
    await ui?.close();
    await Promise.all([native, s3].map(server => new Promise<void>(resolve => {server?.close(() => resolve());})));
    vi.unstubAllEnvs();
});

test.each(redirects)('native auth proxy preserves the backend redirect for $path', async ({path, expected}) => {
    const result = await new Promise<{status?: number; location?: string}>((resolve, reject) => {
        request({hostname: '127.0.0.1', port: uiPort, path, headers: {Host: 'localhost:5178'}}, res => {
            res.resume(); res.on('end', () => resolve({status: res.statusCode, location: res.headers.location}));
        }).on('error', reject).end();
    });
    expect(result.status).toBe(302);
    expect(result.location).toBe(expected);
    expect(calls).toEqual([{path, host: 'localhost:5178'}]);
});
