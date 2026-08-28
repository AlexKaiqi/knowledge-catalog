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
		expect(patch).not.toMatch(/dsh-loom\/(?:control|knowledge|resource)/);
		expect(patch).toContain('name: dsh-loom');
		expect(patch).toMatch(/workspace: !!js process\.env\.KC_WORKSPACE/);
		expect(patch).toMatch(/id: loom-web-runtime\s+name: dsh-loom\/web/);
		expect(patch.match(/name: dsh-loom\/web/g)).toHaveLength(1);
  });

  it('does not export the retired Agent-only filesystem providers', async () => {
    const pkg = JSON.parse(await readFile(new URL('package.json', root), 'utf8')) as { exports: Record<string, unknown> };
    expect(pkg.exports['./fs']).toBeUndefined();
    expect(pkg.exports['./search']).toBeUndefined();
  });

  it('mounts only an optional rc-tree VFS under the host Workspace sidebar', async () => {
    const browser = await readFile(new URL('src/browser.tsx', root), 'utf8');
    const bundler = await readFile(new URL('rolldown.config.mjs', root), 'utf8');
    expect(browser).toContain("from 'rc-tree'");
    expect(browser).toContain("ctx.slots.inject('shell.overlay'");
    expect(browser).toContain('[data-slot="sidebar.workspaces"]');
    expect(browser).not.toContain("ctx.slots.inject('conversation.view'");
    expect(browser).not.toContain('KnowledgeView');
    expect(browser).not.toContain('FilesView');
    expect(browser).not.toContain('TreeRows');
    expect(browser).not.toContain('MountRows');
    expect(bundler).toContain("'process.env.NODE_ENV': JSON.stringify('production')");
  });

  it('targets the packaged web runtime consistently in headless model patches', async () => {
    for (const name of ['deepseek-official', 'lore-openai', 'openai-official', 'openrouter', 'volcengine']) {
      const patch = await readFile(new URL(`scripts/${name}.patch.yml`, root), 'utf8');
      expect(patch).toContain('id: loom-web-runtime');
      expect(patch).not.toMatch(/id: loom-web\s/);
    }
  });
});
