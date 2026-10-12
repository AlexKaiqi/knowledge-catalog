import React, {FC, useContext, useState} from "react";
import { Outlet, useOutletContext } from "react-router-dom";
import { StorageConfigProvider } from "../hooks/storageConfig";
import TopNav from './navbar';
import { AppContext } from "../hooks/appContext";

type LayoutOutletContext = [(isLoggedIn: boolean) => void];

const Layout: FC<React.PropsWithChildren<{logged: boolean; storage?: boolean}>> = ({ logged, storage = true, children }) => {
    const [isLogged, setIsLogged] = useState(logged ?? true);

    // handle global dark mode here
    const {state} = useContext(AppContext);
    document.documentElement.setAttribute('data-bs-theme', state.settings.darkMode ? 'dark' : 'light')

    return (
        <>
            <TopNav logged={isLogged}/>
            <div className="main-app">
                {storage ? <StorageConfigProvider>
                    {children ?? <Outlet context={[setIsLogged] satisfies LayoutOutletContext}/>}
                </StorageConfigProvider> : children}
            </div>
        </>
    );
};

export function useLayoutOutletContext() {
    return useOutletContext<LayoutOutletContext>();
}

export default Layout;
