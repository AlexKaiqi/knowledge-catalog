import React from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import KCApp from './kcApp';
import { IndexPage } from '../pages';
import { Route } from 'react-router-dom';
import { session } from '../lib/api/session';

const nativeRequests: string[] = [];
beforeEach(() => {
    session.logout(); localStorage.clear(); nativeRequests.length = 0;
    window.history.replaceState(null, '', '/ui/');
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
        const path = String(input); nativeRequests.push(path);
        const result = path.includes('setup_lakefs') ? { state: 'initialized', login_config: { RBAC: 'simplified', login_cookie_names: [] } } :
            path.includes('/user') ? { user: { id: 'native-user', friendly_name: 'Native User' } } :
            path.includes('/config/version') ? { version: 'v1.58.0', version_context: 'lakeFS', upgrade_recommended: false } :
            path === '/api/v1/config' ? { storage_config: { blockstore_type: 'local', import_support: true }, version_config: { version: 'v1.58.0', version_context: 'lakeFS', upgrade_recommended: false } } :
            path.startsWith('/api/v1/repositories') ? { results: [], pagination: { has_more: false } } :
            path === '/identity/v1/whoami' ? {principal: 'native-user'} : path === '/identity/v1/auth' ? {mode: 'lakefs', localAssertion: false, accepts: ['Cookie']} :
            path === '/snapshot/v1/datasets' ? {datasets: []} : {};
        return new Response(JSON.stringify(result), { status: 200, headers: { 'Content-Type': 'application/json' } });
    }));
});
afterEach(() => { cleanup(); session.logout(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe('complete lakeFS console', () => {
    it('starts in the original Repositories screen with native navigation and branding', async () => {
        render(<KCApp />);
        await screen.findByRole('button', { name: 'Create Repository' });
        expect(screen.getByRole('link', { name: 'Repositories' }).getAttribute('href')).toBe('/ui/repositories');
        expect(screen.getByRole('link', { name: 'Administration' }).getAttribute('href')).toBe('/ui/auth');
        expect(screen.getByRole('link', { name: 'Datasets' }).getAttribute('href')).toBe('/ui/datasets');
        expect(screen.getByRole('img', { name: 'lakeFS' }).getAttribute('src')).toBe('/ui/logo.png');
        expect(nativeRequests.some(path => path.startsWith('/api/v1/repositories'))).toBe(true);
    });
    it('uses the original native login for Dataset without asking for another credential', async () => {
        render(<KCApp />);
        await screen.findByRole('link', { name: 'Datasets' });
        fireEvent.click(screen.getByRole('link', { name: 'Datasets' }));
        await screen.findByLabelText('搜索 Dataset');
        expect(screen.queryByRole('heading', {name: '登录 Dataset 控制台'})).toBe(null);
        expect(window.location.pathname).toBe('/ui/datasets');
        fireEvent.click(screen.getByRole('link', { name: 'Repositories' }));
        await screen.findByRole('button', { name: 'Create Repository' });
        await waitFor(() => expect(window.location.pathname).toBe('/ui/repositories'));
        const fetcher = vi.mocked(fetch);
        for (const [input, options] of fetcher.mock.calls) if (String(input).startsWith('/api/v1')) {
            const headers = new Headers(options?.headers);
            expect(headers.has('Authorization')).toBe(false);
            expect(headers.has('X-Kc-As')).toBe(false);
        }
    });
    it('keeps native API credentials separate from the fixed cookie bridge marker', async () => {
        localStorage.setItem('kc.ui.token', 'obsolete-kc-token');
        await session.restore({mode: 'lakefs', localAssertion: false, accepts: ['Cookie']});
        render(<KCApp/>);
        await screen.findByRole('button', {name: 'Create Repository'});
        const requests = vi.mocked(fetch).mock.calls.filter(([path]) => String(path).startsWith('/api/v1'));
        expect(requests.length).toBeGreaterThan(0);
        for (const [, options] of requests) {
            const headers = new Headers(options?.headers);
            expect(headers.has('X-Kc-As')).toBe(false);
            expect(headers.has('Authorization')).toBe(false);
            expect(headers.has('X-Kc-Console')).toBe(false);
        }
        const proof = vi.mocked(fetch).mock.calls.find(([path]) => String(path) === '/identity/v1/whoami');
        expect(proof?.[1]?.credentials).toBe('same-origin');
        expect(new Headers(proof?.[1]?.headers).get('X-Kc-Console')).toBe('1');
        expect(localStorage.getItem('kc.ui.token')).toBe(null);
    });
    it('a native authentication refusal arriving after navigation cannot replace the authenticated Dataset page', async () => {
        const nativeFetch = fetch;
        let complete!: (response: Response) => void;
        vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, options?: RequestInit) => String(input).startsWith('/api/v1/repositories') ?
            new Promise<Response>(resolve => {complete = resolve;}) : nativeFetch(input, options)));
        render(<KCApp/>);
        await waitFor(() => expect(complete).toBeDefined());
        fireEvent.click(screen.getByRole('link', {name: 'Datasets'}));
        await screen.findByLabelText('搜索 Dataset');
        await act(async () => {complete(new Response(JSON.stringify({message: 'error authenticating request'}), {status: 401, headers: {'Content-Type': 'application/json'}}));});
        expect(window.location.pathname).toBe('/ui/datasets');
    });
    it('native authentication preserves query and hash in the original login return URL', async () => {
        const errors = vi.spyOn(console, 'error');
        window.history.replaceState(null, '', '/ui/repositories/example/objects?ref=feature%2Fwork#preview');
        const nativeFetch = fetch;
        vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, options?: RequestInit) => String(input) === '/api/v1/user' ?
            Promise.resolve(new Response(JSON.stringify({message: 'error authenticating request'}), {status: 401, headers: {'Content-Type': 'application/json'}})) : nativeFetch(input, options)));
        render(<KCApp/>);
        await screen.findByLabelText('Access Key ID');
        expect(window.location.pathname).toBe('/ui/auth/login');
        expect(new URLSearchParams(window.location.search).get('next')).toBe('/repositories/example/objects?ref=feature%2Fwork#preview');
        expect(errors.mock.calls.some(args => args.join(' ').includes('Cannot update a component'))).toBe(false);
        errors.mockRestore();
    });
    it.each(['//evil.example/path', '/\\evil.example/path', '/repositories\n//evil.example', '/repositories\u0000ignored'])('native login rejects an unsafe return path %j', async next => {
        window.history.replaceState(null, '', `/ui/auth/login?${new URLSearchParams({next})}`);
        const history = vi.spyOn(window.history, 'pushState');
        render(<KCApp/>);
        fireEvent.change(await screen.findByLabelText('Access Key ID'), {target: {value: 'native-key'}});
        fireEvent.change(screen.getByLabelText('Secret Access Key'), {target: {value: 'native-secret'}});
        fireEvent.click(screen.getByRole('button', {name: 'Login'}));
        await screen.findByRole('button', {name: 'Create Repository'});
        expect(history.mock.calls[0][2]).toBe('/ui');
        expect(window.location.pathname).toBe('/ui/repositories');
        expect(window.location.hostname).not.toBe('evil.example');
    });
    it('native login follows an internal return path with its query and hash intact', async () => {
        const next = '/datasets?search=shared#history';
        window.history.replaceState(null, '', `/ui/auth/login?${new URLSearchParams({next})}`);
        render(<KCApp/>);
        fireEvent.change(await screen.findByLabelText('Access Key ID'), {target: {value: 'native-key'}});
        fireEvent.change(screen.getByLabelText('Secret Access Key'), {target: {value: 'native-secret'}});
        fireEvent.click(screen.getByRole('button', {name: 'Login'}));
        await screen.findByLabelText('搜索 Dataset');
        expect(window.location.pathname + window.location.search + window.location.hash).toBe('/ui/datasets?search=shared#history');
    });
    it('the original SSO redirect follows the configured URL outside render', async () => {
        const errors = vi.spyOn(console, 'error');
        const navigate = vi.spyOn(window.location, 'assign').mockImplementation(() => {});
        const nativeFetch = fetch;
        vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, options?: RequestInit) => String(input) === '/api/v1/setup_lakefs' ?
            Promise.resolve(new Response(JSON.stringify({state: 'initialized', login_config: {login_url: '/oidc/login?redirect=original', login_cookie_names: []}}), {status: 200, headers: {'Content-Type': 'application/json'}})) : nativeFetch(input, options)));
        window.history.replaceState(null, '', '/ui/auth/login?next=%2Fdatasets&redirected=true');
        render(<KCApp/>);
        await waitFor(() => expect(navigate).toHaveBeenCalledWith('/oidc/login?redirect=original'));
        expect(navigate).toHaveBeenCalledTimes(1);
        expect(screen.queryByLabelText('Access Key ID')).toBe(null);
        expect(errors.mock.calls.some(args => args.join(' ').includes('Cannot update a component'))).toBe(false);
    });
    it('the original setup redirect preserves its query without navigating during render', async () => {
        const errors = vi.spyOn(console, 'error');
        const nativeFetch = fetch;
        vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, options?: RequestInit) => String(input) === '/api/v1/setup_lakefs' ?
            Promise.resolve(new Response(JSON.stringify({state: 'not_initialized'}), {status: 200, headers: {'Content-Type': 'application/json'}})) : nativeFetch(input, options)));
        window.history.replaceState(null, '', '/ui/auth/login?next=%2Fdatasets%3Fsearch%3Dshared');
        render(<KCApp/>);
        await screen.findByPlaceholderText('Admin Username');
        expect(window.location.pathname).toBe('/ui/setup');
        expect(new URLSearchParams(window.location.search).get('next')).toBe('/datasets?search=shared');
        expect(errors.mock.calls.some(args => args.join(' ').includes('Cannot update a component'))).toBe(false);
    });
    it('native logout navigates through the backend redirect chain without fetching it', async () => {
        await session.restore({mode: 'lakefs', localAssertion: false, accepts: ['Cookie']});
        render(<KCApp/>);
        fireEvent.click(await screen.findByRole('button', {name: /Native User/}));
        expect(localStorage.getItem('user')).not.toBe(null);
        const original = Object.getOwnPropertyDescriptor(window, 'location')!;
        const location = window.location;
        const navigate = vi.fn();
        Object.defineProperty(window, 'location', {configurable: true, get: () => location, set: navigate});
        try {
            fireEvent.click(screen.getByRole('button', {name: 'Logout'}));
            expect(navigate).toHaveBeenCalledTimes(1);
            expect(navigate).toHaveBeenCalledWith('/logout');
            expect(localStorage.getItem('user')).toBe(null);
            expect(session.get()).toBe(null);
            expect(vi.mocked(fetch).mock.calls.some(([path]) => String(path) === '/logout')).toBe(false);
        } finally { Object.defineProperty(window, 'location', original); }
    });
});

