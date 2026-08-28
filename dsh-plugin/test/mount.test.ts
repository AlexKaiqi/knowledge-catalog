import { chmod, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import { MountController } from '../src/mount.js';

describe('task mount controller', () => {
  const roots: string[] = [];
  afterEach(async () => Promise.all(roots.splice(0).map((root) => rm(root, { recursive: true, force: true }))));

  it('waits for ready, shares a parent mount, stores context under KC_HOME, and stops at the final release', async () => {
    const root = await mkdtemp(path.join(os.tmpdir(), 'loom-mount-'));
    const home = await mkdtemp(path.join(os.tmpdir(), 'loom-home-'));
    roots.push(root, home);
    const log = path.join(home, 'calls.log');
    const fake = path.join(home, 'kcfs-fake.mjs');
    await writeFile(fake, `#!/usr/bin/env node
import fs from 'node:fs';
const args=process.argv.slice(2); fs.appendFileSync(${JSON.stringify(log)}, args.join(' ')+'\\n');
if(args[0]==='daemon-mount'){const root=args[args.indexOf('--root')+1];process.stdout.write(JSON.stringify({workspaceId:'agent',pinId:'pin-1',root,readOnly:true,pid:4242,mounts:[{path:'knowledge',mountpoint:root+'/knowledge',repository:'kr://acme/docs',commit:'c1',files:1}]}));}
`);
    await chmod(fake, 0o755);
    const controller = new MountController({ home, bin: fake, workspace: 'agent', principal: 'agent:test' });
    controller.created({ id: 'parent', header: { cwd: root } });
    controller.created({ id: 'child', header: { cwd: root, parentSession: 'parent' } });

    const context = JSON.parse(await readFile(path.join(home, 'tasks', Buffer.from('child').toString('base64url'), 'context.json'), 'utf8'));
    expect(context).toMatchObject({ workspace: 'agent', pinId: 'pin-1', root, readOnly: true });
    controller.disposed({ id: 'parent', header: { cwd: root } });
    expect((await readFile(log, 'utf8')).match(/daemon-mount/g)).toHaveLength(1);
    controller.disposed({ id: 'child', header: { cwd: root, parentSession: 'parent' } });
    const calls = await readFile(log, 'utf8');
    expect(calls.match(/daemon-mount/g)).toHaveLength(1);
    expect(calls).toContain('stop --pid 4242');
  });

  it('uses the typed remote gateway mode without passing the local Home to kcfs', async () => {
    const root = await mkdtemp(path.join(os.tmpdir(), 'loom-remote-mount-'));
    const home = await mkdtemp(path.join(os.tmpdir(), 'loom-remote-home-'));
    roots.push(root, home);
    const log = path.join(home, 'calls.log');
    const fake = path.join(home, 'kcfs-fake.mjs');
    await writeFile(fake, `#!/usr/bin/env node
import fs from 'node:fs';
const args=process.argv.slice(2); fs.appendFileSync(${JSON.stringify(log)}, args.join(' ')+'\\n');
if(args[0]==='daemon-mount'){const root=args[args.indexOf('--root')+1];process.stdout.write(JSON.stringify({workspaceId:'agent',pinId:'pin-remote',root,readOnly:true,pid:4343,mounts:[]}));}
`);
    await chmod(fake, 0o755);
    const controller = new MountController({ home, bin: fake, server: 'https://kc.example', workspace: 'agent', principal: 'agent:test' });
    controller.created({ id: 'remote', header: { cwd: root } });
    controller.disposed({ id: 'remote', header: { cwd: root } });
    const calls = await readFile(log, 'utf8');
    expect(calls).toContain('daemon-mount --server https://kc.example');
    expect(calls).not.toContain(`--home ${home}`);
  });
});
