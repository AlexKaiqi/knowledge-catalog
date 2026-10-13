import { auth } from './index';
import { jsonRequest, KCError } from './transport';
import {userProof} from './sessionProof';

export type Discovery = { mode: string; localAssertion: boolean; accepts: string[]; providerURL?: string; browserLogin?: {oauth2Base: string; clientId: string} };
export type LoginSession = { mode: string; principal: string };
let current: LoginSession | null = null;
let generation = 0;
const listeners = new Set<() => void>();
function notify(): void { listeners.forEach(listener => listener()); }
function changed(): KCError {
    return new KCError(401, {code: 'UNAUTHENTICATED', message: '登录已变更，请重新登录。'});
}

export const session = {
    get: (): LoginSession | null => current,
    version: (): number => generation,
    subscribe(listener: () => void): () => void { listeners.add(listener); return () => {listeners.delete(listener);}; },
    logout(): void {
        generation++;
        current = null;
        userProof.clear();
        auth.clearCurrentUser();
        // Discard credentials saved by the superseded separate Dataset login.
        window.sessionStorage.removeItem(`kc.ui.session.v1:${window.location.origin}`);
        window.localStorage.removeItem('kc.ui.token');
        window.localStorage.removeItem('kc.ui.as');
        notify();
    },
    require(): LoginSession { if (!current) throw changed(); return current; },
    async authenticate(discovery: Discovery, accessToken: string, refreshToken?: string, expiresIn?: number, signal?: AbortSignal): Promise<void> {
        session.logout();
        userProof.set({mode: discovery.mode, accessToken, refreshToken, expiresAt: expiresIn ? Date.now() + expiresIn * 1000 : undefined});
        const proofVersion = userProof.version();
        try {
            await session.restore(discovery, signal);
            if (proofVersion !== userProof.version()) throw changed();
            const proof = userProof.get();
            if (proof && current) userProof.set({...proof, principal: current.principal});
        } catch (error) { if (proofVersion === userProof.version()) session.logout(); throw error; }
    },
    async restore(discovery: Discovery, signal?: AbortSignal): Promise<void> {
        generation++; current = null;
        window.sessionStorage.removeItem(`kc.ui.session.v1:${window.location.origin}`);
        window.localStorage.removeItem('kc.ui.token'); window.localStorage.removeItem('kc.ui.as');
        const external = discovery && ['taihu', 'gitea'].includes(discovery.mode) && discovery.accepts?.includes('Authorization');
        if (!discovery || discovery.localAssertion || !Array.isArray(discovery.accepts) || !(external || discovery.mode === 'lakefs' && discovery.accepts.includes('Cookie'))) {
            session.logout();
            throw new KCError(503, {code: 'PRECONDITION_FAILED', message: '控制台尚未连接共享登录，请联系管理员。'});
        }
        if (userProof.get() && userProof.get()?.mode !== discovery.mode) userProof.clear();
        const version = generation;
        try {
            const who = await jsonRequest<{principal: string}>('/identity/v1/whoami', undefined, signal);
            if (version !== generation || signal?.aborted) throw changed();
            if (!who || typeof who.principal !== 'string' || !who.principal) throw new KCError(502, {code: 'TRANSPORT', message: '服务未确认当前账号。'});
            // The native menu must not display the previous login's cached profile.
            auth.clearCurrentUser();
            current = {mode: discovery.mode, principal: who.principal};
            notify();
        } catch (error) {
            if (version === generation) session.logout();
            throw error;
        }
    },
};
