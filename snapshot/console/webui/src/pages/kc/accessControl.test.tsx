import React from 'react';
import {afterEach, expect, test, vi} from 'vitest';
import {act, cleanup, fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import KCApp from '../../extendable/kcApp';
import {session} from '../../lib/api/session';
import {AllowRule} from '../../lib/api/kc';
import {mockKC} from '../../../test/mockKC';

const rule = {id: 'own/grant', principal: 'alice', actions: ['file.read', 'dataset.manage'], catalog: 'kr://example/catalog', ref: 'refs/heads/main', object: 'schema/*', aspect: 'body', dataset: 'shared', sharedBy: 'maintainer'};
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), {status, headers: {'Content-Type': 'application/json'}});
afterEach(() => {cleanup(); session.logout(); localStorage.clear(); vi.restoreAllMocks(); vi.unstubAllGlobals();});

function fixture(path = '/ui/auth/permissions', requestURL = 'https://access.example/request') {
    localStorage.clear(); window.history.replaceState({}, '', path);
    const native = mockKC(); let failWrites = false, denied = false, readDenied = false, denyAfterRemoval = false, actor = 'alice';
    let rules: AllowRule[] = [rule];
    const fetcher = vi.fn(async (input: RequestInfo | URL, options?: RequestInit) => {
        const target = String(input);
        if (target === '/api/v1/setup_lakefs') return json({state: 'initialized', login_config: {RBAC: 'none', login_cookie_names: [], logout_url: '/logout'}});
        if (target.startsWith('/api/v1/auth/')) return json({message: 'Native access control is unavailable'}, 403);
        if (target === '/identity/v1/admission') return json({principal: actor, grants: rules.map(item => ({id: item.id, actions: item.actions, repository: item.repo, catalog: item.catalog, ref: item.ref, object: item.object, aspect: item.aspect, dataset: item.dataset, sharedBy: item.sharedBy})), request: {url: requestURL, administrators: ['admin-a']}});
        if (target.startsWith('/admin/v1/grants')) {
            if (denied) return json({error: {code: 'FORBIDDEN', message: 'Not a KC grant administrator'}}, 403);
            if (readDenied && options?.method === 'GET') return json({error: {code: 'FORBIDDEN', message: 'No read permission in this scope'}}, 403);
            if (options?.method === 'POST') {
                if (failWrites) return json({error: {code: 'TEMPORARY_UNAVAILABLE', message: 'Try again later'}}, 503);
                const body = JSON.parse(String(options.body));
                const added = {id: 'added', ...body, repo: body.repository}; rules = [...rules, added];
                return json(added);
            }
            if (options?.method === 'DELETE') {rules = []; denied = denyAfterRemoval; return json({revoked: rule.id});}
            return json({rules});
        }
        return native.fetcher(target, options);
    });
    vi.stubGlobal('fetch', fetcher);
    return {fetcher, native, failWrites: (failed: boolean) => {failWrites = failed;}, deny: () => {denied = true;}, denyRead: (failed: boolean) => {readDenied = failed;}, denyAfterRemoval: () => {denyAfterRemoval = true;}, setIdentity: (principal: string) => {actor = principal; native.setIdentity(principal);}, setRules: (value: AllowRule[]) => {rules = value;}};
}

test('Administration opens KC permissions with every explicit constraint even when native RBAC is unavailable', async () => {
    const server = fixture('/ui/auth'); render(<KCApp/>);
    await screen.findByRole('heading', {name: '我的权限'});
    expect(window.location.pathname).toBe('/ui/auth/permissions');
    const table = await screen.findByRole('table', {name: '显式授权规则'});
    for (const value of [rule.id, 'alice', ...rule.actions, rule.catalog, rule.ref, rule.object, rule.aspect, rule.dataset, rule.sharedBy]) expect(within(table).getByText(value, {exact: false})).toBeTruthy();
    expect(screen.getByText(/显式授权列表不是有效权限求值/)).toBeTruthy();
    expect(screen.getByRole('link', {name: '申请权限'}).getAttribute('href')).toBe('https://access.example/request');
    expect(await screen.findByText('admin-a')).toBeTruthy();
    expect(screen.getAllByRole('link', {name: 'Repositories'})).toHaveLength(1);
    expect(screen.getByRole('link', {name: '我的权限'}).getAttribute('href')).toBe('/ui/auth/permissions');
    expect(screen.getByText('My Credentials').closest('.nav-link')?.classList.contains('active')).toBe(false);
    expect(server.fetcher.mock.calls.some(([path]) => String(path) === '/admin/v1/grants')).toBe(false);
});

