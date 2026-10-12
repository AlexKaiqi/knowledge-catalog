import React, {useEffect, useState} from 'react';
import {Link, useNavigate, useSearchParams} from 'react-router-dom';
import {Alert, Badge, Button, ButtonToolbar, Card, Col, Container, Form, InputGroup, Row, Stack} from 'react-bootstrap';
import {DatabaseIcon, SearchIcon} from '@primer/octicons-react';
import {kc, DatasetSummary, uiPath} from '../../lib/api/kc';
import {ActionsBar, Loading} from '../../lib/components/controls';
import ErrorNotice from '../../lib/components/kcError';

const DatasetsPage: React.FC = () => {
    const navigate = useNavigate();
    const [query, setQuery] = useSearchParams();
    const search = query.get('search') ?? '';
    const [datasets, setDatasets] = useState<DatasetSummary[] | null>(null);
    const [error, setError] = useState<Error | null>(null);
    const [attempt, setAttempt] = useState(0);
    useEffect(() => {
        const controller = new AbortController();
        setError(null); setDatasets(null);
        kc.datasets(controller.signal).then(value => {if (!controller.signal.aborted) setDatasets(value.datasets);})
            .catch(err => {if (!controller.signal.aborted) setError(err as Error);});
        return () => controller.abort();
    }, [attempt]);
    const visible = datasets?.filter(dataset => [dataset.id, ...dataset.repositories].some(value => value.toLowerCase().includes(search.toLowerCase())));
    return <Container fluid="xl" className="mt-3 pb-4">
        <ActionsBar>
            <Stack direction="horizontal" gap={2} className="flex-wrap flex-grow-1">
                <Form onSubmit={event => event.preventDefault()} style={{minWidth: 280}}>
                    <InputGroup><InputGroup.Text><SearchIcon/></InputGroup.Text>
                        <Form.Control aria-label="搜索 Dataset" placeholder="Find a dataset..." value={search} onChange={event => setQuery(event.target.value ? {search: event.target.value} : {}, {replace: true})}/>
                    </InputGroup>
                </Form>
                <ButtonToolbar className="ms-auto"><Button variant="success" disabled={datasets === null || !!error} onClick={() => navigate(uiPath.create())}><DatabaseIcon/> 创建 Dataset</Button></ButtonToolbar>
            </Stack>
        </ActionsBar>
        {error && <><ErrorNotice error={error}/><Button variant="outline-secondary" className="mb-3" onClick={() => setAttempt(value => value + 1)}>重试</Button></>}
        {!error && datasets === null && <Loading/>}
        {!error && visible?.length === 0 && <Alert variant="light">{search ? '没有匹配的 Dataset。' : '当前没有 Dataset。'}</Alert>}
        {!error && visible?.map(dataset => <Row className="repository-item" key={dataset.id}><Col className="mb-2 mt-2"><Card><Card.Body>
            <Stack direction="horizontal" gap={2} className="flex-wrap">
                <h5 className="mb-2"><Link to={uiPath.dataset(dataset.id)}>{dataset.id}</Link></h5>
                <Badge bg="light" text="body" className="mb-2">v{dataset.revision}</Badge>
                {dataset.retired && <Badge bg="secondary" className="mb-2">已退役</Badge>}
            </Stack>
            <p className="mb-0"><small>来源仓：{dataset.repositories.join('、') || '—'}<br/>交付范围：{dataset.itemCount} 个条目 · {dataset.retired ? '已退役' : '当前发布版'}</small></p>
        </Card.Body></Card></Col></Row>)}
    </Container>;
};
export default DatasetsPage;
