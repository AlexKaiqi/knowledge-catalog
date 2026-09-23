import React from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { Alert, Navbar, Nav, Container } from 'react-bootstrap';
import { kcSession } from '../lib/api/kc';
import DatasetsPage from '../pages/kc/datasets';
import DatasetPage from '../pages/kc/dataset';
import PublishPage from '../pages/kc/publish';

// KC console routes. lakeFS's own pages stay vendored but are not routed.

const MissingCatalogNotice: React.FC = () => (
    <Container className="mt-4">
        <Alert variant="warning">
            KC Console 需要知道 Server 地址与身份。CLI 里 <code>kc serve</code> 启动的部署默认同源；
            非同源访问时请在 <code>localStorage</code> 配置 <code>kc.ui.token</code> 或
            <code> kc.ui.as</code>（--auth local 部署）。
        </Alert>
    </Container>
);

const KCNav: React.FC = () => {
    const as = kcSession.as();
    const token = kcSession.token();
    return (
        <Navbar bg="dark" variant="dark" expand="sm">
            <Container fluid>
                <Navbar.Brand href="/ui/">KC Dataset 控制台</Navbar.Brand>
                <Nav className="ms-auto">
                    <Nav.Item className="navbar-text text-light small">
                        {token ? 'Authorization 会话已配置' : as ? `local 身份：${as}` : '未配置身份'}
                    </Nav.Item>
                </Nav>
            </Container>
        </Navbar>
    );
};

const KCApp: React.FC = () => (
    <BrowserRouter>
        <KCNav />
        <Routes>
            <Route path="/" element={<Navigate to="/ui/" replace />} />
            <Route path="/ui" element={<Navigate to="/ui/" replace />} />
            <Route path="/ui/" element={<DatasetsPage />} />
            <Route path="/ui/datasets/new" element={<PublishPage />} />
            <Route path="/ui/datasets/:datasetId" element={<DatasetPage />} />
            <Route path="*" element={<MissingCatalogNotice />} />
        </Routes>
    </BrowserRouter>
);

export default KCApp;
