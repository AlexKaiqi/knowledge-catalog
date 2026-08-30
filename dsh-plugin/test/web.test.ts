import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
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

  it('is discoverable but collapsed by default and remembers an explicit opt-out', async () => {
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
    expect(first).toMatchObject({ status: 200, body: { workspace: 'agent', vfs: { enabled: true, state: 'collapsed', entries: [] } } });

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
});
