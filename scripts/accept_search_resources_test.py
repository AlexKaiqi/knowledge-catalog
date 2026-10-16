"""No-service regression for deployment acceptance search-resource ownership."""
import copy
import hashlib
import importlib.util
from pathlib import Path
import unittest

SCRIPT = Path(__file__).with_name('accept_search_resources.py')
PREFIX = 'accept-0123456789abcdef-'
REPOSITORY = PREFIX+'table-meta'
DATASET = PREFIX+'analysis'
CONTROL = 'kc-projection-control-v1-'+'a'*24


def identity(repository):
    return hashlib.sha256(repository.encode()).hexdigest()


def hit(repository):
    digest = identity(repository)
    return {'_id': digest, '_seq_no': 3, '_primary_term': 1, '_version': 4,
            '_source': {'repository': repository, 'active_index': 'kc-proj-'+'a'*24+'-'+digest[:24]+'-g-owned'}}


class AcceptanceSearchOwnershipTest(unittest.TestCase):
    def module(self):
        spec = importlib.util.spec_from_file_location('accept_resources', SCRIPT)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module

    def fixture(self, documents, change_uuid=False, cas_failure=False):
        module, calls = self.module(), []
        indices = {row['_source']['active_index']: 'uuid-'+row['_id'] for row in documents}
        indices['kc-proj-'+'b'*24+'-g-business'] = 'business-uuid'
        def request(url, method, path, body=None):
            calls.append((method, path, body))
            if path.startswith('/_cat/indices/kc-projection-control-'):
                return [{'index': CONTROL}]
            if path.startswith('/_stats/'):
                return {'indices': {name: {'uuid': uid, 'primaries': {'docs': {'count': 1},
                        'indexing': {'index_total': 1, 'delete_total': 0}}} for name, uid in indices.items()}}
            if path.startswith('/_all/_settings'):
                return {name: {'settings': {'index.uuid': uid}} for name, uid in indices.items()}
            if method == 'POST':
                return {'hits': {'total': {'value': len(documents), 'relation': 'eq'}, 'hits': copy.deepcopy(documents)}}
            if method == 'GET' and '/_doc/' in path:
                return copy.deepcopy(next(row for row in documents if row['_id'] == path.split('/_doc/')[1]))
            if method == 'GET' and path.endswith('/_settings?flat_settings=true'):
                name = path[1:].split('/')[0]
                return {name: {'settings': {'index.uuid': 'replacement' if change_uuid else indices[name]}}}
            if method == 'DELETE' and '/_doc/' in path:
                if cas_failure:
                    raise OSError('HTTP 409 control CAS changed')
                return {'result': 'deleted'}
            if method == 'DELETE':
                return {'acknowledged': True}
            self.fail('unexpected request '+method+' '+path)
        module.request = request
        return module, calls

    def test_business_acceptance_prefix_matches_only_the_current_exact_run(self):
        module = self.module()
        module.owned_patterns(PREFIX, [REPOSITORY])
        for invalid in ('kc-'+PREFIX, 'accept-*', 'accept-0123456789abcdef'):
            with self.assertRaises(ValueError):
                module.owned_patterns(invalid, [invalid+'table-meta'])

    def test_source_and_named_dataset_corpus_control_prove_exact_cleanup(self):
        scope = 'd'*64
        owners = [REPOSITORY, REPOSITORY+'@'+'c'*64, REPOSITORY+'@'+'c'*64+'@c2#state',
                  'dataset-index/'+DATASET+'/'+scope+'@'+scope,
                  'dataset-index/'+DATASET+'/'+scope+'@'+scope+'#state']
        foreign = hit('dataset-index/'+PREFIX+'unrecorded/'+scope+'@'+scope)
        module, calls = self.fixture([*(hit(owner) for owner in owners), foreign])
        removed = module.cleanup('http://unused.invalid', PREFIX, [REPOSITORY], [DATASET])
        self.assertEqual(set(removed['indices']), {hit(owner)['_source']['active_index'] for owner in owners})
        deletions = [path for method, path, _ in calls if method == 'DELETE']
        self.assertEqual(len(deletions), 2*len(owners))
        self.assertTrue(all('if_seq_no=3&if_primary_term=1' in path for path in deletions if '/_doc/' in path))
        self.assertFalse(any('*' in path or ',' in path for path in deletions))
        self.assertNotIn('/'+CONTROL, deletions, 'the shared control index must survive')
        self.assertFalse(any(foreign['_id'] in path for path in deletions))
        self.assertLess(next(i for i, path in enumerate(deletions) if '/_doc/' in path),
                        next(i for i, path in enumerate(deletions) if '/_doc/' not in path))

    def test_snapshot_observes_corpus_and_source_without_shared_writes(self):
        scope = 'd'*64
        records = [hit(REPOSITORY+'@'+'c'*64), hit('dataset-index/'+DATASET+'/'+scope+'@'+scope)]
        module, calls = self.fixture(records)
        value = module.snapshot('http://unused.invalid', PREFIX, [REPOSITORY], [DATASET])
        self.assertEqual(set(value['generations']), {row['_source']['active_index'] for row in records})
        self.assertEqual(set(value['control'][CONTROL]), {row['_id'] for row in records})
        self.assertFalse(any(method in ('DELETE', 'PUT') for method, _, _ in calls))

    def test_forged_identity_or_changed_uuid_never_deletes_an_index(self):
        forged = hit(REPOSITORY)
        forged['_id'] = 'e'*64
        for records, changed, failure in [([forged], False, False), ([hit(REPOSITORY)], True, False),
                                          ([hit(REPOSITORY)], False, True)]:
            module, calls = self.fixture(records, changed, failure)
            with self.subTest(changed=changed, failure=failure), self.assertRaises((ValueError, OSError)):
                module.cleanup('http://unused.invalid', PREFIX, [REPOSITORY], [DATASET])
            self.assertFalse(any(method == 'DELETE' and '/_doc/' not in path for method, path, _ in calls))

    def test_control_change_after_observation_prevents_any_delete(self):
        module, calls = self.fixture([hit(REPOSITORY)])
        original = module.request
        def request(url, method, path, body=None):
            value = original(url, method, path, body)
            if method == 'GET' and '/_doc/' in path:
                value['_seq_no'] += 1
            return value
        module.request = request
        with self.assertRaisesRegex(ValueError, 'changed'):
            module.cleanup('http://unused.invalid', PREFIX, [REPOSITORY])
        self.assertFalse(any(method == 'DELETE' for method, _, _ in calls))

    def test_unknown_or_forged_ownership_is_rejected_before_http(self):
        module = self.module()
        module.request = lambda *args: self.fail('invalid ownership must never access search')
        for prefix, repositories, datasets in [('', ['business'], []), ('accept-*', [], []),
            (PREFIX, ['business'], []), (PREFIX, [], []), (PREFIX, [REPOSITORY], ['business'])]:
            with self.subTest(prefix=prefix, repositories=repositories, datasets=datasets), self.assertRaises(ValueError):
                module.cleanup('http://unused.invalid', prefix, repositories, datasets)


if __name__ == '__main__':
    unittest.main()
