import { describe, expect, it } from 'vitest';
import { directChildren, directoryExists, isUnder, joinPath, normalizePath } from '../src/tree.js';

describe('normalizePath', () => {
  it('collapses ./ and leading/trailing slashes, root is ""', () => {
    expect(normalizePath('')).toBe('');
    expect(normalizePath('/')).toBe('');
    expect(normalizePath('./a/./b/')).toBe('a/b');
    expect(normalizePath('/a/b')).toBe('a/b');
  });

  it('joins a relative path against cwd, matching resolve(path, {cwd})', () => {
    expect(normalizePath('b.md', 'a')).toBe('a/b.md');
    expect(normalizePath('../b.md', 'a/nested')).toBe('a/b.md');
  });

  it('an absolute path ignores cwd', () => {
    expect(normalizePath('/x/y', 'a/nested')).toBe('x/y');
  });

  it('an existing host-absolute cwd is the virtual root, not a prefix', () => {
    expect(normalizePath('analysis/hello.md', '/tmp')).toBe('analysis/hello.md');
    expect(normalizePath('/analysis/hello.md', '/tmp')).toBe('analysis/hello.md');
  });

  it('strips a host-absolute path that lives under session cwd (skill-filesystem)', () => {
    expect(normalizePath('/tmp/.dsh/skills/notes-ops/SKILL.md', '/tmp')).toBe('.dsh/skills/notes-ops/SKILL.md');
    expect(normalizePath('/tmp', '/tmp')).toBe('');
  });
});

describe('isUnder', () => {
  it('root is under everything', () => {
    expect(isUnder('', 'anything/at/all.md')).toBe(true);
  });
  it('a path is under itself and its own descendants only', () => {
    expect(isUnder('a/b', 'a/b')).toBe(true);
    expect(isUnder('a/b', 'a/b/c.md')).toBe(true);
    expect(isUnder('a/b', 'a/bc.md')).toBe(false); // no false positive on a string-prefix sibling
    expect(isUnder('a/b', 'a/c.md')).toBe(false);
  });
});

describe('directChildren', () => {
  const paths = ['analysis/churn.md', 'analysis/retention.md', 'refs/semantic/metrics/dau.md', 'readme.md'];

  it('lists direct files and synthesizes directories for deeper paths, at root, sorted by name', () => {
    const children = directChildren(paths, '');
    expect(children).toEqual([
      { name: 'analysis', type: 'directory' },
      { name: 'readme.md', type: 'file' },
      { name: 'refs', type: 'directory' },
    ]);
  });

  it('descends correctly at a nested directory', () => {
    expect(directChildren(paths, 'analysis')).toEqual([
      { name: 'churn.md', type: 'file' },
      { name: 'retention.md', type: 'file' },
    ]);
  });

  it('a directory two levels deep synthesizes correctly, without leaking siblings', () => {
    expect(directChildren(paths, 'refs')).toEqual([{ name: 'semantic', type: 'directory' }]);
    expect(directChildren(paths, 'refs/semantic')).toEqual([{ name: 'metrics', type: 'directory' }]);
  });

  it('a path with no descendants has no children', () => {
    expect(directChildren(paths, 'nowhere')).toEqual([]);
  });
});

describe('directoryExists', () => {
  const paths = ['a/b/c.md'];
  it('root always exists', () => {
    expect(directoryExists(paths, '')).toBe(true);
    expect(directoryExists([], '')).toBe(true);
  });
  it('an ancestor of a real path exists', () => {
    expect(directoryExists(paths, 'a')).toBe(true);
    expect(directoryExists(paths, 'a/b')).toBe(true);
  });
  it('a path that owns no descendants does not exist as a directory', () => {
    expect(directoryExists(paths, 'a/b/c.md')).toBe(false); // it is a file, not a directory
    expect(directoryExists(paths, 'nowhere')).toBe(false);
  });
});

describe('joinPath', () => {
  it('joins under root without a leading slash', () => {
    expect(joinPath('', 'a.md')).toBe('a.md');
  });
  it('joins under a nested parent', () => {
    expect(joinPath('a/b', 'c.md')).toBe('a/b/c.md');
  });
});
