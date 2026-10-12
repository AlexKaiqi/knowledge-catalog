import React, { useEffect, useState } from 'react';
import { Alert, Button, Col, Form, Row, Spinner, Table } from 'react-bootstrap';
import { DatasetDetail, kc } from '../../lib/api/kc';
import ErrorNotice from '../../lib/components/kcError';
import { compareItems, ItemChange } from './definition';

const Compare: React.FC<{catalog: string; dataset: string; detail: DatasetDetail}> = ({catalog, dataset, detail}) => {
    const [left, setLeft] = useState(detail.versions.find(v => v !== detail.revision) ?? detail.revision);
    const [right, setRight] = useState(detail.revision);
    const [changes, setChanges] = useState<ItemChange[] | null>(null);
    const [definitions, setDefinitions] = useState<DatasetDetail[] | null>(null);
    const [error, setError] = useState<Error | null>(null);
    const [attempt, setAttempt] = useState(0);
    useEffect(() => {
        const controller = new AbortController();
        setChanges(null); setDefinitions(null); setError(null);
        Promise.all([kc.dataset(catalog, dataset, left, controller.signal), kc.dataset(catalog, dataset, right, controller.signal)])
            .then(([l, r]) => { if (!controller.signal.aborted) { setChanges(compareItems(l.items, r.items)); setDefinitions([l, r]); } })
            .catch(err => { if (!controller.signal.aborted) setError(err); });
        return () => controller.abort();
    }, [catalog, dataset, left, right, attempt]);
    return <>
        <Row className="mb-3 g-3">{[['比较前版本', left, setLeft], ['比较后版本', right, setRight]].map(([label, value, update]) =>
            <Col md={4} key={String(label)}><Form.Group controlId={`compare-${label}`}><Form.Label>{String(label)}</Form.Label>
                <Form.Select value={value as number} onChange={event => (update as (v: number) => void)(Number(event.target.value))}>
                    {detail.versions.map(v => <option key={v} value={v}>v{v}</option>)}
                </Form.Select></Form.Group></Col>)}</Row>
        {error && <><ErrorNotice error={error}/><Button className="mb-3" onClick={() => setAttempt(v => v + 1)}>重试比较</Button></>}
        {!error && !changes && <Spinner animation="border"/>}
        {changes?.length === 0 && <Alert variant="light">交付文件定义相同。</Alert>}
        {!!changes?.length && <Table responsive><thead><tr><th>交付路径</th><th>变化</th><th>原定义</th><th>新定义</th></tr></thead>
            <tbody>{changes.map(change => <tr key={change.target}><td>{change.target || '/'}</td><td>{{added: '新增', removed: '移除', modified: '修改'}[change.kind]}</td>
                <td><pre className="small text-wrap">{change.before && JSON.stringify(change.before, null, 2)}</pre></td>
                <td><pre className="small text-wrap">{change.after && JSON.stringify(change.after, null, 2)}</pre></td></tr>)}</tbody></Table>}
        {definitions && <details><summary>比较完整来源定义</summary><Row>{definitions.map((value, i) => <Col md={6} key={i}>
            <h3 className="h6 mt-3">v{value.revision}</h3><pre className="small text-wrap">{JSON.stringify(value.sources, null, 2)}</pre></Col>)}</Row></details>}
    </>;
};
export default Compare;
