import {createServer as createHTTPServer, request, Server} from 'node:http';
import {dirname, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {createServer, preview, PreviewServer, ViteDevServer} from 'vite';
import config from '../../../vite.config.mts';

describe.each(['development', 'preview'] as const)('%s KC administration proxy', mode => {
    let kc: Server, native: Server, ui: ViteDevServer | PreviewServer;
    let uiPort: number;
    const calls: {method?: string; path?: string; cookie?: string; marker?: string; body: string}[] = [];
    async function listen(server: Server): Promise<number> {
        await new Promise<void>(resolve => {server.listen(0, '127.0.0.1', resolve);});
        const address = server.address();
        if (!address || typeof address === 'string') throw new Error('Expected a temporary HTTP port');
        return address.port;
    }
    beforeEach(async () => {
        calls.length = 0;
        kc = createHTTPServer((req, res) => {
            let body = '';
            req.on('data', chunk => {body += chunk;});
            req.on('end', () => {
                calls.push({method: req.method, path: req.url, cookie: req.headers.cookie,
                    marker: req.headers['x-kc-console'] as string | undefined, body});
                res.writeHead(200, {'Content-Type': 'application/json'}); res.end('{"rules":[]}');
            });
        });
        native = createHTTPServer((_req, res) => {res.writeHead(418); res.end('native backend');});
        vi.stubEnv('KC_UI_SERVER', `http://127.0.0.1:${await listen(kc)}`);
        const nativeURL = `http://127.0.0.1:${await listen(native)}`;
        vi.stubEnv('LAKEFS_UI_SERVER', nativeURL); vi.stubEnv('LAKEFS_S3_SERVER', nativeURL);
        const configured = config({command: mode === 'development' ? 'serve' : 'build', mode});
        const proxy = configured.preview.proxy;
        const common = {configFile: false as const, root: resolve(dirname(fileURLToPath(import.meta.url)), '../../..'),
            base: configured.base, build: {outDir: configured.build.outDir}, plugins: [], logLevel: 'silent' as const};
        if (mode === 'development') {
            const dev = await createServer({...common, optimizeDeps: {noDiscovery: true, include: []},
                server: {host: '127.0.0.1', port: 0, proxy, watch: {ignored: ['**']}}});
            await dev.listen(); ui = dev;
        } else ui = await preview({...common, preview: {host: '127.0.0.1', port: 0, proxy}});
        const address = ui.httpServer?.address();
        if (!address || typeof address === 'string') throw new Error('Expected a temporary Vite port');
        uiPort = address.port;
    });
    afterEach(async () => {
        await ui?.close();
        await Promise.all([kc, native].map(server => new Promise<void>(resolve => {server?.close(() => resolve());})));
        vi.unstubAllEnvs();
    });
    function call(path: string, method = 'GET', body = ''): Promise<number | undefined> {
        return new Promise((resolve, reject) => {
            const req = request({hostname: '127.0.0.1', port: uiPort, path, method,
                headers: {Cookie: 'internal_auth_session=test-proof', 'X-Kc-Console': '1', 'Content-Type': 'application/json'}}, res => {
                res.resume(); res.on('end', () => resolve(res.statusCode));
            }).on('error', reject);
            req.end(body);
        });
    }
    test.each([
        {method: 'GET', path: '/admin/v1/grants?principal=bob&catalog=kr%3A%2F%2Facme%2Fcatalog', body: ''},
        {method: 'POST', path: '/admin/v1/grants', body: '{"principal":"bob","catalog":"kr://acme/catalog","actions":["file.read"],"dataset":"docs"}'},
        {method: 'DELETE', path: '/admin/v1/grants/alw_one', body: ''},
    ])('$method grants reaches KC with the original user proof and exact scope', async ({method, path, body}) => {
        expect(await call(path, method, body)).toBe(200);
        expect(calls).toEqual([{method, path, body, cookie: 'internal_auth_session=test-proof', marker: '1'}]);
    });
    test('unrelated repository/S3 requests still reach lakeFS', async () => {
        expect(await call('/api/v1/repositories')).toBe(418);
        expect(await call('/native-repo/main/example.csv')).toBe(418);
        expect(calls).toEqual([]);
    });
});
