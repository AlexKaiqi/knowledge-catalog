#!/usr/bin/env python3
"""Serialize automatic OpenSearch fixtures on this host and Docker daemon."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import signal
import stat
import subprocess
import sys
import time
import uuid

MINIMUM_VM_AVAILABLE = 1280 * 1024 * 1024
IMAGE = 'opensearchproject/opensearch:2.19.3'
OWNER_LABEL = 'kc.test.opensearch.owner'


class Cancelled(Exception):
    pass


class Lifecycle:
    def __init__(self):
        self.signal = None
        self.owner = uuid.uuid4().hex
        self.container = 'kc-local-opensearch-'+self.owner[:12]
        self.create_attempted = False

    def cancel(self, signum, _frame):
        self.signal = signum

    def check(self):
        if self.signal is not None:
            raise Cancelled()

    def command(self, args, timeout=30, cleanup=False, env=None, stream=False):
        if not cleanup:
            self.check()
        process = subprocess.Popen(args, stdout=None if stream else subprocess.PIPE,
            stderr=None if stream else subprocess.PIPE, text=True, env=env, start_new_session=True)
        end = None if timeout is None else time.monotonic()+timeout
        try:
            while True:
                try:
                    out, err = process.communicate(timeout=.1)
                    return subprocess.CompletedProcess(args, process.returncode, out, err)
                except subprocess.TimeoutExpired:
                    if not cleanup:
                        self.check()
                    if end is not None and time.monotonic()>=end:
                        raise TimeoutError('command exceeded its bounded wait: '+args[0])
        finally:
            if process.poll() is None or (self.signal is not None and not cleanup):
                self.stop_group(process)

    @staticmethod
    def stop_group(process):
        try:
            os.killpg(process.pid, signal.SIGTERM)
        except ProcessLookupError:
            process.communicate()
            return
        try:
            process.communicate(timeout=5)
        except subprocess.TimeoutExpired:
            pass
        # The shell can exit before a TERM-ignoring descendant. Reap the shell,
        # then end any remaining members of only this created process group.
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        except PermissionError:
            # Darwin may report EPERM for a group containing only its reaped
            # leader. A leader that is still alive must not be treated as done.
            if process.poll() is None:
                raise
        process.communicate()

    def docker(self, *args, cleanup=False):
        result = self.command(['docker', *args], cleanup=cleanup)
        if result.returncode:
            raise RuntimeError('Docker '+args[0]+' failed (exit '+str(result.returncode)+'): '+result.stderr.strip())
        return result.stdout.strip()

    def cleanup(self):
        if not self.create_attempted:
            return True
        try:
            owner = self.docker('inspect', self.container, '--format',
                '{{index .Config.Labels "'+OWNER_LABEL+'"}}', cleanup=True)
            if owner != self.owner:
                raise RuntimeError('container owner mismatch; refusing removal')
            self.docker('rm', '-f', self.container, cleanup=True)
            return True
        except (OSError, RuntimeError, TimeoutError) as error:
            print('FAIL: automatic OpenSearch cleanup: '+str(error), file=sys.stderr)
            return False

    def vm_budget(self, image):
        # cat executes in the daemon's Linux VM; never infer VM memory from the host.
        raw = self.docker('run', '--rm', '--pull=never', '--network=none',
            '--memory=32m', '--memory-swap=32m', '--entrypoint', 'cat', image, '/proc/meminfo')
        values = {}
        for line in raw.splitlines():
            key, separator, value = line.partition(':')
            if separator and key in ('MemTotal', 'MemAvailable'):
                fields = value.split()
                if len(fields)!=2 or fields[1]!='kB':
                    raise ValueError('Docker VM memory probe returned invalid units')
                values[key] = int(fields[0])*1024
        total, available = values.get('MemTotal', 0), values.get('MemAvailable', -1)
        if not 0<=available<=total or total==0:
            raise ValueError('Docker VM memory probe did not return valid MemAvailable')
        print(f'Automatic OpenSearch VM budget: available={available} required={MINIMUM_VM_AVAILABLE} bytes',
              file=sys.stderr, flush=True)
        if available<MINIMUM_VM_AVAILABLE:
            raise ValueError('Docker VM available memory is below the automatic OpenSearch startup budget; no Java fixture or tests were started')
        return values

    def record_lease(self, lease, pending):
        lease.seek(0)
        lease.truncate()
        lease.write(json.dumps({'pid':os.getpid(),'container':self.container,
            'owner':self.owner,'pendingCleanup':pending})+'\n')
        lease.flush()

    def run(self, image, command):
        daemon = self.docker('info', '--format', '{{.ID}}')
        if not daemon:
            raise ValueError('Docker daemon identity is empty; automatic fixture lease unavailable')
        key = hashlib.sha256(daemon.encode()).hexdigest()
        # Fixed host-wide location: checkout paths and TMPDIR cannot split the lease.
        path = Path('/tmp')/('kc-opensearch-auto-'+key+'.lock')
        fd = os.open(path, os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, 'r+') as lease:
            info = os.fstat(lease.fileno())
            if not stat.S_ISREG(info.st_mode) or info.st_uid!=os.getuid():
                raise ValueError('automatic OpenSearch lease is not an owned regular file')
            waiting = False
            while True:
                self.check()
                try:
                    fcntl.flock(lease, fcntl.LOCK_EX|fcntl.LOCK_NB)
                    break
                except BlockingIOError:
                    if not waiting:
                        print('Waiting for this host/Docker daemon automatic OpenSearch lease', file=sys.stderr, flush=True)
                        waiting = True
                    time.sleep(.1)
            # Keep the lock inode; unlinking it would let another process lock a new file.
            previous = lease.read().strip()
            previous = json.loads(previous) if previous else {}
            if previous.get('pendingCleanup'):
                # An empty current inventory cannot prove a cancelled Docker
                # create request has settled. Only explicit owner disposition
                # may clear this fence, under the same locked inode.
                raise ValueError('previous automatic OpenSearch cleanup remains unresolved; owner disposition is required before another instance can start')
            self.record_lease(lease, False)
            result = 1
            try:
                self.docker('image', 'inspect', image, '--format', '{{.Id}}')
                self.vm_budget(image)
                self.check()
                self.record_lease(lease, True)
                self.create_attempted = True
                self.docker('run', '--pull=never', '-d', '--name', self.container,
                    '--label', OWNER_LABEL+'='+self.owner, '-p', '127.0.0.1::9200',
                    '-e', 'discovery.type=single-node', '-e', 'DISABLE_INSTALL_DEMO_CONFIG=true',
                    '-e', 'DISABLE_SECURITY_PLUGIN=true', '-e', 'OPENSEARCH_JAVA_OPTS=-Xms512m -Xmx512m', image)
                port = self.docker('port', self.container, '9200/tcp').rsplit(':', 1)[-1]
                endpoint = 'http://127.0.0.1:'+str(int(port))
                for _ in range(60):
                    self.check()
                    health = self.command(['curl','--connect-timeout','2','--max-time','3',
                        '-fsS',endpoint+'/_cluster/health'],timeout=5)
                    if health.returncode==0:
                        env = dict(os.environ, KC_TEST_OPENSEARCH_URL=endpoint)
                        child = self.command(command, timeout=None, env=env, stream=True)
                        result = child.returncode if child.returncode>=0 else 128-child.returncode
                        break
                    time.sleep(2)
                else:
                    print(self.docker('logs','--tail','200',self.container),file=sys.stderr)
                    raise RuntimeError('automatic OpenSearch readiness timeout')
            finally:
                if self.cleanup():
                    self.record_lease(lease, False)
                elif result==0:
                    result = 1
            self.check()
            return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--image', default=IMAGE)
    parser.add_argument('command', nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command[1:] if args.command[:1]==['--'] else args.command
    if not command:
        parser.error('a test command is required')
    lifecycle = Lifecycle()
    for signum in (signal.SIGINT, signal.SIGTERM):
        signal.signal(signum, lifecycle.cancel)
    try:
        return lifecycle.run(args.image, command)
    except Cancelled:
        return 128+lifecycle.signal
    except (OSError, RuntimeError, TimeoutError, ValueError) as error:
        print('FAIL: automatic OpenSearch fixture: '+str(error),file=sys.stderr)
        return 1


if __name__=='__main__':
    raise SystemExit(main())
