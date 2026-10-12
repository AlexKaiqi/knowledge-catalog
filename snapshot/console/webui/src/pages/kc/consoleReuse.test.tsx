import React from 'react';
import {afterEach, expect, test, vi} from 'vitest';
import {cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';
import KCApp from '../../extendable/kcApp';
import {session} from '../../lib/api/session';
import {mockKC} from '../../../test/mockKC';

afterEach(() => {cleanup(); session.logout(); window.localStorage.clear(); window.sessionStorage.clear(); vi.unstubAllGlobals();});
const json = (value: unknown, status = 200): Response => new Response(JSON.stringify(value), {status});

test('Dataset landing uses the service inventory and creates without Catalog selection', async () => {
    window.history.replaceState({}, '', '/ui/datasets');
    const backend = mockKC();
    vi.stubGlobal('fetch', backend.fetcher);
    render(<KCApp/>);
    const link = await screen.findByRole('link', {name: 'shared'});
    expect(link.getAttribute('href')).toBe('/ui/datasets/shared');
    expect(screen.queryByLabelText('Catalog')).toBe(null);
    expect(backend.fetcher.mock.calls.some(([path]) => path.startsWith('/catalog/'))).toBe(false);
    fireEvent.change(screen.getByLabelText('搜索 Dataset'), {target: {value: 'missing'}});
    expect(screen.queryByRole('link', {name: 'shared'})).toBe(null);
    fireEvent.click(screen.getByRole('button', {name: '创建 Dataset'}));
    await screen.findByLabelText('Dataset 名称');
    expect(window.location.pathname).toBe('/ui/datasets/actions/create');
});

test('an inventory refusal is not reported as an empty successful inventory', async () => {
    window.history.replaceState({}, '', '/ui/datasets');
    const backend = mockKC();
    vi.stubGlobal('fetch', vi.fn(async (path: string, options?: RequestInit) => path === '/snapshot/v1/datasets'
        ? json({error: {code: 'FORBIDDEN', message: '库存访问已撤权'}}, 403) : backend.fetcher(path, options)));
    render(<KCApp/>);
    await screen.findByText(/库存访问已撤权/);
    expect(screen.queryByText('当前没有 Dataset。')).toBe(null);
    expect(screen.getByRole('button', {name: '重试'})).toBeTruthy();
});

test.each([undefined, 'native-files'])('Compare respects explicit native mapping %s and never borrows Dataset access', async native => {
    window.history.replaceState({}, '', '/ui/datasets/shared');
    const backend = mockKC();
    const fetcher = vi.fn(async (input: string, options?: RequestInit) => {
        const path = new URL(input, window.location.origin);
        if (path.pathname === '/snapshot/v1/repositories') return json({repositories: [{id: 'kr://example/source', consoleRepository: native}]});
        if (path.pathname === '/snapshot/v1/datasets/shared') {
            const revision = Number(path.searchParams.get('revision')) || 2;
            return json({id: 'shared', revision, servingRevision: 2, versions: [1, 2], itemCount: 1, repositories: ['kr://example/source'],
                sources: [{repository: 'kr://example/source', commit: `fixed-${revision}`, file: 'data.txt', target: revision === 1 ? 'old-name.txt' : 'new-name.txt'}],
                items: [{repository: 'kr://example/source', commit: `fixed-${revision}`, kind: 'file', file: 'data.txt', target: revision === 1 ? 'old-name.txt' : 'new-name.txt'}]});
        }
        return backend.fetcher(input, options);
    });
    vi.stubGlobal('fetch', fetcher);
    render(<KCApp/>);
    fireEvent.click(await screen.findByRole('tab', {name: 'Compare'}));
    const diff = await screen.findByRole('region', {name: '发布定义差异'});
    await waitFor(() => expect(diff.textContent).toContain('old-name.txt'));
    expect(diff.textContent).toContain('new-name.txt');
    if (native) {
        const link = screen.getByRole('link', {name: /比较来源文件/});
        expect(link.getAttribute('href')).toBe('/ui/repositories/native-files/compare?ref=fixed-1&compare=fixed-2');
        expect(link.getAttribute('href')).not.toContain('kr%3A');
    } else {
        expect(screen.queryByRole('link', {name: /比较来源文件/})).toBe(null);
        expect(screen.getByText('此来源未接入当前 Repository 控制台')).toBeTruthy();
    }
    expect(fetcher.mock.calls.some(([path]) => path.startsWith('/api/v1/repositories/'))).toBe(false);
});

test('a Dataset named create keeps its detail route without opening the creation form', async () => {
    window.history.replaceState({}, '', '/ui/datasets');
    const backend = mockKC();
    const detail = {id: 'create', revision: 1, servingRevision: 1, versions: [1], repositories: [], itemCount: 0, sources: [], items: []};
    vi.stubGlobal('fetch', vi.fn(async (input: string, options?: RequestInit) => {
        const path = new URL(input, window.location.origin).pathname;
        if (path === '/snapshot/v1/datasets') return json({datasets: [detail]});
        if (path === '/snapshot/v1/datasets/create') return json(detail);
        if (path === '/snapshot/v1/files/mounts:list') return json({pin: {setId: 'create', revision: 1, repositories: {}}, mounts: []});
        return backend.fetcher(input, options);
    }));
    render(<KCApp/>);
    fireEvent.click(await screen.findByRole('link', {name: 'create'}));
    await screen.findByText('来源与交付范围');
    expect(window.location.pathname).toBe('/ui/datasets/create');
    expect(screen.queryByLabelText('Dataset 名称')).toBe(null);
});
