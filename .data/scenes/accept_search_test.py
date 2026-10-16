"""Offline contracts for the authored Dataset index acceptance fixtures."""
import unittest
from types import SimpleNamespace
from unittest.mock import patch

from accept_test import load


class DatasetSearchProbeTest(unittest.TestCase):
    def test_plain_help_is_admitted_without_accepting_non_json_business_responses(self):
        runner = load()
        from accept_world import World
        world = World({})
        result = SimpleNamespace(returncode=0, stdout='kc help consume\nkc show', stderr='')
        with patch('accept_world.subprocess.run', return_value=result):
            status, value = world.call(['help'])
            self.assertEqual((status, value), (0, {'text': result.stdout}))
            with self.assertRaises(runner.HarnessError):
                world.call(['show'])
        result.returncode = 1
        with patch('accept_world.subprocess.run', return_value=result):
            with self.assertRaises(runner.HarnessError):
                world.call(['help'])

    def test_consumer_enters_from_help_and_selects_the_observed_revision(self):
        steps = load().read_probes()['accept-search-dataset-is-space']['steps']
        reader = [step['kc'] for step in steps if step.get('as') == 'reader']
        self.assertEqual(reader[:4], [['help'], ['help', 'consume'], ['show'],
                                     ['dataset', 'show', '$dataset', '--revision', '1']])

    def test_publication_assertions_verify_the_public_receipt_identity_and_revision(self):
        runner = load()
        for probe in ('accept-search-dataset-is-space', 'accept-search-pinned-dataset-frozen'):
            for step in runner.read_probes()[probe]['steps']:
                command = step.get('kc', [])
                if 'indexDataset' in step:
                    fixture = step['indexDataset']
                    name, revision = fixture.get('name', '$dataset'), fixture.get('revision', 1)
                elif command[:2] == ['dataset', 'define']:
                    name = command[command.index('--dataset')+1]
                    revision = int(command[command.index('--revision')+1])
                else:
                    continue
                with self.subTest(probe=probe, name=name, revision=revision):
                    receipt = {'setId': name, 'revision': revision, 'disposition': 'APPLIED',
                               'sources': [{'repository': 'published-source', 'commit': 'frozen'}]}
                    runner.check(receipt, step['expect'])
                    for field, wrong in [('setId', 'another-dataset'), ('revision', revision+1),
                                         ('disposition', 'REPLAYED'), ('sources', [])]:
                        with self.assertRaises(runner.ContractFailure):
                            runner.check(dict(receipt, **{field: wrong}), step['expect'])

    def test_inventory_reader_has_both_resolution_and_content_consumption(self):
        steps = load().read_probes()['accept-search-dataset-is-space']['steps']
        actions = {}
        for step in steps:
            command = step.get('kc', [])
            if command[:2] == ['grant', 'add'] and command[command.index('--principal')+1] == 'reader':
                dataset = command[command.index('--dataset')+1]
                actions.setdefault(dataset, set()).update(command[command.index('--action')+1].split(','))
        self.assertEqual(actions, {'$dataset': {'dataset.resolve', 'file.read'},
                                   '$dataset-plain': {'dataset.resolve', 'file.read'}})

    def test_dataset_show_asserts_the_public_management_summary(self):
        runner = load()
        step = next(step for step in runner.read_probes()['accept-search-dataset-is-space']['steps']
                    if step.get('kc', [])[:2] == ['dataset', 'show'])
        summary = {'id': '$dataset', 'revision': 1, 'servingRevision': 1, 'versions': [1],
                   'sources': [{'repository': 'published-source', 'commit': 'frozen'}]}
        runner.check(summary, step['expect'])
        for field, wrong in [('id', 'another-dataset'), ('revision', 2), ('sources', [])]:
            with self.assertRaises(runner.ContractFailure):
                runner.check(dict(summary, **{field: wrong}), step['expect'])

    def test_undeclared_scope_requires_an_explicit_error_and_a_repair_direction(self):
        runner = load()
        step = next(step for step in runner.read_probes()['accept-search-dataset-is-space']['steps']
                    if step.get('kc', [])[:3] == ['search', '--dataset', '$dataset-plain'])
        self.assertEqual(step['expect']['exit'], 1)
        expected = {key: value for key, value in step['expect'].items() if key != 'exit'}
        message = ('Dataset scope cannot satisfy SEARCH: an index declaration selected into the dataset '
                   'must declare the required text/filter/sort use; see kc schema describe --dataset scope --revision 1')
        runner.check({'error': {'code': 'CAPABILITY_UNSATISFIED', 'message': message}}, expected)
        for error in [{'code': 'INTERNAL', 'message': message},
                      {'code': 'CAPABILITY_UNSATISFIED', 'message': 'not available'},
                      {'code': 'CAPABILITY_UNSATISFIED', 'message': 'index declaration is required'}]:
            with self.assertRaises(runner.ContractFailure):
                runner.check({'error': error}, expected)

    def test_saved_observations_are_compared_without_expanding_their_contents_again(self):
        runner = load()
        from accept_world import World
        world = World({})
        world.dataset = world.prefix+'fixed'
        observation = {'commit': 'frozen', 'value': {'repository': 'sales-semantic',
                                                   'literal': '$dataset'}}
        world.saved = {'first': observation, 'later': observation.copy()}
        world.step({'assert': {'actual': '$saved.later', 'equals': '$saved.first'}})
        world.saved['later'] = dict(observation, commit='advanced')
        with self.assertRaises(runner.ContractFailure):
            world.step({'assert': {'actual': '$saved.later', 'equals': '$saved.first'}})

    def test_default_fixture_publishes_members_before_freezing_their_sources(self):
        load()
        from accept_world import World
        world = World({})
        writes, calls = [], []
        world.write_material = lambda *args: writes.append(args)
        world.execute = lambda *args: calls.append(args) or {'setId': 'observed'}
        result = world.index_dataset({'name': '$dataset', 'metadata': {'description': 'sales'}})
        self.assertEqual(result, {'setId': 'observed'})
        self.assertEqual([args[0].rsplit('/', 1)[1] for args in writes],
                         ['table-name.yaml', 'metric-text.yaml', 'table-row-count.yaml', 'metric-unit.yaml'])
        self.assertTrue(all(args[1] == 'sales-analysis' for args in writes))
        args, principal, expected = calls[0]
        self.assertEqual(principal, 'admin')
        self.assertEqual(expected, {})
        sources = [(flag, args[i+1]) for i, flag in enumerate(args[:-1])
                   if flag in ('--source', '--file-source')]
        republish = next(step['kc'] for step in load().read_probes()['accept-search-pinned-dataset-frozen']['steps']
                         if step.get('kc', [])[:2] == ['dataset', 'define'])
        # The portable recipe is independently exercised against actual source
        # trees by the Dataset package contract, including adjacent private files.
        self.assertCountEqual(sources, [(flag, republish[i+1]) for i, flag in enumerate(republish[:-1])
                                   if flag in ('--source', '--file-source')])

    def test_plain_control_omits_only_declarations(self):
        load()
        from accept_world import World
        world = World({})
        calls = []
        world.write_material = lambda *args: self.fail('plain scope must not restore an index member')
        world.execute = lambda *args: calls.append(args) or {}
        world.index_dataset({'name': '$dataset-plain', 'declarations': False})
        args = calls[0][0]
        self.assertNotIn('sales-analysis=$branch@indexes@indexes', args)
        self.assertEqual(args.count('--source'), 3)
        self.assertEqual(args.count('--file-source'), 2)
        self.assertIn('$dataset-plain', args)

    def test_frozen_probe_republishes_the_mutated_branch_without_restoring_the_member(self):
        runner = load()
        steps = runner.read_probes()['accept-search-pinned-dataset-frozen']['steps']
        removal = next(i for i, step in enumerate(steps)
                       if step.get('kc', [])[:3] == ['writer', 'remove',
                           'urn:kc:object:index/sales-search#/index/metric-text'])
        after = steps[removal+1:]
        self.assertFalse(any('writer' in step or 'indexDataset' in step for step in after))
        publication = next(step['kc'] for step in after if step.get('kc', [])[:2] == ['dataset', 'define'])
        self.assertEqual(publication[publication.index('--revision')+1], '2')
        self.assertIn('sales-analysis=$branch@indexes@indexes', publication)
        self.assertTrue(any(step.get('assert', {}).get('actual') == '$saved.frozenRead' for step in after))
        self.assertTrue(any(step.get('assert', {}).get('actual') == '$saved.frozenOverview.specs' for step in after))


if __name__ == '__main__':
    unittest.main()
