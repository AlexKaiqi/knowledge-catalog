import * as duckdb from '@duckdb/duckdb-wasm';
import * as arrow from 'apache-arrow';
import {AsyncDuckDB, AsyncDuckDBConnection, DuckDBDataProtocol} from '@duckdb/duckdb-wasm';
import duckdb_wasm from '@duckdb/duckdb-wasm/dist/duckdb-mvp.wasm?url';
import mvp_worker from '@duckdb/duckdb-wasm/dist/duckdb-browser-mvp.worker.js?url';
import duckdb_wasm_eh from '@duckdb/duckdb-wasm/dist/duckdb-eh.wasm?url';
import eh_worker from '@duckdb/duckdb-wasm/dist/duckdb-browser-eh.worker.js?url';
import {API_ENDPOINT, objects, qs} from '../../../../lib/api';
import {userProof} from '../../../../lib/api/sessionProof';



const MANUAL_BUNDLES: duckdb.DuckDBBundles = {
    mvp: {
        mainModule: duckdb_wasm,
        mainWorker: mvp_worker,
    },
    eh: {
        mainModule: duckdb_wasm_eh,
        mainWorker: eh_worker,
    },
};

let _db: AsyncDuckDB | null = null;

async function getDuckDB(): Promise<duckdb.AsyncDuckDB> {
    if (_db !== null) {
        return _db
    }
    const bundle = await duckdb.selectBundle(MANUAL_BUNDLES)
    if (!bundle.mainWorker) {
        throw Error("could not initialize DuckDB")
    }
    const worker = new Worker(bundle.mainWorker)
    const logger = new duckdb.VoidLogger()
    const db = new duckdb.AsyncDuckDB(logger, worker)
    await db.instantiate(bundle.mainModule, bundle.pthreadWorker)
    const conn = await db.connect()
    await conn.close()
    _db = db
    return _db
}


// taken from @duckdb/duckdb-wasm/dist/types/src/bindings/tokens.d.ts
// which, unfortunately, we cannot import.
const DUCKDB_STRING_CONSTANT = 2;
const LAKEFS_URI_PATTERN = /^(['"]?)(lakefs:\/\/(.*))(['"])\s*$/;

// Read through the original API's current Repo authorization, including ranges.
async function extractFiles(conn: AsyncDuckDBConnection, sql: string): Promise<{ [name: string]: string }> {
    const tokenized = await conn.bindings.tokenize(sql)
    let prev = 0;
    const fileMap: { [name: string]: string } = {};
    tokenized.offsets.forEach((offset, i) => {
        let currentToken = sql.length;
        if (i < tokenized.offsets.length - 1) {
            currentToken = tokenized.offsets[i+1];
        }
        const part = sql.substring(prev, currentToken);
        prev = currentToken;
        if (tokenized.types[i] === DUCKDB_STRING_CONSTANT) {
            const matches = part.match(LAKEFS_URI_PATTERN)
            if (matches !== null) {
                const [repo, ref, ...path] = matches[3].split('/');
                if (!repo || !ref || !path.length) throw new Error('A lakefs URI requires repository, ref and file path');
                fileMap[matches[2]] = `${document.location.origin}${API_ENDPOINT}/repositories/${encodeURIComponent(repo)}/refs/${encodeURIComponent(ref)}/objects?${qs({path: path.join('/'), presign: false})}`;
            }
        }
    })
    return fileMap
}

/* eslint-disable  @typescript-eslint/no-explicit-any */
export async function runDuckDBQuery(sql: string):  Promise<arrow.Table<any>> {
    const db = await getDuckDB()
    /* eslint-disable  @typescript-eslint/no-explicit-any */
    let result: arrow.Table<any>
    const conn  = await db.connect()
    let fileNames: string[] = [];
    try {
        const fileMap = await extractFiles(conn, sql)
        fileNames = Object.getOwnPropertyNames(fileMap)
        await Promise.all(fileNames.map(
            async fileName => {
                if (!userProof.get()) return db.registerFileURL(fileName, fileMap[fileName], DuckDBDataProtocol.HTTP, true);
                const [repo, ref, ...path] = fileName.slice('lakefs://'.length).split('/');
                const content = await objects.blob(repo, ref, path.join('/'));
                return db.registerFileBuffer(fileName, new Uint8Array(await content.arrayBuffer()));
            }
        ))
        // execute the query
        result = await conn.query(sql)

    } finally {
        await Promise.all(fileNames.map(fileName => db.dropFile(fileName)))
        await conn.close()
    }
    return result
}
