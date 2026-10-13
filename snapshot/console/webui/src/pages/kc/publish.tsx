import React, { useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Alert, Breadcrumb, Button, Container, Form, Spinner, Table, ToggleButton, ToggleButtonGroup } from 'react-bootstrap';
import { kc, uiPath, ResolvedPin } from '../../lib/api/kc';
import { Definition, parseDefinition } from './definition';
import ErrorNotice from '../../lib/components/kcError';
import SourceEditor, { emptyRow, sourceInputs, sourceRow, SourceRow } from './sourceEditor';
import {MetadataRow, metadataRows, metadataFromRows} from '../../lib/api/kcMetadata';
import {MetadataEditor} from '../../lib/components/resourceMetadata';

const PublishPage: React.FC = () => {
    const { datasetId } = useParams();
    const navigate = useNavigate();
    const [dataset, setDataset] = useState(datasetId ?? '');
    const [revision, setRevision] = useState(1);
    const [rows, setRows] = useState<SourceRow[]>([emptyRow()]);
    const [metadata, setMetadata] = useState<MetadataRow[]>([]);
    const [metadataDefined, setMetadataDefined] = useState(false);
    const [mode, setMode] = useState<'form' | 'json'>('form');
    const [json, setJson] = useState('');
    const [preview, setPreview] = useState<{signature: string; pin: ResolvedPin} | null>(null);
    const [repositories, setRepositories] = useState<string[]>([]);
    const [ready, setReady] = useState(false);
    const [error, setError] = useState<Error | null>(null);
    const [busy, setBusy] = useState(false);
    const pending = useRef<AbortController>();
    useEffect(() => {
        const controller = new AbortController();
        pending.current = controller;
        setBusy(false); setPreview(null); setMode('form'); setJson('');
        setReady(false); setError(null); setDataset(datasetId ?? ''); setRows([emptyRow()]); setRevision(1);
        setMetadata([]); setMetadataDefined(false);
        (async () => {
            try {
                const registered = await kc.repositories(controller.signal);
                if (controller.signal.aborted) return;
                setRepositories(registered.repositories.map((repo) => repo.id));
                if (datasetId) {
                    const detail = await kc.dataset(datasetId, undefined, controller.signal);
                    if (controller.signal.aborted) return;
                    setRevision(detail.servingRevision + 1);
                    setRows(detail.sources.map(sourceRow));
                    setMetadata(metadataRows(detail.metadata)); setMetadataDefined(detail.metadata !== undefined);
                }
                setReady(true);
            } catch (err) { if (!controller.signal.aborted) setError(err as Error); }
        })();
        return () => controller.abort();
    }, [datasetId]);
    const metadataField = () => metadataDefined ? {metadata: metadataFromRows(metadata)} : {};
    const definition = (): Definition => mode === 'json' ? parseDefinition(json, datasetId) : parseDefinition(JSON.stringify({dataset: dataset.trim(), revision, sources: sourceInputs(rows), ...metadataField()}), datasetId);
    const changeMode = (value: 'form' | 'json'): void => {
        setError(null);
        try {
            if (value === 'json') {
                // Preserve incomplete rows so a new Dataset can start in JSON mode.
                const sources = rows.map(row => ({repository: row.repository, selector: row.selector || 'refs/heads/main',
                    commit: row.basis === 'commit' ? row.commit : undefined, baseRev: row.baseRev,
                    ...(row.kind === 'file' ? {file: row.source, target: row.target} : row.kind === 'prefix' ? {path: row.target, subPath: row.source || undefined} : {subPath: row.source || undefined})}));
                setJson(JSON.stringify({dataset, revision, sources, ...metadataField()}, null, 2));
            } else {
                const current = definition();
                setDataset(current.dataset); setRevision(current.revision); setRows(current.sources.map(sourceRow));
                setMetadata(metadataRows(current.metadata)); setMetadataDefined(current.metadata !== undefined);
            }
            setMode(value);
        } catch (err) { setError(err as Error); }
    };
    const validate = async (): Promise<void> => {
        const signal = pending.current?.signal;
        setError(null); setPreview(null);
        try {
            const value = definition();
            setBusy(true);
            const pin = await kc.preview(value, signal);
            if (pin.setId !== value.dataset || pin.revision !== value.revision || value.sources.some(source => !pin.repositories[source.repository])) throw new Error('来源校验没有返回完整的固定版本，不能采用此预览。');
            if (!signal?.aborted) setPreview({signature: JSON.stringify(value), pin});
        } catch (err) { if (!signal?.aborted) setError(err as Error); }
        finally { if (!signal?.aborted) setBusy(false); }
    };
    let matchingPreview: ResolvedPin | null = null;
    try { if (preview?.signature === JSON.stringify(definition())) matchingPreview = preview.pin; } catch { /* Incomplete edits invalidate the preview. */ }
    const submit = async (): Promise<void> => {
        const signal = pending.current?.signal;
        setError(null);
        try {
            const value = definition();
            const confirmed = matchingPreview;
            if (!confirmed) throw new Error('请先校验并预览当前来源，再发布这个版本。');
            value.sources = value.sources.map(source => ({...source, commit: confirmed.repositories[source.repository]}));
            setBusy(true);
            await kc.define(value, signal);
            if (!signal?.aborted) navigate(uiPath.dataset(value.dataset));
        } catch (err) { if (!signal?.aborted) setError(err as Error); }
        finally { if (!signal?.aborted) setBusy(false); }
    };
    return <Container fluid="xl" className="mt-3 pb-4">
        <Breadcrumb><Breadcrumb.Item linkAs={Link} linkProps={{to: '/datasets'}}>Datasets</Breadcrumb.Item>
            {datasetId && <Breadcrumb.Item linkAs={Link} linkProps={{ to: uiPath.dataset(datasetId) }}>{datasetId}</Breadcrumb.Item>}
            <Breadcrumb.Item active>{datasetId ? '发布新版' : '创建 Dataset'}</Breadcrumb.Item></Breadcrumb>
        <h1 className="h3">{datasetId ? '编辑并发布新版' : '创建 Dataset'}</h1>
        {error && <ErrorNotice error={error} />}
        {!ready && !error && <Spinner animation="border" />}
        {ready && <Form onSubmit={(event) => { event.preventDefault(); void submit(); }}>
            <fieldset disabled={busy}>
                <ToggleButtonGroup type="radio" name="definition-mode" value={mode} onChange={changeMode} className="mb-4">
                    <ToggleButton id="mode-form" value="form" variant="outline-primary">Form</ToggleButton>
                    <ToggleButton id="mode-json" value="json" variant="outline-primary">JSON</ToggleButton>
                </ToggleButtonGroup>
                {mode === 'json' ? <Form.Group controlId="dataset-json" className="mb-4"><Form.Label>Dataset JSON 定义</Form.Label>
                    <Form.Control as="textarea" rows={18} value={json} onChange={event => setJson(event.target.value)} spellCheck={false}/>
                    <Form.Text>使用 KC 发布合同：dataset、revision、sources，以及可选的 metadata 字符串键值对象；不支持的字段会明确拒绝。</Form.Text>
                </Form.Group> : <>
                <Form.Group controlId="dataset-name" className="mb-3"><Form.Label>Dataset 名称</Form.Label>
                    <Form.Control required readOnly={!!datasetId} value={dataset} onChange={(event) => setDataset(event.target.value)} /></Form.Group>
                <Form.Group controlId="dataset-revision" className="mb-3"><Form.Label>发布版本</Form.Label>
                    <Form.Control type="number" min={1} step={1} required value={revision} onChange={(event) => setRevision(Number(event.target.value))} /></Form.Group>
                <MetadataEditor rows={metadata} disabled={busy} change={next => {setMetadata(next); setMetadataDefined(true);}}/>
                <p className="text-muted">元数据随本次发布版本保存；编辑新版时沿用当前版，可修改或清空。</p>
                {datasetId && <Alert variant="info">沿用原版的固定提交。需要采用来源更新时，将对应来源的版本切换为“发布时固定分支版本”。发布会创建新版本，旧版保持原样。</Alert>}
                {repositories.length === 0 && <Alert variant="warning">服务没有已登记的来源仓，请先完成来源接入。</Alert>}
                {rows.map((row, index) => <SourceEditor key={index} row={row} index={index} repositories={repositories} disabled={busy}
                    update={(next) => setRows((before) => before.map((item, at) => at === index ? next : item))}
                    remove={() => setRows((before) => before.filter((_, at) => at !== index))} />)}
                <div className="d-flex gap-2 mb-3"><Button variant="outline-primary" onClick={() => setRows((before) => [...before, emptyRow()])}>添加来源</Button></div>
                </>}
                <p className="text-muted">同一来源仓的多个条目须使用相同版本。服务端检查路径冲突、发布权限和版本并发更新；发布成功不会自动授予消费权限。</p>
                {matchingPreview && <div className="mb-4"><Alert variant="success">来源版本和读取权限已校验。按下面的固定提交发布；修改定义后需重新校验。发布权限与版本并发仍由服务端最终确认。</Alert>
                    <Table responsive size="sm"><thead><tr><th>交付路径</th><th>来源仓</th><th>固定提交</th></tr></thead><tbody>
                        {matchingPreview.items?.map(item => <tr key={item.target}><td>{item.target || '/'}</td><td>{item.repository}</td><td><code>{item.commit}</code></td></tr>)}
                    </tbody></Table></div>}
                <Button variant="outline-primary" className="me-2" onClick={() => {void validate();}} disabled={repositories.length === 0}>校验并预览来源</Button>
                <Button type="submit" variant="success" disabled={repositories.length === 0 || rows.length === 0}>{busy ? '正在发布…' : '发布这个版本'}</Button>
                <Link className="btn btn-link" to={datasetId ? uiPath.dataset(datasetId) : '/datasets'}>返回</Link>
            </fieldset>
        </Form>}
    </Container>;
};
export default PublishPage;
