import { request } from 'node:http';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { afterAll, beforeAll, describe, expect, test } from 'vitest';
import { createServer, preview, Plugin, PreviewServer, ViteDevServer } from 'vite';
import config from '../../../vite.config.mts';

describe.each(['development', 'preview'] as const)('%s UI base routing', mode => {
    let ui: ViteDevServer | PreviewServer;
    let port: number;
    beforeAll(async () => {
        const configured = config({command: mode === 'development' ? 'serve' : 'build', mode});
        const options = {configFile: false as const, root: resolve(dirname(fileURLToPath(import.meta.url)), '../../..'),
            base: configured.base, build: {outDir: configured.build.outDir}, logLevel: 'silent' as const,
            plugins: configured.plugins.filter((plugin): plugin is Plugin => !!plugin && !Array.isArray(plugin) &&
                typeof plugin === 'object' && 'name' in plugin && plugin.name === 'kc-ui-canonical-base')};
        if (mode === 'development') {
            const dev = await createServer({...options, optimizeDeps: {noDiscovery: true, include: []},
                server: {host: '127.0.0.1', port: 0, proxy: {}, watch: {ignored: ['**']}}});
            await dev.listen(); ui = dev;
        } else {
            ui = await preview({...options, preview: {host: '127.0.0.1', port: 0, proxy: {}}});
        }
        const address = ui.httpServer?.address();
        if (!address || typeof address === 'string') throw new Error('Expected a temporary Vite port');
        port = address.port;
    });
    afterAll(async () => {
        await ui?.close();
    });
    async function call(path: string, method = 'GET') {
        return new Promise<{status?: number; location?: string}>((resolve, reject) => {
            request({hostname: '127.0.0.1', port, path, method, headers: {Accept: 'text/html'}}, res => {
                res.resume(); res.on('end', () => resolve({status: res.statusCode, location: res.headers.location}));
            }).on('error', reject).end();
        });
    }
    test.each([
        {method: 'GET', path: '/ui', location: '/ui/'},
        {method: 'HEAD', path: '/ui', location: '/ui/'},
        {method: 'GET', path: '/ui?ref=feature%2Fbranch&next=https%3A%2F%2Fexample.test', location: '/ui/?ref=feature%2Fbranch&next=https%3A%2F%2Fexample.test'},
    ])('$method $path redirects to the canonical base', async ({method, path, location}) => {
        expect(await call(path, method)).toEqual({status: 308, location});
    });
    test.each(['/ui/', '/ui/repositories/project.1/objects?ref=main'])('%s serves the UI without a canonical redirect', async path => {
        expect(await call(path)).toEqual({status: 200, location: undefined});
    });
    test.each([{path: '/ui', method: 'POST'}, {path: '/ui-other', method: 'GET'}])('$method $path does not use the canonical redirect', async ({path, method}) => {
        const result = await call(path, method);
        expect(result.status).not.toBe(308);
        expect(result.location).toBeUndefined();
    });
});
