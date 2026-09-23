import React, { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
    Alert, Breadcrumb, Button, Card, Container, Form, Spinner,
} from 'react-bootstrap';
import { KCError, kc, SourceInput } from '../../lib/api/kc';

// Define / publish a Dataset: directory mappings (--source repo[=selector]@path[@subPath])
// and per-file entries (--file-source repo[=selector]@file@target). The form
// builds exactly the KnowledgeSetSource shape the publish surface validates:
// every mount entry declares a delivered path (root mounts use the empty
// string), file entries declare source file + delivered target, and the two
// never mix fields. Uniqueness and traversal rules are server-enforced.

type MountRow = { repository: string; selector: string; path: string; subPath: string };
type FileRow = { repository: string; selector: string; file: string; target: string };

const PublishPage: React.FC = () => {
    const navigate = useNavigate();
    const [catalogId, setCatalogId] = useState('');
    const [dataset, setDataset] = useState('');
    const [revision, setRevision] = useState(1);
    const [mounts, setMounts] = useState<MountRow[]>([{ repository: '', selector: '', path: '', subPath: '' }]);
    const [files, setFiles] = useState<FileRow[]>([]);
    const [error, setError] = useState<KCError | null>(null);
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        kc.catalogs()
            .then((state) => {
                const first = (state.catalogs ?? []).map((c) => c.id)[0] ?? '';
                if (first) setCatalogId(first);
                else setError(new KCError(404, { code: 'CATALOG_UNRESOLVED', message: '没有可见的 Catalog；先用 kc catalog use 选择。' }));
            })
            .catch((err) => setError(err as KCError));
    }, []);

    const updateMount = (i: number, patch: Partial<MountRow>): void =>
        setMounts((rows) => rows.map((r, at) => (at === i ? { ...r, ...patch } : r)));
    const updateFile = (i: number, patch: Partial<FileRow>): void =>
        setFiles((rows) => rows.map((r, at) => (at === i ? { ...r, ...patch } : r)));

    const sources = (): SourceInput[] => {
        const out: SourceInput[] = [];
        for (const m of mounts) {
            if (!m.repository.trim()) continue;
            out.push({
                repository: m.repository.trim(),
                selector: m.selector.trim() || undefined,
                path: m.path.trim(),
                subPath: m.subPath.trim() || undefined,
            });
        }
        for (const f of files) {
            if (!f.repository.trim() || !f.file.trim() || !f.target.trim()) continue;
            out.push({
                repository: f.repository.trim(),
                selector: f.selector.trim() || undefined,
                file: f.file.trim(),
                target: f.target.trim(),
            });
        }
        return out;
    };

    const submit = async (): Promise<void> => {
        setError(null);
        const list = sources();
        if (!dataset.trim() || list.length === 0) {
            setError(new KCError(400, { code: 'USAGE_INVALID', message: '需要 dataset id 与至少一个来源（目录挂载或逐文件）。' }));
            return;
        }
        setBusy(true);
        try {
            await kc.define(catalogId, { dataset: dataset.trim(), revision, sources: list });
            navigate(`/ui/datasets/${encodeURIComponent(dataset.trim())}`);
        } catch (err) {
            setError(err as KCError);
        } finally {
            setBusy(false);
        }
    };

    if (error === null && !catalogId) {
        return <Container className="mt-4"><Spinner animation="border" /></Container>;
    }
    return (
        <Container className="mt-4">
            <Breadcrumb>
                <Breadcrumb.Item linkAs={Link} linkProps={{ to: '/ui/' }}>Datasets</Breadcrumb.Item>
                <Breadcrumb.Item active>定义 / 发布</Breadcrumb.Item>
            </Breadcrumb>
            {error && (
                <Alert variant="danger">
                    <strong>{error.code}</strong>
                    <div>{error.message}</div>
                </Alert>
            )}
            <Card className="mb-3">
                <Card.Body>
                    <Form.Group className="mb-3">
                        <Form.Label>Catalog</Form.Label>
                        <Form.Control value={catalogId} readOnly />
                    </Form.Group>
                    <Form.Group className="mb-3">
                        <Form.Label>Dataset ID</Form.Label>
                        <Form.Control value={dataset} onChange={(e) => setDataset(e.target.value)}
                            placeholder="例如 scene-set" />
                    </Form.Group>
                    <Form.Group className="mb-3">
                        <Form.Label>版本（revision）</Form.Label>
                        <Form.Control type="number" min={1} value={revision}
                            onChange={(e) => setRevision(Number(e.target.value) || 1)} />
                    </Form.Group>
                </Card.Body>
            </Card>
            <Card className="mb-3">
                <Card.Body>
                    <Card.Title>目录挂载</Card.Title>
                    <Card.Text className="text-muted small">
                        每行把一个仓的一个子目录交付到 Dataset 的一个目录。交付目录不得嵌套重叠；
                        同一仓的所有行共用该仓本次发布冻结的 commit。selector 留空用默认 published。
                    </Card.Text>
                    {mounts.map((row, i) => (
                        <div key={i} className="mb-3">
                            <Form.Control className="mb-1" placeholder="仓库 id，例如 kr://scene/knowledge"
                                value={row.repository} onChange={(e) => updateMount(i, { repository: e.target.value })} />
                            <Form.Control className="mb-1" placeholder="selector（可留空）"
                                value={row.selector} onChange={(e) => updateMount(i, { selector: e.target.value })} />
                            <Form.Control className="mb-1" placeholder="交付目录 path（根挂载用 @ 空串）"
                                value={row.path} onChange={(e) => updateMount(i, { path: e.target.value })} />
                            <Form.Control placeholder="来源仓内子目录 subPath（可留空=整仓）"
                                value={row.subPath} onChange={(e) => updateMount(i, { subPath: e.target.value })} />
                        </div>
                    ))}
                    <Button size="sm" variant="outline-primary" onClick={() => setMounts((r) => [...r, { repository: '', selector: '', path: '', subPath: '' }])}>
                        + 增加目录挂载
                    </Button>
                </Card.Body>
            </Card>
            <Card className="mb-3">
                <Card.Body>
                    <Card.Title>逐文件交付</Card.Title>
                    <Card.Text className="text-muted small">
                        每行把一个仓里的一个文件按新名字交付进 Dataset；可与目录挂载混排进同一交付目录。
                        同一交付目标文件只能有一个来源。
                    </Card.Text>
                    {files.map((row, i) => (
                        <div key={i} className="mb-3">
                            <Form.Control className="mb-1" placeholder="仓库 id"
                                value={row.repository} onChange={(e) => updateFile(i, { repository: e.target.value })} />
                            <Form.Control className="mb-1" placeholder="selector（可留空）"
                                value={row.selector} onChange={(e) => updateFile(i, { selector: e.target.value })} />
                            <Form.Control className="mb-1" placeholder="来源文件路径，例如 reports/gmv.csv"
                                value={row.file} onChange={(e) => updateFile(i, { file: e.target.value })} />
                            <Form.Control placeholder="交付路径（可含改名），例如 reference/gmv.csv"
                                value={row.target} onChange={(e) => updateFile(i, { target: e.target.value })} />
                        </div>
                    ))}
                    <Button size="sm" variant="outline-primary" onClick={() => setFiles((r) => [...r, { repository: '', selector: '', file: '', target: '' }])}>
                        + 增加逐文件条目
                    </Button>
                </Card.Body>
            </Card>
            <Button variant="success" disabled={busy} onClick={() => { void submit(); }}>
                {busy ? '发布中…' : '发布这个版本'}
            </Button>
        </Container>
    );
};

export default PublishPage;
