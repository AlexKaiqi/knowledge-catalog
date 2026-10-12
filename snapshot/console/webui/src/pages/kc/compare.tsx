import React, {useEffect, useState} from 'react';
import {Link} from 'react-router-dom';
import {Alert, Button, Card, Col, Form, Row, Table} from 'react-bootstrap';
import {GitCompareIcon} from '@primer/octicons-react';
import {RegisteredRepository, DatasetDetail, kc} from '../../lib/api/kc';
import {Loading} from '../../lib/components/controls';
import TextDiff from '../../lib/components/repository/TextDiff';
import ErrorNotice from '../../lib/components/kcError';

const definitionText = (detail: DatasetDetail): string => JSON.stringify({sources: detail.sources,
    items: [...detail.items].sort((a, b) => a.target.localeCompare(b.target))}, null, 2);

const Compare: React.FC<{dataset: string; detail: DatasetDetail}> = ({dataset, detail}) => {
    const [left, setLeft] = useState(detail.versions.find(v => v !== detail.revision) ?? detail.revision);
    const [right, setRight] = useState(detail.revision);
    const [definitions, setDefinitions] = useState<[DatasetDetail, DatasetDetail] | null>(null);
    const [repositories, setRepositories] = useState<RegisteredRepository[]>([]);
    const [error, setError] = useState<Error | null>(null);
    const [linkError, setLinkError] = useState<Error | null>(null);
    const [attempt, setAttempt] = useState(0);
    useEffect(() => {
        const controller = new AbortController();
        setDefinitions(null); setError(null); setLinkError(null); setRepositories([]);
        Promise.all([kc.dataset(dataset, left, controller.signal), kc.dataset(dataset, right, controller.signal)])
            .then(values => {if (!controller.signal.aborted) setDefinitions(values);})
            .catch(err => {if (!controller.signal.aborted) setError(err);});
        kc.repositories(controller.signal).then(value => {if (!controller.signal.aborted) setRepositories(value.repositories);})
            .catch(err => {if (!controller.signal.aborted) setLinkError(err);});
        return () => controller.abort();
    }, [dataset, left, right, attempt]);
    const before = definitions && Object.fromEntries(definitions[0].items.map(item => [item.repository, item.commit]));
    const after = definitions && Object.fromEntries(definitions[1].items.map(item => [item.repository, item.commit]));
    return <>
        <Row className="mb-3 g-3 align-items-end">
            <Col md={4}><Form.Group controlId="compare-before"><Form.Label>比较前版本</Form.Label>
                <Form.Select value={left} onChange={event => setLeft(Number(event.target.value))}>
                    {detail.versions.map(v => <option key={v} value={v}>v{v}</option>)}
                </Form.Select></Form.Group></Col>
            <Col md={4}><Form.Group controlId="compare-after"><Form.Label>比较后版本</Form.Label>
                <Form.Select value={right} onChange={event => setRight(Number(event.target.value))}>
                    {detail.versions.map(v => <option key={v} value={v}>v{v}</option>)}
                </Form.Select></Form.Group></Col>
            <Col md="auto"><Button variant="outline-secondary" onClick={() => {setLeft(right); setRight(left);}}><GitCompareIcon/> 交换版本</Button></Col>
        </Row>
        {error && <ErrorNotice error={error}/>}
        {(error || linkError) && <Button className="mb-3" onClick={() => setAttempt(v => v + 1)}>重试比较</Button>}
        {!error && !definitions && <Loading/>}
        {definitions && <>
            <Card className="mb-4"><Card.Header>发布定义 · v{left} → v{right}</Card.Header><Card.Body className="p-0">
                <section aria-label="发布定义差异" className="objects-diff">
                    <TextDiff before={definitionText(definitions[0])} after={definitionText(definitions[1])}/>
                </section>
            </Card.Body></Card>
            <h2 className="h5">来源文件差异</h2>
            <p className="text-muted">在原 Repository Compare 中比较固定提交；按该仓当前权限查看文件。交付路径与范围的变化见上面的发布定义。</p>
            {linkError && <ErrorNotice error={linkError}/>}
            <Table responsive hover><thead><tr><th>来源仓</th><th>原提交</th><th>新提交</th><th/></tr></thead><tbody>
                {[...new Set([...Object.keys(before!), ...Object.keys(after!)])].map(repository => {
                    const native = repositories.find(value => value.id === repository)?.consoleRepository;
                    const from = before![repository], to = after![repository];
                    const compareQuery = new URLSearchParams({ref: from, compare: to});
                    return <tr key={repository}><td><code>{repository}</code></td><td><code title={from}>{from?.slice(0, 12) ?? '—'}</code></td><td><code title={to}>{to?.slice(0, 12) ?? '—'}</code></td><td>
                        {from && to && from === to ? <span className="text-muted">来源提交相同</span> : native && from && to ?
                            <Link to={`/repositories/${encodeURIComponent(native)}/compare?${compareQuery}`}><GitCompareIcon/> 比较来源文件</Link> :
                            native ? <Link to={`/repositories/${encodeURIComponent(native)}/objects?${new URLSearchParams({ref: to || from})}`}>浏览固定来源</Link> :
                                <span className="text-muted">此来源未接入当前 Repository 控制台</span>}
                    </td></tr>;
                })}
            </tbody></Table>
            {definitions.every(value => value.items.length === 0) && <Alert variant="light">没有已发布的文件来源。</Alert>}
        </>}
    </>;
};
export default Compare;