test.each(['javascript:alert(1)', 'data:text/html,<script>bad()</script>', '//untrusted.example/request'])('an unsafe external application URL %s is not linked', async url => {
    fixture('/ui/auth/permissions', url); render(<KCApp/>);
    await screen.findByRole('heading', {name: '我的权限'});
    expect(screen.queryByRole('link', {name: '申请权限'})).toBe(null);
    expect(await screen.findByText('admin-a')).toBeTruthy();
});

test('grant filters only the loaded rules and never asks the server to evaluate a query', async () => {
    const server = fixture('/ui/auth/grants'); render(<KCApp/>);
    await screen.findByRole('table', {name: '授权规则'});
    const count = server.fetcher.mock.calls.length;
    fireEvent.change(screen.getByLabelText('筛选已加载规则'), {target: {value: 'not-present'}});
    expect(screen.queryByRole('button', {name: `撤销 ${rule.id}`})).toBe(null);
    expect(server.fetcher.mock.calls.length).toBe(count);
    expect(server.fetcher.mock.calls.filter(([path]) => String(path).startsWith('/admin/v1/grants')).map(([path]) => path)).toEqual(['/admin/v1/grants']);
});

test('adding a grant retains failed input and sends the exact existing request with multiple actions and constraints', async () => {
    const server = fixture('/ui/auth/grants'); server.failWrites(true); render(<KCApp/>);
    fireEvent.change(await screen.findByLabelText('授权主体'), {target: {value: 'bob'}});
    fireEvent.change(screen.getByLabelText('允许动作'), {target: {value: 'file.read, dataset.manage'}});
    fireEvent.change(screen.getByLabelText('范围类型'), {target: {value: 'catalog'}});
    fireEvent.change(screen.getByLabelText('Catalog 范围'), {target: {value: rule.catalog}});
    fireEvent.click(screen.getByText('高级约束'));
    for (const [label, value] of [['Ref', rule.ref], ['Object', rule.object], ['Aspect', rule.aspect], ['Dataset', rule.dataset]]) fireEvent.change(screen.getByLabelText(label), {target: {value}});
    fireEvent.click(screen.getByRole('button', {name: '添加授权'}));
    await screen.findByText('TEMPORARY_UNAVAILABLE ·', {exact: false});
    expect((screen.getByLabelText('授权主体') as HTMLInputElement).value).toBe('bob');
    expect((screen.getByLabelText('Catalog 范围') as HTMLInputElement).value).toBe(rule.catalog);
    server.failWrites(false);
    fireEvent.click(screen.getByRole('button', {name: '添加授权'}));
    await screen.findByRole('button', {name: '撤销 added'});
    const writes = server.fetcher.mock.calls.filter(([, options]) => options?.method === 'POST');
    expect(writes).toHaveLength(2);
    for (const [, options] of writes) {
        expect(JSON.parse(String(options?.body))).toEqual({principal: 'bob', actions: ['file.read', 'dataset.manage'], catalog: rule.catalog, ref: rule.ref, object: rule.object, aspect: rule.aspect, dataset: rule.dataset});
        expect(options?.credentials).toBe('same-origin');
        expect(new Headers(options?.headers).get('X-Kc-Console')).toBe('1');
    }
});

test('revocation displays the whole rule and self-revocation warning, allows cancel and sends DELETE only after confirmation', async () => {
    const server = fixture('/ui/auth/grants'); render(<KCApp/>);
    fireEvent.click(await screen.findByRole('button', {name: `撤销 ${rule.id}`}));
    let dialog = await screen.findByRole('dialog');
    for (const value of [rule.id, rule.catalog, rule.ref, rule.object, rule.aspect, rule.dataset]) expect(within(dialog).getByText(value)).toBeTruthy();
    expect(within(dialog).getByText(/撤销的是当前账号的规则/)).toBeTruthy();
    fireEvent.click(within(dialog).getByRole('button', {name: '取消'}));
    expect(server.fetcher.mock.calls.some(([, options]) => options?.method === 'DELETE')).toBe(false);
    fireEvent.click(screen.getByRole('button', {name: `撤销 ${rule.id}`}));
    dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', {name: '确认撤销'}));
    await waitFor(() => expect(screen.queryByRole('button', {name: `撤销 ${rule.id}`})).toBe(null));
    const removal = server.fetcher.mock.calls.find(([, options]) => options?.method === 'DELETE');
    expect(removal?.[0]).toBe('/admin/v1/grants/own%2Fgrant');
    expect(new Headers(removal?.[1]?.headers).get('Content-Type')).toBe('application/json');
    expect(new Headers(removal?.[1]?.headers).get('X-Kc-Console')).toBe('1');
});

