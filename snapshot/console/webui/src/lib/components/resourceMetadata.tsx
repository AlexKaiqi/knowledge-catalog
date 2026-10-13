import React from 'react';
import {Button, Col, Form, Row, Table} from 'react-bootstrap';
import {Metadata, MetadataRow} from '../api/kcMetadata';

export const MetadataEditor: React.FC<{rows: MetadataRow[]; change: (rows: MetadataRow[]) => void; disabled?: boolean}> = ({rows, change, disabled}) => <fieldset disabled={disabled} className="mb-3" aria-label="自定义元数据">
    <legend className="h5">自定义元数据</legend>
    <p className="text-muted">可选的字符串键值，无必填键。删除所有条目可清空元数据。</p>
    {rows.map((row, index) => <Row key={index} className="g-2 mb-2 align-items-end">
        <Col md={4}><Form.Group controlId={`metadata-key-${index}`}><Form.Label>键 {index + 1}</Form.Label>
            <Form.Control aria-label={`元数据键 ${index + 1}`} value={row.key} onChange={event => change(rows.map((item, at) => at === index ? {...item, key: event.target.value} : item))}/></Form.Group></Col>
        <Col><Form.Group controlId={`metadata-value-${index}`}><Form.Label>值 {index + 1}</Form.Label>
            <Form.Control as="textarea" rows={2} aria-label={`元数据值 ${index + 1}`} value={row.value} onChange={event => change(rows.map((item, at) => at === index ? {...item, value: event.target.value} : item))}/></Form.Group></Col>
        <Col md="auto"><Button variant="outline-danger" aria-label={`删除元数据 ${index + 1}`} onClick={() => change(rows.filter((_, at) => at !== index))}>删除</Button></Col>
    </Row>)}
    <Button variant="outline-primary" onClick={() => change([...rows, {key: '', value: ''}])}>添加元数据</Button>
</fieldset>;

export const MetadataView: React.FC<{metadata?: Metadata}> = ({metadata}) => Object.keys(metadata ?? {}).length === 0 ? <p className="text-muted mb-0">没有自定义元数据。</p> :
    <Table responsive size="sm" className="mb-0"><thead><tr><th>键</th><th>值</th></tr></thead><tbody>{Object.entries(metadata!).map(([key, value]) => <tr key={key}>
        <td><code>{key || '（空键）'}</code></td><td style={{whiteSpace: 'pre-wrap'}}>{value}</td>
    </tr>)}</tbody></Table>;
