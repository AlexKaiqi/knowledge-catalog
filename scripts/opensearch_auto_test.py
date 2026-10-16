"""Automatic fixture lifecycle contracts; fake Docker never starts a service."""
import fcntl
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import unittest
import uuid

SCRIPTS = Path(__file__).resolve().parent
FAKE = r'''import fcntl,json,os,signal,sys,time
from pathlib import Path
root=Path(os.environ['AUTO_FIXTURE_ROOT'])
args=sys.argv[1:]; tool=Path(sys.argv[0]).name
token=os.environ.get('AUTO_FIXTURE_TOKEN','one')
def record(action, **fields):
    with (root/'state.json').open('a+') as f:
        fcntl.flock(f,fcntl.LOCK_EX);f.seek(0);raw=f.read()
        state=json.loads(raw) if raw else {'active':{},'maxActive':0,'events':[]}
        event={'action':action,'token':token,'pid':os.getpid(),'args':args,**fields}
        state['events'].append(event)
        if action=='start':state['active'][fields['name']]=fields['owner']
        if action=='remove':state['active'].pop(fields['name'],None)
        state['maxActive']=max(state['maxActive'],len(state['active']))
        f.seek(0);f.truncate();json.dump(state,f);f.flush()
        return state
if tool=='docker':
    if args[0]=='info':
        record('info');print(os.environ['AUTO_FIXTURE_DAEMON']);sys.exit(0)
    if args[:2]==['image','inspect']:
        record('image');print('sha256:cached');sys.exit(0)
    if args[0]=='run':
        if '--entrypoint' in args:
            record('memory');mode=os.environ.get('AUTO_FIXTURE_MEMORY','high')
            print('MemTotal: 8388608 kB')
            if mode!='missing':print('MemAvailable: '+('65536' if mode=='low' else '4194304')+' kB')
            sys.exit(0)
        name=args[args.index('--name')+1]
        labels=[args[i+1] for i,x in enumerate(args)if x=='--label']
        owner=next((x.split('=',1)[1]for x in labels if x.startswith('kc.test.opensearch.owner=')), '')
        record('start',name=name,owner=owner);print('fake-container-id');sys.exit(0)
    if args[0]=='port':print('127.0.0.1:19200');sys.exit(0)
    if args[0]=='inspect':
        state=record('inspect');print(state['active'].get(args[1],''));sys.exit(0)
    if args[0]=='rm':
        status=int(os.environ.get('AUTO_FIXTURE_CLEANUP_EXIT','0'));record('cleanup_failed' if status else 'remove',name=args[-1]);sys.exit(status)
    if args[0]=='logs':sys.exit(0)
    sys.exit(97)
if tool=='curl':sys.exit(0)
if tool=='go':
    if args[0]=='list':print('kc/example');sys.exit(0)
    record('test');(root/('ready-'+token)).write_text(str(os.getpid()))
    if os.environ.get('AUTO_FIXTURE_IGNORE_TERM')=='1':signal.signal(signal.SIGTERM,signal.SIG_IGN)
    while (root/('hold-'+token)).exists():time.sleep(.03)
    sys.exit(int(os.environ.get('AUTO_FIXTURE_TEST_EXIT','0')))
sys.exit(98)
'''


class AutoSearchLeaseTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bins = self.root/'bin'
        self.bins.mkdir()
        for name in ('docker', 'go', 'curl'):
            file = self.bins/name
            file.write_text(f'#!{sys.executable}\n'+FAKE)
            file.chmod(0o755)
        self.daemon = 'offline-'+uuid.uuid4().hex
        self.children = []
        self.addCleanup(self.stop_children)

    def checkout(self, name):
        root = self.root/name
        scripts = root/'scripts'
        scripts.mkdir(parents=True)
        for filename in ('testsuite.sh', 'e2e-opensearch.sh', 'e2e-state-runtime-docker.sh',
                         'opensearch_auto.py'):
            source = SCRIPTS/filename
            if source.exists():
                shutil.copy2(source, scripts/filename)
        return root

    def start(self, checkout, token='one', group='component', **changes):
        env = dict(os.environ, PATH=str(self.bins)+os.pathsep+os.environ.get('PATH',''),
                   GO=str(self.bins/'go'), KC_VALIDATION_RUN_ID='offline-auto',
                   KC_VALIDATION_RUN_DIR=str(self.root/('evidence-'+token)),
                   KC_TEST_OPENSEARCH_URL='', AUTO_FIXTURE_ROOT=str(self.root),
                   AUTO_FIXTURE_DAEMON=self.daemon, AUTO_FIXTURE_TOKEN=token)
        env.update(changes)
        process = subprocess.Popen(['bash',str(checkout/'scripts/testsuite.sh'),group],
            env=env,cwd=checkout,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,
            start_new_session=True)
        self.children.append(process)
        return process

    def finish(self, process, timeout=12):
        out, err = process.communicate(timeout=timeout)
        return process.returncode, out+err

    def stop_children(self):
        for process in self.children:
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGKILL)
                process.communicate(timeout=3)
        for marker in self.root.glob('ready-*'):
            try:os.kill(int(marker.read_text()),signal.SIGKILL)
            except ProcessLookupError:pass

    def state(self):
        path=self.root/'state.json'
        if not path.exists():return {'events':[]}
        with path.open() as stream:
            fcntl.flock(stream,fcntl.LOCK_SH)
            raw=stream.read()
            return json.loads(raw) if raw else {'events':[]}

    def wait_ready(self, token):
        deadline=time.monotonic()+8
        while time.monotonic()<deadline:
            if (self.root/('ready-'+token)).exists():return
            time.sleep(.03)
        self.fail('fixture did not reach test child: '+str(self.state()))

    def wait_event(self, action, token):
        deadline=time.monotonic()+8
        while time.monotonic()<deadline:
            if any(e['action']==action and e['token']==token for e in self.state()['events']):return
            time.sleep(.03)
        self.fail('fixture event not observed: '+action+'/'+token)

    def test_same_daemon_serializes_two_checkouts_until_cleanup(self):
        a,b=self.checkout('a'),self.checkout('b')
        (self.root/'hold-one').touch()
        first=self.start(a);self.wait_ready('one')
        second=self.start(b,'two',KC_OPENSEARCH_IMAGE='another-cached-image')
        self.wait_event('info','two');time.sleep(.2)
        self.assertFalse((self.root/'ready-two').exists(), 'second checkout must wait for the daemon lease')
        (self.root/'hold-one').unlink()
        self.assertEqual(self.finish(first)[0],0)
        self.assertEqual(self.finish(second)[0],0)
        state=self.state()
        self.assertEqual(state['maxActive'],1)
        self.assertFalse(state['active'])
        events=state['events']
        remove=next(i for i,x in enumerate(events)if x['action']=='remove')
        starts=[i for i,x in enumerate(events)if x['action']=='start']
        self.assertLess(remove,starts[1], 'lease must cover cleanup, not only startup')
        memory_two=next(i for i,x in enumerate(events)if x['action']=='memory' and x['token']=='two')
        self.assertLess(remove,memory_two, 'VM budget must be sampled after acquiring the lease')

    def test_sigterm_releases_running_lease_and_cleans_before_next_child(self):
        a,b=self.checkout('a'),self.checkout('b')
        (self.root/'hold-one').touch()
        first=self.start(a);self.wait_ready('one')
        second=self.start(b,'two')
        first.send_signal(signal.SIGTERM)
        status,output=self.finish(first)
        self.assertEqual(status,143,output)
        self.assertEqual(self.finish(second)[0],0)
        self.assertFalse(self.state()['active'])
        self.assertEqual(self.state()['maxActive'],1)

    def test_sigterm_reaps_term_ignoring_descendants(self):
        checkout=self.checkout('a')
        (self.root/'hold-one').touch()
        process=self.start(checkout,AUTO_FIXTURE_IGNORE_TERM='1')
        self.wait_ready('one')
        pid=int((self.root/'ready-one').read_text())
        process.send_signal(signal.SIGTERM)
        status,output=self.finish(process)
        self.assertEqual(status,143,output)
        state=subprocess.run(['ps','-p',str(pid),'-o','stat='],capture_output=True,text=True).stdout.strip()
        self.assertTrue(not state or state.startswith('Z'), 'test descendant still running: '+state)
        self.assertFalse(self.state()['active'])

    def test_sigterm_interrupts_a_waiter_without_starting_another_instance(self):
        a,b=self.checkout('a'),self.checkout('b')
        (self.root/'hold-one').touch()
        first=self.start(a);self.wait_ready('one')
        second=self.start(b,'two');self.wait_event('info','two');time.sleep(.1)
        second.send_signal(signal.SIGTERM)
        status,output=self.finish(second)
        self.assertEqual(status,143,output)
        self.assertEqual(len([x for x in self.state()['events']if x['action']=='start']),1)
        (self.root/'hold-one').unlink()
        self.assertEqual(self.finish(first)[0],0)

    def test_low_or_unknown_vm_budget_fails_before_java_or_tests(self):
        checkout=self.checkout('a')
        for mode in ('low','missing'):
            with self.subTest(mode=mode):
                status,output=self.finish(self.start(checkout,mode,AUTO_FIXTURE_MEMORY=mode))
                self.assertNotEqual(status,0,output)
                self.assertIn('VM',output)
                self.assertFalse(any(e['action']in('start','test') for e in self.state()['events']))
        memory=[x for x in self.state()['events']if x['action']=='memory']
        self.assertEqual(len(memory),2)
        self.assertTrue(all('/proc/meminfo'in x['args'] for x in memory))

    def test_test_failure_and_cleanup_failure_preserve_nonzero_outcomes(self):
        checkout=self.checkout('a')
        for token,child,cleanup,expected in (('failed','73','0',73),
                                           ('cleanup','0','75',1),
                                           ('both','73','75',73)):
            with self.subTest(token=token):
                status,output=self.finish(self.start(checkout,token,
                    AUTO_FIXTURE_TEST_EXIT=child,AUTO_FIXTURE_CLEANUP_EXIT=cleanup,
                    AUTO_FIXTURE_DAEMON=self.daemon+'-'+token))
                self.assertEqual(status,expected,output)
                state=self.state()
                if cleanup!='0':
                    self.assertIn('cleanup',output.lower())
                    self.assertTrue(state['active'])
                    # Manual fixture cleanup between independent error cases.
                    state['active']={}
                    (self.root/'state.json').write_text(json.dumps(state))
                else:self.assertFalse(state['active'])

    def test_unresolved_cleanup_blocks_next_auto_spawn_without_removing_previous(self):
        checkout=self.checkout('a')
        status,output=self.finish(self.start(checkout,AUTO_FIXTURE_CLEANUP_EXIT='75'))
        self.assertEqual(status,1,output)
        before=dict(self.state()['active'])
        status,output=self.finish(self.start(checkout,'two'))
        self.assertNotEqual(status,0,output)
        self.assertIn('cleanup',output.lower())
        self.assertEqual(self.state()['active'],before)
        self.assertEqual(len([x for x in self.state()['events']if x['action']=='start']),1)

    def test_uncertain_cleanup_stays_fenced_when_current_inventory_is_empty(self):
        checkout=self.checkout('a')
        status,output=self.finish(self.start(checkout,AUTO_FIXTURE_CLEANUP_EXIT='75'))
        self.assertEqual(status,1,output)
        # An empty current inventory cannot prove an interrupted create request
        # has settled; preserve its uncertainty rather than authorizing overlap.
        state=self.state();state['active']={}
        (self.root/'state.json').write_text(json.dumps(state))
        status,output=self.finish(self.start(checkout,'two'))
        self.assertNotEqual(status,0,output)
        self.assertIn('cleanup',output.lower())
        self.assertEqual(len([x for x in self.state()['events']if x['action']=='start']),1)

    def test_explicit_endpoint_never_queries_docker_or_takes_auto_lease(self):
        status,output=self.finish(self.start(self.checkout('a'),
            KC_TEST_OPENSEARCH_URL='http://existing.invalid:9200',AUTO_FIXTURE_TEST_EXIT='73'))
        self.assertEqual(status,73,output)
        self.assertEqual([e['action']for e in self.state()['events']],['test'])

    def test_adapter_entry_reuses_the_already_leased_instance(self):
        status,output=self.finish(self.start(self.checkout('a'),group='adapters'))
        self.assertEqual(status,0,output)
        state=self.state()
        self.assertEqual(len([e for e in state['events']if e['action']=='start']),1)
        self.assertFalse(state['active'])


if __name__=='__main__':
    unittest.main()