test('a management read refusal offers exact scope selection and self-check without disabling writes or asking for another login', async () => {
    const server = fixture('/ui/auth/grants'); server.deny(); render(<KCApp/>);
    await screen.findByText('当前账号无法读取这个范围的 KC 授权规则。请选择获准管理的范围。');
    expect(screen.getByRole('link', {name: '返回我的权限'}).getAttribute('href')).toBe('/ui/auth/permissions');
    expect(screen.getByLabelText('授权主体')).toBeTruthy();
    expect(window.location.pathname).toBe('/ui/auth/grants');
    expect(session.get()?.principal).toBe('alice');
});

test('an anonymous permissions visitor returns through the original login with query and hash intact', async () => {
    const server = fixture('/ui/auth/permissions?from=catalog#request'); server.native.setIdentity(); render(<KCApp/>);
    await screen.findByLabelText('Access Key ID');
    expect(new URLSearchParams(window.location.search).get('next')).toBe('/auth/permissions?from=catalog#request');
    expect(server.fetcher.mock.calls.some(([path]) => String(path) === '/identity/v1/admission')).toBe(false);
});

test('leaving permissions aborts and discards a late response from the previous page', async () => {
    const server = fixture();
    let complete!: (response: Response) => void; let signal: AbortSignal | undefined;
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, options?: RequestInit) => String(input) === '/identity/v1/admission' ?
        new Promise<Response>(resolve => {complete = resolve; signal = options?.signal ?? undefined;}) : server.fetcher(input, options)));
    render(<KCApp/>);
    await waitFor(() => expect(complete).toBeDefined());
    fireEvent.click(screen.getByRole('link', {name: 'Repositories'}));
    await screen.findByRole('button', {name: 'Create Repository'});
    expect(signal?.aborted).toBe(true);
    await act(async () => {complete(json({principal: 'alice', grants: [{id: 'private-previous-user', actions: ['*']}], request: {administrators: []}}));});
    expect(screen.queryByText('private-previous-user')).toBe(null);
    expect(window.location.pathname).toBe('/ui/repositories');
});

test('a scoped grant reader can choose an exact Catalog and Dataset and refresh the unchanged scope', async () => {
    const server = fixture('/ui/auth/grants/'); server.denyRead(true); render(<KCApp/>);
    await screen.findByText('当前账号无法读取这个范围的 KC 授权规则。请选择获准管理的范围。');
    expect(screen.getByRole('link', {name: '授权管理'}).classList.contains('active')).toBe(true);
    server.denyRead(false);
    fireEvent.change(screen.getByLabelText('读取范围'), {target: {value: 'catalog'}});
    fireEvent.change(screen.getByLabelText('读取 Catalog'), {target: {value: 'kr://example/catalog'}});
    fireEvent.change(screen.getByLabelText('读取 Dataset（可选）'), {target: {value: 'shared'}});
    fireEvent.click(screen.getByRole('button', {name: '加载规则'}));
    await screen.findByRole('table', {name: '授权规则'});
    fireEvent.click(screen.getByRole('button', {name: '加载规则'}));
    await waitFor(() => expect(server.fetcher.mock.calls.filter(([input]) => String(input).includes('catalog=kr%3A%2F%2Fexample%2Fcatalog&dataset=shared'))).toHaveLength(2));
    expect(server.fetcher.mock.calls.some(([input]) => String(input).includes('principal=') || String(input).includes('action='))).toBe(false);
});

test('manage without read can add a rule and keeps successful publication separate from the forbidden refresh', async () => {
    const server = fixture('/ui/auth/grants'); server.denyRead(true); render(<KCApp/>);
    fireEvent.change(await screen.findByLabelText('授权主体'), {target: {value: 'bob'}});
    fireEvent.change(screen.getByLabelText('允许动作'), {target: {value: 'file.read'}});
    fireEvent.change(screen.getByLabelText('Repository 范围'), {target: {value: 'kr://example/source'}});
    fireEvent.click(screen.getByRole('button', {name: '添加授权'}));
    await screen.findByText('授权已添加。');
    expect(screen.getByText('added')).toBeTruthy();
    expect(screen.getByText('kr://example/source')).toBeTruthy();
    await screen.findByText('当前账号无法读取这个范围的 KC 授权规则。请选择获准管理的范围。');
    expect(window.location.pathname).toBe('/ui/auth/grants');
    expect(session.get()?.principal).toBe('alice');
});

