import React from 'react';
import {afterEach, expect, test, vi} from 'vitest';
import {act, cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';
import KCApp from '../../extendable/kcApp';
import {ResolvedPin} from '../../lib/api/kc';
import {session} from '../../lib/api/session';
import {mockKC} from '../../../test/mockKC';

afterEach(() => {cleanup(); session.logout(); window.localStorage.clear(); window.sessionStorage.clear(); vi.unstubAllGlobals();});
const json = (value: unknown, status = 200): Response => new Response(JSON.stringify(value), {status});
const fault = (message: string, status = 403): Response => json({error: {code: status === 403 ? 'FORBIDDEN' : 'TEMPORARY_UNAVAILABLE', message}}, status);
const accepted = (revision: number): ResolvedPin => ({setId: 'shared', revision, ref: `v${revision}`, repositories: {'kr://example/source': `accepted-${revision}`},
    items: [{target: 'docs', repository: 'kr://example/source', commit: `accepted-${revision}`, kind: 'prefix', prefix: 'handbook'}], pinId: `accepted-${revision}`});
const detail = (revision: number, retired = false) => ({id: 'shared', revision, servingRevision: 3, versions: [1, 3], retired,
    repositories: ['kr://example/source'], itemCount: 1, sources: [], items: accepted(revision).items, metadata: {description: `release-${revision}`}});
function backend(intercept?: (path: URL, options?: RequestInit) => Response | Promise<Response> | undefined) {
    const existing = mockKC();
    const fetcher = vi.fn(async (input: string, options?: RequestInit) => {
        const path = new URL(input, window.location.origin);
        const response = intercept?.(path, options);
        if (response !== undefined) return response;
        if (path.pathname === '/snapshot/v1/datasets/shared/resolve') {
            const body = JSON.parse(String(options?.body));
            return json(accepted(body.revision ?? 3));
        }
        if (path.pathname === '/snapshot/v1/datasets/shared') return fault('无权查看管理记录');
        if (path.pathname === '/snapshot/v1/files/mounts:list') return json({pin: JSON.parse(String(options?.body)).pin, mounts: []});
        return existing.fetcher(input, options);
    });
    vi.stubGlobal('fetch', fetcher);
    return fetcher;
}
function open(path = '/ui/datasets/shared'): void {
    window.history.replaceState({}, '', path);
    render(<KCApp/>);
}

test.each([undefined, 1])('a consumer can read revision %s when management inventory is forbidden', async revision => {
    const fetcher = backend();
    open(`/ui/datasets/shared${revision === undefined ? '' : `?revision=${revision}`}`);
    fireEvent.click(await screen.findByRole('button', {name: 'first.txt'}));
    await screen.findByText('published bytes');
    const resolve = fetcher.mock.calls.find(([path]) => path.endsWith('/shared/resolve'));
    expect(JSON.parse(String(resolve?.[1]?.body))).toEqual(revision === undefined ? {} : {revision});
    for (const [, options] of fetcher.mock.calls.filter(([path]) => path.startsWith('/snapshot/v1/files/'))) {
        expect(JSON.parse(String(options?.body)).pin).toEqual(accepted(revision ?? 3));
    }
    expect(screen.queryByText(/自定义元数据/)).toBe(null);
    expect(screen.getByRole('tab', {name: 'History'}).getAttribute('aria-disabled')).toBe('true');
    expect(screen.queryByRole('button', {name: '退役 Dataset'})).toBe(null);
});

test('a slow current management read does not delay delivery or replace the release selected by resolve', async () => {
    let complete: (value: Response) => void = () => {throw new Error('Management request has not started');};
    const fetcher = backend(path => path.pathname === '/snapshot/v1/datasets/shared' ?
        path.searchParams.has('revision') ? json(detail(Number(path.searchParams.get('revision')))) : new Promise<Response>(resolve => {complete = resolve;}) : undefined);
    open();
    await screen.findByRole('button', {name: 'first.txt'});
    expect(fetcher.mock.calls.some(([path]) => path === '/snapshot/v1/datasets/shared')).toBe(true);
    await act(async () => {complete(json(detail(4)));});
    expect(await screen.findByText('release-3')).toBeTruthy();
    expect(screen.queryByText('release-4')).toBe(null);
    expect(fetcher.mock.calls.filter(([path]) => path.startsWith('/snapshot/v1/datasets/shared?'))).toEqual([
        ['/snapshot/v1/datasets/shared?revision=3', expect.any(Object)],
    ]);
    fireEvent.click(screen.getByRole('tab', {name: 'History'}));
    expect(await screen.findByRole('button', {name: '打开 v1'})).toBeTruthy();
});

test('a slow resolve does not delay separately authorized management records', async () => {
    let complete: (value: Response) => void = () => {throw new Error('Resolve request has not started');};
    const fetcher = backend(path => path.pathname.endsWith('/shared/resolve') ? new Promise<Response>(resolve => {complete = resolve;}) :
        path.pathname === '/snapshot/v1/datasets/shared' ? json(detail(1)) : undefined);
    open('/ui/datasets/shared?revision=1');
    expect(await screen.findByText('release-1')).toBeTruthy();
    fireEvent.click(screen.getByRole('tab', {name: 'JSON'}));
    expect((await screen.findByText(/"metadata"/)).textContent).toContain('release-1');
    await act(async () => {complete(fault('文件消费已撤权'));});
    expect(fetcher.mock.calls.some(([path]) => path.startsWith('/snapshot/v1/files/'))).toBe(false);
});

test('an earlier current management response is replaced when resolve accepts a different release', async () => {
    let complete: (value: Response) => void = () => {throw new Error('Resolve request has not started');};
    backend(path => path.pathname.endsWith('/shared/resolve') ? new Promise<Response>(resolve => {complete = resolve;}) :
        path.pathname === '/snapshot/v1/datasets/shared' ? json(detail(Number(path.searchParams.get('revision')) || 4)) : undefined);
    open();
    await screen.findByText('release-4');
    await act(async () => {complete(json(accepted(3)));});
    await screen.findByText('release-3');
    expect(screen.queryByText('release-4')).toBe(null);
    expect(await screen.findByRole('button', {name: 'first.txt'})).toBeTruthy();
});

test('an inconsistent selected management response cannot cause unbounded reloads or block delivery', async () => {
    let reads = 0;
    const fetcher = backend(path => path.pathname === '/snapshot/v1/datasets/shared' ? ++reads > 3 ? fault('管理请求超过回归上界', 503) : json(detail(4)) : undefined);
    open();
    await screen.findByRole('button', {name: 'first.txt'});
    await screen.findByText(/管理详情返回了不同的发布版本/);
    expect(screen.queryByText('release-4')).toBe(null);
    expect(fetcher.mock.calls.filter(([path]) => new URL(path, window.location.origin).pathname === '/snapshot/v1/datasets/shared')).toHaveLength(2);
});

test('management records remain available when consumption is refused', async () => {
    backend(path => path.pathname.endsWith('/shared/resolve') ? fault('文件消费已撤权') :
        path.pathname === '/snapshot/v1/datasets/shared' ? json(detail(1)) : undefined);
    open('/ui/datasets/shared?revision=1');
    expect(await screen.findByText('release-1')).toBeTruthy();
    expect(await screen.findByText(/文件消费已撤权/)).toBeTruthy();
    fireEvent.click(screen.getByRole('tab', {name: 'JSON'}));
    expect((await screen.findByText(/"metadata"/)).textContent).toContain('release-1');
});

test('retired management records never expose a delivery tree after resolve refuses consumption', async () => {
    const fetcher = backend(path => path.pathname.endsWith('/shared/resolve') ? fault('Dataset 已退役') :
        path.pathname === '/snapshot/v1/datasets/shared' ? json(detail(1, true)) : undefined);
    open('/ui/datasets/shared?revision=1');
    await screen.findByRole('heading', {name: 'shared v1 已退役'});
    expect(screen.queryByRole('button', {name: 'first.txt'})).toBe(null);
    expect(fetcher.mock.calls.some(([path]) => path.startsWith('/snapshot/v1/files/'))).toBe(false);
});

test('a resolve failure can retry delivery without gaining management access', async () => {
    let unavailable = true;
    const fetcher = backend(path => path.pathname.endsWith('/shared/resolve') && unavailable ? fault('来源暂不可用', 503) : undefined);
    open('/ui/datasets/shared?revision=1');
    await screen.findByText(/来源暂不可用/);
    unavailable = false;
    fireEvent.click(screen.getByRole('button', {name: '重试打开交付范围'}));
    await screen.findByRole('button', {name: 'first.txt'});
    expect(fetcher.mock.calls.filter(([path]) => path.endsWith('/shared/resolve'))).toHaveLength(2);
});

test('leaving a Dataset aborts a pending resolve and ignores its late accepted pin', async () => {
    let complete: (value: Response) => void = () => {throw new Error('Resolve request has not started');};
    const fetcher = backend(path => path.pathname.endsWith('/shared/resolve') ? new Promise<Response>(resolve => {complete = resolve;}) : undefined);
    open('/ui/datasets/shared?revision=1');
    await waitFor(() => expect(fetcher.mock.calls.some(([path]) => path.endsWith('/shared/resolve'))).toBe(true));
    const signal = fetcher.mock.calls.find(([path]) => path.endsWith('/shared/resolve'))?.[1]?.signal;
    fireEvent.click(screen.getAllByRole('link', {name: 'Datasets'})[0]);
    await screen.findByRole('link', {name: 'shared'});
    expect(signal?.aborted).toBe(true);
    await act(async () => {complete(json(accepted(1)));});
    expect(fetcher.mock.calls.some(([path]) => path.startsWith('/snapshot/v1/files/'))).toBe(false);
    expect(screen.queryByText(/交付目录/)).toBe(null);
});

test.each(['resolve', 'management', 'mounts'])('switching from history to current aborts and discards late %s data', async stage => {
    let complete: (value: Response) => void = () => {throw new Error('Deferred request has not started');};
    let oldSignal: AbortSignal | null | undefined;
    const fetcher = backend((path, options) => {
        const body = options?.body ? JSON.parse(String(options.body)) : undefined;
        const held = stage === 'resolve' && path.pathname.endsWith('/shared/resolve') && body?.revision === 1 ||
            stage === 'management' && path.pathname === '/snapshot/v1/datasets/shared' && path.searchParams.get('revision') === '1' ||
            stage === 'mounts' && path.pathname.endsWith('/mounts:list') && body?.pin?.revision === 1;
        if (held) {oldSignal = options?.signal; return new Promise<Response>(resolve => {complete = resolve;});}
        if (path.pathname === '/snapshot/v1/datasets/shared') return json(detail(Number(path.searchParams.get('revision')) || 3));
        return undefined;
    });
    open('/ui/datasets/shared?revision=1');
    await waitFor(() => expect(oldSignal).toBeTruthy());
    fireEvent.click(screen.getByRole('button', {name: '打开当前版本'}));
    await screen.findByRole('heading', {name: 'shared v3'});
    await screen.findByText('release-3');
    expect(oldSignal?.aborted).toBe(true);
    await act(async () => {complete(json(stage === 'resolve' ? accepted(1) : stage === 'management' ? detail(1) : {pin: accepted(1), mounts: []}));});
    expect(screen.queryByRole('heading', {name: 'shared v1'})).toBe(null);
    expect(screen.queryByText('release-1')).toBe(null);
    fireEvent.click(await screen.findByRole('button', {name: 'first.txt'}));
    await screen.findByText('published bytes');
    const read = fetcher.mock.calls.find(([path]) => path.endsWith('/file:read'));
    expect(JSON.parse(String(read?.[1]?.body)).pin).toEqual(accepted(3));
});
