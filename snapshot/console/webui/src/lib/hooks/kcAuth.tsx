import React, { createContext, useContext, useEffect, useState } from 'react';
import { Discovery, LoginSession, session } from '../api/session';
import { jsonRequest } from '../api/transport';

type AuthState = { user: LoginSession | null; loading: boolean; error: Error | null; retry: () => void };
const AuthContext = createContext<AuthState | null>(null);
export const useAuth = (): AuthState => {
    const auth = useContext(AuthContext);
    if (!auth) throw new Error('Missing authentication provider');
    return auth;
};
export const AuthProvider: React.FC<React.PropsWithChildren> = ({ children }) => {
    const [user, setUser] = useState(session.get());
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<Error | null>(null);
    const [attempt, setAttempt] = useState(0);
    useEffect(() => session.subscribe(() => setUser(session.get())), []);
    useEffect(() => {
        const controller = new AbortController();
        setLoading(true); setError(null);
        (async () => {
            try {
                const discovery = await jsonRequest<Discovery>('/identity/v1/auth', undefined, controller.signal);
                if (!controller.signal.aborted) await session.restore(discovery, controller.signal);
            } catch (err) { if (!controller.signal.aborted) setError(err as Error); }
            finally { if (!controller.signal.aborted) setLoading(false); }
        })();
        return () => { controller.abort(); };
    }, [attempt]);
    return <AuthContext.Provider value={{user, loading, error, retry: () => setAttempt(value => value + 1)}}>{children}</AuthContext.Provider>;
};

export function useKCQuery<T>(request: (signal: AbortSignal) => Promise<T>, deps: unknown[] = []) {
    const [data, setData] = useState<T | null>(null);
    const [error, setError] = useState<Error | null>(null);
    const [loading, setLoading] = useState(true);
    const [attempt, setAttempt] = useState(0);
    useEffect(() => {
        const controller = new AbortController();
        setData(null); setError(null); setLoading(true);
        request(controller.signal).then(value => {
            if (!controller.signal.aborted) setData(value);
        }).catch(error => {
            if (!controller.signal.aborted) setError(error);
        }).finally(() => {
            if (!controller.signal.aborted) setLoading(false);
        });
        return () => controller.abort();
    }, [...deps, attempt]);
    return {data, error, loading, reload: () => setAttempt(value => value + 1)};
}
