"""Offline contracts for the deployment capability and Writer probes."""
import importlib.util
import json
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

from accept_test import load


class CapabilityProbeTest(unittest.TestCase):
    def test_source_probe_rejects_a_skipped_or_failed_descendant_of_a_passing_parent(self):
        runner = load()
        from accept_world import World
        world = World({})
        for action, error in [('skip', runner.HarnessError), ('fail', runner.ContractFailure)]:
            events = [{'Test': 'TestSelected/counterexample', 'Action': action},
                      {'Test': 'TestSelected', 'Action': 'pass'}]
            result = SimpleNamespace(returncode=0, stdout='\n'.join(json.dumps(event) for event in events))
            with self.subTest(action=action), patch('accept_world.subprocess.run', return_value=result):
                with self.assertRaises(error):
                    world.step({'go': {'package': './internal/arch', 'tests': ['TestSelected']},
                                'expect': {'count': {'tests': 1}}})

    def test_dry_run_compares_writer_code_and_full_diagnostic_from_each_original_file(self):
        runner = load()
        from accept_world import World
        plan = runner.read_probes()['accept-index-dry-run-matches-writer']
        world = World({})
        world.dataset, world.branch = world.prefix+'case', 'refs/heads/probe'
        uploaded, written = {}, []
        def upload(path, relative):
            remote = '/tmp/'+relative
            uploaded[remote] = Path(path).read_text()
            return remote
        world.upload = upload
        corrupt = [False]
        codes = {'doesNotExist': 'OBJECT_REF_UNRESOLVED', 'instance': 'USAGE_INVALID', 'mapping': 'USAGE_INVALID'}
        def call(args, principal):
            if args[:2] == ['writer', 'head']:
                return 0, {'commit': 'fixed'}
            if args[:3] == ['plugin', 'declarative-index', 'trial']:
                content = uploaded[args[args.index('--file')+1]]
                coordinate = 'doesNotExist' if 'doesNotExist' in content else 'instance' if 'instance:' in content else 'mapping'
                written.append(coordinate)
                return 0, {'members': [{'status': 'invalid', 'code': codes[coordinate],
                                       'reason': codes[coordinate]+': invalid position: '+coordinate}]}
            if args[:2] == ['writer', 'put']:
                return 1, {'error': {'code': codes[written[-1]],
                                    'message': 'invalid position: '+('wrong' if corrupt[0] else written[-1])}}
            self.fail('unexpected product command: '+str(args))
        world.call = call
        for incorrect in (False, True):
            corrupt[0] = incorrect
            world.saved = {}
            with tempfile.TemporaryDirectory() as temp:
                world.directory = type('Directory', (), {'name': temp})()
                def execute_plan():
                    for step in plan['steps']:
                        world.step(step)
                if incorrect:
                    with self.assertRaises(runner.ContractFailure):
                        execute_plan()
                else:
                    execute_plan()
        self.assertEqual(written[:3], ['doesNotExist', 'instance', 'mapping'])

    def test_trial_uses_expanded_original_material_at_the_probe_ref(self):
        runner = load()
        from accept_world import World
        world = World({})
        world.dataset, world.branch = world.prefix+'case', 'refs/heads/probe'
        calls, uploaded = [], []
        world.upload = lambda path, relative: uploaded.append(Path(path).read_text()) or '/tmp/probe.yaml'
        world.execute = lambda args, principal, expected: calls.append((args, principal, expected)) or {'members': []}
        material = '$materials/acceptance/sales-analysis/indexes/sales-search/table-name.yaml'
        with tempfile.TemporaryDirectory() as temp:
            world.directory = type('Directory', (), {'name': temp})()
            world.step({'trial': material, 'repo': 'sales-analysis', 'expect': {'count': {'members': 1}}})
        self.assertEqual(uploaded, [world.expand(runner.material_path(runner.QINGHE/'_materials', material).read_text())])
        self.assertEqual(calls, [(['plugin', 'declarative-index', 'trial', '--repo', 'sales-analysis',
                                  '--ref', 'refs/heads/probe', '--file', '/tmp/probe.yaml'],
                                 'admin', {'count': {'members': 1}})])

    def test_capability_probe_asserts_every_member_rebuild_and_no_side_effect(self):
        runner = load()
        probe = runner.read_probes()['accept-search-capability-catalog']
        trials = {step['trial']: step for step in probe['steps'] if 'trial' in step}
        root = '$materials/acceptance/'
        for key in ('table-name', 'metric-text', 'table-row-count', 'metric-unit'):
            expected = trials[root+'sales-analysis/indexes/sales-search/'+key+'.yaml']['expect']
            runner.check({'members': [{'status': 'supported'}]}, expected)
            with self.assertRaises(runner.ContractFailure):
                runner.check({'members': [{'status': 'unsupported', 'reason': 'not implemented'}]}, expected)
        unsupported = trials[root+'variants/index/table-name-unsupported-analyzer.yaml']['expect']
        with self.assertRaises(runner.ContractFailure):
            runner.check({'members': [{'status': 'supported'}]}, unsupported)
        boost = trials[root+'variants/index/table-name-boost.yaml']['expect']
        with self.assertRaises(runner.ContractFailure):
            runner.check({'members': [{'status': 'supported', 'rebuild': True}]}, boost)
        snapshots = [step for step in probe['steps'] if 'searchSnapshot' in step]
        self.assertEqual(len(snapshots), 2)
        comparisons = [step['assert'] for step in probe['steps'] if 'assert' in step]
        self.assertIn({'actual': '$saved.afterHead.commit', 'equals': '$saved.beforeHead.commit'}, comparisons)
        self.assertIn({'actual': '$saved.afterSearch', 'equals': '$saved.beforeSearch'}, comparisons)

    def test_owned_snapshot_is_read_only_and_detects_writes_and_new_generations(self):
        import hashlib
        runner = load()
        script = runner.REPO/'scripts/accept_search_resources.py'
        spec = importlib.util.spec_from_file_location('accept_resources_snapshot', script)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        prefix = 'accept-0123456789abcdef-'
        repository = prefix+'sales-analysis'
        digest = hashlib.sha256(repository.encode()).hexdigest()[:24]
        owned = 'kc-proj-'+digest+'-g-one'
        foreign = 'kc-proj-'+'e'*24+'-g-other'
        state = {owned: {'uuid': 'one', 'primaries': {'indexing': {'index_total': 3, 'delete_total': 0}}},
                 foreign: {'uuid': 'other', 'primaries': {'indexing': {'index_total': 4}}}}
        control = {'_id': hashlib.sha256(repository.encode()).hexdigest(),
                   '_version': 1, '_seq_no': 0, '_primary_term': 1, '_source': {'repository': repository}}
        calls = []
        def request(endpoint, method, path, body=None):
            calls.append((method, path))
            if path.startswith('/_cat/indices/kc-projection-control-'):
                return [{'index': 'kc-projection-control-v1'}]
            if method == 'POST':
                self.assertEqual(path, '/kc-projection-control-v1/_search')
                self.assertEqual(body['query'], {'bool': {'should': [
                    {'terms': {'repository': [repository]}},
                    {'prefix': {'repository': repository+'@'}}], 'minimum_should_match': 1}})
                if 'error' in control:
                    raise OSError(control['error']['type'])
                return {'hits': {'total': {'value': 1, 'relation': 'eq'},
                                 'hits': [json.loads(json.dumps(control))]}}
            self.assertEqual((method, path), ('GET', '/_stats/indexing,docs?level=indices'))
            return {'indices': json.loads(json.dumps(state))}
        module.request = request
        before = module.snapshot('http://unused.invalid', prefix, [repository])
        state[foreign]['primaries']['indexing']['index_total'] += 1
        self.assertEqual(before, module.snapshot('http://unused.invalid', prefix, [repository]))
        control['_seq_no'] += 1
        self.assertNotEqual(before, module.snapshot('http://unused.invalid', prefix, [repository]),
                            'an owned control write must be visible even without generation writes')
        control['_seq_no'] -= 1
        state[owned]['primaries']['indexing']['index_total'] += 1
        self.assertNotEqual(before, module.snapshot('http://unused.invalid', prefix, [repository]))
        state['kc-proj-'+digest+'-g-new'] = {'uuid': 'new', 'primaries': {'indexing': {}}}
        self.assertEqual(len(module.snapshot('http://unused.invalid', prefix, [repository])['generations']), 2)
        self.assertTrue(all(method == 'GET' or method == 'POST' and path.endswith('/_search')
                            for method, path in calls))
        control['error'] = {'type': 'unavailable_shards_exception'}
        with self.assertRaises(OSError):
            module.snapshot('http://unused.invalid', prefix, [repository])
        module.request = lambda *args: self.fail('forged ownership must be refused before HTTP')
        with self.assertRaises(ValueError):
            module.snapshot('http://unused.invalid', prefix, ['business'])


if __name__ == '__main__':
    unittest.main()
