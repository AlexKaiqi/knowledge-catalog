import React, {useContext} from 'react';
import ReactDiffViewer, {DiffMethod} from 'react-diff-viewer-continued';
import {AppContext} from '../../hooks/appContext';

// Shared by native object comparisons and Dataset publication definitions.
const TextDiff: React.FC<{before: string; after: string}> = ({before, after}) => {
    const {state} = useContext(AppContext);
    return <ReactDiffViewer oldValue={before} newValue={after} splitView={false}
        useDarkTheme={state.settings.darkMode} compareMethod={DiffMethod.WORDS}/>;
};
export default TextDiff;
