import React, { useEffect, useRef, useState } from 'react';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { Alert, Badge, Breadcrumb, Button, Card, Container, Form, Spinner, Table, Tab, Tabs } from 'react-bootstrap';
import { DatasetDetail, kc, MountsResponse, uiPath } from '../../lib/api/kc';
import ErrorNotice from '../../lib/components/kcError';
import DeliveryTree from './deliveryTree';
import Compare from './compare';

const DatasetPage: React.FC = () => {
    const { catalogId = '', datasetId = '' } = useParams();
    const [query, setQuery] = useSearchParams();
    const rawRevision = query.get('revision') ?? '';
    const [detail, setDetail] = useState<DatasetDetail | null>(null);
    const [opened, setOpened] = useState<MountsResponse | null>(null);
    const [error, setError] = useState<Error | null>(null);
    const [readError, setReadError] = useState<Error | null>(null);
    const [confirming, setConfirming] = useState(false);
    const [busy, setBusy] = useState(false);
    const [attempt, setAttempt] = useState(0);
    const [history, setHistory] = useState<{basis: number; versions: number[]; continuation?: string} | null>(null);
    const [historyError, setHistoryError] = useState<Error | null>(null);
    const [historyBusy, setHistoryBusy] = useState(false);
    const pending = useRef<AbortController>();
    useEffect(() => { setHistory(null); }, [catalogId, datasetId]);
    useEffect(() => {
        const controller = new AbortController();
        pending.current = controller;
        setBusy(false);
        setHistoryBusy(false); setHistoryError(null);
        setDetail(null); setOpened(null); setError(null); setReadError(null); setConfirming(false);
        (async () => {
            try {
                const revision = rawRevision ? Number(rawRevision) : undefined;
                if (revision !== undefined && (!Number.isSafeInteger(revision) || revision <= 0)) throw new Error('版本号必须是正整数。');
                const next = await kc.dataset(catalogId, datasetId, revision, controller.signal);
                if (controller.signal.aborted) return;
                setDetail(next);
                setHistory(before => before?.basis === next.servingRevision ? {...before, versions: [...new Set([...before.versions, next.revision])].sort((a, b) => a - b)} :
                    {basis: next.servingRevision, versions: [...new Set([...next.versions, next.revision])].sort((a, b) => a - b), continuation: next.continuation});
                if (!next.retired) {
                    try {
                        // Coordinates come from accepted metadata; the File Gateway
                        // reconstructs and validates the published scope and current access.
                        const repositories = Object.fromEntries(next.items.map((item) => [item.repository, item.commit]));
                        const pin = { setId: datasetId, revision: next.revision, repositories, items: next.items };
                        const result = await kc.mounts(catalogId, datasetId, pin, controller.signal);
                        if (!controller.signal.aborted) setOpened(result);
                    } catch (err) { if (!controller.signal.aborted) setReadError(err as Error); }
                }
            } catch (err) { if (!controller.signal.aborted) setError(err as Error); }
        })();
        return () => controller.abort();
    }, [catalogId, datasetId, rawRevision, attempt]);
    const moreHistory = async (): Promise<void> => {
        if (!history?.continuation || !detail) return;
        const signal = pending.current?.signal;
        setHistoryBusy(true); setHistoryError(null);
        try {
            const page = await kc.dataset(catalogId, datasetId, detail.revision, signal, {continuation: history.continuation});
            if (!signal?.aborted) setHistory(before => before?.basis === page.servingRevision ?
                {basis: before.basis, versions: [...new Set([...before.versions, ...page.versions])].sort((a, b) => a - b), continuation: page.continuation} : before);
        } catch (err) { if (!signal?.aborted) setHistoryError(err as Error); }
        finally { if (!signal?.aborted) setHistoryBusy(false); }
    };
    const versions = history?.versions ?? detail?.versions ?? [];
    const retire = async (): Promise<void> => {
        const signal = pending.current?.signal;
        setBusy(true); setError(null);
        try {
            await kc.retire(catalogId, datasetId, signal);
            if (!signal?.aborted) { setConfirming(false); setAttempt((value) => value + 1); }
        } catch (err) { if (!signal?.aborted) setError(err as Error); }
        finally { if (!signal?.aborted) setBusy(false); }
    };
    return <Container fluid="xl" className="mt-5 pb-4">
        <Breadcrumb><Breadcrumb.Item linkAs={Link} linkProps={{ to: uiPath.catalog(catalogId) }}>Catalog {catalogId}</Breadcrumb.Item><Breadcrumb.Item active>{datasetId}</Breadcrumb.Item></Breadcrumb>
        {error && <ErrorNotice error={error} />}
        {!detail ? <>{!error && <Spinner animation="border" />}<Button variant="outline-primary" className="ms-2" onClick={() => setAttempt((value) => value + 1)}>重新加载</Button></> : <>
            <div className="d-flex justify-content-between align-items-start gap-3 flex-wrap mb-3">
                <h1 className="h3">{detail.id} <Badge bg="info">v{detail.revision}</Badge> {detail.retired && <Badge bg="secondary">已退役</Badge>}</h1>
                {!detail.retired && <Link className="btn btn-primary" to={`${uiPath.dataset(catalogId, datasetId)}/edit`}>编辑当前版并发布新版</Link>}
            </div>
            <div className="d-flex align-items-end gap-3 mb-4 flex-wrap">
                <Form.Group controlId="dataset-version"><Form.Label>已发布版本</Form.Label>
                    <Form.Select value={detail.revision} onChange={(event) => setQuery({ revision: event.target.value })}>
                        {versions.map((revision) => <option key={revision} value={revision}>v{revision}{revision === detail.servingRevision ? '（当前版）' : ''}</option>)}
                    </Form.Select></Form.Group>
                <Button variant="outline-secondary" onClick={() => { setHistory(null); setQuery({}); setAttempt((value) => value + 1); }}>打开当前版本</Button>
            </div>
            <Tabs defaultActiveKey="data" className="mb-4" mountOnEnter unmountOnExit>
            <Tab eventKey="data" title="Data">
            <Card className="mb-4"><Card.Body><Card.Title>来源与交付范围</Card.Title>
                <Table responsive size="sm"><thead><tr><th>交付路径</th><th>来源仓</th><th>来源路径</th><th>固定提交</th><th>形态</th></tr></thead>
                    <tbody>{detail.items.map((item) => <tr key={item.target}><td>{item.target || '/'}</td><td>{item.repository}</td><td>{item.file ?? item.prefix ?? '/'}</td><td><code title={item.commit}>{item.commit.slice(0, 12)}</code></td><td>{item.kind === 'file' ? '文件' : '目录'}</td></tr>)}</tbody>
                </Table>
            </Card.Body></Card>
            {detail.retired ? <Alert variant="secondary">此 Dataset 已退役。发布记录与来源保留，后续文件读取和再次发布按当前生命周期拒绝。</Alert> : <>
                {readError && <><ErrorNotice error={readError} /><Button variant="outline-primary" className="mb-3" onClick={() => setAttempt((value) => value + 1)}>重试打开交付范围</Button></>}
                {!opened && !readError && <Spinner animation="border" />}
                {opened && <DeliveryTree key={`${opened.pin.revision}:${opened.pin.pinId}`} catalog={catalogId} dataset={datasetId} pin={opened.pin} />}
                {opened && <details className="mb-3"><summary>保存本次版本依据</summary><p className="text-muted mt-2">记录仅包含版本、来源和范围；后续使用仍检查当前权限。</p><pre style={{ maxHeight: 280, overflow: 'auto' }}>{JSON.stringify(opened.pin, null, 2)}</pre></details>}
                <Alert variant="light">交付到本地目录：先在 KC 客户端选择此 Catalog，再执行 Dataset clone（采用执行时的当前版本）。</Alert>
                {confirming ? <Alert variant="warning"><p>确认退役当前 Dataset？发布记录和来源仍保留，后续消费停止。</p>
                    <Button variant="danger" disabled={busy} onClick={() => { void retire(); }}>{busy ? '正在退役…' : '确认退役'}</Button>
                    <Button variant="link" disabled={busy} onClick={() => setConfirming(false)}>取消</Button></Alert> :
                    <Button variant="outline-danger" onClick={() => setConfirming(true)}>退役 Dataset</Button>}
            </>}
            </Tab>
            <Tab eventKey="history" title="History"><Table hover><thead><tr><th>发布版本</th><th>状态</th><th/></tr></thead><tbody>
                {[...versions].reverse().map(version => <tr key={version}><td>v{version}</td><td>{version === detail.servingRevision ? '当前版' : '历史版'}</td><td>
                    <Button variant="link" onClick={() => setQuery({revision: String(version)})}>打开 v{version}</Button></td></tr>)}
            </tbody></Table>
                {historyError && <ErrorNotice error={historyError}/>}
                {history?.continuation && <Button className="mb-3" variant="outline-primary" disabled={historyBusy} onClick={() => {void moreHistory();}}>{historyBusy ? '正在加载历史…' : '加载更多历史版本'}</Button>}
                <p className="text-muted">历史按页读取，比较与版本选择只展示已加载的发布记录；当前退役和授权判定继续适用于历史读取。Dataset 有新发布导致分页失效时，请打开当前版本重新加载。</p></Tab>
            <Tab eventKey="compare" title="Compare"><Compare key={`${catalogId}:${datasetId}:${detail.revision}`} catalog={catalogId} dataset={datasetId} detail={{...detail, versions}}/></Tab>
            <Tab eventKey="definition" title="JSON"><pre className="text-wrap">{JSON.stringify({dataset: detail.id, revision: detail.revision, sources: detail.sources}, null, 2)}</pre></Tab>
            </Tabs>
        </>}
    </Container>;
};
export default DatasetPage;
