import React from 'react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import KCApp from './kcApp';
import { session } from '../lib/api/session';
import { mockKC } from '../../test/mockKC';

beforeEach(() => { vi.stubGlobal('localStorage', window.localStorage); });
afterEach(() => { cleanup(); session.logout(); window.localStorage.clear(); window.sessionStorage.clear(); vi.unstubAllGlobals(); });

test('an anonymous Dataset visitor uses the original login with the complete return URL', async () => {
    window.history.replaceState({}, '', '/ui/catalogs/beta/datasets/shared?revision=1#history');
    const server = mockKC(); server.setIdentity();
    vi.stubGlobal('fetch', server.fetcher);
    render(<KCApp />);
    expect(await screen.findByLabelText('Access Key ID')).toBeTruthy();
    expect(window.location.pathname).toBe('/ui/auth/login');
    expect(new URLSearchParams(window.location.search).get('next')).toBe('/catalogs/beta/datasets/shared?revision=1#history');
    expect(screen.queryByText('登录 Dataset 控制台')).toBe(null);
    expect(server.fetcher.mock.calls.some(([path]) => path.startsWith('/catalog/'))).toBe(false);
});

async function loggedIn(path = '/ui/datasets') {
    window.history.replaceState({}, '', path);
    const server = mockKC();
    vi.stubGlobal('fetch', server.fetcher);
    render(<KCApp />);
    return server;
}

async function previewSources(): Promise<void> {
    fireEvent.click(await screen.findByRole('button', { name: '校验并预览来源' }));
    await screen.findByText(/来源版本和读取权限已校验/);
}

test('Catalog selection survives navigation, editing and deep links', async () => {
    const server = await loggedIn();
    await screen.findByRole('option', { name: 'beta' });
    fireEvent.change(await screen.findByLabelText('Catalog'), { target: { value: 'beta' } });
    fireEvent.click(await screen.findByRole('link', { name: 'shared' }));
    await screen.findByText('来源与交付范围');
    expect(window.location.pathname).toBe('/ui/catalogs/beta/datasets/shared');
    fireEvent.click(screen.getByRole('link', { name: '编辑当前版并发布新版' }));
    expect((await screen.findByLabelText('来源仓 1') as HTMLSelectElement).value).toBe('kr://example/source');
    expect((screen.getByLabelText('提交版本 1') as HTMLInputElement).value).toBe('c1');
    fireEvent.change(screen.getByLabelText('交付路径 1'), { target: { value: 'reference' } });
    await previewSources();
    fireEvent.click(screen.getByRole('button', { name: '发布这个版本' }));
    await screen.findByText('来源与交付范围');
    const call = server.fetcher.mock.calls.find(([path, options]) => path === '/catalog/v1/catalogs/beta/datasets' && options?.method === 'POST');
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ dataset: 'shared', revision: 2, sources: [{ repository: 'kr://example/source', selector: 'refs/heads/main', commit: 'c1', path: 'reference', subPath: 'handbook' }] });
    expect(server.fetcher.mock.calls.some(([path]) => path.startsWith('/catalog/v1/catalogs/alpha/'))).toBe(false);
});

test('directory pagination and previews keep the accepted historical pin', async () => {
    const server = await loggedIn('/ui/catalogs/beta/datasets/shared?revision=1');
    fireEvent.click(await screen.findByRole('button', { name: '加载更多文件' }));
    await screen.findByText('📄 second.txt');
    expect(screen.getByText('📄 first.txt')).toBeTruthy();
    fireEvent.click(screen.getByText('📄 second.txt'));
    await screen.findByText('published bytes');
    const reads = server.fetcher.mock.calls.filter(([path]) => path.startsWith('/dataset-files/v1/'));
    expect(reads.length).toBeGreaterThanOrEqual(4);
    for (const [, options] of reads) {
        expect(JSON.parse(String(options?.body)).catalog).toBe('beta');
        expect(JSON.parse(String(options?.body)).pin.revision).toBe(1);
    }
    const pages = reads.filter(([path]) => path.endsWith('tree:list'));
    expect(JSON.parse(String(pages[1][1]?.body)).continuation).toBe('next-page');
    fireEvent.click(screen.getByRole('button', { name: /alice/ }));
    const original = Object.getOwnPropertyDescriptor(window, 'location')!;
    const location = window.location; const navigate = vi.fn();
    Object.defineProperty(window, 'location', {configurable: true, get: () => location, set: navigate});
    try { fireEvent.click(screen.getByRole('button', {name: 'Logout'})); }
    finally { Object.defineProperty(window, 'location', original); }
    expect(navigate).toHaveBeenCalledWith('/logout');
    await waitFor(() => expect(window.location.pathname).toBe('/ui/auth/login'));
    expect(screen.queryByText('published bytes')).toBe(null);
    expect(window.sessionStorage.length).toBe(0);
});

