#!/usr/bin/env python3
"""Serialize automatic OpenSearch fixtures on this host and Docker daemon."""
import argparse
from contextlib import contextmanager
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
FENCE_ENV = 'KC_OPENSEARCH_FENCE'
FENCE_DEFAULT = '/tmp/kc-opensearch-auto-fence.json'


class Cancelled(Exception):
    pass


class Fence:
    """Host-wide record of container creations whose removal is unproven.

    The per-daemon lease file cannot carry this fact. Its name is derived from
    the Docker daemon ID, so a daemon restart opens a new, empty lease and the
    previous fence disappears without any owner disposition: the documented
    guarantee that an unresolved cleanup blocks the next start held only while
    the daemon kept its identity. Container names are unique per owner, so a map
    keyed by container name survives that change, and the daemon recorded beside
    each entry says whether the container can still exist at all.
    """

    def __init__(self, path=None):
        self.path = Path(path) if path is not None else Path(os.environ.get(FENCE_ENV) or FENCE_DEFAULT)

    @contextmanager
    def _locked(self):
        fd = os.open(self.path, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
        try:
            info = os.fstat(fd)
            if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid():
                raise ValueError('automatic OpenSearch fence is not an owned regular file')

            def write(value):
                os.lseek(fd, 0, os.SEEK_SET)
                os.ftruncate(fd, 0)
                os.write(fd, (json.dumps(value, sort_keys=True) + '\n').encode())

            fcntl.flock(fd, fcntl.LOCK_EX)
            os.lseek(fd, 0, os.SEEK_SET)
            raw = b''
            while True:
                chunk = os.read(fd, 65536)
                if not chunk:
                    break
                raw += chunk
            if not raw.strip():
                value = {}
            else:
                try:
                    value = json.loads(raw)
                except ValueError:
                    raise ValueError('automatic OpenSearch fence is unreadable; dispose it explicitly')
                if not isinstance(value, dict):
                    raise ValueError('automatic OpenSearch fence is not a container map; dispose it explicitly')
            yield value, write
        finally:
            os.close(fd)

    def read(self):
        with self._locked() as (value, _):
            return value

    def mutate(self, change):
        with self._locked() as (value, write):
            result = change(value)
            write(value)
            return result

    def note(self, container, daemon, pid):
        """Record intent before a create request, so a cancelled create stays visible."""
        def change(value):
            value[container] = {'daemon': daemon, 'pid': pid, 'notedAt': int(time.time())}
        self.mutate(change)

    def clear(self, container):
        """Drop an entry only after the container is confirmed removed."""
        def change(value):
            value.pop(container, None)
        self.mutate(change)

    def assert_clear(self, present):
        """Refuse to start while any recorded container still exists.

        Presence is the fact that matters, and it is independent of the Docker
        daemon identity: a restarted daemon keeps its containers, so a container
        that survives is exactly the instance whose cleanup was never confirmed.
        An absent container settles its own create request, so it is dropped.
        """
        def change(value):
            remaining = []
            for name in sorted(value):
                if present(name):
                    remaining.append(name)
                else:
                    value.pop(name)
            return remaining
        entries = self.mutate(change)
        if entries:
            raise ValueError(
                'previous automatic OpenSearch cleanup remains unresolved for '
                + ', '.join(entries)
                + '; the container still exists. Stop automatic starts until the '
                'owner disposes of it explicitly')


class Lifecycle:
    def __init__(self):
        self.signal = None
        self.owner = uuid.uuid4().hex
        self.container = 'kc-local-opensearch-'+self.owner[:12]
        self.create_attempted = False
        self.fence = Fence()

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
            self.fence.clear(self.container)
            return True
        except (OSError, RuntimeError, TimeoutError, ValueError) as error:
            print('FAIL: automatic OpenSearch cleanup: '+str(error), file=sys.stderr)
            return False

    def container_exists(self, name):
        """True only when Docker positively reports the container."""
        result = subprocess.run(['docker', 'inspect', name, '--format', '{{.Id}}'],
                                capture_output=True, text=True)
        return result.returncode == 0 and bool(result.stdout.strip())

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
            # The lease is per daemon identity and is opened fresh after a daemon
            # restart; the host fence is what survives that, so it is checked
            # under the lease rather than instead of it.
            self.fence.assert_clear(self.container_exists)
            self.record_lease(lease, False)
            result = 1
            try:
                self.docker('image', 'inspect', image, '--format', '{{.Id}}')
                self.vm_budget(image)
                self.check()
                # Record intent before the request: a cancelled create may still
                # have produced a container, and only its absence settles that.
                self.fence.note(self.container, daemon, os.getpid())
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


def prune_leases(apply=None, directory='/tmp'):
    """Drop lease files that can never be read again.

    A lease file is named by the Docker daemon identity that created it, so once
    that daemon is gone nothing opens the file again: the record inside is not a
    fence, it is garbage. Two facts are checked before removal, and neither is
    'the file looks old':

    The lock must be obtainable. Holding LOCK_EX proves no process is using the
    lease, which is what makes removal safe rather than a race against a start.

    The recorded container must be absent. A lease whose container still exists
    describes a real instance, whatever its daemon, so it is kept and reported
    instead of deleted.
    """
    present = {}
    kept_exists, kept_busy, kept_current, removed, failed = [], [], [], [], []
    reclaimed = 0
    try:
        current = hashlib.sha256(
            subprocess.run(['docker', 'info', '--format', '{{.ID}}'],
                           capture_output=True, text=True).stdout.strip().encode()).hexdigest()
    except OSError:
        current = None
    for path in sorted(Path(directory).glob('kc-opensearch-auto-*.lock')):
        # Never remove the lease of the daemon that is current right now. A start
        # can open that very path at any moment, and unlinking a file while
        # another process is queued on its lock lets two starts hold "the" lease
        # against two different inodes. Its content is overwritten by the next
        # start anyway, so keeping it costs nothing.
        if current and path.name == 'kc-opensearch-auto-'+current+'.lock':
            kept_current.append(path.name)
            continue
        try:
            fd = os.open(path, os.O_RDWR | os.O_NOFOLLOW)
        except OSError as error:
            failed.append((path.name, str(error)))
            continue
        try:
            info = os.fstat(fd)
            if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid():
                failed.append((path.name, 'not an owned regular file'))
                continue
            try:
                fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except OSError:
                kept_busy.append(path.name)
                continue
            os.lseek(fd, 0, os.SEEK_SET)
            raw = b''
            while True:
                chunk = os.read(fd, 65536)
                if not chunk:
                    break
                raw += chunk
            try:
                record = json.loads(raw) if raw.strip() else {}
            except ValueError:
                record = None
            if not isinstance(record, dict):
                failed.append((path.name, 'unreadable record'))
                continue
            container = record.get('container')
            if container:
                if container not in present:
                    result = subprocess.run(['docker', 'inspect', container,
                                             '--format', '{{.Id}}'], capture_output=True, text=True)
                    present[container] = result.returncode == 0 and bool(result.stdout.strip())
                if present[container]:
                    kept_exists.append((path.name, container))
                    continue
            if apply:
                try:
                    reclaimed += info.st_size
                    os.unlink(path)
                    removed.append(path.name)
                except OSError as error:
                    failed.append((path.name, str(error)))
            else:
                reclaimed += info.st_size
                removed.append(path.name)
        finally:
            os.close(fd)
    return {'removed': removed, 'kept_exists': kept_exists,
            'kept_busy': kept_busy, 'kept_current': kept_current,
            'failed': failed, 'bytes': reclaimed}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--image', default=IMAGE)
    parser.add_argument('command', nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command[1:] if args.command[:1]==['--'] else args.command
    if command[:1]==['leases']:
        rest = command[1:]
        unknown = [item for item in rest if item!='--apply']
        if unknown:
            parser.error('unexpected lease argument: '+' '.join(unknown))
        result = prune_leases(apply='--apply' in rest)
        for name in result['removed']:
            print(('removed ' if '--apply' in rest else 'would remove ')+name)
        for name, container in result['kept_exists']:
            print('keep '+name+': container '+container+' still exists')
        for name in result['kept_busy']:
            print('keep '+name+': a process holds the lease')
        for name in result['kept_current']:
            print('keep '+name+': belongs to the current Docker daemon and is rewritten by the next start')
        for name, reason in result['failed']:
            print('keep '+name+': '+reason)
        verb = 'reclaimed' if '--apply' in rest else 'reclaimable'
        print('%d lease file(s) %s, %d kept, %d B %s'
              % (len(result['removed']), 'removed' if '--apply' in rest else 'to remove',
                 len(result['kept_exists'])+len(result['kept_busy'])
                 +len(result['kept_current'])+len(result['failed']),
                 result['bytes'], verb))
        if not '--apply' in rest and result['removed']:
            print('dry run; pass leases --apply to delete')
        return 0 if not result['failed'] else 1
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
