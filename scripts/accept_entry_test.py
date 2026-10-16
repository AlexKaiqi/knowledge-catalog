"""Exercise the real Make/deployment acceptance entry without any service setup."""
import json
import importlib.util
import io
import os
from pathlib import Path
import subprocess
import tempfile
import sys
import unittest
from unittest import mock
import urllib.error

ROOT = Path(__file__).resolve().parents[1]


class AcceptanceEntryTest(unittest.TestCase):
    def test_origin_request_helper_closes_http_error_response(self):
        origin_dir = ROOT/'.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_materials/acceptance/live-origin'
        sys.path.insert(0, str(origin_dir))
        try:
            spec = importlib.util.spec_from_file_location('accept_origin_test', origin_dir/'test_origin.py')
            module = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(module)
        finally:
            sys.path.remove(str(origin_dir))
        response = io.BytesIO(b'{"error":{"code":"FORBIDDEN"}}')
        failure = urllib.error.HTTPError('http://origin.invalid', 403, 'forbidden', {}, response)
        case = module.LiveOriginTest('test_aspect_value')
        case.base = 'http://origin.invalid'
        with mock.patch('urllib.request.urlopen', side_effect=failure):
            status, body = case.call('POST', '/v1/access', {})
        self.assertEqual(status, 403)
        self.assertEqual(body['error']['code'], 'FORBIDDEN')
        self.assertTrue(response.closed, 'the HTTPError body must be closed before returning')

    def test_make_entry_missing_stack_reports_each_specification_without_starting_services(self):
        with tempfile.TemporaryDirectory() as temp:
            folder = Path(temp)
            bins = folder/'bin'
            bins.mkdir()
            marker = folder/'unexpected-docker'
            docker = bins/'docker'
            docker.write_text('#!/bin/sh\nprintf called > "'+str(marker)+'"\nexit 79\n')
            docker.chmod(0o755)
            env = dict(os.environ, PATH=str(bins)+os.pathsep+os.environ['PATH'],
                       KC_DEPLOY_HOME=str(folder/'missing-stack'), KC_VALIDATION_RUN_ID='entry-offline',
                       KC_VALIDATION_RUN_DIR=str(folder/'report'))
            result = subprocess.run(['make', 'deploy-local-accept'], cwd=ROOT, env=env,
                                    capture_output=True, text=True, timeout=15)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(marker.exists(), 'the acceptance entry must not start/check Docker itself')
            report = json.loads((folder/'report/acceptance.json').read_text())
            self.assertIn('stack configuration missing', report['prestate']['reason'])
            self.assertEqual(report['summary'], {'pass': 0, 'fail': 0, 'error': len(report['results'])})
            self.assertEqual(len(report['results']), len(report['inventory']))
            self.assertTrue(all(row['executionStatus'] == 'not_started' for row in report['results']))

    def test_dev_dispatch_rejects_acceptance_without_loading_or_starting_services(self):
        with tempfile.TemporaryDirectory() as temp:
            result = subprocess.run(['bash', 'scripts/deploy/deploy.sh', 'dev', 'accept'], cwd=ROOT,
                                    env=dict(os.environ, KC_DEPLOY_HOME=temp),
                                    capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode, 2, result.stdout+result.stderr)
            self.assertIn('acceptance runs only', result.stderr)


if __name__ == '__main__':
    unittest.main()
