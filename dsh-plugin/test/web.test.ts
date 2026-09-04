import { chmod, mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { createServer } from 'node:http';
import { Readable } from 'node:stream';
import os from 'node:os';
import path from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import { apply, createLoomWorkspaceHandler } from '../src/web.js';

class CapturedResponse {
  status = 0;
  body = '';

  writeHead(status: number): void { this.status = status; }
  end(body: string): void { this.body = body; }
}

function request(method: string, url: string, body?: object): Readable & { method: string; url: string } {
  const input = Readable.from(body ? [JSON.stringify(body)] : []);
  return Object.assign(input, { method, url });
}

async function call(handler: ReturnType<typeof createLoomWorkspaceHandler>, method: string, url: string, body?: object) {
  const response = new CapturedResponse();
  await handler(request(method, url, body) as never, response as never);
  return {
    status: response.status,
    body: JSON.parse(response.body) as {
      path?: string;
      content?: string;
      error?: { code: string; message: string };
      vfs?: { entries?: unknown[]; continuation?: string };
      workspace?: string;
      state?: string;
      inventory?: { catalogs: Array<{ repositories: Array<{ id: string; schemas: Array<{ entity: string }> }>; knowledgeSets: Array<{ id: string }> }> };
      connection?: { workspace: string; pinId?: string };
    },
  };
}

describe('first-use knowledge browser', () => {
  const roots: string[] = [];
  afterEach(async () => Promise.all(roots.splice(0).map((root) => rm(root, { recursive: true, force: true }))));

  it('registers exactly one GET/POST host bridge path', () => {
    let route: { kind: string; path: string; handler: unknown } | undefined;
    let label = '';
    const dispose = () => undefined;
    const context = {
      webServer: { register(value: typeof route) { route = value; return dispose; } },
      effect(factory: () => unknown, description: string) { label = description; expect(factory()).toBe(dispose); },
    };
    apply(context as never, { home: '/tmp/kc-web-contract' });
    expect(route).toMatchObject({ kind: 'exact', path: '/api/loom/vfs' });
    expect(typeof route?.handler).toBe('function');
    expect(label).toMatch(/mounted-files host bridge/);
  });

  it('keeps mounted files hidden by default and remembers an explicit visibility choice', async () => {
    const home = await mkdtemp(path.join(os.tmpdir(), 'loom-web-home-'));
    const project = await mkdtemp(path.join(os.tmpdir(), 'loom-web-project-'));
    roots.push(home, project);
    const task = path.join(home, 'tasks', 'first');
    const mountpoint = path.join(project, 'knowledge');
    await mkdir(task, { recursive: true });
    await mkdir(mountpoint);
    await writeFile(path.join(mountpoint, 'welcome.md'), '# Welcome\n');
    await writeFile(path.join(mountpoint, 'after.md'), '# After\n');
    await writeFile(path.join(task, 'context.json'), JSON.stringify({
      version: 1,
      workspace: 'agent',
      pinId: 'pin-1',
      root: project,
      readOnly: true,
      mounts: [{ path: 'knowledge', mountpoint, repository: 'kr://acme/docs', commit: 'c1' }],
    }));

    const handler = createLoomWorkspaceHandler({ home });
    const first = await call(handler, 'GET', `/api/loom/vfs?cwd=${encodeURIComponent(project)}`);
    expect(first).toMatchObject({ status: 200, body: { workspace: 'agent', vfs: { enabled: false, state: 'disabled', entries: [] } } });

    const enabled = await call(handler, 'POST', '/api/loom/vfs', { action: 'set-vfs-enabled', cwd: project, enabled: true });
    expect(enabled).toMatchObject({ status: 200, body: { preferences: { vfsEnabled: true } } });

    const loaded = await call(handler, 'GET', `/api/loom/vfs?cwd=${encodeURIComponent(project)}&load=1`);
    expect(loaded).toMatchObject({ status: 200, body: { vfs: { enabled: true, state: 'ready' } } });
    expect(loaded.body.vfs.entries).toEqual([{ path: 'knowledge', repository: 'kr://acme/docs', commit: 'c1', kind: 'directory' }]);

    const firstPage = await call(handler, 'GET', `/api/loom/vfs?cwd=${encodeURIComponent(project)}&load=1&directory=knowledge&limit=1`);
    expect(firstPage).toMatchObject({ status: 200, body: { vfs: { entries: [{ path: 'knowledge/after.md', kind: 'file' }] } } });
    expect(firstPage.body.vfs?.continuation).toBeTypeOf('string');
    const secondPage = await call(handler, 'GET', `/api/loom/vfs?cwd=${encodeURIComponent(project)}&load=1&directory=knowledge&limit=1&continuation=${encodeURIComponent(firstPage.body.vfs?.continuation ?? '')}`);
    expect(secondPage).toMatchObject({ status: 200, body: { vfs: { entries: [{ path: 'knowledge/welcome.md', kind: 'file' }] } } });

    const preview = await call(handler, 'GET', `/api/loom/vfs?cwd=${encodeURIComponent(project)}&load=1&path=knowledge%2Fwelcome.md`);
    expect(preview).toMatchObject({ status: 200, body: { path: 'knowledge/welcome.md', content: '# Welcome\n' } });

    const replayedElsewhere = await call(handler, 'GET', `/api/loom/vfs?cwd=${encodeURIComponent(project)}&load=1&directory=other&continuation=${encodeURIComponent(firstPage.body.vfs?.continuation ?? '')}`);
    expect(replayedElsewhere).toMatchObject({ status: 400, body: { error: { code: 'USAGE_INVALID' } } });
    const escaped = await call(handler, 'GET', `/api/loom/vfs?cwd=${encodeURIComponent(project)}&load=1&path=knowledge%2F..%2Foutside`);
    expect(escaped).toMatchObject({ status: 400, body: { error: { code: 'USAGE_INVALID' } } });

    const disabled = await call(handler, 'POST', '/api/loom/vfs', { action: 'set-vfs-enabled', cwd: project, enabled: false });
    expect(disabled).toMatchObject({ status: 200, body: { preferences: { vfsEnabled: false } } });
    expect(JSON.parse(await readFile(path.join(home, 'ui', `${Buffer.from(project).toString('base64url')}.json`), 'utf8'))).toEqual({ enabled: false });

    const reopened = await call(handler, 'GET', `/api/loom/vfs?cwd=${encodeURIComponent(project)}`);
    expect(reopened).toMatchObject({ status: 200, body: { vfs: { enabled: false, state: 'disabled' } } });

    const invalidPost = await call(handler, 'POST', '/api/loom/vfs', { action: 'unknown', cwd: project });
    expect(invalidPost).toMatchObject({ status: 400, body: { error: { code: 'USAGE_INVALID' } } });
    const wrongMethod = await call(handler, 'PUT', '/api/loom/vfs');
    expect(wrongMethod).toMatchObject({ status: 405, body: { error: { code: 'USAGE_INVALID' } } });
  });

  it('discovers knowledge before attachment and explicitly adds or removes a named knowledge set', async () => {
    const home = await mkdtemp(path.join(os.tmpdir(), 'loom-discovery-home-'));
    const project = await mkdtemp(path.join(os.tmpdir(), 'loom-discovery-project-'));
    roots.push(home, project);
    const task = path.join(home, 'tasks', 'unbound');
    await mkdir(task, { recursive: true });
    await writeFile(path.join(task, 'context.json'), JSON.stringify({
      version: 1, workspace: '', pinId: '', root: project, readOnly: true, mounts: [],
    }));

    const kc = createServer(async (req, res) => {
      const send = (body: unknown) => { res.writeHead(200, { 'content-type': 'application/json' }); res.end(JSON.stringify(body)); };
      if (req.url === '/catalog/v1/catalogs') return send({ catalogs: [{ id: 'kr://acme/catalog' }] });
      if (req.url?.endsWith('/repositories')) return send({ catalogId: 'kr://acme/catalog', repositories: [{ id: 'kr://kc/system', profile: 'missing', schemaCount: 4 }, { id: 'kr://acme/metrics', profile: 'missing', schemaCount: 1 }] });
      if (req.url?.endsWith('/workspaces')) return send({ catalogId: 'kr://acme/catalog', workspaces: [{ workspaceId: 'sales', revision: 1, sources: [{ repository: 'kr://acme/metrics' }] }] });
      if (req.url === '/knowledge/v1/schemas:page') {
        const chunks: Buffer[] = [];
        for await (const chunk of req) chunks.push(Buffer.from(chunk));
        const requestBody = JSON.parse(Buffer.concat(chunks).toString('utf8')) as { repository: string };
        return send({ repository: requestBody.repository, commit: 'c1', schemas: [{ objectId: 'schema/metric/definition/v1', entity: 'Metric' }], coverage: { enumerated: 1, total: 1, complete: true }, exhausted: true });
      }
      res.writeHead(404); res.end();
    });
    await new Promise<void>((resolve) => kc.listen(0, '127.0.0.1', resolve));
    const address = kc.address();
    if (!address || typeof address === 'string') throw new Error('test KC server did not bind');
    const kcURL = `http://127.0.0.1:${address.port}`;
    const log = path.join(home, 'kcfs.log');
    const fake = path.join(home, 'kcfs-fake.mjs');
    await writeFile(fake, `#!/usr/bin/env node
import fs from 'node:fs';
const args=process.argv.slice(2); fs.appendFileSync(${JSON.stringify(log)}, args.join(' ')+'\\n');
if(args[0]==='daemon-mount'){const root=args[args.indexOf('--root')+1];const workspace=args[args.indexOf('--workspace')+1];process.stdout.write(JSON.stringify({workspaceId:workspace,pinId:'pin-sales',root,readOnly:true,pid:5252,pin:{workspaceId:workspace,pinId:'pin-sales',repositories:{'kr://acme/metrics':'c1'}},mounts:[{path:'knowledge/metrics',mountpoint:root+'/knowledge/metrics',repository:'kr://acme/metrics',commit:'c1'}]}));}
`);
    await chmod(fake, 0o755);

    try {
      const handler = createLoomWorkspaceHandler({ home, bin: fake, server: kcURL, catalog: 'kr://acme/catalog', principal: 'agent:test' });
      const discovered = await call(handler, 'GET', `/api/loom/vfs?cwd=${encodeURIComponent(project)}&discover=1`);
      expect(discovered).toMatchObject({ status: 200, body: { state: 'unbound', inventory: { catalogs: [{ knowledgeSets: [{ id: 'sales' }] }] } } });
      expect(discovered.body.inventory?.catalogs[0].repositories[0].schemas[0].entity).toBe('Metric');

      const connected = await call(handler, 'POST', '/api/loom/vfs', { action: 'connect-workspace', cwd: project, catalog: 'kr://acme/catalog', workspace: 'sales' });
      expect(connected).toMatchObject({ status: 200, body: { connection: { workspace: 'sales', pinId: 'pin-sales' } } });
      const ready = await call(handler, 'GET', `/api/loom/vfs?cwd=${encodeURIComponent(project)}`);
      expect(ready).toMatchObject({ status: 200, body: { state: 'ready', workspace: 'sales' } });

      const disconnected = await call(handler, 'POST', '/api/loom/vfs', { action: 'disconnect-workspace', cwd: project });
      expect(disconnected).toMatchObject({ status: 200, body: { connection: { workspace: '' } } });
      expect(await readFile(log, 'utf8')).toContain('stop --pid 5252');
    } finally {
      await new Promise<void>((resolve, reject) => kc.close((error) => error ? reject(error) : resolve()));
    }
  });
});
