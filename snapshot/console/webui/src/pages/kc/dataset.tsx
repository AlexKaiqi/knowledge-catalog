import React, { useEffect, useRef, useState } from 'react';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { Alert, Badge, Breadcrumb, Button, ButtonToolbar, Card, Container, Form, Spinner, Stack, Table, Tab, Tabs } from 'react-bootstrap';
import { DatasetDetail, kc, MountsResponse, ResolvedPin, uiPath } from '../../lib/api/kc';
import ErrorNotice from '../../lib/components/kcError';
import DeliveryTree from './deliveryTree';
import Compare from './compare';
import {ActionsBar} from '../../lib/components/controls';
import {CodeIcon, DatabaseIcon, GitCommitIcon, GitCompareIcon, PencilIcon} from '@primer/octicons-react';
import {MetadataView} from '../../lib/components/resourceMetadata';

const DatasetPage: React.FC = () => {
    const { datasetId = '' } = useParams();
    const [query, setQuery] = useSearchParams();
    const rawRevision = query.get('revision') ?? '';
    const [detail, setDetail] = useState<DatasetDetail | null>(null);
    const [resolved, setResolved] = useState<ResolvedPin | null>(null);
    const [opened, setOpened] = useState<MountsResponse | null>(null);
    const [error, setError] = useState<Error | null>(null);
    const [detailError, setDetailError] = useState<Error | null>(null);
    const [detailLoading, setDetailLoading] = useState(false);
    const [readError, setReadError] = useState<Error | null>(null);
    const [confirming, setConfirming] = useState(false);
    const [busy, setBusy] = useState(false);
    const [attempt, setAttempt] = useState(0);
    const [history, setHistory] = useState<{basis: number; versions: number[]; continuation?: string} | null>(null);
    const [historyError, setHistoryError] = useState<Error | null>(null);
    const [historyBusy, setHistoryBusy] = useState(false);
    const pending = useRef<AbortController>();
    useEffect(() => { setHistory(null); }, [datasetId]);
    useEffect(() => {
        const controller = new AbortController();
        pending.current = controller;
        setBusy(false);
        setHistoryBusy(false); setHistoryError(null);
        setDetail(null); setResolved(null); setOpened(null); setError(null); setDetailError(null); setReadError(null); setConfirming(false); setDetailLoading(true);
        let acceptedRevision: number | undefined;
        const observed: {revision?: number} = {};
        let detailRequest = 0;
        const loadDetail = async (revision?: number): Promise<void> => {
            const request = ++detailRequest;
            setDetailLoading(true); setDetailError(null);
            try {
                const next = await kc.dataset(datasetId, revision, controller.signal);
                if (controller.signal.aborted || request !== detailRequest) return;
                if (acceptedRevision !== undefined && next.revision !== acceptedRevision) {
                    if (revision === acceptedRevision) throw new Error('管理详情返回了不同的发布版本，请重新加载。');
                    void loadDetail(acceptedRevision); return;
                }
                observed.revision = next.revision;
                setDetail(next);
                setHistory(before => before?.basis === next.servingRevision ? {...before, versions: [...new Set([...before.versions, next.revision])].sort((a, b) => a - b)} :
                    {basis: next.servingRevision, versions: [...new Set([...next.versions, next.revision])].sort((a, b) => a - b), continuation: next.continuation});
            } catch (err) { if (!controller.signal.aborted && request === detailRequest) setDetailError(err as Error); }
            finally { if (!controller.signal.aborted && request === detailRequest) setDetailLoading(false); }
        };
        (async () => {
            const revision = rawRevision ? Number(rawRevision) : undefined;
            if (revision !== undefined && (!Number.isSafeInteger(revision) || revision <= 0)) {
                setError(new Error('版本号必须是正整数。')); setDetailLoading(false); return;
            }
            void loadDetail(revision);
            let pin: ResolvedPin;
            try {
                pin = await kc.resolve(datasetId, revision, controller.signal);
            } catch (err) {
                if (!controller.signal.aborted) setReadError(err as Error);
                return;
            }
            if (controller.signal.aborted) return;
            acceptedRevision = pin.revision;
            setResolved(pin);
            // Management records describe this same accepted release, but their
            // availability never gates delivery or supplies its coordinates.
            if (observed.revision !== undefined && observed.revision !== pin.revision) { setDetail(null); void loadDetail(pin.revision); }
            try {
                const result = await kc.mounts(datasetId, pin, controller.signal);
                if (controller.signal.aborted) return;
                setOpened(result);
            } catch (err) { if (!controller.signal.aborted) setReadError(err as Error); }
        })();
        return () => controller.abort();
    }, [datasetId, rawRevision, attempt]);
    const moreHistory = async (): Promise<void> => {
        if (!history?.continuation || !detail) return;
        const signal = pending.current?.signal;
        setHistoryBusy(true); setHistoryError(null);
        try {
            const page = await kc.dataset(datasetId, detail.revision, signal, {continuation: history.continuation});
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
            await kc.retire(datasetId, signal);
            if (!signal?.aborted) { setConfirming(false); setAttempt((value) => value + 1); }
        } catch (err) { if (!signal?.aborted) setError(err as Error); }
        finally { if (!signal?.aborted) setBusy(false); }
    };
    const revision = resolved?.revision ?? detail?.revision;
    return <div className="pb-4">
        <Breadcrumb><Breadcrumb.Item linkAs={Link} linkProps={{to: '/datasets'}}>Datasets</Breadcrumb.Item><Breadcrumb.Item active>{datasetId}</Breadcrumb.Item></Breadcrumb>
        {error && <ErrorNotice error={error} />}
        {error && <Button variant="outline-primary" className="ms-2" onClick={() => setAttempt((value) => value + 1)}>重新加载</Button>}
        <Tabs activeKey={detail ? query.get('tab') ?? 'data' : 'data'} onSelect={tab => setQuery(before => {const next = new URLSearchParams(before); if (tab) next.set('tab', tab); return next;}, {replace: true})} mountOnEnter unmountOnExit>
            <Tab eventKey="data" title={<><DatabaseIcon/> Data</>}>
            <Container fluid="sm"><div className="mt-4">
            <ActionsBar><Stack direction="horizontal" gap={2} className="flex-wrap flex-grow-1">
                {detail && <Form.Group controlId="dataset-version"><Form.Label className="visually-hidden">已发布版本</Form.Label>
                    <Form.Select value={detail.revision} onChange={event => setQuery({revision: event.target.value})}>
                        {versions.map(revision => <option key={revision} value={revision}>v{revision}{revision === detail.servingRevision ? '（当前版）' : ''}</option>)}
                    </Form.Select></Form.Group>}
                <Button variant="outline-secondary" onClick={() => {setHistory(null); setQuery({}); setAttempt(value => value + 1);}}>打开当前版本</Button>
                {revision !== undefined && <Badge bg="light" text="body">{datasetId} · v{revision}</Badge>}{detail?.retired && <Badge bg="secondary">已退役</Badge>}
                <ButtonToolbar className="ms-auto">{detail && !detail.retired && <Link className="btn btn-success" to={`${uiPath.dataset(datasetId)}/edit`}><PencilIcon/> 编辑当前版并发布新版</Link>}</ButtonToolbar>
            </Stack></ActionsBar>
            {revision !== undefined && <h1 className="visually-hidden">{datasetId} v{revision} {detail?.retired ? '已退役' : ''}</h1>}
            {detail && <>
            <Card className="mb-4"><Card.Header>自定义元数据 · v{detail.revision}</Card.Header><Card.Body><MetadataView metadata={detail.metadata}/></Card.Body></Card>
            <Card className="mb-4"><Card.Header>来源与交付范围</Card.Header><Card.Body className="p-0">
                <Table responsive hover className="mb-0"><thead><tr><th>交付路径</th><th>来源仓</th><th>来源路径</th><th>固定提交</th><th>形态</th></tr></thead>
                    <tbody>{detail.items.map((item) => <tr key={item.target}><td>{item.target || '/'}</td><td>{item.repository}</td><td>{item.file ?? item.prefix ?? '/'}</td><td><code title={item.commit}>{item.commit.slice(0, 12)}</code></td><td>{item.kind === 'file' ? '文件' : '目录'}</td></tr>)}</tbody>
                </Table>
            </Card.Body></Card></>}
            {detail?.retired ? <Alert variant="secondary">此 Dataset 已退役。发布记录与来源保留，后续文件读取和再次发布按当前生命周期拒绝。</Alert> : <>
                {readError && <><ErrorNotice error={readError} /><Button variant="outline-primary" className="mb-3" onClick={() => setAttempt((value) => value + 1)}>重试打开交付范围</Button></>}
                {!opened && !readError && !error && <Spinner animation="border" />}
                {opened && <DeliveryTree key={`${opened.pin.revision}:${opened.pin.pinId}`} dataset={datasetId} pin={opened.pin} />}
                {opened && <details className="mb-3"><summary>保存本次版本依据</summary><p className="text-muted mt-2">记录仅包含版本、来源和范围；后续使用仍检查当前权限。</p><pre style={{ maxHeight: 280, overflow: 'auto' }}>{JSON.stringify(opened.pin, null, 2)}</pre></details>}
          <Alert variant="light">交付到本地目录：在 KC 客户端连接本服务后执行 kcfs mount（采用执行时的当前版本）。</Alert>
                {detail && (confirming ? <Alert variant="warning"><p>确认退役当前 Dataset？发布记录和来源仍保留，后续消费停止。</p>
                    <Button variant="danger" disabled={busy} onClick={() => { void retire(); }}>{busy ? '正在退役…' : '确认退役'}</Button>
                    <Button variant="link" disabled={busy} onClick={() => setConfirming(false)}>取消</Button></Alert> :
                    <Button variant="outline-danger" onClick={() => setConfirming(true)}>退役 Dataset</Button>)}
            </>}
            {detailLoading && <p className="text-muted mt-3"><Spinner animation="border" size="sm" className="me-2"/>正在读取管理记录…</p>}
            {detailError && <Card className="mt-3"><Card.Header>管理详情</Card.Header><Card.Body><ErrorNotice error={detailError}/>
                <Button variant="outline-primary" onClick={() => setAttempt(value => value + 1)}>重新加载</Button></Card.Body></Card>}
            </div></Container></Tab>
            {(detail || detailError) && <Tab eventKey="history" disabled={!detail} title={<><GitCommitIcon/> History</>}>{detail && <Container fluid="sm"><div className="mt-4"><Table hover><thead><tr><th>发布版本</th><th>状态</th><th/></tr></thead><tbody>
                {[...versions].reverse().map(version => <tr key={version}><td>v{version}</td><td>{version === detail.servingRevision ? '当前版' : '历史版'}</td><td>
                    <Button variant="link" onClick={() => setQuery({revision: String(version)})}>打开 v{version}</Button></td></tr>)}
            </tbody></Table>
                {historyError && <ErrorNotice error={historyError}/>}
                {history?.continuation && <Button className="mb-3" variant="outline-primary" disabled={historyBusy} onClick={() => {void moreHistory();}}>{historyBusy ? '正在加载历史…' : '加载更多历史版本'}</Button>}
                <p className="text-muted">历史按页读取，比较与版本选择只展示已加载的发布记录；当前退役和授权判定继续适用于历史读取。Dataset 有新发布导致分页失效时，请打开当前版本重新加载。</p></div></Container>}</Tab>}
            {(detail || detailError) && <Tab eventKey="compare" disabled={!detail} title={<><GitCompareIcon/> Compare</>}>{detail && <Container fluid="sm"><div className="mt-4"><Compare key={`${datasetId}:${detail.revision}`} dataset={datasetId} detail={{...detail, versions}}/></div></Container>}</Tab>}
            {(detail || detailError) && <Tab eventKey="definition" disabled={!detail} title={<><CodeIcon/> JSON</>}>{detail && <Container fluid="sm"><div className="mt-4"><pre className="text-wrap">{JSON.stringify({dataset: detail.id, revision: detail.revision, sources: detail.sources, ...(detail.metadata === undefined ? {} : {metadata: detail.metadata})}, null, 2)}</pre></div></Container>}</Tab>}
        </Tabs>
    </div>;
};
export default DatasetPage;
