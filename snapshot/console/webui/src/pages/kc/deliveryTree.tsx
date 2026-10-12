import React, { useEffect, useRef, useState } from 'react';
import { Alert, Button, Card, ListGroup, Spinner } from 'react-bootstrap';
import { decodeContent, DirectoryResponse, kc, ResolvedPin } from '../../lib/api/kc';
import ErrorNotice from '../../lib/components/kcError';

const DeliveryTree: React.FC<{ catalog: string; dataset: string; pin: ResolvedPin }> = ({ catalog, dataset, pin }) => {
    const [directory, setDirectory] = useState('');
    const [cursor, setCursor] = useState('');
    const [listing, setListing] = useState<DirectoryResponse | null>(null);
    const [error, setError] = useState<Error | null>(null);
    const [failedFile, setFailedFile] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [preview, setPreview] = useState<{ path: string; text: string; truncated: boolean; download?: string } | null>(null);
    const [reading, setReading] = useState(false);
    const fileRequest = useRef<AbortController>();
    const [attempt, setAttempt] = useState(0);
    useEffect(() => {
        const controller = new AbortController();
        setLoading(true); setError(null); setFailedFile(null);
        kc.tree(catalog, dataset, pin, directory, cursor || undefined, controller.signal).then((page) => {
            if (!controller.signal.aborted) setListing((before) => ({ ...page, entries: cursor && before ? [...before.entries, ...page.entries] : page.entries }));
        }).catch((err) => { if (!controller.signal.aborted) { setError(err); setFailedFile(null); } })
            .finally(() => { if (!controller.signal.aborted) setLoading(false); });
        return () => controller.abort();
    }, [catalog, dataset, pin, directory, cursor, attempt]);
    useEffect(() => () => { fileRequest.current?.abort(); }, []);
    useEffect(() => () => { if (preview?.download) URL.revokeObjectURL(preview.download); }, [preview]);
    const move = (path: string): void => {
        fileRequest.current?.abort(); setReading(false); setPreview(null); setListing(null); setCursor(''); setDirectory(path); setFailedFile(null);
    };
    const openFile = async (name: string): Promise<void> => {
        fileRequest.current?.abort();
        const controller = new AbortController(); fileRequest.current = controller;
        const path = directory ? `${directory}/${name}` : name;
        setReading(true); setPreview(null); setError(null); setFailedFile(null);
        try {
            const value = await kc.file(catalog, dataset, pin, path, controller.signal);
            if (controller.signal.aborted) return;
            const bytes = decodeContent(value.content);
            let text;
            try { text = new TextDecoder('utf-8', { fatal: true }).decode(bytes, { stream: !value.eof }); if (text.includes('\u0000')) throw new Error('binary'); }
            catch { text = '此文件是二进制内容，无法显示文本预览。'; }
            const download = value.eof ? URL.createObjectURL(new Blob([bytes], { type: 'application/octet-stream' })) : undefined;
            setPreview({ path, text, truncated: !value.eof, download });
        } catch (err) { if (!controller.signal.aborted) { setError(err as Error); setFailedFile(name); } }
        finally { if (!controller.signal.aborted) setReading(false); }
    };
    return <Card className="mb-4"><Card.Body>
        <Card.Title>交付目录 · {pin.ref ?? `v${pin.revision}`}</Card.Title>
        <div className="d-flex align-items-center gap-2 mb-3">
            <Button size="sm" variant="outline-secondary" disabled={!directory} onClick={() => move(directory.split('/').slice(0, -1).join('/'))}>上一级</Button>
            <code>/{directory}</code>
        </div>
        {error && <><ErrorNotice error={error} /><Button size="sm" variant="outline-primary" onClick={() => failedFile === null ? setAttempt((value) => value + 1) : void openFile(failedFile)}>重试读取</Button></>}
        {listing && <ListGroup variant="flush">{listing.entries.map((entry) => <ListGroup.Item key={entry.name} action
            onClick={() => entry.kind === 'directory' ? move(directory ? `${directory}/${entry.name}` : entry.name) : void openFile(entry.name)}>
            {entry.kind === 'directory' ? '📁' : '📄'} {entry.name}
        </ListGroup.Item>)}</ListGroup>}
        {listing?.entries.length === 0 && <p className="text-muted">这个目录没有文件。</p>}
        {loading && <Spinner animation="border" size="sm" />}
        {listing && !listing.exhausted && listing.continuation && <Button size="sm" className="mt-3" disabled={loading} onClick={() => setCursor(listing.continuation ?? '')}>加载更多文件</Button>}
        {reading && <p className="mt-3">正在读取文件…</p>}
        {preview && <Card className="mt-3"><Card.Header className="d-flex justify-content-between gap-2"><span>{preview.path}</span>
            <Button size="sm" variant="outline-secondary" onClick={() => setPreview(null)}>关闭预览</Button></Card.Header><Card.Body>
            {preview.truncated && <Alert variant="info">显示前 64 KiB；完整文件可通过 Dataset 的本地交付命令取得。</Alert>}
            <pre className="mb-3" style={{ maxHeight: 360, overflow: 'auto' }}>{preview.text}</pre>
            {preview.download && <a className="btn btn-outline-primary btn-sm" href={preview.download} download={preview.path.split('/').pop()}>下载文件</a>}
        </Card.Body></Card>}
    </Card.Body></Card>;
};
export default DeliveryTree;
