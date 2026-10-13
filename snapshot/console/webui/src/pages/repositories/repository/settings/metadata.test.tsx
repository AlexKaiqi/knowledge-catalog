import React from 'react';
import {afterEach, expect, test, vi} from 'vitest';
import {cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';
import RepositoryMetadata from './metadata';

afterEach(() => {cleanup(); vi.unstubAllGlobals();});
function backend(refuseDelete = false) {
    let metadata: Record<string, string> = {description: '原说明', 'remove/me': '待删除'};
    const fetcher = vi.fn(async (_path: string, options: RequestInit) => {
        if (options.method === 'POST') {metadata = {...metadata, ...JSON.parse(String(options.body)).metadata}; return new Response(null, {status: 204});}
        if (options.method === 'DELETE') {
            if (refuseDelete) return new Response('{"message":"删除元数据被拒绝"}', {status: 403, headers: {'Content-Type': 'application/json'}});
            for (const key of JSON.parse(String(options.body)).keys) delete metadata[key];
            return new Response(null, {status: 204});
        }
        return new Response(JSON.stringify(metadata), {status: 200, headers: {'Content-Type': 'application/json'}});
    });
    vi.stubGlobal('fetch', fetcher);
    return fetcher;
}
test('Repository settings updates values and deletes native keys, then permits clearing everything', async () => {
    const fetcher = backend();
    render(<RepositoryMetadata repository="repo"/>);
    fireEvent.change(await screen.findByLabelText('元数据值 1'), {target: {value: '新说明\n原样保留'}});
    fireEvent.click(screen.getByRole('button', {name: '删除元数据 2'}));
    fireEvent.click(screen.getByRole('button', {name: '保存元数据'}));
    await screen.findByText('自定义元数据已保存并重新读取。');
    expect(JSON.parse(String(fetcher.mock.calls.find(([, options]) => options.method === 'POST')?.[1].body))).toEqual({metadata: {description: '新说明\n原样保留'}});
    expect(JSON.parse(String(fetcher.mock.calls.find(([, options]) => options.method === 'DELETE')?.[1].body))).toEqual({keys: ['remove/me']});
    expect((screen.getByLabelText('元数据值 1') as HTMLTextAreaElement).value).toBe('新说明\n原样保留');
    fireEvent.click(screen.getByRole('button', {name: '删除元数据 1'}));
    fireEvent.click(screen.getByRole('button', {name: '保存元数据'}));
    await screen.findByText('自定义元数据已保存并重新读取。');
    expect(screen.queryByLabelText('元数据键 1')).toBe(null);
    expect(fetcher.mock.calls.filter(([, options]) => options.method === 'DELETE').map(([, options]) => JSON.parse(String(options.body)))).toEqual([{keys: ['remove/me']}, {keys: ['description']}]);
});
test('a partially refused native save displays failure and rereads the accepted values', async () => {
    backend(true);
    render(<RepositoryMetadata repository="repo"/>);
    fireEvent.change(await screen.findByLabelText('元数据值 1'), {target: {value: '已保存的更新'}});
    fireEvent.click(screen.getByRole('button', {name: '删除元数据 2'}));
    fireEvent.click(screen.getByRole('button', {name: '保存元数据'}));
    await screen.findByText('删除元数据被拒绝');
    await waitFor(() => expect((screen.getByLabelText('元数据值 2') as HTMLTextAreaElement).value).toBe('待删除'));
    expect((screen.getByLabelText('元数据值 1') as HTMLTextAreaElement).value).toBe('已保存的更新');
    expect(screen.queryByText('自定义元数据已保存并重新读取。')).toBe(null);
});
test('a metadata read refusal does not become an empty editable map', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{"message":"没有读取元数据权限"}', {status: 403, headers: {'Content-Type': 'application/json'}})));
    render(<RepositoryMetadata repository="repo"/>);
    await screen.findByText('没有读取元数据权限');
    expect(screen.queryByRole('button', {name: '添加元数据'})).toBe(null);
    expect(screen.queryByRole('button', {name: '保存元数据'})).toBe(null);
    expect(screen.getByRole('button', {name: '重新读取元数据'})).toBeTruthy();
});
