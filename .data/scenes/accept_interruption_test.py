"""Real signals at the acceptance entry, without a deployment or services."""
from __future__ import annotations

import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest


HERE = Path(__file__).resolve().parent
FIXTURE = r'''import atexit, datetime, io, json, os, signal, sys, time, types, urllib.request
from pathlib import Path
root = Path(os.environ['INTERRUPTION_FIXTURE'])
window = os.environ['INTERRUPTION_WINDOW']
initial_handlers = {sig: signal.getsignal(sig) for sig in (signal.SIGINT, signal.SIGTERM)}
def final_handlers():
    (root/'restored.json').write_text(json.dumps({str(sig): signal.getsignal(sig) == handler
                                               for sig, handler in initial_handlers.items()}))
atexit.register(final_handlers)
def gate(name):
    (root/'window-opened').write_text(datetime.datetime.now(datetime.timezone.utc).isoformat())
    (root/'window').write_text(name)
    deadline = time.monotonic()+10
    while not (root/'release').exists():
        if time.monotonic() >= deadline:
            raise RuntimeError('fixture window was not released')
        time.sleep(.01)
def observe_signals():
    def observed(signum, frame, delegate):
        path = root/'received'
        count = int(path.read_text()) if path.exists() else 0
        path.write_text(str(count+1))
        return delegate(signum, frame)
    for signum in initial_handlers:
        delegate = signal.getsignal(signum)
        signal.signal(signum, lambda sig, frame, delegate=delegate: observed(sig, frame, delegate))
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
original_replace = Path.replace
publication_gated = False
def replace(source, target):
    global publication_gated
    if Path(target).name == 'acceptance.json':
        count = root/'replacements'
        count.write_text(str(int(count.read_text())+1 if count.exists() else 1))
        if window == 'publish' and not publication_gated:
            publication_gated = True
            gate('publish')
    return original_replace(source, target)
Path.replace = replace
module = types.ModuleType('accept_world')
class World:
    def __init__(self, plans):
        self.prefix = 'accept-0123456789abcdef-'
        self.cleanup_report, self.events = {}, []
        self.calls, self.closes = 0, 0
    def construct(self):
        observe_signals()
        if window == 'construct':
            gate('construct')
        if window == 'pending':
            pending_gated = False
            def before_dispatch(frame, event, arg):
                nonlocal pending_gated
                if not pending_gated and event == 'line' and frame.f_code.co_name == 'run' and frame.f_code.co_filename.endswith('/accept.py'):
                    pending_gated = True
                    sys.settrace(None)
                    gate('pending')
                return before_dispatch
            sys.settrace(before_dispatch)
    def probe(self, spec_id):
        self.calls += 1
        self.events.append({'probe': spec_id, 'state': 'started'})
        if window == 'probe' and self.calls == 2:
            gate('probe')
        self.events.append({'probe': spec_id, 'state': 'completed'})
    def close(self):
        self.closes += 1
        self.cleanup_report['calls'] = self.closes
        if window == 'close':
            gate('close')
        self.cleanup_report['completed'] = True
        if os.environ['INTERRUPTION_CLEANUP_ERROR'] == '1':
            raise RuntimeError('owned cleanup failed')
module.World = World
sys.modules['accept_world'] = module
'''


