import React from 'react';
import LakeFSApp from './lakefsApp';
import { PluginManager } from './plugins/pluginManager';

const plugins = new PluginManager();
const KCApp: React.FC = () => <LakeFSApp pluginManager={plugins} />;
export default KCApp;
