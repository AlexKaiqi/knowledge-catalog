"""Offline regressions for the deployment-only acceptance runner."""
import importlib.util
import copy
import io
import json
import os
import subprocess
from pathlib import Path
from types import SimpleNamespace
import tempfile
import sys
import unittest
from unittest.mock import patch

HERE = Path(__file__).resolve().parent


def load():
    if 'accept' in sys.modules:
        return sys.modules['accept']
    spec = importlib.util.spec_from_file_location('accept', HERE/'accept.py')
    module = importlib.util.module_from_spec(spec)
    sys.modules['accept'] = module
    spec.loader.exec_module(module)
    return module


class AcceptTest(unittest.TestCase):
    def test_deadline_probe_uses_a_reader_with_only_its_required_repository_grants(self):
        steps = load().read_probes()['accept-cross-request-deadline']['steps']
        read_index = next(i for i, step in enumerate(steps) if step.get('kc', [None])[0] == 'read')
        self.assertEqual(steps[read_index].get('as'), 'reader',
                         'historic managed repositories must not enlarge this timing fixture')
        grants = [step['kc'] for step in steps[:read_index]
                  if step.get('kc', [])[:2] == ['grant', 'add']]
        self.assertCountEqual(grants, [
            ['grant', 'add', '--repo', repository, '--principal', 'reader', '--action', 'object.read']
            for repository in ('table-meta', 'ops-shared')])
        self.assertEqual(steps[read_index]['expect']['count'], {'observations': 1, 'failures': 2})
        self.assertEqual(steps[read_index]['expect']['maxSeconds'], 1.05)
        self.assertEqual([step['origin']['body']['delayMs'] for step in steps
                          if 'origin' in step and 'delayMs' in step['origin'].get('body', {})], [400, 400, 400])

    def test_live_dataset_sources_publish_required_schema_paths_without_selecting_descriptors(self):
        plans = load().read_probes()
        for name in ('accept-live-dataset-read-consume-only', 'accept-live-descriptor-deleted-at-head',
                     'accept-live-pinned-dataset-unit-changed', 'accept-cross-descriptor-repo-not-selected',
                     'liveaccess-mixed-values'):
            step = next(step for step in plans[name]['steps'] if step.get('kc', [])[:2] == ['dataset', 'define'])
            args = step['kc']
            sources = [args[i+1] for i, value in enumerate(args[:-1]) if value == '--source']
            self.assertIn('table-meta=$branch@tables@tables', sources)
            self.assertIn('table-meta=$branch@_schemas/table-meta@_schemas', sources)
            self.assertFalse(any(value.startswith('ops-shared=') for value in sources),
                             'descriptor dependencies must remain unselected and hidden')
            self.assertTrue(all('@' in value for value in sources), 'whole repositories must not replace selected slices')
            files = [args[i+1] for i, value in enumerate(args[:-1]) if value == '--file-source']
            self.assertFalse(any(value.startswith('ops-shared=') for value in files))
            if any(value.startswith('sales-semantic=') for value in sources):
                for name in ('metric.properties', 'metric.definition'):
                    self.assertIn('sales-semantic=$branch@_schemas/$prefix'+name+
                                  '.aspect.yaml@_schemas/sales-semantic/'+name+'.aspect.yaml', files)
                self.assertFalse(any(value.startswith('sales-semantic=$branch@_schemas/')
                                     for value in sources), 'the common Schema must keep one selected holder')

    def test_dependency_descriptor_denial_keeps_dataset_object_scope_hidden(self):
        runner = load()
        steps = runner.read_probes()['accept-cross-descriptor-repo-not-selected']['steps']
        denied = steps[-1]['expect']
        self.assertEqual(denied['exit'], 1)
        self.assertEqual(denied['has']['error.code'], 'OBJECT_REF_UNRESOLVED')
        expected = {key: value for key, value in denied.items() if key != 'exit'}
        runner.check({'error': {'code': 'OBJECT_REF_UNRESOLVED'}}, expected)
        for field in ('value', 'declarations', 'observations'):
            self.assertIn(field, denied['absent'])
            with self.subTest(field=field), self.assertRaises(runner.ContractFailure):
                runner.check({'error': {'code': 'OBJECT_REF_UNRESOLVED'}, field: {'origin': 'private'}}, expected)

    def test_schema_draft_uses_writer_canonical_path_inside_selected_schema_tree(self):
        load()
        from accept_world import World
        world = World({})
        world.dataset, world.branch = 'isolated', 'refs/heads/isolated'
        captured = []
        world.execute = lambda args, principal, expected: captured.append(args) or {}
        with tempfile.TemporaryDirectory() as temp:
            world.directory = type('Directory', (), {'name': temp})()
            for name in ('table.runtime', 'table.live-stats'):
                world.write_material('$materials/acceptance/table-meta/drafts/'+name+'.aspect.yaml',
                                     'table-meta', {})
        self.assertEqual(len(captured), 2)
        for args in captured:
            self.assertTrue(args[2].startswith('urn:kc:object:schema/'+world.prefix))
            self.assertNotIn('--path-hint', args,
                             'draft directories must not override Writer canonical Schema placement')

    def test_explicit_material_path_hint_keeps_mixed_member_inside_selected_tables(self):
        load()
        from accept_world import World
        world = World({})
        world.dataset, world.branch = 'isolated', 'refs/heads/isolated'
        captured = []
        world.execute = lambda args, principal, expected: captured.append(args) or {}
        with tempfile.TemporaryDirectory() as temp:
            world.directory = type('Directory', (), {'name': temp})()
            world.write_material('$materials/acceptance/variants/shared-descriptor/orders-error-rate.yaml', 'table-meta', {})
        self.assertIn('--path-hint', captured[0])
        self.assertEqual(captured[0][captured[0].index('--path-hint')+1], 'tables/orders.runtime/error-rate.yaml')
        self.assertEqual(json.loads(captured[0][captured[0].index('--value-source')+1])['binding']['descriptorRef'],
                         world.expand('ops/warehouse-stats'))

    def test_live_failure_diagnostic_uses_selected_stream_terms(self):
        runner = load()
        stream = runner.read_probes()['accept-live-stream-not-read']['steps'][2]['expect']
        failure = next(row['expect'] for row in stream['find'] if row['in'] == 'failures')
        runner.check({'message': 'ordinary READ cannot hydrate stream Binding at table/shop.orders#runtime/recent-events; use an explicit window/query surface'}, failure)
        with self.assertRaises(runner.ContractFailure):
            runner.check({'message': 'no runtime installed'}, failure)
    def test_live_failure_diagnostic_uses_selected_timeout_terms(self):
        runner = load()
        timeout = runner.read_probes()['accept-cross-request-deadline']['steps'][-1]['expect']['forEach']['failures']
        runner.check({'code': 'TEMPORARY_UNAVAILABLE', 'message': "external lookup timed out: the read's time limit was used up before this unit started"}, timeout)
        with self.assertRaises(runner.ContractFailure):
            runner.check({'code': 'TEMPORARY_UNAVAILABLE', 'message': 'unrelated connection refused'}, timeout)

    def test_whole_object_live_probes_remove_unrelated_sql_aspect_on_their_own_branch(self):
        runner = load()
        addresses = [{'kind': 'Member', 'objectId': 'table/shop.orders', 'aspectName': 'runtime', 'memberKey': member}
                     for member in ('row-count', 'freshness-lag')]
        addresses.append({'kind': 'Aspect', 'objectId': 'table/shop.orders', 'aspectName': 'live-stats'})
        for probe_id in ('accept-live-aspect-level-source', 'accept-cross-request-deadline'):
            steps = runner.read_probes()[probe_id]['steps']
            remove = next((i for i, step in enumerate(steps)
                           if step.get('kc', [])[:3] == ['writer', 'remove', 'urn:kc:object:table/shop.orders#/stats']), None)
            self.assertIsNotNone(remove, probe_id+' must remove the unrelated original SQL source')
            self.assertEqual(steps[remove-1]['kc'][:2], ['writer', 'head'])
            self.assertEqual(steps[remove]['expect']['nonempty'], ['result.newCommit'])
            self.assertEqual(steps[remove+1]['expect']['has']['commit'], '$saved.statsRemoval.result.newCommit')
            self.assertEqual(steps[remove+2]['assert']['different'], '$saved.beforeStatsRemoval.commit')
            read = next(step for step in steps if step.get('kc', [])[:2] == ['read', 'urn:kc:object:table/shop.orders'])
            self.assertIn('value.stats', read['expect']['absent'])
            self.assertEqual(read['expect']['unitAddresses'], addresses)
            counts = {'observations': 3} if probe_id == 'accept-live-aspect-level-source' else {'observations': 1, 'failures': 2}
            self.assertEqual(read['expect']['count'], counts)
            if probe_id == 'accept-cross-request-deadline':
                self.assertEqual(read['expect']['maxSeconds'], 1.05)

    def test_explicit_commit_mode_submits_the_original_complete_member_material(self):
        runner = load()
        from accept_world import World
        world = World({})
        world.dataset, world.branch = 'isolated', 'refs/heads/isolated'
        captured = []
        uploaded = []
        world.execute = lambda args, principal, expected: captured.append(args) or {}
        def upload(local, relative):
            files = list(Path(local).glob('*'))
            self.assertEqual(len(files), 1)
            uploaded.append(files[0].read_text())
            return '/tmp/isolated/'+relative
        world.upload = upload
        material = '$materials/acceptance/variants/rejected/member-under-aspect-source.yaml'
        with tempfile.TemporaryDirectory() as temp:
            world.directory = type('Directory', (), {'name': temp})()
            world.step({'writer': material, 'writerMode': 'commit', 'repo': 'table-meta'})
            self.assertEqual(captured[-1][:2], ['writer', 'commit'])
            for field in ('aspect_name: live-stats', 'member_key: note', 'kind: Member', 'rowCount: 0'):
                self.assertIn(field, uploaded[-1])
            self.assertIn(world.expand('schema/table.live-stats'), uploaded[-1])
            world.step({'writer': material, 'repo': 'table-meta'})
            self.assertEqual(captured[-1][:3], ['writer', 'put', 'urn:kc:object:table/shop.orders#/live-stats/note'])
            self.assertEqual(captured[-1][captured[-1].index('--kind')+1], 'Member')
            with self.assertRaises(runner.HarnessError):
                world.step({'writer': material, 'writerMode': 'unsupported'})

    def test_member_under_bound_aspect_checks_both_public_rejection_boundaries(self):
        steps = load().read_probes()['accept-live-reject-member-under-aspect-source']['steps']
        writers = [(i, step) for i, step in enumerate(steps) if 'writer' in step]
        self.assertEqual(len(writers), 2)
        for (i, step), mode, code, location in zip(writers, ('put', 'commit'),
                ('USAGE_INVALID', 'OBJECT_ID_CONFLICT'), ('Aspect', 'live-stats')):
            self.assertEqual(step.get('writerMode', 'put'), mode)
            self.assertEqual(step['expect']['has']['error.code'], code)
            self.assertIn(location, step['expect']['contains']['error.message'])
            following = steps[i+1:i+4]
            self.assertEqual(following[0]['kc'][:2], ['writer', 'head'])
            self.assertIn('equals', following[1]['assert'])
            self.assertEqual(following[2]['kc'][:2], ['read', 'urn:kc:object:table/shop.orders#/live-stats'])
            self.assertEqual(following[2]['expect']['has']['value.rowCount'], 6)
            self.assertEqual(following[2]['expect']['count']['observations'], 1)

    def test_failure_oracles_require_in_place_reason_and_separate_unit_failure(self):
        runner = load()
        expected = runner.read_probes()['accept-live-invalid-value-and-basis']['steps'][1]['expect']
        address = {'objectId': 'table/shop.orders', 'aspectName': 'runtime', 'memberKey': 'row-count'}
        error = {'code': 'SCHEMA_INSTANCE_INVALID', 'message': 'external value does not match its adopted Schema'}
        observed = {'value': {'row-count': {'error': error}, 'freshness-lag': {'value': 120},
                              'retention-days': {'value': 365}},
                    'failures': [{'address': address, **error}], 'observations': []}
        runner.check(observed, expected)
        for invalid in (None, 0, {'metric': 'row_count', 'value': 6, 'unit': 'rows', 'status': 'ok'},
                        {'error': error, 'value': 6}, {'error': {'code': error['code'], 'message': ''}}):
            with self.subTest(invalid=invalid), self.assertRaises(runner.ContractFailure):
                changed = copy.deepcopy(observed)
                changed['value']['row-count'] = invalid
                runner.check(changed, expected)
        for group, rows in (('failures', []), ('observations', [{'address': address}])):
            with self.subTest(group=group), self.assertRaises(runner.ContractFailure):
                changed = copy.deepcopy(observed)
                changed[group] = rows
                runner.check(changed, expected)

    def test_all_failed_member_oracles_retain_error_and_no_success_observation(self):
        runner = load()
        checked = 0
        for probe in runner.read_probes().values():
            for step in probe['steps']:
                expected = step.get('expect', {})
                failures = list(expected.get('includes', {}).get('failures', []))
                failures += [row['where'] for row in expected.get('find', []) if row['in'] == 'failures']
                failures = {json.dumps(row, sort_keys=True): row for row in failures}.values()
                for failure in failures:
                    member = failure.get('address', {}).get('memberKey')
                    if not member:
                        continue
                    path = 'value.'+member
                    if path not in expected.get('absent', []) and path+'.error.code' not in expected.get('has', {}):
                        continue
                    checked += 1
                    with self.subTest(probe=probe['id'], member=member):
                        self.assertEqual(expected.get('has', {}).get(path+'.error.code'), failure['code'])
                        self.assertIn(path+'.error.message', expected.get('nonempty', []))
                        self.assertEqual(expected.get('count', {}).get(path), 1,
                                         'a failed value contains only its error, never stale business fields')
                        self.assertTrue(any(row['in'] == 'observations' and row.get('count') == 0 and
                                            row['where'].get('address') == {'objectId': 'table/shop.orders',
                                             'aspectName': 'runtime', 'memberKey': member}
                                            for row in expected.get('find', [])))
        self.assertEqual(checked, 14)

    def test_dependency_repository_reads_are_explicit_in_principal_probe_prestate(self):
        plans = load().read_probes()
        for probe_id, principals in (('accept-live-identity-pass-through', ('alice', 'bob')),
                                     ('liveaccess-audit-separate-authorization', ('alice',))):
            steps = plans[probe_id]['steps']
            for principal in principals:
                first_read = next(i for i, step in enumerate(steps)
                                  if step.get('as') == principal and step.get('kc', [None])[0] == 'read')
                for repository in ('table-meta', 'ops-shared'):
                    grant = ['grant', 'add', '--repo', repository, '--principal', principal,
                             '--action', 'object.read']
                    self.assertTrue(any(step.get('kc') == grant for step in steps[:first_read]),
                                    probe_id+' needs explicit '+repository+' read scope for '+principal)
        audit = plans['liveaccess-audit-separate-authorization']['steps']
        self.assertTrue(any(step.get('as') == 'alice' and step.get('expect', {}).get('has', {}).get(
            'error.code') == 'FORBIDDEN' for step in audit), 'object.read must not grant audit access')

    def test_explicit_runtime_aspect_rejection_reaches_the_complete_file_writer(self):
        steps = load().read_probes()['accept-live-writer-rejects-declarations']['steps']
        step = next(row for row in steps if row.get('writer', '').endswith('/runtime-aspect-declared.yaml'))
        self.assertEqual(step.get('writerMode'), 'commit')
        self.assertEqual(step['expect']['has']['error.code'], 'OBJECT_ID_CONFLICT')
        self.assertIn('runtime', step['expect']['contains']['error.message'])

    def test_maintenance_probe_checks_canonical_reference_and_empty_snapshot_body(self):
        import base64
        runner = load()
        from accept_world import World
        world = World({})
        world.dataset, world.branch = 'isolated', 'refs/heads/isolated'
        steps = runner.read_probes()['accept-live-maintenance-read-declaration']['steps']
        def encoded(descriptor, body=''):
            source = {'kind': 'binding', 'binding': {'descriptorRef': world.expand(descriptor)}}
            raw = '---\nobject_id: table/shop.orders\naspect_name: runtime\nmember_key: row-count\n'
            raw += 'valueSource: '+json.dumps(source, separators=(',', ':'))+'\n---\n'+body
            return base64.b64encode(raw.encode()).decode()
        world.saved = {'rawContent': encoded('ops/warehouse-stats'),
                       'newRaw': {'content': encoded('ops/pipeline-monitor')}}
        for step in (steps[2], steps[7]):
            world.step(step)
        for body in ('null\n', '0\n', '{"metric":"row_count","value":6}\n'):
            with self.subTest(body=body), self.assertRaises(runner.ContractFailure):
                world.saved['rawContent'] = encoded('ops/warehouse-stats', body)
                world.step(steps[2])
        world.saved['rawContent'] = encoded('ops/pipeline-monitor')
        with self.assertRaises(runner.ContractFailure):
            world.step(steps[2])

    def test_shared_resource_descriptors_and_references_are_isolated_per_run(self):
        runner = load()
        from accept_world import World
        first, second = World({}), World({})
        for world in (first, second):
            world.dataset, world.branch = world.prefix+'case', 'refs/heads/probe'
        descriptor = first.expand('resource/shop-stats')
        self.assertNotEqual(descriptor, 'resource/shop-stats')
        self.assertNotEqual(descriptor, second.expand('resource/shop-stats'))
        first.descriptor_ids = {name: 'ops/probe/'+name.split('/', 1)[1] for name in first.descriptor_keys}
        self.assertEqual(first.expand('resource/shop-stats'), descriptor,
                         'base descriptor identity must not change when a probe selects its own ops descriptors')
        with tempfile.TemporaryDirectory() as temp:
            first.directory = type('Directory', (), {'name': temp})()
            prepared = first.prepared_materials(runner.QINGHE/'_materials/table-meta', 'prepared')
            contents = [path.read_text() for path in prepared.rglob('*.yaml')]
            self.assertTrue(any('object_id: '+descriptor in text for text in contents))
            self.assertTrue(any('descriptorRef: '+descriptor in text for text in contents))
            self.assertTrue(all('resource/shop-stats' not in text for text in contents))
        self.assertEqual(first.expand('resource/shop-sql'), 'resource/shop-sql',
                         'the separate SQL fixture uses its own explicit descriptor identity')

    def test_direct_external_put_steps_supply_public_json_null_syntax(self):
        runner = load()
        checked = 0
        for probe in runner.read_probes().values():
            for step in probe['steps']:
                args = step.get('kc', [])
                if args[:2] == ['writer', 'put'] and '--value-source' in args:
                    self.assertIn('--value', args, probe['id'])
                    self.assertIsNone(json.loads(args[args.index('--value')+1]), probe['id'])
                    checked += 1
        self.assertEqual(checked, 3)

    def test_original_schema_rejection_material_reaches_writer_without_losing_access(self):
        load()
        from accept_world import World
        world = World({})
        world.dataset, world.branch = 'isolated', 'refs/heads/isolated'
        captured = []
        world.execute = lambda args, principal, expected: captured.append(args) or {}
        with tempfile.TemporaryDirectory() as temp:
            world.directory = type('Directory', (), {'name': temp})()
            world.write_material('$materials/acceptance/variants/rejected/schema-with-access.yaml', 'table-meta', {})
        value = json.loads(captured[0][captured[0].index('--value')+1])
        self.assertEqual(value['fields']['name']['access'], ['text'])

    def test_shared_construct_keeps_uploaded_material_paths_opaque_and_rewrites_contents(self):
        runner = load()
        from accept_world import World
        world = World({})
        uploaded = {}
        class FirstPublicationReached(Exception):
            pass
        def upload(local, relative):
            remote = '/tmp/'+world.prefix+'/'+relative
            uploaded[remote] = Path(local)
            self.assertTrue(Path(local).is_dir())
            return remote
        def require(args):
            if args[0] == 'create':
                name = args[args.index('--name')+1]
                return {'repositoryId': name, 'name': name, 'store': 'lakefs'}
            if args[0] == 'attach':
                return {'repositoryId': args[args.index('--repo')+1]}
            if args[0] == 'show':
                return {'repositories': [{'id': name} for name in ['kr://kc/system', *world.repositories.values()]]}
            if args[:2] == ['writer', 'commit']:
                remote = args[args.index('--dir')+1]
                parent, local = next((key, path) for key, path in uploaded.items()
                                     if remote.startswith(key+'/'))
                material = local/remote[len(parent)+1:]
                self.assertTrue(material.is_dir(), 'uploaded material paths must name actual unchanged directories: '+remote)
                self.assertEqual(material.name, 'sales-semantic')
                self.assertEqual(args[args.index('--repo')+1], world.repositories['sales-semantic'])
                texts = [path.read_text() for path in material.rglob('*.yaml')]
                self.assertTrue(any(world.schema_ids['schema/metric.properties'] in text for text in texts),
                                'uploaded content must still use run-isolated Schema identities')
                raise FirstPublicationReached()
            self.fail('unexpected construct command: '+str(args[:2]))
        world.upload, world.require = upload, require
        try:
            with self.assertRaises(FirstPublicationReached):
                world.construct()
        finally:
            if world.directory:
                world.directory.cleanup()

    def test_generated_repositories_use_unique_legal_business_names(self):
        load()
        from accept_world import World
        first, second = World({}), World({})
        self.assertNotEqual(first.prefix, second.prefix)
        for repository in first.repositories.values():
            self.assertRegex(repository, r'^[a-z0-9][a-z0-9-]{2,62}$')
            self.assertFalse(repository.startswith('kc-'), 'platform prefixes cannot be created as business repositories')

    def test_non_json_cli_failure_identifies_action_and_code_without_output_body(self):
        runner = load()
        from accept_world import World
        world = World({})
        result = SimpleNamespace(returncode=2, stdout='', stderr='USAGE_INVALID: PRIVATE-BODY\nusage details')
        with patch('accept_world.subprocess.run', return_value=result):
            with self.assertRaises(runner.HarnessError) as failure:
                world.call(['writer', 'put', '--repo', 'owned-repository', '--value', 'PRIVATE-BODY'])
        message = str(failure.exception)
        for value in ('writer put', 'owned-repository', 'exit 2', 'USAGE_INVALID'):
            self.assertIn(value, message)
        self.assertNotIn('PRIVATE-BODY', message)

    def test_cli_usage_failure_preserves_the_product_diagnostic_for_negative_probes(self):
        runner = load()
        from accept_world import World
        world = World({})
        reason = 'index declaration inputs[0].from: expected a Schema field locator'
        result = SimpleNamespace(returncode=1, stderr='',
                                 stdout='USAGE_INVALID: '+reason+'\n\nWriter leaf help\n')
        with patch('accept_world.subprocess.run', return_value=result):
            status, value = world.call(['writer', 'put', '--repo', 'owned-repository'])
        self.assertEqual(status, 1)
        self.assertEqual(value, {'error': {'code': 'USAGE_INVALID', 'message': reason}})
        self.assertEqual(world.events[-1]['errorCode'], 'USAGE_INVALID')
        runner.check(value, {'has': {'error.code': 'USAGE_INVALID', 'error.message': reason}})
        with self.assertRaises(runner.ContractFailure):
            runner.check(value, {'has': {'error.message': 'a different rejected position'}})

    def test_preflight_accepts_plaintext_lakefs_health_but_rejects_failed_http(self):
        import urllib.error
        runner = load()
        with tempfile.TemporaryDirectory() as temp:
            (Path(temp)/'compose.env').write_text('')
            env = {'KC_DEPLOY_HOME': temp, 'KC_DEPLOY_PROFILE': 'local',
                   'KC_SERVER_URL': 'http://server', 'KC_ACCEPT_LAKEFS_URL': 'http://lakefs',
                   'KC_ACCEPT_OPENSEARCH_URL': 'http://search', 'KC_ACCEPT_ORIGIN_URL': 'http://origin',
                   'KC_ACCEPT_UNTRUSTED_ORIGIN_URL': 'http://untrusted', 'KC_ACCEPT_ALT_ORIGIN_URL': 'http://alt'}
            failed = [False]
            rejected = []
            def response(request, timeout=5):
                url = request.full_url
                if url == 'http://lakefs/_health':
                    if failed[0]:
                        error = urllib.error.HTTPError(url, 503, 'unavailable', {}, io.BytesIO(b'unavailable'))
                        rejected.append(error)
                        raise error
                    return io.BytesIO(b'OK')
                value = {'role': 'live-origin', 'status': 'ready'}
                if '/_cluster/settings' in url:
                    value = {'defaults': {'cluster.max_shards_per_node': '1000'}}
                elif '/_nodes' in url:
                    value = {'nodes': {'data': {'roles': ['data']}}}
                elif '/_all/_settings' in url:
                    value = {}
                return io.BytesIO(json.dumps(value).encode())
            with patch.dict(os.environ, env), patch('urllib.request.urlopen', side_effect=response):
                runner.preflight()
                failed[0] = True
                with self.assertRaisesRegex(runner.HarnessError, 'lakefs/_health: HTTP 503'):
                    runner.preflight()
            self.assertTrue(rejected[0].closed)

    def test_precise_range_spec_keeps_fragment_and_last_no_call_assertion(self):
        spec = next(row for row in load().specifications() if row['id'] == 'accept-live-precise-range')
        self.assertIn('#/runtime/row-count；第三次测试源没有新调用。', spec['then'][0])

    def test_assertion_failure_is_fail_but_fixture_failure_is_error(self):
        runner = load()
        spec = {'id': 'one', 'then': ['value is six'], 'title': 'one'}
        def assertion():
            raise runner.ContractFailure('value: got 5; want 6')
        def fixture():
            raise runner.HarnessError('fixture file is missing')
        self.assertEqual(runner.run_probe(spec, assertion)['status'], 'fail')
        self.assertEqual(runner.run_probe(spec, fixture)['status'], 'error')
        self.assertEqual(runner.run_probe(spec, lambda: None)['status'], 'pass')

    def test_zero_successful_commands_cannot_pass(self):
        runner = load()
        with self.assertRaises(runner.HarnessError):
            runner.validate_probe({'id': 'x', 'steps': [{'kc': ['read', 'x']} ]})
        with self.assertRaises(runner.HarnessError):
            runner.validate_probe({'id': 'x', 'steps': []})

    def test_matchers_cover_absent_null_exact_and_distinct_failures(self):
        runner = load()
        value = {'value': {'static': 365, 'legalNull': None},
                 'failures': [{'code': 'TEMPORARY_UNAVAILABLE', 'address': 'freshness'}]}
        runner.check(value, {'has': {'value.static': 365, 'value.legalNull': None},
                             'absent': ['value.freshness'],
                             'includes': {'failures': [{'code': 'TEMPORARY_UNAVAILABLE', 'address': 'freshness'}]}})
        for wanted in ({'has': {'value.static': 0}}, {'absent': ['value.legalNull']},
                       {'includes': {'failures': [{'code': 'FORBIDDEN'}]}}):
            with self.subTest(wanted=wanted), self.assertRaises(runner.ContractFailure):
                runner.check(value, wanted)

    def test_materials_are_node_relative_and_cannot_escape(self):
        runner = load()
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            ((root/'allowed.json').resolve()).write_text('{}')
            self.assertEqual(runner.material_path(root, '$materials/allowed.json'), (root/'allowed.json').resolve())
            for path in ('/etc/passwd', '$materials/../../secret', '../secret'):
                with self.subTest(path=path), self.assertRaises(runner.HarnessError):
                    runner.material_path(root, path)

    def test_missing_stack_is_error_without_building_fixture(self):
        runner = load()
        constructed = []
        def preflight():
            raise runner.HarnessError('local test stack is missing')
        specs = [{'id': 'a', 'title': 'a'}, {'id': 'b', 'title': 'b'}]
        report = runner.batch(specs, {'a': lambda: None, 'b': lambda: None},
                              preflight=preflight, construct=lambda: constructed.append(True))
        self.assertEqual(constructed, [])
        self.assertEqual([r['status'] for r in report['results']], ['error', 'error'])
        self.assertEqual(report['summary'], {'pass': 0, 'fail': 0, 'error': 2})

    def test_prestate_failure_identifies_the_failed_resource_and_product_reason(self):
        runner = load()
        from accept_world import World
        world = World({})
        world.call = lambda args: (1, {'error': {'code': 'OBJECT_REF_UNRESOLVED',
                                                'message': 'missing referenced schema at inputs[0].from'}})
        args = ['writer', 'commit', '--repo', world.prefix+'table-meta', '--dir', '/tmp/materials']
        with self.assertRaises(runner.HarnessError) as failure:
            world.require(args)
        for text in ('writer commit', world.prefix+'table-meta', '/tmp/materials',
                     'OBJECT_REF_UNRESOLVED', 'inputs[0].from'):
            self.assertIn(text, str(failure.exception))

    def test_probe_failure_identifies_the_product_command_without_publishing_input(self):
        runner = load()
        from accept_world import World
        world = World({})
        world.dataset, world.branch = world.prefix+'case', 'refs/heads/probe'
        world.call = lambda args, principal: (0, {'value': 5})
        args = ['read', 'urn:kc:object:table/orders', '--repo', 'table-meta', '--input', 'PRIVATE-INPUT']
        with self.assertRaises(runner.ContractFailure) as failure:
            world.execute(args, 'admin', {'has': {'value': 6}})
        for text in ('read', 'table/orders', world.repositories['table-meta'], 'refs/heads/probe', 'value'):
            self.assertIn(text, str(failure.exception))
        self.assertNotIn('PRIVATE-INPUT', str(failure.exception))

    def test_shared_prestate_constructs_once_and_probes_never_inherit_last_result(self):
        runner = load()
        observed = []
        specs = [{'id': 'a', 'title': 'a'}, {'id': 'b', 'title': 'b'}]
        report = runner.batch(specs, {'a': lambda: observed.append('a'),
                                     'b': lambda: observed.append('b')},
                              preflight=lambda: None, construct=lambda: observed.append('construct'))
        self.assertEqual(observed, ['construct', 'a', 'b'])
        self.assertEqual(report['summary'], {'pass': 2, 'fail': 0, 'error': 0})


    def test_failed_shared_prestate_is_not_a_product_failure_for_unstarted_specs(self):
        runner = load()
        executed = []
        specs = [{'id': 'a', 'title': 'a'}, {'id': 'b', 'title': 'b'}]
        def construct():
            raise runner.ContractFailure('actual Writer rejected the shared seed')
        report = runner.batch(specs, {'a': lambda: executed.append('a'), 'b': lambda: executed.append('b')},
                              preflight=lambda: None, construct=construct)
        self.assertEqual(executed, [])
        self.assertEqual(report['summary'], {'pass': 0, 'fail': 0, 'error': 2})
        self.assertEqual(report['prestate']['status'], 'fail')
        self.assertEqual(report['prestate']['phase'], 'construct')
        self.assertTrue(all(row['executionStatus'] == 'not_started' for row in report['results']))

    def test_interruption_keeps_remaining_specs_unexecuted(self):
        import signal
        runner = load()
        executed = []
        def interrupted():
            raise runner.RunInterrupted(signal.SIGTERM)
        specs = [{'id': 'a', 'title': 'a'}, {'id': 'b', 'title': 'b'}]
        report = runner.batch(specs, {'a': interrupted, 'b': lambda: executed.append('b')},
                              preflight=lambda: None, construct=lambda: None)
        self.assertEqual(executed, [])
        self.assertEqual(report['summary'], {'pass': 0, 'fail': 0, 'error': 2})
        self.assertEqual(report['results'][0]['interrupted'], signal.SIGTERM)
        self.assertEqual(report['results'][1]['executionStatus'], 'not_started')

    def test_index_declaration_specs_split_at_dataset_preparation(self):
        runner = load()
        rows = {spec['id']: (Path(spec['file']).name, runner.owners(spec)) for spec in runner.specifications()}
        self.assertTrue(runner.DATASET_INDEX_SPECS <= set(rows))
        declaration = {id for id, (file, owner) in rows.items() if owner == ['INDEX-DECLARATION-01']}
        self.assertEqual(declaration, {
            'accept-search-writer-rejects-index', 'accept-index-reject-shape',
            'accept-index-cross-repo-schema-ref', 'accept-search-capability-catalog',
            'accept-index-dry-run-matches-writer'})
        for id, (file, owner) in rows.items():
            if file.startswith(('accept-06', 'accept-07')):
                with self.subTest(id=id):
                    self.assertIn(owner, (['INDEX-DECLARATION-01'], ['INDEX-SEARCH-SPACE-01']))
                    self.assertEqual(owner == ['INDEX-SEARCH-SPACE-01'], id in runner.DATASET_INDEX_SPECS)

    def test_probe_inventory_is_explicit_and_every_material_reference_exists(self):
        runner = load()
        specifications = {spec['id']: spec for spec in runner.specifications()}
        probes = runner.read_probes()
        self.assertTrue(probes)
        self.assertTrue(set(probes) <= set(specifications))
        for id, probe in probes.items():
            with self.subTest(id=id):
                runner.validate_probe(probe)
                for step in probe['steps']:
                    if 'writer' in step:
                        runner.material_path(runner.QINGHE/'_materials', step['writer'])

    def test_world_namespaces_cross_repository_schema_descriptor_and_principals(self):
        load()
        from accept_world import World
        world = World({})
        world.dataset = world.prefix+'case'
        world.branch = 'refs/heads/case'
        world.saved = {'prior': {'value': 6}}
        expanded = world.expand({'from': 'schema/table.properties#/fields/name',
                                 'binding': {'descriptorRef': 'ops/warehouse-stats'},
                                 'saved': '$saved.prior.value', 'dataset': '$dataset-suffix'})
        self.assertEqual(expanded['saved'], 6)
        self.assertEqual(expanded['dataset'], world.dataset+'-suffix')
        self.assertIn(world.prefix, expanded['from'])
        self.assertIn(world.prefix, expanded['binding']['descriptorRef'])
        self.assertEqual(world.expand('schema/core/index-declaration/v1'), 'schema/core/index-declaration/v1')
        self.assertNotEqual(World({}).prefix, world.prefix)

    def test_unknown_assertion_operator_cannot_succeed(self):
        runner = load()
        from accept_world import World
        world = World({})
        world.dataset, world.branch = 'isolated', 'refs/heads/isolated'
        with self.assertRaises(runner.HarnessError):
            world.step({'assert': {'actual': 6, 'unimplemented': 6}})

    def test_base64_maintenance_oracle_checks_declaration_and_rejects_observed_values(self):
        import base64
        runner = load()
        from accept_world import World
        world = World({})
        world.dataset, world.branch = 'isolated', 'refs/heads/isolated'
        body = base64.b64encode(b'valueSource:\n  descriptorRef: ops/stats\n').decode()
        world.step({'assert': {'actual': body, 'base64Contains': ['valueSource:'], 'base64Excludes': ['value: 6']}})
        with self.assertRaises(runner.ContractFailure):
            world.step({'assert': {'actual': body, 'base64Contains': ['value: 6']}})

    def test_external_material_with_empty_or_explicit_null_body_has_no_private_value(self):
        load()
        from accept_world import World
        world = World({})
        world.dataset, world.branch = 'isolated', 'refs/heads/isolated'
        captured = []
        world.execute = lambda args, principal, expected: captured.append(args) or {}
        with tempfile.TemporaryDirectory() as temp:
            world.directory = type('Directory', (), {'name': temp})()
            for material in ('table-meta/tables/orders.runtime/row-count.yaml',
                             'table-meta/tables/orders.runtime/freshness-lag.yaml'):
                world.write_material('$materials/acceptance/'+material, 'table-meta', {})
                self.assertIn('--value', captured[-1], 'the public put command requires an explicit JSON value or file')
                self.assertIsNone(json.loads(captured[-1][captured[-1].index('--value')+1]),
                                  'external declarations must not carry an observed or private value')
                self.assertIn('--value-source', captured[-1])


    def test_real_script_entry_retains_product_and_fixture_failures(self):
        # sitecustomize supplies only an isolated transport/world; the child
        # executes the real script path with its real __main__ import identity.
        fixture_source = r"""import io, json, os, sys, types, urllib.request
from pathlib import Path
def response(request, timeout=5):
    url = request.full_url
    value = {'role': 'live-origin', 'status': 'ready'}
    if '/_cluster/settings' in url:
        value = {'defaults': {'cluster.max_shards_per_node': '1000'}}
    elif '/_nodes' in url:
        value = {'nodes': {'data': {'roles': ['data']}}}
    elif '/_all/_settings' in url:
        value = {}
    return io.BytesIO(json.dumps(value).encode())
urllib.request.urlopen = response
module = types.ModuleType('accept_world')
class World:
    def __init__(self, plans):
        self.prefix = 'accept-0123456789abcdef-'
        self.cleanup_report, self.events = {}, []
    def construct(self):
        if os.environ['ENTRY_MODE'] == 'prestate':
            from accept import ContractFailure
            raise ContractFailure('shared Writer fixture rejection')
    def probe(self, spec_id):
        from accept import ContractFailure, HarnessError
        if os.environ['ENTRY_MODE'] == 'product':
            raise ContractFailure('actual product assertion failed')
        raise HarnessError('fixture is unavailable')
    def close(self):
        Path(os.environ['ENTRY_CLOSED']).write_text('closed')
module.World = World
sys.modules['accept_world'] = module
"""
        for mode, status, prestate_status in [('product', 'fail', 'pass'), ('fixture', 'error', 'pass'),
                                              ('prestate', 'error', 'fail')]:
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                (root/'sitecustomize.py').write_text(fixture_source)
                stack, report = root/'stack', root/'report'
                stack.mkdir()
                (stack/'compose.env').write_text('')
                closed = root/'closed'
                env = dict(os.environ, PYTHONPATH=str(root), KC_DEPLOY_HOME=str(stack), KC_DEPLOY_PROFILE='local',
                           KC_VALIDATION_RUN_DIR=str(report), KC_VALIDATION_RUN_ID='entry-fixture',
                           KC_VALIDATION_SOURCE_FINGERPRINT='fixture-source', ENTRY_MODE=mode, ENTRY_CLOSED=str(closed),
                           KC_SERVER_URL='http://server.invalid', KC_ACCEPT_LAKEFS_URL='http://lakefs.invalid',
                           KC_ACCEPT_OPENSEARCH_URL='http://search.invalid', KC_ACCEPT_ORIGIN_URL='http://origin.invalid',
                           KC_ACCEPT_UNTRUSTED_ORIGIN_URL='http://untrusted.invalid', KC_ACCEPT_ALT_ORIGIN_URL='http://alt.invalid')
                result = subprocess.run([sys.executable, str(HERE/'accept.py'), '--select', 'liveaccess-plugin-help'],
                                        env=env, capture_output=True, text=True, timeout=10)
                self.assertEqual(result.returncode, 1, result.stdout+result.stderr)
                evidence = json.loads((report/'acceptance.json').read_text())
                self.assertEqual(evidence['results'][0]['status'], status, evidence)
                self.assertEqual(evidence['prestate']['status'], prestate_status)
                self.assertEqual(closed.read_text(), 'closed')
                self.assertEqual(evidence['summary'][status], 1)
                if mode == 'product':
                    self.assertEqual(evidence['summary']['error'], 0)
                    self.assertTrue(evidence['results'][0]['requiresDebtRegistration'])
                if mode == 'prestate':
                    self.assertEqual(evidence['results'][0]['executionStatus'], 'not_started')


if __name__ == '__main__':
    unittest.main()
