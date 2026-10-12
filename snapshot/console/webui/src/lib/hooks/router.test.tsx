import React from 'react';
import { afterEach, expect, test } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { useRouter } from './router';

afterEach(cleanup);

const CurrentRoute = () => <output>{useRouter().route}</output>;
const FullRoute = () => <output>{useRouter().fullPath}</output>;

test.each([
    ['/ui/repositories/example/objects', '/repositories/example/objects'],
    ['/ui/ui/example', '/ui/example'],
])('the router basename consumes only the transport prefix in %s', (url, expected) => {
    render(<MemoryRouter basename="/ui" initialEntries={[url]}><CurrentRoute/></MemoryRouter>);
    expect(screen.getByText(expected)).toBeTruthy();
});

test('login return paths preserve the complete internal URL beneath the basename', () => {
    render(<MemoryRouter basename="/ui" initialEntries={['/ui/catalogs/example/datasets/shared?revision=4#history']}><FullRoute/></MemoryRouter>);
    expect(screen.getByText('/catalogs/example/datasets/shared?revision=4#history')).toBeTruthy();
});