class AcceptanceInterruptionTest(unittest.TestCase):
    def wait_for(self, process, condition, description):
        deadline = time.monotonic()+10
        while time.monotonic() < deadline:
            if condition():
                return
            if process.poll() is not None:
                stdout, stderr = process.communicate()
                self.fail(f'child exited before {description}: {process.returncode}\n{stdout}\n{stderr}')
            time.sleep(.01)
        self.fail('child did not reach '+description)

    def run_entry(self, window='', *, repetitions=1, cleanup_error=False):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root/'sitecustomize.py').write_text(FIXTURE)
            stack, output = root/'stack', root/'report'
            stack.mkdir()
            output.mkdir()
            (stack/'compose.env').write_text('')
            if window == 'publish':
                (output/'acceptance.json').write_text('prior complete report')
            ids = ['accept-search-writer-rejects-index', 'accept-index-reject-shape',
                   'accept-index-cross-repo-schema-ref'] if window == 'probe' else ['liveaccess-plugin-help']
            command = [sys.executable, str(HERE/'accept.py')]
            for spec_id in ids:
                command += ['--select', spec_id]
            environment = dict(os.environ, PYTHONPATH=str(root), KC_DEPLOY_HOME=str(stack),
                               KC_DEPLOY_PROFILE='local', KC_VALIDATION_RUN_DIR=str(output),
                               KC_VALIDATION_RUN_ID='interruption-fixture',
                               KC_VALIDATION_SOURCE_FINGERPRINT='interruption-source',
                               INTERRUPTION_FIXTURE=str(root), INTERRUPTION_WINDOW=window,
                               INTERRUPTION_CLEANUP_ERROR=str(int(cleanup_error)),
                               KC_SERVER_URL='http://server.invalid', KC_ACCEPT_LAKEFS_URL='http://lakefs.invalid',
                               KC_ACCEPT_OPENSEARCH_URL='http://search.invalid', KC_ACCEPT_ORIGIN_URL='http://origin.invalid',
                               KC_ACCEPT_UNTRUSTED_ORIGIN_URL='http://untrusted.invalid',
                               KC_ACCEPT_ALT_ORIGIN_URL='http://alt.invalid')
            process = subprocess.Popen(command, env=environment, stdout=subprocess.PIPE,
                                       stderr=subprocess.PIPE, text=True)
            try:
                if window:
                    self.wait_for(process, lambda: (root/'window').exists(), window+' window')
                    self.assertEqual((root/'window').read_text(), window)
                    for count in range(1, repetitions+1):
                        process.send_signal(signal.SIGTERM)
                        self.wait_for(process, lambda: (root/'received').exists() and
                                      (root/'received').read_text() == str(count), 'SIGTERM delivery')
                    if window == 'publish':
                        self.assertEqual((output/'acceptance.json').read_text(), 'prior complete report',
                                         'a signal must not expose a partial replacement')
                    (root/'release').touch()
                stdout, stderr = process.communicate(timeout=15)
                self.assertTrue((output/'acceptance.json').exists(), stdout+stderr)
                report = json.loads((output/'acceptance.json').read_text())
                if window == 'probe':
                    self.assertLessEqual(report['results'][1]['startedAt'], (root/'window-opened').read_text(),
                                         'the interrupted probe lost its actual start time')
                restored = json.loads((root/'restored.json').read_text())
                self.assertTrue(all(restored.values()), 'signal handlers were not restored')
                self.assertEqual(list(output.glob('.acceptance-*.json')), [], 'report temporary file leaked')
                self.assertEqual(report['cleanup']['calls'], 1, 'cleanup was retried')
                self.assertTrue(report['cleanup']['completed'])
                replacements = int((root/'replacements').read_text())
                return process.returncode, report, replacements
            finally:
                if process.poll() is None:
                    process.kill()
                    process.communicate()

    def test_sigterm_during_probe_preserves_completed_and_unstarted_specs(self):
        code, report, _ = self.run_entry('probe')
        self.assertEqual(code, 143)
        self.assertTrue(report['interrupted'])
        self.assertEqual(report['summary'], {'pass': 1, 'fail': 0, 'error': 2})
        completed, interrupted, unstarted = report['results']
        self.assertEqual((completed['status'], completed['executionStatus']), ('pass', 'completed'))
        self.assertEqual((interrupted['status'], interrupted['executionStatus']), ('error', 'interrupted'))
        self.assertEqual(interrupted['interrupted'], signal.SIGTERM)
        self.assertLessEqual(interrupted['startedAt'], interrupted['finishedAt'])
        self.assertEqual(unstarted['executionStatus'], 'not_started')
        self.assertNotIn('interrupted', unstarted)
        self.assertEqual([event['state'] for event in report['events']], ['started', 'completed', 'started'])
        self.assertIsNone(report['cleanupError'])

    def test_sigterm_during_construct_does_not_start_a_probe(self):
        code, report, _ = self.run_entry('construct')
        self.assertEqual(code, 143)
        self.assertEqual(report['prestate']['interrupted'], signal.SIGTERM)
        self.assertEqual(report['events'], [])
        self.assertEqual(report['results'][0]['executionStatus'], 'not_started')

    def test_sigterm_before_dispatch_keeps_the_probe_unstarted(self):
        code, report, _ = self.run_entry('pending')
        self.assertEqual(code, 143)
        self.assertEqual(report['prestate']['status'], 'pass')
        self.assertEqual(report['events'], [])
        self.assertEqual(report['results'][0]['executionStatus'], 'not_started')

    def test_sigterm_during_close_keeps_completed_results_and_finishes_cleanup(self):
        code, report, replacements = self.run_entry('close')
        self.assertEqual(code, 143)
        self.assertTrue(report['interrupted'])
        self.assertEqual(report['summary'], {'pass': 1, 'fail': 0, 'error': 0})
        self.assertEqual(report['results'][0]['executionStatus'], 'completed')
        self.assertIsNone(report['cleanupError'])
        self.assertEqual(replacements, 1)

    def test_repeated_sigterm_during_close_does_not_restart_cleanup(self):
        code, report, _ = self.run_entry('close', repetitions=2)
        self.assertEqual(code, 143)
        self.assertTrue(report['interrupted'])
        self.assertEqual(report['results'][0]['status'], 'pass')

    def test_cleanup_error_is_preserved_with_sigterm(self):
        code, report, _ = self.run_entry('close', cleanup_error=True)
        self.assertEqual(code, 143)
        self.assertEqual(report['cleanupError'], 'owned cleanup failed')
        self.assertEqual(report['results'][0]['status'], 'pass')

    def test_cleanup_error_without_a_signal_remains_a_failure(self):
        code, report, _ = self.run_entry(cleanup_error=True)
        self.assertEqual(code, 1)
        self.assertFalse(report['interrupted'])
        self.assertEqual(report['cleanupError'], 'owned cleanup failed')

    def test_sigterm_while_publishing_keeps_atomic_report_and_records_interruption(self):
        code, report, replacements = self.run_entry('publish')
        self.assertEqual(code, 143)
        self.assertTrue(report['interrupted'])
        self.assertEqual(report['summary'], {'pass': 1, 'fail': 0, 'error': 0})
        self.assertEqual(replacements, 2, 'late cancellation needs one bounded report replacement')


if __name__ == '__main__':
    unittest.main()