test('partial source rows cannot disappear into a successful publication', async () => {
    const server = await loggedIn('/ui/catalogs/beta/datasets/shared/edit');
    await screen.findByLabelText('来源仓 1');
    fireEvent.click(screen.getByRole('button', { name: '添加来源' }));
    fireEvent.click(screen.getByRole('button', { name: '发布这个版本' }));
    await screen.findByText(/未填完整的条目不会被忽略/);
    expect(server.fetcher.mock.calls.some(([, options]) => options?.method === 'POST' && String(options.body).includes('sources'))).toBe(false);
});

test('publication refusal keeps edits and the selected Catalog', async () => {
    const server = await loggedIn('/ui/catalogs/beta/datasets/shared/edit');
    await screen.findByLabelText('交付路径 1');
    fireEvent.change(screen.getByLabelText('交付路径 1'), { target: { value: 'reference' } });
    await previewSources();
    server.reject('/datasets');
    fireEvent.click(screen.getByRole('button', { name: '发布这个版本' }));
    await screen.findByText(/当前账号无权发布/);
    expect((screen.getByLabelText('交付路径 1') as HTMLInputElement).value).toBe('reference');
    expect(window.location.pathname).toBe('/ui/catalogs/beta/datasets/shared/edit');
    expect(session.get()?.principal).toBe('alice');
});

test('a shared login configuration failure offers retry without another credential form', async () => {
    window.history.replaceState({}, '', '/ui/datasets');
    const server = mockKC();
    vi.stubGlobal('fetch', vi.fn(async (path: string, options?: RequestInit) => path === '/identity/v1/auth'
        ? new Response(JSON.stringify({mode: 'local', localAssertion: true, accepts: ['X-Kc-As']})) : server.fetcher(path, options)));
    render(<KCApp />);
    await screen.findByText('控制台尚未连接共享登录，请联系管理员。');
    expect(screen.getByRole('button', {name: '重新连接'})).toBeTruthy();
    expect(screen.queryByRole('textbox', {name: '用户名'})).toBe(null);
    await waitFor(() => expect(session.get()).toBe(null));
    expect(window.sessionStorage.length).toBe(0);
    expect(server.fetcher.mock.calls.some(([path]) => path.startsWith('/catalog/'))).toBe(false);
});

test.each([403, 503])('identity HTTP %i stays on Dataset and can retry without a login loop', async status => {
    window.history.replaceState({}, '', '/ui/datasets');
    const server = mockKC(); let refused = true;
    const fetcher = vi.fn(async (path: string, options?: RequestInit) => path === '/identity/v1/whoami' && refused
        ? new Response(JSON.stringify({error: {code: status === 403 ? 'FORBIDDEN' : 'TEMPORARY_UNAVAILABLE', message: '共享登录暂不可用'}}), {status})
        : server.fetcher(path, options));
    vi.stubGlobal('fetch', fetcher); render(<KCApp/>);
    await screen.findByText(/共享登录暂不可用/);
    expect(window.location.pathname).toBe('/ui/datasets');
    expect(screen.queryByLabelText('Access Key ID')).toBe(null);
    expect(fetcher.mock.calls.some(([path]) => path.startsWith('/catalog/'))).toBe(false);
    refused = false; fireEvent.click(screen.getByRole('button', {name: '重新连接'}));
    await screen.findByRole('option', {name: 'beta'});
    expect(fetcher.mock.calls.filter(([path]) => path === '/identity/v1/whoami')).toHaveLength(2);
});

