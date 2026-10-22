"""Owned-resource and capacity regressions; no OpenSearch process is started."""
import importlib.util
import json
import os
import signal
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

SCRIPT = Path(__file__).with_name('opensearch_fixture.py')


class SearchFixtureTest(unittest.TestCase):
    def load(self):
        spec = importlib.util.spec_from_file_location('search_fixture', SCRIPT)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module

    def run_signal_fixture(self, source, child_pid, timeout=8):
        process = subprocess.Popen([sys.executable, '-c', source], stdout=subprocess.PIPE,
                                   stderr=subprocess.PIPE, text=True, start_new_session=True)
        try:
            stdout, stderr = process.communicate(timeout=timeout)
            return subprocess.CompletedProcess(process.args, process.returncode, stdout, stderr)
        except subprocess.TimeoutExpired:
            # The tested wrapper creates another session; kill that exact test
            # child before the outer process, otherwise inherited pipes can hang.
            if child_pid.exists():
                try:
                    os.killpg(int(child_pid.read_text()), signal.SIGKILL)
                except ProcessLookupError:
                    pass
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGKILL)
            process.communicate(timeout=3)
            self.fail('signal fixture exceeded its bounded exit; owned processes were killed')
        finally:
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGKILL)
                process.communicate(timeout=3)

    def server(self, free=20, initializing=0):
        calls = []
        prefix = 'kc-test-0123456789abcdef0123456789abcdef-'
        indices = {prefix + 'kc-proj-one', 'kc-scene-someone-else-kc-proj-one', 'business-data'}
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass
            def do_GET(self):
                calls.append(('GET', self.path))
                if self.path.startswith('/_cluster/settings'):
                    body = {'defaults': {'cluster': {'max_shards_per_node': '1000'}}}
                elif self.path.startswith('/_cluster/health'):
                    body = {'number_of_data_nodes': 1, 'active_shards': 1000-free-initializing,
                            'initializing_shards': initializing, 'unassigned_shards': 0}
                elif self.path.startswith('/_nodes'):
                    body = {'nodes': {'one': {'roles': ['cluster_manager', 'data']}}}
                elif self.path.startswith('/_all/_settings'):
                    body = {'fixture': {'settings': {'index.number_of_shards': str(1000-free), 'index.number_of_replicas': '0'}}}
                else:
                    body = [{'index': index} for index in indices]
                self.send_response(200)
                self.end_headers()
                self.wfile.write(json.dumps(body).encode())
            def do_DELETE(self):
                calls.append(('DELETE', self.path))
                indices.discard(self.path[1:])
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b'{"acknowledged":true}')
            def do_PUT(self):
                calls.append(('PUT', self.path))
                indices.add(self.path[1:])
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b'{"acknowledged":true}')
        server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        return 'http://127.0.0.1:' + str(server.server_port), prefix, calls, indices

    def test_capacity_prevents_child_and_never_deletes_existing_indices(self):
        endpoint, _, calls, _ = self.server(free=0)
        with tempfile.TemporaryDirectory() as temp:
            marker = Path(temp)/'started'
            result = subprocess.run([sys.executable, str(SCRIPT), 'run', '--endpoint', endpoint,
                '--', str(SCRIPT.parent/'testsuite.sh'), 'lakefs'],
                capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode, 2, result.stderr)
            self.assertIn('capacity', result.stderr)
            self.assertIn('1000', result.stderr)
            self.assertFalse(marker.exists())
            self.assertFalse(any(method == 'DELETE' for method, _ in calls))

    def test_non_scene_commands_are_rejected_before_accessing_shared_search(self):
        for command in (["go", "test", "-run", "^TestConsumeDataset$", "./cli"],
                        ["go", "test", "-run", ".*", "./cli"],
                        ["go", "test", "-run", "TestProductScenes", "./cli"],
                        ["bash", "-c", "go test ./cli"],
                        ["go", "test", "-run", "TestProductScenes|TestConsumeDataset", "./cli"]):
            with self.subTest(command=command):
                result = subprocess.run([sys.executable, str(SCRIPT), "run", "--endpoint",
                    "http://127.0.0.1:1", "--", *command], capture_output=True, text=True, timeout=5)
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertIn("scene namespace", result.stderr)
                self.assertNotIn("Connection refused", result.stderr)

    def test_supported_commands_use_only_the_scene_namespace_transport(self):
        module = self.load()
        for command in ([str(SCRIPT.parent/"testsuite.sh"), "lakefs"],
                        ["go", "test", "-count=1", "-run", "^(TestProductScenes|TestMetricPermissionScenes)$", "./cli"],
                        ["go", "test", "./cli", "-run=^TestSceneProjectionNamespaceUsesInvocationOwnership$"]):
            with self.subTest(command=command):
                module.require_scene_command(command)
        for command in ([str(SCRIPT.parent/"testsuite.sh"), "contracts"],
                        ["go", "test", "-run", "TestProductScenes", "./..."],
                        ["go", "test", "-run", "TestProductScenes/anything", "./cli"],
                        ["go", "test", "-run", "TestProductScenes", "-run", ".*", "./cli"]):
            with self.subTest(command=command), self.assertRaisesRegex(ValueError, "scene namespace"):
                module.require_scene_command(command)

    def test_cleanup_deletes_exact_owned_names_only(self):
        endpoint, prefix, calls, indices = self.server()
        module = self.load()
        cleaned = module.cleanup(endpoint, prefix)
        self.assertEqual(cleaned, [prefix + 'kc-proj-one'])
        self.assertEqual(indices, {'kc-scene-someone-else-kc-proj-one', 'business-data'})
        self.assertEqual([p for method, p in calls if method == 'DELETE'], ['/' + prefix + 'kc-proj-one'])

    def test_guard_reclaims_only_what_the_group_added(self):
        # Groups without a run-owned prefix cannot be cleaned by name. Their
        # ownership is the difference against a snapshot, so an index that
        # existed before the group started is somebody else's and must survive,
        # including one whose name merely looks like a prefix this run might own.
        endpoint, prefix, calls, indices = self.server()
        module = self.load()
        before = module.inventory(endpoint)
        self.assertIn(prefix + 'kc-proj-one', before)
        indices.add('kc-proj-added-by-group')
        result = module.cleanup_added(endpoint, before)
        self.assertEqual(result['added'], ['kc-proj-added-by-group'])
        self.assertEqual(result['failed'], [])
        self.assertEqual(result['leaked'], [])
        self.assertEqual(indices, {prefix + 'kc-proj-one',
                                   'kc-scene-someone-else-kc-proj-one', 'business-data'})
        deletes = [path for method, path in calls if method == 'DELETE']
        self.assertEqual(deletes, ['/kc-proj-added-by-group'])

    def test_guard_reports_a_surviving_addition_instead_of_passing(self):
        # A cleanup that cannot remove what it added must fail the run: silently
        # leaving the index is the leak this guard exists to prevent.
        module = self.load()
        survivor = 'kc-proj-survives'
        calls = []
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass
            def do_GET(self):
                calls.append(('GET', self.path))
                self.send_response(200)
                self.end_headers()
                self.wfile.write(json.dumps([{'index': survivor}]).encode())
            def do_DELETE(self):
                calls.append(('DELETE', self.path))
                self.send_response(500)
                self.end_headers()
                self.wfile.write(b'{"error":"refused"}')
        server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        endpoint = 'http://127.0.0.1:' + str(server.server_port)
        result = module.cleanup_added(endpoint, [])
        self.assertEqual(result['added'], [survivor])
        self.assertEqual(result['leaked'], [survivor])
        self.assertEqual(len(result['failed']), 1)

    def run_guarded_group(self, group, exit_status):
        # A group bound to an explicitly configured instance has no owned prefix,
        # so the suite must re-enter itself through the guard. Both halves matter:
        # the index it created has to be reclaimed, and its own failure status has
        # to survive the guard instead of being read as a successful invocation.
        endpoint, _, calls, indices = self.server()
        before = set(indices)
        with tempfile.TemporaryDirectory() as temp:
            go = Path(temp)/'go'
            go.write_text(f'#!{sys.executable}\n'+f'''import urllib.request
request=urllib.request.Request({endpoint!r}+'/kc-proj-group-g-one',method='PUT')
with urllib.request.urlopen(request) as response: response.read()
raise SystemExit({exit_status})
''')
            go.chmod(0o755)
            result = subprocess.run(['bash', str(SCRIPT.parent/'testsuite.sh'), group],
                env=dict(os.environ, GO=str(go), KC_TEST_OPENSEARCH_URL=endpoint,
                         KC_VALIDATION_RUN_ID='guarded-group-offline'),
                capture_output=True, text=True, timeout=30)
        return result, calls, indices, before

    def test_shared_instance_group_is_guarded_and_keeps_its_failure_status(self):
        result, calls, indices, before = self.run_guarded_group('boundary', 73)
        self.assertEqual(result.returncode, 73, result.stdout+result.stderr)
        self.assertNotIn('can only `return', result.stderr)
        created = [path[1:] for method, path in calls if method == 'PUT']
        deleted = [path[1:] for method, path in calls if method == 'DELETE']
        self.assertTrue(created)
        self.assertEqual(deleted, created)
        self.assertEqual(indices, before)

    def test_shared_instance_group_reports_success_when_the_group_passes(self):
        result, calls, indices, before = self.run_guarded_group('boundary', 0)
        self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertIn('kc-proj-group-g-one', [path[1:] for method, path in calls if method == 'DELETE'])
        self.assertEqual(indices, before)

    def test_contracts_group_runs_every_step_not_only_the_first(self):
        # contracts is component + boundary + e2e. Passing the three functions as
        # one list ran only the first and handed the others to it as arguments, so
        # the group looked green while two thirds never executed. Each step issues
        # its own go test invocation, so the invocation count is the witness.
        endpoint, _, calls, indices = self.server()
        before = set(indices)
        with tempfile.TemporaryDirectory() as temp:
            counter = Path(temp)/'invocations'
            go = Path(temp)/'go'
            go.write_text(f'#!{sys.executable}\n'+f'''import pathlib,sys,urllib.request
if sys.argv[1:2]==['list']:
    print('kc/object'); print('kc/internal/arch'); print('kc/cli')
    raise SystemExit(0)
log=pathlib.Path({str(counter)!r})
log.write_text((log.read_text() if log.exists() else '')+' '.join(sys.argv[3:])+'\\n')
urllib.request.urlopen(urllib.request.Request({endpoint!r}+'/kc-proj-step-g-one',method='PUT')).read()
''')
            go.chmod(0o755)
            result = subprocess.run(['bash', str(SCRIPT.parent/'testsuite.sh'), 'contracts'],
                env=dict(os.environ, GO=str(go), KC_TEST_OPENSEARCH_URL=endpoint,
                         KC_VALIDATION_RUN_ID='contracts-steps-offline'),
                capture_output=True, text=True, timeout=60)
            steps = [line for line in counter.read_text().splitlines() if line.strip()]
            deleted = [path[1:] for method, path in calls if method == 'DELETE']
        self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertEqual(len(steps), 3, 'contracts must run component, boundary and e2e: '+repr(steps))
        self.assertTrue(any('internal/arch' in step for step in steps), 'boundary step missing: '+repr(steps))
        self.assertTrue(any('./cli' in step for step in steps), 'e2e step missing: '+repr(steps))
        self.assertTrue(deleted, 'added indices must be reclaimed')
        self.assertEqual(indices, before)

    def test_cleanup_refuses_an_incomplete_prefix(self):
        endpoint, prefix, _, _ = self.server()
        module = self.load()
        for bad in ('', 'kc-', 'kc-test-*', prefix+'another-'):
            with self.subTest(prefix=bad), self.assertRaises(ValueError):
                module.cleanup(endpoint, bad)

    def test_testsuite_lakefs_uses_owned_wrapper_and_preserves_test_failure(self):
        endpoint, _, calls, indices = self.server()
        before = set(indices)
        with tempfile.TemporaryDirectory() as temp:
            go = Path(temp)/'go'
            go.write_text(f'#!{sys.executable}\n'+'''import os,re,sys,urllib.request
prefix=os.environ.get('KC_TEST_OPENSEARCH_RUN_PREFIX','')
if not re.fullmatch('kc-test-[0-9a-f]{32}-',prefix):
    print('missing invocation ownership',file=sys.stderr);sys.exit(98)
request=urllib.request.Request(os.environ['KC_TEST_OPENSEARCH_URL']+'/'+prefix+'kc-proj-fixture-g-one',method='PUT')
with urllib.request.urlopen(request) as response: response.read()
sys.exit(73)
''')
            go.chmod(0o755)
            result = subprocess.run(['bash', str(SCRIPT.parent/'testsuite.sh'), 'lakefs'],
                env=dict(os.environ, GO=str(go), KC_TEST_OPENSEARCH_URL=endpoint,
                         KC_VALIDATION_RUN_ID='owned-offline', KC_TEST_OPENSEARCH_RUN_PREFIX=''),
                capture_output=True, text=True, timeout=15)
            self.assertEqual(result.returncode, 73, result.stdout+result.stderr)
            created = [path[1:] for method, path in calls if method == 'PUT']
            deleted = [path[1:] for method, path in calls if method == 'DELETE']
            self.assertEqual(len(created), 1)
            self.assertRegex(created[0], r'^kc-test-[0-9a-f]{32}-kc-proj-fixture-g-one$')
            self.assertEqual(deleted, created)
            self.assertEqual(indices, before)

    def test_failure_and_interrupt_clean_only_child_prefix(self):
        module = self.load()
        endpoint, _, _, _ = self.server()
        for status in (0, 73):
            observed = []
            original = module.cleanup
            module.cleanup = lambda url, prefix: observed.append(prefix) or []
            try:
                code = module.run_owned(endpoint, [sys.executable, '-c', f'import sys; sys.exit({status})'])
            finally:
                module.cleanup = original
            self.assertEqual(code, status)
            self.assertEqual(len(observed), 1)
            self.assertRegex(observed[0], r'^kc-test-[0-9a-f]{32}-$')
        # An actual SIGTERM must run cleanup, rather than depend on Go t.Cleanup.
        with tempfile.TemporaryDirectory() as temp:
            marker = Path(temp)/'owned-prefix'
            child_pid = Path(temp)/'child.pid'
            child = (f'import os,signal,time; from pathlib import Path; Path({str(child_pid)!r}).write_text(str(os.getpid())); '
                     'os.kill(os.getppid(),signal.SIGTERM); time.sleep(30)')
            source = (
                "import importlib.util, os, signal, sys; "
                f"s=importlib.util.spec_from_file_location('fixture', {str(SCRIPT)!r}); "
                "m=importlib.util.module_from_spec(s); s.loader.exec_module(m); "
                f"m.cleanup=lambda url,prefix: __import__('pathlib').Path({str(marker)!r}).write_text(prefix) or []; "
                f"sys.exit(m.run_owned({endpoint!r}, [sys.executable, '-c', {child!r}]))")
            result = self.run_signal_fixture(source, child_pid)
            self.assertEqual(result.returncode, 143, result.stderr)
            self.assertRegex(marker.read_text(), r'^kc-test-[0-9a-f]{32}-$')


    def test_initializing_shards_are_included_in_capacity(self):
        endpoint, _, _, _ = self.server(free=0, initializing=20)
        with self.assertRaisesRegex(ValueError, '1000/1000'):
            self.load().capacity(endpoint)

    def test_sigterm_ignoring_child_and_pre_spawn_signal_have_bounded_cleanup(self):
        endpoint, _, _, _ = self.server()
        for before_spawn in (False, True):
            with self.subTest(before_spawn=before_spawn), tempfile.TemporaryDirectory() as temp:
                marker = Path(temp)/'cleanup'
                child_pid = Path(temp)/'child.pid'
                source = ("import importlib.util, os, signal, sys; "
                    f"s=importlib.util.spec_from_file_location('fixture', {str(SCRIPT)!r}); "
                    "m=importlib.util.module_from_spec(s); s.loader.exec_module(m); "
                    f"m.cleanup=lambda url,prefix: __import__('pathlib').Path({str(marker)!r}).write_text(prefix) or []; ")
                if before_spawn:
                    source += ("original=m.subprocess.Popen; "
                        "m.subprocess.Popen=lambda *a,**kw: (os.kill(os.getpid(),signal.SIGTERM), original(*a,**kw))[1]; ")
                    child = 'import time; time.sleep(30)'
                else:
                    child = 'import os,signal,time; signal.signal(signal.SIGTERM,signal.SIG_IGN); os.kill(os.getppid(),signal.SIGTERM); time.sleep(30)'
                child = f"import os; from pathlib import Path; Path({str(child_pid)!r}).write_text(str(os.getpid())); "+child
                source += f"sys.exit(m.run_owned({endpoint!r}, [sys.executable, '-c', {child!r}]))"
                result = self.run_signal_fixture(source, child_pid)
                self.assertEqual(result.returncode, 143, result.stderr)
                self.assertTrue(marker.exists(), 'cleanup must run after cancellation')


if __name__ == '__main__':
    unittest.main()
