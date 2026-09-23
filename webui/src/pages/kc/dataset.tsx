import React, { useCallback, useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import {
    Alert, Badge, Breadcrumb, Button, Card, Container, ListGroup,
    Spinner, Table,
} from 'react-bootstrap';
import { DirectoryEntry, KCError, kc, DatasetDetail, DirectoryResponse, MountsResponse } from '../../lib/api/kc';

// One Dataset: composition truth (mounts + per-file entries), the delivered
// tree, and the clone command a consumer can copy. The delivered tree is
// browsed through dataset-files/v1 tree:list with Path addressing — the same
// surface kcfs and kc dataset clone consume. Authorization is evaluated per
// request by the server; the console holds no dataset capabilities of its own.

const DatasetPage: React.FC = () => {
    const { datasetId = '' } = useParams();
    const [catalogId, setCatalogId] = useState('');
    const [detail, setDetail] = useState<DatasetDetail | null>(null);
    const [opened, setOpened] = useState<MountsResponse | null>(null);
    const [dir, setDir] = useState<string>('');
    const [listing, setListing] = useState<DirectoryResponse | null>(null);
    const [preview, setPreview] = useState<{ path: string; text: string } | null>(null);
    const [error, setError] = useState<KCError | null>(null);
    const [confirming, setConfirming] = useState(false);
    const [retireBusy, setRetireBusy] = useState(false);

    useEffect(() => {
        setError(null);
        setDetail(null);
        setOpened(null);
        setListing(null);
        setDir('');
        setPreview(null);
        (async () => {
            try {
                const catalogs = await kc.catalogs();
                const catalog = (catalogs.catalogs ?? []).map((c) => c.id)[0] ?? '';
                if (!catalog) throw new KCError(404, { code: 'CATALOG_UNRESOLVED', message: '没有可见的 Catalog；先用 kc catalog use 选择。' });
                setCatalogId(catalog);
                const next = await kc.dataset(catalog, datasetId);
                setDetail(next);
                // U4 lifecycle: a retired dataset refuses further consumption,
                // while the publish record survives — so skip the mounts open
                // instead of turning lifecycle rejection into a page error.
                if (!next.retired) setOpened(await kc.mounts(catalog, datasetId));
            } catch (err) {
                setError(err as KCError);
            }
        })();
    }, [datasetId]);

    useEffect(() => {
        if (!catalogId || !opened) return;
        setListing(null);
        (async () => {
            try {
                setListing(await kc.tree(catalogId, datasetId, opened.pin, dir));
            } catch (err) {
                setError(err as KCError);
            }
        })();
    }, [catalogId, opened, datasetId, dir]);

    const openFile = useCallback(async (name: string) => {
        if (!catalogId || !opened) return;
        const path = dir ? `${dir}/${name}` : name;
        try {
            const bytes = await kc.readFile(catalogId, datasetId, opened.pin, path);
            setPreview({ path, text: new TextDecoder().decode(bytes) });
        } catch (err) {
            setPreview({ path, text: `${(err as KCError).code}: ${(err as KCError).message}` });
        }
    }, [catalogId, opened, datasetId, dir]);

    // U4 lifecycle: retire stops future consumption but never rewrites the
    // publish record or the source repository. The server enforces
    // dataset.manage; this control only surfaces the closed route.
    const retire = async (): Promise<void> => {
        if (!catalogId || detail?.retired) return;
        setRetireBusy(true);
        try {
            await kc.retire(catalogId, datasetId);
            setConfirming(false);
            const next = await kc.dataset(catalogId, datasetId);
            setDetail(next);
            setOpened(null);
            setListing(null);
            setPreview(null);
            setDir('');
        } catch (err) {
            setError(err as KCError);
        } finally {
            setRetireBusy(false);
        }
    };

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
    if (!detail) {
        return <Container className="mt-4"><Spinner animation="border" /></Container>;
    }
    if (detail.retired) {
        // U4 lifecycle: the publish record survives retirement, but every
        // consumption surface below needs an open serving version, so the
        // retired page renders header-only instead of failing per card.
        return (
            <Container className="mt-4">
                <Breadcrumb>
                    <Breadcrumb.Item linkAs={Link} linkProps={{ to: '/ui/' }}>Datasets</Breadcrumb.Item>
                    <Breadcrumb.Item active>{detail.id}</Breadcrumb.Item>
                </Breadcrumb>
                <h3>
                    {detail.id}{' '}
                    <Badge bg="info">v{detail.revision}</Badge>{' '}
                    <Badge bg="secondary">已退役</Badge>
                </h3>
                <Alert variant="secondary">
                    <strong>已退役。</strong>发布记录与来源内容保留；进一步的消费（mounts、目录枚举、读取、
                    <code>kc dataset clone</code>）按当前生命周期拒绝。要以新版本继续交付，用同一 dataset id 发布下一个 revision。
                </Alert>
            </Container>
        );
    }
    if (!opened) {
        return <Container className="mt-4"><Spinner animation="border" /></Container>;
    }

    const crumb = dir ? dir.split('/') : [];
    const fileItems = (opened.pin.items ?? []).filter((i) => i.kind === 'file');
    const up = (): void => setDir(crumb.slice(0, -1).join('/'));

    return (
        <Container className="mt-4">
            <Breadcrumb>
                <Breadcrumb.Item linkAs={Link} linkProps={{ to: '/ui/' }}>Datasets</Breadcrumb.Item>
                <Breadcrumb.Item active>{detail.id}</Breadcrumb.Item>
            </Breadcrumb>
            <div className="d-flex justify-content-between align-items-center mb-3 flex-wrap gap-2">
                <h3 className="mb-0">
                    {detail.id}{' '}
                    <Badge bg="info">v{detail.revision}</Badge>
                </h3>
                {confirming ? (
                    <span className="d-inline-flex align-items-center gap-2">
                        <span className="text-muted small">退役后不再可消费；发布记录与来源内容保留。</span>
                        <Button size="sm" variant="danger" disabled={retireBusy}
                            onClick={() => { void retire(); }}>
                            {retireBusy ? '退役中…' : '确认退役'}
                        </Button>
                        <Button size="sm" variant="link" disabled={retireBusy}
                            onClick={() => setConfirming(false)}>取消</Button>
                    </span>
                ) : (
                    <Button size="sm" variant="outline-danger" onClick={() => setConfirming(true)}>
                        退役此 Dataset
                    </Button>
                )}
            </div>
            <Card className="mb-3">
                <Card.Body>
                    <Card.Title>组合真相（当前服务版 {opened.pin.ref || `v${opened.pin.revision}`}）</Card.Title>
                    <Table size="sm" className="mb-0">
                        <thead>
                            <tr><th>交付路径</th><th>来源仓</th><th>冻结 commit</th><th>形态</th></tr>
                        </thead>
                        <tbody>
                            {opened.mounts.map((m) => (
                                <tr key={`mount:${m.path}`}>
                                    <td>{m.path || '/'}</td>
                                    <td>{m.repository}</td>
                                    <td><code>{m.commit.slice(0, 12)}</code></td>
                                    <td><Badge bg="primary">目录挂载{m.subPath ? ` ← ${m.subPath}` : ''}</Badge></td>
                                </tr>
                            ))}
                            {fileItems.map((item) => (
                                <tr key={`file:${item.target}`}>
                                    <td>{item.target}</td>
                                    <td>{item.repository}</td>
                                    <td><code>{item.commit.slice(0, 12)}</code></td>
                                    <td><Badge bg="warning" text="dark">逐文件 ← {item.file}</Badge></td>
                                </tr>
                            ))}
                        </tbody>
                    </Table>
                </Card.Body>
            </Card>
            <Card className="mb-3">
                <Card.Body>
                    <Card.Title>交付目录树</Card.Title>
                    <div className="mb-2 d-flex align-items-center gap-2">
                        <Button size="sm" variant="outline-secondary" disabled={!dir} onClick={up}>上一级</Button>
                        <code>/{dir ? ` ${crumb.join(' / ')}` : ''}</code>
                    </div>
                    {listing === null ? (
                        <Spinner animation="border" size="sm" />
                    ) : (
                        <ListGroup variant="flush">
                            {listing.entries.map((entry: DirectoryEntry) => {
                                const path = dir ? `${dir}/${entry.name}` : entry.name;
                                return (
                                    <ListGroup.Item key={entry.name} action
                                        onClick={() => (entry.kind === 'directory' ? setDir(path) : openFile(entry.name))}>
                                        {entry.kind === 'directory' ? '📁 ' : '📄 '}
                                        {entry.name}
                                    </ListGroup.Item>
                                );
                            })}
                            {listing.entries.length === 0 && <ListGroup.Item>（空目录）</ListGroup.Item>}
                        </ListGroup>
                    )}
                </Card.Body>
            </Card>
            {preview && (
                <Card className="mb-3">
                    <Card.Header className="d-flex justify-content-between">
                        <code>{preview.path}</code>
                        <Button size="sm" variant="outline-dark" onClick={() => setPreview(null)}>关闭</Button>
                    </Card.Header>
                    <Card.Body>
                        <pre className="mb-0" style={{ maxHeight: 320, overflow: 'auto' }}>{preview.text}</pre>
                    </Card.Body>
                </Card>
            )}
            <Alert variant="light">
                消费：把这份交付目录树物化到本地目录（当前服务版，不覆盖已有文件）：
                <br />
                <code>kc dataset clone {detail.id} ./&lt;目标目录&gt;</code>
            </Alert>
            <div className="text-muted small mb-4">
                预览按需逐文件读取，不做整树复制；授权（file.read）与 CLI、kcfs 完全同源，由服务端逐请求评价。
            </div>
        </Container>
    );
};

export default DatasetPage;
