import { mkdtemp, readFile, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import { ensureWorkspaceAnchor, readWorkspaceBinding } from '../src/binding.js';

describe('Catalog Workspace session binding', () => {
  const roots: string[] = [];
  afterEach(async () => Promise.all(roots.splice(0).map((root) => rm(root, { recursive: true, force: true }))));

  it('creates a stable DSH workspace anchor carrying the Catalog binding', async () => {
    const home = await mkdtemp(path.join(os.tmpdir(), 'dsh-loom-binding-'));
    roots.push(home);
    const binding = { catalog: 'kr://acme/catalog', workspace: 'warehouse' };
    const first = await ensureWorkspaceAnchor(home, binding);
    const second = await ensureWorkspaceAnchor(home, binding);

    expect(second).toBe(first);
    await expect(readWorkspaceBinding(first)).resolves.toEqual(binding);
    expect(JSON.parse(await readFile(path.join(first, '.dsh-loom-workspace.json'), 'utf8'))).toMatchObject({ version: 1, ...binding });
  });

  it('does not infer a binding from an arbitrary host workspace', async () => {
    const cwd = await mkdtemp(path.join(os.tmpdir(), 'dsh-loom-unbound-'));
    roots.push(cwd);
    await expect(readWorkspaceBinding(cwd)).resolves.toBeUndefined();
  });
});
