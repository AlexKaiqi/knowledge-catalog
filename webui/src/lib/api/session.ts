import { auth } from './index';
import { jsonRequest, KCError } from './transport';

export type Discovery = { mode: string; localAssertion: boolean; accepts: string[] };
export type LoginSession = { mode: 'lakefs'; principal: string };
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
        // Discard credentials saved by the superseded separate Dataset login.
        window.sessionStorage.removeItem(`kc.ui.session.v1:${window.location.origin}`);
        window.localStorage.removeItem('kc.ui.token');
        window.localStorage.removeItem('kc.ui.as');
        notify();
    },
    require(): LoginSession { if (!current) throw changed(); return current; },
    async restore(discovery: Discovery, signal?: AbortSignal): Promise<void> {
        session.logout();
        if (!discovery || discovery.mode !== 'lakefs' || discovery.localAssertion || !Array.isArray(discovery.accepts) || !discovery.accepts.includes('Cookie')) {
            throw new KCError(503, {code: 'PRECONDITION_FAILED', message: '控制台尚未连接共享登录，请联系管理员。'});
        }
        const version = generation;
        try {
            const who = await jsonRequest<{principal: string}>('/identity/v1/whoami', undefined, signal);
            if (version !== generation || signal?.aborted) throw changed();
            if (!who || typeof who.principal !== 'string' || !who.principal) throw new KCError(502, {code: 'TRANSPORT', message: '服务未确认当前账号。'});
            // The native menu must not display the previous login's cached profile.
            auth.clearCurrentUser();
            current = {mode: 'lakefs', principal: who.principal};
            notify();
        } catch (error) {
            if (version === generation) session.logout();
            throw error;
        }
    },
};
