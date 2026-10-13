import React from 'react';
import {afterEach, expect, test, vi} from 'vitest';
import {cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';
import KCApp from '../../extendable/kcApp';
import {session} from '../../lib/api/session';
import {mockKC} from '../../../test/mockKC';

afterEach(() => {cleanup(); session.logout(); window.localStorage.clear(); window.sessionStorage.clear(); vi.unstubAllGlobals();});
function datasetBackend(path: string) {
    window.history.replaceState({}, '', path);
    const backend = mockKC();
    const fetcher = vi.fn(async (input: string, options?: RequestInit) => {
        const response = await backend.fetcher(input, options);
        const url = new URL(input, window.location.origin);
        if (url.pathname === '/snapshot/v1/datasets/shared') {
            const detail = await response.json();
            detail.metadata = {description: `说明 v${detail.revision}`, '任意/键': '保留\n内容'};
            return new Response(JSON.stringify(detail), {status: 200});
        }
        return response;
    });
    vi.stubGlobal('fetch', fetcher);
    render(<KCApp/>);
    return fetcher;
}

test('a historical Dataset displays metadata from the selected revision and its JSON', async () => {
    datasetBackend('/ui/datasets/shared?revision=7');
    await screen.findByText('自定义元数据 · v7');
    expect(screen.getByText('说明 v7')).toBeTruthy();
    fireEvent.click(screen.getByRole('tab', {name: 'JSON'}));
    expect((await screen.findByText(/"metadata"/)).textContent).toContain('说明 v7');
});
test('Dataset Form and JSON preserve metadata when adopting source updates and publishing a new revision', async () => {
    const fetcher = datasetBackend('/ui/datasets/shared/edit');
    expect((await screen.findByLabelText('元数据值 1') as HTMLTextAreaElement).value).toBe('说明 v1');
    fireEvent.click(screen.getByLabelText('JSON'));
    const definition = JSON.parse((screen.getByLabelText('Dataset JSON 定义') as HTMLTextAreaElement).value);
    expect(definition.metadata).toEqual({description: '说明 v1', '任意/键': '保留\n内容'});
    fireEvent.click(screen.getByLabelText('Form'));
    fireEvent.change(screen.getByLabelText('来源版本 1'), {target: {value: 'branch'}});
    fireEvent.click(screen.getByRole('button', {name: '校验并预览来源'}));
    await screen.findByText(/来源版本和读取权限已校验/);
    fireEvent.click(screen.getByRole('button', {name: '发布这个版本'}));
    await screen.findByText('来源与交付范围');
    const publication = fetcher.mock.calls.find(([path, options]) => path === '/snapshot/v1/datasets' && options?.method === 'POST');
    expect(JSON.parse(String(publication?.[1]?.body))).toMatchObject({revision: 2, metadata: definition.metadata});
});
test('clearing all metadata publishes an explicit empty map', async () => {
    const fetcher = datasetBackend('/ui/datasets/shared/edit');
    await screen.findByLabelText('元数据值 1');
    fireEvent.click(screen.getByRole('button', {name: '删除元数据 2'}));
    fireEvent.click(screen.getByRole('button', {name: '删除元数据 1'}));
    fireEvent.click(screen.getByRole('button', {name: '校验并预览来源'}));
    await screen.findByText(/来源版本和读取权限已校验/);
    fireEvent.click(screen.getByRole('button', {name: '发布这个版本'}));
    await waitFor(() => expect(fetcher.mock.calls.some(([path, options]) => path === '/snapshot/v1/datasets' && options?.method === 'POST')).toBe(true));
    const publication = fetcher.mock.calls.find(([path, options]) => path === '/snapshot/v1/datasets' && options?.method === 'POST');
    expect(JSON.parse(String(publication?.[1]?.body)).metadata).toEqual({});
});
