import {afterEach, expect, test, vi} from 'vitest';
import {repositories} from './index';
import {metadataFromRows} from './kcMetadata';

afterEach(() => {vi.unstubAllGlobals();});

test('native metadata keeps its response and mutation contracts with the original user proof', async () => {
    const fetcher = vi.fn(async (_path: string, options: RequestInit) => options.method ? new Response(null, {status: 204}) :
        new Response(JSON.stringify({'自定义/键': '原始\n内容', description: ''}), {status: 200, headers: {'Content-Type': 'application/json'}}));
    vi.stubGlobal('fetch', fetcher);
    expect(await repositories.metadata('repo/one')).toEqual({'自定义/键': '原始\n内容', description: ''});
    await repositories.setMetadata('repo/one', {['__proto__']: 'custom', '自定义/键': '更新'});
    await repositories.deleteMetadata('repo/one', ['description']);
    const calls = fetcher.mock.calls;
    expect(calls.map(([path]) => path)).toEqual(Array(3).fill('/api/v1/repositories/repo%2Fone/metadata'));
    expect(JSON.parse(String(calls[1][1].body))).toEqual({metadata: {['__proto__']: 'custom', '自定义/键': '更新'}});
    expect(JSON.parse(String(calls[2][1].body))).toEqual({keys: ['description']});
    for (const [, options] of calls) {
        expect(options.credentials).toBe('same-origin');
        expect(new Headers(options.headers).get('X-Kc-Console')).toBe('1');
    }
});

test('invalid metadata and refusal remain errors instead of successful empty data', async () => {
    const fetcher = vi.fn(async () => new Response('{"message":"元数据写权限不足"}', {status: 403, headers: {'Content-Type': 'application/json'}}));
    vi.stubGlobal('fetch', fetcher);
    await expect(repositories.metadata('repo')).rejects.toThrow('元数据写权限不足');
    await expect(repositories.setMetadata('repo', {count: 1})).rejects.toThrow('字符串');
    expect(fetcher).toHaveBeenCalledTimes(1);
});

test('metadata rows keep arbitrary keys and empty values, and reject repeated keys', () => {
    const rows = [{key: '__proto__', value: 'custom'}, {key: '', value: ''}];
    const result = metadataFromRows(rows);
    expect(Object.prototype.hasOwnProperty.call(result, '__proto__')).toBe(true);
    expect(result['__proto__']).toBe('custom');
    expect(result['']).toBe('');
    expect(() => metadataFromRows([...rows, {key: '', value: 'duplicate'}])).toThrow('不能重复');
});
