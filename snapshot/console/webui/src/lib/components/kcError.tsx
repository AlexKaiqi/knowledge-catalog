import React from 'react';
import { Alert } from 'react-bootstrap';
import { KCError } from '../api/kc';

const ErrorNotice: React.FC<{ error: Error }> = ({ error }) => (
    <Alert variant="danger" role="alert">
        {error instanceof KCError && <strong>{error.code} · </strong>}{error.message}
        {error instanceof KCError && error.code === 'FORBIDDEN' && <div>当前账号没有执行此操作的权限，请联系管理员。</div>}
    </Alert>
);
export default ErrorNotice;
