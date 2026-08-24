import { describe, expect, it } from 'vitest';
import { globRegex, matchGlob } from '../src/search.js';

const entries = [
  { path: 'README.md', repository: 'root', commit: 'a' },
  { path: 'analysis/brief.md', repository: 'root', commit: 'a' },
  { path: 'refs/semantic/governance/audit/window-17.md', repository: 'semantic', commit: 'b' },
  { path: 'refs/semantic/model/table.go', repository: 'semantic', commit: 'b' },
];

describe('Workspace glob matching', () => {
  it('matches basename patterns at every depth', () => {
    expect(matchGlob(entries, '*.md')).toEqual([
      'README.md',
      'analysis/brief.md',
      'refs/semantic/governance/audit/window-17.md',
    ]);
  });

  it('matches recursive patterns relative to a search root', () => {
    expect(matchGlob(entries, '**/*.md', 'refs/semantic')).toEqual([
      'refs/semantic/governance/audit/window-17.md',
    ]);
  });

  it('supports question marks and brace alternatives', () => {
    expect(globRegex('*.{go,ts}').test('table.go')).toBe(true);
    expect(globRegex('window-??.md').test('window-17.md')).toBe(true);
  });
});
