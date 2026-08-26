import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
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

  it('discovers the nearest project binding from a nested Agent cwd', async () => {
    const cwd = await mkdtemp(path.join(os.tmpdir(), 'dsh-loom-project-'));
    roots.push(cwd);
    const nested = path.join(cwd, 'services', 'api');
    await mkdir(nested, { recursive: true });
    await writeFile(path.join(cwd, '.dsh-loom-workspace.json'), JSON.stringify({
      version: 1, catalog: 'kr://acme/catalog', workspace: 'warehouse',
    }));

    await expect(readWorkspaceBinding(nested)).resolves.toEqual({
      catalog: 'kr://acme/catalog', workspace: 'warehouse',
    });
  });

  it('reports a malformed nearest binding instead of silently using another scope', async () => {
    const cwd = await mkdtemp(path.join(os.tmpdir(), 'dsh-loom-invalid-binding-'));
    roots.push(cwd);
    await writeFile(path.join(cwd, '.dsh-loom-workspace.json'), '{broken');

    await expect(readWorkspaceBinding(cwd)).rejects.toThrow('invalid Workspace binding');
  });
});
