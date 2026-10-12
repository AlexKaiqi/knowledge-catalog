import React from 'react';
import {Button, Spinner} from 'react-bootstrap';
import {kc} from '../../lib/api/kc';
import {useKCQuery} from '../../lib/hooks/kcAuth';
import ErrorNotice from '../../lib/components/kcError';
import {RuleTable} from './grantRules';

function applicationURL(value?: string): string | undefined {
    if (!value || !/^https?:\/\//i.test(value)) return;
    try {
        const url = new URL(value);
        if ((url.protocol === 'https:' || url.protocol === 'http:') && !url.username && !url.password) return url.href;
    } catch { /* Invalid deployment URLs are shown without an executable link. */ }
}

const PermissionsPage: React.FC = () => {
    const {data, loading, error, reload} = useKCQuery(kc.admission);
    const url = applicationURL(data?.request.url);
    return <>
        <h1>我的权限</h1>
        <p>KC 权限与 lakeFS 权限独立。下面的显式授权列表不是有效权限求值，也不包含公开默认读取等规则；实际操作由服务端判断。</p>
        {loading && <Spinner animation="border" aria-label="正在读取权限"/>}
        {error && <ErrorNotice error={error}/>}
        <Button variant="outline-secondary" onClick={reload} disabled={loading}>刷新权限</Button>
        {data && <>
            <p className="mt-3">当前主体：<strong>{data.principal}</strong></p>
            <RuleTable label="显式授权规则" rules={data.grants.map(grant => ({...grant, principal: data.principal, repo: grant.repository}))}/>
            {data.grants.length === 0 && <p>没有显式授权规则。该列表不代表所有可用操作。</p>}
            <p>Object 和 Aspect 约束不构成对象或字段正文的读 ACL。</p>
            <h2>申请权限</h2>
            {url ? <p><a href={url} target="_blank" rel="noopener noreferrer">申请权限</a>（外部申请入口）</p> : <p>未提供可用的外部申请链接，请联系管理员。</p>}
            <p>KC 不在此保存申请或审批状态。</p>
            <h3>授权管理员</h3>
            {data.request.administrators.length ? <ul>{data.request.administrators.map(principal => <li key={principal}>{principal}</li>)}</ul> : <p>服务未列出授权管理员。</p>}
        </>}
    </>;
};
export default PermissionsPage;
