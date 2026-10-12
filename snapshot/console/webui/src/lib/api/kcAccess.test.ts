import {afterEach, expect, test, vi} from 'vitest';
import {kc} from './kc';
import {session} from './session';

afterEach(() => {session.logout(); localStorage.clear(); vi.unstubAllGlobals();});
async function reply(value: unknown, status = 200) {
    vi.stubGlobal('fetch', vi.fn(async (path: string) => new Response(JSON.stringify(path === '/identity/v1/whoami' ? {principal: 'alice'} : value), {status: path === '/identity/v1/whoami' ? 200 : status})));
    await session.restore({mode: 'lakefs', localAssertion: false, accepts: ['Cookie']});
}

test.each([
    ['grants inventory', {rules: {}}, () => kc.grants()],
    ['grant rule', {rules: [{id: 'bad', principal: 123, actions: ['file.read']}]}, () => kc.grants()],
    ['admission grants', {principal: 'alice', grants: null, request: {administrators: []}}, () => kc.admission()],
    ['admission request', {principal: 'alice', grants: [], request: {administrators: 'admin'}}, () => kc.admission()],
    ['new grant', {}, () => kc.addGrant({principal: 'bob', actions: ['file.read'], })],
    ['revoke receipt', {}, () => kc.removeGrant('expected')],
    ['mismatched revoke receipt', {revoked: 'other-rule'}, () => kc.removeGrant('expected')],
] as const)('an invalid %s response is an actionable transport error', async (_label, value, operation) => {
    await reply(value);
    await expect(operation()).rejects.toMatchObject({status: 502, code: 'TRANSPORT'});
});

test('an authentication refusal with a null JSON fault still clears the verified identity', async () => {
    await reply(null, 401);
    await expect(kc.admission()).rejects.toMatchObject({status: 401});
    expect(session.get()).toBe(null);
});

test('the public legacy grant shape may omit actions without becoming an unrestricted action rule', async () => {
    await reply({rules: [{id: 'legacy-empty', principal: 'alice', repo: 'kr://example/source'}]});
    expect((await kc.grants()).rules[0].actions).toEqual([]);
});


test.each(['catalog', 'Catalog', 'CATALOG'])('retired %s grant qualifiers cannot become service-wide rules', async key => {
    vi.stubGlobal('fetch', vi.fn(async (path: string) => new Response(JSON.stringify(path === '/identity/v1/whoami' ? {principal: 'alice'} : {rules: [{id: 'old', principal: 'alice', actions: ['file.read'], [key]: 'retired'}]}), {status: 200})));
    await session.restore({mode: 'lakefs', localAssertion: false, accepts: ['Cookie']});
    await expect(kc.grants()).rejects.toMatchObject({status: 502});
});
