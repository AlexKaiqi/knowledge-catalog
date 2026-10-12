import React from 'react';
import {Table} from 'react-bootstrap';
import {AllowRule} from '../../lib/api/kc';
import './grantRules.css';

const unrestricted = '不限制该维度';
const columns: {label: string; value: (rule: AllowRule) => string}[] = [
    {label: 'ID', value: rule => rule.id || '—'},
    {label: '主体', value: rule => rule.principal || '—'},
    {label: '动作', value: rule => rule.actions.join(', ') || '—'},
    ...(['repo', 'catalog', 'ref', 'object', 'aspect', 'dataset'] as const).map(key => ({label: key === 'repo' ? 'Repository' : key[0].toUpperCase() + key.slice(1), value: (rule: AllowRule) => rule[key] || unrestricted})),
    {label: '分享者', value: rule => rule.sharedBy || '—'},
    {label: '分享标识', value: rule => rule.shareId || '—'},
];
export const RuleDetails: React.FC<{rule: AllowRule}> = ({rule}) => <dl className="kc-grant-details">
    {columns.map(column => <React.Fragment key={column.label}><dt>{column.label}</dt><dd>{column.value(rule)}</dd></React.Fragment>)}
</dl>;

export const RuleTable: React.FC<{rules: AllowRule[]; label: string; action?: (rule: AllowRule) => React.ReactNode}> = ({rules, label, action}) => <Table className="kc-grant-table" striped bordered aria-label={label}>
    <colgroup>{action && <col className="kc-grant-operation"/>}<col className="kc-grant-identity"/><col className="kc-grant-actions"/><col/></colgroup>
    <thead><tr>{action && <th scope="col">操作</th>}<th scope="col">规则</th><th scope="col">动作</th><th scope="col">范围与来源</th></tr></thead>
    <tbody>{rules.map((rule, index) => <tr key={`${rule.id}:${index}`}>
        {action && <td>{action(rule)}</td>}
        <td><dl className="kc-grant-identity-details">{columns.slice(0, 2).map(column => <React.Fragment key={column.label}><dt>{column.label}</dt><dd>{column.value(rule)}</dd></React.Fragment>)}</dl></td>
        <td>{columns[2].value(rule)}</td>
        <td><dl className="kc-grant-scope">{columns.slice(3).map(column => <React.Fragment key={column.label}><dt>{column.label}</dt><dd>{column.value(rule)}</dd></React.Fragment>)}</dl></td>
    </tr>)}</tbody>
</Table>;