it('retains every route from the vendored Apache-2.0 console', () => {
    const routes = new Set<string>();
    function visit(node: React.ReactNode, prefix = ''): void {
        React.Children.forEach(node, child => {
            if (!React.isValidElement<{path?: string; children?: React.ReactNode}>(child)) return;
            let path = prefix;
            if (child.type === Route) {
                path = child.props.path ? `${prefix}/${child.props.path}`.replace(/\/+/g, '/') : prefix;
                routes.add(path);
            }
            visit(child.props.children, path);
        });
    }
    visit(IndexPage());
    const native = ['/repositories', '/repositories/:repoId', '/repositories/:repoId/objects', '/repositories/:repoId/object', '/repositories/:repoId/changes',
        '/repositories/:repoId/commits', '/repositories/:repoId/commits/:commitId', '/repositories/:repoId/branches', '/repositories/:repoId/tags',
        '/repositories/:repoId/pulls', '/repositories/:repoId/pulls/create', '/repositories/:repoId/pulls/:pullId', '/repositories/:repoId/compare/*',
        '/repositories/:repoId/actions', '/repositories/:repoId/actions/:runId', '/repositories/:repoId/settings', '/repositories/:repoId/settings/general',
        '/repositories/:repoId/settings/retention', '/repositories/:repoId/settings/branches', '/auth', '/auth/login', '/auth/users/create', '/auth/credentials',
        '/auth/users', '/auth/users/:userId', '/auth/users/:userId/groups', '/auth/users/:userId/policies', '/auth/users/:userId/policies/effective',
        '/auth/users/:userId/credentials', '/auth/groups', '/auth/groups/:groupId', '/auth/groups/:groupId/members', '/auth/groups/:groupId/policies',
        '/auth/policies', '/auth/policies/:policyId', '/auth/permissions', '/auth/grants', '/setup', '/setup/*'];
    for (const path of native) expect(routes.has(path), path).toBe(true);
});
