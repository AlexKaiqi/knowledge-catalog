import React, {useEffect, useRef, useState} from 'react';
import {Alert, Modal} from 'react-bootstrap';
import {AllowRule, GrantRequest, kc} from '../../lib/api/kc';
import ErrorNotice from '../../lib/components/kcError';
import GrantForm from './grantForm';

const GrantEditor: React.FC<{rule: AllowRule | null; own?: boolean; onClose: () => void; onSaved: (rule: AllowRule) => void}> = ({rule, own = false, onClose, onSaved}) => {
    const [pending, setPending] = useState(false);
    const [error, setError] = useState<Error | null>(null);
    const inFlight = useRef<AbortController>();
    useEffect(() => {setError(null); setPending(false); return () => inFlight.current?.abort();}, [rule]);
    const save = async (request: GrantRequest): Promise<void> => {
        if (!rule || inFlight.current && !inFlight.current.signal.aborted) return;
        const controller = new AbortController(); inFlight.current = controller; setPending(true); setError(null);
        try {const saved = await kc.updateGrant(rule, request, controller.signal); if (!controller.signal.aborted) onSaved(saved);}
        catch (err) {if (!controller.signal.aborted) setError(err as Error);}
        finally {if (!controller.signal.aborted) {inFlight.current = undefined; setPending(false);}}
    };
    return <Modal show={!!rule} onHide={() => {if (!pending) onClose();}} size="lg" backdrop={pending ? 'static' : true} keyboard={!pending}>
        <Modal.Header closeButton={!pending}><Modal.Title>编辑授权</Modal.Title></Modal.Header><Modal.Body>
            {own && <Alert variant="warning">正在修改自己的授权，保存后可能失去部分操作或管理能力。</Alert>}
            {error && <ErrorNotice error={error}/>}
            {rule && <GrantForm key={rule.id} initial={rule} own={own} pending={pending} onSubmit={value => {void save(value);}} onCancel={onClose}/>}
        </Modal.Body>
    </Modal>;
};
export default GrantEditor;