test('revoking a current account rule keeps success visible when the following management read is forbidden', async () => {
    const server = fixture('/ui/auth/grants'); render(<KCApp/>);
    fireEvent.click(await screen.findByRole('button', {name: `撤销 ${rule.id}`}));
    server.denyAfterRemoval();
    fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', {name: '确认撤销'}));
    await screen.findByText('授权已撤销。');
    await screen.findByText('当前账号无法读取这个范围的 KC 授权规则。请选择获准管理的范围。');
    expect(screen.getByText(rule.id)).toBeTruthy();
    expect(window.location.pathname).toBe('/ui/auth/grants');
    expect(session.get()?.principal).toBe('alice');
});

test('duplicate legacy rule IDs remain visible and cannot send an ambiguous revoke request', async () => {
    const server = fixture('/ui/auth/grants'); server.setRules([rule, {...rule, principal: 'bob'}]); render(<KCApp/>);
    await screen.findByText('发现重复授权标识，相关规则无法明确撤销。请联系管理员显式修复授权账。');
    const table = screen.getByRole('table', {name: '授权规则'});
    expect(within(table).getByText('alice')).toBeTruthy(); expect(within(table).getByText('bob')).toBeTruthy();
    const buttons = within(table).getAllByRole('button', {name: `撤销 ${rule.id}`});
    expect(buttons).toHaveLength(2);
    for (const button of buttons) {expect((button as HTMLButtonElement).disabled).toBe(true); fireEvent.click(button);}
    expect(server.fetcher.mock.calls.some(([, options]) => options?.method === 'DELETE')).toBe(false);
});

test('an identity change resets the permission form and pending revoke target even when logout notifications are batched', async () => {
    const server = fixture('/ui/auth/grants'); render(<KCApp/>);
    fireEvent.change(await screen.findByLabelText('授权主体'), {target: {value: 'private-alice-input'}});
    fireEvent.click(await screen.findByRole('button', {name: `撤销 ${rule.id}`}));
    await screen.findByRole('dialog');
    server.setIdentity('bob');
    await act(async () => {await session.restore({mode: 'lakefs', localAssertion: false, accepts: ['Cookie']});});
    await screen.findByLabelText('授权主体');
    expect((screen.getByLabelText('授权主体') as HTMLInputElement).value).toBe('');
    expect(screen.queryByRole('dialog')).toBe(null);
    expect(session.get()?.principal).toBe('bob');
    expect(window.location.pathname).toBe('/ui/auth/grants');
});

test('leaving the grant form aborts its single write and does not publish a late success receipt', async () => {
    const server = fixture('/ui/auth/grants'); let complete!: (response: Response) => void; let signal: AbortSignal | undefined;
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, options?: RequestInit) => options?.method === 'POST' && String(input) === '/admin/v1/grants' ?
        new Promise<Response>(resolve => {complete = resolve; signal = options.signal ?? undefined;}) : server.fetcher(input, options)));
    render(<KCApp/>);
    fireEvent.change(await screen.findByLabelText('授权主体'), {target: {value: 'bob'}});
    fireEvent.change(screen.getByLabelText('允许动作'), {target: {value: 'file.read'}});
    fireEvent.change(screen.getByLabelText('Repository 范围'), {target: {value: 'kr://example/source'}});
    fireEvent.click(screen.getByRole('button', {name: '添加授权'}));
    await waitFor(() => expect(complete).toBeDefined());
    expect((screen.getByRole('button', {name: '添加授权'}) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(await screen.findByRole('link', {name: 'Repositories'}));
    await screen.findByRole('button', {name: 'Create Repository'});
    expect(signal?.aborted).toBe(true);
    await act(async () => {complete(json({...rule, id: 'late-rule'}));});
    expect(screen.queryByText('授权已添加。')).toBe(null);
    expect(window.location.pathname).toBe('/ui/repositories');
});

