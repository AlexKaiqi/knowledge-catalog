"""Observe and remove only projections proved by this acceptance invocation.

Known Dataset IDs own their derived corpus identities. Known source Repository
IDs own their store-qualified identities. A control record proves each physical
prefix; its CAS and each generation UUID are checked before exact-name deletion.
The shared control index and unrecorded Dataset identities always survive.
"""
import hashlib
from pathlib import Path
import re
import sys
import urllib.parse

SCRIPTS = Path(__file__).resolve().parent
if str(SCRIPTS) not in sys.path:
    sys.path.insert(0, str(SCRIPTS))
from opensearch_fixture import request  # noqa: E402

RUN_PREFIX = re.compile(r'accept-[0-9a-f]{16}-\Z')
CONTROL_INDEX = re.compile(r'kc-projection-control-v1(?:-[0-9a-f]{24})?\Z')
STORE_SUFFIX = re.compile(r'@[0-9a-f]{64}(?:@[^\s/#?]+)?(?:#state)?\Z')


def owned_patterns(prefix, repositories, datasets=()):
    if not RUN_PREFIX.fullmatch(prefix) or not repositories:
        raise ValueError('acceptance search cleanup requires an owned run prefix and repositories')
    if any(not value.startswith(prefix) or not re.fullmatch(r'[a-z0-9-]+', value)
           for value in [*repositories, *datasets]):
        raise ValueError('acceptance search cleanup refuses identities outside this invocation')
    identities = {repository: hashlib.sha256(repository.encode()).hexdigest() for repository in repositories}
    patterns = [re.compile(r'kc-proj-(?:[0-9a-f]{24}-)?'+digest[:24]+r'-g-[A-Za-z0-9-]+\Z')
                for digest in identities.values()]
    return identities, patterns


def owns(repository, repositories, datasets):
    for source in repositories:
        if repository == source or repository.startswith(source) and STORE_SUFFIX.fullmatch(repository[len(source):]):
            return True
    for dataset in datasets:
        stem = 'dataset-index/'+dataset+'/'
        if not repository.startswith(stem):
            continue
        scope, separator, suffix = repository[len(stem):].partition('@')
        if re.fullmatch('[0-9a-f]{64}', scope) and separator and suffix.startswith(scope):
            tail = suffix[len(scope):]
            if tail in ('', '#state'):
                return True
    return False


def generation_pattern(index, identity):
    namespace = index[len('kc-projection-control-v1'):]
    prefix = 'kc-proj-'+(namespace[1:]+'-' if namespace else '')+identity[:24]+'-g-'
    return re.compile(re.escape(prefix)+r'[A-Za-z0-9-]+\Z')


def owned_controls(endpoint, repositories, datasets):
    controls = request(endpoint, 'GET', '/_cat/indices/kc-projection-control-*?format=json&h=index&expand_wildcards=all')
    result = []
    clauses = [{'terms': {'repository': list(repositories)}}]
    clauses += [{'prefix': {'repository': source+'@'}} for source in repositories]
    clauses += [{'prefix': {'repository': 'dataset-index/'+dataset+'/'}} for dataset in datasets]
    for index in sorted({row['index'] for row in controls}):
        if not CONTROL_INDEX.fullmatch(index):
            continue
        body = {'size': 10000, 'version': True, 'seq_no_primary_term': True,
                'query': {'bool': {'should': clauses, 'minimum_should_match': 1}}}
        matches = request(endpoint, 'POST', '/'+index+'/_search', body)
        hits = matches['hits']['hits']
        total = matches['hits']['total']
        if total['relation'] != 'eq' or total['value'] != len(hits):
            raise OSError('search control ownership response was truncated')
        for hit in hits:
            repository = hit['_source'].get('repository', '')
            if not owns(repository, repositories, datasets):
                continue
            identity = hashlib.sha256(repository.encode()).hexdigest()
            if hit['_id'] != identity:
                raise ValueError('search control response does not prove exact identity ownership')
            pattern = generation_pattern(index, identity)
            active = hit['_source'].get('active_index')
            if active and not pattern.fullmatch(active):
                raise ValueError('search control active generation does not match its identity and namespace')
            for coordinate in ('_seq_no', '_primary_term', '_version'):
                if not isinstance(hit.get(coordinate), int):
                    raise ValueError('search control response lacks ownership CAS coordinates')
            result.append((index, hit, pattern))
    return result


def snapshot(endpoint, prefix, repositories, datasets=()):
    """Observe corpus/source generations and write counters without any writes."""
    owned_patterns(prefix, repositories, datasets)
    controls = owned_controls(endpoint, repositories, datasets)
    indices = request(endpoint, 'GET', '/_stats/indexing,docs?level=indices')['indices']
    generations = {name: {'uuid': value['uuid'],
                   'docs': value['primaries'].get('docs', {}),
                   'writes': {key: value['primaries']['indexing'].get(key, 0)
                              for key in ('index_total', 'delete_total')}}
            for name, value in sorted(indices.items())
            if any(pattern.fullmatch(name) for _, _, pattern in controls)}
    documents = {}
    for index, hit, _ in controls:
        documents.setdefault(index, {})[hit['_id']] = {
            key: hit[key] for key in ('_version', '_seq_no', '_primary_term')}
    return {'generations': generations, 'control': documents}


def check_uuid(endpoint, name, expected):
    settings = request(endpoint, 'GET', '/'+name+'/_settings?flat_settings=true')
    if settings[name]['settings'].get('index.uuid') != expected:
        raise ValueError('owned search generation UUID changed; refused exact-name deletion')


def cleanup(endpoint, prefix, repositories, datasets=()):
    owned_patterns(prefix, repositories, datasets)
    controls = owned_controls(endpoint, repositories, datasets)
    indices = request(endpoint, 'GET', '/_all/_settings?expand_wildcards=all&flat_settings=true')
    removed = {'indices': [], 'controlDocuments': []}
    for index, hit, pattern in controls:
        names = {name: value['settings']['index.uuid'] for name, value in indices.items() if pattern.fullmatch(name)}
        path = '/'+index+'/_doc/'+hit['_id']
        current = request(endpoint, 'GET', path)
        if any(current.get(key) != hit[key] for key in ('_seq_no', '_primary_term', '_source')):
            raise ValueError('owned search control changed after observation; refused deletion')
        for name, expected in names.items():
            check_uuid(endpoint, name, expected)
        claimed = request(endpoint, 'DELETE', path+'?if_seq_no='+str(hit['_seq_no'])+
                          '&if_primary_term='+str(hit['_primary_term'])+'&refresh=true')
        if claimed.get('result') != 'deleted':
            raise OSError('owned search control CAS delete was not acknowledged')
        removed['controlDocuments'].append({'index': index, 'id': hit['_id']})
        for name, expected in sorted(names.items()):
            check_uuid(endpoint, name, expected)
            reply = request(endpoint, 'DELETE', '/'+urllib.parse.quote(name, safe=''))
            if reply.get('acknowledged') is not True:
                raise OSError('owned search generation exact-name delete was not acknowledged')
            removed['indices'].append(name)
    return removed
