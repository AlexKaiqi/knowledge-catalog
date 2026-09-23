import React, { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Alert, Badge, Button, Container, ListGroup, Spinner, Table } from 'react-bootstrap';
import { KCError, kc, DatasetSummary } from '../../lib/api/kc';

// Dataset inventory. Data comes from the closed catalog state surface; the
// console never widens it with raw repository listing.

// react-bootstrap v2's polymorphic `as` rejects react-router's
// ForwardRefExoticComponent outright (ElementType included). Alias Button
// through the exact intersection we need — anchor props plus variant/size —
// instead of an `as any` cast; runtime is unchanged (Button renders Link).
const ButtonLink = Button as unknown as React.FC<
    React.ComponentProps<typeof Link> & { variant?: string; size?: 'sm' | 'lg' }
>;

const DatasetsPage: React.FC = () => {
    const [catalogs, setCatalogs] = useState<string[]>([]);
    const [catalogId, setCatalogId] = useState('');
    const [datasets, setDatasets] = useState<DatasetSummary[] | null>(null);
    const [error, setError] = useState<KCError | null>(null);

    useEffect(() => {
        kc.catalogs()
            .then((state) => {
                const ids = (state.catalogs ?? []).map((c) => c.id);
                setCatalogs(ids);
                setCatalogId(ids[0] ?? '');
            })
            .catch((err) => setError(err));
    }, []);

    useEffect(() => {
        if (!catalogId) return;
        setDatasets(null);
        kc.datasets(catalogId)
            .then((state) => setDatasets(state.datasets ?? []))
            .catch((err) => setError(err));
    }, [catalogId]);

    if (error) {
        return (
            <Container className="mt-4">
                <Alert variant="danger">
                    <strong>{error.code}</strong>
                    <div>{error.message}</div>
                </Alert>
            </Container>
        );
    }
    if (!catalogId) {
        return (
            <Container className="mt-4">
                <Spinner animation="border" /> 正在连接 KC Server…
            </Container>
        );
    }
    return (
        <Container className="mt-4">
            <div className="d-flex justify-content-between align-items-center mb-3">
                <h3 className="mb-0">Catalog {catalogId} 的 Dataset</h3>
                <ButtonLink to="/ui/datasets/new" variant="primary">
                    定义 / 发布新 Dataset
                </ButtonLink>
            </div>
            <ListGroup className="mb-3" horizontal="sm">
                {catalogs.map((id) => (
                    <ListGroup.Item key={id} active={id === catalogId} action onClick={() => setCatalogId(id)}>
                        {id}
                    </ListGroup.Item>
                ))}
            </ListGroup>
            {datasets === null ? (
                <Spinner animation="border" />
            ) : datasets.length === 0 ? (
                <Alert variant="light">这个 Catalog 还没有已定义的 Dataset。</Alert>
            ) : (
                <Table striped hover responsive>
                    <thead>
                        <tr>
                            <th>Dataset</th>
                            <th>服务版</th>
                            <th>来源仓</th>
                            <th>条目</th>
                            <th>状态</th>
                        </tr>
                    </thead>
                    <tbody>
                        {datasets.map((d) => (
                            <tr key={d.id}>
                                <td>
                                    <Link to={`/ui/datasets/${encodeURIComponent(d.id)}`}>{d.id}</Link>
                                </td>
                                <td>v{d.revision}</td>
                                <td>{d.repositories.join(', ')}</td>
                                <td>{d.itemCount}</td>
                                <td>{d.retired ? <Badge bg="secondary">已退役</Badge> : <Badge bg="success">服务中</Badge>}</td>
                            </tr>
                        ))}
                    </tbody>
                </Table>
            )}
        </Container>
    );
};

export default DatasetsPage;
