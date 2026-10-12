import {useAPI} from "./api";
import {auth} from "../api";
import {useEffect, useState} from 'react';


const useUser = () => {
    const [version, setVersion] = useState(auth.currentUserVersion());
    useEffect(() => auth.subscribeCurrentUser(() => setVersion(auth.currentUserVersion())), []);
    const { response, loading, error } = useAPI(async () => ({version, user: await auth.getCurrentUserWithCache()}), [version]);
    return { user: response?.version === version ? response.user : null, loading, error };
}

export default useUser;
