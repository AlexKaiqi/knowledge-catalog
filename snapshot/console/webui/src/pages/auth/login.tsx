import React, {useEffect, useState} from "react";
import Row from "react-bootstrap/Row";
import Card from "react-bootstrap/Card";
import Form from "react-bootstrap/Form";
import Col from "react-bootstrap/Col";
import Button from "react-bootstrap/Button";
import {auth, AuthenticationError, setup, SETUP_STATE_INITIALIZED} from "../../lib/api";
import {AlertError} from "../../lib/components/controls"
import {useRouter} from "../../lib/hooks/router";
import {useAPI} from "../../lib/hooks/api";
import {useNavigate} from "react-router-dom";
import SharedLogin, {safeLoginTarget} from './sharedLogin';

interface LoginConfig {
    login_url: string;
    username_ui_placeholder: string;
    password_ui_placeholder: string;
    login_failed_message?: string;
    fallback_login_url?: string;
    fallback_login_label?: string;
    login_cookie_names: string[];
    logout_url: string;
    snapshot_identity?: boolean;
}

const LoginForm = ({loginConfig = {login_url: "", login_cookie_names: [], logout_url: "", username_ui_placeholder: "", password_ui_placeholder: ""}}: {loginConfig?: LoginConfig}) => {
    const router = useRouter();
    const navigate = useNavigate();
    const [loginError, setLoginError] = useState<string | null>(null);
    const { next } = router.query;
    const usernamePlaceholder = loginConfig.username_ui_placeholder || "Access Key ID";
    const passwordPlaceholder = loginConfig.password_ui_placeholder || "Secret Access Key";
    return (
        <Row>
            <Col md={{offset: 4, span: 4}}>
                <Card className="login-widget">
                    <Card.Header>Login</Card.Header>
                    <Card.Body>
                        <Form onSubmit={async (e) => {
                            e.preventDefault()
                            try {
                                setLoginError(null);
                                const data = new FormData(e.currentTarget);
                                await auth.login(String(data.get('username')), String(data.get('password')))
                                router.push(safeLoginTarget(next));
                                navigate(0);
                            } catch(err) {
                                if (err instanceof AuthenticationError && err.status === 401) {
                                    setLoginError(loginConfig.login_failed_message || "Credentials don't match.");
                                } else {
                                    setLoginError(err instanceof Error ? err.message : 'Could not connect to lakeFS.');
                                }
                            }
                        }}>
                            <Form.Group controlId="username" className="mb-3">
                                <Form.Control name="username" aria-label={usernamePlaceholder} required type="text" placeholder={usernamePlaceholder} autoFocus/>
                            </Form.Group>

                            <Form.Group controlId="password" className="mb-3">
                                <Form.Control name="password" aria-label={passwordPlaceholder} required type="password" placeholder={passwordPlaceholder}/>
                            </Form.Group>

                            {(!!loginError) && <AlertError error={loginError}/>}

                            <Button variant="primary" type="submit">Login</Button>
                        </Form>
                        <div className={"mt-2 mb-1"}>
                            { loginConfig.fallback_login_url ?
                                <Button variant="link" className="text-secondary mt-2" onClick={async ()=> {
                                    loginConfig.login_cookie_names?.forEach(
                                        cookie => {
                                            document.cookie = `${cookie}=; Path=/; Expires=Thu, 01 Jan 1970 00:00:01 GMT;`;
                                        }
                                    );
                                    window.location.assign(loginConfig.fallback_login_url!);
                                }}>{loginConfig.fallback_login_label || 'Try another way to login'}</Button>
                                : ""
                            }
                        </div>
                    </Card.Body>
                </Card>
            </Col>
        </Row>
    )
}


const LoginPage = () => {
    const router = useRouter();
    const { response, error, loading } = useAPI(() => setup.getState());
    const loginConfig = response?.login_config;
    const requiresSetup = !loading && !error && !!response && (response.state !== SETUP_STATE_INITIALIZED || response.comm_prefs_missing === true);
    const redirected = router.query.redirected;
    useEffect(() => {
        if (loading || error) return;
        if (requiresSetup) {
            router.push({pathname: '/setup', query: router.query});
        } else if (redirected) {
            if (loginConfig?.login_url) window.location.assign(loginConfig.login_url);
            else {
                const query = {...router.query};
                delete query.redirected;
                router.push({pathname: '/auth/login', query});
            }
        }
    }, [loading, error, requiresSetup, loginConfig?.login_url, router.fullPath]);
    if (loading || requiresSetup || (!error && redirected && loginConfig?.login_url)) {
        return null;
    }

    if (error) return <Row><Col md={{offset: 4, span: 4}}><AlertError error={error}/></Col></Row>;
    if (loginConfig?.snapshot_identity) return <SharedLogin/>;
    return (
        <LoginForm loginConfig={loginConfig}/>
    );
};

export default LoginPage;
