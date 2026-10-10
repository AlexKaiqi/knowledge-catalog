import React from 'react';
import { Outlet } from 'react-router-dom';
import Layout from '../lib/components/layout';
import { AuthProvider, useAuth } from '../lib/hooks/kcAuth';
import { WithLoginConfigContext } from '../lib/hooks/conf';
import {KCAuthContent} from './kcAuthGate';

const DatasetBody: React.FC = () => {
    const {user} = useAuth();
    return <Layout key={user?.principal || 'connecting'} logged storage={false}>
        <KCAuthContent><main key={user?.principal}><Outlet/></main></KCAuthContent>
    </Layout>;
};
const DatasetLayout: React.FC = () => <WithLoginConfigContext><AuthProvider><DatasetBody/></AuthProvider></WithLoginConfigContext>;
export default DatasetLayout;
