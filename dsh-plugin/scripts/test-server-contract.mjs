// Invoked by cli's dsh_contract build-tag test against its real HTTP Server.
import { createLoomWorkspaceHandler } from '../dist/web.js';
import { mkdtemp, mkdir, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { Readable } from 'node:stream';
import assert from 'node:assert/strict';
import os from 'node:os';
import path from 'node:path';

const home = await mkdtemp(path.join(os.tmpdir(), 'kc-consumer-contract-'));
const root = path.join(home, 'project');
const server = process.env.KC_DSH_CONTRACT_SERVER;
assert.ok(server);
try {
  await mkdir(root);
  await mkdir(path.join(home, 'tasks', 'active'), { recursive: true });
  await writeFile(path.join(home, 'tasks', 'active', 'context.json'), JSON.stringify({ version: 1, workspace: '', root, readOnly: true, mounts: [] }));
  const handler = createLoomWorkspaceHandler({ home, server, principal: 'consumer', bin: '/FUSE-is-not-required/kcfs' });
  async function request(method, url, body) {
    const input = Object.assign(Readable.from(body ? [JSON.stringify(body)] : []), { method, url });
    let status, value;
    await handler(input, { writeHead(code) { status = code; }, end(data) { value = JSON.parse(data); } });
    assert.equal(status, 200, JSON.stringify(value));
    return value;
  }
  const url = `/api/loom/vfs?cwd=${encodeURIComponent(root)}`;
  const inventory = await request('GET', `${url}&discover=1`);
  assert.equal(inventory.inventoryError, undefined);
  const catalog = inventory.inventory.catalogs.find((item) => item.id === 'kr://probe/catalog');
  assert.deepEqual(catalog.knowledgeSets.find((item) => item.id === 'named').repositories, ['kr://probe/docs']);
  await request('POST', '/api/loom/vfs', { action: 'connect-workspace', cwd: root, catalog: catalog.id, workspace: 'named' });
  assert.equal((await request('GET', url)).workspace, 'named');
  await request('POST', '/api/loom/vfs', { action: 'connect-sources', cwd: root, catalog: catalog.id, repositories: ['kr://probe/docs'] });
  const listing = await request('GET', url);
  assert.equal(listing.state, 'ready');
  assert.equal(listing.filesMounted, false);
  const [projectContext] = await readdir(path.join(home, 'projects'));
  const context = JSON.parse(await readFile(path.join(home, 'projects', projectContext, 'context.json'), 'utf8'));
  assert.equal(context.pin.definition.sources[0].selector, process.env.KC_DSH_DEFAULT_REF);
  const { definition, catalog: catalogID, ...pin } = context.pin;
  const read = await fetch(`${server}/knowledge/v1/objects:read`, {
    method: 'POST', headers: { 'content-type': 'application/json', 'x-kc-as': 'consumer' },
    body: JSON.stringify({ catalog: catalogID, definition, pin, object: 'note/one' }),
  });
  const content = await read.json();
  assert.equal(read.status, 200, JSON.stringify(content));
  assert.match(JSON.stringify(content), /consumed through the actual Server/);
  assert.equal(context.pinId, listing.pin.pinId);
  console.log('PASS actual Server inventory → named pin → selected source pin → fixed structured read, with no FUSE');
} finally { await rm(home, { recursive: true, force: true }); }