test('re-entering Dataset replaces the old native profile and inventory after an identity switch', async () => {
    window.history.replaceState({}, '', '/ui/datasets');
    const server = mockKC(); let principal = 'alice';
    vi.stubGlobal('fetch', vi.fn(async (path: string, options?: RequestInit) => path === '/catalog/v1/catalogs'
        ? new Response(JSON.stringify({catalogs: [{id: `${principal}-private`}]})) : server.fetcher(path, options)));
    render(<KCApp/>);
    await screen.findByRole('option', {name: 'alice-private'});
    fireEvent.click(screen.getByRole('link', {name: 'Repositories'}));
    await screen.findByRole('button', {name: 'Create Repository'});
    principal = 'bob'; server.setIdentity('bob');
    fireEvent.click(screen.getByRole('link', {name: 'Datasets'}));
    await screen.findByRole('option', {name: 'bob-private'});
    await screen.findByRole('button', {name: /bob/});
    expect(screen.queryByRole('option', {name: 'alice-private'})).toBe(null);
    expect(screen.queryByRole('button', {name: /alice/})).toBe(null);
    expect(session.get()?.principal).toBe('bob');
});

test('a late cookie proof cannot restore Dataset after the user leaves', async () => {
    window.history.replaceState({}, '', '/ui/datasets');
    const server = mockKC(); let complete!: (value: Response) => void;
    vi.stubGlobal('fetch', vi.fn(async (path: string, options?: RequestInit) => path === '/identity/v1/whoami'
        ? new Promise<Response>(resolve => {complete = resolve;}) : server.fetcher(path, options)));
    render(<KCApp/>);
    await waitFor(() => expect(complete).toBeDefined());
    fireEvent.click(screen.getByRole('link', {name: 'Repositories'}));
    await screen.findByRole('button', {name: 'Create Repository'});
    await act(async () => {complete(new Response(JSON.stringify({principal: 'alice'})));});
    expect(window.location.pathname).toBe('/ui/repositories');
    expect(session.get()).toBe(null);
    expect(screen.queryByLabelText('Catalog')).toBe(null);
});

test.each(['publish', 'retire'])('late %s responses cannot reopen a page the user has left', async (operation) => {
    const server = await loggedIn(`/ui/catalogs/beta/datasets/shared${operation === 'publish' ? '/edit' : ''}`);
    let complete: (value: Response) => void = () => { throw new Error('Request has not started'); };
    const fetcher = vi.fn(async (path: string, options?: RequestInit) => {
        if (options?.method === 'POST' && path === (operation === 'publish' ? '/catalog/v1/catalogs/beta/datasets' : '/catalog/v1/catalogs/beta/datasets/shared/retire')) {
            return new Promise<Response>((resolve) => { complete = resolve; });
        }
        return server.fetcher(path, options);
    });
    vi.stubGlobal('fetch', fetcher);
    if (operation === 'publish') {
        await previewSources();
        fireEvent.click(await screen.findByRole('button', { name: '发布这个版本' }));
    }
    else {
        fireEvent.click(await screen.findByRole('button', { name: '退役 Dataset' }));
        fireEvent.click(screen.getByRole('button', { name: '确认退役' }));
    }
    await waitFor(() => expect(fetcher.mock.calls.some(([path, options]) => options?.method === 'POST' &&
        path === (operation === 'publish' ? '/catalog/v1/catalogs/beta/datasets' : '/catalog/v1/catalogs/beta/datasets/shared/retire'))).toBe(true));
    fireEvent.click(screen.getByRole('link', { name: 'Catalog beta' }));
    await screen.findByRole('link', { name: 'shared' });
    await act(async () => { complete(new Response('{}', { status: 200 })); });
    expect(window.location.pathname).toBe('/ui/catalogs/beta');
    expect(screen.queryByText('来源与交付范围')).toBe(null);
});

test('a legal Dataset named new does not collide with the creation page', async () => {
    const server = await loggedIn('/ui/catalogs/beta/datasets/new');
    vi.stubGlobal('fetch', vi.fn(async (path: string, options?: RequestInit) => path === '/catalog/v1/catalogs/beta/datasets/new'
        ? new Response(JSON.stringify({ id: 'new', revision: 1, servingRevision: 1, versions: [1], items: [], sources: [], repositories: [], itemCount: 0, retired: true }))
        : server.fetcher(path, options)));
    expect(await screen.findByRole('heading', { name: 'new v1 已退役' })).toBeTruthy();
    expect(screen.queryByLabelText('Dataset 名称')).toBe(null);
});

