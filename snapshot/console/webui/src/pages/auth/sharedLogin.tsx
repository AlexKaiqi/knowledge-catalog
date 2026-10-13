import React, {useEffect, useRef, useState} from 'react';
import {Alert, Button, Card, Col, Form, Row, Spinner} from 'react-bootstrap';
import {Discovery, session} from '../../lib/api/session';
import {jsonRequest} from '../../lib/api/transport';
import {useRouter} from '../../lib/hooks/router';

export function safeLoginTarget(value: unknown): string {
    return typeof value === 'string' && value.startsWith('/') && !value.startsWith('//') && Array.from(value).every(character => {
        const code = character.charCodeAt(0);
        return character !== '\\' && code >= 32 && (code < 127 || code > 159);
    }) ? value : '/';
}
const base64url = (bytes: Uint8Array): string => btoa(String.fromCharCode(...bytes)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');

export default function SharedLogin() {
    const router = useRouter();
    const [discovery, setDiscovery] = useState<Discovery | null>(null);
    const [token, setToken] = useState('');
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState<Error | null>(null);
    const request = useRef<AbortController | null>(null);
    useEffect(() => () => request.current?.abort(), []);
    useEffect(() => {
        const controller = new AbortController();
        jsonRequest<Discovery>('/identity/v1/auth', undefined, controller.signal).then(setDiscovery).catch(setError);
        return () => controller.abort();
    }, []);
    const finish = () => window.location.assign(`/ui${safeLoginTarget(router.query.next)}`);
    const organization = async () => {
        request.current?.abort();
        const controller = new AbortController(); request.current = controller;
        const tab = window.open('about:blank', 'kc-auth');
        if (!tab) {setError(new Error('请允许打开认证页面后重试。')); return;}
        setBusy(true); setError(null);
        try {
            const verifier = base64url(crypto.getRandomValues(new Uint8Array(32)));
            const state = base64url(crypto.getRandomValues(new Uint8Array(32)));
            const challenge = base64url(new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier))));
            const authorization = await jsonRequest<{authorizationURL: string; requestURI: string; expiresIn: number}>('/identity/v1/authorize', {codeChallenge: challenge, state}, controller.signal);
            tab.opener = null; tab.location.href = authorization.authorizationURL;
            const deadline = Date.now() + authorization.expiresIn * 1000;
            while (Date.now() < deadline && !controller.signal.aborted) {
                const result = await jsonRequest<{status: string; code?: string; redirectURI?: string}>('/identity/v1/authorize:poll', {requestURI: authorization.requestURI}, controller.signal);
                if (result.status === 'completed') {
                    const issued = await jsonRequest<{accessToken: string; refreshToken?: string; expiresIn: number}>('/identity/v1/token', {grantType: 'authorization_code', code: result.code, redirectURI: result.redirectURI, codeVerifier: verifier}, controller.signal);
                    await session.authenticate(discovery!, issued.accessToken, issued.refreshToken, issued.expiresIn, controller.signal);
                    finish(); return;
                }
                await new Promise(resolve => setTimeout(resolve, 1000));
            }
            throw new Error('认证请求已过期，请重试。');
        } catch (failure) {tab.close(); if (!controller.signal.aborted) setError(failure as Error);}
        finally {if (!controller.signal.aborted) setBusy(false);}
    };
    return <Row><Col md={{offset: 4, span: 4}}><Card className="login-widget">
        <Card.Header>认证</Card.Header><Card.Body>
            <p>使用现有认证系统的身份访问整个控制台。</p>
            {error && <Alert variant="danger">{error.message}</Alert>}
            {!discovery ? <Spinner animation="border"/> : <>
                {discovery.browserLogin && <Button className="mb-3 w-100" disabled={busy} onClick={organization}>组织认证</Button>}
                <Form onSubmit={async event => {
                    request.current?.abort();
                    const controller = new AbortController(); request.current = controller;
                    event.preventDefault(); setBusy(true); setError(null);
                    try {await session.authenticate(discovery, token.trim(), undefined, undefined, controller.signal); finish();}
                    catch (failure) {if (!controller.signal.aborted) setError(failure as Error);}
                    finally {if (!controller.signal.aborted) setBusy(false);}
                }}>
                    <Form.Group className="mb-3"><Form.Label>已签发的访问令牌</Form.Label>
                        <Form.Control type="password" autoComplete="off" aria-label="访问令牌" value={token} onChange={event => setToken(event.target.value)} required/>
                        <Form.Text>令牌由认证系统签发，无需 lakeFS 管理员凭据。</Form.Text>
                    </Form.Group>
                    <Button type="submit" disabled={busy} className="w-100">验证身份</Button>
                </Form>
            </>}
        </Card.Body></Card></Col></Row>;
}