test('native RBAC controls remain disabled while the KC self-check remains usable', async () => {
    fixture('/ui/auth/users'); render(<KCApp/>);
    await screen.findByText('Role-based access control not configured.');
    expect(screen.getByText('Users').closest('.nav-link')?.classList.contains('disabled')).toBe(true);
    expect(screen.getByRole('link', {name: '我的权限'}).getAttribute('href')).toBe('/ui/auth/permissions');
    fireEvent.click(screen.getByRole('link', {name: '我的权限'}));
    await screen.findByRole('table', {name: '显式授权规则'});
    expect(screen.queryByText('Role-based access control not configured.')).toBe(null);
});

test('a current permission request authentication refusal returns to the original login instead of leaving a stale self-check', async () => {
    const server = fixture('/ui/auth/permissions?from=catalog#request');
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, options?: RequestInit) => String(input) === '/identity/v1/admission' ?
        Promise.resolve(json({error: {code: 'UNAUTHENTICATED', message: 'Expired native proof'}}, 401)) : server.fetcher(input, options)));
    render(<KCApp/>);
    await screen.findByLabelText('Access Key ID');
    expect(window.location.pathname).toBe('/ui/auth/login');
    expect(new URLSearchParams(window.location.search).get('next')).toBe('/auth/permissions?from=catalog#request');
    expect(screen.queryByRole('table', {name: '显式授权规则'})).toBe(null);
});

test('the original menu re-reads the native profile after an in-place identity change', async () => {
    const server = fixture('/ui/auth/grants'); render(<KCApp/>);
    await screen.findByRole('button', {name: /alice/});
    server.setIdentity('bob');
    await act(async () => {await session.restore({mode: 'lakefs', localAssertion: false, accepts: ['Cookie']});});
    await screen.findByRole('button', {name: /bob/});
    expect(screen.queryByRole('button', {name: /alice/})).toBe(null);
});

test('a previous native profile finishing late cannot change the menu back after a new identity proof', async () => {
    const server = fixture('/ui/auth/permissions'); let complete!: (response: Response) => void; let reads = 0;
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, options?: RequestInit) => String(input) === '/api/v1/user' && ++reads === 1 ?
        new Promise<Response>(resolve => {complete = resolve;}) : server.fetcher(input, options)));
    render(<KCApp/>);
    await waitFor(() => expect(complete).toBeDefined());
    server.setIdentity('bob');
    await act(async () => {await session.restore({mode: 'lakefs', localAssertion: false, accepts: ['Cookie']});});
    await screen.findByRole('button', {name: /bob/});
    await act(async () => {complete(json({user: {id: 'alice', friendly_name: 'alice'}}));});
    expect(screen.queryByRole('button', {name: /alice/})).toBe(null);
    expect(screen.getByRole('button', {name: /bob/})).toBeTruthy();
    expect(JSON.parse(localStorage.getItem('user') ?? 'null').id).toBe('bob');
});

test('a whitespace-only selected read scope cannot silently become a global inventory request', async () => {
    const server = fixture('/ui/auth/grants'); render(<KCApp/>);
    await screen.findByRole('table', {name: '授权规则'});
    const count = server.fetcher.mock.calls.length;
    fireEvent.change(screen.getByLabelText('读取范围'), {target: {value: 'repository'}});
    fireEvent.change(screen.getByLabelText('读取 Repository'), {target: {value: '   '}});
    fireEvent.click(screen.getByRole('button', {name: '加载规则'}));
    await screen.findByText('请填写读取范围的资源标识。');
    expect(server.fetcher.mock.calls.length).toBe(count);
});

test('the rules list keeps operations next to a compact summary while displaying every scope and provenance field', async () => {
    const server = fixture('/ui/auth/grants');
    const complete = {...rule, repo: 'kr://example/source', shareId: 'share-receipt'};
    server.setRules([complete]); render(<KCApp/>);
    const table = await screen.findByRole('table', {name: '授权规则'});
    const headers = within(table).getAllByRole('columnheader');
    expect(headers).toHaveLength(4);
    expect(headers[0].textContent).toBe('操作');
    const row = within(table).getAllByRole('row')[1];
    const cells = within(row).getAllByRole('cell');
    expect(within(cells[0]).getByRole('button', {name: `撤销 ${rule.id}`})).toBeTruthy();
    for (const value of [complete.id, complete.principal, complete.actions.join(', '), complete.repo, complete.catalog, complete.ref, complete.object, complete.aspect, complete.dataset, complete.sharedBy, complete.shareId]) expect(within(row).getByText(value)).toBeTruthy();
});
