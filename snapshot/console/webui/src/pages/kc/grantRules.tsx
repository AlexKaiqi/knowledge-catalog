import React from 'react';
import {Badge, Card, Stack} from 'react-bootstrap';
import {AllowRule} from '../../lib/api/kc';
import './grantRules.css';

import GrantForm from './grantForm';
export {RuleDetails} from './grantForm';

export const RuleCards: React.FC<{rules: AllowRule[]; label: string; action?: (rule: AllowRule) => React.ReactNode}> = ({rules, label, action}) => <section aria-label={label}>
    {rules.map((rule, index) => <Card className="kc-grant-card mb-3" key={`${rule.id}:${index}`}>
        <Card.Header><Stack direction="horizontal" gap={2} className="flex-wrap"><strong>规则 {index + 1}</strong><Badge bg="light" text="body">{rule.repo ? 'Repository' : rule.dataset ? 'Dataset' : '全服务'}</Badge>
            <div className="ms-auto d-flex gap-2">{action?.(rule)}</div></Stack></Card.Header>
        <Card.Body><GrantForm initial={rule} readOnly/></Card.Body>
    </Card>)}
</section>;
