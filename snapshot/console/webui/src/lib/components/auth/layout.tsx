import React, {useCallback, useEffect, useState} from "react";
import {Outlet, useOutletContext} from "react-router-dom";
import Container from "react-bootstrap/Container";
import Row from "react-bootstrap/Row";
import Col from "react-bootstrap/Col";
import Nav from "react-bootstrap/Nav";
import Card from "react-bootstrap/Card";

import {Link} from "../nav";
import {useLoginConfigContext} from "../../hooks/conf";
import {useLayoutOutletContext} from "../layout";
import Alert from "react-bootstrap/Alert";
import {InfoIcon} from "@primer/octicons-react";
import {useAuth} from '../../hooks/kcAuth';
import KCAuthGate from '../../../extendable/kcAuthGate';
import {useRouter} from '../../hooks/router';
import {useMatch} from 'react-router-dom';

type AuthOutletContext = [(tab: string) => void];

const rbacDismissedKey = "lakefs:ui:acl:dismissRBACAlert";
export const AuthLayout = ({kcPage = false}: {kcPage?: boolean}) => {
    const [showRBACAlert, setShowRBACAlert] = useState(!window.localStorage.getItem(rbacDismissedKey));
    const [activeTab, setActiveTab] = useState("credentials");  
    const {RBAC: rbac, snapshot_identity: external, provider_url: providerURL} = useLoginConfigContext();
    const [setIsLogged] = useLayoutOutletContext();
    const router = useRouter();
    const permissionsTab = useMatch('/auth/permissions');
    const grantsTab = useMatch('/auth/grants');
    const onVerified = useCallback(() => setIsLogged(true), [setIsLogged]);
    useEffect(() => {
        setIsLogged(!kcPage);
        if (external && !kcPage) setActiveTab(router.route.split('/')[2] || 'credentials');
    }, [setIsLogged, kcPage, external, router.route]);
    return (
        <Container fluid="xl">
            <Row className="mt-3" >
                <div>
                    {!kcPage && rbac === 'simplified' && showRBACAlert &&
                    <Alert variant="info" title="rbac CTA" dismissible onClose={() => {
                        window.localStorage.setItem(rbacDismissedKey, "true");
                        setShowRBACAlert(false);
                    }}><InfoIcon/>{" "}Enhance Your Security with {" "}<Alert.Link href={"https://docs.lakefs.io/reference/security/rbac.html"}>Role-Based Access Control</Alert.Link>{" "}
                        – Available on <Alert.Link href={"https://lakefs.cloud/register"}>lakeFS Cloud</Alert.Link> and <Alert.Link href={"https://docs.lakefs.io/understand/enterprise/"}>lakeFS Enterprise</Alert.Link>!</Alert>
                    }
                </div>
                <Col md={{span: 3}}>
                        <Card>
                            <Card.Header>
                                <Card.Title>Access Control</Card.Title>
                            </Card.Header>
                            <Card.Body>
                                <p>Knowledge Catalog</p>
                                <Nav variant="pills" className="flex-column">
                                    <Link component={Nav.Link} href="/auth/permissions" active={!!permissionsTab}>我的权限</Link>
                                    <Link component={Nav.Link} href="/auth/grants" active={!!grantsTab}>授权管理</Link>
                                </Nav>
                                <hr/>
                                <p>{external ? '认证系统' : 'lakeFS'}</p>
                                <Nav variant="pills" className="flex-column">
                                    <Link component={Nav.Link} href="/auth/credentials" active={!kcPage && activeTab === 'credentials'} >
                                        My Credentials
                                    </Link>
                                </Nav>

                                <hr/>

                                <Nav variant="pills" className="flex-column">
                                    <Link component={Nav.Link} href="/auth/users" active={!kcPage && activeTab === 'users'} >
                                        Users
                                    </Link>

                                    <Link component={Nav.Link} href="/auth/groups" active={!kcPage && activeTab === 'groups'} >
                                        Groups
                                    </Link>
                                    {rbac !== 'simplified' && rbac !== 'none' &&
                                        <Link component={Nav.Link} href="/auth/policies" active={!kcPage && activeTab === 'policies'} >
                                            Policies
                                        </Link>}
                                </Nav>
                            </Card.Body>
                        </Card>
                    </Col>
                <Col md={{span: 9}}>
                    {kcPage ? <KCAuthGate key={router.route} onVerified={onVerified}><Outlet context={[setActiveTab] satisfies AuthOutletContext}/></KCAuthGate> : <>
                        {rbac === 'none' && <Alert variant="info">当前 lakeFS 后端为单管理员模式，用户列表可查看；凭证管理、组与策略管理接口未开放。接入支持这些接口的身份服务后，可继续使用原管理页面。</Alert>}
                        {external ? <KCAuthGate onVerified={onVerified}><ProviderAccounts providerURL={providerURL}/></KCAuthGate> : <Outlet context={[setActiveTab] satisfies AuthOutletContext}/>}
                    </>
                    }
                </Col>
            </Row>
        </Container>
    )
};

function ProviderAccounts({providerURL}: {providerURL?: string}) {
    const {user} = useAuth();
    return <Card><Card.Header>账号与凭据</Card.Header><Card.Body>
        <Row className="mb-3"><Col sm={3} className="text-muted">当前主体</Col><Col>{user?.principal}</Col></Row>
        <Row className="mb-3"><Col sm={3} className="text-muted">认证来源</Col><Col>{user?.mode}</Col></Row>
        <p>用户、组与访问凭据由现有认证系统管理。Repo、Dataset 的规则在“我的权限”和“授权管理”查看与维护。</p>
        {providerURL && <a className="btn btn-primary" href={providerURL} target="_blank" rel="noreferrer">打开认证系统</a>}
    </Card.Body></Card>;
}

export function useAuthOutletContext() {
    return useOutletContext<AuthOutletContext>();
}
