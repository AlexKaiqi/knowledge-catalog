import React, { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Alert, Badge, Button, Container, Form, Spinner, Table } from 'react-bootstrap';
import { kc, DatasetSummary, uiPath } from '../../lib/api/kc';
import ErrorNotice from '../../lib/components/kcError';

const DatasetsPage: React.FC = () => {
    const { catalogId = '' } = useParams();
    const navigate = useNavigate();
    const [catalogs, setCatalogs] = useState<string[] | null>(null);
    const [datasets, setDatasets] = useState<DatasetSummary[] | null>(null);
    const [error, setError] = useState<Error | null>(null);
    const [attempt, setAttempt] = useState(0);
    useEffect(() => {
        const controller = new AbortController();
        setError(null); setDatasets(null); setCatalogs(null);
        (async () => {
            try {
                const listed = await kc.catalogs(controller.signal);
                if (controller.signal.aborted) return;
                const ids = listed.catalogs.map((catalog) => catalog.id);
                setCatalogs(ids);
                if (catalogId && !ids.includes(catalogId)) throw new Error('这个 Catalog 当前不可见，请重新选择。');
                if (catalogId) {
                    const data = await kc.datasets(catalogId, controller.signal);
                    if (!controller.signal.aborted) setDatasets(data.datasets);
                }
            } catch (err) { if (!controller.signal.aborted) setError(err as Error); }
        })();
        return () => controller.abort();
    }, [catalogId, attempt]);
    return <Container fluid="xl" className="mt-5 pb-4">
        <div className="d-flex justify-content-between align-items-center mb-3">
            <h1 className="h3 mb-0">Datasets</h1>
            {catalogId && catalogs?.includes(catalogId) && <Link className="btn btn-primary" to={uiPath.create(catalogId)}>创建 Dataset</Link>}
        </div>
        <Form.Group controlId="kc-catalog" className="mb-4" style={{ maxWidth: 480 }}>
            <Form.Label>Catalog</Form.Label>
            <Form.Select disabled={catalogs === null} value={catalogId} onChange={(event) => navigate(event.target.value ? uiPath.catalog(event.target.value) : '/datasets')}>
                <option value="">请选择 Catalog</option>
                {catalogs?.map((id) => <option key={id} value={id}>{id}</option>)}
            </Form.Select>
        </Form.Group>
        {error && <><ErrorNotice error={error} /><Button variant="outline-primary" onClick={() => setAttempt((value) => value + 1)}>重试</Button></>}
        {!error && catalogs === null && <Spinner animation="border" />}
        {!error && catalogs?.length === 0 && <Alert variant="info">当前账号没有可见的 Catalog，请联系管理员确认访问权限。</Alert>}
        {!error && catalogs && catalogs.length > 0 && !catalogId && <Alert variant="light">选择一个 Catalog 查看并管理其中的 Dataset。</Alert>}
        {!error && catalogId && datasets === null && catalogs !== null && <Spinner animation="border" />}
        {!error && datasets?.length === 0 && <Alert variant="light">这个 Catalog 还没有 Dataset，可以创建第一份文件交付。</Alert>}
        {!error && datasets && datasets.length > 0 && <Table striped hover responsive>
            <thead><tr><th>Dataset</th><th>当前版本</th><th>来源仓</th><th>条目</th><th>状态</th></tr></thead>
            <tbody>{datasets.map((dataset) => <tr key={dataset.id}>
                <td><Link to={uiPath.dataset(catalogId, dataset.id)}>{dataset.id}</Link></td>
                <td>v{dataset.revision}</td><td>{dataset.repositories.join(', ')}</td><td>{dataset.itemCount}</td>
                <td><Badge bg={dataset.retired ? 'secondary' : 'success'}>{dataset.retired ? '已退役' : '服务中'}</Badge></td>
            </tr>)}</tbody>
        </Table>}
    </Container>;
};
export default DatasetsPage;
