import React from 'react';
import { Button, Card, Col, Form, Row } from 'react-bootstrap';
import { SourceInput } from '../../lib/api/kc';

export type SourceRow = { baseRev?: string; kind: 'prefix' | 'file' | 'unmapped'; repository: string; basis: 'branch' | 'commit'; selector: string; commit: string; source: string; target: string };
export const emptyRow = (): SourceRow => ({ kind: 'prefix', repository: '', basis: 'branch', selector: '', commit: '', source: '', target: '' });
export const sourceRow = (source: SourceInput): SourceRow => ({
    baseRev: source.baseRev, kind: source.file ? 'file' : source.path === undefined ? 'unmapped' : 'prefix', repository: source.repository, basis: source.commit ? 'commit' : 'branch',
    selector: source.selector ?? '', commit: source.commit ?? '', source: source.file ?? source.subPath ?? '', target: source.target ?? source.path ?? '',
});
export function sourceInputs(rows: SourceRow[]): SourceInput[] {
    if (rows.length === 0) throw new Error('至少添加一个来源。');
    return rows.map((row, index) => {
        if (!row.repository || (row.basis === 'commit' && !row.commit.trim()) ||
            (row.kind === 'file' && (!row.source.trim() || !row.target.trim()))) {
            throw new Error(`请填写来源 ${index + 1} 的仓库、版本与文件路径；未填完整的条目不会被忽略。`);
        }
        // Same reference as snapshot.DefaultRef and the public CLI source parser.
        const common = { baseRev: row.baseRev, repository: row.repository, selector: row.selector.trim() || 'refs/heads/main', commit: row.basis === 'commit' ? row.commit.trim() : undefined };
        if (row.kind === 'unmapped') return { ...common, subPath: row.source.trim() || undefined };
        return row.kind === 'file' ? { ...common, file: row.source.trim(), target: row.target.trim() } : { ...common, path: row.target.trim(), subPath: row.source.trim() || undefined };
    });
}
const SourceEditor: React.FC<{ row: SourceRow; index: number; repositories: string[]; update: (row: SourceRow) => void; remove: () => void; disabled: boolean }> = ({ row, index, repositories, update, remove, disabled }) => {
    const change = (patch: Partial<SourceRow>): void => update({ ...row, ...patch });
    const number = index + 1;
    return <Card className="mb-3"><Card.Body><fieldset disabled={disabled}>
        <legend className="h6">来源 {number}</legend>
        <Row className="g-3">
            <Col md={6}><Form.Group controlId={`source-repo-${index}`}><Form.Label>来源仓 {number}</Form.Label>
                <Form.Select value={row.repository} onChange={(event) => change({ repository: event.target.value })}>
                    <option value="">请选择来源仓</option>
                    {row.repository && !repositories.includes(row.repository) && <option value={row.repository}>{row.repository}（当前未登记）</option>}
                    {repositories.map((id) => <option key={id} value={id}>{id}</option>)}
                </Form.Select></Form.Group></Col>
            <Col md={6}><Form.Group controlId={`source-kind-${index}`}><Form.Label>选择范围 {number}</Form.Label>
                <Form.Select value={row.kind} onChange={(event) => change({ kind: event.target.value as SourceRow['kind'] })}>
                    {row.kind === 'unmapped' && <option value="unmapped">原有来源（未声明交付路径）</option>}
                    <option value="prefix">目录</option><option value="file">单个文件</option>
                </Form.Select>
                {row.kind === 'unmapped' && <Form.Text>保留原有来源声明；改选目录或单文件后，才会新增文件交付范围。</Form.Text>}
                </Form.Group></Col>
            <Col md={6}><Form.Group controlId={`source-basis-${index}`}><Form.Label>来源版本 {number}</Form.Label>
                <Form.Select value={row.basis} onChange={(event) => change({ basis: event.target.value as SourceRow['basis'] })}>
                    <option value="branch">发布时固定分支版本</option><option value="commit">指定固定提交</option>
                </Form.Select></Form.Group></Col>
            <Col md={6}><Form.Group controlId={`source-ref-${index}`}><Form.Label>{row.basis === 'commit' ? '提交版本' : '分支引用'} {number}</Form.Label>
                <Form.Control value={row.basis === 'commit' ? row.commit : row.selector} placeholder={row.basis === 'branch' ? '留空使用默认发布分支' : '完整 commit'}
                    onChange={(event) => change(row.basis === 'commit' ? { commit: event.target.value } : { selector: event.target.value })} /></Form.Group></Col>
            <Col md={6}><Form.Group controlId={`source-path-${index}`}><Form.Label>来源路径 {number}</Form.Label>
                <Form.Control disabled={row.kind === 'unmapped'} value={row.source} placeholder={row.kind === 'prefix' ? '来源目录；留空选择整仓' : '来源文件路径'} onChange={(event) => change({ source: event.target.value })} /></Form.Group></Col>
            <Col md={6}><Form.Group controlId={`target-path-${index}`}><Form.Label>交付路径 {number}</Form.Label>
                <Form.Control disabled={row.kind === 'unmapped'} value={row.target} placeholder={row.kind === 'prefix' ? '交付目录；留空为根目录' : row.kind === 'file' ? '交付文件路径，可改名' : '未声明文件交付路径'} onChange={(event) => change({ target: event.target.value })} /></Form.Group></Col>
        </Row>
        <Button variant="outline-danger" size="sm" className="mt-3" onClick={remove}>移除来源 {number}</Button>
    </fieldset></Card.Body></Card>;
};
export default SourceEditor;
