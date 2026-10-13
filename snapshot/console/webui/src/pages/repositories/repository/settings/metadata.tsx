import React, {useEffect, useRef, useState} from 'react';
import {Alert, Button, Form, Spinner} from 'react-bootstrap';
import {repositories} from '../../../../lib/api';
import {Metadata, MetadataRow, metadataFromRows, metadataRows} from '../../../../lib/api/kcMetadata';
import {MetadataEditor} from '../../../../lib/components/resourceMetadata';
import ErrorNotice from '../../../../lib/components/kcError';

const RepositoryMetadata: React.FC<{repository: string; readOnly?: boolean}> = ({repository, readOnly}) => {
    const [stored, setStored] = useState<Metadata | null>(null);
    const [rows, setRows] = useState<MetadataRow[]>([]);
    const [error, setError] = useState<Error | null>(null);
    const [readError, setReadError] = useState<Error | null>(null);
    const [busy, setBusy] = useState(false);
    const [saved, setSaved] = useState(false);
    const [writeFailed, setWriteFailed] = useState(false);
    const [attempt, setAttempt] = useState(0);
    const pending = useRef<AbortController>();
    useEffect(() => {
        const controller = new AbortController(); pending.current = controller;
        setStored(null); setRows([]); setReadError(null); setError(null); setBusy(false); setSaved(false); setWriteFailed(false);
        repositories.metadata(repository, controller.signal).then(value => {
            if (!controller.signal.aborted) {setStored(value); setRows(metadataRows(value));}
        }).catch(err => {if (!controller.signal.aborted) setReadError(err);});
        return () => controller.abort();
    }, [repository, attempt]);
    const save = async (): Promise<void> => {
        if (!stored) return;
        const signal = pending.current?.signal;
        setError(null); setSaved(false); setWriteFailed(false);
        let writesStarted = false;
        let writeSucceeded = false;
        try {
            const desired = metadataFromRows(rows);
            const updates = Object.fromEntries(Object.entries(desired).filter(([key, value]) => stored[key] !== value || !Object.prototype.hasOwnProperty.call(stored, key)));
            const removed = Object.keys(stored).filter(key => !Object.prototype.hasOwnProperty.call(desired, key));
            setBusy(true);
            writesStarted = true;
            if (Object.keys(updates).length) await repositories.setMetadata(repository, updates, signal);
            if (removed.length) await repositories.deleteMetadata(repository, removed, signal);
            writeSucceeded = true;
        } catch (err) {if (!signal?.aborted) {setError(err as Error); setWriteFailed(writesStarted);}}
        finally {
            if (writesStarted && !signal?.aborted) {
                // Writes are per native operation. Read the authority after success or
                // partial failure instead of claiming the draft was accepted atomically.
                try {
                    const value = await repositories.metadata(repository, signal);
                    if (!signal?.aborted) {setStored(value); setRows(metadataRows(value)); setReadError(null); setSaved(writeSucceeded);}
                } catch (err) {if (!signal?.aborted) {setStored(null); setReadError(err as Error); setSaved(false);}}
                if (!signal?.aborted) setBusy(false);
            }
        }
    };
    return <section className="mt-4" aria-label="Repository 自定义元数据">
        {error && <ErrorNotice error={error}/>}
        {writeFailed && <p className="text-muted">{busy ? '保存未全部完成，正在重新读取当前仓的元数据。' : readError ? '保存可能部分完成，请重新读取元数据后核对。' : '保存未全部完成；已重新读取当前仓的元数据，请核对后重试。'}</p>}
        {readError && <><ErrorNotice error={readError}/><Button variant="outline-primary" onClick={() => setAttempt(value => value + 1)}>重新读取元数据</Button></>}
        {stored === null && !readError && <Spinner animation="border"/>}
        {stored !== null && <Form onSubmit={event => {event.preventDefault(); void save();}}>
            <MetadataEditor rows={rows} disabled={busy || readOnly} change={next => {setRows(next); setSaved(false);}}/>
            {saved && <Alert variant="success">自定义元数据已保存并重新读取。</Alert>}
            <Button type="submit" disabled={busy || readOnly} variant="primary">{busy ? '正在保存…' : '保存元数据'}</Button>
        </Form>}
    </section>;
};
export default RepositoryMetadata;
