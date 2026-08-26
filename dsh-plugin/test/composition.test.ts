import { readFile } from 'node:fs/promises';
import { describe, expect, it } from 'vitest';

const root = new URL('../', import.meta.url);

describe('default DSH composition', () => {
  it('keeps stock host filesystem and search providers', async () => {
    const patch = await readFile(new URL('cordis.patch.yml', root), 'utf8');
    expect(patch).not.toMatch(/^- id: fs-sandbox/m);
    expect(patch).not.toMatch(/^- id: tool-fs-search/m);
    expect(patch).not.toMatch(/id: loom-(?:fs|search)/);
    expect(patch).not.toContain('KC_MOUNT_PATH');
	expect(patch).toContain('name: dsh-loom/knowledge');
	expect(patch).toMatch(/workspace: !!js process\.env\.KC_WORKSPACE/);
  });

  it('does not export the retired Agent-only filesystem providers', async () => {
    const pkg = JSON.parse(await readFile(new URL('package.json', root), 'utf8')) as { exports: Record<string, unknown> };
    expect(pkg.exports['./fs']).toBeUndefined();
    expect(pkg.exports['./search']).toBeUndefined();
  });
});
