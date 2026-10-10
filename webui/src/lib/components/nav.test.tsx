import React from 'react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { RefContextProvider } from '../hooks/repo';
import { RepositoryNavTabs } from './repository/tabs';
import { UserNav, GroupNav } from './auth/nav';
import { WithLoginConfigContext } from '../hooks/conf';

beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
        const path = String(input);
        const value = path.includes('/branches/') ? {id: 'feature/name', commit_id: 'c1'} :
            path.endsWith('/browser-repo') ? {id: 'browser-repo', default_branch: 'feature/name'} :
            path.includes('setup_lakefs') ? {state: 'initialized', login_config: {RBAC: 'full'}} :
            path.includes('/auth/groups/') ? {id: 'operators', description: 'Operators'} : {};
        return new Response(JSON.stringify(value), {status: 200, headers: {'Content-Type': 'application/json'}});
    }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const Location = () => <output>{useLocation().pathname}</output>;

test('all native repository tabs apply the UI basename once and retain reference context', async () => {
    render(<MemoryRouter basename="/ui" initialEntries={['/ui/repositories/browser-repo/objects']}>
        <Routes><Route path="/repositories/:repoId/*" element={<RefContextProvider><RepositoryNavTabs active="objects"/><Location/></RefContextProvider>}/></Routes>
    </MemoryRouter>);
    const suffixes: Record<string, string> = {
        Objects: 'objects?ref=feature%2Fname', 'Uncommitted Changes': 'changes?ref=feature%2Fname',
        Commits: 'commits?ref=feature%2Fname', Branches: 'branches', Tags: 'tags', 'Pull Requests': 'pulls',
        Compare: 'compare?ref=feature%2Fname&compare=feature%2Fname', Actions: 'actions', Settings: 'settings',
    };
    await vi.waitFor(() => expect(screen.getByRole('link', {name: 'Objects'}).getAttribute('href')).toContain('ref=feature%2Fname'));
    for (const [name, suffix] of Object.entries(suffixes)) expect(screen.getByRole('link', {name}).getAttribute('href'), name).toBe(`/ui/repositories/browser-repo/${suffix}`);
    fireEvent.click(screen.getByRole('link', {name: 'Branches'}));
    expect(screen.getByText('/repositories/browser-repo/branches')).toBeTruthy();
});

test('native Administration tabs reuse the same single-basename navigation', async () => {
    render(<MemoryRouter basename="/ui" initialEntries={['/ui/auth/users/alice/groups']}>
        <WithLoginConfigContext><UserNav userId="alice" page="groups"/><GroupNav groupId="operators" page="members"/><Location/></WithLoginConfigContext>
    </MemoryRouter>);
    await screen.findByRole('link', {name: 'Directly Attached Policies'});
    expect(screen.getByRole('link', {name: 'Access Credentials'}).getAttribute('href')).toBe('/ui/auth/users/alice/credentials');
    expect(screen.getByRole('link', {name: 'Attached Policies'}).getAttribute('href')).toBe('/ui/auth/groups/operators/policies');
    fireEvent.click(screen.getByRole('link', {name: 'Access Credentials'}));
    expect(screen.getByText('/auth/users/alice/credentials')).toBeTruthy();
});
