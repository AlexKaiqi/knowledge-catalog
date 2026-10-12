import React, {useEffect} from 'react';
import {Button, Container, Spinner} from 'react-bootstrap';
import {AuthProvider, useAuth} from '../lib/hooks/kcAuth';
import {useRouter} from '../lib/hooks/router';
import {KCError} from '../lib/api/transport';
import ErrorNotice from '../lib/components/kcError';

type Props = React.PropsWithChildren<{onVerified?: () => void}>;
export const KCAuthContent: React.FC<Props> = ({children, onVerified}) => {
    const {user, loading, error, retry} = useAuth();
    const router = useRouter();
    const needLogin = !loading && !user && (!error || error instanceof KCError && error.status === 401);
    useEffect(() => {
        if (needLogin) router.push({pathname: '/auth/login', query: {next: router.fullPath, redirected: 'true'}});
    }, [needLogin, router.fullPath]);
    useEffect(() => {if (user && !loading && !error) onVerified?.();}, [user, loading, error, onVerified]);
    if (loading || needLogin) return <Container className="mt-5"><Spinner animation="border"/> 正在连接服务…</Container>;
    if (error) return <Container className="mt-5"><ErrorNotice error={error}/><Button onClick={retry}>重新连接</Button></Container>;
    return user ? <React.Fragment key={user.principal}>{children}</React.Fragment> : null;
};
const KCAuthGate: React.FC<Props> = ({children, onVerified}) => <AuthProvider><KCAuthContent onVerified={onVerified}>{children}</KCAuthContent></AuthProvider>;
export default KCAuthGate;
