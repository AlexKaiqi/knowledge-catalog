import React, {useEffect, useRef, useState} from 'react';
import {Alert, Button, Card, Form, Modal, Spinner} from 'react-bootstrap';
import {AllowRule, GrantRequest, GrantScope, kc, KCError} from '../../lib/api/kc';
import {useAuth, useKCQuery} from '../../lib/hooks/kcAuth';
import {Link} from '../../lib/components/nav';
import ErrorNotice from '../../lib/components/kcError';
import {RuleDetails, RuleCards} from './grantRules';
import GrantForm, {GrantField} from './grantForm';
import GrantEditor from './grantEditor';

type ScopeKind = 'global' | 'repository' | 'dataset';
type Receipt = {action: '添加' | '撤销' | '修改'; rule: AllowRule};
const GrantsPage: React.FC = () => {
    const {user} = useAuth();
    const [readKind, setReadKind] = useState<ScopeKind>('global');
    const [readResource, setReadResource] = useState('');
    const [readDataset, setReadDataset] = useState('');
    const [readScope, setReadScope] = useState<GrantScope>({});
    const [readError, setReadError] = useState<Error | null>(null);
    const list = useKCQuery(signal => kc.grants(readScope, signal), [JSON.stringify(readScope)]);
    const [filter, setFilter] = useState('');
    const [editing, setEditing] = useState<AllowRule | null>(null);
    const [target, setTarget] = useState<AllowRule | null>(null);
    const [pending, setPending] = useState(false);
    const [error, setError] = useState<Error | null>(null);
    const [receipt, setReceipt] = useState<Receipt | null>(null);
    const inFlight = useRef<AbortController | null>(null);
    useEffect(() => () => {inFlight.current?.abort();}, []);

    async function mutate(operation: (signal: AbortSignal) => Promise<Receipt>) {
        if (inFlight.current) return;
        const controller = new AbortController(); inFlight.current = controller;
        setPending(true); setError(null); setReceipt(null);
        try {
            const result = await operation(controller.signal);
            if (controller.signal.aborted) return;
            setReceipt(result); setTarget(null); list.reload();
        } catch (error) {if (!controller.signal.aborted) setError(error as Error);}
        finally {if (!controller.signal.aborted) {inFlight.current = null; setPending(false);}}
    }
    function add(request: GrantRequest) {void mutate(async signal => ({action: '添加', rule: await kc.addGrant(request, signal)}));}
    function remove() {
        if (!target) return;
        const selected = target;
        void mutate(async signal => {
            const result = await kc.removeGrant(selected.id, signal);
            return {action: '撤销', rule: {...selected, id: result.revoked}};
        });
    }
    const allRules = list.data?.rules ?? [];
    const seenIDs = new Set<string>(), duplicateIDs = new Set<string>();
    for (const rule of allRules) {
        if (seenIDs.has(rule.id)) duplicateIDs.add(rule.id);
        seenIDs.add(rule.id);
    }
    const rules = allRules.filter(rule => JSON.stringify(rule).toLowerCase().includes(filter.toLowerCase()));
    const denied = list.error instanceof KCError && list.error.status === 403;
    return <>
        <h1 className="h3">授权管理</h1>
        <p>这里管理 KC 的显式授权规则，与 lakeFS 权限独立。读取权限和修改权限分别由服务端判断。</p>
        {receipt && <Alert variant="success"><strong>授权已{receipt.action}。</strong><RuleDetails rule={receipt.rule}/></Alert>}
        <Form className="mb-3" onSubmit={event => {
            event.preventDefault();
            if (readKind !== 'global' && !readResource.trim()) {setReadError(new Error('请填写读取范围的资源标识。')); return;}
            setReadError(null);
            setReadScope(readKind === 'global' ? {} : {[readKind]: readResource.trim(), ...(readDataset.trim() ? {dataset: readDataset.trim()} : {})});
            list.reload();
        }}>
            <GrantField id="grant-read-kind" label="读取范围"><Form.Select value={readKind} onChange={event => {setReadKind(event.target.value as ScopeKind); setReadResource(''); setReadDataset('');}}>
                <option value="global">全服务</option><option value="dataset">Dataset</option><option value="repository">Repository</option>
            </Form.Select></GrantField>
            {readKind !== 'global' && <>
                <GrantField id="grant-read-resource" label={readKind === 'dataset' ? '读取 Dataset' : '读取 Repository'}><Form.Control required value={readResource} onChange={event => setReadResource(event.target.value)}/></GrantField>
                {readKind === 'repository' && <GrantField id="grant-read-dataset" label="读取 Dataset（可选）"><Form.Control value={readDataset} onChange={event => setReadDataset(event.target.value)}/></GrantField>}
            </>}
            <Button type="submit" className="mt-2" disabled={list.loading}>加载规则</Button>
        </Form>
        {readError && <ErrorNotice error={readError}/>}
        {list.loading && <Spinner animation="border" aria-label="正在读取授权规则"/>}
        {list.error && <><ErrorNotice error={list.error}/>{denied && <p>当前账号无法读取这个范围的 KC 授权规则。请选择获准管理的范围。</p>}<Link href="/auth/permissions">返回我的权限</Link></>}
        {list.data && <>
            <GrantField id="grant-filter" label="筛选已加载规则"><Form.Control value={filter} onChange={event => setFilter(event.target.value)}/></GrantField>
            <p>筛选仅作用于已加载的规则，不是权限求值。</p>
            {duplicateIDs.size > 0 && <Alert variant="warning">发现重复授权标识，相关规则无法明确撤销。请联系管理员显式修复授权账。</Alert>}
            <RuleCards label="授权规则" rules={rules} action={rule => <><Button variant="outline-primary" size="sm" disabled={pending || duplicateIDs.has(rule.id) || !!rule.shareId || rule.canManage === false} onClick={() => setEditing(rule)} aria-label={`编辑 ${rule.id}`}>编辑</Button><Button variant="outline-danger" disabled={pending || duplicateIDs.has(rule.id) || rule.canManage === false} onClick={() => {setError(null); setTarget(rule);}} aria-label={`撤销 ${rule.id}`}>撤销</Button></>}/>
            {rules.length === 0 && <p>没有匹配的授权规则。</p>}
        </>}
        <h2>添加授权</h2>
        <p>即使无法读取规则，也可以提交已获准范围的授权；是否允许由服务端最终判断。</p>
        {error && <><ErrorNotice error={error}/>{error instanceof KCError && error.status === 403 && <Link href="/auth/permissions">返回我的权限</Link>}</>}
        <Card className="mb-4"><Card.Body><GrantForm pending={pending} onSubmit={add}/></Card.Body></Card>
        <GrantEditor rule={editing} own={editing?.principal === user?.principal} onClose={() => setEditing(null)} onSaved={rule => {setEditing(null); setReceipt({action: '修改', rule}); list.reload();}}/>
        <Modal show={!!target} onHide={() => {if (!pending) setTarget(null);}} backdrop={pending ? 'static' : true} keyboard={!pending}>
            <Modal.Header closeButton={!pending}><Modal.Title>确认撤销授权</Modal.Title></Modal.Header>
            <Modal.Body>{target && <><RuleDetails rule={target}/>{target.principal === user?.principal && <Alert variant="warning">撤销的是当前账号的规则，可能影响后续操作，包括授权管理。撤销后不会自动恢复。</Alert>}</>}{error && <ErrorNotice error={error}/>}</Modal.Body>
            <Modal.Footer><Button variant="secondary" disabled={pending} onClick={() => setTarget(null)}>取消</Button><Button variant="danger" disabled={pending} onClick={remove}>确认撤销</Button></Modal.Footer>
        </Modal>
    </>;
};
export default GrantsPage;
