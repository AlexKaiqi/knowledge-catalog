import { describe, expect, it } from 'vitest';
import { parseDefinition } from './definition';

describe('Dataset definition', () => {
    it('keeps explicit file mappings, fixed commits and source CAS in JSON', () => {
        const value = { dataset: 'docs', revision: 2, sources: [{ repository: 'kr://example/source', selector: 'refs/heads/main', commit: 'c1', baseRev: 'c0', file: 'README.md', target: 'guide.md' }] };
        expect(parseDefinition(JSON.stringify(value), 'docs')).toEqual(value);
    });
    it('refuses unsupported metadata and renaming rather than dropping fields', () => {
        const value = { dataset: 'docs', revision: 2, sources: [{ repository: 'repo', selector: 'refs/heads/main', path: '' }] };
        expect(() => parseDefinition(JSON.stringify({...value, metadata: {owner: 'x'}}))).toThrow(/metadata/);
        expect(() => parseDefinition(JSON.stringify({...value, dataset: 'renamed'}), 'docs')).toThrow(/名称/);
        expect(() => parseDefinition(JSON.stringify({...value, sources: [{...value.sources[0], table: {name: 'a'}}]}))).toThrow(/table/);
    });
    it('does not turn a path-less legacy source into a root mount', () => {
        const value = { dataset: 'docs', revision: 3, sources: [{repository: 'repo', commit: 'c1', selector: 'refs/heads/main'}] };
        expect(parseDefinition(JSON.stringify(value)).sources[0]).not.toHaveProperty('path');
    });
    it('refuses an empty file field instead of treating it as a directory source', () => {
        expect(() => parseDefinition(JSON.stringify({dataset: 'docs', revision: 1, sources: [{repository: 'repo', file: ''}]}))).toThrow('文件路径不能为空');
    });
});