test('a failed file preview retries the file request and keeps the directory intact', async () => {
    const server = await loggedIn('/ui/catalogs/beta/datasets/shared');
    let refused = true;
    const fetcher = vi.fn(async (path: string, options?: RequestInit) => {
        if (path.endsWith('/file:read') && refused) return new Response(JSON.stringify({ error: { code: 'TEMPORARY_UNAVAILABLE', message: '来源暂不可用' } }), { status: 503 });
        return server.fetcher(path, options);
    });
    vi.stubGlobal('fetch', fetcher);
    fireEvent.click(await screen.findByText('📄 first.txt'));
    await screen.findByText(/来源暂不可用/);
    refused = false;
    fireEvent.click(screen.getByRole('button', { name: '重试读取' }));
    expect(await screen.findByText('published bytes')).toBeTruthy();
    expect(fetcher.mock.calls.filter(([path]) => path.endsWith('/file:read'))).toHaveLength(2);
    expect(screen.getByText('📄 first.txt')).toBeTruthy();
});

test('a fresh Dataset can start in JSON and publish exactly the validated fixed sources', async () => {
    const server = await loggedIn('/ui/catalogs/beta/create-dataset');
    await screen.findByLabelText('Dataset 名称');
    fireEvent.click(screen.getByLabelText('JSON'));
    const definition = {dataset: 'shared', revision: 2, sources: [{repository: 'kr://example/source', selector: 'refs/heads/main', path: 'reference', subPath: 'handbook'}]};
    fireEvent.change(screen.getByLabelText('Dataset JSON 定义'), {target: {value: JSON.stringify(definition)}});
    fireEvent.click(screen.getByRole('button', {name: '校验并预览来源'}));
    await screen.findByText(/来源版本和读取权限已校验/);
    fireEvent.click(screen.getByRole('button', {name: '发布这个版本'}));
    await screen.findByText('来源与交付范围');
    const publication = server.fetcher.mock.calls.find(([path, options]) => path === '/catalog/v1/catalogs/beta/datasets' && options?.method === 'POST');
    expect(JSON.parse(String(publication?.[1]?.body))).toEqual({...definition, sources: [{...definition.sources[0], commit: 'c1'}]});
});

test('changing JSON requires a new source confirmation before publication', async () => {
    const server = await loggedIn('/ui/catalogs/beta/create-dataset');
    await screen.findByLabelText('Dataset 名称');
    fireEvent.click(screen.getByLabelText('JSON'));
    const definition = {dataset: 'shared', revision: 2, sources: [{repository: 'kr://example/source', selector: 'refs/heads/main', path: 'reference'}]};
    fireEvent.change(screen.getByLabelText('Dataset JSON 定义'), {target: {value: JSON.stringify(definition)}});
    fireEvent.click(screen.getByRole('button', {name: '校验并预览来源'}));
    await screen.findByText(/来源版本和读取权限已校验/);
    const changed = {...definition, sources: [{...definition.sources[0], path: 'changed'}]};
    fireEvent.change(screen.getByLabelText('Dataset JSON 定义'), {target: {value: JSON.stringify(changed)}});
    expect(screen.queryByText(/来源版本和读取权限已校验/)).toBe(null);
    fireEvent.click(screen.getByRole('button', {name: '发布这个版本'}));
    await screen.findByText('请先校验并预览当前来源，再发布这个版本。');
    expect(server.fetcher.mock.calls.some(([path, options]) => path === '/catalog/v1/catalogs/beta/datasets' && options?.method === 'POST')).toBe(false);
    await previewSources();
    fireEvent.click(screen.getByRole('button', {name: '发布这个版本'}));
    await screen.findByText('来源与交付范围');
    const publication = server.fetcher.mock.calls.find(([path, options]) => path === '/catalog/v1/catalogs/beta/datasets' && options?.method === 'POST');
    expect(JSON.parse(String(publication?.[1]?.body))).toEqual({...changed, sources: [{...changed.sources[0], commit: 'c1'}]});
});

test('unconfirmed form sources cannot be published', async () => {
    const server = await loggedIn('/ui/catalogs/beta/datasets/shared/edit');
    fireEvent.click(await screen.findByRole('button', {name: '发布这个版本'}));
    await screen.findByText('请先校验并预览当前来源，再发布这个版本。');
    expect(server.fetcher.mock.calls.some(([path, options]) => path === '/catalog/v1/catalogs/beta/datasets' && options?.method === 'POST')).toBe(false);
});

