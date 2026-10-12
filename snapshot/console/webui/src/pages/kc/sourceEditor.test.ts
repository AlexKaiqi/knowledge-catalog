import { expect, test } from 'vitest';
import { emptyRow, sourceInputs, sourceRow } from './sourceEditor';

test('a blank branch still supplies the KC default reference required by accepted publications', () => {
    expect(sourceInputs([{ ...emptyRow(), repository: 'kr://delivery/docs', source: 'handbook/policies', target: 'reference' }])).toEqual([
        { repository: 'kr://delivery/docs', selector: 'refs/heads/main', commit: undefined, path: 'reference', subPath: 'handbook/policies' },
    ]);
});

test('an explicit commit retains a default reference when the branch field is blank', () => {
    expect(sourceInputs([{ ...emptyRow(), repository: 'kr://delivery/docs', basis: 'commit', commit: 'c1' }])[0]).toMatchObject({ selector: 'refs/heads/main', commit: 'c1' });
});

test('editing a legacy source without a delivered path never turns it into a whole-repository mount', () => {
    const source = { repository: 'kr://delivery/docs', selector: 'refs/heads/main', commit: 'c1', subPath: 'handbook' };
    const result = sourceInputs([sourceRow(source)])[0];
    expect(result).toEqual(source);
    expect(result).not.toHaveProperty('path');
});

test('per-file entries preserve a renamed target without creating a directory mount', () => {
    const source = { repository: 'kr://delivery/docs', selector: 'refs/heads/main', commit: 'c1', file: 'handbook/policies/README.md', target: 'reference/policy.md' };
    expect(sourceInputs([sourceRow(source)])[0]).toEqual(source);
    expect(sourceInputs([sourceRow(source)])[0]).not.toHaveProperty('path');
});
