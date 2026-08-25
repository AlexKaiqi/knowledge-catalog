import { Context } from '@deepseek-ai/cordis';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { spawn, type ChildProcess } from 'node:child_process';
import { existsSync } from 'node:fs';
import { execFile as execFileCallback } from 'node:child_process';
import { mkdtemp, readFile, readdir, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { promisify } from 'node:util';
import LoomFileSystem from '../src/fs.js';

const execFile = promisify(execFileCallback);

/**
 * Drives the real Loom stack end to end: a real `kc serve` process (built
 * from this repo's Go source), a real repo-add + define-workspace over HTTP, and
 * the actual LoomFileSystem class (not a mock) exercising every ctx.fs
 * method the way dsh itself would call them. This is the one place the TS
 * side gets tested against the real protocol rather than a fabricated
 * fetch(); if `go build` is unavailable the suite skips instead of failing.
 */

const KC_BIN = process.env.KC_BIN ?? path.join(import.meta.dirname, '..', '..', 'kc-test-bin');
const haveKc = existsSync(KC_BIN);

describe.skipIf(!haveKc)('LoomFileSystem over a real kc serve', () => {
  let home: string;
  let port: number;
  let proc: ChildProcess;
  let baseURL: string;
  let fs: LoomFileSystem;

  function post(verb: string, body: Record<string, unknown>): Promise<any> {
    return fetch(`${baseURL}/v1/${verb}`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(body),
    }).then(async (res) => {
      const json = await res.json();
      if (!res.ok) throw new Error(`${verb}: ${JSON.stringify(json)}`);
      return json;
    });
  }

  async function waitReady(): Promise<void> {
    const deadline = Date.now() + 10000;
    while (Date.now() < deadline) {
      try {
        const res = await fetch(`${baseURL}/health`);
        if (res.ok) return;
      } catch {
        // not up yet
      }
      await new Promise((r) => setTimeout(r, 100));
    }
    throw new Error('kc serve did not become ready');
  }

  beforeAll(async () => {
    home = await mkdtemp(path.join(tmpdir(), 'dsh-loom-'));
    port = 20000 + Math.floor(Math.random() * 10000);
    baseURL = `http://127.0.0.1:${port}`;
    proc = spawn(KC_BIN, ['--home', home, 'serve', '--listen', `127.0.0.1:${port}`], { stdio: 'pipe' });
    const failure = new Promise<never>((_, reject) => {
      proc.once('error', reject);
      proc.once('exit', (code) => reject(new Error(`kc serve exited early with code ${code}`)));
    });
    await Promise.race([waitReady(), failure]);

    await post('init', { catalog: 'kr://acme/catalog' });
    await post('repo-add', { repo: 'kr://acme/personals/alice' });
    await post('repo-add', { repo: 'kr://acme/public/semantic' });
    await post('define-workspace', {
      workspace: 'notes',
      revision: 1,
      source: ['kr://acme/personals/alice=refs/heads/main@', 'kr://acme/public/semantic=refs/heads/main@refs/semantic'],
    });

    fs = new LoomFileSystem(new Context(), { baseURL, workspace: 'notes' });
  }, 30000);

  afterAll(async () => {
    proc?.kill();
    if (home) await rm(home, { recursive: true, force: true });
  });

  it('writes then reads back exact text, across both the root and a nested mount', async () => {
    const rootTarget = await fs.resolve('analysis/churn.md');
    const created = await fs.writeText(rootTarget, 'draft one\n');
    expect(created.operation).toBe('create');
    expect(created.before).toBeNull();
    expect(created.after).toBe('draft one\n');

    const read = await fs.readText(rootTarget);
    expect(read).toBe('draft one\n');

    const nestedTarget = await fs.resolve('refs/semantic/metrics/dau.md');
    await fs.writeText(nestedTarget, 'daily actives\n');
    expect(await fs.readText(nestedTarget)).toBe('daily actives\n');
  });

  it('stat distinguishes file, directory, and absent', async () => {
    const file = await fs.stat(await fs.resolve('analysis/churn.md'));
    expect(file?.type).toBe('file');

    const dir = await fs.stat(await fs.resolve('analysis'));
    expect(dir?.type).toBe('directory');

    const root = await fs.stat(await fs.resolve(''));
    expect(root?.type).toBe('directory');

    const absent = await fs.stat(await fs.resolve('nowhere/at/all.md'));
    expect(absent).toBeUndefined();
  });

  it('listDir returns direct children only, across the composed tree', async () => {
    const rootChildren = await fs.listDir(await fs.resolve(''));
    const names = rootChildren.map((c) => c.name).sort();
    expect(names).toContain('analysis');
    expect(names).toContain('refs');
    expect(rootChildren.find((c) => c.name === 'analysis')?.type).toBe('directory');

    const nestedChildren = await fs.listDir(await fs.resolve('refs/semantic/metrics'));
    expect(nestedChildren.map((c) => c.name)).toContain('dau.md');
  });

  it('writeText with replaceIfVersion enforces CAS: a stale version is rejected', async () => {
    const target = await fs.resolve('analysis/versioned.md');
    const first = await fs.writeText(target, 'v1\n');
    await fs.writeText(target, 'v2\n', { kind: 'replaceIfVersion', version: first.version });

    await expect(
      fs.writeText(target, 'v3-stale\n', { kind: 'replaceIfVersion', version: first.version }),
    ).rejects.toMatchObject({ code: 'FS_STALE_VERSION' });
  });

  it('writeText with createIfAbsent rejects an existing target', async () => {
    const target = await fs.resolve('analysis/exists-already.md');
    await fs.writeText(target, 'first\n');
    await expect(fs.writeText(target, 'second\n', { kind: 'createIfAbsent' })).rejects.toThrow();
  });

  it('editText applies a literal replace and advances the version', async () => {
    const target = await fs.resolve('analysis/editable.md');
    await fs.writeText(target, 'hello world\n');
    const edited = await fs.editText(target, { oldString: 'world', newString: 'loom', replaceAll: false });
    expect(edited.after).toBe('hello loom\n');
    expect(await fs.readText(target)).toBe('hello loom\n');
  });

  it('editText reports FS_EDIT_NOT_FOUND and FS_AMBIGUOUS_EDIT honestly', async () => {
    const target = await fs.resolve('analysis/ambiguous.md');
    await fs.writeText(target, 'a a\n');
    await expect(fs.editText(target, { oldString: 'zzz', newString: 'x', replaceAll: false }, undefined)).rejects.toMatchObject({
      code: 'FS_EDIT_NOT_FOUND',
    });
    await expect(fs.editText(target, { oldString: 'a', newString: 'x', replaceAll: false }, undefined)).rejects.toMatchObject({
      code: 'FS_AMBIGUOUS_EDIT',
    });
  });

  it('readText on a path nobody mounts is FS_NOT_FOUND, not a generic error', async () => {
    await expect(fs.readText(await fs.resolve('nowhere/at/all.md'))).rejects.toMatchObject({ code: 'FS_NOT_FOUND' });
  });

  it('readBytes returns raw content and FS_TOO_LARGE when over the cap', async () => {
    const target = await fs.resolve('analysis/bytes.md');
    await fs.writeText(target, 'twelve bytes');
    const raw = await fs.readBytes(target, undefined, 64);
    expect(new TextDecoder().decode(raw)).toBe('twelve bytes');
    await expect(fs.readBytes(target, undefined, 4)).rejects.toMatchObject({ code: 'FS_TOO_LARGE' });
  });

  it('contains reflects the virtual path hierarchy', async () => {
    const parent = await fs.resolve('analysis');
    const child = await fs.resolve('analysis/churn.md');
    const unrelated = await fs.resolve('refs/semantic/metrics/dau.md');
    expect(fs.contains(parent, child)).toBe(true);
    expect(fs.contains(parent, unrelated)).toBe(false);
  });

  it('resolve is stable: the same path yields the same targetKey', async () => {
    const a = await fs.resolve('analysis/churn.md');
    const b = await fs.resolve('/analysis/churn.md');
    expect(a.targetKey).toBe(b.targetKey);
  });

  it('overlays read-only remote knowledge without materializing into a non-Git project', async () => {
    const project = await mkdtemp(path.join(tmpdir(), 'dsh-project-plain-'));
    try {
      await writeFile(path.join(project, 'TASK.md'), 'Use the mounted evidence.\n');
      await post('vfs-write', {
        workspace: 'notes', 'command-id': 'overlay-plain-v1', path: 'evidence/answer.md',
        content: Buffer.from('ANSWER=REMOTE-731\n').toString('base64'),
      });
      const overlay = new LoomFileSystem(new Context(), {
        baseURL, workspace: 'notes', mountPath: '.knowledge',
      });
      const signal = new AbortController().signal;

      expect(await overlay.readText(await overlay.resolve('TASK.md', { cwd: project, signal }), signal))
        .toBe('Use the mounted evidence.\n');
      expect(await overlay.readText(await overlay.resolve('.knowledge/evidence/answer.md', { cwd: project, signal }), signal))
        .toBe('ANSWER=REMOTE-731\n');
      const root = await overlay.listDir(await overlay.resolve('.', { cwd: project, signal }), signal);
      expect(root.map((entry) => entry.name).sort()).toEqual(['.knowledge', 'TASK.md']);
      expect(await readdir(project)).toEqual(['TASK.md']);
      await expect(overlay.writeText(
        await overlay.resolve('.knowledge/evidence/answer.md', { cwd: project, signal }),
        'forbidden\n', undefined, signal,
      )).rejects.toMatchObject({ code: 'FS_SANDBOX_DENIED' });
      expect(await readdir(project)).toEqual(['TASK.md']);
    } finally {
      await rm(project, { recursive: true, force: true });
    }
  });

  it('keeps one Agent task pinned while a new task observes the remote update', async () => {
    const project = await mkdtemp(path.join(tmpdir(), 'dsh-project-pin-'));
    try {
      await post('vfs-write', {
        workspace: 'notes', 'command-id': 'overlay-pin-v1', path: 'updates/project-version.txt',
        content: Buffer.from('V1\n').toString('base64'),
      });
      const overlay = new LoomFileSystem(new Context(), { baseURL, workspace: 'notes', mountPath: '.knowledge' });
      const oldTask = new AbortController().signal;
      const oldPath = await overlay.resolve('.knowledge/updates/project-version.txt', { cwd: project, signal: oldTask });
      expect(await overlay.readText(oldPath, oldTask)).toBe('V1\n');

      await post('vfs-write', {
        workspace: 'notes', 'command-id': 'overlay-pin-v2', path: 'updates/project-version.txt',
        content: Buffer.from('V2\n').toString('base64'),
      });

      const oldPathAgain = await overlay.resolve('.knowledge/updates/project-version.txt', { cwd: project, signal: oldTask });
      expect(await overlay.readText(oldPathAgain, oldTask)).toBe('V1\n');
      const newTask = new AbortController().signal;
      const newPath = await overlay.resolve('.knowledge/updates/project-version.txt', { cwd: project, signal: newTask });
      expect(await overlay.readText(newPath, newTask)).toBe('V2\n');
    } finally {
      await rm(project, { recursive: true, force: true });
    }
  });

  it('keeps ordinary local project edits working while the remote mount stays read-only', async () => {
    const project = await mkdtemp(path.join(tmpdir(), 'dsh-project-edit-'));
    try {
      await writeFile(path.join(project, 'draft.txt'), 'local v1\n');
      const overlay = new LoomFileSystem(new Context(), { baseURL, workspace: 'notes', mountPath: '.knowledge' });
      const signal = new AbortController().signal;
      const local = await overlay.resolve('draft.txt', { cwd: project, signal });
      const edited = await overlay.editText(local, {
        oldString: 'v1', newString: 'v2', replaceAll: false,
      }, undefined, signal);
      expect(edited.after).toBe('local v2\n');
      expect(await readFile(path.join(project, 'draft.txt'), 'utf8')).toBe('local v2\n');
      expect(overlay.processPath(local)).toBe(await realpath(path.join(project, 'draft.txt')));

      const remote = await overlay.resolve('.knowledge/evidence/answer.md', { cwd: project, signal });
      expect(overlay.processPath(remote)).toBe('loom://evidence/answer.md');
      await expect(overlay.editText(remote, {
        oldString: 'REMOTE', newString: 'MUTATED', replaceAll: false,
      }, undefined, signal)).rejects.toMatchObject({ code: 'FS_SANDBOX_DENIED' });
    } finally {
      await rm(project, { recursive: true, force: true });
    }
  });

  it('does not change a Git project index or working tree while reading mounted knowledge', async () => {
    const project = await mkdtemp(path.join(tmpdir(), 'dsh-project-git-'));
    try {
      await execFile('git', ['init', '-q', '-b', 'main'], { cwd: project });
      await writeFile(path.join(project, 'app.txt'), 'local-app\n');
      await execFile('git', ['add', 'app.txt'], { cwd: project });
      await execFile('git', ['-c', 'user.name=fixture', '-c', 'user.email=fixture@local', 'commit', '-q', '-m', 'root'], { cwd: project });
      const before = await execFile('git', ['status', '--porcelain=v1'], { cwd: project });
      const overlay = new LoomFileSystem(new Context(), { baseURL, workspace: 'notes', mountPath: '.knowledge' });
      const signal = new AbortController().signal;
      const target = await overlay.resolve('.knowledge/evidence/answer.md', { cwd: project, signal });
      expect(await overlay.readText(target, signal)).toContain('REMOTE-731');
      expect(await readFile(path.join(project, 'app.txt'), 'utf8')).toBe('local-app\n');
      const after = await execFile('git', ['status', '--porcelain=v1'], { cwd: project });
      expect(after.stdout).toBe(before.stdout);
      expect(existsSync(path.join(project, '.knowledge'))).toBe(false);
    } finally {
      await rm(project, { recursive: true, force: true });
    }
  });

  it('rejects an ambiguous mount instead of hiding an existing project directory', async () => {
    const project = await mkdtemp(path.join(tmpdir(), 'dsh-project-collision-'));
    try {
      await writeFile(path.join(project, '.knowledge'), 'occupied\n');
      const overlay = new LoomFileSystem(new Context(), { baseURL, workspace: 'notes', mountPath: '.knowledge' });
      await expect(overlay.resolve('.knowledge/evidence/answer.md', {
        cwd: project, signal: new AbortController().signal,
      })).rejects.toMatchObject({ code: 'FS_SANDBOX_DENIED' });
    } finally {
      await rm(project, { recursive: true, force: true });
    }
  });
});