test('JSON to Form rejects unknown fields and preserves supported file definitions', async () => {
    const server = await loggedIn('/ui/catalogs/beta/create-dataset');
    await screen.findByLabelText('Dataset 名称');
    fireEvent.click(screen.getByLabelText('JSON'));
    const value = {dataset: 'docs', revision: 2, sources: [{repository: 'kr://example/source', commit: 'c1', selector: 'refs/heads/main', file: 'README.md', target: 'guide.md'}]};
    const unsupported = JSON.stringify({...value, metadata: {owner: 'team'}});
    fireEvent.change(screen.getByLabelText('Dataset JSON 定义'), {target: {value: unsupported}});
    fireEvent.click(screen.getByLabelText('Form'));
    await screen.findByText(/不支持字段 metadata/);
    expect((screen.getByLabelText('Dataset JSON 定义') as HTMLTextAreaElement).value).toBe(unsupported);
    expect(screen.queryByLabelText('Dataset 名称')).toBe(null);
    fireEvent.change(screen.getByLabelText('Dataset JSON 定义'), {target: {value: JSON.stringify(value)}});
    fireEvent.click(screen.getByLabelText('Form'));
    expect((screen.getByLabelText('Dataset 名称') as HTMLInputElement).value).toBe('docs');
    expect((screen.getByLabelText('选择范围 1') as HTMLSelectElement).value).toBe('file');
    expect((screen.getByLabelText('来源路径 1') as HTMLInputElement).value).toBe('README.md');
    expect((screen.getByLabelText('交付路径 1') as HTMLInputElement).value).toBe('guide.md');
    fireEvent.click(screen.getByLabelText('JSON'));
    expect(JSON.parse((screen.getByLabelText('Dataset JSON 定义') as HTMLTextAreaElement).value)).toEqual(value);
    expect(server.fetcher.mock.calls.some(([path, options]) => path === '/catalog/v1/catalogs/beta/datasets' && options?.method === 'POST')).toBe(false);
});

test('History loads bounded sparse pages on demand and Compare keeps a deep-linked revision', async () => {
    window.history.replaceState({}, '', '/ui/catalogs/beta/datasets/shared?revision=1');
    const server = mockKC();
    const fetcher = vi.fn(async (input: string, options?: RequestInit) => {
        const path = new URL(input, window.location.origin);
        if (path.pathname !== '/catalog/v1/catalogs/beta/datasets/shared') return server.fetcher(input, options);
        const revision = Number(path.searchParams.get('revision')) || 10;
        return new Response(JSON.stringify({id: 'shared', revision, servingRevision: 10, versions: path.searchParams.has('continuation') ? [4, 7] : [9, 10],
            continuation: path.searchParams.has('continuation') ? undefined : 'older-page', repositories: ['kr://example/source'], itemCount: 1,
            sources: [], items: [{target: 'docs', repository: 'kr://example/source', commit: `c${revision}`, kind: 'prefix', prefix: 'handbook'}]}));
    });
    vi.stubGlobal('fetch', fetcher);
    render(<KCApp/>);
    await screen.findByText('来源与交付范围');
    expect((screen.getByLabelText('已发布版本') as HTMLSelectElement).value).toBe('1');
    expect(fetcher.mock.calls.filter(([path]) => path.includes('continuation='))).toHaveLength(0);
    fireEvent.click(screen.getByRole('tab', {name: 'History'}));
    fireEvent.click(screen.getByRole('button', {name: '加载更多历史版本'}));
    await screen.findByRole('button', {name: '打开 v4'});
    expect(fetcher.mock.calls.filter(([path]) => path.includes('continuation='))).toEqual([
        ['/catalog/v1/catalogs/beta/datasets/shared?revision=1&continuation=older-page', expect.any(Object)],
    ]);
    expect(screen.queryByRole('button', {name: '加载更多历史版本'})).toBe(null);
    fireEvent.click(screen.getByRole('tab', {name: 'Compare'}));
    await screen.findByText('修改');
    expect((screen.getByLabelText('比较前版本') as HTMLSelectElement).value).toBe('4');
    expect((screen.getByLabelText('比较后版本') as HTMLSelectElement).value).toBe('1');
    expect(screen.getByLabelText('比较前版本').querySelectorAll('option')).toHaveLength(5);
    expect(screen.getByText(/"commit": "c4"/)).toBeTruthy();
    expect(screen.getByText(/"commit": "c1"/)).toBeTruthy();
});
