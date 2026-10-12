import React, {useId, useState} from 'react';
import {Alert, Button, Form, Spinner} from 'react-bootstrap';
import {AllowRule, GrantRequest} from '../../lib/api/kc';
import './grantRules.css';

export const GrantField: React.FC<React.PropsWithChildren<{label: string; id: string; help?: string}>> = ({label, id, help, children}) =>
    <Form.Group controlId={id} className="kc-grant-field"><Form.Label>{label}</Form.Label><div>{children}{help && <Form.Text>{help}</Form.Text>}</div></Form.Group>;

const unrestricted = '不限制该维度';
const columns: {label: string; value: (rule: AllowRule) => string}[] = [
    {label: 'ID', value: rule => rule.id || '—'},
    {label: '主体', value: rule => rule.principal || '—'},
    {label: '动作', value: rule => rule.actions.join(', ') || '—'},
    ...(['repo', 'ref', 'object', 'aspect', 'dataset'] as const).map(key => ({label: key === 'repo' ? 'Repository' : key[0].toUpperCase() + key.slice(1), value: (rule: AllowRule) => rule[key] || unrestricted})),
    {label: '分享者', value: rule => rule.sharedBy || '—'},
    {label: '分享标识', value: rule => rule.shareId || '—'},
];
export const RuleDetails: React.FC<{rule: AllowRule}> = ({rule}) => <dl className="kc-grant-details">
    {columns.map(column => <React.Fragment key={column.label}><dt>{column.label}</dt><dd>{column.value(rule)}</dd></React.Fragment>)}
</dl>;


const GrantForm: React.FC<{initial?: AllowRule; readOnly?: boolean; pending?: boolean; own?: boolean; onSubmit?: (value: GrantRequest) => void; onCancel?: () => void}> = ({initial, readOnly = false, pending = false, own = false, onSubmit, onCancel}) => {
    const prefix = useId();
    const [principal, setPrincipal] = useState(initial?.principal ?? '');
    const [actions, setActions] = useState(initial?.actions.join(', ') ?? '');
    const [kind, setKind] = useState(initial?.repo ? 'repository' : initial?.dataset ? 'dataset' : initial ? 'global' : 'repository');
    const [repository, setRepository] = useState(initial?.repo ?? '');
    const [error, setError] = useState<string | null>(null);
    const [constraints, setConstraints] = useState({ref: initial?.ref ?? '', object: initial?.object ?? '', aspect: initial?.aspect ?? '', dataset: initial?.dataset ?? ''});
    if (readOnly && initial) return <RuleDetails rule={initial}/>;
    return <Form onSubmit={event => {
        event.preventDefault();
        const value: GrantRequest = {principal: principal.trim(), actions: [...new Set(actions.split(/[\s,]+/).filter(Boolean))]};
        if (kind === 'repository') value.repository = repository.trim();
        for (const [key, text] of Object.entries(constraints)) if (text.trim()) Object.assign(value, {[key]: text.trim()});
        setError(null);
        if (kind === 'global') delete value.dataset;
        if (kind === 'dataset' && !value.dataset) {setError('请填写 Dataset 范围。'); return;}
        if (!value.principal || !value.actions.length || (kind === 'repository') && !value.repository) {setError('请填写主体、至少一个动作和有效的资源范围。'); return;}
        onSubmit?.(value);
    }}>
        <fieldset disabled={pending}>{error && <Alert variant="danger">{error}</Alert>}
            <GrantField id={`${prefix}-principal`} label="授权主体"><Form.Control required readOnly={own} value={principal} placeholder="用户名" onChange={event => setPrincipal(event.target.value)}/></GrantField>
            <GrantField id={`${prefix}-actions`} label="允许动作" help="多个动作用逗号分隔，例如 file.read、dataset.resolve。"><Form.Control as="textarea" rows={2} required value={actions} placeholder="file.read, dataset.resolve" onChange={event => setActions(event.target.value)}/></GrantField>
            <GrantField id={`${prefix}-kind`} label="范围类型"><Form.Select value={kind} onChange={event => setKind(event.target.value)}>
                <option value="global">全服务</option><option value="repository">Repository</option><option value="dataset">Dataset</option>
            </Form.Select></GrantField>
            {kind === 'repository' && <GrantField id={`${prefix}-repo`} label="Repository 范围"><Form.Control required value={repository} placeholder="kr://organization/repository" onChange={event => setRepository(event.target.value)}/></GrantField>}
            {kind === 'dataset' && <GrantField id={`${prefix}-dataset-scope`} label="Dataset 范围"><Form.Control required value={constraints.dataset} onChange={event => setConstraints({...constraints, dataset: event.target.value})}/></GrantField>}
            <details className="kc-grant-advanced my-3" open={!!initial && Object.values(constraints).some(Boolean)}><summary>高级约束</summary>
                <p className="small text-muted mt-2">留空表示不限制。Ref、Aspect、Dataset 精确匹配，Object 使用路径 glob；Object 和 Aspect 不构成正文读 ACL。</p>
                {(['ref', 'object', 'aspect'] as const).map(key => <GrantField key={key} id={`${prefix}-${key}`} label={key[0].toUpperCase() + key.slice(1)}>
                    <Form.Control value={constraints[key]} onChange={event => setConstraints({...constraints, [key]: event.target.value})}/>
                </GrantField>)}
                {kind === 'repository' && <GrantField id={`${prefix}-dataset`} label="Dataset（可选）"><Form.Control value={constraints.dataset} onChange={event => setConstraints({...constraints, dataset: event.target.value})}/></GrantField>}
            </details>
            <div className="kc-grant-form-actions"><Button variant="success" type="submit" disabled={pending}>{pending && <Spinner as="span" animation="border" size="sm" className="me-2" aria-hidden="true"/>}{initial ? '保存修改' : '添加授权'}</Button>
                {onCancel && <Button variant="outline-secondary" onClick={onCancel}>取消</Button>}</div>
        </fieldset>
    </Form>;
};
export default GrantForm;
